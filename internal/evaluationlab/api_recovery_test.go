package evaluationlab_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	"sync/atomic"
	"testing"
)

func authorizedAPIRequest(api *lab.API, method, path string, input any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(input)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Host = "127.0.0.1:8123"
	req.Header.Set("Origin", "http://127.0.0.1:8123")
	req.Header.Set("Authorization", api.Session().Authorization)
	req.Header.Set("X-Lab-CSRF", api.Session().CSRF)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, req)
	return response
}
func TestAPIStorageFailureKeepsFailedRunReadableAndStopsLaterAdmission(t *testing.T) {
	var commits atomic.Int64
	store, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{BeforeWrite: func(operation string) error {
		if operation == "rename:manifest.json" && commits.Add(1) == 3 {
			return errors.New("injected full disk")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	p := semanticPlanner(t)
	runner := lab.NewRunner(p, lab.RunnerOptions{Retain: store.Save, BeforeDispatch: store.Health})
	api, err := lab.NewAPI(p, runner, store, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	plan := factPlan(t, p)
	input := map[string]any{"plan": plan, "idempotencyToken": "persist-failure-token-0000000"}
	response := authorizedAPIRequest(api, "POST", "/lab/api/runs", input)
	var admitted struct {
		ID string `json:"id"`
	}
	json.Unmarshal(response.Body.Bytes(), &admitted)
	if response.Code != 201 {
		t.Fatal("initial admission failed")
	}
	result := awaitRun(t, runner, admitted.ID)
	if result.EngineState != "failed" || len(result.Dispatches) > 1 {
		t.Fatal("storage failure did not stop execution")
	}
	detail := authorizedAPIRequest(api, "GET", "/lab/api/runs/"+admitted.ID, nil)
	var record lab.RunRecord
	json.Unmarshal(detail.Body.Bytes(), &record)
	if detail.Code != 200 || record.EngineState != "failed" {
		t.Fatal("HTTP hid failed partial run")
	}
	if replay := authorizedAPIRequest(api, "POST", "/lab/api/runs", input); replay.Code != 200 {
		t.Fatal("failure broke idempotent lookup")
	}
	input["idempotencyToken"] = "later-admission-token-0000000"
	if later := authorizedAPIRequest(api, "POST", "/lab/api/runs", input); later.Code != 503 {
		t.Fatal("unhealthy storage admitted later execution")
	}
	unchanged, _ := runner.Read(admitted.ID)
	if len(unchanged.Dispatches) != len(result.Dispatches) {
		t.Fatal("HTTP failure redispatched")
	}
}
func TestAPIRestartReadsInterruptedHistoryAndRequiresCheckpointCursor(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	store, err := lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p := semanticPlanner(t)
	runner := lab.NewRunner(p, lab.RunnerOptions{Retain: store.Save, BeforeDispatch: store.Health})
	api, err := lab.NewAPI(p, runner, store, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	plan := factPlan(t, p)
	id, err := runner.Start(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, runner, id)
	response := authorizedAPIRequest(api, "GET", "/lab/api/runs/"+id+"/events", nil)
	var oldCursor struct {
		Epoch string `json:"epoch"`
	}
	json.Unmarshal(response.Body.Bytes(), &oldCursor)
	// Retain a synthetic last-before-crash checkpoint; preserve completed case
	// evidence while the logical run and one dispatch lack final confirmation.
	record.EngineState = "running"
	record.Dispatches[0].State = "attempted"
	if err = store.Save(record); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	freshPlanner := semanticPlanner(t)
	var dispatches atomic.Int64
	freshRunner := lab.NewRunner(freshPlanner, lab.RunnerOptions{Retain: store.Save, BeforeDispatch: func(ctx context.Context) error { dispatches.Add(1); return store.Health(ctx) }})
	freshAPI, err := lab.NewAPI(freshPlanner, freshRunner, store, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	detail := authorizedAPIRequest(freshAPI, "GET", "/lab/api/runs/"+id, nil)
	var recovered lab.RunRecord
	json.Unmarshal(detail.Body.Bytes(), &recovered)
	if detail.Code != 200 || recovered.EngineState != "interrupted" || recovered.Cases[0].EngineState != record.Cases[0].EngineState || recovered.Dispatches[0].State != "unknown-final" {
		t.Fatal("HTTP restart lost partial evidence")
	}
	if stale := authorizedAPIRequest(freshAPI, "GET", "/lab/api/runs/"+id+"/events?epoch="+oldCursor.Epoch, nil); stale.Code != 409 {
		t.Fatal("live cursor reused for checkpoints")
	}
	events := authorizedAPIRequest(freshAPI, "GET", "/lab/api/runs/"+id+"/events", nil)
	var stream struct {
		Kind  string `json:"kind"`
		Epoch string `json:"epoch"`
	}
	json.Unmarshal(events.Body.Bytes(), &stream)
	if events.Code != 200 || stream.Kind != "durable-checkpoints" || stream.Epoch == oldCursor.Epoch {
		t.Fatal("checkpoint stream unavailable")
	}
	for i := 0; i < 2; i++ {
		if canceled := authorizedAPIRequest(freshAPI, "POST", "/lab/api/runs/"+id+"/cancel", struct{}{}); canceled.Code != 200 {
			t.Fatal("restarted cancellation not idempotent")
		}
	}
	if resumed := authorizedAPIRequest(freshAPI, "POST", "/lab/api/runs", map[string]any{"plan": plan, "idempotencyToken": "old-plan-token-0000000000000"}); resumed.Code != 400 {
		t.Fatal("old frozen plan resumed")
	}
	if dispatches.Load() != 0 {
		t.Fatal("restart caused dispatch")
	}
}

func TestAPIComparesPairedNativeConfigurationsWithFrozenEvidenceVersions(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/workflow-settings/cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := lab.CaptureSource("../..")
	if err != nil {
		t.Fatal(err)
	}
	planner, err := lab.NewPlanner(map[string][]byte{"workflow-settings": raw}, provenance)
	if err != nil {
		t.Fatal(err)
	}
	store, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runner := lab.NewRunner(planner, lab.RunnerOptions{Retain: store.Save, BeforeDispatch: store.Health})
	api, err := lab.NewAPI(planner, runner, store, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, configuration := range []string{"current", "luna-low"} {
		plan, err := planner.Plan(lab.Selection{Suite: "workflow-settings", Mode: "controlled", Configuration: configuration, CaseIDs: []string{"en-fact"}}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 5, MaxArtifactBytes: 1 << 20, DeadlineMS: 5000})
		if err != nil {
			t.Fatal(err)
		}
		id, err := runner.Start(context.Background(), plan)
		if err != nil {
			t.Fatal(err)
		}
		record := awaitRun(t, runner, id)
		if record.Provenance.FixtureHash != plan.InputHashes["workflow-settings"] || record.Context.LabelHash == "" {
			t.Fatal("known frozen fixture evidence missing")
		}
		ids = append(ids, id)
	}
	response := authorizedAPIRequest(api, "POST", "/lab/api/comparisons", map[string]any{"leftId": ids[0], "rightId": ids[1], "changedDimension": "configuration"})
	var result lab.CaseComparison
	json.Unmarshal(response.Body.Bytes(), &result)
	if response.Code != 200 || !result.Compatibility.Compatible || result.Partial || len(result.Deltas) != 1 || result.Deltas[0].ForbiddenAdditionsDelta == nil || *result.Deltas[0].ForbiddenAdditionsDelta != 0 {
		t.Fatalf("paired finite comparison unavailable: %+v", result)
	}
}

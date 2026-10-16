package evaluationlab_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func TestDashboardStatusExposesMeasuredStorageAndUnavailableSuites(t *testing.T) {
	planner := semanticPlanner(t)
	store, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	api, err := lab.NewAPI(planner, lab.NewRunner(planner, lab.RunnerOptions{}), store, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/lab/api/status", nil)
	req.Host = "127.0.0.1:8123"
	req.Header.Set("Authorization", api.Session().Authorization)
	out := httptest.NewRecorder()
	api.ServeHTTP(out, req)
	var status struct {
		StorageReady bool                   `json:"storageReady"`
		Usage        lab.StoreUsage         `json:"usage"`
		Metrics      []lab.MetricDefinition `json:"metricDefinitions"`
		Suites       []struct {
			ID        string `json:"id"`
			Available bool   `json:"available"`
			Reason    string `json:"reason"`
		} `json:"suites"`
	}
	if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &status) != nil || !status.StorageReady || status.Usage.Files < 1 || len(status.Metrics) != 29 {
		t.Fatalf("dashboard status unavailable: %d", out.Code)
	}
	found := false
	for _, suite := range status.Suites {
		if suite.ID == "qualification-evidence" {
			found = true
			if suite.Available || suite.Reason == "" {
				t.Fatal("missing fixture capability hidden")
			}
		}
	}
	if !found {
		t.Fatal("missing suite not explained")
	}
	store.Close()
	out = httptest.NewRecorder()
	api.ServeHTTP(out, req)
	if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &status) != nil || status.StorageReady {
		t.Fatal("closed storage reported ready")
	}
}

func TestDashboardDeletionRequiresConfirmationAndCannotResurrectMemoryRun(t *testing.T) {
	planner := semanticPlanner(t)
	store, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runner := lab.NewRunner(planner, lab.RunnerOptions{Retain: store.Save})
	api, err := lab.NewAPI(planner, runner, store, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := runner.Start(context.Background(), factPlan(t, planner))
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner, id)
	call := func(method, path, body string) int {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Host = "127.0.0.1:8123"
		req.Header.Set("Authorization", api.Session().Authorization)
		req.Header.Set("Origin", "http://127.0.0.1:8123")
		req.Header.Set("X-Lab-CSRF", api.Session().CSRF)
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		api.ServeHTTP(out, req)
		return out.Code
	}
	if code := call("POST", "/lab/api/runs/"+id+"/delete", `{"confirmId":"other"}`); code != 400 {
		t.Fatalf("unconfirmed deletion: %d", code)
	}
	if code := call("POST", "/lab/api/runs/"+id+"/delete", `{"confirmId":"`+id+`"}`); code != 200 {
		t.Fatalf("confirmed deletion unavailable: %d", code)
	}
	if code := call("GET", "/lab/api/runs/"+id, ""); code != 404 {
		t.Fatalf("deleted evidence resurrected: %d", code)
	}
}

func TestDashboardViewKeepsCaseArtifactsBehindLazyLookup(t *testing.T) {
	planner := semanticPlanner(t)
	store, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runner := lab.NewRunner(planner, lab.RunnerOptions{Retain: store.Save})
	api, err := lab.NewAPI(planner, runner, store, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := runner.Start(context.Background(), factPlan(t, planner))
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, runner, id)
	req := httptest.NewRequest("GET", "/lab/api/runs/"+id+"/view", nil)
	req.Host = "127.0.0.1:8123"
	req.Header.Set("Authorization", api.Session().Authorization)
	out := httptest.NewRecorder()
	api.ServeHTTP(out, req)
	var view struct {
		StartedAt string             `json:"startedAt"`
		ElapsedMS *float64           `json:"elapsedMs"`
		Cases     []lab.ImportedCase `json:"cases"`
		Metrics   []lab.Metric       `json:"metrics"`
	}
	if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &view) != nil || len(view.Cases) != 1 || len(view.Metrics) == 0 {
		t.Fatalf("view unavailable: %d", out.Code)
	}
	if view.StartedAt == "" || view.ElapsedMS == nil {
		t.Fatal("observed native elapsed evidence unavailable")
	}
	if len(view.Cases[0].Source) > 0 || len(view.Cases[0].Actual) > 0 && string(view.Cases[0].Actual) != "null" || len(view.Cases[0].Expected) > 0 {
		t.Fatal("case artifacts eagerly disclosed")
	}
	for _, query := range []struct {
		language string
		count    int
	}{{"en", 1}, {"pt-BR", 0}} {
		req := httptest.NewRequest("GET", "/lab/api/runs?workflow=profile_ingestion&language="+query.language+"&limit=1", nil)
		req.Host = "127.0.0.1:8123"
		req.Header.Set("Authorization", api.Session().Authorization)
		out := httptest.NewRecorder()
		api.ServeHTTP(out, req)
		var page lab.RunPage
		if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &page) != nil || len(page.Runs) != query.count {
			t.Fatalf("history filter %s unavailable: %d", query.language, out.Code)
		}
	}
}

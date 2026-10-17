package evaluationlab_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	"professional-information-repo/internal/semanticeval"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLiveCapabilityDisabledByDefaultAndNoRealTransport(t *testing.T) {
	store, err := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	planner := semanticPlanner(t)
	api, err := lab.NewAPI(planner, lab.NewRunner(planner, lab.RunnerOptions{}), store, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	out := reviewAPIRequest(api, "GET", "/lab/api/live/capability", nil)
	var capability struct {
		Enabled                bool   `json:"enabled"`
		RealExecutionAvailable bool   `json:"realExecutionAvailable"`
		Version                string `json:"version"`
	}
	if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &capability) != nil || capability.Enabled || capability.RealExecutionAvailable || capability.Version == "" {
		t.Fatal("default server exposed live authority or hid disabled capability")
	}
	out = reviewAPIRequest(api, "POST", "/lab/api/live/preflights", []byte(`{}`))
	if out.Code != 403 {
		t.Fatal("disabled capability accepted preflight")
	}
	out = reviewAPIRequest(api, "POST", "/lab/api/live/dispatch", []byte(`{}`))
	if out.Code != 403 {
		t.Fatal("HTTP offered a real dispatch route")
	}
}

func TestLivePreflightBindsOwnedPlanAndNeverCarriesAuthority(t *testing.T) {
	store, err := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	planner := guardPlanner(t, "semantic-quality", "../../docs/evaluations/semantic-quality/cases.v2.json")
	plan := factPlan(t, planner)
	now := time.Now()
	guard, err := lab.NewLiveGuard(planner, store, lab.LiveGuardOptions{Enabled: true, HumanReviewer: "controlled-human-example", Origin: "controlled-test", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	if duplicate, duplicateErr := lab.NewLiveGuard(planner, store, lab.LiveGuardOptions{Enabled: true}); duplicateErr == nil {
		duplicate.Close()
		t.Fatal("second guard owner bypassed single active-run admission")
	}
	request := lab.LivePreflightRequest{Plan: plan, MaxTokenUnits: 3000, MaxOutputTokens: 100, ApprovalLifetimeMS: 60000, Dispatches: []lab.GuardDispatchSpec{
		{CaseID: "en-fact", HTTPID: "en-fact:initial", Stage: "field_validation", Recipient: "api.typesafe.ai", Model: "jev-1.13.0", TokenReservation: 1000},
		{CaseID: "en-fact", HTTPID: "en-fact:initial", Stage: "extract", Recipient: "api.openai.com", Model: "gpt-6-luna", TokenReservation: 1000},
		{CaseID: "en-fact", HTTPID: "en-fact:initial", Stage: "reconcile", Recipient: "api.openai.com", Model: "gpt-6-luna", TokenReservation: 1000},
	}}
	preflight, err := guard.Preflight(request)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := guard.ExportPreflight(preflight.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.Hash == "" || len(preflight.Dispatches) != 3 || preflight.Dispatches[0].PayloadHash == "" || preflight.Identities["labels"] == "" || preflight.Identities["prompt"] == "" || bytes.Contains(exported, []byte("authorizationToken")) {
		t.Fatal("incomplete preflight or leaked authority")
	}
	altered := request
	altered.Plan.Caps.MaxProviderCalls++
	if _, err = guard.Preflight(altered); err == nil {
		t.Fatal("altered plan accepted")
	}
	bad := request
	bad.Dispatches = append([]lab.GuardDispatchSpec{}, request.Dispatches...)
	bad.Dispatches[0].Recipient = "https://unapproved.example"
	if _, err = guard.Preflight(bad); err == nil {
		t.Fatal("arbitrary recipient accepted")
	}
	api, apiErr := lab.NewAPI(planner, lab.NewRunner(planner, lab.RunnerOptions{}), store, lab.APIOptions{Origin: "http://127.0.0.1:8123", LiveGuard: guard})
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	capability := reviewAPIRequest(api, "GET", "/lab/api/live/capability", nil)
	if capability.Code != 200 || !bytes.Contains(capability.Body.Bytes(), []byte(`"enabled":true`)) || !bytes.Contains(capability.Body.Bytes(), []byte(`"realExecutionAvailable":false`)) {
		t.Fatal("enablement certified real execution")
	}
	body, _ := json.Marshal(request)
	prepared := reviewAPIRequest(api, "POST", "/lab/api/live/preflights", body)
	if prepared.Code != 201 {
		t.Fatalf("enabled preflight=%d", prepared.Code)
	}
	if out := reviewAPIRequest(api, "POST", "/lab/api/live/dispatch", []byte(`{}`)); out.Code != 403 {
		t.Fatal("enabled capability offered HTTP dispatch")
	}
	tinyCaps := request.Plan.Caps
	tinyCaps.MaxArtifactBytes = 1
	tinyPlan, planErr := planner.Plan(request.Plan.Selection, tinyCaps)
	if planErr != nil {
		t.Fatal(planErr)
	}
	tiny := request
	tiny.Plan = tinyPlan
	if _, err = guard.Preflight(tiny); err == nil {
		t.Fatal("retention cap smaller than preflight was ignored")
	}
	failHead := true
	faultStore, faultErr := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{BeforeWrite: func(operation string) error {
		if failHead && operation == "rename:head.json" {
			return fmt.Errorf("controlled uncommitted preflight")
		}
		return nil
	}})
	if faultErr != nil {
		t.Fatal(faultErr)
	}
	defer faultStore.Close()
	faultGuard, faultErr := lab.NewLiveGuard(planner, faultStore, lab.LiveGuardOptions{Enabled: true})
	if faultErr != nil {
		t.Fatal(faultErr)
	}
	if _, faultErr = faultGuard.Preflight(request); faultErr == nil {
		t.Fatal("failed preflight head was accepted")
	}
	faultGuard.Close()
	failHead = false
	recovered, recoveryErr := lab.NewLiveGuard(planner, faultStore, lab.LiveGuardOptions{Enabled: true})
	if recoveryErr != nil {
		t.Fatalf("uncommitted preflight prevented recovery: %v", recoveryErr)
	}
	defer recovered.Close()
	if _, recoveryErr = recovered.Preflight(request); recoveryErr != nil {
		t.Fatal(recoveryErr)
	}
	now = now.Add(2 * time.Minute)
	if _, err = guard.Approval(preflight.ID); err == nil {
		t.Fatal("expired preflight acquired authority")
	}

}

func TestControlledGuardAuthorizationCountsEveryLegAndRevokesSecrets(t *testing.T) {
	guard, preflight := guardExample(t)
	challenge, err := guard.Approval(preflight.ID)
	if err != nil {
		t.Fatal(err)
	}
	approval := lab.GuardApproval{PreflightID: preflight.ID, Hash: preflight.Hash, Actor: lab.ReviewActor{ID: "controlled-human-example", Type: "human"}, Confirmation: challenge.Confirmation, Rationale: "Controlled protocol example only; no real experiment or human gold."}
	credentials := map[string]string{"api.openai.com": "OPENAI-CONTROLLED-SENTINEL", "api.typesafe.ai": "JEV-CONTROLLED-SENTINEL"}
	token, err := guard.Authorize(approval, credentials)
	if err != nil {
		t.Fatal(err)
	}
	credentials["api.openai.com"] = "caller mutation"
	calls := 0
	transport := func(ctx context.Context, spec lab.GuardDispatchSpec, credential string) (lab.GuardCallObservation, error) {
		calls++
		if spec.MaxOutputTokens != 100 || spec.Recipient == "api.openai.com" && spec.Reasoning != "none" {
			t.Fatal("transport did not receive exact preflight output/model settings")
		}
		if spec.Recipient == "api.openai.com" && credential != "OPENAI-CONTROLLED-SENTINEL" {
			t.Fatal("credential ownership changed")
		}
		input, output := int64(10), int64(5)
		return lab.GuardCallObservation{Outcome: "completed", Call: semanticeval.Call{Model: credential, Reasoning: credential, PromptHash: credential, SchemaHash: credential, RequestHash: credential, ReturnedModel: spec.Model, Status: 200, InputTokens: &input, OutputTokens: &output}}, nil
	}
	for _, dispatch := range preflight.Dispatches {
		if err = guard.Dispatch(context.Background(), token, dispatch.ID, transport); err != nil {
			t.Fatal(err)
		}
	}
	report, err := guard.Report(preflight.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "completed" || report.AttemptedCalls != 3 || report.ReservedTokenUnits != 3000 || report.Usage.TotalUSD != nil || calls != 3 {
		t.Fatal("guard hid calls/reservations or invented full cost")
	}
	retainGuardExample(t, "preflight.v1.json", preflight)
	retainGuardExample(t, "approval.v1.json", report.Approval)
	retainGuardExample(t, "guard-success.v1.json", report)
	if err = guard.Dispatch(context.Background(), token, preflight.Dispatches[0].ID, transport); err == nil || calls != 3 {
		t.Fatal("completed/double-click authority redispatched")
	}
	exported, err := guard.ExportReport(preflight.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range []string{"OPENAI-CONTROLLED-SENTINEL", "JEV-CONTROLLED-SENTINEL", token.Token} {
		if bytes.Contains(exported, []byte(sentinel)) {
			t.Fatal("credential or authorization token persisted")
		}
	}
}
func guardExample(t *testing.T, supplied ...*lab.FileStore) (*lab.LiveGuard, lab.LivePreflight) {
	t.Helper()
	var store *lab.FileStore
	var err error
	if len(supplied) > 0 {
		store = supplied[0]
	} else {
		store, err = lab.OpenFileStore(t.TempDir(), lab.StoreOptions{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { store.Close() })
	}
	planner := guardPlanner(t, "semantic-quality", "../../docs/evaluations/semantic-quality/cases.v2.json")
	guard, err := lab.NewLiveGuard(planner, store, lab.LiveGuardOptions{Enabled: true, HumanReviewer: "controlled-human-example", Origin: "controlled-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(guard.Close)
	preflight, err := guard.Preflight(lab.LivePreflightRequest{Plan: factPlan(t, planner), MaxTokenUnits: 3000, MaxOutputTokens: 100, ApprovalLifetimeMS: 60000, Dispatches: []lab.GuardDispatchSpec{
		{CaseID: "en-fact", HTTPID: "en-fact:initial", Stage: "field_validation", Recipient: "api.typesafe.ai", Model: "jev-1.13.0", TokenReservation: 1000},
		{CaseID: "en-fact", HTTPID: "en-fact:initial", Stage: "extract", Recipient: "api.openai.com", Model: "gpt-6-luna", TokenReservation: 1000},
		{CaseID: "en-fact", HTTPID: "en-fact:initial", Stage: "reconcile", Recipient: "api.openai.com", Model: "gpt-6-luna", TokenReservation: 1000},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return guard, preflight
}

func TestGuardRestartKeepsUnknownFinalAndCannotRedispatch(t *testing.T) {
	writes := 0
	store, err := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{BeforeWrite: func(operation string) error {
		if operation == "guard-evidence" {
			writes++
			if writes == 4 {
				return fmt.Errorf("controlled result retention failure")
			}
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	guard, preflight := guardExample(t, store)
	specs := append([]lab.GuardDispatchSpec{}, preflight.Dispatches...)
	for i := range specs {
		specs[i].ID, specs[i].PayloadHash, specs[i].PayloadKind = "", "", ""
	}
	specs[0].Recipient, specs[0].Model = "api.openai.com", "gpt-6-luna"
	preflight, err = guard.Preflight(lab.LivePreflightRequest{Plan: preflight.Plan, MaxTokenUnits: 3000, MaxOutputTokens: 100, ApprovalLifetimeMS: 60000, Dispatches: specs, PriceSnapshotVersion: "controlled-fictional-prices-v1", Prices: []semanticeval.Rate{{Model: "gpt-6-luna", AsOf: "2026-10-17", Input: 1, Output: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	writes = 1 // The next three writes retain approval, intent, then the failed result.

	challenge, err := guard.Approval(preflight.ID)
	if err != nil {
		t.Fatal(err)
	}
	token, err := guard.Authorize(lab.GuardApproval{PreflightID: preflight.ID, Hash: preflight.Hash, Actor: lab.ReviewActor{ID: "controlled-human-example", Type: "human"}, Confirmation: challenge.Confirmation, Rationale: "Controlled restart test only"}, map[string]string{"api.openai.com": "synthetic-openai"})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	transport := func(context.Context, lab.GuardDispatchSpec, string) (lab.GuardCallObservation, error) {
		count++
		input, output, zero := int64(10), int64(5), int64(0)
		return lab.GuardCallObservation{Outcome: "completed", Call: semanticeval.Call{ReturnedModel: "gpt-6-luna", Status: 200, InputTokens: &input, OutputTokens: &output, CachedTokens: &zero, CacheWriteTokens: &zero, ReasoningTokens: &zero}}, nil
	}
	if err = guard.Dispatch(context.Background(), token, preflight.Dispatches[0].ID, transport); err == nil || count != 1 {
		t.Fatal("result write failure hidden")
	}
	inMemory, err := guard.Report(preflight.ID)
	if err != nil || inMemory.State != "unknown-final" || inMemory.Usage.TotalUSD != nil || inMemory.Usage.KnownUSD == nil || *inMemory.Usage.KnownUSD <= 0 {
		t.Fatal("unknown-final report invented full cost or lost known subtotal", inMemory.Usage)
	}

	guard.Close()
	restarted, _ := guardExample(t, store)
	report, err := restarted.Report(preflight.ID)
	if err != nil || report.State != "unknown-final" || report.AttemptedCalls != 1 || report.Calls[0].State != "unknown-final" || report.ReservedTokenUnits != 1000 || report.Usage.TotalUSD != nil {
		t.Fatal("restart lost admitted intent or invented final outcome")
	}
	retainGuardExample(t, "unknown-final.v1.json", report)
	if err = restarted.Dispatch(context.Background(), token, preflight.Dispatches[0].ID, transport); err == nil || count != 1 {
		t.Fatal("old authorization resumed unknown-final call")
	}
}

func TestGuardRejectsDistinctInFlightAndOutOfOrderDispatches(t *testing.T) {
	guard, preflight := guardExample(t)
	token, err := authorizeGuardExample(t, guard, preflight, "Controlled concurrency test only")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	transport := func(ctx context.Context, spec lab.GuardDispatchSpec, _ string) (lab.GuardCallObservation, error) {
		close(entered)
		<-release
		return lab.GuardCallObservation{Outcome: "completed", Call: semanticeval.Call{ReturnedModel: spec.Model, Status: 200}}, nil
	}
	if err = guard.Dispatch(context.Background(), token, preflight.Dispatches[1].ID, transport); err == nil {
		t.Fatal("out-of-order valid ID entered transport")
	}
	go func() { finished <- guard.Dispatch(context.Background(), token, preflight.Dispatches[0].ID, transport) }()
	<-entered
	if err = guard.Dispatch(context.Background(), token, preflight.Dispatches[1].ID, transport); err == nil {
		t.Fatal("distinct ID entered while another call was in flight")
	}
	if err = guard.Dispatch(context.Background(), token, preflight.Dispatches[0].ID, transport); err == nil {
		t.Fatal("double click dispatched in-flight ID twice")
	}
	close(release)
	if err = <-finished; err != nil {
		t.Fatal(err)
	}
	report, err := guard.Report(preflight.ID)
	if err != nil || report.AttemptedCalls != 1 || report.ReservedTokenUnits != 1000 {
		t.Fatal("rejected admissions consumed calls or lost reservation")
	}
}

func TestGuardCancellationBeforeAndAfterAdmissionNeverRefunds(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(fmt.Sprint(after), func(t *testing.T) {
			guard, preflight := guardExample(t)
			token, err := authorizeGuardExample(t, guard, preflight, "Controlled cancellation only")
			if err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{})
			finished := make(chan error, 1)
			transport := func(ctx context.Context, _ lab.GuardDispatchSpec, _ string) (lab.GuardCallObservation, error) {
				close(entered)
				<-ctx.Done()
				return lab.GuardCallObservation{}, ctx.Err()
			}
			if after {
				go func() { finished <- guard.Dispatch(context.Background(), token, preflight.Dispatches[0].ID, transport) }()
				<-entered
			}
			if err = guard.Cancel(token); err != nil {
				t.Fatal(err)
			}
			if after {
				if err = <-finished; err == nil {
					t.Fatal("cancellation hidden")
				}
			}
			report, err := guard.Report(preflight.ID)
			if err != nil || report.State != "canceled" {
				t.Fatal("cancellation outcome unavailable")
			}
			expected := 0
			if after {
				expected = 1
			}
			if report.AttemptedCalls != expected || report.ReservedTokenUnits != int64(expected*1000) {
				t.Fatal("canceled dispatch refunded or undispatched call counted")
			}
			if err = guard.Dispatch(context.Background(), token, preflight.Dispatches[0].ID, transport); err == nil {
				t.Fatal("canceled run resumed")
			}
		})
	}
}

func TestGuardDeadlineExpiresWithoutDispatchAndRevokesAuthority(t *testing.T) {
	guard, preflight := guardExample(t)
	token, err := authorizeGuardExample(t, guard, preflight, "Controlled deadline test only")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Duration(preflight.Plan.Caps.DeadlineMS)*time.Millisecond + time.Second)
	for time.Now().Before(deadline) {
		report, err := guard.Report(preflight.ID)
		if err != nil {
			t.Fatal(err)
		}
		if report.State == "expired" {
			if report.AttemptedCalls != 0 {
				t.Fatal("expiry invented a dispatch")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("deadline left unused authority alive: %s", token.RunID)
}

func TestGuardApprovalRejectsAgentChangedHashAndCredentialEcho(t *testing.T) {
	guard, preflight := guardExample(t)
	challenge, err := guard.Approval(preflight.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := lab.GuardApproval{PreflightID: preflight.ID, Hash: preflight.Hash, Actor: lab.ReviewActor{ID: "controlled-human-example", Type: "human"}, Confirmation: challenge.Confirmation, Rationale: "Controlled authorization tests only"}
	credentials := map[string]string{"api.openai.com": "SENSITIVE-CONTROLLED-SENTINEL", "api.typesafe.ai": "synthetic-jev"}
	changed := input
	changed.Hash = "changed"
	if _, err = guard.Authorize(changed, credentials); err == nil {
		t.Fatal("changed approval hash accepted")
	}
	changed = input
	changed.Actor.Type = "agent"
	if _, err = guard.Authorize(changed, credentials); err == nil {
		t.Fatal("agent acquired human authority")
	}
	changed = input
	changed.Rationale = credentials["api.openai.com"]
	if _, err = guard.Authorize(changed, credentials); err == nil {
		t.Fatal("credential echoed into approval history")
	}
	missing := map[string]string{"api.openai.com": "synthetic"}
	if _, err = guard.Authorize(input, missing); err == nil {
		t.Fatal("missing recipient accepted")
	}
	extra := map[string]string{"api.openai.com": "synthetic", "api.typesafe.ai": "synthetic", "unapproved.example": "synthetic"}
	if _, err = guard.Authorize(input, extra); err == nil {
		t.Fatal("extra recipient accepted")
	}
	exported, err := guard.ExportReport(preflight.ID)
	if err != nil || bytes.Contains(exported, []byte(credentials["api.openai.com"])) {
		t.Fatal("rejected authority persisted credentials")
	}
}

func TestGuardPartialPricesRetainKnownSubtotalWithoutInventingTotal(t *testing.T) {
	guard, original := guardExample(t)
	specs := append([]lab.GuardDispatchSpec{}, original.Dispatches...)
	for i := range specs {
		specs[i].ID = ""
		specs[i].PayloadHash = ""
		specs[i].PayloadKind = ""
	}
	preflight, err := guard.Preflight(lab.LivePreflightRequest{Plan: original.Plan, MaxTokenUnits: 3000, MaxOutputTokens: 100, ApprovalLifetimeMS: 60000, Dispatches: specs, PriceSnapshotVersion: "controlled-fictional-prices-v1", Prices: []semanticeval.Rate{{Model: "gpt-6-luna", AsOf: "2026-10-17", Input: 1, Cached: 1, Write: 1, Output: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	token, err := authorizeGuardExample(t, guard, preflight, "Controlled fictional price accounting only")
	if err != nil {
		t.Fatal(err)
	}
	transport := func(ctx context.Context, spec lab.GuardDispatchSpec, _ string) (lab.GuardCallObservation, error) {
		input, output, zero := int64(10), int64(5), int64(0)
		return lab.GuardCallObservation{Outcome: "completed", Call: semanticeval.Call{ReturnedModel: spec.Model, Status: 200, InputTokens: &input, OutputTokens: &output, CachedTokens: &zero, CacheWriteTokens: &zero, ReasoningTokens: &zero}}, nil
	}
	for _, spec := range preflight.Dispatches {
		if err = guard.Dispatch(context.Background(), token, spec.ID, transport); err != nil {
			t.Fatal(err)
		}
	}
	report, err := guard.Report(preflight.ID)
	if err != nil || report.Usage.Calls != 3 || report.Usage.Unpriced != 1 || report.Usage.KnownUSD == nil || math.Abs(*report.Usage.KnownUSD-0.00004) > 1e-12 || report.Usage.TotalUSD != nil || report.Preflight.PriceSnapshotVersion != "controlled-fictional-prices-v1" {
		t.Fatal("partial prices hid known subtotal or certified a full cost")
	}
}

func TestGuardFailureClassesStopWithoutRetryAndKeepAttempt(t *testing.T) {
	for _, scenario := range []struct {
		name, outcome  string
		status         int
		transportError bool
	}{
		{"http-failure", "completed", 500, false}, {"refused", "refused", 200, false}, {"malformed", "malformed", 200, false}, {"truncated", "truncated", 200, false}, {"rate-limited", "rate_limited", 429, false}, {"unavailable", "unavailable", 503, false}, {"transport", "", 0, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			guard, preflight := guardExample(t)
			token, err := authorizeGuardExample(t, guard, preflight, "Controlled failure accounting only")
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			transport := func(_ context.Context, spec lab.GuardDispatchSpec, credential string) (lab.GuardCallObservation, error) {
				count++
				observation := lab.GuardCallObservation{Outcome: scenario.outcome, Call: semanticeval.Call{ReturnedModel: spec.Model, Status: scenario.status}}
				if scenario.transportError {
					return observation, fmt.Errorf("sensitive error %s", credential)
				}
				return observation, nil
			}
			if err = guard.Dispatch(context.Background(), token, preflight.Dispatches[0].ID, transport); err == nil {
				t.Fatal("failed call counted as completed")
			}
			if err = guard.Dispatch(context.Background(), token, preflight.Dispatches[1].ID, transport); err == nil || count != 1 {
				t.Fatal("failure automatically retried or continued")
			}
			report, err := guard.Report(preflight.ID)
			if err != nil || report.State != "failed" || report.AttemptedCalls != 1 || report.UnreachedCalls != 2 || report.ReservedTokenUnits != 1000 || report.Usage.TotalUSD != nil {
				t.Fatal("failure lost attempt, reservation or unknown cost")
			}
			exported, err := guard.ExportReport(preflight.ID)
			if err != nil || bytes.Contains(exported, []byte("synthetic-jev")) {
				t.Fatal("transport error leaked credential")
			}
		})
	}
}

func TestGuardTokenBudgetRacesNeverOverspend(t *testing.T) {
	guard, original := guardExample(t)
	specs := append([]lab.GuardDispatchSpec{}, original.Dispatches...)
	for i := range specs {
		specs[i].ID = ""
		specs[i].PayloadHash = ""
		specs[i].PayloadKind = ""
	}
	preflight, err := guard.Preflight(lab.LivePreflightRequest{Plan: original.Plan, MaxTokenUnits: 1000, MaxOutputTokens: 100, ApprovalLifetimeMS: 60000, Dispatches: specs})
	if err != nil {
		t.Fatal(err)
	}
	token, err := authorizeGuardExample(t, guard, preflight, "Controlled budget race only")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	var group sync.WaitGroup
	transport := func(_ context.Context, spec lab.GuardDispatchSpec, _ string) (lab.GuardCallObservation, error) {
		calls.Add(1)
		return lab.GuardCallObservation{Outcome: "completed", Call: semanticeval.Call{ReturnedModel: spec.Model, Status: 200}}, nil
	}
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_ = guard.Dispatch(context.Background(), token, preflight.Dispatches[0].ID, transport)
		}()
	}
	group.Wait()
	if err = guard.Dispatch(context.Background(), token, preflight.Dispatches[1].ID, transport); err == nil {
		t.Fatal("token cap overspent")
	}
	report, err := guard.Report(preflight.ID)
	if err != nil || calls.Load() != 1 || report.AttemptedCalls != 1 || report.ReservedTokenUnits != 1000 || report.State != "failed" {
		t.Fatal("racing budget admitted duplicate or refunded reservations")
	}
}

func TestGuardIntentRetentionFailurePreventsTransport(t *testing.T) {
	writes := 0
	store, err := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{BeforeWrite: func(operation string) error {
		if operation == "guard-evidence" {
			writes++
			if writes == 3 {
				return fmt.Errorf("controlled intent retention failure")
			}
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	guard, preflight := guardExample(t, store)
	token, err := authorizeGuardExample(t, guard, preflight, "Controlled pre-dispatch retention failure only")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	transport := func(context.Context, lab.GuardDispatchSpec, string) (lab.GuardCallObservation, error) {
		count++
		return lab.GuardCallObservation{}, nil
	}
	if err = guard.Dispatch(context.Background(), token, preflight.Dispatches[0].ID, transport); err == nil || count != 0 {
		t.Fatal("unretained intent reached transport")
	}
	report, err := guard.Report(preflight.ID)
	if err != nil || report.State != "failed" || report.AttemptedCalls != 0 {
		t.Fatal("retention failure hid revocation or invented a call")
	}
	if err = guard.Dispatch(context.Background(), token, preflight.Dispatches[0].ID, transport); err == nil || count != 0 {
		t.Fatal("failed admission automatically resumed")
	}
}
func TestControlledGuardNeverAttemptsExternalNetwork(t *testing.T) {
	original := http.DefaultTransport
	var external atomic.Int64
	http.DefaultTransport = deniedTransport{calls: &external}
	defer func() { http.DefaultTransport = original }()
	guard, preflight := guardExample(t)
	token, err := authorizeGuardExample(t, guard, preflight, "Controlled zero-network verification only")
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range preflight.Dispatches {
		if err = guard.Dispatch(context.Background(), token, spec.ID, func(_ context.Context, spec lab.GuardDispatchSpec, _ string) (lab.GuardCallObservation, error) {
			return lab.GuardCallObservation{Outcome: "completed", Call: semanticeval.Call{ReturnedModel: spec.Model, Status: 200}}, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if external.Load() != 0 {
		t.Fatal("guard verification attempted an external request")
	}
}

func TestGuardRejectsOverflowingUsageWithoutTrustingItsCost(t *testing.T) {
	guard, preflight := guardExample(t)
	token, err := authorizeGuardExample(t, guard, preflight, "Controlled malformed usage test only")
	if err != nil {
		t.Fatal(err)
	}
	input, output := int64(math.MaxInt64), int64(1)
	err = guard.Dispatch(context.Background(), token, preflight.Dispatches[0].ID, func(_ context.Context, spec lab.GuardDispatchSpec, _ string) (lab.GuardCallObservation, error) {
		return lab.GuardCallObservation{Outcome: "completed", Call: semanticeval.Call{ReturnedModel: spec.Model, Status: 200, InputTokens: &input, OutputTokens: &output}}, nil
	})
	if err == nil {
		t.Fatal("integer overflow bypassed token reservation")
	}
	report, err := guard.Report(preflight.ID)
	if err != nil || report.State != "failed" || report.Usage.TotalUSD != nil || report.Usage.Input.Total != nil {
		t.Fatal("out-of-bounds usage became trusted cost/token evidence")
	}
}

func retainGuardExample(t *testing.T, name string, value any) {
	t.Helper()
	directory := os.Getenv("LAB_GUARD_EVIDENCE_DIR")
	if directory == "" {
		return
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, name), raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestGuardCallerDeadlineRetainsTimeoutAndNonrefundableAttempt(t *testing.T) {
	guard, preflight := guardExample(t)
	token, err := authorizeGuardExample(t, guard, preflight, "Controlled timeout attribution")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	calls := 0
	err = guard.Dispatch(ctx, token, preflight.Dispatches[0].ID, func(ctx context.Context, _ lab.GuardDispatchSpec, _ string) (lab.GuardCallObservation, error) {
		calls++
		<-ctx.Done()
		return lab.GuardCallObservation{}, ctx.Err()
	})
	if err == nil {
		t.Fatal("timeout completed")
	}
	report, err := guard.Report(preflight.ID)
	if err != nil || report.Calls[0].State != "timeout" || report.AttemptedCalls != 1 || report.ReservedTokenUnits != 1000 || report.UnreachedCalls != 2 {
		t.Fatal("timeout attribution or admission lost", report.Calls)
	}
	if err = guard.Dispatch(context.Background(), token, preflight.Dispatches[1].ID, func(context.Context, lab.GuardDispatchSpec, string) (lab.GuardCallObservation, error) {
		calls++
		return lab.GuardCallObservation{}, nil
	}); err == nil || calls != 1 {
		t.Fatal("timed out run continued")
	}
}

func guardPlanner(t *testing.T, suite, fixture string) *lab.Planner {
	t.Helper()
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := lab.CaptureSource("../..")
	if err != nil {
		t.Fatal(err)
	}
	planner, err := lab.NewPlanner(map[string][]byte{suite: raw}, provenance)
	if err != nil {
		t.Fatal(err)
	}
	return planner
}

func authorizeGuardExample(t *testing.T, guard *lab.LiveGuard, preflight lab.LivePreflight, rationale string) (lab.GuardAuthorization, error) {
	t.Helper()
	challenge, err := guard.Approval(preflight.ID)
	if err != nil {
		return lab.GuardAuthorization{}, err
	}
	return guard.Authorize(lab.GuardApproval{PreflightID: preflight.ID, Hash: challenge.Hash, Actor: lab.ReviewActor{ID: "controlled-human-example", Type: "human"}, Confirmation: challenge.Confirmation, Rationale: rationale}, map[string]string{"api.openai.com": "synthetic-openai", "api.typesafe.ai": "synthetic-jev"})
}

package evaluationlab_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func semanticPlanner(t *testing.T) *lab.Planner {
	t.Helper()
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/cases.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := lab.NewPlanner(map[string][]byte{"semantic-quality": raw})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

type deniedTransport struct{ calls *atomic.Int64 }

func (d deniedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	d.calls.Add(1)
	return nil, errors.New("external network denied by controlled test")
}

func TestInProcessCatalogSuitesExecuteDirectlyWithoutExternalTransport(t *testing.T) {
	read := func(path string) []byte {
		raw, err := os.ReadFile("../../docs/evaluations/" + path)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	labels := read("qualification-evidence/cases.json")
	assets := map[string][]byte{"semantic-quality": read("semantic-quality/cases.v2.json"), "workflow-settings": read("workflow-settings/cases.v1.json"), "qualification-evidence": labels, "application-context": labels, "application-baseline": read("semantic-quality/controlled.v2.json"), "section-review": read("section-review/cases.v1.json")}
	assets["cv-integrity-evidence"] = read("cv-lab/evidence.v1.json")
	var external atomic.Int64
	original := http.DefaultTransport
	http.DefaultTransport = deniedTransport{calls: &external}
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, suite := range lab.Catalog() {
		if suite.ID == "test-adapters" {
			continue
		} // Separately exercised through the isolated public runner seam.
		t.Run(suite.ID, func(t *testing.T) {
			p, err := lab.NewPlanner(assets)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := p.Plan(lab.Selection{Suite: suite.ID, Mode: suite.Modes[0], Configuration: suite.Configurations[0]}, lab.ExecutionCaps{MaxCases: 100, MaxHTTPAttempts: 100, MaxProviderCalls: 500, MaxArtifactBytes: 8 << 20, DeadlineMS: 10000})
			if err != nil {
				t.Fatal(err)
			}
			runner := lab.NewRunner(p, lab.RunnerOptions{})
			id, err := runner.Start(context.Background(), plan)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			record, err := runner.Wait(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if record.EngineState != "completed" || len(record.Cases) != len(suite.Cases) || len(record.Metrics) == 0 {
				t.Fatalf("suite did not finish: state=%s cases=%d metrics=%d", record.EngineState, len(record.Cases), len(record.Metrics))
			}
			seen := map[string]bool{}
			for _, dispatch := range record.Dispatches {
				if seen[dispatch.Identity] || dispatch.Origin != "simulated" {
					t.Fatal("duplicate or non-simulated dispatch")
				}
				seen[dispatch.Identity] = true
			}
		})
	}
	if external.Load() != 0 {
		t.Fatalf("controlled execution attempted %d external requests", external.Load())
	}
}

func TestSequenceArtifactCapStopsBeforeNextRequiredLeg(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/workflow-settings/cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := lab.NewPlanner(map[string][]byte{"workflow-settings": raw})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := p.Plan(lab.Selection{Suite: "workflow-settings", Mode: "controlled", Configuration: "current", CaseIDs: []string{"en-clarification"}}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 2, MaxProviderCalls: 10, MaxArtifactBytes: 1, DeadlineMS: 5000})
	if err != nil {
		t.Fatal(err)
	}
	runner := lab.NewRunner(p, lab.RunnerOptions{})
	id, err := runner.Start(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, runner, id)
	if record.EngineState != "failed" || record.Progress.AttemptedHTTP != 1 || record.Progress.UnreachedHTTP != 1 {
		t.Fatalf("artifact cap allowed another leg: state=%s attempted=%d unreached=%d", record.EngineState, record.Progress.AttemptedHTTP, record.Progress.UnreachedHTTP)
	}
	if record.Cases[0].Children[0].AttemptedSteps != 1 {
		t.Fatal("executed child lacks attempted count")
	}
	child := record.Cases[0].Children[1]
	if child.EngineState != "skipped" || child.SkippedSteps != 1 || child.AttemptedSteps != 0 || len(child.UnavailableReasons) != 1 || child.UnavailableReasons[0] != "capacity_exhausted" {
		t.Fatalf("unreached child lacks accounting: %+v", child)
	}
}

type labBlockedBody struct {
	closed chan struct{}
	once   sync.Once
}

func (b *labBlockedBody) Read([]byte) (int, error) { <-b.closed; return 0, io.ErrClosedPipe }
func (b *labBlockedBody) Close() error             { b.once.Do(func() { close(b.closed) }); return nil }

func TestRunnerCancellationRetainsUnknownFinalDispatchAndStopsFutureCalls(t *testing.T) {
	p := semanticPlanner(t)
	reading := make(chan struct{})
	runner := lab.NewRunner(p, lab.RunnerOptions{ControlledBody: func(context.Context, io.ReadCloser) io.ReadCloser {
		close(reading)
		return &labBlockedBody{closed: make(chan struct{})}
	}})
	id, err := runner.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-reading:
	case <-time.After(time.Second):
		t.Fatal("body read never started")
	}
	if err := runner.Cancel(id); err != nil {
		t.Fatal(err)
	}
	r := awaitRun(t, runner, id)
	if r.EngineState != "canceled" || len(r.Dispatches) != 1 || r.Dispatches[0].State != "unknown-final" || r.Dispatches[0].Call.InputTokens != nil || r.Cases[0].EngineState != "canceled" {
		t.Fatalf("lost canceled evidence: state=%s calls=%d", r.EngineState, len(r.Dispatches))
	}
}

func TestRunnerDeadlineStopsBodyReadWithExplicitAttribution(t *testing.T) {
	p := semanticPlanner(t)
	caps := factPlan(t, p).Caps
	caps.DeadlineMS = 100
	plan, err := p.Plan(lab.Selection{Suite: "semantic-quality", Mode: "controlled", Configuration: "current", CaseIDs: []string{"en-fact"}}, caps)
	if err != nil {
		t.Fatal(err)
	}
	runner := lab.NewRunner(p, lab.RunnerOptions{ControlledBody: func(context.Context, io.ReadCloser) io.ReadCloser {
		return &labBlockedBody{closed: make(chan struct{})}
	}})
	id, err := runner.Start(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, runner, id)
	found := false
	for _, reason := range record.UnavailableReasons {
		found = found || reason == "deadline_exceeded"
	}
	if record.EngineState != "failed" || !found || record.Progress.AttemptedDispatches > 1 {
		t.Fatalf("deadline attribution lost: state=%s calls=%d", record.EngineState, record.Progress.AttemptedDispatches)
	}
}

func TestRunnerProviderCapStopsBeforeAnotherDispatch(t *testing.T) {
	p := semanticPlanner(t)
	caps := factPlan(t, p).Caps
	caps.MaxProviderCalls = 1
	plan, err := p.Plan(lab.Selection{Suite: "semantic-quality", Mode: "controlled", Configuration: "current", CaseIDs: []string{"en-fact"}}, caps)
	if err != nil {
		t.Fatal(err)
	}
	runner := lab.NewRunner(p, lab.RunnerOptions{})
	id, err := runner.Start(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	r := awaitRun(t, runner, id)
	if r.EngineState != "failed" || r.Progress.AttemptedDispatches != 1 || len(r.Dispatches) != 1 {
		t.Fatalf("provider cap exceeded: state=%s calls=%d", r.EngineState, r.Progress.AttemptedDispatches)
	}
}

func TestRunnerBoundsEventHistoryAndReportsCursorGaps(t *testing.T) {
	p := semanticPlanner(t)
	runner := lab.NewRunner(p, lab.RunnerOptions{MaxEvents: 3})
	id, err := runner.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	_ = awaitRun(t, runner, id)
	page, err := runner.Events(id, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 3 || !page.Gap || page.Earliest < 2 {
		t.Fatalf("missing bounded cursor gap: %+v", page)
	}
	if _, err := runner.Events(id, page.Next+1, 100); err == nil {
		t.Fatal("future cursor accepted")
	}
}

func TestRunnerRejectsConcurrentAdmissionAndCancelIsIdempotent(t *testing.T) {
	p := semanticPlanner(t)
	entered := make(chan struct{})
	once := false
	runner := lab.NewRunner(p, lab.RunnerOptions{BeforeDispatch: func(ctx context.Context) error {
		if !once {
			once = true
			close(entered)
		}
		<-ctx.Done()
		return ctx.Err()
	}})
	plan := factPlan(t, p)
	id, err := runner.Start(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("run never reached dispatch health guard")
	}
	if _, err := runner.Start(context.Background(), plan); !errors.Is(err, lab.ErrRunnerBusy) {
		t.Fatalf("second run admitted: %v", err)
	}
	if err := runner.Cancel(id); err != nil {
		t.Fatal(err)
	}
	if err := runner.Cancel(id); err != nil {
		t.Fatal(err)
	}
	r := awaitRun(t, runner, id)
	if r.EngineState != "canceled" || r.Progress.AttemptedDispatches != 0 || r.Progress.UnreachedHTTP != 1 {
		t.Fatalf("cancel result: state=%s calls=%d unreached=%d", r.EngineState, r.Progress.AttemptedDispatches, r.Progress.UnreachedHTTP)
	}
}

func TestRunnerStopsAfterPersistenceFailureWithoutRetry(t *testing.T) {
	p := semanticPlanner(t)
	failed := false
	retried := false
	runner := lab.NewRunner(p, lab.RunnerOptions{Retain: func(record lab.RunRecord) error {
		if failed {
			retried = true
		}
		if record.Progress.AttemptedDispatches == 1 {
			failed = true
			return errors.New("disk full")
		}
		return nil
	}})
	id, err := runner.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	r := awaitRun(t, runner, id)
	if r.EngineState != "failed" || r.Progress.AttemptedDispatches != 1 || retried {
		t.Fatalf("persistence failure retried or dispatched: state=%s calls=%d retried=%t", r.EngineState, r.Progress.AttemptedDispatches, retried)
	}
}

func TestTerminalPersistenceFailureCannotAppearSuccessful(t *testing.T) {
	p := semanticPlanner(t)
	finishing := make(chan struct{})
	release := make(chan struct{})
	runner := lab.NewRunner(p, lab.RunnerOptions{Retain: func(record lab.RunRecord) error {
		if record.EngineState == "completed" {
			close(finishing)
			<-release
			return errors.New("terminal save failed")
		}
		return nil
	}})
	id, err := runner.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-finishing:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("terminal retention never reached")
	}
	pending, readErr := runner.Read(id)
	close(release)
	if readErr != nil || pending.EngineState != "running" {
		t.Fatalf("terminal state published before save result: %s %v", pending.EngineState, readErr)
	}
	r := awaitRun(t, runner, id)
	if r.EngineState != "failed" || r.Durable {
		t.Fatalf("failed terminal save appeared successful: %s durable=%t", r.EngineState, r.Durable)
	}
}
func factPlan(t *testing.T, p *lab.Planner) lab.Plan {
	t.Helper()
	plan, err := p.Plan(lab.Selection{Suite: "semantic-quality", Mode: "controlled", Configuration: "current", CaseIDs: []string{"en-fact"}}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 5, MaxArtifactBytes: 1 << 20, DeadlineMS: 5000})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
func awaitRun(t *testing.T, r *lab.Runner, id string) lab.RunRecord {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		record, err := r.Read(id)
		if err != nil {
			t.Fatal(err)
		}
		if record.EngineState != "running" && record.EngineState != "planned" {
			return record
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("runner did not terminate")
	return lab.RunRecord{}
}
func TestRunnerExecutesFrozenControlledPlanAndExposesIndependentProgress(t *testing.T) {
	p := semanticPlanner(t)
	runner := lab.NewRunner(p, lab.RunnerOptions{})
	id, err := runner.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	r := awaitRun(t, runner, id)
	if r.EngineState != "completed" || r.AssertionVerdict != "pass" || len(r.Cases) != 1 || len(r.Dispatches) != 3 || r.Durable {
		t.Fatalf("unexpected controlled result: state=%s assertion=%s cases=%d calls=%d", r.EngineState, r.AssertionVerdict, len(r.Cases), len(r.Dispatches))
	}
	if r.Cases[0].Identity == "" || r.Cases[0].LeafIdentity == "" || r.Cases[0].Identity == r.Cases[0].LeafIdentity {
		t.Fatal("logical and leaf identities conflated")
	}
	page, err := runner.Events(id, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var last uint64
	for _, event := range page.Events {
		if event.Sequence <= last {
			t.Fatal("nonmonotonic observations")
		}
		last = event.Sequence
	}
	if len(page.Events) == 0 || r.Progress.AttemptedHTTP != 1 || r.Progress.FinishedCases != 1 {
		t.Fatal("missing observed progress")
	}
	r.EngineState = "forged"
	r.Cases[0].Actual[0] = '!'
	again, err := runner.Read(id)
	if err != nil {
		t.Fatal(err)
	}
	if again.EngineState != "completed" || len(again.Cases[0].Actual) == 0 || again.Cases[0].Actual[0] != '{' {
		t.Fatal("public read aliases owned run state")
	}
}

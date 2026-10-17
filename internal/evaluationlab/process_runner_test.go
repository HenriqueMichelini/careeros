package evaluationlab_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	"sync/atomic"
	"testing"
	"time"
)

func TestProcessRunnerExecutesAllowlistedGoContractsWithoutInference(t *testing.T) {
	runner, id := startProcessRun(t, lab.Selection{Suite: "test-adapters", Mode: "offline", Configuration: "current", CaseIDs: []string{"go-preprocessing"}}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 1, MaxArtifactBytes: 1 << 20, DeadlineMS: 60000})
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	record, err := runner.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Outcome         string  `json:"outcome"`
		Origin          string  `json:"origin"`
		DurationMS      float64 `json:"durationMs"`
		CleanupComplete bool    `json:"cleanupComplete"`
	}
	if len(record.Cases) != 1 || json.Unmarshal(record.Cases[0].Actual, &actual) != nil || actual.Outcome != "passed" || actual.Origin != "isolated-go-tests" || !actual.CleanupComplete || actual.DurationMS <= 0 || record.EngineState != "completed" || record.AssertionVerdict != "pass" {
		t.Fatalf("structured process result unavailable: %+v, %s/%s; %s", actual, record.EngineState, record.AssertionVerdict, record.Cases[0].Actual)
	}
	if len(record.Dispatches) != 0 || record.Progress.AttemptedHTTP != 0 {
		t.Fatal("ordinary tests became inference or HTTP attempts")
	}
}

func TestProcessRunnerCannotReadHostSentinelsOrReachHostLoopback(t *testing.T) {
	const sentinel = "/tmp/careeros-lab-host-sentinel"
	file, err := os.OpenFile(sentinel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Skip("Controlled host sentinel path unavailable")
	}
	file.WriteString("synthetic-host-only-canary")
	file.Close()
	defer os.Remove(sentinel)
	listener, err := net.Listen("tcp4", "127.0.0.1:54679")
	if err != nil {
		t.Skip("Controlled host loopback port unavailable")
	}
	defer listener.Close()
	var hits atomic.Int32
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Write([]byte("synthetic-host-only-response"))
	})}
	go server.Serve(listener)
	defer server.Close()
	t.Setenv("OPENAI_API_KEY", "synthetic-lab-environment-canary")
	runner, id := startProcessRun(t, lab.Selection{Suite: "test-adapters", Mode: "offline", Configuration: "current", CaseIDs: []string{"probe-isolation"}}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 1, MaxArtifactBytes: 1 << 20, DeadlineMS: 5000})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	record, err := runner.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Outcome         string
		Evidence        map[string]bool
		CleanupComplete bool
	}
	json.Unmarshal(record.Cases[0].Actual, &actual)
	if actual.Outcome != "passed" || !actual.CleanupComplete || len(actual.Evidence) != 4 || hits.Load() != 0 {
		t.Fatalf("isolation sentinel failed: %s", record.Cases[0].Actual)
	}
	for _, key := range []string{"credentialEnvAbsent", "hostFileAbsent", "hostNetworkUnreachable", "privateHome"} {
		if !actual.Evidence[key] {
			t.Fatalf("isolation proof missing: %s", key)
		}
	}
}

func TestProcessRunnerCancellationStopsDetachedChildAndGrandchild(t *testing.T) {
	runner, id := startProcessRun(t, lab.Selection{Suite: "test-adapters", Mode: "offline", Configuration: "current", CaseIDs: []string{"probe-hang-tree", "node-profile-document"}}, lab.ExecutionCaps{MaxCases: 2, MaxHTTPAttempts: 1, MaxProviderCalls: 1, MaxArtifactBytes: 1 << 20, DeadlineMS: 5000})
	ready := false
	for i := 0; i < 200; i++ {
		page, eventErr := runner.Events(id, 0, 100)
		if eventErr != nil {
			t.Fatal(eventErr)
		}
		for _, event := range page.Events {
			if event.Kind == "process_tree_ready" {
				ready = true
			}
		}
		if ready {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("actual detached child/grandchild tree was not reached")
	}
	if err := runner.Cancel(id); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	record, err := runner.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Outcome                                      string
		TreeReady, NamespaceStopped, CleanupComplete bool
	}
	json.Unmarshal(record.Cases[0].Actual, &actual)
	if record.EngineState != "canceled" || actual.Outcome != "canceled" || !actual.TreeReady || !actual.NamespaceStopped || !actual.CleanupComplete || record.Progress.AttemptedProcesses != 1 || record.Progress.UnreachedProcesses != 1 || record.Cases[1].EngineState != "skipped" {
		t.Fatalf("tree cancellation incomplete: %s; %+v", record.Cases[0].Actual, record.Progress)
	}
}

func TestProcessRunnerDeadlineRetainsPartialTreeAndUnreachedTest(t *testing.T) {
	runner, id := startProcessRun(t, lab.Selection{Suite: "test-adapters", Mode: "offline", Configuration: "current", CaseIDs: []string{"probe-hang-tree", "node-profile-document"}}, lab.ExecutionCaps{MaxCases: 2, MaxHTTPAttempts: 1, MaxProviderCalls: 1, MaxArtifactBytes: 1 << 20, DeadlineMS: 1000})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	record, err := runner.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Outcome                                      string
		TreeReady, NamespaceStopped, CleanupComplete bool
	}
	json.Unmarshal(record.Cases[0].Actual, &actual)
	if record.EngineState != "failed" || actual.Outcome != "timed_out" || !actual.TreeReady || !actual.NamespaceStopped || !actual.CleanupComplete || record.Progress.UnreachedProcesses != 1 || record.Cases[1].EngineState != "skipped" {
		t.Fatalf("deadline tree cleanup incomplete: %s", record.Cases[0].Actual)
	}
}

func TestProcessRunnerBoundsOutputAndStopsFollowingTests(t *testing.T) {
	runner, id := startProcessRun(t, lab.Selection{Suite: "test-adapters", Mode: "offline", Configuration: "current", CaseIDs: []string{"probe-output", "node-profile-document"}}, lab.ExecutionCaps{MaxCases: 2, MaxHTTPAttempts: 1, MaxProviderCalls: 1, MaxArtifactBytes: 1 << 20, DeadlineMS: 5000})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	record, err := runner.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Outcome, OutputText, OutputHash                    string
		OutputTruncated, NamespaceStopped, CleanupComplete bool
	}
	json.Unmarshal(record.Cases[0].Actual, &actual)
	var hashed struct {
		Hash string `json:"capturedOutputHash"`
	}
	json.Unmarshal(record.Cases[0].Actual, &hashed)
	if actual.Outcome != "output_limited" || !actual.OutputTruncated || !actual.NamespaceStopped || !actual.CleanupComplete || len(actual.OutputText) > 128<<10 || hashed.Hash != fmt.Sprintf("%x", sha256.Sum256([]byte(actual.OutputText))) || record.Progress.UnreachedProcesses != 1 || record.Cases[1].EngineState != "skipped" {
		t.Fatalf("output bound contract failed: outcome%s bytes%d", actual.Outcome, len(actual.OutputText))
	}
}

func TestProcessRunnerMissingToolIsBlockedWithoutSpawning(t *testing.T) {
	runner, id := startProcessRun(t, lab.Selection{Suite: "test-adapters", Mode: "offline", Configuration: "current", CaseIDs: []string{"probe-tool-missing", "node-profile-document"}}, lab.ExecutionCaps{MaxCases: 2, MaxHTTPAttempts: 1, MaxProviderCalls: 1, MaxArtifactBytes: 1 << 20, DeadlineMS: 5000})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	record, err := runner.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Outcome                         string
		ExitCode                        *int
		SpawnAttempted, CleanupComplete bool
	}
	json.Unmarshal(record.Cases[0].Actual, &actual)
	if record.EngineState != "failed" || record.AssertionVerdict != "blocked" || record.Cases[0].AssertionVerdict != "blocked" || actual.Outcome != "tool_missing" || actual.ExitCode != nil || actual.SpawnAttempted || !actual.CleanupComplete || record.Progress.UnreachedProcesses != 2 || record.Cases[1].EngineState != "skipped" {
		t.Fatalf("tool absence became a passing test: %s", record.Cases[0].Actual)
	}
}

func TestProcessRunnerReusesSectionBrowserHarnessWithScreenshotEvidence(t *testing.T) {
	runner, id := startProcessRun(t, lab.Selection{Suite: "test-adapters", Mode: "offline", Configuration: "current", CaseIDs: []string{"browser-section-review"}}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 1, MaxArtifactBytes: 2 << 20, DeadlineMS: 60000})
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	record, err := runner.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Outcome, Origin string
		CleanupComplete bool
		RuntimeVersions map[string]string
		Artifacts       []struct {
			Name, Kind, Origin, Hash string
			Bytes                    []byte
		}
	}
	if len(record.Cases) != 1 || json.Unmarshal(record.Cases[0].Actual, &actual) != nil || actual.Outcome != "passed" || actual.Origin != "isolated-browser-harness" || !actual.CleanupComplete || actual.RuntimeVersions["chrome"] == "" || len(actual.Artifacts) < 4 {
		t.Fatalf("isolated browser evidence unavailable: %s", record.Cases[0].Actual)
	}
	for _, artifact := range actual.Artifacts {
		if artifact.Kind != "screenshot" || artifact.Origin != "isolated-browser-harness" || len(artifact.Bytes) == 0 || artifact.Hash != fmt.Sprintf("%x", sha256.Sum256(artifact.Bytes)) {
			t.Fatal("artifact kind/origin/hash drift")
		}
	}
	if len(record.Dispatches) != 0 || record.Progress.AttemptedHTTP != 0 {
		t.Fatal("browser tests became inference or handler attempts")
	}
}

func TestProcessRunnerExecutesExistingNodeCanonicalContracts(t *testing.T) {
	runner, id := startProcessRun(t, lab.Selection{Suite: "test-adapters", Mode: "offline", Configuration: "current", CaseIDs: []string{"node-profile-document"}}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 1, MaxArtifactBytes: 1 << 20, DeadlineMS: 60000})
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	record, err := runner.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Outcome, Origin string
		CleanupComplete bool
		RuntimeVersions map[string]string
	}
	if len(record.Cases) != 1 || json.Unmarshal(record.Cases[0].Actual, &actual) != nil || record.Cases[0].PlannedSteps != 1 || record.Cases[0].AttemptedSteps != 1 || record.Cases[0].SkippedSteps != 0 || actual.Outcome != "passed" || actual.Origin != "isolated-node-tests" || !actual.CleanupComplete || actual.RuntimeVersions["node"] == "" || record.AssertionVerdict != "pass" || record.Progress.FinishedProcesses != 1 {
		t.Fatalf("isolated Node contract unavailable: %s", record.Cases[0].Actual)
	}
	if len(record.Dispatches) != 0 || record.Progress.AttemptedHTTP != 0 {
		t.Fatal("Node tests became inference or handler attempts")
	}
}

func TestProcessRunnerCrashRetainsDistinctOutcome(t *testing.T) {
	record, actual := runProcessSelector(t, "probe-crash")
	if actual.Outcome != "crashed" || !actual.SpawnAttempted || !actual.NamespaceStopped || !actual.CleanupComplete || record.AssertionVerdict != "blocked" {
		t.Fatalf("crash misclassified: %+v", actual)
	}
}
func runProcessSelector(t *testing.T, selector string) (lab.RunRecord, lab.ProcessResult) {
	t.Helper()
	runner, id := startProcessRun(t, lab.Selection{Suite: "test-adapters", Mode: "offline", Configuration: "current", CaseIDs: []string{selector}}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 1, MaxArtifactBytes: 2 << 20, DeadlineMS: 60000})
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	record, err := runner.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var actual lab.ProcessResult
	if len(record.Cases) != 1 || json.Unmarshal(record.Cases[0].Actual, &actual) != nil {
		t.Fatal("missing structured case")
	}
	if destination := os.Getenv("CAREEROS_LAB_TEST_EVIDENCE_DIR"); destination != "" {
		if err := os.MkdirAll(destination, 0700); err != nil {
			t.Fatal(err)
		}
		raw, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(destination, selector+".v1.json"), append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return record, actual
}

func TestProcessRunnerNativePrintRemainsUnavailable(t *testing.T) {
	record, actual := runProcessSelector(t, "native-print-capability")
	if actual.Outcome != "native_print_unavailable" || actual.SpawnAttempted || len(actual.Artifacts) != 0 || actual.ExitCode != nil || record.AssertionVerdict != "blocked" {
		t.Fatalf("native print evidence invented: %+v", actual)
	}
}

func TestProcessRunnerUsesOfflineFieldHelpers(t *testing.T) {
	record, actual := runProcessSelector(t, "node-field-tooling")
	if actual.Outcome != "passed" || !actual.CleanupComplete || record.Progress.AttemptedHTTP != 0 || len(record.Dispatches) != 0 {
		t.Fatalf("offline helpers failed: %s", actual.OutputText)
	}
}

func TestProcessCatalogPlansExistingWorkflowEntryPoints(t *testing.T) {
	planner := processPlanner(t)
	for _, selector := range []string{"go-canonical", "go-ingestion", "go-job-application", "go-cv", "node-ingestion", "node-job-reuse", "node-artifact-review", "browser-canonical-edit", "browser-ingestion", "browser-enrichment", "browser-application-reuse", "browser-resume-review"} {
		t.Run(selector, func(t *testing.T) {
			plan, err := planner.Plan(lab.Selection{Suite: "test-adapters", Mode: "offline", Configuration: "current", CaseIDs: []string{selector}}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 1, MaxArtifactBytes: 2 << 20, DeadlineMS: 60000})
			if err != nil {
				t.Fatal(err)
			}
			if plan.RequiredProcessAttempts != 1 || plan.RequiredHTTPAttempts != 0 || plan.MaximumPlannedProviderCalls != 0 {
				t.Fatal("test driver admission miscounted")
			}
		})
	}
}

func TestProcessRunnerCDPPDFIsDistinctAndSurvivesExport(t *testing.T) {
	record, actual := runProcessSelector(t, "browser-resume-review")
	if actual.Outcome != "passed" || !actual.CleanupComplete {
		t.Fatalf("browser export harness failed: %s", actual.OutputText)
	}
	kinds := map[string]int{}
	for _, artifact := range actual.Artifacts {
		kinds[artifact.Kind]++
		if artifact.Hash != fmt.Sprintf("%x", sha256.Sum256(artifact.Bytes)) || artifact.Runtime == "" || artifact.Origin != "isolated-browser-harness" {
			t.Fatal("artifact provenance drift")
		}
		if artifact.Kind == "cdp-produced-pdf" && (len(artifact.Bytes) < 5 || string(artifact.Bytes[:5]) != "%PDF-") {
			t.Fatal("invalid PDF bytes")
		}
	}
	if kinds["screenshot"] == 0 || kinds["cdp-produced-pdf"] == 0 || kinds["native-saved-pdf"] != 0 {
		t.Fatalf("artifact distinction unavailable: %+v %v", kinds, actual.UnavailableReasons)
	}
	store, err := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Save(record); err != nil {
		t.Fatal(err)
	}
	exported, err := store.ExportJSON(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.ImportExport(exported)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := store.LoadImport(id)
	if err != nil {
		t.Fatal(err)
	}
	if !imported.Quarantined || string(imported.Original) != string(exported) {
		t.Fatal("export lost quarantine or original bytes")
	}
	var bundle struct {
		Native lab.RunRecord `json:"nativeRun"`
	}
	if json.Unmarshal(imported.Original, &bundle) != nil || len(bundle.Native.Cases) != 1 {
		t.Fatal("native evidence lost")
	}
	var roundtrip lab.ProcessResult
	if json.Unmarshal(bundle.Native.Cases[0].Actual, &roundtrip) != nil || len(roundtrip.Artifacts) != len(actual.Artifacts) {
		t.Fatal("artifact bytes lost on export")
	}
	for i, a := range roundtrip.Artifacts {
		if a.Hash != actual.Artifacts[i].Hash || a.Hash != fmt.Sprintf("%x", sha256.Sum256(a.Bytes)) || a.Kind != actual.Artifacts[i].Kind {
			t.Fatal("export changed artifact evidence")
		}
	}
}

func TestProcessRunnerExistingWorkflowSelectorsExecute(t *testing.T) {
	for _, selector := range []string{"go-canonical", "go-ingestion", "go-job-application", "go-cv", "node-ingestion", "node-job-reuse", "node-artifact-review"} {
		t.Run(selector, func(t *testing.T) {
			record, actual := runProcessSelector(t, selector)
			if actual.Outcome != "passed" || !actual.CleanupComplete || record.Progress.AttemptedHTTP != 0 || len(record.Dispatches) != 0 {
				t.Fatalf("fixed harness unavailable: %s / %s: %s", actual.Outcome, record.EngineState, actual.OutputText[:min(len(actual.OutputText), 4000)])
			}
		})
	}
}

func TestProcessRunnerAssertionRegressionCompletesWithFail(t *testing.T) {
	record, actual := runProcessSelector(t, "probe-assertion")
	if actual.Outcome != "assertion_failed" || actual.ExitCode == nil || *actual.ExitCode != 1 || !actual.CleanupComplete || record.EngineState != "completed" || record.AssertionVerdict != "fail" {
		t.Fatalf("assertion regression misclassified: %+v %s/%s", actual, record.EngineState, record.AssertionVerdict)
	}
}

func TestProcessRunnerAdditionalBrowserInventoryRetainsOutcomes(t *testing.T) {
	for _, selector := range []string{"browser-canonical-edit", "browser-ingestion", "browser-enrichment", "browser-application-reuse"} {
		t.Run(selector, func(t *testing.T) {
			record, actual := runProcessSelector(t, selector)
			if !actual.SpawnAttempted || !actual.NamespaceStopped || !actual.CleanupComplete || actual.Outcome == "source_changed" || actual.Outcome == "tool_missing" || actual.Outcome == "dependency_unavailable" {
				t.Fatalf("browser driver did not execute safely: %s", actual.Outcome)
			}
			switch actual.Outcome {
			case "passed":
				if record.EngineState != "completed" || record.AssertionVerdict != "pass" || actual.ExitCode == nil || *actual.ExitCode != 0 {
					t.Fatal("passing driver state mismatch")
				}
			case "assertion_failed":
				if record.EngineState != "completed" || record.AssertionVerdict != "fail" || actual.ExitCode == nil || *actual.ExitCode == 0 {
					t.Fatal("regression was promoted or blocked")
				}
			case "timed_out":
				if record.EngineState != "failed" || record.AssertionVerdict != "blocked" {
					t.Fatal("unreached driver became passing evidence")
				}
			default:
				t.Fatalf("unexpected driver outcome: %s", actual.Outcome)
			}
			t.Logf("Observed existing harness: outcome=%s engine=%s assertion=%s", actual.Outcome, record.EngineState, record.AssertionVerdict)
		})
	}
}

func processPlanner(t *testing.T) *lab.Planner {
	t.Helper()
	raw, err := os.ReadFile("../../docs/evaluations/test-adapters/cases.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := lab.CaptureSource("../..")
	if err != nil {
		t.Fatal(err)
	}
	planner, err := lab.NewPlanner(map[string][]byte{"test-adapters": raw}, provenance)
	if err != nil {
		t.Fatal(err)
	}
	return planner
}
func startProcessRun(t *testing.T, selection lab.Selection, caps lab.ExecutionCaps) (*lab.Runner, string) {
	t.Helper()
	adapter, err := lab.NewProcessAdapter("../..")
	if err != nil {
		t.Fatal(err)
	}
	if !adapter.Capabilities().IsolationAvailable {
		t.Skip("Host namespace isolation unavailable")
	}
	planner := processPlanner(t)
	plan, err := planner.Plan(selection, caps)
	if err != nil {
		t.Fatal(err)
	}
	runner := lab.NewRunner(planner, lab.RunnerOptions{Processes: adapter})
	id, err := runner.Start(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	return runner, id
}

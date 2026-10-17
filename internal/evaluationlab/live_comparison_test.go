package evaluationlab_test

import (
	"context"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
	"professional-information-repo/internal/semanticeval"
	"testing"
)

func TestGuardFrozenComparisonInterleavesAndEnforcesEquivalentPlans(t *testing.T) {
	store, err := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
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
	guard, err := lab.NewLiveGuard(planner, store, lab.LiveGuardOptions{Enabled: true, HumanReviewer: "controlled-human-example", Origin: "controlled-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	left, right := []string{}, []string{}
	byID := map[string]lab.LivePreflight{}
	for _, caseID := range []string{"en-fact", "pt-fact"} {
		for _, configuration := range []string{"current", "luna-low"} {
			plan, err := planner.Plan(lab.Selection{Suite: "workflow-settings", Mode: "controlled", Configuration: configuration, CaseIDs: []string{caseID}}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 5, MaxArtifactBytes: 1 << 20, DeadlineMS: 5000})
			if err != nil {
				t.Fatal(err)
			}
			preflight, err := guard.Preflight(lab.LivePreflightRequest{Plan: plan, MaxTokenUnits: 3000, MaxOutputTokens: 100, ApprovalLifetimeMS: 60000, Dispatches: []lab.GuardDispatchSpec{
				{CaseID: caseID, HTTPID: caseID + ":initial", Stage: "field_validation", Recipient: "api.typesafe.ai", Model: "jev-1.13.0", TokenReservation: 1000},
				{CaseID: caseID, HTTPID: caseID + ":initial", Stage: "extract", Recipient: "api.openai.com", Model: "gpt-6-luna", TokenReservation: 1000},
				{CaseID: caseID, HTTPID: caseID + ":initial", Stage: "reconcile", Recipient: "api.openai.com", Model: "gpt-6-luna", TokenReservation: 1000},
			}})
			if err != nil {
				t.Fatal(err)
			}
			byID[preflight.ID] = preflight
			if configuration == "current" {
				left = append(left, preflight.ID)
			} else {
				right = append(right, preflight.ID)
			}
		}
	}
	frozen, err := guard.FreezeComparison(left, right, "configuration")
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"current", "luna-low", "luna-low", "current"}
	if frozen.Hash == "" || len(frozen.Schedule) != 4 {
		t.Fatal("comparison not frozen")
	}
	for i, group := range frozen.Schedule {
		if group.Configuration != expected[i] {
			t.Fatal("case/config order was not interleaved")
		}
	}
	approve := func(id string) (lab.GuardAuthorization, error) {
		challenge, err := guard.Approval(id)
		if err != nil {
			return lab.GuardAuthorization{}, err
		}
		return guard.Authorize(lab.GuardApproval{PreflightID: id, Hash: challenge.Hash, Actor: lab.ReviewActor{ID: "controlled-human-example", Type: "human"}, Confirmation: challenge.Confirmation, Rationale: "Controlled finite comparison only"}, map[string]string{"api.openai.com": "synthetic-openai", "api.typesafe.ai": "synthetic-jev"})
	}
	if _, err = approve(frozen.Schedule[1].PreflightID); err == nil {
		t.Fatal("comparison authorized out of order")
	}
	for _, group := range frozen.Schedule {
		token, err := approve(group.PreflightID)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range byID[group.PreflightID].Dispatches {
			if err = guard.Dispatch(context.Background(), token, spec.ID, func(_ context.Context, spec lab.GuardDispatchSpec, _ string) (lab.GuardCallObservation, error) {
				return lab.GuardCallObservation{Outcome: "completed", Call: semanticeval.Call{ReturnedModel: spec.Model, Status: 200}}, nil
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	retainGuardExample(t, "comparison.v1.json", frozen)
	observations := []lab.GuardReport{}
	for _, group := range frozen.Schedule {
		report, reportErr := guard.Report(group.PreflightID)
		if reportErr != nil {
			t.Fatal(reportErr)
		}
		observations = append(observations, report)
	}
	retainGuardExample(t, "comparison-observations.v1.json", observations)
	exported, err := guard.ExportComparison(frozen.ID)
	if err != nil || len(exported) == 0 {
		t.Fatal("frozen comparison unavailable")
	}
}

func TestGuardSequenceKeepsLogicalCaseHTTPAndProviderLegsSeparate(t *testing.T) {
	store, err := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
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
	guard, err := lab.NewLiveGuard(planner, store, lab.LiveGuardOptions{Enabled: true, HumanReviewer: "controlled-human-example", Origin: "controlled-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	plan, err := planner.Plan(lab.Selection{Suite: "workflow-settings", Mode: "controlled", Configuration: "current", CaseIDs: []string{"en-clarification"}}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 2, MaxProviderCalls: 10, MaxArtifactBytes: 2 << 20, DeadlineMS: 5000})
	if err != nil {
		t.Fatal(err)
	}
	specs := []lab.GuardDispatchSpec{}
	for _, leaf := range []string{"en-clarification-initial", "en-clarification-answer"} {
		specs = append(specs, lab.GuardDispatchSpec{CaseID: "en-clarification", LeafID: leaf, HTTPID: leaf, Stage: "gate", Recipient: "api.typesafe.ai", Model: "jev-1.13.0", TokenReservation: 1000}, lab.GuardDispatchSpec{CaseID: "en-clarification", LeafID: leaf, HTTPID: leaf, Stage: "generation", Recipient: "api.openai.com", Model: "gpt-6-luna", TokenReservation: 1000})
	}
	preflight, err := guard.Preflight(lab.LivePreflightRequest{Plan: plan, MaxTokenUnits: 4000, MaxOutputTokens: 100, ApprovalLifetimeMS: 60000, Dispatches: specs})
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := guard.Approval(preflight.ID)
	if err != nil {
		t.Fatal(err)
	}
	token, err := guard.Authorize(lab.GuardApproval{PreflightID: preflight.ID, Hash: challenge.Hash, Actor: lab.ReviewActor{ID: "controlled-human-example", Type: "human"}, Confirmation: challenge.Confirmation, Rationale: "Controlled sequence accounting only, not a production-handler result"}, map[string]string{"api.openai.com": "synthetic-openai", "api.typesafe.ai": "synthetic-jev"})
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
	report, err := guard.Report(preflight.ID)
	values := map[string]float64{}
	for _, metric := range report.Metrics {
		if metric.ID == "M18" && metric.Value != nil {
			values[metric.Measure] = *metric.Value
		}
	}
	if values["controlled-guard-provider-calls"] != 4 || values["controlled-guard-logical-cases"] != 1 || values["controlled-guard-workflow-http-legs"] != 2 {
		t.Fatal("guard metrics conflated admitted populations")
	}
	if err != nil || report.AttemptedLogicalCases != 1 || report.AttemptedHTTPAttempts != 2 || report.AttemptedCalls != 4 {
		t.Fatal("sequence conflated logical, HTTP or provider counters")
	}
}

func TestGuardPreflightRejectsReorderedCasesAndSequenceLeaves(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		cases, leaves []string
	}{
		{"cases", []string{"en-fact", "pt-fact"}, []string{"en-fact", "pt-fact"}},
		{"sequence", []string{"en-clarification"}, []string{"en-clarification-initial", "en-clarification-answer"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, err := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{})
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
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
			guard, err := lab.NewLiveGuard(planner, store, lab.LiveGuardOptions{Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			defer guard.Close()
			plan, err := planner.Plan(lab.Selection{Suite: "workflow-settings", Mode: "controlled", Configuration: "current", CaseIDs: scenario.cases}, lab.ExecutionCaps{MaxCases: 2, MaxHTTPAttempts: 2, MaxProviderCalls: 10, MaxArtifactBytes: 2 << 20, DeadlineMS: 5000})
			if err != nil {
				t.Fatal(err)
			}
			specs := []lab.GuardDispatchSpec{}
			for i, leaf := range scenario.leaves {
				root := scenario.cases[0]
				if len(scenario.cases) > 1 {
					root = scenario.cases[i]
				}
				specs = append(specs, lab.GuardDispatchSpec{CaseID: root, LeafID: leaf, HTTPID: leaf, Stage: "gate", Recipient: "api.typesafe.ai", Model: "jev-1.13.0", TokenReservation: 1000})
			}
			request := lab.LivePreflightRequest{Plan: plan, MaxTokenUnits: 3000, MaxOutputTokens: 100, ApprovalLifetimeMS: 60000, Dispatches: specs}
			if _, err = guard.Preflight(request); err != nil {
				t.Fatal("ordered schedule rejected", err)
			}
			request.Dispatches = []lab.GuardDispatchSpec{specs[1], specs[0]}
			if _, err = guard.Preflight(request); err == nil {
				t.Fatal("reordered frozen cohort accepted")
			}
			request.Dispatches = []lab.GuardDispatchSpec{specs[0], specs[1], specs[0]}
			request.Dispatches[2].Stage = "review"
			if _, err = guard.Preflight(request); err == nil {
				t.Fatal("noncontiguous leaf stages accepted")
			}
		})
	}
}

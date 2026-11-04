package evaluationlab_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	lab "professional-information-repo/internal/evaluationlab"
	eval "professional-information-repo/internal/semanticeval"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func legacyIngestionVariant(t *testing.T) (lab.ResolvedVariant, *lab.Planner) {
	t.Helper()
	variant, err := lab.ResolveVariant("semantic-quality", lab.LegacyIngestionVariantID, "current")
	if err != nil {
		t.Fatal(err)
	}
	return variant, semanticPlanner(t)
}

func legacyIngestionCaps() lab.ExecutionCaps {
	return lab.ExecutionCaps{MaxCases: 5, MaxHTTPAttempts: 5, MaxProviderCalls: 25, MaxArtifactBytes: 8 << 20, DeadlineMS: 10000}
}

func TestLegacyIngestionVariantExecutesFixedFiveCaseProposalCohort(t *testing.T) {
	var external atomic.Int64
	previous := http.DefaultTransport
	http.DefaultTransport = deniedTransport{calls: &external}
	t.Cleanup(func() { http.DefaultTransport = previous })

	variant, planner := legacyIngestionVariant(t)
	plan, err := variant.Plan(planner, nil, legacyIngestionCaps())
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := lab.LegacyIngestionProposalCaseIDs()
	if !reflect.DeepEqual(plan.Selection.CaseIDs, wantOrder) || !reflect.DeepEqual(plan.CaseOrder, wantOrder) {
		t.Fatalf("cohort order was not frozen: selection=%v order=%v", plan.Selection.CaseIDs, plan.CaseOrder)
	}
	if plan.RequiredHTTPAttempts != 5 || plan.MaximumPlannedProviderCalls != 25 {
		t.Fatalf("unexpected plan ceilings: %+v", plan)
	}

	runner := lab.NewRunner(planner, lab.RunnerOptions{})
	id, err := variant.Start(context.Background(), runner, plan)
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, runner, id)
	if record.VariantID != lab.LegacyIngestionVariantID || record.EngineState != "completed" || record.AssertionVerdict != "unavailable" {
		t.Fatalf("wrong proposal run identity or terminal state: %+v", record)
	}
	if record.Context.ScorerVersion != "" {
		t.Fatalf("proposal-only run retained a scorer identity: %q", record.Context.ScorerVersion)
	}
	if len(record.Cases) != 5 {
		t.Fatalf("wrong cohort size: %d", len(record.Cases))
	}
	for i, c := range record.Cases {
		if c.ID != wantOrder[i] || c.EngineState != "completed" || c.AssertionVerdict != "unavailable" {
			t.Fatalf("case %d lacks proposal-only completion: %+v", i, c)
		}
		if c.OperationalOutcome != "completed" {
			t.Fatalf("case %d lacks its observed operational outcome: %+v", i, c)
		}
		if !containsReason(c.UnavailableReasons, "proposal-only execution") {
			t.Fatalf("case %d does not disclose its scope: %+v", i, c.UnavailableReasons)
		}
	}
	if record.OperationalOutcomes["completed"] != 5 {
		t.Fatalf("operational outcomes do not cover the cohort: %+v", record.OperationalOutcomes)
	}
	for _, metric := range record.Metrics {
		if metric.ID == "M03" || metric.ID == "M05" {
			t.Fatalf("proposal-only run derived a semantic score metric: %+v", metric)
		}
	}

	seen := map[string]bool{}
	perCase := map[string]int{}
	for _, dispatch := range record.Dispatches {
		if dispatch.Origin != "simulated" || dispatch.State != "completed" || dispatch.Identity == "" {
			t.Fatalf("non-simulated or incomplete dispatch: %+v", dispatch)
		}
		if seen[dispatch.Identity] {
			t.Fatalf("duplicate dispatch identity: %s", dispatch.Identity)
		}
		seen[dispatch.Identity] = true
		perCase[dispatch.LogicalCaseIdentity]++
	}
	if len(record.Dispatches) != 15 || record.Progress.AttemptedDispatches != 15 {
		t.Fatalf("wrong dispatch count: ledger=%d attempted=%d", len(record.Dispatches), record.Progress.AttemptedDispatches)
	}
	for _, c := range record.Cases {
		if perCase[c.Identity] != 3 {
			t.Fatalf("case %s did not retain its three simulated dispatches: %d", c.ID, perCase[c.Identity])
		}
	}

	var fixture struct {
		Cases []struct {
			ID      string          `json:"id"`
			Request json.RawMessage `json:"request"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(taskFixture(t), &fixture); err != nil {
		t.Fatal(err)
	}
	requests := map[string]json.RawMessage{}
	for _, c := range fixture.Cases {
		requests[c.ID] = c.Request
	}
	evidences := make([]lab.LegacyIngestionExecutionEvidence, len(record.Cases))
	for i, c := range record.Cases {
		var evidence lab.LegacyIngestionExecutionEvidence
		if err := json.Unmarshal(c.Actual, &evidence); err != nil {
			t.Fatal(err)
		}
		evidences[i] = evidence
		if evidence.Version != lab.LegacyIngestionEvidenceVersion || evidence.VariantID != lab.LegacyIngestionVariantID || evidence.CaseID != c.ID || evidence.Language != c.Language {
			t.Fatalf("wrong evidence identity for %s: %+v", c.ID, evidence)
		}
		if evidence.RequestHash != eval.Hash(requests[c.ID]) || evidence.Response.ID != c.ID || evidence.Response.Status != 200 || len(evidence.Response.Body) == 0 {
			t.Fatalf("raw handler response was not retained for %s: %+v", c.ID, evidence)
		}
		if evidence.Observation.Version != eval.IngestionObservationVersion || len(evidence.Observation.Raw) != 1 || !bytes.Equal(evidence.Observation.Raw[0].Response.Body, evidence.Response.Body) {
			t.Fatalf("observation does not retain the exact response bytes for %s: %+v", c.ID, evidence.Observation)
		}
		if !strings.Contains(evidence.Scope, "reviewable proposal only") || !strings.Contains(evidence.Scope, "Apply/Save") {
			t.Fatalf("proposal-only scope is not explicit for %s: %q", c.ID, evidence.Scope)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(c.Actual, &fields); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"gold", "forbidden", "expected", "score", "assertionVerdict"} {
			if _, present := fields[forbidden]; present {
				t.Fatalf("execution evidence for %s leaked %q: %s", c.ID, forbidden, c.Actual)
			}
		}
	}
	evidence := evidences[0]
	if !evidence.Observation.Availability["endpoint"].Available || len(evidence.Observation.Claims) == 0 || len(evidence.Observation.Operations) == 0 {
		t.Fatalf("legacy proposal was not mechanically projected: %+v", evidence.Observation)
	}

	// The endpoint evaluator consumes the captured response and observation only
	// after execution; it is not part of the handler run and cannot feed gold back.
	request := endpointRequest(t, "en-fact")
	request.Binding = lab.EndpointEvaluationBinding{ExperimentHash: strings.Repeat("a", 64), VariantID: lab.LegacyIngestionVariantID, RunID: record.ID, RunSnapshotHash: strings.Repeat("b", 64)}
	request.Evidence = lab.IngestionEndpointEvidence{Legacy: &evidence.Response}
	request.Observation = &evidence.Observation
	before, _ := json.Marshal(request)
	receipt, err := lab.EvaluateIngestionEndpoint(request)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(request)
	if !bytes.Equal(before, after) {
		t.Fatal("endpoint evaluation mutated captured execution evidence")
	}
	if receipt.EndpointState != "available" {
		t.Fatalf("captured legacy proposal was not evaluable: %+v", receipt)
	}
	for _, facet := range []string{"claim-diagnostics", "proposal-operations"} {
		counts := receipt.Facets[facet].Counts
		if counts == nil || counts.Expected != 1 || counts.Observed != 1 || counts.Matched != 1 || counts.Covered != 1 || len(counts.Missing) != 0 {
			t.Fatalf("wrong %s counts for captured response: %+v", facet, counts)
		}
	}
	if receipt.HumanReview != "pending" || receipt.SemanticReview != "unavailable" {
		t.Fatalf("endpoint evaluation invented acceptance: %+v", receipt)
	}
	if external.Load() != 0 {
		t.Fatalf("proposal execution attempted %d external requests", external.Load())
	}
}

func TestLegacyIngestionVariantRequiresExactCohortAndPinnedConfiguration(t *testing.T) {
	variant, planner := legacyIngestionVariant(t)
	cohort := lab.LegacyIngestionProposalCaseIDs()
	reversed := append([]string{}, cohort...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	for _, ids := range [][]string{
		{"en-fact"},
		{"en-fact", "pt-fact"},
		append(append([]string{}, cohort...), "en-fact"),
		append(append([]string{}, cohort[:4]...), "en-ambiguous-date"),
		reversed[:4],
	} {
		if plan, err := variant.Plan(planner, ids, legacyIngestionCaps()); err == nil {
			t.Fatalf("accepted non-cohort selection %v: %+v", ids, plan.Selection)
		}
	}
	plan, err := variant.Plan(planner, reversed, legacyIngestionCaps())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Selection.CaseIDs, cohort) || !reflect.DeepEqual(plan.CaseOrder, cohort) {
		t.Fatalf("caller order changed the frozen cohort: %+v", plan)
	}

	if _, err := lab.ResolveVariant("semantic-quality", lab.LegacyIngestionVariantID, "luna-low"); err == nil {
		t.Fatal("legacy proposal variant accepted an unpinned model configuration")
	}

	native, err := lab.ResolveVariant("semantic-quality", "semantic-quality-native-v1", "current")
	if err != nil {
		t.Fatal(err)
	}
	nativePlan, err := native.Plan(planner, []string{"en-fact"}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 5, MaxArtifactBytes: 1 << 20, DeadlineMS: 5000})
	if err != nil {
		t.Fatal(err)
	}
	runner := lab.NewRunner(planner, lab.RunnerOptions{})
	if id, err := variant.Start(context.Background(), runner, nativePlan); err == nil || id != "" {
		t.Fatalf("legacy proposal variant accepted a native one-case plan: id=%s err=%v", id, err)
	}
	altered := plan
	altered.CaseOrder = reversed
	if id, err := variant.Start(context.Background(), runner, altered); err == nil || id != "" {
		t.Fatalf("legacy proposal variant accepted altered cohort order: id=%s err=%v", id, err)
	}
	if id, err := variant.Start(context.Background(), runner, plan); err != nil || id == "" {
		t.Fatalf("valid proposal plan was rejected after mutations: id=%s err=%v", id, err)
	} else {
		awaitRun(t, runner, id)
	}
}

func TestCatalogKeepsNativeSemanticVariantFirstAndRegistersLegacyProposalSecond(t *testing.T) {
	var semantic *lab.Suite
	for _, suite := range lab.Catalog() {
		if suite.ID == "semantic-quality" {
			copy := suite
			semantic = &copy
		}
	}
	if semantic == nil || len(semantic.Variants) != 2 {
		t.Fatalf("semantic-quality variants changed: %+v", semantic)
	}
	if semantic.Variants[0].ID != "semantic-quality-native-v1" || semantic.Variants[0].AdapterVersion != "lab-native-adapter-v1" {
		t.Fatalf("native variant is not first: %+v", semantic.Variants[0])
	}
	if semantic.Variants[1].ID != lab.LegacyIngestionVariantID || semantic.Variants[1].AdapterVersion != lab.LegacyIngestionAdapterVersion {
		t.Fatalf("legacy proposal descriptor changed: %+v", semantic.Variants[1])
	}
	if !reflect.DeepEqual(semantic.Variants[1].Modes, []string{"controlled"}) || !reflect.DeepEqual(semantic.Variants[1].Configurations, []string{"current"}) {
		t.Fatalf("legacy proposal descriptor widened its selection: %+v", semantic.Variants[1])
	}
}

func containsReason(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

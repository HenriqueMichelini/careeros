package evaluationlab_test

import (
	"math"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
	"professional-information-repo/internal/semanticeval"
	"testing"
)

func TestCostIncludesFailedCallsAndKeepsPartialTotalsUnavailable(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	call := semanticeval.Call{Provider: "api.openai.com", ReturnedModel: "exact-model", InputTokens: n(100), CachedTokens: n(20), CacheWriteTokens: n(0), OutputTokens: n(10), ReasoningTokens: n(5), Status: 500}
	rates := []semanticeval.Rate{{Model: "exact-model", AsOf: "2026-10-15", Input: 2, Cached: 1, Write: 2, Output: 10}}
	ledger := []lab.Dispatch{{Identity: "a", Origin: "historical-provider-observed", Call: call}, {Identity: "b", Origin: "historical-provider-observed", Call: call}}
	r, err := lab.SummarizeDispatches(ledger, rates)
	if err != nil {
		t.Fatal(err)
	}
	if r.Calls != 2 || r.TotalUSD == nil || math.Abs(*r.TotalUSD-0.00056) > 1e-12 || r.KnownUSD == nil {
		t.Fatalf("failed call lost from cost: %+v", r)
	}
	ledger[1].Call.OutputTokens = nil
	r, err = lab.SummarizeDispatches(ledger, rates)
	if err != nil {
		t.Fatal(err)
	}
	if r.TotalUSD != nil || r.KnownUSD == nil || r.Unpriced != 1 || r.Output.Missing != 1 {
		t.Fatalf("partial usage invented full total: %+v", r)
	}
	ledger[0].Call.ReasoningTokens = n(11)
	r, err = lab.SummarizeDispatches(ledger, rates)
	if err != nil {
		t.Fatal(err)
	}
	if r.InconsistentCalls != 1 || r.TotalUSD != nil {
		t.Fatalf("inconsistent reasoning subset accepted: %+v", r)
	}
}

func TestUsageDoesNotPoolSimulatedAndProviderObservedCalls(t *testing.T) {
	_, err := lab.SummarizeDispatches([]lab.Dispatch{{Identity: "a", Origin: "simulated"}, {Identity: "b", Origin: "provider-observed"}}, nil)
	if err == nil {
		t.Fatal("different evidence modes pooled")
	}
}

func TestAcceptedCostRequiresNamedEvidenceAndChargesAllCohortCalls(t *testing.T) {
	total := 0.6
	accepted := 1
	cost, err := lab.ResultCosts(lab.UsageSummary{TotalUSD: &total}, "all compared current configuration calls", 2, &accepted, "")
	if err != nil || cost.PerCompletedUSD == nil || *cost.PerCompletedUSD != 0.3 || cost.PerAcceptedUSD != nil {
		t.Fatalf("unnamed acceptance acquired cost: %+v %v", cost, err)
	}
	cost, err = lab.ResultCosts(lab.UsageSummary{TotalUSD: &total}, "all compared current configuration calls", 2, &accepted, "source-grounded evaluator acceptance")
	if err != nil || cost.PerAcceptedUSD == nil || *cost.PerAcceptedUSD != 0.6 {
		t.Fatalf("named accepted cohort cost: %+v %v", cost, err)
	}
}

func TestDispatchLedgerCountsFlattenedSettingsCallsOnceAndLabelsSimulation(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/workflow-settings/controlled.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	ledger := lab.DispatchLedger(r)
	if len(ledger) != 186 {
		t.Fatalf("dispatch count: %d", len(ledger))
	}
	seen := map[string]bool{}
	for _, call := range ledger {
		if call.Origin != "simulated" || call.Identity == "" || seen[call.Identity] {
			t.Fatalf("invalid simulation identity: %+v", call)
		}
		seen[call.Identity] = true
	}
}

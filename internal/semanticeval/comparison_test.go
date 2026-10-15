package semanticeval

import (
	"encoding/json"
	"math"
	"professional-information-repo/internal/aidiagnostics"
	"testing"
)

func TestComparisonKeepsUnknownAndExpectedRejectionSeparateFromFailure(t *testing.T) {
	runs := []ComparisonRun{
		{Configuration: "current", Run: RunResult{Status: 200, Diagnostics: &aidiagnostics.Attempt{Outcome: "future_outcome"}}},
		{Configuration: "current", Run: RunResult{Status: 200, Diagnostics: &aidiagnostics.Attempt{Outcome: "irrelevant"}}},
		{Configuration: "current", Run: RunResult{Status: 502, Diagnostics: &aidiagnostics.Attempt{Outcome: "transport_failure"}}},
	}
	summaries, err := Compare(runs, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := summaries["current"]
	if s.Completed != 0 || s.Failed != 1 || s.OperationalOutcomes["unknown"] != 1 || s.OperationalOutcomes["policy_rejection"] != 1 {
		t.Fatalf("outcomes: %+v", s)
	}
}

func TestComparisonCountsNestedLeavesOnceAndReportsUnreachedRequiredLegs(t *testing.T) {
	leaf := RunResult{Status: 200, Diagnostics: &aidiagnostics.Attempt{Outcome: "completed", DurationMS: 3}, Calls: []Call{{Provider: "controlled"}}}
	parent := RunResult{Status: 200, RequiredSteps: 2, Steps: []RunResult{{Status: 200, RequiredSteps: 2, Steps: []RunResult{leaf, leaf}, Calls: append(append([]Call{}, leaf.Calls...), leaf.Calls...)}}, Calls: append(append([]Call{}, leaf.Calls...), leaf.Calls...)}
	summaries, err := Compare([]ComparisonRun{{Configuration: "current", Run: parent}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := summaries["current"]
	if s.Completed != 0 || s.LeafAttempts != 2 || s.RequiredUnreached != 1 || s.Calls != 2 || s.DurationMS != 6 || s.OperationalOutcomes["incomplete"] != 1 {
		t.Fatalf("sequence accounting: %+v", s)
	}
}

func TestComparisonChargesFailuresAndCacheWritesBeforeAcceptedCost(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	call := Call{Provider: "api.openai.com", ReturnedModel: "gpt-6-luna", InputTokens: n(1200), OutputTokens: n(40), CachedTokens: n(1024), CacheWriteTokens: n(128), ReasoningTokens: n(10)}
	runs := []ComparisonRun{{Configuration: "current", Run: RunResult{Status: 502, Calls: []Call{call}}}, {Configuration: "current", Run: RunResult{Status: 200, Calls: []Call{call}}, Review: &Review{Accepted: true, Method: "agent-assisted source review", Reason: "Each assertion supported"}}}
	summary, err := Compare(runs, []Rate{{Model: "gpt-6-luna", AsOf: "2026-10-10", Input: 0.1, Cached: 0.01, Write: 0.125, Output: 0.5}})
	if err != nil {
		t.Fatal(err)
	}
	s := summary["current"]
	if s.TotalUSD == nil || math.Abs(*s.TotalUSD-0.00010208) > 1e-10 || s.PerAcceptedUSD == nil || *s.PerAcceptedUSD != *s.TotalUSD || s.Accepted != 1 || s.Failed != 1 {
		t.Fatalf("incorrect accepted-result accounting: %+v", s)
	}
}

func TestComparisonDoesNotInferAcceptanceOrPriceMissingValidation(t *testing.T) {
	runs := []ComparisonRun{{Configuration: "current", Run: RunResult{Status: 200, Score: Result{Precision: 1, Completeness: 1}, Calls: []Call{{Provider: "api.typesafe.ai"}}, Output: json.RawMessage(`{}`)}}}
	summary, err := Compare(runs, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := summary["current"]
	if s.Accepted != 0 || s.TotalUSD != nil || s.PerAcceptedUSD != nil || s.Unpriced != 1 {
		t.Fatalf("invented cost/acceptance: %+v", s)
	}
}

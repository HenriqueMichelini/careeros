package semanticeval

import (
	"encoding/json"
	"math"
	"testing"
)

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

package aidiagnostics_test

import (
	d "professional-information-repo/internal/aidiagnostics"
	"testing"
)

func i(n int64) *int64 { return &n }
func TestReportKeepsMissingBillingAndAcceptanceUnavailable(t *testing.T) {
	model := "gpt-6-luna"
	yes := true
	attempts := []d.Attempt{
		{Outcome: "completed", Accepted: &yes, Stages: []*d.Stage{{Name: "extraction", Provider: &d.Provider{Name: "openai", Model: &model, InputTokens: i(100), OutputTokens: i(20), CachedTokens: i(40)}}}},
		{Outcome: "truncated", Stages: []*d.Stage{{Name: "extraction", Provider: &d.Provider{Name: "openai", Model: &model, InputTokens: i(50)}}}},
	}
	report, err := d.Summarize(attempts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Requests != 2 || report.Completed != 1 || report.Accepted != 1 || report.CompletionRate == nil || *report.CompletionRate != 0.5 || report.Input.Observed == nil || *report.Input.Observed != 150 || report.Output.Unavailable != 1 || report.EstimatedTotalUSD != nil {
		t.Fatalf("%+v", report)
	}
	// Exact model and dated caller-supplied synthetic prices; output tokens already
	// include reasoning, and cached input is a subset of total input.
	prices := []d.Price{{Provider: "openai", Model: model, AsOf: "2026-10-08", InputPerMillion: 2, CachedPerMillion: 1, OutputPerMillion: 10}}
	report, err = d.Summarize(attempts[:1], prices)
	if err != nil {
		t.Fatal(err)
	}
	if report.EstimatedTotalUSD == nil || *report.EstimatedTotalUSD != 0.00036 || report.EstimatedPerAcceptedUSD == nil || *report.EstimatedPerAcceptedUSD != 0.00036 {
		t.Fatalf("%+v", report)
	}
}

func TestReportChargesFailedAttemptsToCompletedResultAndRequiresAcceptanceEvidence(t *testing.T) {
	model := "gpt-6-luna"
	stage := func() *d.Stage {
		return &d.Stage{Provider: &d.Provider{Name: "openai", Model: &model, InputTokens: i(100), OutputTokens: i(20), CachedTokens: i(40)}}
	}
	attempts := []d.Attempt{{Outcome: "completed", Stages: []*d.Stage{stage()}}, {Outcome: "truncated", Stages: []*d.Stage{stage()}}}
	report, err := d.Summarize(attempts, []d.Price{{Provider: "openai", Model: model, AsOf: "2026-10-08", InputPerMillion: 2, CachedPerMillion: 1, OutputPerMillion: 10}})
	if err != nil {
		t.Fatal(err)
	}
	if report.EstimatedPerCompletedUSD == nil || *report.EstimatedPerCompletedUSD != 0.00072 || report.EstimatedPerAcceptedUSD != nil || report.AcceptanceRate != nil {
		t.Fatalf("%+v", report)
	}
}

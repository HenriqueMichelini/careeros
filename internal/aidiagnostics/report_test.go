package aidiagnostics_test

import (
	"encoding/json"
	"os"
	d "professional-information-repo/internal/aidiagnostics"
	"testing"
)

func TestRetainedControlledDraftsCountAsCompletedWithoutRewritingOutcomes(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/workflow-settings/controlled.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var artifact struct {
		Runs []struct {
			Run struct {
				Diagnostics *d.Attempt        `json:"diagnostics"`
				Steps       []json.RawMessage `json:"steps"`
			} `json:"run"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatal(err)
	}
	var attempts []d.Attempt
	for _, entry := range artifact.Runs {
		if len(entry.Run.Steps) == 0 {
			attempts = append(attempts, *entry.Run.Diagnostics)
		}
	}
	report, err := d.Summarize(attempts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Attempts != 24 || report.Completed != 24 || report.Failed != 0 || report.Outcomes["complete"] != 6 || report.Outcomes["completed"] != 18 {
		t.Fatalf("historical completion interpretation: %+v", report)
	}
}

func i(n int64) *int64 { return &n }

func TestReportSeparatesNoncompletionFromInfrastructureFailure(t *testing.T) {
	attempts := []d.Attempt{{Outcome: "request_information"}, {Outcome: "input_rejected"}, {Outcome: "incomplete"}, {Outcome: "canceled"}, {Outcome: "unavailable"}, {Outcome: "future_outcome"}, {Outcome: "transport_failure"}}
	r, err := d.Summarize(attempts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Failed != 1 || r.Completed != 0 || r.OutcomeNormalizerVersion != "terminal-outcomes-v1" {
		t.Fatalf("classification: %+v", r)
	}
	for _, category := range []string{"requested_information", "policy_rejection", "incomplete", "canceled", "assessment_unavailable", "unknown", "infrastructure_failure"} {
		if r.OperationalOutcomes[category] != 1 {
			t.Fatalf("missing category %s: %+v", category, r)
		}
	}
}

func TestReportRecognizesProductionFieldPolicyOutcomes(t *testing.T) {
	for _, tc := range []struct{ outcome, category string }{
		{"reject_attack", "policy_rejection"}, {"unusable", "policy_rejection"}, {"request_rephrasing", "requested_rephrasing"},
	} {
		r, err := d.Summarize([]d.Attempt{{Outcome: tc.outcome}}, nil)
		if err != nil || r.OperationalOutcomes[tc.category] != 1 || r.Completed != 0 || r.Failed != 0 {
			t.Fatalf("%s: %+v %v", tc.outcome, r, err)
		}
	}
}
func TestReportKeepsMissingBillingAndAcceptanceUnavailable(t *testing.T) {
	model := "gpt-6-luna"
	yes := true
	attempts := []d.Attempt{
		{Outcome: "completed", Accepted: &yes, Stages: []*d.Stage{{Name: "extraction", Provider: &d.Provider{Name: "openai", Model: &model, TokenUsage: d.TokenUsage{InputTokens: i(100), OutputTokens: i(20), CachedTokens: i(40)}}}}},
		{Outcome: "truncated", Stages: []*d.Stage{{Name: "extraction", Provider: &d.Provider{Name: "openai", Model: &model, TokenUsage: d.TokenUsage{InputTokens: i(50)}}}}},
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
		return &d.Stage{Provider: &d.Provider{Name: "openai", Model: &model, TokenUsage: d.TokenUsage{InputTokens: i(100), OutputTokens: i(20), CachedTokens: i(40)}}}
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

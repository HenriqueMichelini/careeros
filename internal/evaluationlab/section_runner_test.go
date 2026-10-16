package evaluationlab_test

import (
	"context"
	"encoding/json"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func TestSectionSuiteRunsSelectiveCasesWithMaterialFacetsAndPrivacyOpportunities(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/section-review/cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	planner, err := lab.NewPlanner(map[string][]byte{"section-review": raw})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planner.Plan(lab.Selection{Suite: "section-review", Mode: "controlled", Configuration: "current", CaseIDs: []string{"en-grounded", "pt-valid-reference-mismatch", "en-outage"}}, lab.ExecutionCaps{MaxCases: 3, MaxHTTPAttempts: 3, MaxProviderCalls: 6, MaxArtifactBytes: 2 << 20, DeadlineMS: 5000})
	if err != nil {
		t.Fatal(err)
	}
	runner := lab.NewRunner(planner, lab.RunnerOptions{})
	id, err := runner.Start(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, runner, id)
	if record.EngineState != "completed" || record.AssertionVerdict != "pass" || len(record.Dispatches) != 5 {
		t.Fatalf("section runner drift: %s/%s calls%d", record.EngineState, record.AssertionVerdict, len(record.Dispatches))
	}
	var source struct {
		SourceSpans    []json.RawMessage `json:"sourceSpans"`
		Adjudication   string            `json:"adjudication"`
		RequiredStages []string          `json:"requiredStages"`
	}
	json.Unmarshal(record.Cases[0].Source, &source)
	if len(source.SourceSpans) == 0 || source.Adjudication != "pending" || len(source.RequiredStages) != 2 {
		t.Fatal("lazy source/gold scope lost")
	}
	foundPrecision, foundPrivacy, foundUnavailable := false, false, false
	for _, metric := range record.Metrics {
		if metric.CaseID == "en-grounded" && metric.Measure == "curated-label-precision" {
			foundPrecision = true
			if metric.Field != "skills" || metric.Numerator == nil || *metric.Numerator != 1 || metric.Denominator == nil || *metric.Denominator != 1 {
				t.Fatal("style summary diluted material facet")
			}
		}
		if metric.CaseID == "en-grounded" && metric.ID == "M28" {
			foundPrivacy = true
			if metric.Numerator == nil || *metric.Numerator != 0 || metric.Denominator == nil || *metric.Denominator != 2 {
				t.Fatal("privacy opportunity accounting wrong")
			}
		}
		if metric.CaseID == "en-outage" && metric.Measure == "curated-label-completeness" {
			foundUnavailable = true
			if metric.Value != nil || metric.Eligible != 1 {
				t.Fatal("failed output got passing completeness")
			}
		}
		if metric.ID == "M10" && metric.Value != nil {
			t.Fatal("scripted judgments became real-assessor accuracy")
		}
	}
	if !foundPrecision || !foundPrivacy || !foundUnavailable {
		t.Fatal("metric facets/opportunities missing")
	}
}

func TestRetainedSectionReportKeepsExactFixtureScopeAndPendingClientDecision(t *testing.T) {
	report, err := os.ReadFile("../../docs/evaluations/section-review/controlled.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("../../docs/evaluations/section-review/cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	imported, err := lab.ImportWithDependencies(report, [][]byte{fixture})
	if err != nil {
		t.Fatal(err)
	}
	if imported.Quarantined || len(imported.Cases) != 44 || len(imported.Dependencies) != 1 {
		t.Fatal("registered evidence lost")
	}
	first := imported.Cases[0]
	var review struct {
		State string `json:"state"`
	}
	json.Unmarshal(first.Review, &review)
	if review.State != "unavailable" {
		t.Fatal("scripted handler became client acceptance")
	}
	for _, metric := range lab.Metrics(imported) {
		if metric.CaseID == "en-grounded" && metric.Measure == "curated-label-precision" {
			if metric.Field != "skills" || metric.Denominator == nil || *metric.Denominator != 1 {
				t.Fatal("historical style summary diluted material labels")
			}
			return
		}
	}
	t.Fatal("retained material metric missing")
}

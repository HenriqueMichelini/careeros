package evaluationlab_test

import (
	"os"
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func TestApplicationRetrievalUsesFifteenUniqueLabeledFacts(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/application-context/controlled-report.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	denominator := 0
	rows := 0
	for _, m := range lab.Metrics(r) {
		if m.ID == "M08" && m.Measure == "post-budget-relevant-retention" && m.Denominator != nil {
			denominator += *m.Denominator
			rows++
		}
	}
	if rows != 13 || denominator != 15 {
		t.Fatalf("wrong labeled opportunities: rows=%d denominator=%d", rows, denominator)
	}
}

func TestRetrievalSeparatesNegativeRetentionLanguageAndSelectionExclusions(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/application-context/controlled-report.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	negative := 0
	for _, m := range lab.Metrics(r) {
		if m.ID != "M08" || m.Denominator == nil {
			continue
		}
		if m.Scope != "application-facts" || m.Language == "" || m.LabelHash == "" || m.BudgetExcluded == nil || m.RelevanceExcluded == nil {
			t.Fatalf("retrieval context lost: %+v", m)
		}
		if m.Measure == "post-budget-negative-retention" {
			negative++
		}
	}
	if negative != 13 {
		t.Fatalf("negative retention rows: %d", negative)
	}
}

func TestRetainedSemanticMetricsExposeCuratedCountsWithoutOverallScore(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/controlled.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, m := range lab.Metrics(r) {
		if m.Measure != "curated-label-precision" {
			continue
		}
		count++
		if m.CaseID == "" || m.Denominator == nil || m.Source == "" || m.Estimator == "" {
			t.Fatalf("missing metric provenance: %+v", m)
		}
		if *m.Denominator == 0 && m.Value != nil {
			t.Fatal("zero opportunity acquired perfect score")
		}
	}
	if count == 0 {
		t.Fatal("retained curated measures missing")
	}
}

func TestMetricsExposeUsageCoverageAndDatedCostWithoutAcceptedInference(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/workflow-settings/reviewed.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	foundSubtotal := false
	foundMissingTotal := false
	foundUsage := false
	for _, m := range lab.Metrics(r) {
		if m.ID == "M23" && m.Measure == "known-priced-subtotal" && m.Value != nil {
			foundSubtotal = true
		}
		if m.ID == "M23" && m.Measure == "full-cohort-cost" && m.Value == nil && m.UnavailableReason != "" {
			foundMissingTotal = true
		}
		if m.ID == "M21" && m.Measure == "input-tokens" && m.ObservationCount != nil {
			foundUsage = true
		}
	}
	if !foundSubtotal || !foundMissingTotal || !foundUsage {
		t.Fatal("missing partial cost or nullable usage derivation")
	}
}

func TestSettingsMetricsKeepLogicalCasesHttpLegsAndCallsDistinct(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/workflow-settings/controlled.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]float64{}
	for _, m := range lab.Metrics(r) {
		if m.ID == "M18" && m.Value != nil {
			counts[m.Measure] = *m.Value
		}
	}
	if counts["logical-case-count"] != 42 || counts["http-leaf-attempt-count"] != 60 || counts["dispatch-count"] != 186 {
		t.Fatalf("conflated calls and logical cases: %+v", counts)
	}
}

func TestMetricRegistryIncludesEveryContractWithExplicitMissingness(t *testing.T) {
	registry := lab.MetricRegistry()
	if len(registry) != 29 {
		t.Fatalf("registry has %d definitions", len(registry))
	}
	for _, definition := range registry {
		if definition.ID == "" || definition.Version == "" || definition.Definition == "" {
			t.Fatalf("incomplete definition: %+v", definition)
		}
	}
	metrics := lab.Metrics(lab.ImportResult{Quarantined: true})
	if len(metrics) != 29 {
		t.Fatal("missing unavailable capabilities")
	}
	for _, m := range metrics {
		if m.Value != nil || m.UnavailableReason == "" {
			t.Fatalf("quarantine acquired score: %+v", m)
		}
	}
}

func TestZeroOpportunityMetricIsUnavailableRatherThanPerfect(t *testing.T) {
	m := lab.CountMetric("M03", "curated-label-precision", 0, 0, "controlled", "hash")
	if m.Value != nil || m.Denominator == nil || *m.Denominator != 0 || m.UnavailableReason != "not applicable: zero opportunities" {
		t.Fatalf("empty score: %+v", m)
	}
	m = lab.CountMetric("M03", "curated-label-precision", 2, 4, "controlled", "hash")
	if m.Value == nil || *m.Value != 0.5 {
		t.Fatalf("ratio: %+v", m)
	}
}

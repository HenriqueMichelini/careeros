package evaluationlab_test

import (
	"encoding/json"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func TestHistoricalComparisonsDeclineMissingRuntimeAndChangedSourceEvidence(t *testing.T) {
	read := func(path string) lab.ImportResult {
		raw, err := os.ReadFile("../../docs/evaluations/" + path)
		if err != nil {
			t.Fatal(err)
		}
		r, err := lab.Import(raw)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	left := read("semantic-quality/controlled.v1.json")
	right := read("semantic-quality/controlled.v2.json")
	a := lab.ComparisonContract{FixtureHash: left.Provenance.FixtureHash, LabelHash: left.Provenance.FixtureHash, SourceVersion: left.Provenance.Commit, ScorerVersion: left.Provenance.SourceHashes["internal/semanticeval/score.go"], Mode: left.OriginMode, Configuration: "current"}
	b := lab.ComparisonContract{FixtureHash: right.Provenance.FixtureHash, LabelHash: right.Provenance.FixtureHash, SourceVersion: right.Provenance.Commit, ScorerVersion: right.Provenance.SourceHashes["internal/semanticeval/score.go"], Mode: right.OriginMode, Configuration: "candidate"}
	r := lab.CompareCases(a, b, "configuration", left.Cases, right.Cases)
	if r.Compatibility.Compatible || len(r.Compatibility.Reasons) < 3 || len(r.Deltas) != 0 {
		t.Fatal("historical source/label/runtime mismatch pooled")
	}
}

func TestCompatibleCaseComparisonShowsDeltasAndNeverPoolsPartialPairs(t *testing.T) {
	contract := lab.ComparisonContract{FixtureHash: "f", LabelHash: "l", SourceVersion: "s", PromptVersion: "p", SchemaVersion: "j", ScorerVersion: "v", StrategyVersion: "r", BudgetVersion: "b", RuntimeVersion: "t", Mode: "controlled", Configuration: "current"}
	candidate := contract
	candidate.Configuration = "candidate"
	left := []lab.ImportedCase{{ID: "en-case", Identity: "left", Kind: "workflow-result", Configuration: "current", Actual: json.RawMessage(`{"status":200,"score":{"forbiddenAdditions":0,"relationshipErrors":0,"missing":[]}}`)}}
	right := []lab.ImportedCase{{ID: "en-case", Identity: "right", Kind: "workflow-result", Configuration: "candidate", Actual: json.RawMessage(`{"status":200,"score":{"forbiddenAdditions":1,"relationshipErrors":2,"missing":["required"]}}`)}}
	r := lab.CompareCases(contract, candidate, "configuration", left, right)
	if !r.Compatibility.Compatible || r.Partial || len(r.Deltas) != 1 || r.Deltas[0].ForbiddenAdditionsDelta == nil || *r.Deltas[0].ForbiddenAdditionsDelta != 1 || *r.Deltas[0].MissingLabelsDelta != 1 {
		t.Fatalf("lost paired deltas: %+v", r)
	}
	r = lab.CompareCases(contract, candidate, "configuration", left, nil)
	if r.Compatibility.Compatible || !r.Partial || len(r.Compatibility.Reasons) == 0 {
		t.Fatal("partial pairs accepted")
	}
	right[0].Actual = json.RawMessage(`{"status":502,"score":{"forbiddenAdditions":0,"missing":[]}}`)
	r = lab.CompareCases(contract, candidate, "configuration", left, right)
	if !r.Partial || r.Deltas[0].ForbiddenAdditionsDelta != nil {
		t.Fatal("failed case acquired passing score delta")
	}
}

func TestComparisonRejectsChangedLabelsAndUnspecifiedConfounders(t *testing.T) {
	a := lab.ComparisonContract{FixtureHash: "fixtures", LabelHash: "labels", SourceVersion: "source", PromptVersion: "prompt", SchemaVersion: "schema", ScorerVersion: "scorer", StrategyVersion: "strategy", BudgetVersion: "budget", RuntimeVersion: "runtime", Mode: "controlled", Configuration: "current"}
	b := a
	b.Configuration = "candidate"
	r := lab.CheckComparison(a, b, "configuration")
	if !r.Compatible || len(r.Reasons) != 0 {
		t.Fatalf("paired comparison rejected: %+v", r)
	}
	b.LabelHash = "changed-labels"
	r = lab.CheckComparison(a, b, "configuration")
	if r.Compatible || len(r.Reasons) == 0 {
		t.Fatal("changed labels pooled")
	}
	b = a
	b.Configuration = "candidate"
	b.BudgetVersion = "larger"
	if lab.CheckComparison(a, b, "configuration").Compatible {
		t.Fatal("confounded budgets pooled")
	}
	b = a
	b.ScorerVersion = ""
	if lab.CheckComparison(a, b, "configuration").Compatible {
		t.Fatal("missing compatibility evidence pooled")
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	eval "professional-information-repo/internal/semanticeval"
	"testing"
)

func TestOfflineReviewPreservesOriginalEvidenceAndNeverInfersAcceptance(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "original.json")
	annotations := filepath.Join(dir, "reviews.json")
	output := filepath.Join(dir, "reviewed.json")
	rep := report{Version: "workflow-settings-v1", Mode: "live", Runs: []eval.ComparisonRun{{Configuration: "current", Run: eval.RunResult{ID: "synthetic", Status: 200, Output: json.RawMessage(`{"summary":[]}`)}}}}
	raw, _ := json.Marshal(rep)
	if err := os.WriteFile(original, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(annotations, []byte(`[{"index":0,"review":{"accepted":false,"method":"agent-assisted source review","reason":"Required summary is omitted."}}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := reassess(original, annotations, output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var reviewed report
	if err = json.Unmarshal(data, &reviewed); err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err = json.Compact(&compact, reviewed.Runs[0].Run.Output); err != nil {
		t.Fatal(err)
	}
	if reviewed.DerivedFrom != eval.Hash(raw) || compact.String() != `{"summary":[]}` || reviewed.Summary["current"].Accepted != 0 || reviewed.Summary["current"].PerAcceptedUSD != nil {
		t.Fatalf("offline evidence/acceptance corrupted: %+v", reviewed)
	}
	if err := reassess(original, annotations, output); err == nil {
		t.Fatal("overwrote an existing report")
	}
}

func TestMatrixBudgetCountsEveryContinuationPortion(t *testing.T) {
	cases := []eval.Case{{ID: "one"}, {ID: "continued", Steps: []eval.Case{{ID: "portion0"}, {ID: "portion1"}}}}
	if workflowCount(cases) != 3 {
		t.Fatal("continuation hidden from authorization budget")
	}
}

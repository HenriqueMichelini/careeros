package evaluationlab_test

import (
	"bytes"
	"os"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func TestSourceReviewRoundTripKeepsRegexMismatchPending(t *testing.T) {
	binding := lab.ReviewBinding{RunID: "synthetic-run", CaseID: "java-future", LabelVersion: "labels-v1", SourceBytes: []byte(`{"text":"I might use Java next year."}`), OutputBytes: []byte(`{"text":"I use Java professionally."}`), GoldBytes: []byte(`{"required":[]}`)}
	worksheet, err := lab.NewReviewWorksheet(binding)
	if err != nil {
		t.Fatal(err)
	}
	annotation := lab.ReviewAnnotation{ID: "java-claim", Reviewer: lab.ReviewActor{ID: "assistant", Type: "agent"}, SourceSpan: lab.ReviewSpan{Pointer: "/text", StartByte: 12, EndByte: 16, Text: "Java"}, OutputAtom: lab.ReviewSpan{Pointer: "/text", StartByte: 6, EndByte: 10, Text: "Java"}, Owner: "unspecified", Context: "future personal plan", Polarity: "affirmative", Certainty: "uncertain", Intent: "aspirational", Time: "next year", Qualifier: "might", Materiality: "required", ProposedVerdict: "unsupported-strength", Rationale: "Regex mismatch needs Henrique/domain review; uncertain future use cannot establish current professional use.", RawScorerVerdict: []byte(`{"regex":"unmatched"}`)}
	worksheet, err = lab.ProposeSourceReview(worksheet, binding, annotation)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := lab.ExportReviewWorksheet(worksheet)
	if err != nil {
		t.Fatal(err)
	}
	if directory := os.Getenv("LAB_REVIEW_EVIDENCE_DIR"); directory != "" {
		if err = os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(directory, "regex-mismatch.pending.v1.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	imported, err := lab.ImportReviewWorksheet(raw, binding)
	if err != nil {
		t.Fatal(err)
	}
	if imported.State != "pending" || len(imported.Annotations) != 1 || imported.Annotations[0].Reviewer.Type != "agent" || imported.Annotations[0].State != "pending" || imported.Binding.LabelVersion != "labels-v1" || string(imported.Binding.SourceBytes) != string(binding.SourceBytes) || imported.Annotations[0].SourceSpan.Text != "Java" {
		t.Fatal("agent proposal became approved or lost exact context")
	}
}

func TestPrivateWorksheetRoundTripDoesNotRewriteOriginalRun(t *testing.T) {
	directory := t.TempDir()
	store, err := lab.OpenFileStore(directory, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	record := controlledHistoryRecord(t, lab.RunnerOptions{Retain: store.Save})
	original, err := store.ExportJSON(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	worksheet, err := store.CreateReviewWorksheet(record.ID, record.Cases[0].Identity)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := store.ExportReviewWorksheet(record.ID, worksheet.ID)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := store.ImportReviewWorksheet(record.ID, record.Cases[0].Identity, exported)
	if err != nil {
		t.Fatal(err)
	}
	if imported.ID != worksheet.ID || imported.State != "pending" {
		t.Fatal("pending import changed authority")
	}
	after, err := store.ExportJSON(record.ID)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("worksheet rewrote original source/gold/output")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = lab.OpenFileStore(directory, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	loaded, err := store.LoadReviewWorksheet(record.ID, worksheet.ID)
	if err != nil || loaded.ContextHash != worksheet.ContextHash || loaded.State != "pending" {
		t.Fatal("private worksheet lost on restart")
	}
}

func TestWorksheetRejectsChangedEvidenceAndLabelVersion(t *testing.T) {
	binding := lab.ReviewBinding{RunID: "synthetic-run", CaseID: "case", LabelVersion: "labels-v1", SourceBytes: []byte(`{"text":"I use Java"}`), OutputBytes: []byte(`{"text":"Java"}`), GoldBytes: []byte(`{"required":["Java"]}`)}
	worksheet, err := lab.NewReviewWorksheet(binding)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := lab.ExportReviewWorksheet(worksheet)
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range []lab.ReviewBinding{
		{RunID: binding.RunID, CaseID: binding.CaseID, LabelVersion: "labels-v2", SourceBytes: binding.SourceBytes, OutputBytes: binding.OutputBytes, GoldBytes: binding.GoldBytes},
		{RunID: binding.RunID, CaseID: binding.CaseID, LabelVersion: binding.LabelVersion, SourceBytes: []byte(`{"text":"I might use Java"}`), OutputBytes: binding.OutputBytes, GoldBytes: binding.GoldBytes},
		{RunID: binding.RunID, CaseID: binding.CaseID, LabelVersion: binding.LabelVersion, SourceBytes: binding.SourceBytes, OutputBytes: []byte(`{"text":"Java expert"}`), GoldBytes: binding.GoldBytes},
	} {
		if _, err = lab.ImportReviewWorksheet(raw, changed); err == nil {
			t.Fatal("changed source/output/labels acquired original review")
		}
	}
	// An omission is a source fact with explicitly absent output, not invented text.
	annotation := lab.ReviewAnnotation{ID: "omitted-java", OutputAbsent: true, Reviewer: lab.ReviewActor{ID: "assistant", Type: "agent"}, SourceSpan: lab.ReviewSpan{Pointer: "/text", StartByte: 6, EndByte: 10, Text: "Java"}, Materiality: "required", ProposedVerdict: "omission", Rationale: "Required source fact absent in the selected output field."}
	worksheet, err = lab.ProposeSourceReview(worksheet, binding, annotation)
	if err != nil {
		t.Fatal(err)
	}
	if worksheet.State != "pending" {
		t.Fatal("omission proposal certified itself")
	}
}

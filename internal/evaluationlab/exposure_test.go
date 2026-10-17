package evaluationlab_test

import (
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func TestBlindedViewingExposesTranslationGroupAndSurvivesRestart(t *testing.T) {
	directory := t.TempDir()
	store, err := lab.OpenFileStore(directory, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	en := lab.CohortEntry{ContentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", HistoricalName: "new-blinded-en", Split: "newly-authored-blinded", ExposureGroup: "pair-example", Language: "en", Authorship: "controlled test example"}
	pt := en
	pt.ContentHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	pt.HistoricalName = "new-blinded-pt"
	pt.Language = "pt-BR"
	if err = store.RegisterCohort(en); err != nil {
		t.Fatal(err)
	}
	if err = store.RegisterCohort(pt); err != nil {
		t.Fatal(err)
	}
	before, err := store.CohortStatus(pt.ContentHash)
	if err != nil || !before.Untouched {
		t.Fatal("unviewed new cohort unavailable")
	}
	if err = store.RecordExposure(en.ContentHash, lab.ReviewActor{ID: "local-session", Type: "unknown"}, "view"); err != nil {
		t.Fatal(err)
	}
	status, err := store.CohortStatus(pt.ContentHash)
	if err != nil || status.Untouched || status.EffectiveSplit != "exposed-regression" || status.Entry.HistoricalName != "new-blinded-pt" || len(status.Events) != 1 {
		t.Fatal("translated partner retained untouched status")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = lab.OpenFileStore(directory, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	status, err = store.CohortStatus(en.ContentHash)
	if err != nil || status.Untouched {
		t.Fatal("exposure lost on restart")
	}
	changed := en
	changed.Split = "development"
	if err = store.RegisterCohort(changed); err == nil {
		t.Fatal("historical cohort rewritten")
	}
	if err = store.RecordExposure(pt.ContentHash, lab.ReviewActor{ID: "assistant", Type: "agent"}, "prompt-development"); err != nil {
		t.Fatal(err)
	}
	status, err = store.CohortStatus(en.ContentHash)
	if err != nil || len(status.Events) != 2 {
		t.Fatal("development use missing from paired ledger")
	}
}

package evaluationlab_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	"strings"
	"sync/atomic"
	"testing"
)

func TestStorePersistsPrivateRunAndRecoversWithoutRedispatch(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	store, err := lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p := semanticPlanner(t)
	runner := lab.NewRunner(p, lab.RunnerOptions{Retain: store.Save, BeforeDispatch: store.Health})
	id, err := runner.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	original := awaitRun(t, runner, id)
	saved, err := store.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Durable || saved.EngineState != original.EngineState || len(saved.Dispatches) != len(original.Dispatches) {
		t.Fatal("durable evidence differs")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	page, err := store.List(lab.RunFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Runs) != 1 || page.Runs[0].ID != id {
		t.Fatal("index rebuild lost history")
	}
	caseDetail, err := store.Case(id, saved.Cases[0].Identity)
	if err != nil || caseDetail.ID != saved.Cases[0].ID {
		t.Fatalf("lazy case: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("directory is not private")
	}
	filepath.WalkDir(dir, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			t.Fatal(e)
		}
		info, e := d.Info()
		if e != nil {
			t.Fatal(e)
		}
		if !d.IsDir() && info.Mode().Perm() != 0600 {
			t.Fatalf("file is not private: %s", path)
		}
		return nil
	})
}

func TestStoreRestartPreservesPartialRunAndNeverResumes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	s, err := lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p := semanticPlanner(t)
	r := lab.NewRunner(p, lab.RunnerOptions{})
	id, err := r.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, r, id)
	record.EngineState = "running"
	record.Dispatches[0].State = "attempted"
	if err = s.Save(record); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	saved, err := s.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.EngineState != "interrupted" || saved.Dispatches[0].State != "unknown-final" || saved.Cases[0].EngineState != record.Cases[0].EngineState {
		t.Fatal("restart lost partial state")
	}
	if len(saved.Dispatches) != len(record.Dispatches) {
		t.Fatal("restart redispatched")
	}
}
func TestStoreFailureStopsRunnerAndKeepsCommittedEvidence(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	var writes atomic.Int64
	s, err := lab.OpenFileStore(dir, lab.StoreOptions{BeforeWrite: func(operation string) error {
		if operation == "rename:manifest.json" && writes.Add(1) == 3 {
			return errors.New("injected disk full")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	p := semanticPlanner(t)
	r := lab.NewRunner(p, lab.RunnerOptions{Retain: s.Save, BeforeDispatch: s.Health})
	id, err := r.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, r, id)
	if record.EngineState != "failed" || len(record.Dispatches) > 1 || s.Health(context.Background()) == nil {
		t.Fatal("failure did not stop dispatch")
	}
	saved, err := s.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.EngineState == "completed" {
		t.Fatal("claimed completion before manifest commit")
	}
	s.Close()
	s, err = lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	saved, err = s.Load(id)
	if err != nil || saved.EngineState != "interrupted" {
		t.Fatalf("lost previous snapshot: %v", err)
	}
}

func TestStoreDetectsTruncatedEventsWithoutLosingOtherHistory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	s, err := lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p := semanticPlanner(t)
	r := lab.NewRunner(p, lab.RunnerOptions{Retain: s.Save})
	id, err := r.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, r, id)
	events, err := s.Events(id, 0, 100)
	if err != nil || len(events) == 0 {
		t.Fatalf("missing durable events: %v", err)
	}
	s.Close()
	f, err := os.OpenFile(filepath.Join(dir, id, "events.jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"sequence":`)
	f.Close()
	s, err = lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	page, err := s.List(lab.RunFilter{Limit: 10})
	if err != nil || len(page.Problems) == 0 || len(page.Runs) != 1 {
		t.Fatalf("damage lost independent manifest: %+v %v", page, err)
	}
	if _, err = s.Events(id, 0, 100); err == nil {
		t.Fatal("truncated event log was accepted")
	}
	if _, err = s.Load(id); err != nil {
		t.Fatal("valid snapshot discarded")
	}
}

func TestStoreImportExportKeepsOriginalAndQuarantine(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	s, err := lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/controlled.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Import(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := s.ExportOriginal(id)
	if err != nil || string(exported) != string(raw) {
		t.Fatal("original export changed bytes")
	}
	imported, err := s.LoadImport(id)
	if err != nil || imported.Quarantined || len(imported.Cases) == 0 {
		t.Fatalf("trusted import lost: %v", err)
	}
	raw[0] = '!'
	again, err := s.ExportOriginal(id)
	if err != nil || string(again) != string(exported) {
		t.Fatal("caller mutated original")
	}
	other, err := s.Import([]byte(`{"private":"unknown"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	quarantined, err := s.LoadImport(other)
	if err != nil || !quarantined.Quarantined {
		t.Fatal("unknown promoted")
	}
	if _, err = s.ExportOriginal(other); err == nil {
		t.Fatal("unknown artifact exported")
	}
	if _, err = s.ExportMarkdown(other); err == nil {
		t.Fatal("unknown artifact rendered")
	}
	if err = s.Save(lab.RunRecord{ID: id}); err == nil {
		t.Fatal("import overwritten")
	}
	s.Close()
	s, err = lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	restored, err := s.LoadImport(id)
	if err != nil || restored.OriginalHash != imported.OriginalHash {
		t.Fatal("reopen lost immutable import")
	}
}

func TestStoreManagementBoundsAndDeliberateDeletion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	s, err := lab.OpenFileStore(dir, lab.StoreOptions{SoftWarningBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if other, err := lab.OpenFileStore(dir, lab.StoreOptions{}); err == nil {
		other.Close()
		t.Fatal("second writer admitted")
	}
	ids := []string{}
	for i := 0; i < 3; i++ {
		id, err := s.Import([]byte(`{"unknown":true}`), nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	page, err := s.List(lab.RunFilter{Limit: 2})
	if err != nil || len(page.Runs) != 2 || page.Next != 2 {
		t.Fatal("pagination incorrect")
	}
	second, err := s.List(lab.RunFilter{Offset: page.Next, Limit: 2})
	if err != nil || len(second.Runs) != 1 {
		t.Fatal("next page incorrect")
	}
	usage, err := s.Usage()
	if err != nil || usage.Bytes <= 0 || !usage.SoftWarning {
		t.Fatal("missing disk warning")
	}
	if _, err = s.Load("../escape"); err == nil {
		t.Fatal("traversal accepted")
	}
	if err = s.Delete("../escape"); err == nil {
		t.Fatal("traversal delete accepted")
	}
	if err = s.Delete(ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = s.LoadImport(ids[0]); err == nil {
		t.Fatal("deleted history remains")
	}
	page, err = s.List(lab.RunFilter{Limit: 10})
	if err != nil || len(page.Runs) != 2 {
		t.Fatal("deletion touched unrelated history")
	}
}
func TestStoreRejectsSymlinkAndCorruptVersionWithoutHidingValidHistory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	s, err := lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Import([]byte(`{"unknown":true}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := s.Import([]byte(`{"other":true}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	manifest := filepath.Join(dir, id, "manifest.json")
	if err = os.WriteFile(manifest, []byte(`{"version":"future"}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err = lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	page, err := s.List(lab.RunFilter{Limit: 10})
	if err != nil || len(page.Runs) != 1 || page.Runs[0].ID != valid || len(page.Problems) == 0 {
		t.Fatal("corruption hid valid history")
	}
	outside := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(outside, []byte("secret"), 0600)
	// Lookup uses the manifest's immutable snapshot, with every path checked.
	entries, _ := os.ReadDir(filepath.Join(dir, valid))
	for _, entry := range entries {
		if entry.IsDir() {
			target := filepath.Join(dir, valid, entry.Name(), "original.json")
			os.Remove(target)
			os.Symlink(outside, target)
		}
	}
	if _, err = s.LoadImport(valid); err == nil {
		t.Fatal("symlink artifact accepted")
	}
}

func TestStoreVersionedSyntheticBundleRoundTrip(t *testing.T) {
	s, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/controlled.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Import(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := s.ExportJSON(id)
	if err != nil {
		t.Fatal(err)
	}
	copyID, err := s.ImportExport(bundle)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := s.LoadImport(id)
	copy, _ := s.LoadImport(copyID)
	if copy.OriginalHash != original.OriginalHash || len(copy.Cases) != len(original.Cases) || copy.Derivation != original.Derivation {
		t.Fatal("bundle changed evidence")
	}
	detail, err := s.Case(copyID, copy.Cases[0].Identity)
	if err != nil || detail.Identity != copy.Cases[0].Identity {
		t.Fatalf("import lazy detail: %v", err)
	}
}

func TestDamagedJournalDoesNotPreventReadingOtherRunsOnRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	s, err := lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p := semanticPlanner(t)
	r := lab.NewRunner(p, lab.RunnerOptions{})
	id, err := r.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, r, id)
	record.EngineState = "running"
	if err = s.Save(record); err != nil {
		t.Fatal(err)
	}
	other, err := s.Import([]byte(`{"safeHistory":true}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	f, err := os.OpenFile(filepath.Join(dir, id, "events.jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("truncated")
	f.Close()
	s, err = lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatalf("one damaged journal blocked all history: %v", err)
	}
	defer s.Close()
	if _, err = s.LoadImport(other); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.Load(id)
	if err != nil || recovered.EngineState != "interrupted" || recovered.Durable {
		t.Fatalf("unsafe recovered state: %+v %v", recovered, err)
	}
	if s.Health(context.Background()) == nil {
		t.Fatal("recovery write failure permitted new dispatch")
	}
}

func TestNativeSyntheticExportPreservesEvidenceWithoutTrustPromotion(t *testing.T) {
	s, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := semanticPlanner(t)
	r := lab.NewRunner(p, lab.RunnerOptions{Retain: s.Save})
	id, err := r.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, r, id)
	encoded, err := s.ExportJSON(id)
	if err != nil {
		t.Fatal(err)
	}
	copyID, err := s.ImportExport(encoded)
	if err != nil {
		t.Fatal(err)
	}
	copy, err := s.LoadImport(copyID)
	if err != nil || !copy.Quarantined || string(copy.Original) != string(encoded) {
		t.Fatal("native export altered or promoted")
	}
	markdown, err := s.ExportMarkdown(id)
	if err != nil || len(markdown) == 0 {
		t.Fatal("native worksheet unavailable")
	}
	artifact, err := s.Artifact(id, record.Cases[0].Identity)
	if err != nil || string(artifact) != string(record.Cases[0].Actual) {
		t.Fatalf("artifact differs: %v", err)
	}
	diagnostics, err := s.ExportDiagnostics(id)
	if err != nil || bytes.Contains(diagnostics, []byte("actual")) || bytes.Contains(diagnostics, record.Cases[0].Source) {
		t.Fatal("diagnostics include content")
	}
}

func TestStoreRebuildUsesMetadataAndKeepsIndependentCaseArtifacts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	s, err := lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p := semanticPlanner(t)
	r := lab.NewRunner(p, lab.RunnerOptions{Retain: s.Save})
	id, err := r.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, r, id)
	s.Close()
	entries, _ := os.ReadDir(filepath.Join(dir, id))
	for _, entry := range entries {
		if entry.IsDir() {
			os.Remove(filepath.Join(dir, id, entry.Name(), "result.json"))
		}
	}
	s, err = lab.OpenFileStore(dir, lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	page, err := s.List(lab.RunFilter{Limit: 10})
	if err != nil || len(page.Runs) != 1 {
		t.Fatal("index eagerly discarded independent evidence")
	}
	if _, err = s.Load(id); err == nil {
		t.Fatal("missing aggregate result accepted")
	}
	if _, err = s.Case(id, record.Cases[0].Identity); err != nil {
		t.Fatalf("lazy case lost: %v", err)
	}
}

func TestNativeExportRejectsBundleTooLargeToRoundTrip(t *testing.T) {
	s, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := semanticPlanner(t)
	r := lab.NewRunner(p, lab.RunnerOptions{})
	id, err := r.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, r, id)
	record.UnavailableReasons = append(record.UnavailableReasons, strings.Repeat("x", lab.MaxImportBytes))
	if err = s.Save(record); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExportJSON(id); err == nil {
		t.Fatal("successful export cannot be reimported")
	}
}

func TestCaseEnvelopePreservesArtifactAtConfiguredBound(t *testing.T) {
	s, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := semanticPlanner(t)
	r := lab.NewRunner(p, lab.RunnerOptions{})
	id, err := r.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, r, id)
	record.Cases[0].Actual = json.RawMessage(`"` + strings.Repeat("x", lab.MaxImportBytes-2) + `"`)
	record.Cases[0].ArtifactHash = ""
	if err = s.Save(record); err != nil {
		t.Fatal(err)
	}
	detail, err := s.Case(id, record.Cases[0].Identity)
	if err != nil || len(detail.Actual) != lab.MaxImportBytes {
		t.Fatalf("acknowledged unreadable case: %v", err)
	}
	artifact, err := s.Artifact(id, record.Cases[0].Identity)
	if err != nil || len(artifact) != lab.MaxImportBytes {
		t.Fatalf("bounded artifact unavailable: %v", err)
	}
}

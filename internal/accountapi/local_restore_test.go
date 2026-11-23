package accountapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/testsupport/aiadmission"
)

const (
	restoreQualification = "qualification"
	restoreDraft         = "managed-draft"
	keptDraft            = "restore-kept-draft"
	canceledDraft        = "restore-closed-draft"
	oldUnknown           = "restore-old-unknown"
	newUnknown           = "restore-later-unknown"
)

type restoreIngress struct {
	upstream http.Handler
	closed   atomic.Bool
}

func newRestoreIngress(h http.Handler) *restoreIngress {
	gate := &restoreIngress{upstream: h}
	gate.closed.Store(true)
	return gate
}

func (g *restoreIngress) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.closed.Load() {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"restore_quarantine"}`))
		return
	}
	g.upstream.ServeHTTP(w, r)
}

type restoreRuntime struct {
	account       *accountstore.Store
	service       *auth.Service
	qualification *aicontrol.Harness
	draft         *aicontrol.Harness
	purpose       *draftrecovery.Store
	http          *restoreIngress
}

func restoreManagedDraft(t *testing.T, dsn string, gate *aicontrol.SyntheticGate) (*aicontrol.Harness, *draftrecovery.Store) {
	t.Helper()
	ctx := context.Background()
	cfg := aicontrol.ManagedDraftSyntheticConfig(restoreDraft, gate, managedDraftFullBound)
	cfg.ExpiresAt = time.Now().Add(time.Hour)
	h, err := aicontrol.NewSynthetic(ctx, dsn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	drafts, err := draftrecovery.OpenManagedTerminal(ctx, dsn, draftrecovery.Config{
		Actions: h, BindingKey: []byte("synthetic-managed-draft-binding-key"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { drafts.Close() })
	return h, drafts
}

func restoreTargetRuntime(t *testing.T, dsn string, calls *atomic.Int32) restoreRuntime {
	t.Helper()
	ctx := context.Background()
	store, err := accountstore.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	service := aiadmission.NewSyntheticService(t, store)
	gate := aicontrol.NewSyntheticGate()
	for _, ns := range []string{restoreQualification, restoreDraft} {
		// Register the older generation, but do not serve from it.
		gate.Reconcile(ns, 1)
		gate.Close(ns)
	}
	qualCfg := aicontrol.QualificationSyntheticConfig(restoreQualification, gate, managedDraftFullBound)
	qualCfg.ExpiresAt = time.Now().Add(time.Hour)
	qual, err := aicontrol.NewSynthetic(ctx, dsn, qualCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { qual.Close() })
	draft, purpose := restoreManagedDraft(t, dsn, gate)
	for _, h := range []*aicontrol.Harness{qual, draft} {
		if err := h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
			calls.Add(1)
			return nil, false, errors.New("unexpected synthetic callback after restore")
		}); err != nil {
			t.Fatal(err)
		}
	}
	return restoreRuntime{store, service, qual, draft, purpose, newRestoreIngress(accountapi.NewSyntheticTerminalDraft(service, store, draft, purpose))}
}

func restoreJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func restoreCode(t *testing.T, label string, w *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if w.Code != expected {
		t.Fatalf("%s expected HTTP %d, got %d", label, expected, w.Code)
	}
	t.Logf("restore observation %s HTTP %d", label, w.Code)
}

func restoreDenied(t *testing.T, label string, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code < 400 {
		t.Fatalf("%s unexpectedly succeeded with HTTP %d", label, w.Code)
	}
	t.Logf("restore observation %s denied with HTTP %d", label, w.Code)
}

func restoreAccount(f *aiadmission.Fixture, login auth.LoginResult) *aiadmission.Fixture {
	copy := *f
	copy.Login, copy.Principal = login, login.Principal
	return &copy
}

func restoreGrant(t *testing.T, f *aiadmission.Fixture, subject string) *aiadmission.Fixture {
	t.Helper()
	ctx := context.Background()
	who := restoreAccount(f, controlFreshIdentity(t, f.Service, subject))
	identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: subject}
	invitation, err := f.Store.PilotCommand(ctx, f.Principal, subject+"-invite", accountstore.PilotCommand{Kind: "invite", Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.PilotCommand(ctx, f.Principal, subject+"-clear", accountstore.PilotCommand{Kind: "clear", Identity: identity}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.ActivatePilot(ctx, who.Principal, subject+"-activate", invitation.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.SaveProfile(ctx, who.Principal, subject+"-profile", 0, f.Document); err != nil {
		t.Fatal(err)
	}
	return who
}

func restoreDraftFixture(t *testing.T, f *aiadmission.Fixture, h http.Handler) (draftrecovery.Input, draftrecovery.Result, []byte, draftrecovery.OpeningConfirmation) {
	t.Helper()
	var doc profiledocument.Document
	if err := json.Unmarshal(f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	input := draftrecovery.Input{Profile: draftrecovery.Source{ID: doc.ID, Revision: doc.Revision},
		Job: draftrecovery.Job{Posting: "Java required."}, Language: "en"}
	var ready draftrecovery.Result
	var confirmation draftrecovery.OpeningConfirmation
	for _, key := range []string{keptDraft, canceledDraft} {
		w := qualificationRequest(t, f, h, "/api/drafts", key, "", restoreJSON(t, input))
		restoreCode(t, key+" ready before backup", w, http.StatusCreated)
		if err := json.Unmarshal(w.Body.Bytes(), &ready); err != nil || ready.Bundle == nil || ready.Bundle.Resume == "" || ready.Bundle.CoverLetter == "" {
			t.Fatal("fixture did not produce a complete Draft", err)
		}
		if key != keptDraft {
			break
		}
		receiptResult := openingRequest(t, f, h, "POST", "/api/drafts/"+key+"/versions/1/opening", struct{}{})
		restoreCode(t, "complete opening receipt", receiptResult, http.StatusOK)
		var receipt draftrecovery.OpeningReceipt
		if err := json.Unmarshal(receiptResult.Body.Bytes(), &receipt); err != nil {
			t.Fatal(err)
		}
		confirmation = draftrecovery.OpeningConfirmation{Receipt: receipt.Receipt, BundleDigest: receipt.BundleDigest, Manifest: receipt.Manifest}
		delivered := openingRequest(t, f, h, "POST", "/api/drafts/"+key+"/versions/1/opening/confirm", confirmation)
		restoreCode(t, "one delivered right", delivered, http.StatusOK)
		var result draftrecovery.Result
		if err := json.Unmarshal(delivered.Body.Bytes(), &result); err != nil || !result.Delivered {
			t.Fatal("fixture did not persist a completed delivery", err)
		}
		review := openingRequest(t, f, h, "POST", "/api/drafts/"+key+"/versions/1/review", humanCommand(result, "restore-reviewed-resume", "resume", "review", 0))
		restoreCode(t, "original Draft review", review, http.StatusOK)
	}
	kept := openingRequest(t, f, h, "GET", "/api/drafts/"+keptDraft+"/versions/1", nil)
	restoreCode(t, "acknowledged reviewed Draft before backup", kept, http.StatusOK)
	return input, ready, bytes.Clone(kept.Body.Bytes()), confirmation
}

func seedRestoreRemovedExcerpt(t *testing.T, f *aiadmission.Fixture) {
	t.Helper()
	var document map[string]json.RawMessage
	var evidence []json.RawMessage
	if err := json.Unmarshal(f.Document, &document); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(document["evidence"], &evidence); err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-cleanup record in the private fixture. The actual
	// lifecycle worker owns the subsequent removal.
	evidence = append(evidence, json.RawMessage(`{"id":"restore-legacy-excerpt","revision":1,"excerpt":"SYNTHETIC DETAIL TO REMOVE","origin":"synthetic","approval":"approved"}`))
	document["evidence"] = restoreJSON(t, evidence)
	if _, err := f.DB.Exec("UPDATE careeros_profiles SET document=$2 WHERE account_id=$1", f.Principal.AccountID, restoreJSON(t, document)); err != nil {
		t.Fatal(err)
	}
}

func TestLocalOlderPilotRestoreRehearsal(t *testing.T) {
	if os.Getenv("CAREEROS_LOCAL_RESTORE_REHEARSAL") != "1" {
		t.Skip("run the explicit disposable older-backup rehearsal")
	}
	tools := os.Getenv("CAREEROS_LOCAL_RESTORE_PG_BIN")
	if !filepath.IsAbs(tools) {
		t.Fatal("set the absolute installed PostgreSQL bin directory")
	}
	// The PG client defaults must not supplement any fixture connection.
	for _, variable := range os.Environ() {
		key, _, _ := bytes.Cut([]byte(variable), []byte("="))
		if len(key) > 2 && string(key[:2]) == "PG" {
			t.Setenv(string(key), "")
			if err := os.Unsetenv(string(key)); err != nil {
				t.Fatal(err)
			}
		}
	}
	sourcePG, targetPG := newRestorePostgres(t, tools), newRestorePostgres(t, tools)
	t.Setenv("CAREEROS_TEST_DATABASE_URL", sourcePG.dsn)
	ctx := context.Background()
	sourceGate := aicontrol.NewSyntheticGate()
	for _, ns := range []string{restoreQualification, restoreDraft} {
		sourceGate.Reconcile(ns, 1)
	}
	cfg := aicontrol.QualificationSyntheticConfig(restoreQualification, sourceGate, managedDraftFullBound)
	f, qual, _, qualHTTP := qualificationConfigFixture(t, cfg)
	draft, purpose := restoreManagedDraft(t, f.DSN, sourceGate)
	if err := purpose.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	provider := &aiadmission.Provider{T: t, Workflow: "managed_application_draft"}
	var beforeCalls, afterCalls, targetCalls atomic.Int32
	if err := draft.SetQualificationResponder(func(ctx context.Context, providerID string, req *http.Request) (*http.Response, bool, error) {
		response, err := provider.Respond(ctx, providerID, req)
		return response, true, err
	}); err != nil {
		t.Fatal(err)
	}
	unknownResponder := func(count *atomic.Int32) func(context.Context, string, *http.Request) (*http.Response, bool, error) {
		return func(context.Context, string, *http.Request) (*http.Response, bool, error) {
			count.Add(1)
			return nil, false, errors.New("synthetic response unavailable")
		}
	}
	if err := qual.SetQualificationResponder(unknownResponder(&beforeCalls)); err != nil {
		t.Fatal(err)
	}
	draftHTTP := accountapi.NewSyntheticTerminalDraft(f.Service, f.Store, draft, purpose)
	oldDraft, oldQual := newRestoreIngress(draftHTTP), newRestoreIngress(qualHTTP)
	oldDraft.closed.Store(false)
	oldQual.closed.Store(false)
	retired := restoreGrant(t, f, "restore-retired-account")
	revoked := restoreGrant(t, f, "restore-revoked-grant")
	pruned := restoreGrant(t, f, "restore-cleaned-detail")
	seedRestoreRemovedExcerpt(t, pruned)
	input, canceled, kept, confirmation := restoreDraftFixture(t, f, oldDraft)
	oldQualificationBody := f.Body(t, "/api/qualification-gaps")
	if w := qualificationRequest(t, f, oldQual, "/api/qualification-gaps", oldUnknown, "", oldQualificationBody); w.Code < 400 || beforeCalls.Load() != 1 {
		t.Fatal("backup must already include one unknown dispatch")
	}
	priorUnknown := restoreUnknownDispatch(t, f.DB, restoreQualification, oldUnknown)
	oldResult, err := qual.Open(ctx, f.Principal, oldUnknown, "qualification_gaps", oldQualificationBody)
	if err != nil || oldResult.ResultAllowed(ctx) != nil {
		t.Fatal("fixture could not capture the old protected result capability", err)
	}
	older, err := url.Parse(f.DSN)
	if err != nil {
		t.Fatal(err)
	}
	schema := older.Query().Get("search_path")
	artifacts := t.TempDir()
	backup := filepath.Join(artifacts, "older.dump")
	backupDigest := sourcePG.backup(t, schema, backup)
	backupFinished := time.Now()

	restoreCode(t, "later cancellation", openingRequest(t, f, oldDraft, "POST",
		"/api/drafts/"+canceledDraft+"/cancel", map[string]any{"action": canceled.Action}), http.StatusOK)
	if err := draft.SetQualificationResponder(unknownResponder(&afterCalls)); err != nil {
		t.Fatal(err)
	}
	if w := qualificationRequest(t, f, oldDraft, "/api/drafts", newUnknown, "", restoreJSON(t, input)); w.Code < 400 || afterCalls.Load() != 1 {
		t.Fatal("current authority must contain a second unknown dispatch missing from backup")
	}
	postBackupUnknown := restoreUnknownDispatch(t, f.DB, restoreDraft, newUnknown)
	if _, err := f.Store.PilotCommand(ctx, f.Principal, "restore-revoke", accountstore.PilotCommand{Kind: "revoke",
		Identity: auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "restore-revoked-grant"}}); err != nil {
		t.Fatal(err)
	}
	originalRevocation, err := f.Store.PilotStatus(ctx, revoked.Principal)
	if err != nil || originalRevocation.ExpiresAt == nil || originalRevocation.RetrieveUntil == nil {
		t.Fatal("source revocation must have explicit original access clocks", err)
	}
	deletion, err := f.Store.StartDeletion(ctx, retired.Principal, accountstore.DeletionRequest{OperationID: "restore-delete-account", Kind: "account", Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Store.PurgeDeletion(ctx, retired.Principal.AccountID, deletion.OperationID); err != nil {
		t.Fatal(err)
	}
	worker, err := accountstore.NewLifecycleWorker(f.Store, nil, func(_ context.Context, o accountstore.LifecycleResidualObligation) (accountstore.LifecycleResidualEvidence, error) {
		return accountstore.LifecycleResidualEvidence{ID: "restore-pending-" + o.Class, Revision: 1, State: "pending"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worker.Run(ctx, 10); err != nil {
		t.Fatal(err)
	}
	originalPruned := openingRequest(t, pruned, oldDraft, "GET", "/api/profile/current", nil)
	restoreCode(t, "source repaired excerpt", originalPruned, 200)
	if bytes.Contains(originalPruned.Body.Bytes(), []byte("SYNTHETIC DETAIL TO REMOVE")) {
		t.Fatal("source cleanup failed to remove the legacy detail")
	}
	started := time.Now()
	oldDraft.closed.Store(true)
	oldQual.closed.Store(true)
	controls := f.Store.LocalControls(nil)
	inviteStop, err := controls.Apply(ctx, f.ControlOperator.Principal, "restore-stop-invitations",
		accountstore.ControlCommand{Control: "invitations", Stop: true, ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = controls.Apply(ctx, f.ControlOperator.Principal, "restore-stop-inference",
		accountstore.ControlCommand{Control: "inference", Stop: true, ExpectedVersion: inviteStop.Version}); err != nil {
		t.Fatal(err)
	}
	for _, ns := range []string{restoreQualification, restoreDraft} {
		sourceGate.Close(ns)
		if err := sourceGate.Drain(ctx, ns); err != nil {
			t.Fatal(err)
		}
	}
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatal(err)
	}
	sourceRecovery, err := accountstore.NewDeletionRecovery(f.Store, key[:])
	if err != nil {
		t.Fatal(err)
	}
	decisions := captureRestoreDecisions(t, f.DB, sourceRecovery, backupDigest)
	if len(decisions.Pruning.Fences) != 1 || decisions.Pruning.Fences[0].EvidenceID != "restore-legacy-excerpt" {
		t.Fatal("source did not record the independent detail-removal fence")
	}
	decisionRaw := restoreJSON(t, decisions)
	if bytes.Contains(decisionRaw, []byte("SYNTHETIC DETAIL TO REMOVE")) || bytes.Contains(decisionRaw, []byte("I use Java.")) {
		t.Fatal("decision receipt retained private fixture content")
	}
	decisionPath := filepath.Join(artifacts, "decisions.json")
	restoreWrite(t, decisionPath, decisionRaw)
	decisionDigest, err := restoreFileDigest(decisionPath)
	if err != nil {
		t.Fatal(err)
	}
	restoreCode(t, "old reads during quarantine", openingRequest(t, f, oldDraft, "GET", "/api/profile/current", nil), 503)
	restoreCode(t, "old publication during quarantine", openingRequest(t, f, oldDraft, "POST", "/api/drafts/"+canceledDraft+"/versions/1/opening", struct{}{}), 503)
	restoreCode(t, "old send during quarantine", qualificationRequest(t, f, oldQual, "/api/qualification-gaps", "restore-no-new-send", "", oldQualificationBody), 503)
	corrupt := filepath.Join(artifacts, "bad.dump")
	restoreWrite(t, corrupt, []byte("invalid local PostgreSQL archive"))
	corruptDigest, err := restoreFileDigest(corrupt)
	if err != nil {
		t.Fatal(err)
	}
	if err := targetPG.restore(corrupt, corruptDigest); err == nil {
		t.Fatal("failed restore was accepted")
	}
	targetRaw := restoreSQL(t, targetPG.dsn)
	var found bool
	if err := targetRaw.QueryRow("SELECT EXISTS(SELECT FROM pg_namespace WHERE nspname=$1)", schema).Scan(&found); err != nil || found {
		t.Fatal("failed restore published the old schema", err)
	}
	t.Log("restore observation invalid archive: actual pg_restore failed; no schema installed")
	restoreFixtureRoles(t, f.DB, targetRaw)
	if err := targetPG.restore(backup, backupDigest); err != nil {
		t.Fatal(err)
	}
	targetURL, err := url.Parse(targetPG.dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := targetURL.Query()
	query.Set("search_path", schema)
	targetURL.RawQuery = query.Encode()
	target := restoreSQL(t, targetURL.String())
	checkRestoreOlderImage(t, target, retired.Principal.AccountID, revoked.Principal.AccountID, pruned.Principal.AccountID)
	replacement := restoreTargetRuntime(t, targetURL.String(), &targetCalls)
	restoreCode(t, "restored backup has no public read", openingRequest(t, f, replacement.http, "GET", "/api/drafts/"+keptDraft, nil), 503)
	startGeneration := restoreGeneration(t, target)
	if _, err := target.Exec("ALTER TABLE careeros_control_state ADD COLUMN incompatible_fixture_epoch bigint"); err != nil {
		t.Fatal(err)
	}
	if _, err := readRestoreDecisions(decisionPath, decisionDigest, backupDigest, restoreCatalog(t, target)); err == nil {
		t.Fatal("incompatible rollback accepted")
	}
	checkRestoreQuarantine(t, f, replacement.http, target, startGeneration, "incompatible rollback")
	if _, err := target.Exec("ALTER TABLE careeros_control_state DROP COLUMN incompatible_fixture_epoch"); err != nil {
		t.Fatal(err)
	}
	missing := decisions
	missing.Tables = make(map[string]json.RawMessage, len(decisions.Tables))
	for table, rows := range decisions.Tables {
		missing.Tables[table] = rows
	}
	delete(missing.Tables, "ai_fixture_dispatch")
	incompletePath := filepath.Join(artifacts, "incomplete.json")
	restoreWrite(t, incompletePath, restoreJSON(t, missing))
	incompleteDigest, err := restoreFileDigest(incompletePath)
	if err != nil {
		t.Fatal(err)
	}
	catalog := restoreCatalog(t, target)
	if _, err := readRestoreDecisions(incompletePath, incompleteDigest, backupDigest, catalog); err == nil {
		t.Fatal("incomplete authoritative reconciliation accepted")
	}
	checkRestoreQuarantine(t, f, replacement.http, target, startGeneration, "incomplete reconciliation")
	pinned, err := readRestoreDecisions(decisionPath, decisionDigest, backupDigest, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyRestoreLedger(ctx, target, pinned); err != nil {
		t.Fatal(err)
	}
	targetRecovery, err := accountstore.NewDeletionRecovery(replacement.account, key[:])
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := targetRecovery.Apply(ctx, pinned.Deletion); err != nil {
			t.Fatal(err)
		}
		if err := targetRecovery.ApplyProfilePruning(ctx, pinned.Pruning); err != nil {
			t.Fatal(err)
		}
		if err := targetRecovery.ApplyLifecycle(ctx, pinned.Lifecycle); err != nil {
			t.Fatal(err)
		}
	}
	restoreCode(t, "pruning fence blocks direct stale detail access",
		openingRequest(t, pruned, replacement.http.upstream, "GET", "/api/profile/current", nil), 403)
	if err := applyRestorePrunedProfile(ctx, target, pinned.Pruning.Fences[0]); err != nil {
		t.Fatal(err)
	}
	targetWorker, err := accountstore.NewLifecycleWorker(replacement.account, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := targetWorker.Run(ctx, 10); err != nil {
		t.Fatal(err)
	}
	pruning, err := targetRecovery.ExportProfilePruning(ctx)
	if err != nil || len(pruning.Fences) != 1 || !pruning.Fences[0].RemovedAt.Equal(pinned.Pruning.Fences[0].RemovedAt) ||
		!pruning.Fences[0].RecoveryCopyDeadline.Equal(pinned.Pruning.Fences[0].RecoveryCopyDeadline) {
		t.Fatal("detail cleanup replay reset original source anchors", err)
	}
	if err := replacement.purpose.Cleanup(ctx, f.Principal, canceledDraft); err != nil {
		t.Fatal(err)
	}
	if err := restoreExclusion(sourcePG, f.DB); err == nil {
		t.Fatal("old database was treated as fenced before physical shutdown")
	}
	checkRestoreQuarantine(t, f, replacement.http, target, startGeneration, "failed old-system fence")
	if err := sourcePG.stop(); err != nil {
		t.Fatal(err)
	}
	if err := restoreExclusion(sourcePG, f.DB); err != nil {
		t.Fatal(err)
	}
	restoreDenied(t, "direct old Profile read after physical fence",
		openingRequest(t, f, draftHTTP, "GET", "/api/profile/current", nil))
	var nextProfile profiledocument.Document
	if err := json.Unmarshal(f.Document, &nextProfile); err != nil {
		t.Fatal(err)
	}
	nextProfile.Revision++
	restoreDenied(t, "direct old Profile write after physical fence", openingRequest(t, f, draftHTTP, "PUT", "/api/profile/current", map[string]any{
		"operationId": "old-write-probe", "expectedRevision": 1, "document": nextProfile,
	}))
	restoreDenied(t, "direct old Draft opening after physical fence",
		openingRequest(t, f, draftHTTP, "POST", "/api/drafts/"+canceledDraft+"/versions/1/opening", struct{}{}))
	restoreDenied(t, "direct old provider-capable route after physical fence",
		qualificationRequest(t, f, qualHTTP, "/api/qualification-gaps", "fenced-old-send", "", oldQualificationBody))
	if beforeCalls.Load() != 1 {
		t.Fatal("direct old router invoked the fake transport after exclusion")
	}
	if _, finish, err := oldResult.BeginResult(ctx); err == nil {
		finish()
		t.Fatal("old cached result capability survived actual fencing")
	}
	t.Log("restore observation old owned server stopped; direct old read/write/publication/send denied")
	if _, err := target.Exec("DELETE FROM careeros_sessions; DELETE FROM careeros_deletion_status_sessions; DELETE FROM careeros_oidc_transactions"); err != nil {
		t.Fatal(err)
	}
	reopenedOperator := controlFreshIdentity(t, replacement.service, "fixture-control-operator")
	current := restoreGeneration(t, target)
	if current != startGeneration {
		t.Fatal("reading or reconciling decisions advanced the generation")
	}
	restoreAdvanceGeneration(t, replacement, reopenedOperator.Principal, startGeneration, sourcePG, f.DB)
	fresh := restoreAccount(f, controlFreshIdentity(t, replacement.service, "fixture-account"))
	restoreDenied(t, "old session on the recovered system",
		openingRequest(t, f, replacement.http, "GET", "/api/drafts/"+keptDraft, nil))
	recoveredPruned := restoreAccount(pruned, controlFreshIdentity(t, replacement.service, "restore-cleaned-detail"))
	prunedProfile := openingRequest(t, recoveredPruned, replacement.http, "GET", "/api/profile/current", nil)
	restoreCode(t, "cleaned detail stays removed", prunedProfile, 200)
	if !bytes.Equal(prunedProfile.Body.Bytes(), originalPruned.Body.Bytes()) {
		t.Fatal("restored pruning changed retained Profile details")
	}
	checkRestoreRecovered(t, target, replacement, fresh, retired, revoked, input, kept, confirmation, deletion, originalRevocation, &targetCalls)
	if restored := restoreUnknownDispatch(t, target, restoreQualification, oldUnknown); restored != priorUnknown {
		t.Fatal("backup unknown changed its identity or intent clock")
	}
	if restored := restoreUnknownDispatch(t, target, restoreDraft, newUnknown); restored != postBackupUnknown {
		t.Fatal("post-backup unknown changed its identity or intent clock")
	}
	t.Log("restore observation both original unknown dispatch identities and durable intent clocks retained")
	if beforeCalls.Load() != 1 || afterCalls.Load() != 1 || len(provider.Calls) != 8 || targetCalls.Load() != 0 {
		t.Fatal("recovery changed independently expected fake callback counts")
	}
	t.Logf("local restore observation backup_to_quarantine_ms=%d quarantine_to_verified_retrieval_ms=%d; backup_sha256=%s decisions_sha256=%s; no selected hosted target",
		started.Sub(backupFinished).Milliseconds(), time.Since(started).Milliseconds(), backupDigest, decisionDigest)
}

type restoreDispatchObservation struct {
	action, id         string
	intent, obligation time.Time
}

func restoreUnknownDispatch(t *testing.T, db *sql.DB, namespace, key string) restoreDispatchObservation {
	t.Helper()
	var d restoreDispatchObservation
	var maximum, counted int64
	err := db.QueryRow(`SELECT a.id,d.id,d.intent_at,d.durable_bound,d.maximum,d.counted
		FROM ai_fixture_action a JOIN ai_fixture_dispatch d ON d.namespace=a.namespace AND d.action=a.id
		WHERE a.namespace=$1 AND a.creation_key=$2 AND d.remote_state='possibly_active'`, namespace, key).
		Scan(&d.action, &d.id, &d.intent, &d.obligation, &maximum, &counted)
	if err != nil || d.action == "" || d.id == "" || d.intent.IsZero() || d.obligation.IsZero() || counted != 1_344 || maximum != 1_344 {
		t.Fatal("expected one full-exposure unknown fixture dispatch", err)
	}
	d.intent, d.obligation = d.intent.UTC(), d.obligation.UTC()
	return d
}

func restoreGeneration(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var generation int64
	if err := db.QueryRow("SELECT system_generation FROM careeros_control_state WHERE singleton").Scan(&generation); err != nil || generation < 1 {
		t.Fatal("local recovery generation unavailable", err)
	}
	return generation
}

func restoreExclusion(pg *restorePostgres, old *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if old.PingContext(ctx) == nil || pg.running {
		return errors.New("old PostgreSQL is not confirmed stopped")
	}
	if _, err := os.Stat(filepath.Join(pg.socket, ".s.PGSQL.55493")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("old PostgreSQL socket still exists or cannot be verified")
	}
	return nil
}

func checkRestoreQuarantine(t *testing.T, who *aiadmission.Fixture, ingress *restoreIngress, db *sql.DB, generation int64, label string) {
	t.Helper()
	if restoreGeneration(t, db) != generation {
		t.Fatalf("%s advanced generation", label)
	}
	restoreCode(t, label+" retains closed reads", openingRequest(t, who, ingress, "GET", "/api/profile/current", nil), 503)
	restoreCode(t, label+" retains closed mutation", openingRequest(t, who, ingress, "POST", "/api/pilot/activate", struct{}{}), 503)
}

func checkRestoreOlderImage(t *testing.T, db *sql.DB, retired, revoked, pruned string) {
	t.Helper()
	var profiles, draftContent, unknown, effects, reviews, fences int
	var oldAccount, oldGrant string
	var oldExcerpt bool
	err := db.QueryRow(`SELECT (SELECT count(*) FROM careeros_profiles),
		(SELECT count(*) FROM careeros_draft_content), (SELECT count(*) FROM ai_fixture_dispatch WHERE remote_state='possibly_active'),
		(SELECT count(*) FROM ai_fixture_action WHERE customer_effect), (SELECT count(*) FROM careeros_draft_reviews),
		(SELECT count(*) FROM careeros_deletion_operations),
		(SELECT status FROM careeros_accounts WHERE id=$1), (SELECT state FROM careeros_pilot_access WHERE account_id=$2),
		EXISTS(SELECT 1 FROM careeros_profiles WHERE account_id=$3 AND convert_from(document,'UTF8') LIKE '%SYNTHETIC DETAIL TO REMOVE%')`, retired, revoked, pruned).
		Scan(&profiles, &draftContent, &unknown, &effects, &reviews, &fences, &oldAccount, &oldGrant, &oldExcerpt)
	if err != nil || profiles != 4 || draftContent != 2 || unknown != 1 || effects != 1 || reviews != 1 || fences != 0 || oldAccount != "active" || oldGrant != "active" || !oldExcerpt {
		t.Fatalf("not the independently expected older image profiles=%d, Drafts=%d, unknown=%d, delivered=%d, reviews=%d, fences=%d, account=%s, grant=%s: %v",
			profiles, draftContent, unknown, effects, reviews, fences, oldAccount, oldGrant, err)
	}
	t.Log("restore observation older backup: Profiles=4, complete Drafts=2, reviewed=1, delivered=1, unknown=1; later detail/deletion and revocation decisions absent")
}

func restoreAdvanceGeneration(t *testing.T, runtime restoreRuntime, operator auth.Principal, generation int64, source *restorePostgres, old *sql.DB) {
	t.Helper()
	if err := restoreExclusion(source, old); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	closed, err := runtime.account.LocalControls(nil).Status(ctx, operator, "")
	if err != nil || closed.SystemGeneration != generation || !closed.InferenceStopped || !closed.InvitationStopped {
		t.Fatal("applying recovered facts did not retain both stops", closed, err)
	}
	recovery, err := aicontrol.NewSyntheticControlRecovery(ctx, runtime.account, runtime.qualification, runtime.draft)
	if err != nil {
		t.Fatal(err)
	}
	command := accountstore.ControlCommand{Control: "inference", ExpectedVersion: closed.Version}
	reference, err := recovery.Prepare(ctx, operator, command)
	if err != nil {
		t.Fatal(err)
	}
	command.RecoveryReference = reference
	if err := recovery.Complete(ctx, operator, reference); err == nil {
		t.Fatal("independent gateway opened before durable generation advance")
	}
	opened, err := runtime.account.LocalControls(recovery).Apply(ctx, operator, "restore-verified-local-opening", command)
	if err != nil || opened.SystemGeneration != generation+1 || !opened.InvitationStopped || opened.InferenceStopped {
		t.Fatal("verified generation advance did not preserve independent invitation stop", opened, err)
	}
	if err := recovery.Complete(ctx, operator, reference); err != nil {
		t.Fatal(err)
	}
	runtime.http.closed.Store(false)
	t.Logf("restore observation verified local generation %d -> %d; invitations remain stopped", generation, opened.SystemGeneration)
}

func checkRestoreRecovered(t *testing.T, db *sql.DB, runtime restoreRuntime, fresh, deleted, revoked *aiadmission.Fixture,
	input draftrecovery.Input, kept []byte, confirmation draftrecovery.OpeningConfirmation, originalDeletion accountstore.DeletionStatus,
	originalRevocation accountstore.PilotStatus, calls *atomic.Int32) {
	t.Helper()
	ctx := context.Background()
	for _, path := range []string{"/api/drafts/" + keptDraft + "/versions/1", "/.netlify/functions/account/api/drafts/" + keptDraft + "/versions/1"} {
		w := openingRequest(t, fresh, runtime.http, "GET", path, nil)
		restoreCode(t, "exact reviewed Draft survives restoration", w, 200)
		if !bytes.Equal(w.Body.Bytes(), kept) {
			t.Fatal("recovery altered a previously acknowledged reviewed Draft")
		}
	}
	confirmed := openingRequest(t, fresh, runtime.http, "POST", "/api/drafts/"+keptDraft+"/versions/1/opening/confirm", confirmation)
	restoreCode(t, "old session-bound delivery receipt cannot settle again", confirmed, 422)
	restoreCode(t, "canceled Draft remains closed", openingRequest(t, fresh, runtime.http, "GET", "/api/drafts/"+canceledDraft+"/versions/1", nil), 410)
	restoreCode(t, "canceled Draft cannot be opened", openingRequest(t, fresh, runtime.http, "POST", "/api/drafts/"+canceledDraft+"/versions/1/opening", struct{}{}), 403)
	restoreCode(t, "retained Profile is readable", openingRequest(t, fresh, runtime.http, "GET", "/api/profile/current", nil), 200)
	reply := openingRequest(t, fresh, runtime.http, "GET", "/api/draft-allowance", nil)
	var allowance openingAllowance
	if reply.Code != 200 || json.Unmarshal(reply.Body.Bytes(), &allowance) != nil || allowance != (openingAllowance{5, 1, 1, 3}) {
		t.Fatal("finite allowance no longer reflects one delivery, one unresolved reservation", allowance, reply.Code)
	}
	for _, path := range []string{"/api/profile/current", "/api/profile/export", "/api/drafts/" + keptDraft} {
		if w := openingRequest(t, deleted, runtime.http, "GET", path, nil); w.Code < 400 {
			t.Fatal("deleted account cookie exposed restored content")
		}
	}
	closed := restoreAccount(deleted, controlFreshIdentity(t, runtime.service, "restore-retired-account"))
	var status accountstore.DeletionStatus
	err := runtime.account.WithDeletionStatus(ctx, closed.Principal, originalDeletion.OperationID, func(v accountstore.DeletionStatus) error { status = v; return nil })
	if err != nil || status.State != "active_content_removed" || !status.FencedAt.Equal(originalDeletion.FencedAt) ||
		status.RecoveryCopyDeadline == nil || !status.RecoveryCopyDeadline.Equal(originalDeletion.FencedAt.Add(720*time.Hour)) {
		t.Fatal("restored deletion metadata failed to preserve original fence and copy clock", err)
	}
	relogged := restoreAccount(revoked, controlFreshIdentity(t, runtime.service, "restore-revoked-grant"))
	grant, err := runtime.account.PilotStatus(ctx, relogged.Principal)
	if err != nil || grant.State != "revoked" || grant.CanActivate || grant.CanDraft || grant.CanStandalone || grant.ExpiresAt == nil || grant.RetrieveUntil == nil ||
		!grant.ExpiresAt.Equal(*originalRevocation.ExpiresAt) || !grant.RetrieveUntil.Equal(*originalRevocation.RetrieveUntil) {
		t.Fatal("revoked grant was resurrected", err)
	}
	var profiles, content, effects, unknown, copyEvidence, qualificationOccupied, draftOccupied int
	var qSpend, dSpend int64
	err = db.QueryRow(`SELECT (SELECT count(*) FROM careeros_profiles WHERE account_id=$1),
		(SELECT count(*) FROM careeros_draft_content), (SELECT count(*) FROM ai_fixture_action WHERE customer_effect),
		(SELECT count(*) FROM ai_fixture_dispatch WHERE remote_state='possibly_active'),
		(SELECT count(*) FROM careeros_lifecycle_residuals WHERE account_id=$1 AND class='recovery_copy'
		    AND original_fenced_at=$2 AND deadline=$2::timestamptz+interval '720 hours' AND revision=1 AND state='pending'),
		(SELECT counted FROM ai_fixture_scope WHERE namespace='qualification' AND scope='global'),
		(SELECT counted FROM ai_fixture_scope WHERE namespace='managed-draft' AND scope='global'),
		(SELECT occupied FROM ai_fixture_scope WHERE namespace='qualification' AND scope='global'),
		(SELECT occupied FROM ai_fixture_scope WHERE namespace='managed-draft' AND scope='global')`, deleted.Principal.AccountID, originalDeletion.FencedAt).
		Scan(&profiles, &content, &effects, &unknown, &copyEvidence, &qSpend, &dSpend, &qualificationOccupied, &draftOccupied)
	if err != nil || profiles != 0 || content != 1 || effects != 1 || unknown != 2 || copyEvidence != 1 || qSpend != 1_344 || dSpend != 72_032 || qualificationOccupied != 1 || draftOccupied != 1 {
		t.Fatalf("recovery changed independently expected evidence: deleted Profiles=%d content=%d delivered=%d unknown=%d copy receipts=%d counted=(%d,%d): %v",
			profiles, content, effects, unknown, copyEvidence, qSpend, dSpend, err)
	}
	for _, item := range []struct {
		h          http.Handler
		route, key string
		body       []byte
	}{
		{accountapi.NewSyntheticQualification(runtime.service, runtime.account, runtime.qualification), "/api/qualification-gaps", oldUnknown, fresh.Body(t, "/api/qualification-gaps")},
		{runtime.http, "/api/drafts", newUnknown, restoreJSON(t, input)},
	} {
		for _, retry := range []string{"", "restore-deliberate-retry"} {
			w := qualificationRequest(t, fresh, item.h, item.route, item.key, retry, item.body)
			if w.Code == http.StatusCreated || calls.Load() != 0 {
				t.Fatalf("restored unknown replay repeated a provider dispatch: %s HTTP %d", item.key, w.Code)
			}
		}
	}
	var again openingAllowance
	if w := openingRequest(t, fresh, runtime.http, "GET", "/api/draft-allowance", nil); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &again) != nil || again != allowance {
		t.Fatal("replay consumed another customer right")
	}
	t.Logf("restore observation reconciled: deleted content absent, copy anchor unchanged, reviewed Draft exact, grants preserved, allowance=%+v spend_microUSD=(%d,%d) occupancy=(%d,%d) unknown=%d, no resend",
		allowance, qSpend, dSpend, qualificationOccupied, draftOccupied, unknown)
}

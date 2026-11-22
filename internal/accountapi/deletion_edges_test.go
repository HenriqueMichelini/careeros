package accountapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
)

func TestDeletionExpiredSessionAfterAllowanceWaitRollsBackFence(t *testing.T) {
	f, _, _, _, router, input := terminalDraftFixture(t)
	body, _ := json.Marshal(input)
	if w := qualificationRequest(t, f, router, "/api/drafts", "deletion-wait", "", body); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	tx, err := f.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT state FROM careeros_draft_allowance WHERE owner=$1 FOR UPDATE`, f.Principal.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.DB.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '700 milliseconds' WHERE id=$1`, f.Principal.SessionID); err != nil {
		t.Fatal(err)
	}
	deletion := accountapi.New(f.Service, f.Store)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- openingRequest(t, f, deletion, http.MethodPost, "/api/deletion/operations", accountstore.DeletionRequest{OperationID: "wait-delete", Kind: "profile", ProfileID: input.Profile.ID, Revision: input.Profile.Revision, Confirm: true})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		if err = f.DB.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE 'UPDATE careeros_draft_allowance SET state=%'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("deletion never reached allowance wait")
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(750 * time.Millisecond)
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	w := <-done
	if w.Code != 401 {
		t.Fatal("expired blocked deletion committed", w.Code, w.Body.String())
	}
	var profiles, operations int
	if err = f.DB.QueryRow(`SELECT count(*) FROM careeros_profiles WHERE account_id=$1`, f.Principal.AccountID).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if err = f.DB.QueryRow(`SELECT count(*) FROM careeros_deletion_operations WHERE account_id=$1`, f.Principal.AccountID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if profiles != 1 || operations != 0 {
		t.Fatal("unauthorized fence persisted", profiles, operations)
	}
}

func TestDeletionBoundDraftRemovalKeepsProviderAndDeliveredEffects(t *testing.T) {
	for _, deliver := range []bool{false, true} {
		t.Run(map[bool]string{false: "unopened", true: "delivered"}[deliver], func(t *testing.T) {
			f, h, provider, _, router, input := terminalDraftFixture(t)
			ctx := context.Background()
			body, _ := json.Marshal(input)
			created := qualificationRequest(t, f, router, "/api/drafts", "bound-history", "", body)
			var ready draftrecovery.Result
			if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &ready) != nil {
				t.Fatal(created.Code, created.Body.String())
			}
			if deliver {
				w := openingRequest(t, f, router, "POST", "/api/drafts/bound-history/versions/1/opening", struct{}{})
				var receipt draftrecovery.OpeningReceipt
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &receipt) != nil {
					t.Fatal(w.Code, w.Body.String())
				}
				w = openingRequest(t, f, router, "POST", "/api/drafts/bound-history/versions/1/opening/confirm", draftrecovery.OpeningConfirmation{Receipt: receipt.Receipt, BundleDigest: receipt.BundleDigest, Manifest: receipt.Manifest})
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			before, err := h.ScopeStatus(ctx, f.Principal, "global")
			if err != nil {
				t.Fatal(err)
			}
			deletion := accountapi.New(f.Service, f.Store)
			w := openingRequest(t, f, deletion, "POST", "/api/deletion/operations", accountstore.DeletionRequest{OperationID: "delete-history", Kind: "profile", ProfileID: input.Profile.ID, Revision: input.Profile.Revision, Confirm: true})
			if w.Code != 202 {
				t.Fatal(w.Code, w.Body.String())
			}
			w = openingRequest(t, f, router, "GET", "/api/drafts/bound-history/versions/1", nil)
			if w.Code != 410 || strings.Contains(w.Body.String(), `"bundle":`) {
				t.Fatal("history escaped fence", w.Code, w.Body.String())
			}
			if _, err = f.Store.PurgeDeletion(ctx, f.Principal.AccountID, "delete-history"); err != nil {
				t.Fatal(err)
			}
			var contents int
			if err = f.DB.QueryRow(`SELECT count(*) FROM careeros_draft_content`).Scan(&contents); err != nil || contents != 0 {
				t.Fatal("active history survived", contents, err)
			}
			after, err := h.ScopeStatus(ctx, f.Principal, "global")
			if err != nil || after != before {
				t.Fatal("deletion changed provider exposure", before, after, err)
			}
			var effect, reserved bool
			if err = f.DB.QueryRow(`SELECT customer_effect,customer_reserved FROM ai_fixture_action WHERE id=$1`, ready.Action.ID).Scan(&effect, &reserved); err != nil || effect != deliver || reserved {
				t.Fatal("customer outcome changed", effect, reserved, err)
			}
			if len(provider.Calls) != 4 {
				t.Fatal("deletion sent inference", len(provider.Calls))
			}
		})
	}
}

func TestDeletionCleanupFailureIsVisibleWithoutReopeningOrMovingClock(t *testing.T) {
	f, _, _, _, router, input := terminalDraftFixture(t)
	ctx := context.Background()
	body, _ := json.Marshal(input)
	if w := qualificationRequest(t, f, router, "/api/drafts", "cleanup-failure", "", body); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	req := accountstore.DeletionRequest{OperationID: "failed-purge", Kind: "profile", ProfileID: input.Profile.ID, Revision: input.Profile.Revision, Confirm: true}
	started, err := f.Store.StartDeletion(ctx, f.Principal, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.DB.Exec(`CREATE FUNCTION deletion_fail_cleanup() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'synthetic removal failure'; END$$; CREATE TRIGGER fail_cleanup BEFORE DELETE ON careeros_draft_content FOR EACH ROW EXECUTE FUNCTION deletion_fail_cleanup()`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.PurgeDeletion(ctx, f.Principal.AccountID, req.OperationID); err == nil {
		t.Fatal("cleanup failure claimed success")
	}
	var status accountstore.DeletionStatus
	if err = f.Store.WithDeletionStatus(ctx, f.Principal, req.OperationID, func(v accountstore.DeletionStatus) error { status = v; return nil }); err != nil {
		t.Fatal(err)
	}
	if status.State != "failed" || status.RemovedAt != nil || status.FailureCode != "active_removal_failed" || !status.RecoveryCopyDeadline.Equal(*started.RecoveryCopyDeadline) {
		t.Fatal("untruthful failed status", status)
	}
	if w := openingRequest(t, f, router, "GET", "/api/drafts/cleanup-failure", nil); w.Code != 410 {
		t.Fatal("failure reopened content", w.Code)
	}
	if _, err = f.DB.Exec(`DROP TRIGGER fail_cleanup ON careeros_draft_content`); err != nil {
		t.Fatal(err)
	}
	done, err := f.Store.PurgeDeletion(ctx, f.Principal.AccountID, req.OperationID)
	if err != nil || done.State != "active_content_removed" || !done.RecoveryCopyDeadline.Equal(*started.RecoveryCopyDeadline) || !done.FencedAt.Equal(started.FencedAt) {
		t.Fatal("retry changed original ceiling", done, err)
	}
}

func TestDeletionUnknownDispatchKeepsEveryHoldAndCannotRetry(t *testing.T) {
	f, h, provider, _, router, input := terminalDraftFixture(t)
	if err := h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
		response, err := provider.Respond(ctx, p, r)
		return response, false, err
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(input)
	qualificationRequest(t, f, router, "/api/drafts", "delete-unknown", "", body)
	before, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || before.Counted != 1344 || before.Occupied != 1 {
		t.Fatal("unknown floor", before, err)
	}
	if _, err = f.Store.StartDeletion(context.Background(), f.Principal, accountstore.DeletionRequest{OperationID: "delete-unknown", Kind: "profile", ProfileID: input.Profile.ID, Revision: input.Profile.Revision, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.PurgeDeletion(context.Background(), f.Principal.AccountID, "delete-unknown"); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"global", "account:" + f.Principal.AccountID} {
		after, err := h.ScopeStatus(context.Background(), f.Principal, scope)
		if err != nil || after.Counted != 1344 || after.Occupied != 1 {
			t.Fatal("deletion released unknown", scope, after, err)
		}
	}
	w := qualificationRequest(t, f, router, "/api/drafts", "delete-unknown", "forbidden-retry", body)
	if w.Code < 400 || len(provider.Calls) != 1 {
		t.Fatal("deleted unknown retried", w.Code, len(provider.Calls))
	}
}

func TestDeletionSurvivingMarkersRejectRestoredProfileAndDeliveredHistory(t *testing.T) {
	f, _, _, _, router, input := terminalDraftFixture(t)
	ctx := context.Background()
	body, _ := json.Marshal(input)
	w := qualificationRequest(t, f, router, "/api/drafts", "restore-deleted", "", body)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	issued := openingRequest(t, f, router, "POST", "/api/drafts/restore-deleted/versions/1/opening", struct{}{})
	var receipt draftrecovery.OpeningReceipt
	if issued.Code != 200 || json.Unmarshal(issued.Body.Bytes(), &receipt) != nil {
		t.Fatal(issued.Code)
	}
	w = openingRequest(t, f, router, "POST", "/api/drafts/restore-deleted/versions/1/opening/confirm", draftrecovery.OpeningConfirmation{Receipt: receipt.Receipt, BundleDigest: receipt.BundleDigest, Manifest: receipt.Manifest})
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if _, err := f.DB.Exec(`CREATE TABLE deletion_old_content AS SELECT * FROM careeros_draft_content`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Store.StartDeletion(ctx, f.Principal, accountstore.DeletionRequest{OperationID: "restore-marker", Kind: "profile", ProfileID: input.Profile.ID, Revision: input.Profile.Revision, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Store.PurgeDeletion(ctx, f.Principal.AccountID, "restore-marker"); err != nil {
		t.Fatal(err)
	}
	// A deliberately stale restore keeps the newer deletion markers but brings
	// older mutable source/history rows back. It must remain closed to career data.
	if _, err := f.DB.Exec(`INSERT INTO careeros_profiles VALUES($1,$2,$3,$4)`, f.Principal.AccountID, []byte(input.Profile.ID), input.Profile.Revision, []byte(f.Document)); err != nil {
		t.Fatal(err)
	}
	profiles := accountapi.New(f.Service, f.Store)
	for _, path := range []string{"/api/profile/current", "/api/profile/export"} {
		w := openingRequest(t, f, profiles, "GET", path, nil)
		if w.Code == 200 {
			t.Error("restored deleted Profile escaped marker", path, w.Body.String())
		}
	}
	if _, err := f.DB.Exec(`DELETE FROM careeros_profiles;INSERT INTO careeros_draft_content SELECT * FROM deletion_old_content;UPDATE careeros_draft_actions SET state='ready',cleanup_queued=false;UPDATE ai_fixture_action SET state='delivered' WHERE workflow='application_draft'`); err != nil {
		t.Fatal(err)
	}
	w = openingRequest(t, f, router, "GET", "/api/drafts/restore-deleted/versions/1", nil)
	if w.Code != 410 || strings.Contains(w.Body.String(), `"bundle":`) {
		t.Error("restored delivered content escaped tombstone", w.Code, w.Body.String())
	}
}

func TestDeletionOriginalCutoffDeniesOldAbsentBindingWithoutClosingNewAction(t *testing.T) {
	f, _, _, router := ingestionFixture(t)
	ctx := context.Background()
	snapshot := emptyIngestionSnapshot(t, f)
	body := ingestionBody(t, f, "plan", snapshot)
	w := qualificationRequest(t, f, router, "/api/profile/ingest", "old-absent-plan", "", body)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := f.Store.SaveProfile(ctx, f.Principal, "first-save", 0, f.Document); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	json.Unmarshal(f.Document, &doc)
	if _, err := f.Store.StartDeletion(ctx, f.Principal, accountstore.DeletionRequest{OperationID: "cutoff", Kind: "profile", ProfileID: doc.ID, Revision: doc.Revision, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.Exec(`UPDATE ai_fixture_action SET state='recoverable' WHERE creation_key='old-absent-plan'`); err != nil {
		t.Fatal(err)
	}
	w = qualificationRequest(t, f, router, "/api/profile/ingest", "old-absent-plan", "", body)
	if w.Code < 400 {
		t.Error("older absent binding revived", w.Code, w.Body.String())
	}
	w = qualificationRequest(t, f, router, "/api/profile/ingest", "new-post-fence-plan", "", body)
	if w.Code != 200 {
		t.Fatal("new legitimate Action denied", w.Code, w.Body.String())
	}
	recovery, err := accountstore.NewDeletionRecovery(f.Store, []byte("synthetic-independent-cutoff-recovery-key"))
	if err != nil {
		t.Fatal(err)
	}
	marker, err := recovery.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = recovery.Apply(ctx, marker); err != nil {
		t.Fatal(err)
	}
	w = qualificationRequest(t, f, router, "/api/profile/ingest", "new-post-fence-plan", "", body)
	if w.Code != 200 {
		t.Fatal("reapplied old fence closed new Action", w.Code, w.Body.String())
	}
}

func TestDeletionWinsDuringGenerationBeforePublicationOrOpening(t *testing.T) {
	f, h, provider, _, router, input := terminalDraftFixture(t)
	reached, release := make(chan struct{}), make(chan struct{})
	if err := h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
		if aidiagnostics.StageName(ctx) == "generation" {
			close(reached)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, false, ctx.Err()
			}
		}
		response, err := provider.Respond(ctx, p, r)
		return response, true, err
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(input)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- qualificationRequest(t, f, router, "/api/drafts", "late-deleted-generation", "", body) }()
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("generation not reached")
	}
	if _, err := f.Store.StartDeletion(context.Background(), f.Principal, accountstore.DeletionRequest{OperationID: "during-generation", Kind: "profile", ProfileID: input.Profile.ID, Revision: input.Profile.Revision, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	close(release)
	w := <-done
	if w.Code < 400 || strings.Contains(w.Body.String(), `"bundle":`) {
		t.Fatal("late generation published after fence", w.Code, w.Body.String())
	}
	w = openingRequest(t, f, router, "GET", "/api/drafts/late-deleted-generation", nil)
	if w.Code != 410 {
		t.Fatal("late publication revived purpose", w.Code, w.Body.String())
	}
	var effect, reserved bool
	if err := f.DB.QueryRow(`SELECT customer_effect,customer_reserved FROM ai_fixture_action WHERE creation_key='late-deleted-generation'`).Scan(&effect, &reserved); err != nil || effect || reserved {
		t.Fatal("late customer effect", effect, reserved, err)
	}
	if len(provider.Calls) != 2 {
		t.Fatal("late support/fallback sent", len(provider.Calls))
	}
	var counted int64
	if err := f.DB.QueryRow(`SELECT sum(counted) FROM ai_fixture_dispatch`).Scan(&counted); err != nil || counted < 13344 {
		t.Fatal("deletion erased reached obligation", counted, err)
	}
}

func TestDeletionKnownEarlyAccessEndPreservesDeliveredDraftReadOnly(t *testing.T) {
	f, _, provider, _, router, input := terminalDraftFixture(t)
	ctx := context.Background()
	body, _ := json.Marshal(input)
	created := qualificationRequest(t, f, router, "/api/drafts", "early-end-delivered", "", body)
	if created.Code != 201 {
		t.Fatal(created.Code)
	}
	issued := openingRequest(t, f, router, "POST", "/api/drafts/early-end-delivered/versions/1/opening", struct{}{})
	var receipt draftrecovery.OpeningReceipt
	if issued.Code != 200 || json.Unmarshal(issued.Body.Bytes(), &receipt) != nil {
		t.Fatal(issued.Code)
	}
	delivered := openingRequest(t, f, router, "POST", "/api/drafts/early-end-delivered/versions/1/opening/confirm", draftrecovery.OpeningConfirmation{Receipt: receipt.Receipt, BundleDigest: receipt.BundleDigest, Manifest: receipt.Manifest})
	if delivered.Code != 200 {
		t.Fatal(delivered.Code)
	}
	if _, err := f.Store.PilotCommand(ctx, f.Principal, "early-end", accountstore.PilotCommand{Kind: "revoke", Identity: auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "fixture-account"}}); err != nil {
		t.Fatal(err)
	}
	w := openingRequest(t, f, router, "GET", "/api/drafts/early-end-delivered/versions/1", nil)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), delivered.Body.Bytes()) {
		t.Fatal("known early end lost delivered history", w.Code, w.Body.String())
	}
	if w := qualificationRequest(t, f, router, "/api/drafts", "forbidden-after-end", "", body); w.Code < 400 {
		t.Fatal("revocation allowed new AI", w.Code)
	}
	if _, err := f.DB.Exec(`UPDATE careeros_pilot_access SET retrieval_held=true WHERE account_id=$1`, f.Principal.AccountID); err != nil {
		t.Fatal(err)
	}
	if w := openingRequest(t, f, router, "GET", "/api/drafts/early-end-delivered/versions/1", nil); w.Code != 403 {
		t.Fatal("compromise hold released history", w.Code)
	}
	if _, err := f.DB.Exec(`UPDATE careeros_pilot_access SET retrieval_held=false,access_ended_at=clock_timestamp()-interval '30 days' WHERE account_id=$1`, f.Principal.AccountID); err != nil {
		t.Fatal(err)
	}
	if w := openingRequest(t, f, router, "GET", "/api/drafts/early-end-delivered/versions/1", nil); w.Code != 403 {
		t.Fatal("earliest retrieval ceiling ignored", w.Code)
	}
	if len(provider.Calls) != 4 {
		t.Fatal("read-only recovery inferred", len(provider.Calls))
	}
}

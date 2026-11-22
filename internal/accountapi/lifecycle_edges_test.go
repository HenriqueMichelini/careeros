package accountapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"professional-information-repo/internal/accountstore"
	"testing"
	"time"
)

func TestLifecycleCleanupFailureKeepsFenceAndOriginalCopyDeadline(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "cleanup-failure")
	store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true)
	if w := api(t, NewPilot(svc, store, nil), who, "PUT", "/api/profile/current", command("save", 0, fixture(t, 1))); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec(`UPDATE careeros_pilot_access SET activated_at=clock_timestamp()-interval '45 days',expires_at=clock_timestamp()-interval '31 days' WHERE account_id=$1`, who.Account.ID)
	if _, err = db.Exec(`CREATE FUNCTION lifecycle_fail_complete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='active_content_removed' THEN RAISE EXCEPTION 'synthetic cleanup failure'; END IF;RETURN NEW;END $$;CREATE TRIGGER lifecycle_fail BEFORE UPDATE ON careeros_deletion_operations FOR EACH ROW EXECUTE FUNCTION lifecycle_fail_complete()`); err != nil {
		t.Fatal(err)
	}
	worker, _ := accountstore.NewLifecycleWorker(store, nil)
	if _, err = worker.Run(context.Background(), 10); err == nil {
		t.Fatal("cleanup failure was hidden")
	}
	h := NewPilot(svc, store, nil)
	w := api(t, h, who, "GET", "/api/lifecycle/status", nil)
	var before accountstore.LifecycleStatus
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &before) != nil || len(before.Cleanup) != 1 || before.Cleanup[0].State != "failed" || !before.ContentExpired {
		t.Fatal("failure status", w.Code, w.Body.String())
	}
	if x := api(t, h, who, "GET", "/api/profile/export", nil); x.Code != 403 {
		t.Fatal("failure reopened career read", x.Code)
	}
	if _, err = db.Exec(`DROP TRIGGER lifecycle_fail ON careeros_deletion_operations`); err != nil {
		t.Fatal(err)
	}
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	w = api(t, h, who, "GET", "/api/lifecycle/status", nil)
	var after accountstore.LifecycleStatus
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &after) != nil || after.Cleanup[0].State != "active_content_removed" || !after.Cleanup[0].FencedAt.Equal(before.Cleanup[0].FencedAt) || !after.Cleanup[0].RecoveryCopyDeadline.Equal(*before.Cleanup[0].RecoveryCopyDeadline) {
		t.Fatal("retry reset deletion/copy clock", w.Code, w.Body.String())
	}
}

func TestLifecycleStatusUsesFreshSessionAfterAuthorityLockWait(t *testing.T) {
	dsn, db := database(t)
	store := open(t, dsn, "lifecycle_status_wait")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, store, fakeOIDC{})
	who := login(t, svc, "status-wait")
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(72481306)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '2 seconds' WHERE id=$1`, who.Principal.SessionID); err != nil {
		t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- api(t, NewPilot(svc, store, nil), who, "GET", "/api/lifecycle/status", nil) }()
	waitBlocked(t, db, "lifecycle_status_wait", "")
	time.Sleep(2100 * time.Millisecond)
	tx.Commit()
	select {
	case w := <-done:
		if w.Code != 401 {
			t.Fatal("stale session released metadata", w.Code, w.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("status wait did not terminate")
	}
}

func TestLifecycleStatusOuterWriteSerializesAccountClosure(t *testing.T) {
	dsn, db := database(t)
	store := open(t, dsn, "lifecycle_outer_read")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, store, fakeOIDC{})
	who := login(t, svc, "outer-status")
	writer := open(t, dsn, "lifecycle_closer")
	done := make(chan error, 1)
	out := exportWriteFence{httptest.NewRecorder(), func() {
		go func() {
			_, err := writer.StartDeletion(context.Background(), who.Principal, accountstore.DeletionRequest{OperationID: "close-during-status", Kind: "account", Confirm: true})
			done <- err
		}()
		waitBlocked(t, db, "lifecycle_closer", "")
		select {
		case e := <-done:
			t.Fatal("closure won before outer write", e)
		default:
		}
	}}
	r := httptest.NewRequest("GET", "/api/lifecycle/status", nil)
	r.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: who.SessionToken})
	r.Header.Set("X-CareerOS-Account", who.Account.ID)
	NewPilot(svc, store, nil).ServeHTTP(out, r)
	if out.Code != 200 {
		t.Fatal(out.Code, out.Body.String())
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if w := api(t, NewPilot(svc, store, nil), who, "GET", "/api/lifecycle/status", nil); w.Code != 401 {
		t.Fatal("old session survived closure", w.Code)
	}
}

func TestLifecycleLocalCommandIsBoundedAndClosedToRecipients(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "cli-lifecycle")
	store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec(`UPDATE careeros_pilot_access SET activated_at=clock_timestamp()-interval '15 days',expires_at=clock_timestamp()-interval '1 day' WHERE account_id=$1`, who.Account.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	exe := filepath.Join(t.TempDir(), "lifecycle-local")
	build := exec.CommandContext(ctx, "rtk", "proxy", "go", "build", "-o", exe, "./cmd/lifecycle-local")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	run := exec.CommandContext(ctx, "rtk", "proxy", exe, "--synthetic-only", "--limit", "1")
	run.Env = append(os.Environ(), "CAREEROS_LIFECYCLE_LOCAL_DATABASE_URL="+dsn)
	out, err := run.CombinedOutput()
	var batch accountstore.LifecycleBatch
	if err != nil || json.Unmarshal(out, &batch) != nil || batch.Processed != 1 {
		t.Fatal("normal buildable bounded command", err, string(out))
	}
	var state string
	if err = db.QueryRow(`SELECT email_state FROM careeros_lifecycle_notices WHERE account_id=$1`, who.Account.ID).Scan(&state); err != nil || state != "unavailable" {
		t.Fatal("CLI enabled recipient transport", state, err)
	}
	deny := exec.CommandContext(ctx, "rtk", "proxy", exe, "--synthetic-only")
	deny.Env = append(os.Environ(), "CAREEROS_LIFECYCLE_LOCAL_DATABASE_URL=postgresql://synthetic.invalid/careeros_test")
	if out, err := deny.CombinedOutput(); err == nil {
		t.Fatal("nonlocal command accepted", string(out))
	}
}

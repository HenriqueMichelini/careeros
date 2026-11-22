package accountapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"testing"
	"time"
)

func TestDeletionProfileRequestFencesImmediatelyAndRecoversStatus(t *testing.T) {
	dsn, _ := database(t)
	store := open(t, dsn, "deletion_tracer")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, store, fakeOIDC{})
	who := login(t, svc, "deletion-owner")
	h := New(svc, store)
	raw := fixture(t, 1)
	if w := api(t, h, who, "PUT", "/api/profile/current", command("save", 0, raw)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var doc struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"operationId": "delete-one", "kind": "profile", "profileId": doc.ID, "revision": doc.Revision, "confirm": true})
	first := api(t, h, who, "POST", "/api/deletion/operations", body)
	if first.Code != 202 {
		t.Fatal("deletion must commit its fence", first.Code, first.Body.String())
	}
	var started map[string]any
	if json.Unmarshal(first.Body.Bytes(), &started) != nil || started["state"] != "fenced_pending" {
		t.Fatal(first.Body.String())
	}
	if w := api(t, h, who, "GET", "/api/profile/export", nil); w.Code != 404 {
		t.Fatal("fenced export", w.Code, w.Body.String())
	}
	for _, method := range []string{"GET", "POST"} {
		path := "/api/deletion/operations/delete-one"
		var data []byte
		if method == "POST" {
			path = "/api/deletion/operations"
			data = body
		}
		w := api(t, h, who, method, path, data)
		if w.Code != 200 && w.Code != 202 {
			t.Fatal("lost-response recovery", w.Code, w.Body.String())
		}
		var got map[string]any
		if json.Unmarshal(w.Body.Bytes(), &got) != nil || got["operationId"] != "delete-one" || got["state"] != "fenced_pending" {
			t.Fatal(w.Body.String())
		}
	}
	if w := api(t, h, who, "PUT", "/api/profile/current", command("late-new-key", 0, raw)); w.Code != 409 {
		t.Fatal("deleted identity resurrected", w.Code, w.Body.String())
	}
}

func TestDeletionProfileCleanupDoesNotRestoreIdentityOrGrant(t *testing.T) {
	_, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "purge-person")
	if _, err := store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	h := NewPilot(svc, store, nil)
	raw := fixture(t, 1)
	if w := api(t, h, who, "PUT", "/api/profile/current", command("save", 0, raw)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var doc struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	json.Unmarshal(raw, &doc)
	before, _ := store.PilotStatus(context.Background(), who.Principal)
	_, err := store.StartDeletion(context.Background(), who.Principal, accountstore.DeletionRequest{OperationID: "purge", Kind: "profile", ProfileID: doc.ID, Revision: doc.Revision, Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	out, err := store.PurgeDeletion(context.Background(), who.Account.ID, "purge")
	if err != nil || out.State != "active_content_removed" || out.RemovedAt == nil || out.RecoveryCopyDeadline == nil || out.RecoveryCopies != "not_verified" {
		t.Fatal("cleanup", out, err)
	}
	duplicate, err := store.PurgeDeletion(context.Background(), who.Account.ID, "purge")
	if err != nil || !duplicate.RemovedAt.Equal(*out.RemovedAt) {
		t.Fatal("cleanup reset anchor", duplicate, err)
	}
	after, _ := store.PilotStatus(context.Background(), who.Principal)
	if !after.CanStandalone || !after.ExpiresAt.Equal(*before.ExpiresAt) || after.DraftAllowance != 5 {
		t.Fatal("deletion altered original rights", before, after)
	}
	if w := api(t, h, who, "PUT", "/api/profile/current", command("recreate", 0, raw)); w.Code != 409 {
		t.Fatal("old identity returned", w.Code)
	}
	next := bytes.ReplaceAll(raw, []byte(`"`+doc.ID+`"`), []byte(`"replacement-profile"`))
	if w := api(t, h, who, "PUT", "/api/profile/current", command("new-profile", 0, next)); w.Code != 200 {
		t.Fatal("new identity denied", w.Code, w.Body.String())
	}
}

func TestDeletionEarlyRevocationStartsOriginalRetrievalWindow(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "revocation-window")
	if _, err := store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	h := NewPilot(svc, store, nil)
	if w := api(t, h, who, "PUT", "/api/profile/current", command("save", 0, fixture(t, 1))); w.Code != 200 {
		t.Fatal(w.Code)
	}
	identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "revocation-window"}
	ended, err := store.PilotCommand(context.Background(), operator.Principal, "revoke", accountstore.PilotCommand{Kind: "revoke", Identity: identity})
	if err != nil || ended.CanStandalone || !ended.CanRetrieve || ended.RetrieveUntil == nil {
		t.Fatal("fresh trusted retrieval after early revoke", ended, err)
	}
	if w := api(t, h, who, "GET", "/api/profile/export", nil); w.Code != 200 {
		t.Fatal("retained export", w.Code, w.Body.String())
	}
	time.Sleep(2 * time.Millisecond)
	again, err := store.PilotCommand(context.Background(), operator.Principal, "revoke-again", accountstore.PilotCommand{Kind: "revoke", Identity: identity})
	if err != nil || !again.RetrieveUntil.Equal(*ended.RetrieveUntil) {
		t.Fatal("repeated revoke extended retrieval", again, err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE careeros_pilot_access SET access_ended_at=clock_timestamp()-interval '30 days' WHERE account_id=$1`, who.Account.ID); err != nil {
		t.Fatal(err)
	}
	if w := api(t, h, who, "GET", "/api/profile/export", nil); w.Code != 403 {
		t.Fatal("strict earliest end", w.Code)
	}
}

func TestDeletionClosedAccountFreshStatusDoesNotReactivateWorkspace(t *testing.T) {
	dsn, _ := database(t)
	store := open(t, dsn, "deletion_closed")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, store, fakeOIDC{})
	who := login(t, svc, "closing-person")
	h := New(svc, store)
	if w := api(t, h, who, "PUT", "/api/profile/current", command("save", 0, fixture(t, 1))); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := api(t, h, who, "POST", "/api/deletion/operations", []byte(`{"operationId":"closure","kind":"account","profileId":"","revision":0,"confirm":true}`)); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := api(t, h, who, "GET", "/api/deletion/operations/closure", nil); w.Code != 401 {
		t.Fatal("old session survived closure", w.Code)
	}
	fresh := login(t, svc, "closing-person")
	if fresh.Account.ID != who.Account.ID {
		t.Fatal("closure created replacement account")
	}
	for _, path := range []string{"/api/profile/current", "/api/profile/export"} {
		if w := api(t, h, fresh, "GET", path, nil); w.Code != 401 {
			t.Fatal("status session entered workspace", path, w.Code)
		}
	}
	w := api(t, h, fresh, "GET", "/api/deletion/operations/closure", nil)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"fenced_pending"`)) {
		t.Fatal("fresh metadata status", w.Code, w.Body.String())
	}
	if _, err := store.PurgeDeletion(context.Background(), who.Account.ID, "closure"); err != nil {
		t.Fatal(err)
	}
	w = api(t, h, fresh, "GET", "/api/deletion/operations/closure", nil)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"active_content_removed"`)) {
		t.Fatal(w.Code, w.Body.String())
	}
	other := login(t, svc, "different-person")
	if w := api(t, h, other, "GET", "/api/deletion/operations/closure", nil); w.Code != 404 {
		t.Fatal("foreign status", w.Code)
	}
}

func TestDeletionLegacyRevocationCannotInventAccessEnd(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "unknown-access-end")
	if _, err := store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE careeros_pilot_access SET state='revoked',access_ended_at=NULL WHERE account_id=$1`, who.Account.ID); err != nil {
		t.Fatal(err)
	}
	status, err := store.PilotCommand(context.Background(), operator.Principal, "later-revoke", accountstore.PilotCommand{Kind: "revoke", Identity: auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "unknown-access-end"}})
	if err != nil || status.CanRetrieve {
		t.Fatal("unknown earlier end gained new rights", status, err)
	}
	if status.RetrieveUntil != nil {
		t.Fatal("unknown effective end advertised a retrieval deadline", status.RetrieveUntil)
	}
	var ended sql.NullTime
	if err = db.QueryRow(`SELECT access_ended_at FROM careeros_pilot_access WHERE account_id=$1`, who.Account.ID).Scan(&ended); err != nil || ended.Valid {
		t.Fatal("invented revocation timestamp", ended, err)
	}
}

func TestDeletionCannotWinBeforeCurrentProfileOuterWriteEnds(t *testing.T) {
	dsn, db := database(t)
	store := open(t, dsn, "profile_delete_reader")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, store, fakeOIDC{})
	who := login(t, svc, "profile-delete-write")
	h := New(svc, store)
	raw := fixture(t, 1)
	if w := api(t, h, who, "PUT", "/api/profile/current", command("save", 0, raw)); w.Code != 200 {
		t.Fatal(w.Code)
	}
	var doc struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	json.Unmarshal(raw, &doc)
	writer := open(t, dsn, "deletion_outer_writer")
	finished := make(chan error, 1)
	w := exportWriteFence{httptest.NewRecorder(), func() {
		go func() {
			_, err := writer.StartDeletion(context.Background(), who.Principal, accountstore.DeletionRequest{OperationID: "during-write", Kind: "profile", ProfileID: doc.ID, Revision: doc.Revision, Confirm: true})
			finished <- err
		}()
		time.Sleep(30 * time.Millisecond)
		select {
		case err := <-finished:
			t.Fatal("deletion won before protected outer write", err)
		default:
		}
		waitBlocked(t, db, "deletion_outer_writer", "")
	}}
	req := httptest.NewRequest("GET", "/api/profile/current", nil)
	req.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: who.SessionToken})
	req.Header.Set("X-CareerOS-Account", who.Account.ID)
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if next := api(t, h, who, "GET", "/api/profile/export", nil); next.Code != 404 {
		t.Fatal("post-fence export", next.Code)
	}
}

func TestDeletionClosedMarkerOverridesRestoredActiveAccountAndSession(t *testing.T) {
	dsn, db := database(t)
	store := open(t, dsn, "closed_restore")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, store, fakeOIDC{})
	who := login(t, svc, "closed-restore")
	h := New(svc, store)
	if _, err := db.Exec(`CREATE TABLE deletion_old_sessions AS SELECT * FROM careeros_sessions`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartDeletion(context.Background(), who.Principal, accountstore.DeletionRequest{OperationID: "closed-restore", Kind: "account", Confirm: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE careeros_accounts SET status='active',generation=1;INSERT INTO careeros_sessions SELECT * FROM deletion_old_sessions`); err != nil {
		t.Fatal(err)
	}
	if w := api(t, h, who, "GET", "/api/profile/current", nil); w.Code != 401 {
		t.Fatal("closed account revived from old rows", w.Code, w.Body.String())
	}
}

func TestDeletionSealedRecoveryHandoffPreservesOriginalFenceAndNewProfile(t *testing.T) {
	dsn, db := database(t)
	store := open(t, dsn, "deletion_handoff")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, store, fakeOIDC{})
	who := login(t, svc, "handoff-owner")
	h := New(svc, store)
	raw := fixture(t, 1)
	if w := api(t, h, who, "PUT", "/api/profile/current", command("save", 0, raw)); w.Code != 200 {
		t.Fatal(w.Code)
	}
	var doc struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	json.Unmarshal(raw, &doc)
	started, err := store.StartDeletion(context.Background(), who.Principal, accountstore.DeletionRequest{OperationID: "handoff", Kind: "profile", ProfileID: doc.ID, Revision: doc.Revision, Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := accountstore.NewDeletionRecovery(store, []byte("synthetic-independent-deletion-audit-key"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := recovery.Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(snapshot)
	if bytes.Contains(encoded, []byte("ação")) || bytes.Contains(encoded, []byte("synthetic-independent-deletion-audit-key")) {
		t.Fatal("handoff leaked career/key content")
	}
	// Restored mutable rows and lost markers are fenced again before any opening.
	if _, err = db.Exec(`DELETE FROM careeros_deleted_profiles;DELETE FROM careeros_deletion_operations`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO careeros_profiles VALUES($1,$2,1,$3)`, who.Account.ID, []byte(doc.ID), raw); err != nil {
		t.Fatal(err)
	}
	changed := snapshot
	changed.Fences = append([]accountstore.DeletionFence(nil), snapshot.Fences...)
	changed.Fences[0].ProfileID = "tampered"
	if err = recovery.Apply(context.Background(), changed); err == nil {
		t.Fatal("unverified handoff applied")
	}
	if err = recovery.Apply(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	var recovered accountstore.DeletionStatus
	if err = store.WithDeletionStatus(context.Background(), who.Principal, "handoff", func(v accountstore.DeletionStatus) error { recovered = v; return nil }); err != nil {
		t.Fatal(err)
	}
	if !recovered.FencedAt.Equal(started.FencedAt) || !recovered.RecoveryCopyDeadline.Equal(*started.RecoveryCopyDeadline) {
		t.Fatal("restore reset original anchors", started, recovered)
	}
	next := bytes.ReplaceAll(raw, []byte(`"`+doc.ID+`"`), []byte(`"new-profile-after-deletion"`))
	if w := api(t, h, who, "PUT", "/api/profile/current", command("new", 0, next)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err = recovery.Apply(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	w := api(t, h, who, "GET", "/api/profile/current", nil)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"new-profile-after-deletion"`)) {
		t.Fatal("old-marker replay removed new Profile", w.Code, w.Body.String())
	}
}

func TestDeletionHandoffRetainsUnknownEndCompromiseHold(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "handoff-held")
	if _, err := store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PilotCommand(context.Background(), operator.Principal, "hold", accountstore.PilotCommand{Kind: "suspend", Identity: auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "handoff-held"}}); err != nil {
		t.Fatal(err)
	}
	recovery, err := accountstore.NewDeletionRecovery(store, []byte("synthetic-independent-held-recovery-key"))
	if err != nil {
		t.Fatal(err)
	}
	marker, err := recovery.Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE careeros_pilot_access SET state='active',retrieval_held=false WHERE account_id=$1`, who.Account.ID); err != nil {
		t.Fatal(err)
	}
	if err = recovery.Apply(context.Background(), marker); err != nil {
		t.Fatal(err)
	}
	state, err := store.PilotStatus(context.Background(), who.Principal)
	if err != nil || state.CanRetrieve || state.CanDraft || state.CanStandalone || state.State != "suspended" {
		t.Fatal("restore lost an independent hold", state, err)
	}
	var end sql.NullTime
	if err = db.QueryRow(`SELECT access_ended_at FROM careeros_pilot_access WHERE account_id=$1`, who.Account.ID).Scan(&end); err != nil || end.Valid {
		t.Fatal("suspension invented access-end clock", end, err)
	}
}

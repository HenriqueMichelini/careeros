package accountapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestProfileExportCompleteRetainedValues(t *testing.T) {
	dsn, _ := database(t)
	store := open(t, dsn, "profile_export")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, store, fakeOIDC{})
	who := login(t, svc, "export-owner")
	h := New(svc, store)
	raw, err := os.ReadFile("testdata/profile-export-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if w := api(t, h, who, "PUT", "/api/profile/current", command("export-save", 0, raw)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := api(t, h, who, "GET", "/api/profile/export", nil)
	if w.Code != 200 {
		t.Fatal("export unavailable", w.Code, w.Body.String())
	}
	expected := append([]byte(`{"format":"careeros-profile-export","version":1,"profile":`), bytes.TrimSpace(raw)...)
	expected = append(expected, '}')
	// Compact only JSON whitespace; preserve number lexemes and exact Unicode values.
	var want, got bytes.Buffer
	if json.Compact(&want, expected) != nil || json.Compact(&got, w.Body.Bytes()) != nil || !bytes.Equal(want.Bytes(), got.Bytes()) {
		t.Fatal("full retained document/provenance changed")
	}
	if w.Header().Get("Content-Disposition") != `attachment; filename="careeros-profile-v1.json"` || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("unsafe download metadata", w.Header())
	}
}

func TestProfileExportOwnershipAndRevocation(t *testing.T) {
	dsn, db := database(t)
	store := open(t, dsn, "profile_export_rights")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, store, fakeOIDC{})
	a, b := login(t, svc, "export-a"), login(t, svc, "export-b")
	h := New(svc, store)
	raw, _ := os.ReadFile("testdata/profile-export-v1.json")
	if w := api(t, h, a, "PUT", "/api/profile/current", command("save", 0, raw)); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := api(t, h, b, "GET", "/api/profile/export", nil); w.Code != 404 {
		t.Fatal("foreign empty owner", w.Code)
	}
	for _, path := range []string{"/api/profile/export?accountId=" + a.Account.ID, "/api/profile/export?profileId=perfil", "/api/profile/export/foreign"} {
		w := api(t, h, b, "GET", path, nil)
		if w.Code == 200 || bytes.Contains(w.Body.Bytes(), []byte("Érica")) {
			t.Fatal("foreign substitution", path, w.Code)
		}
	}
	req := httptest.NewRequest("GET", "/api/profile/export", nil)
	req.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: b.SessionToken})
	req.Header.Set("X-CareerOS-Account", a.Account.ID)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Fatal("cookie switch", w.Code)
	}
	if _, err := db.Exec("UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", a.Principal.SessionID); err != nil {
		t.Fatal(err)
	}
	if w := api(t, h, a, "GET", "/api/profile/export", nil); w.Code != 401 || bytes.Contains(w.Body.Bytes(), []byte("Érica")) {
		t.Fatal("expired session", w.Code)
	}
	if w := api(t, h, nil, "GET", "/api/profile/export", nil); w.Code != 401 {
		t.Fatal("missing session", w.Code)
	}
}

func TestProfileExportPilotWindowAndAlias(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "export-person")
	if _, err := store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	h := NewPilot(svc, store, nil)
	raw, _ := os.ReadFile("testdata/profile-export-v1.json")
	if w := api(t, h, who, "PUT", "/api/profile/current", command("save", 0, raw)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("UPDATE careeros_pilot_access SET expires_at=clock_timestamp()-interval '1 day' WHERE account_id=$1", who.Account.ID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/profile/export", "/.netlify/functions/account/api/profile/export"} {
		w := api(t, h, who, "GET", path, nil)
		if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("Érica")) {
			t.Fatal("retrieval window", path, w.Code, w.Body.String())
		}
	}
	if _, err = db.Exec("UPDATE careeros_pilot_access SET expires_at=clock_timestamp()-interval '30 days' WHERE account_id=$1", who.Account.ID); err != nil {
		t.Fatal(err)
	}
	if w := api(t, h, who, "GET", "/api/profile/export", nil); w.Code != 403 {
		t.Fatal("strict window end", w.Code)
	}
	if _, err = db.Exec("UPDATE careeros_pilot_access SET expires_at=clock_timestamp()+interval '1 day',state='revoked' WHERE account_id=$1", who.Account.ID); err != nil {
		t.Fatal(err)
	}
	if w := api(t, h, who, "GET", "/api/profile/export", nil); w.Code != 403 {
		t.Fatal("revoked", w.Code)
	}
	// Export has no dependency on or effect on customer/AI ledgers.
	var profiles int
	if err = db.QueryRow("SELECT count(*) FROM careeros_profiles").Scan(&profiles); err != nil || profiles != 1 {
		t.Fatal("export mutated Profile", profiles, err)
	}
}

func TestProfileExportRechecksTimeAfterBlockedDocumentRead(t *testing.T) {
	dsn, db := database(t)
	store := open(t, dsn, "profile_export_after_read")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, store, fakeOIDC{})
	who := login(t, svc, "export-wait")
	h := New(svc, store)
	if w := api(t, h, who, "PUT", "/api/profile/current", command("save", 0, fixture(t, 1))); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if _, err := db.Exec("UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '600 milliseconds' WHERE id=$1", who.Principal.SessionID); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("LOCK TABLE careeros_profiles IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- api(t, h, who, "GET", "/api/profile/export", nil) }()
	waitBlocked(t, db, "profile_export_after_read", "")
	time.Sleep(650 * time.Millisecond)
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	w := <-done
	if w.Code != 401 || w.Header().Get("Content-Disposition") != "" {
		t.Fatal("late export released", w.Code, w.Header())
	}
}

type exportWriteFence struct {
	*httptest.ResponseRecorder
	before func()
}

func (w exportWriteFence) Write(raw []byte) (int, error) {
	w.before()
	return w.ResponseRecorder.Write(raw)
}

func TestProfileExportActualAliasWriteHoldsAuthority(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "export-outer")
	if _, err := store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	h := NewPilot(svc, store, nil)
	if w := api(t, h, who, "PUT", "/api/profile/current", command("save", 0, fixture(t, 1))); w.Code != 200 {
		t.Fatal(w.Code)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	other := open(t, dsn, "profile_export_revoke_writer")
	finished := make(chan error, 1)
	w := exportWriteFence{httptest.NewRecorder(), func() {
		go func() { finished <- other.AuthStores().Sessions.Revoke(context.Background(), who.Principal.SessionID) }()
		waitBlocked(t, db, "profile_export_revoke_writer", "")
		select {
		case err := <-finished:
			t.Fatal("authority changed before outer write", err)
		default:
		}
	}}
	req := httptest.NewRequest("GET", "/.netlify/functions/account/api/profile/export", nil)
	req.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: who.SessionToken})
	req.Header.Set("X-CareerOS-Account", who.Account.ID)
	h.ServeHTTP(w, req)
	if w.Code != 200 || !json.Valid(w.Body.Bytes()) {
		t.Fatal("outer alias export", w.Code)
	}
	if err = <-finished; err != nil {
		t.Fatal(err)
	}
	if next := api(t, h, who, "GET", "/api/profile/export", nil); next.Code != 401 {
		t.Fatal("post-revoke export", next.Code)
	}
}

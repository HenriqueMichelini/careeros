package accountapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

func TestPilotExplicitActivationIsAccountBoundAndRecoverable(t *testing.T) {
	dsn, db := database(t)
	store := open(t, dsn, "pilot_a")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	sa := service(t, store, fakeOIDC{})
	operator, participant, other := login(t, sa, "operator"), login(t, sa, "participant"), login(t, sa, "other")
	if _, err := db.Exec("INSERT INTO careeros_pilot_operators(account_id) VALUES($1)", operator.Account.ID); err != nil {
		t.Fatal(err)
	}
	identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "participant"}
	issue, err := store.PilotCommand(context.Background(), operator.Principal, "issue-one", accountstore.PilotCommand{Kind: "invite", Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	h := NewPilot(sa, store, nil)
	if w := api(t, h, other, "POST", "/api/pilot/activate", []byte(`{"operationId":"activate-one","invitationId":"`+issue.InvitationID+`","confirm":true}`)); w.Code != http.StatusForbidden {
		t.Fatalf("substitution: %d %s", w.Code, w.Body.String())
	}
	if w := api(t, h, participant, "POST", "/api/pilot/activate", []byte(`{"operationId":"activate-one","invitationId":"`+issue.InvitationID+`","confirm":true}`)); w.Code != http.StatusForbidden {
		t.Fatalf("missing clearance: %d %s", w.Code, w.Body.String())
	}
	if _, err = store.PilotCommand(context.Background(), operator.Principal, "clear-one", accountstore.PilotCommand{Kind: "clear", Identity: identity}); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"operationId":"activate-one","invitationId":"` + issue.InvitationID + `","confirm":true}`)
	var first accountstore.PilotStatus
	for n := 0; n < 2; n++ {
		w := api(t, h, participant, "POST", "/api/pilot/activate", body)
		if w.Code != 200 {
			t.Fatalf("activation: %d %s", w.Code, w.Body.String())
		}
		var got accountstore.PilotStatus
		if err = json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.State != "active" || !got.CanDraft || !got.CanStandalone || !got.CanRetrieve || got.DraftAllowance != 5 {
			t.Fatalf("rights: %+v", got)
		}
		if n == 0 {
			first = got
		} else if !got.ExpiresAt.Equal(*first.ExpiresAt) {
			t.Fatal("replay extended grant")
		}
	}
	if _, err = store.PilotCommand(context.Background(), operator.Principal, "suspend-one", accountstore.PilotCommand{Kind: "suspend", Identity: identity}); err != nil {
		t.Fatal(err)
	}
	w := api(t, h, participant, "GET", "/api/pilot/status", nil)
	var status accountstore.PilotStatus
	json.Unmarshal(w.Body.Bytes(), &status)
	if w.Code != 200 || status.State != "suspended" || status.CanDraft || status.CanStandalone {
		t.Fatalf("suspended: %d %+v", w.Code, status)
	}
}

func pilotFixture(t *testing.T) (string, *accountstore.Store, *auth.Service, *auth.LoginResult) {
	t.Helper()
	dsn, db := database(t)
	store := open(t, dsn, "pilot_fixture")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	sa := service(t, store, fakeOIDC{})
	operator := login(t, sa, "operator")
	if _, err := db.Exec("INSERT INTO careeros_pilot_operators(account_id) VALUES($1)", operator.Account.ID); err != nil {
		t.Fatal(err)
	}
	return dsn, store, sa, operator
}
func pilotInvite(t *testing.T, store *accountstore.Store, sa *auth.Service, operator *auth.LoginResult, subject string) (*auth.LoginResult, accountstore.PilotStatus) {
	t.Helper()
	who := login(t, sa, subject)
	identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: subject}
	var invitation accountstore.PilotStatus
	for _, kind := range []string{"invite", "clear"} {
		result, err := store.PilotCommand(context.Background(), operator.Principal, kind+subject, accountstore.PilotCommand{Kind: kind, Identity: identity})
		if err != nil {
			t.Fatal(err)
		}
		if kind == "invite" {
			invitation = result
		}
	}
	return who, invitation
}
func TestPilotConcurrentSlotsAndDuplicateCommands(t *testing.T) {
	_, store, sa, operator := pilotFixture(t)
	people := make([]*auth.LoginResult, 6)
	invites := make([]accountstore.PilotStatus, 6)
	for n := range people {
		people[n], invites[n] = pilotInvite(t, store, sa, operator, fmt.Sprintf("person%d", n))
	}
	var wg sync.WaitGroup
	results := make(chan int, 12)
	h := NewPilot(sa, store, nil)
	for n := range people {
		for repeat := 0; repeat < 2; repeat++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				w := api(t, h, people[n], "POST", "/api/pilot/activate", []byte(fmt.Sprintf(`{"operationId":"activate","invitationId":%q,"confirm":true}`, invites[n].InvitationID)))
				results <- w.Code
			}(n)
		}
	}
	wg.Wait()
	close(results)
	success, capacity := 0, 0
	for code := range results {
		if code == 200 {
			success++
		} else if code == 409 {
			capacity++
		} else {
			t.Fatalf("unexpected code %d", code)
		}
	}
	if success != 10 || capacity != 2 {
		t.Fatalf("slot race %d successes %d denials", success, capacity)
	}
	active, unused := -1, -1
	for n := range people {
		status, err := store.PilotStatus(context.Background(), people[n].Principal)
		if err != nil {
			t.Fatal(err)
		}
		if status.CanDraft {
			active = n
		} else {
			unused = n
		}
	}
	if active < 0 || unused < 0 {
		t.Fatal("race did not assign exactly five permanent slots")
	}
	removed := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: fmt.Sprintf("person%d", active)}
	if _, err := store.PilotCommand(context.Background(), operator.Principal, "revoke-slot", accountstore.PilotCommand{Kind: "revoke", Identity: removed}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ActivatePilot(context.Background(), people[unused].Principal, "sixth-after-revocation", invites[unused].InvitationID, true); err != auth.ErrCapacity {
		t.Fatalf("revocation recycled slot: %v", err)
	}
	if _, err := store.PilotCommand(context.Background(), operator.Principal, "reset-grant", accountstore.PilotCommand{Kind: "invite", Identity: removed}); err != accountstore.ErrConflict {
		t.Fatalf("revocation reset grant: %v", err)
	}
	identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "person0"}
	if _, err := store.PilotCommand(context.Background(), operator.Principal, "inviteperson0", accountstore.PilotCommand{Kind: "revoke", Identity: identity}); err != accountstore.ErrConflict {
		t.Fatalf("changed replay: %v", err)
	}
	if _, err := store.PilotCommand(context.Background(), people[0].Principal, "unauthorized", accountstore.PilotCommand{Kind: "revoke", Identity: identity}); err != auth.ErrForbidden {
		t.Fatalf("operator substitution: %v", err)
	}
}
func TestPilotActualRouterExpiryRightsAndAliases(t *testing.T) {
	dsn, store, sa, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, sa, operator, "expiry")
	h := NewPilot(sa, store, map[string]http.Handler{"/api/profile/ingest": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })})
	if w := api(t, h, who, "PUT", "/api/profile/current", command("save-unactivated", 0, fixture(t, 1))); w.Code != 403 {
		t.Fatalf("unactivated save: %d", w.Code)
	}
	if _, err := store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	if w := api(t, h, who, "PUT", "/api/profile/current", command("save-active", 0, fixture(t, 1))); w.Code != 200 {
		t.Fatalf("active save: %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/profile/ingest", "/.netlify/functions/profile-ingestion"} {
		if w := api(t, h, who, "POST", path, []byte(`{}`)); w.Code != 204 {
			t.Fatalf("active route %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	// A fresh application instance recovers persisted grant/status and Profile.
	restarted := open(t, dsn, "pilot_restarted")
	fresh := NewPilot(service(t, restarted, fakeOIDC{}), restarted, nil)
	if w := api(t, fresh, who, "GET", "/api/pilot/status", nil); w.Code != 200 {
		t.Fatalf("restart: %d", w.Code)
	}
	db, err := storeDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("UPDATE careeros_pilot_access SET expires_at=clock_timestamp()-interval '1 day' WHERE account_id=$1", who.Account.ID); err != nil {
		t.Fatal(err)
	}
	if w := api(t, h, who, "GET", "/api/profile/current", nil); w.Code != 200 {
		t.Fatalf("retrieval window: %d", w.Code)
	}
	if w := api(t, h, who, "PUT", "/api/profile/current", command("save-expired", 1, fixture(t, 2))); w.Code != 403 {
		t.Fatalf("expired save: %d", w.Code)
	}
	for _, path := range []string{"/api/profile/ingest", "/.netlify/functions/profile-ingestion", "/api/application-draft", "/.netlify/functions/application-draft"} {
		if w := api(t, h, who, "POST", path, []byte(`{}`)); w.Code != 403 {
			t.Fatalf("expired %s: %d", path, w.Code)
		}
	}
	if _, err = db.Exec("UPDATE careeros_pilot_access SET expires_at=clock_timestamp()-interval '31 days' WHERE account_id=$1", who.Account.ID); err != nil {
		t.Fatal(err)
	}
	if w := api(t, h, who, "GET", "/.netlify/functions/account/api/profile/current", nil); w.Code != 403 {
		t.Fatalf("retrieval expired: %d", w.Code)
	}
}
func TestPilotRechecksExpiryAfterBlockingLock(t *testing.T) {
	dsn, store, sa, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, sa, operator, "lockexpiry")
	if _, err := store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	db, err := storeDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	blockedStore := open(t, dsn, "pilot_blocked")
	blockedHandler := NewPilot(service(t, blockedStore, fakeOIDC{}), blockedStore, nil)
	blocker, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err = blocker.Exec("SELECT pg_advisory_xact_lock(72481306)"); err != nil {
		t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- api(t, blockedHandler, who, "PUT", "/api/profile/current", command("save-blocked", 0, fixture(t, 1)))
	}()
	waitBlocked(t, db, "pilot_blocked", "advisory")
	if _, err = blocker.Exec("UPDATE careeros_pilot_access SET expires_at=clock_timestamp()-interval '1 second' WHERE account_id=$1", who.Account.ID); err != nil {
		t.Fatal(err)
	}
	if err = blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case w := <-done:
		if w.Code != 403 {
			t.Fatalf("postlock expiry: %d %s", w.Code, w.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked save never completed")
	}
}

func storeDB(dsn string) (*sql.DB, error) { return sql.Open("pgx", dsn) }

func TestPilotInvitationExpiryReissueAndRevocation(t *testing.T) {
	dsn, store, sa, operator := pilotFixture(t)
	who, inv := pilotInvite(t, store, sa, operator, "reissue")
	db, err := storeDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("UPDATE careeros_pilot_access SET invitation_expires_at=clock_timestamp()-interval '1 second' WHERE account_id=$1", who.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ActivatePilot(context.Background(), who.Principal, "expired", inv.InvitationID, true); err != auth.ErrForbidden {
		t.Fatalf("expired invitation: %v", err)
	}
	identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "reissue"}
	reissued, err := store.PilotCommand(context.Background(), operator.Principal, "reissue", accountstore.PilotCommand{Kind: "invite", Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	if reissued.InvitationID == inv.InvitationID {
		t.Fatal("reissue reused invitation")
	}
	if _, err = store.ActivatePilot(context.Background(), who.Principal, "old-invitation", inv.InvitationID, true); err != auth.ErrForbidden {
		t.Fatalf("old invitation: %v", err)
	}
	if reissued.CanActivate { t.Fatal("reissue retained old eligibility clearance") }
	if _,err=store.PilotCommand(context.Background(),operator.Principal,"clear-reissue",accountstore.PilotCommand{Kind:"clear",Identity:identity});err!=nil{t.Fatal(err)}
	first, err := store.ActivatePilot(context.Background(), who.Principal, "activate", reissued.InvitationID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PilotCommand(context.Background(), operator.Principal, "after-activation", accountstore.PilotCommand{Kind: "invite", Identity: identity}); err != accountstore.ErrConflict {
		t.Fatalf("extend activated grant: %v", err)
	}
	duplicate, err := store.ActivatePilot(context.Background(), who.Principal, "different-operation", reissued.InvitationID, true)
	if err != nil || !duplicate.ExpiresAt.Equal(*first.ExpiresAt) {
		t.Fatalf("second redemption extended: %+v %v", duplicate, err)
	}
	for n := 0; n < 2; n++ {
		if _, err = store.PilotCommand(context.Background(), operator.Principal, "revoke", accountstore.PilotCommand{Kind: "revoke", Identity: identity}); err != nil {
			t.Fatal(err)
		}
	}
	status, err := store.ActivatePilot(context.Background(), who.Principal, "activate", reissued.InvitationID, true)
	// #288's selected known early access-end rule preserves read/export only;
	// redemption/reissue still cannot recreate generation rights or extend time.
	if err != nil || status.CanDraft || status.CanStandalone || !status.CanRetrieve || status.RetrieveUntil == nil || !status.RetrieveUntil.Before(first.ExpiresAt.Add(30*24*time.Hour)) || status.State != "revoked" {
		t.Fatalf("revoked replay: %+v %v", status, err)
	}
	current, exists, err := store.AuthStores().Accounts.Get(context.Background(), who.Account.ID)
	if err != nil || !exists || current.Status != auth.AccountActive || current.ID != who.Account.ID {
		t.Fatal("grant revocation changed account identity")
	}
}
func TestPilotRevocationAndSessionExpiryWinBlockedSave(t *testing.T) {
	for _, kind := range []string{"revoke", "session-expiry"} {
		t.Run(kind, func(t *testing.T) {
			dsn, store, sa, operator := pilotFixture(t)
			who, invite := pilotInvite(t, store, sa, operator, "race")
			if _, err := store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true); err != nil {
				t.Fatal(err)
			}
			db, err := storeDB(dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			blocked := open(t, dsn, "pilot_revocation_wait")
			h := NewPilot(service(t, blocked, fakeOIDC{}), blocked, nil)
			blocker, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback()
			if _, err = blocker.Exec("SELECT pg_advisory_xact_lock(72481306)"); err != nil {
				t.Fatal(err)
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			raw := command("blocked", 0, fixture(t, 1))
			go func() { done <- api(t, h, who, "PUT", "/api/profile/current", raw) }()
			waitBlocked(t, db, "pilot_revocation_wait", "advisory")
			expected := 403
			if kind == "revoke" {
				if _, err = blocker.Exec("UPDATE careeros_pilot_access SET state='revoked' WHERE account_id=$1", who.Account.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				expected = 401
				if _, err = blocker.Exec("UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", who.Principal.SessionID); err != nil {
					t.Fatal(err)
				}
			}
			if err = blocker.Commit(); err != nil {
				t.Fatal(err)
			}
			select {
			case w := <-done:
				if w.Code != expected {
					t.Fatalf("postlock %s: %d %s", kind, w.Code, w.Body.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("blocked save never completed")
			}
		})
	}
}
func TestPilotInFlightFakeResultAndMissingCompositionFailClosed(t *testing.T) {
	_, store, sa, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, sa, operator, "late")
	if _, err := store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	h := NewPilot(sa, store, map[string]http.Handler{"/api/profile/ingest": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.Write([]byte("LATE_RESULT_SENTINEL"))
	})})
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- api(t, h, who, "POST", "/.netlify/functions/profile-ingestion", []byte(`{}`)) }()
	<-entered
	identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "late"}
	if _, err := store.PilotCommand(context.Background(), operator.Principal, "suspend", accountstore.PilotCommand{Kind: "suspend", Identity: identity}); err != nil {
		t.Fatal(err)
	}
	close(release)
	w := <-done
	if w.Code != 403 || strings.Contains(w.Body.String(), "LATE_RESULT_SENTINEL") {
		t.Fatalf("late result leaked: %d %s", w.Code, w.Body.String())
	}
	for _, missing := range []http.Handler{NewPilot(nil, store, nil), NewPilot(sa, nil, nil)} {
		if w = api(t, missing, who, "POST", "/api/profile/ingest", []byte(`{}`)); w.Code != 503 {
			t.Fatalf("missing dependency: %d", w.Code)
		}
	}
	if w = api(t, h, who, "POST", "/.netlify/functions/unknown", []byte(`{}`)); w.Code != 404 {
		t.Fatalf("unknown alias fallback: %d", w.Code)
	}
}

func TestPilotOperatorCommandUsesLiveRoleAndOpaqueSession(t *testing.T) {
	dsn, store, sa, operator := pilotFixture(t)
	participant := login(t, sa, "synthetic-cli-participant")
	binary := filepath.Join(t.TempDir(), "pilot-local")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/pilot-local")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build operator: %v %s", err, out)
	}
	run := func(token, action, operation string) ([]byte, error) {
		cmd := exec.Command(binary, "--synthetic-only", "--action", action, "--subject", "synthetic-cli-participant", "--operation-id", operation)
		cmd.Env = append(os.Environ(), "CAREEROS_PILOT_LOCAL_DATABASE_URL="+dsn, "CAREEROS_PILOT_LOCAL_SESSION_TOKEN="+token)
		return cmd.CombinedOutput()
	}
	if out, err := run(participant.SessionToken, "invite", "unauthorized"); err == nil {
		t.Fatalf("caller claimed operator: %s", out)
	}
	var invitation accountstore.PilotStatus
	for _, action := range []string{"invite", "clear"} {
		out, err := run(operator.SessionToken, action, action)
		if err != nil {
			t.Fatalf("operator %s: %v %s", action, err, out)
		}
		if action == "invite" {
			if err = json.Unmarshal(out, &invitation); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := store.ActivatePilot(context.Background(), participant.Principal, "activate", invitation.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	if out, err := run(operator.SessionToken, "suspend", "suspend"); err != nil {
		t.Fatalf("suspend: %v %s", err, out)
	}
	if err := sa.Logout(context.Background(), operator.SessionToken); err != nil {
		t.Fatal(err)
	}
	if out, err := run(operator.SessionToken, "revoke", "after-logout"); err == nil {
		t.Fatalf("revoked operator session accepted: %s", out)
	}
}
func TestPilotApplicationProcessRestart(t *testing.T) {
	dsn, store, sa, operator := pilotFixture(t)
	who, invitation := pilotInvite(t, store, sa, operator, "process-restart")
	if _, err := store.ActivatePilot(context.Background(), who.Principal, "activate", invitation.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PilotProfiles().SaveProfile(context.Background(), who.Principal, "save", 0, fixture(t, 1)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("CAREEROS_PILOT_RESTART_DATABASE") == "1" {
		// Explicit opt-in, fixed disposable container created for this slice.
		// Never restart an arbitrary DSN-selected database or container.
		if !strings.Contains(dsn, "127.0.0.1:55476/careeros_test") {
			t.Fatal("database restart requires the dedicated disposable fixture")
		}
		restart := exec.Command("docker", "restart", "careeros-276-postgres")
		if out, err := restart.CombinedOutput(); err != nil {
			t.Fatalf("disposable database restart: %v %s", err, out)
		}
		probe, err := storeDB(dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer probe.Close()
		deadline := time.Now().Add(10 * time.Second)
		for {
			if err = probe.Ping(); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("restarted PostgreSQL unavailable")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	child := exec.Command(os.Args[0], "-test.run=^TestPilotRestartChild$")
	child.Env = append(os.Environ(), "CAREEROS_PILOT_RESTART_DSN="+dsn, "CAREEROS_PILOT_RESTART_TOKEN="+who.SessionToken)
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatalf("new process recovery: %v %s", err, out)
	}
}
func TestPilotRestartChild(t *testing.T) {
	dsn := os.Getenv("CAREEROS_PILOT_RESTART_DSN")
	if dsn == "" {
		t.Skip("subprocess-only helper")
	}
	store := open(t, dsn, "pilot_process_restart")
	sa := service(t, store, fakeOIDC{})
	principal, err := sa.Authenticate(context.Background(), os.Getenv("CAREEROS_PILOT_RESTART_TOKEN"))
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.PilotStatus(context.Background(), principal)
	if err != nil || !status.CanDraft || status.State != "active" {
		t.Fatalf("persisted authority: %+v %v", status, err)
	}
	profile, err := store.PilotProfiles().ReadProfile(context.Background(), principal, "")
	if err != nil || string(profile.Document) == "null" {
		t.Fatalf("persisted profile: %v", err)
	}
	replay, err := store.ActivatePilot(context.Background(), principal, "activate", status.InvitationID, true)
	if err != nil || !replay.ExpiresAt.Equal(*status.ExpiresAt) {
		t.Fatalf("persisted replay: %+v %v", replay, err)
	}
}

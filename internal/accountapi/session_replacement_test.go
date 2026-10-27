package accountapi

import (
	"context"
	"crypto/sha256"
	"errors"
	"professional-information-repo/internal/auth"
	"testing"
	"time"
)

func TestPostgresSessionReplacementRejectsExpiryWhileWaiting(t *testing.T) {
	dsn, db := database(t)
	s := open(t, dsn, "careeros_review_expiry")
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, s, fakeOIDC{})
	who := login(t, svc, "review-expiry")
	if _, err := db.Exec("UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '1 second' WHERE id=$1", who.Principal.SessionID); err != nil {
		t.Fatal(err)
	}
	lock, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback()
	if _, err := lock.Exec("SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE", who.Account.ID); err != nil {
		t.Fatal(err)
	}
	beforeLock := time.Now()
	var initiallyValid bool
	if err := db.QueryRow("SELECT expires_at>clock_timestamp() AND expires_at>$2 FROM careeros_sessions WHERE id=$1", who.Principal.SessionID, beforeLock).Scan(&initiallyValid); err != nil || !initiallyValid {
		t.Fatal("initiating session must be valid when replacement starts", initiallyValid, err)
	}
	replacement := auth.Session{ID: "review-replacement", AccountID: who.Account.ID, Generation: who.Principal.Generation, TokenHash: sha256.Sum256([]byte("review-replacement-token")), CSRFHash: sha256.Sum256([]byte("review-csrf")), CreatedAt: beforeLock, ExpiresAt: beforeLock.Add(time.Hour)}
	done := make(chan error, 1)
	go func() {
		done <- s.AuthStores().Sessions.Create(context.Background(), replacement, who.Principal.SessionID, beforeLock)
	}()
	waitBlocked(t, db, "careeros_review_expiry", "")
	// Observe natural DB expiry while the replacement is actually lock-blocked.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var expired bool
		if err := db.QueryRow("SELECT expires_at <= clock_timestamp() FROM careeros_sessions WHERE id=$1", who.Principal.SessionID).Scan(&expired); err != nil {
			t.Fatal(err)
		}
		if expired {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("natural expiry not observed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	var expired bool
	if err := db.QueryRow("SELECT expires_at <= clock_timestamp() FROM careeros_sessions WHERE id=$1", who.Principal.SessionID).Scan(&expired); err != nil || !expired {
		t.Fatal("expiry positive control", expired, err)
	}
	if err := lock.Commit(); err != nil {
		t.Fatal(err)
	}
	err = <-done
	var created bool
	if e := db.QueryRow("SELECT EXISTS(SELECT 1 FROM careeros_sessions WHERE id=$1)", replacement.ID).Scan(&created); e != nil {
		t.Fatal(e)
	}
	t.Logf("initiating_session_expired_before_lock_release=%v replacement_created=%v Create_error=%v", expired, created, err)
	if !errors.Is(err, auth.ErrInvalidTransaction) || created {
		t.Fatalf("expired initiating session authorized replacement insertion: err=%v created=%v", err, created)
	}
}

func TestPostgresSessionReplacementLogoutOrderings(t *testing.T) {
	for _, order := range []string{"logout-first", "replacement-first"} {
		t.Run(order, func(t *testing.T) {
			dsn, db := database(t)
			first := open(t, dsn, "careeros_replace_order")
			if err := first.Migrate(context.Background()); err != nil {
				t.Fatal(err)
			}
			second := open(t, dsn, "careeros_logout_order")
			svc := service(t, first, fakeOIDC{})
			who := login(t, svc, "ordered-subject")
			device := login(t, svc, "ordered-subject")
			now := time.Now()
			replacement := auth.Session{ID: "ordered-replacement", AccountID: who.Account.ID, Generation: who.Principal.Generation,
				TokenHash: sha256.Sum256([]byte("ordered-token")), CSRFHash: sha256.Sum256([]byte("ordered-csrf")), CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
			lock, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			if _, err = lock.ExecContext(context.Background(), "SELECT pg_advisory_lock(72481991)"); err != nil {
				t.Fatal(err)
			}
			defer lock.ExecContext(context.Background(), "SELECT pg_advisory_unlock(72481991)")
			event := "DELETE"
			if order == "replacement-first" {
				event = "INSERT"
			}
			_, err = db.Exec(`CREATE FUNCTION pause_order() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(72481991); IF TG_OP='DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF; END $$; CREATE TRIGGER pause_order BEFORE ` + event + ` ON careeros_sessions FOR EACH ROW EXECUTE FUNCTION pause_order()`)
			if err != nil {
				t.Fatal(err)
			}
			replaceDone, logoutDone := make(chan error, 1), make(chan error, 1)
			replace := func() {
				replaceDone <- first.AuthStores().Sessions.Create(context.Background(), replacement, who.Principal.SessionID, now)
			}
			logout := func() {
				logoutDone <- second.AuthStores().Sessions.Revoke(context.Background(), who.Principal.SessionID)
			}
			if order == "logout-first" {
				go logout()
				waitBlocked(t, db, "careeros_logout_order", "advisory")
				go replace()
				waitBlocked(t, db, "careeros_replace_order", "")
			} else {
				go replace()
				waitBlocked(t, db, "careeros_replace_order", "advisory")
				go logout()
				waitBlocked(t, db, "careeros_logout_order", "")
			}
			if _, err = lock.ExecContext(context.Background(), "SELECT pg_advisory_unlock(72481991)"); err != nil {
				t.Fatal(err)
			}
			if err = <-logoutDone; err != nil {
				t.Fatal(err)
			}
			err = <-replaceDone
			var created bool
			if e := db.QueryRow("SELECT EXISTS(SELECT 1 FROM careeros_sessions WHERE id=$1)", replacement.ID).Scan(&created); e != nil {
				t.Fatal(e)
			}
			if order == "logout-first" {
				if !errors.Is(err, auth.ErrInvalidTransaction) || created {
					t.Fatal("logout must fence replacement", err, created)
				}
			} else {
				if err != nil || !created {
					t.Fatal("replacement must commit before ordinary logout", err, created)
				}
				if _, err = svc.Authenticate(context.Background(), "ordered-token"); err != nil {
					t.Fatal("replacement positive control", err)
				}
			}
			if _, err = svc.Authenticate(context.Background(), device.SessionToken); err != nil {
				t.Fatal("another device must survive", err)
			}
			account, ok, err := first.AuthStores().Accounts.Get(context.Background(), who.Account.ID)
			if err != nil || !ok || account.Generation != who.Principal.Generation {
				t.Fatal("ordinary logout changed generation", account, err)
			}
			login(t, svc, "ordered-subject") // Fresh sign-in remains permitted in both orders.
		})
	}
}

func TestPostgresValidReplacementUsesCurrentDatabaseTime(t *testing.T) {
	dsn, _ := database(t)
	s := open(t, dsn, "careeros_valid_replace")
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, s, fakeOIDC{})
	who := login(t, svc, "valid-subject")
	other := login(t, svc, "valid-subject")
	now := time.Now()
	replacement := auth.Session{ID: "valid-replacement", AccountID: who.Account.ID, Generation: who.Principal.Generation,
		TokenHash: sha256.Sum256([]byte("valid-token")), CSRFHash: sha256.Sum256([]byte("valid-csrf")), CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	// Even an incorrect caller clock cannot override current database validity.
	if err := s.AuthStores().Sessions.Create(context.Background(), replacement, who.Principal.SessionID, now.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(context.Background(), who.SessionToken); !errors.Is(err, auth.ErrAuthenticationRequired) {
		t.Fatal("initiator was not replaced", err)
	}
	for _, token := range []string{"valid-token", other.SessionToken} {
		if _, err := svc.Authenticate(context.Background(), token); err != nil {
			t.Fatal(err)
		}
	}
	login(t, svc, "valid-subject")
}

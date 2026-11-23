package accountapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/testsupport/aiadmission"
)

type rollingInspection struct {
	accountstore.OperationsRiskSource
	accountstore.OperationsRiskBinding
	sample accountstore.OperationsRiskSample
}

func (r rollingInspection) OperationsSample(context.Context) (accountstore.OperationsRiskSample, error) {
	return r.sample, nil
}

func TestPilotOperationsManualRollingRecoveryKeepsSeparateCrossings(t *testing.T) {
	f, h, _, _ := qualificationFixture(t)
	j, ops, notices, _ := pilotOperationsFixture(t, f, h, time.Now)
	ctx := context.Background()
	start := time.Now().UTC().Add(-4 * time.Second)
	sample := func(offset time.Duration, amount, sequence int64) accountstore.OperationsRiskSample {
		at := start.Add(offset)
		return accountstore.OperationsRiskSample{At: at, Namespace: "qualification", Complete: true, DurableSequence: sequence,
			Caps: []accountstore.OperationsCap{{Key: "fixture-rolling", Scope: "account_rolling24h", Policy: "fixture-policy",
				Currency: "USD", Counted: amount, Limit: 100}}, Figures: accountstore.OperationsFigures{At: at, Currency: "USD", Counted: amount}}
	}
	if err := ops.ObserveCommittedRisk(ctx, sample(0, 80, 1)); err != nil {
		t.Fatal(err)
	}
	source := h.LocalOperationsSource()
	manual := sample(2*time.Second, 79, 0)
	inspector, err := f.Store.LocalOperations(accountstore.OperationsConfig{Owner: "Henrique", Build: "fixture-rolling-clock"},
		rollingInspection{OperationsRiskSource: source, OperationsRiskBinding: source.(accountstore.OperationsRiskBinding), sample: manual}, j)
	if err != nil {
		t.Fatal(err)
	}
	if err := inspector.Inspect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ops.ObserveCommittedRisk(ctx, sample(time.Second, 81, 2)); err != nil {
		t.Fatal(err)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || status.Figures == nil || !status.Figures.At.Equal(manual.At) || len(*notices) != 1 {
		t.Fatalf("late capture altered the display or split the old warning: %+v %v %+v", status, err, *notices)
	}
	if err := ops.ObserveCommittedRisk(ctx, sample(3*time.Second, 80, 3)); err != nil {
		t.Fatal(err)
	}
	if len(*notices) != 2 || (*notices)[0].Kind != "spend_warning" || (*notices)[1].Kind != "spend_warning" || (*notices)[0].ID == (*notices)[1].ID {
		t.Fatalf("new crossing after complete manual recovery reused the old warning: %+v", *notices)
	}
}

func waitForOperationsControlLease(t *testing.T, db *sql.DB, mode string, granted bool) {
	t.Helper()
	waitForOperationsLease(t, db, 72481312, mode, granted)
}

func waitForOperationsLease(t *testing.T, db *sql.DB, key int, mode string, granted bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var found bool
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory'
		 AND classid=0 AND objid=$1 AND mode=$2 AND granted=$3)`, key, mode, granted).Scan(&found); err != nil {
			t.Fatal(err)
		}
		if found {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("control lease was not observed: %s granted=%v", mode, granted)
}

func TestPilotOperationsCaptureGuardRechecksManagedSessionBeforeCommit(t *testing.T) {
	for _, stage := range []string{"reserve", "intent"} {
		t.Run(stage, func(t *testing.T) {
			f, h, provider, router := qualificationFixture(t)
			_, _, _, _ = pilotOperationsFixture(t, f, h, time.Now)
			body := f.Body(t, "/api/qualification-gaps")
			ctx := context.Background()
			blocker, err := f.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback()
			h.SetFault("before_"+stage+"_commit", func() error {
				_, err := blocker.Exec(`SELECT pg_advisory_xact_lock(72481324)`)
				return err
			})
			var expiry time.Time
			if err = f.DB.QueryRow(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '2 seconds'
			 WHERE id=$1 RETURNING expires_at`, f.Principal.SessionID).Scan(&expiry); err != nil {
				t.Fatal(err)
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				done <- qualificationRequest(t, f, router, "/api/qualification-gaps", "fixture-session-expires-behind-capture", "", body)
			}()
			waitForOperationsLease(t, f.DB, 72481324, "ExclusiveLock", false)
			time.Sleep(time.Until(expiry) + 50*time.Millisecond)
			if err = blocker.Rollback(); err != nil {
				t.Fatal(err)
			}
			response := <-done
			if response.Code == http.StatusOK || len(provider.Calls) != 0 {
				t.Fatalf("expired Session advanced to a fake send after %s: %d", stage, response.Code)
			}
			var total, intended int
			if err = f.DB.QueryRow(`SELECT count(*),count(*) FILTER(WHERE intent_at IS NOT NULL) FROM ai_fixture_dispatch`).Scan(&total, &intended); err != nil {
				t.Fatal(err)
			}
			want := 0
			if stage == "intent" {
				want = 1
			}
			if total != want || intended != 0 {
				t.Fatalf("expired Session committed the %s transition: total=%d intent=%d", stage, total, intended)
			}
		})
	}
}

func TestPilotOperationsInvitationAndActivationRecheckSessionAfterJournalWait(t *testing.T) {
	for _, action := range []string{"invite", "activate"} {
		t.Run(action, func(t *testing.T) {
			f := aiadmission.NewFixture(t)
			ctx := context.Background()
			gate := aicontrol.NewSyntheticGate()
			gate.Reconcile("qualification", 1)
			cfg := aicontrol.QualificationSyntheticConfig("qualification", gate, managedDraftFullBound)
			cfg.ExpiresAt = time.Now().Add(time.Hour)
			h, err := aicontrol.NewSynthetic(ctx, f.DSN, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
			if _, err = f.DB.Exec(`INSERT INTO careeros_pilot_operators(account_id) VALUES($1)`, f.Principal.AccountID); err != nil {
				t.Fatal(err)
			}
			_, _, _, directory := pilotOperationsFixture(t, f, h, time.Now)
			identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "fixture-account"}
			var invitation string
			if action == "activate" {
				issued, err := f.Store.PilotCommand(ctx, f.Principal, "fixture-prepared-invitation", accountstore.PilotCommand{Kind: "invite", Identity: identity})
				if err != nil {
					t.Fatal(err)
				}
				invitation = issued.InvitationID
				if _, err = f.Store.PilotCommand(ctx, f.Principal, "fixture-prepared-clearance", accountstore.PilotCommand{Kind: "clear", Identity: identity}); err != nil {
					t.Fatal(err)
				}
			}
			lock, err := os.OpenFile(filepath.Join(directory, "journal.lock"), os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
				t.Fatal(err)
			}
			var expiry time.Time
			if err = f.DB.QueryRow(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '2 seconds'
			 WHERE id=$1 RETURNING expires_at`, f.Principal.SessionID).Scan(&expiry); err != nil {
				t.Fatal(err)
			}
			const operation = "fixture-expire-behind-journal"
			finished := make(chan error, 1)
			go func() {
				if action == "invite" {
					_, err := f.Store.PilotCommand(ctx, f.Principal, operation, accountstore.PilotCommand{Kind: "invite", Identity: identity})
					finished <- err
					return
				}
				_, err := f.Store.ActivatePilot(ctx, f.Principal, operation, invitation, true)
				finished <- err
			}()
			waitForOperationsControlLease(t, f.DB, "ShareLock", true)
			select {
			case err := <-finished:
				t.Fatalf("operation bypassed the held journal lease: %v", err)
			case <-time.After(time.Until(expiry) + 50*time.Millisecond):
			}
			if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
				t.Fatal(err)
			}
			if err = <-finished; !errors.Is(err, auth.ErrAuthenticationRequired) && !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("expired Session was allowed to finish %s: %v", action, err)
			}
			var changed bool
			if err = f.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM careeros_pilot_commands WHERE operation_id=$1)
			 OR EXISTS(SELECT 1 FROM careeros_pilot_access WHERE activated_at IS NOT NULL)`, operation).Scan(&changed); err != nil || changed {
				t.Fatalf("operation recorded a protected transition: %v %v", changed, err)
			}
		})
	}
}

func TestPilotOperationsEmergencySQLStopOrdersAfterTheActualAdmissionLease(t *testing.T) {
	f, h, _, _ := qualificationFixture(t)
	_, ops, _, directory := pilotOperationsFixture(t, f, h, time.Now)
	ctx := context.Background()
	arrived, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	h.SetFault("before_operations_action_commit", func() error { close(arrived); <-release; return nil })
	const operation = "fixture-emergency-ordered-after-admission"
	body := f.Body(t, "/api/qualification-gaps")
	admitted := make(chan error, 1)
	go func() {
		_, err := h.Open(ctx, f.Principal, operation, "qualification_gaps", body)
		admitted <- err
	}()
	awaitOperationsPhase(t, arrived)
	stopped := make(chan error, 1)
	go func() { stopped <- ops.ObserveFault(ctx, accountstore.OperationsControlFault) }()
	waitForOperationsControlLease(t, f.DB, "ExclusiveLock", false)
	marker, err := os.ReadFile(filepath.Join(directory, "journal.fence"))
	if err != nil || !bytes.Equal(marker, []byte{0, 0}) {
		t.Fatalf("timed-out writer changed the latch outside the existing lease: %v %v", marker, err)
	}
	if _, err = os.Stat(filepath.Join(directory, "journal.uncertain")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("timed-out writer published uncertainty outside the existing lease: %v", err)
	}
	if _, err = os.Stat(filepath.Join(directory, "journal.queued")); err != nil {
		t.Fatalf("timed-out writer did not durably deny new lease decisions: %v", err)
	}
	if unlock, err := ops.HoldProtected(ctx, "inference"); !errors.Is(err, auth.ErrUnavailable) {
		if unlock != nil {
			unlock()
		}
		t.Fatalf("a later lease decision crossed the durable queue: %v", err)
	}
	close(release)
	if err = <-admitted; err != nil {
		t.Fatalf("previously leased admission did not commit: %v", err)
	}
	if err = <-stopped; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("operation did not report independent storage failure: %v", err)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || status.Emergency == nil {
		t.Fatalf("ordered SQL fallback did not record its emergency receipt: %+v %v", status, err)
	}
	h.SetFault("before_operations_action_commit", nil)
	if _, err = h.Open(ctx, f.Principal, "fixture-after-ordered-emergency", "qualification_gaps", body); err == nil {
		t.Fatal("new admission passed the committed emergency stop")
	}
}

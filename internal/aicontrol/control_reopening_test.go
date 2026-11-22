package aicontrol

import (
	"context"
	"database/sql"
	"net/url"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/aidispatch"
	"testing"
	"time"

	"professional-information-repo/internal/accountstore"
)

func stopForControlRecovery(t *testing.T, f *fixture) (*SyntheticControlRecovery, accountstore.ControlCommand) {
	t.Helper()
	ctx := context.Background()
	stopped, err := f.store.LocalControls(nil).Apply(ctx, f.p, "stop-for-recovery", accountstore.ControlCommand{Control: "inference", Stop: true, ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	authority, err := NewSyntheticControlRecovery(ctx, f.store, f.h)
	if err != nil {
		t.Fatal(err)
	}
	return authority, accountstore.ControlCommand{Control: "inference", ExpectedVersion: stopped.Version}
}

func TestOperatorControlReopenDoesNotReviveOldActionPermit(t *testing.T) {
	f := setup(t, nil)
	old := f.open(t, "old-unsent-action")
	authority, command := stopForControlRecovery(t, f)
	ref, err := authority.Prepare(context.Background(), f.p, command)
	if err != nil {
		t.Fatal(err)
	}
	command.RecoveryReference = ref
	if _, err = f.store.LocalControls(authority).Apply(context.Background(), f.p, "open-with-current-proof", command); err != nil {
		t.Fatal(err)
	}
	response, err := send(old)
	closeResp(response)
	if err == nil || f.calls.Load() != 0 {
		t.Fatalf("old Action permit revived after reopening: %v calls=%d", err, f.calls.Load())
	}
	if scope := f.scope(t); scope.Counted != 0 || scope.Occupied != 0 {
		t.Fatalf("old permit created new exposure: %+v", scope)
	}
	fresh := f.open(t, "explicit-fresh-action")
	response, err = send(fresh)
	closeResp(response)
	if err != nil || f.calls.Load() != 1 {
		t.Fatalf("fresh explicit authority unavailable: %v calls=%d", err, f.calls.Load())
	}
}

func TestOperatorControlRecoveryRejectsChangedIndependentGateway(t *testing.T) {
	f := setup(t, nil)
	authority, command := stopForControlRecovery(t, f)
	ref, err := authority.Prepare(context.Background(), f.p, command)
	if err != nil {
		t.Fatal(err)
	}
	command.RecoveryReference = ref
	f.cfg.Gate.Close(f.cfg.Namespace)
	f.cfg.Gate.Reconcile(f.cfg.Namespace, 1)
	if _, err = f.store.LocalControls(authority).Apply(context.Background(), f.p, "old-gateway-checkpoint", command); err == nil {
		t.Fatal("same-epoch gateway replacement revived stale recovery proof")
	}
}

func TestOperatorControlGenerationRowAloneDoesNotProveExclusion(t *testing.T) {
	f := setup(t, nil)
	authority, command := stopForControlRecovery(t, f)
	if _, err := f.db.Exec(`UPDATE ai_fixture_namespace SET epoch=epoch+1 WHERE id=$1`, f.cfg.Namespace); err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Prepare(context.Background(), f.p, command); err == nil {
		t.Fatal("a database generation bump substituted for independent old-system exclusion")
	}
}

func TestOperatorControlWholeGatewayDrainPrecedesGenerationAndOpening(t *testing.T) {
	f := setup(t, nil)
	old := f.open(t, "old-before-whole-exclusion")
	_, finish, err := old.BeginResult(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	authority, command := stopForControlRecovery(t, f)
	f.cfg.Gate.Close(f.cfg.Namespace)
	if _, err = authority.Prepare(context.Background(), f.p, command); err == nil {
		t.Fatal("an underlying result callback had not actually drained")
	}
	finish()
	ref, err := authority.Prepare(context.Background(), f.p, command)
	if err != nil {
		t.Fatal(err)
	}
	command.RecoveryReference = ref
	if err = authority.Complete(context.Background(), f.p, ref); err == nil {
		t.Fatal("independent gateway opened before durable generation advance")
	}
	if _, err = f.store.LocalControls(authority).Apply(context.Background(), f.p, "advance-after-verified-exclusion", command); err != nil {
		t.Fatal(err)
	}
	var system, namespace int64
	if err = f.db.QueryRow(`SELECT system_generation FROM careeros_control_state WHERE singleton`).Scan(&system); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(`SELECT epoch FROM ai_fixture_namespace WHERE id=$1`, f.cfg.Namespace).Scan(&namespace); err != nil {
		t.Fatal(err)
	}
	if system != 2 || namespace != 2 {
		t.Fatalf("expected verified generation2, got system%d namespace%d", system, namespace)
	}
	if _, err = f.h.Open(context.Background(), f.p, "still-closed-independent-gate", "cv_generation", []byte(`{"synthetic":"root"}`)); err == nil {
		t.Fatal("database opening silently opened the independent gateway")
	}
	if err = authority.Complete(context.Background(), f.p, ref); err != nil {
		t.Fatal(err)
	}
	if err = old.ResultAllowed(context.Background()); err == nil {
		t.Fatal("old protected result lease revived after generation change")
	}
	fresh := f.open(t, "fresh-after-whole-exclusion")
	response, err := send(fresh)
	closeResp(response)
	if err != nil || f.calls.Load() != 1 {
		t.Fatalf("fresh new work after verified sequence: %v calls%d", err, f.calls.Load())
	}
	if err = authority.Complete(context.Background(), f.p, ref); err != nil {
		t.Fatalf("duplicate completion: %v", err)
	}
	if err = fresh.ResultAllowed(context.Background()); err != nil {
		t.Fatal("duplicate completion invalidated current new authority")
	}
}

func TestOperatorControlSelfCompromiseNeedsFreshRestrictedOperator(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	stopped, err := f.store.LocalControls(nil).Apply(ctx, f.p, "operator-stops-own-access", accountstore.ControlCommand{Control: "compromise", Stop: true, AccountID: f.p.AccountID, ExpectedVersion: 1})
	if err != nil || !stopped.Compromised {
		t.Fatalf("authorized self-compromise stop: %+v %v", stopped, err)
	}
	if _, err = f.store.LocalControls(nil).Status(ctx, f.p, f.p.AccountID); err == nil {
		t.Fatal("revoked old operator session remained usable")
	}
	start, err := f.svc.BeginLogin(ctx, "google", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(start.URL)
	fresh, err := f.svc.CompleteLogin(ctx, "google", u.Query().Get("state"), "fixture-owner", start.BindingToken)
	if err != nil {
		t.Fatal(err)
	}
	if string(fresh.Account.Status) != "compromised" {
		t.Fatal("fresh identity was not restricted")
	}
	authority, err := NewSyntheticControlRecovery(ctx, f.store, f.h)
	if err != nil {
		t.Fatal(err)
	}
	command := accountstore.ControlCommand{Control: "compromise", AccountID: f.p.AccountID, ExpectedVersion: stopped.Version}
	ref, err := authority.Prepare(ctx, fresh.Principal, command)
	if err != nil {
		t.Fatal(err)
	}
	command.RecoveryReference = ref
	if _, err = f.store.LocalControls(authority).Apply(ctx, fresh.Principal, "fresh-operator-approved-reopen", command); err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.AuthenticateDeletionStatus(ctx, fresh.SessionToken); err == nil {
		t.Fatal("restricted operator token was promoted to normal access")
	}
	normal, _ := login(t, f.svc, "fixture-owner")
	if status, err := f.store.LocalControls(nil).Status(ctx, normal, normal.AccountID); err != nil || status.Compromised {
		t.Fatalf("fresh normal operator status: %+v %v", status, err)
	}
}

func TestOperatorControlNewIndependentFenceWinsBeforeGatewayOpening(t *testing.T) {
	f := setup(t, nil)
	authority, command := stopForControlRecovery(t, f)
	f.cfg.Gate.Close(f.cfg.Namespace)
	ref, err := authority.Prepare(context.Background(), f.p, command)
	if err != nil {
		t.Fatal(err)
	}
	command.RecoveryReference = ref
	if _, err = f.store.LocalControls(authority).Apply(context.Background(), f.p, "advance-before-new-fence", command); err != nil {
		t.Fatal(err)
	}
	f.h.SetFault("before_control_gateway_open", func() error { f.cfg.Gate.Close(f.cfg.Namespace); return nil })
	if err = authority.Complete(context.Background(), f.p, ref); err == nil {
		t.Fatal("old checkpoint reopened a newer independent gateway fence")
	}
	if _, err = f.h.Open(context.Background(), f.p, "newer-fence-remains-closed", "cv_generation", []byte(`{"synthetic":"root"}`)); err == nil {
		t.Fatal("newer independent fence was lost")
	}
}

func TestOperatorControlSingleConnectionDispatchLease(t *testing.T) {
	f := setup(t, nil)
	f.h.db.SetMaxOpenConns(1)
	action := f.open(t, "single-connection-dispatch")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ctx, _ = aidiagnostics.Start(aidispatch.WithWorkflowRoot(ctx, action.workflow, action.rootInput), "generation", "cv-policy-v2/schema-v2")
	response, err := action.Send(ctx, "openai", req())
	closeResp(response)
	if err != nil || f.calls.Load() != 1 {
		t.Fatalf("leased final validation: %v calls%d", err, f.calls.Load())
	}
}

func TestOperatorControlRestrictedRolesCannotMutateAuthority(t *testing.T) {
	f := setup(t, nil)
	action := f.open(t, "restricted-controls")
	for _, db := range []*sql.DB{f.h.db, f.h.reviewerDB} {
		for _, query := range []string{`SELECT * FROM careeros_control_state`, `SELECT * FROM careeros_control_operators`, `SELECT * FROM careeros_control_commands`, `SELECT * FROM careeros_control_accounts`, `SELECT * FROM careeros_control_recovery`, `UPDATE careeros_control_state SET inference_stopped=false`, `INSERT INTO careeros_control_operators(account_id) SELECT id FROM careeros_accounts LIMIT 1`} {
			if rows, err := db.Query(query); err == nil {
				rows.Close()
				t.Fatalf("restricted pool accessed control authority: %s", query)
			}
		}
	}
	if _, err := f.h.db.Exec(`UPDATE ai_fixture_action SET control_epoch=control_epoch+1 WHERE id=$1`, action.ID()); err == nil {
		t.Fatal("runtime forged current inference epoch")
	}
	var allowed bool
	if err := f.h.db.QueryRow(`SELECT careeros_inference_allowed()`).Scan(&allowed); err != nil || !allowed {
		t.Fatal("narrow predicate unavailable", err)
	}
	if _, err := f.store.LocalControls(nil).Apply(context.Background(), f.p, "stop-predicate", accountstore.ControlCommand{Control: "inference", Stop: true, ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if err := f.h.db.QueryRow(`SELECT careeros_inference_allowed()`).Scan(&allowed); err != nil || allowed {
		t.Fatal("narrow predicate ignored stop", err)
	}
}

func TestOperatorControlStopAndCompromiseDoNotSuppressIndependentEvidence(t *testing.T) {
	f := setup(t, nil)
	action := f.open(t, "late-obligation")
	d := f.sent(t, action)
	operator := reconciliationOperator(t, f)
	if _, err := f.store.LocalControls(nil).Apply(context.Background(), f.p, "stop-before-late-evidence", accountstore.ControlCommand{Control: "inference", Stop: true, ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.LocalControls(nil).Apply(context.Background(), f.p, "compromise-before-late-evidence", accountstore.ControlCommand{Control: "compromise", Stop: true, ExpectedVersion: 2, AccountID: f.p.AccountID}); err != nil {
		t.Fatal(err)
	}
	command := reconciliationUsage(d, "late-under-closed-access")
	result := applyReconciliation(t, f.h.Reconciliation(), operator, command)
	if result.After != 20 {
		t.Fatal(result)
	}
	higher := command
	higher.ID = "late-above-bound"
	higher.Kind = "bill"
	higher.ExpectedVersion = 1
	higher.Evidence.Source = "late-confirmed-bill"
	higher.Evidence.Usage = nil
	higher.Evidence.Gross = pointer(120)
	higher.Evidence.Difference = pointer(0)
	result = applyReconciliation(t, f.h.Reconciliation(), operator, higher)
	state, err := f.h.Reconciliation().Status(context.Background(), operator, d.ID)
	if err != nil || result.After != 120 || !state.AdmissionStopped || state.Remote != "possibly_active" {
		t.Fatalf("late containment: %+v %+v %v", result, state, err)
	}
	var counted int64
	var occupied int
	if err = f.db.QueryRow(`SELECT counted,occupied FROM ai_fixture_scope WHERE namespace=$1 AND scope='global'`, f.cfg.Namespace).Scan(&counted, &occupied); err != nil || counted != 120 || occupied != 1 {
		t.Fatalf("late numeric evidence lost: %d/%d %v", counted, occupied, err)
	}
}

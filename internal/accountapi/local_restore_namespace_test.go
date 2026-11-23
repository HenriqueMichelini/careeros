package accountapi_test

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/testsupport/aiadmission"
)

const restoreUnpricedCommand = "restore-reviewed-unpriced"

type restoreNamespaceDecision struct {
	command aicontrol.ReconciliationCommand
	rows    string
}

func recordRestoreNamespaceStop(t *testing.T, db *sql.DB, h *aicontrol.Harness, reviewer auth.Principal, dispatch string) restoreNamespaceDecision {
	t.Helper()
	ctx := context.Background()
	gross := int64(900)
	cmd := aicontrol.ReconciliationCommand{
		ID: restoreUnpricedCommand, Kind: "unpriced", Dispatch: dispatch,
		Evidence: aicontrol.ReconciliationEvidence{
			Source: "restore-reviewed-report", Revision: 1, Reason: "fixture-unpriced", Gross: &gross, Complete: true,
			Provider: "typesafe", Model: "jev-1.13.0", Tier: "standard", Rate: "unpriced-fixture-rate",
			Currency: "EUR", BillingNamespace: "synthetic", RequestID: "restore-reviewed-request",
		},
	}
	control := h.Reconciliation()
	state, err := control.Status(ctx, reviewer, dispatch)
	if err != nil || state.Version != 0 || state.Counted != 1344 || state.Maximum != 1344 || state.AdmissionStopped {
		t.Fatal("source review must begin from an unresolved original reservation", state, err)
	}
	preview, err := control.Preview(ctx, reviewer, cmd)
	if err != nil || preview.Token == "" || preview.Before != 1344 || preview.After != 1344 || !preview.Unpriced {
		t.Fatal("unpriced preview must preserve the original expense", preview, err)
	}
	cmd.Preview = preview.Token
	verifyRestoreUnpricedApply(t, control, reviewer, cmd)
	return restoreNamespaceDecision{command: cmd, rows: observeRestoreNamespaceRows(t, db, dispatch)}
}

func verifyRestoreUnpricedApply(t *testing.T, control *aicontrol.Reconciliation, reviewer auth.Principal, command aicontrol.ReconciliationCommand) {
	t.Helper()
	ctx := context.Background()
	applied, err := control.Apply(ctx, reviewer, command)
	if err != nil || applied.Version != 1 || applied.Before != 1344 || applied.After != 1344 || !applied.Unpriced {
		t.Fatal("reviewed unpriced command did not retain the original one-use outcome", applied, err)
	}
	status, err := control.Status(ctx, reviewer, command.Dispatch)
	if err != nil || status.Version != 1 || !status.Unpriced || status.Expense != nil || status.Counted != 1344 ||
		status.Maximum != 1344 || status.Remote != "possibly_active" || !status.AdmissionStopped {
		t.Fatal("reviewed unpriced dispatch did not retain its stop and unresolved exposure", status, err)
	}
	for _, scope := range status.Scopes {
		if scope.Name == "global" && scope.Counted == 1344 && scope.Occupied == 1 {
			return
		}
	}
	t.Fatal("reviewed unpriced decision changed the global expense or occupancy", status.Scopes)
}

func observeRestoreNamespaceRows(t *testing.T, db *sql.DB, dispatch string) string {
	t.Helper()
	var rows string
	err := db.QueryRow(`SELECT jsonb_build_array(to_jsonb(c),to_jsonb(e),to_jsonb(s))::text
		FROM ai_recon_command c JOIN ai_recon_event e USING(namespace,id)
		JOIN ai_recon_stop s ON s.namespace=c.namespace AND s.event_id=e.id
		WHERE c.namespace='qualification' AND c.id=$1 AND c.dispatch=$2 AND e.dispatch=$2
		AND c.kind='unpriced' AND c.source='restore-reviewed-report' AND c.source_revision=1 AND c.version=1
		AND c.before_amount=1344 AND c.after_amount=1344 AND c.detail->>'gross'='900'
		AND e.contract='typesafe/jev-1.13.0/standard/unpriced-fixture-rate/EUR'
		AND e.obligation=1344 AND e.reserved=1344 AND e.owner_review_required
		AND s.stopped AND s.version=1 AND s.contract=e.contract AND s.obligation=1344 AND s.reserved=1344`,
		restoreUnpricedCommand, dispatch).Scan(&rows)
	if err != nil {
		t.Fatal("the independent qualification command, owner review event and active stop must all exist", err)
	}
	return rows
}

func checkRestoreNamespaceAbsent(t *testing.T, db *sql.DB) {
	t.Helper()
	var later bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM ai_recon_command WHERE id=$1)
		OR EXISTS(SELECT 1 FROM ai_recon_event WHERE id=$1)
		OR EXISTS(SELECT 1 FROM ai_recon_stop WHERE stopped)`, restoreUnpricedCommand).Scan(&later)
	if err != nil || later {
		t.Fatal("archive already contains later namespace containment", err)
	}
}

func checkRestoreNamespaceRefusal(t *testing.T, db *sql.DB, runtime restoreRuntime, reviewer auth.Principal,
	fixture *aiadmission.Fixture, generation int64, decision restoreNamespaceDecision, calls *atomic.Int32) {
	t.Helper()
	ctx := context.Background()
	if actual := observeRestoreNamespaceRows(t, db, decision.command.Dispatch); actual != decision.rows {
		t.Fatal("restore changed exact content-free reconciliation command, event or stop evidence")
	}
	verifyRestoreUnpricedApply(t, runtime.qualification.Reconciliation(), reviewer, decision.command)
	if afterReplay := observeRestoreNamespaceRows(t, db, decision.command.Dispatch); afterReplay != decision.rows {
		t.Fatal("reviewed command replay rewrote or duplicated source authority")
	}
	status, err := runtime.account.LocalControls(nil).Status(ctx, reviewer, "")
	if err != nil || status.SystemGeneration != generation || !status.InferenceStopped || !status.InvitationStopped {
		t.Fatal("failed branch lost its whole-system stops", status, err)
	}
	recovery, err := aicontrol.NewSyntheticControlRecovery(ctx, runtime.account, runtime.qualification, runtime.draft)
	if err != nil {
		t.Fatal(err)
	}
	command := accountstore.ControlCommand{Control: "inference", ExpectedVersion: status.Version}
	if reference, err := recovery.Prepare(ctx, reviewer, command); reference != "" || !errors.Is(err, auth.ErrForbidden) {
		t.Fatal("whole-system generation proof accepted a contained namespace", err)
	}
	checkRestoreQuarantine(t, fixture, runtime.http, db, generation, "retained namespace stop refusal")
	fresh := restoreAccount(fixture, controlFreshIdentity(t, runtime.service, "fixture-account"))
	http := accountapi.NewSyntheticQualification(runtime.service, runtime.account, runtime.qualification)
	restoreDenied(t, "fresh qualification request after refused generation advance",
		qualificationRequest(t, fresh, http, "/api/qualification-gaps", "restore-denied-after-namespace-replay", "", fresh.Body(t, "/api/qualification-gaps")))
	if calls.Load() != 0 {
		t.Fatal("retained namespace stop allowed fake egress")
	}
	t.Logf("restore observation later reviewed namespace stop: counted and reserved=1344 microUSD, occupancy=1, owner review and command retained; generation stays %d under quarantine, zero additional fake sends", generation)
}

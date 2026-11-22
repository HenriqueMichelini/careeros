package accountapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

func TestOperatorControlInvitationStopPreservesExistingGrant(t *testing.T) {
	_, store, service, operator := pilotFixture(t)
	ctx := context.Background()
	identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "operator"}
	if err := store.BootstrapLocalControlOperator(ctx, identity); err != nil {
		t.Fatal(err)
	}
	controls := store.LocalControls(nil)
	initial, err := controls.InitializeSynthetic(ctx, operator.Principal, "fresh-control-fixture")
	if err != nil {
		t.Fatal(err)
	}
	active, invitation := pilotInvite(t, store, service, operator, "control-active")
	if _, err = store.ActivatePilot(ctx, active.Principal, "activate-control-active", invitation.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	pending, pendingInvitation := pilotInvite(t, store, service, operator, "control-pending")
	command := accountstore.ControlCommand{Control: "invitations", Stop: true, ExpectedVersion: initial.Version}
	stopped, err := controls.Apply(ctx, operator.Principal, "stop-control-invitations", command)
	if err != nil || !stopped.InvitationStopped || stopped.InferenceStopped {
		t.Fatalf("separate invitation stop: %+v %v", stopped, err)
	}
	duplicate, err := controls.Apply(ctx, operator.Principal, "stop-control-invitations", command)
	if err != nil || !reflect.DeepEqual(duplicate, stopped) {
		t.Fatalf("lost command acknowledgment: %+v %v", duplicate, err)
	}
	changed := command
	changed.Control = "inference"
	if _, err = controls.Apply(ctx, operator.Principal, "stop-control-invitations", changed); err != accountstore.ErrConflict {
		t.Fatalf("changed operator command: %v", err)
	}
	if _, err = controls.Apply(ctx, active.Principal, "not-the-control-operator", changed); err != auth.ErrForbidden {
		t.Fatalf("grant owner cannot operate controls: %v", err)
	}
	h := NewPilot(service, store, nil)
	body, _ := json.Marshal(map[string]any{"operationId": "activate-control-pending", "invitationId": pendingInvitation.InvitationID, "confirm": true})
	for _, path := range []string{"/api/pilot/activate", "/.netlify/functions/account/api/pilot/activate"} {
		if response := api(t, h, pending, "POST", path, body); response.Code != 403 {
			t.Fatalf("stopped activation %s: %d", path, response.Code)
		}
	}
	if _, err = store.PilotCommand(ctx, operator.Principal, "new-invite-after-stop", accountstore.PilotCommand{Kind: "invite", Identity: identity}); err != auth.ErrForbidden {
		t.Fatalf("stopped issuance: %v", err)
	}
	status, err := store.PilotStatus(ctx, active.Principal)
	if err != nil || !status.CanStandalone || !status.CanRetrieve {
		t.Fatalf("independently active grant survives: %+v %v", status, err)
	}
	status, err = store.PilotStatus(ctx, pending.Principal)
	if err != nil || status.CanActivate {
		t.Fatalf("pending status consumes stop: %+v %v", status, err)
	}
}

func TestOperatorControlCompromiseDeniesUnactivatedReissue(t *testing.T) {
	_, store, service, operator := pilotFixture(t)
	ctx := context.Background()
	if err := store.BootstrapLocalControlOperator(ctx, auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "operator"}); err != nil {
		t.Fatal(err)
	}
	controls := store.LocalControls(nil)
	if _, err := controls.InitializeSynthetic(ctx, operator.Principal, "fresh-control-fixture"); err != nil {
		t.Fatal(err)
	}
	pending, _ := pilotInvite(t, store, service, operator, "compromised-before-activation")
	if _, err := controls.Apply(ctx, operator.Principal, "stop-unactivated-account", accountstore.ControlCommand{Control: "compromise", AccountID: pending.Account.ID, Stop: true, ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PilotCommand(ctx, operator.Principal, "reissue-while-compromised", accountstore.PilotCommand{Kind: "invite", Identity: auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "compromised-before-activation"}}); err != auth.ErrForbidden {
		t.Fatalf("affected account reissue while compromised: %v", err)
	}
}

func TestOperatorControlFreshInitializationRejectsExistingAdmission(t *testing.T) {
	dsn, _ := database(t)
	store := open(t, dsn, "fresh_initialization_negative")
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	operator := login(t, service(t, store, fakeOIDC{}), "operator")
	if err := store.BootstrapLocalControlOperator(ctx, auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "operator"}); err != nil {
		t.Fatal(err)
	}
	// An older managed schema may exist before these control tables. Its
	// presence is not evidence of a fresh, empty authority installation.
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE ai_fixture_action(id text PRIMARY KEY);INSERT INTO ai_fixture_action(id) VALUES('older-admission')`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.LocalControls(nil).InitializeSynthetic(ctx, operator.Principal, "not-a-fresh-installation"); err != auth.ErrForbidden {
		t.Fatalf("older admission must require verified recovery: %v", err)
	}
}

func TestOperatorControlCLIUsesDistinctRoleAndRecoversReceipt(t *testing.T) {
	dsn, store, service, operator := pilotFixture(t)
	participant := login(t, service, "synthetic-cli-participant")
	binary := filepath.Join(t.TempDir(), "control-local")
	build := exec.Command("rtk", "proxy", "go", "build", "-o", binary, "../../cmd/control-local")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	run := func(token string, args ...string) ([]byte, error) {
		cmd := exec.Command("rtk", append([]string{"proxy", binary, "--synthetic-only"}, args...)...)
		cmd.Env = append(os.Environ(), "CAREEROS_CONTROL_LOCAL_DATABASE_URL="+dsn, "CAREEROS_CONTROL_LOCAL_SESSION_TOKEN="+token)
		return cmd.CombinedOutput()
	}
	if out, err := run(participant.SessionToken, "--action", "status"); err == nil {
		t.Fatalf("participant elevated: %s", out)
	}
	args := []string{"--action", "command", "--control", "invitations", "--stop", "--version", "1", "--operation-id", "cli-stop"}
	first, err := run(operator.SessionToken, args...)
	if err != nil {
		t.Fatalf("stop: %v %s", err, first)
	}
	again, err := run(operator.SessionToken, args...)
	if err != nil || !bytes.Equal(first, again) {
		t.Fatalf("receipt recovery %s %v", again, err)
	}
	if out, err := run(operator.SessionToken, "--action", "command", "--control", "invitations", "--version", "2", "--operation-id", "label-is-not-recovery", "--recovery-reference", "arbitrary-label"); err == nil {
		t.Fatalf("CLI label reopened: %s", out)
	}
	if bytes.Contains(first, []byte(operator.SessionToken)) || bytes.Contains(first, []byte(dsn)) {
		t.Fatal("secret in command result")
	}
	if err := service.Logout(context.Background(), operator.SessionToken); err != nil {
		t.Fatal(err)
	}
	if _, err := run(operator.SessionToken, "--action", "status"); err == nil {
		t.Fatal("revoked session recovered command authority")
	}
	_ = store
}

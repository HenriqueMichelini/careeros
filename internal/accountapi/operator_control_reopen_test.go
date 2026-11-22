package accountapi_test

import (
	"context"
	"testing"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
)

func TestOperatorControlReopenNeedsVerifiedRecoveryNotLabel(t *testing.T) {
	f, h, _, _ := qualificationFixture(t)
	ctx := context.Background()
	controls := f.Store.LocalControls(nil)
	stopped, err := controls.Apply(ctx, f.ControlOperator.Principal, "stop-before-recovery", accountstore.ControlCommand{Control: "inference", Stop: true, ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	command := accountstore.ControlCommand{Control: "inference", ExpectedVersion: stopped.Version, RecoveryReference: "unproved-checkpoint-label"}
	if _, err = controls.Apply(ctx, f.ControlOperator.Principal, "unproved-reopening", command); err != auth.ErrForbidden {
		t.Fatalf("a label is not verified recovery: %v", err)
	}
	authority, err := aicontrol.NewSyntheticControlRecovery(ctx, f.Store, h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.LocalControls(authority).Apply(ctx, f.ControlOperator.Principal, "wrong-label-reopening", command); err != auth.ErrForbidden {
		t.Fatalf("unknown independent reference: %v", err)
	}
	reference, err := authority.Prepare(ctx, f.ControlOperator.Principal, command)
	if err != nil {
		t.Fatal(err)
	}
	command.RecoveryReference = reference
	opened, err := f.Store.LocalControls(authority).Apply(ctx, f.ControlOperator.Principal, "verified-reopening", command)
	if err != nil || opened.InferenceStopped || opened.InvitationStopped {
		t.Fatalf("verified local reopening: %+v %v", opened, err)
	}
	if _, err = controls.Apply(ctx, f.ControlOperator.Principal, "verified-reopening", command); err != nil {
		t.Fatalf("lost acknowledgment must recover unchanged receipt without reconstructing proof: %v", err)
	}
	if _, err = f.Store.LocalControls(authority).Apply(ctx, f.ControlOperator.Principal, "reused-old-proof", command); err == nil {
		t.Fatal("old recovery checkpoint became a new authorization")
	}
}

func TestOperatorControlCompromiseReopenPreservesIndependentGrantHold(t *testing.T) {
	for _, held := range []bool{false, true} {
		t.Run(map[bool]string{false: "own_hold_only", true: "independent_suspension"}[held], func(t *testing.T) {
			f, h, provider, router := qualificationFixture(t)
			ctx := context.Background()
			body := f.Body(t, "/api/qualification-gaps")
			if _, err := h.Open(ctx, f.Principal, "old-terminal-on-reopen", "qualification_gaps", body); err != nil {
				t.Fatal(err)
			}
			before, err := f.Store.PilotStatus(ctx, f.Principal)
			if err != nil {
				t.Fatal(err)
			}
			if held {
				if _, err = f.Store.PilotCommand(ctx, f.Principal, "independent-pause", accountstore.PilotCommand{Kind: "suspend", Identity: auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "fixture-account"}}); err != nil {
					t.Fatal(err)
				}
			}
			stopped, err := f.Store.LocalControls(nil).Apply(ctx, f.ControlOperator.Principal, "compromise-before-reopen", accountstore.ControlCommand{Control: "compromise", Stop: true, AccountID: f.Principal.AccountID, ExpectedVersion: 1})
			if err != nil {
				t.Fatal(err)
			}
			restricted := controlFreshIdentity(t, f.Service, "fixture-account")
			authority, err := aicontrol.NewSyntheticControlRecovery(ctx, f.Store, h)
			if err != nil {
				t.Fatal(err)
			}
			command := accountstore.ControlCommand{Control: "compromise", AccountID: f.Principal.AccountID, ExpectedVersion: stopped.Version}
			ref, err := authority.Prepare(ctx, f.ControlOperator.Principal, command)
			if err != nil {
				t.Fatal(err)
			}
			command.RecoveryReference = ref
			if _, err = f.Store.LocalControls(authority).Apply(ctx, f.ControlOperator.Principal, "verified-compromise-reopen", command); err != nil {
				t.Fatal(err)
			}
			if _, err = f.Service.Authenticate(ctx, f.Login.SessionToken); err == nil {
				t.Fatal("reopening revived revoked ordinary session")
			}
			if _, err = f.Service.AuthenticateDeletionStatus(ctx, restricted.SessionToken); err == nil {
				t.Fatal("restricted token was promoted across reopening")
			}
			fresh := controlFreshIdentity(t, f.Service, "fixture-account")
			after, err := f.Store.PilotStatus(ctx, fresh.Principal)
			if err != nil {
				t.Fatal(err)
			}
			if after.CanRetrieve == held || after.CanStandalone == held || after.DraftAllowance != before.DraftAllowance || !after.ExpiresAt.Equal(*before.ExpiresAt) {
				t.Fatalf("independent rights changed: held=%v before=%+v after=%+v", held, before, after)
			}
			var state string
			if err = f.DB.QueryRow(`SELECT state FROM ai_fixture_action WHERE owner=$1 AND creation_key='old-terminal-on-reopen'`, f.Principal.AccountID).Scan(&state); err != nil || state != "canceled" {
				t.Fatalf("terminal Action revived: %q %v", state, err)
			}
			if !held {
				copy := *f
				copy.Login = fresh
				copy.Principal = fresh.Principal
				response := qualificationRequest(t, &copy, router, "/api/qualification-gaps", "old-terminal-on-reopen", "", body)
				if response.Code == 200 || len(provider.Calls) != 0 {
					t.Fatalf("terminal replay sent after reopening: %d calls%d", response.Code, len(provider.Calls))
				}
			}
		})
	}
}

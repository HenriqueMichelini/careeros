package accountapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/draftrecovery"
	"testing"
)

func TestLifecycleMixedDraftCleanupFindsContentWithoutCurrentProfile(t *testing.T) {
	x := newOpeningFixture(t)
	delivered := x.create(t, "lifecycle-delivered")
	receipt := x.receipt(t, "lifecycle-delivered")
	if w := x.confirm(t, "lifecycle-delivered", receipt); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	pending := x.create(t, "lifecycle-pending")
	if _, err := x.f.DB.Exec(`UPDATE careeros_draft_actions SET admitted_at=clock_timestamp()-interval '31 minutes',deadline=clock_timestamp()-interval '1 minute' WHERE action_id=$1`, pending.Action.ID); err != nil {
		t.Fatal(err)
	}
	worker, _ := accountstore.NewLifecycleWorker(x.f.Store, nil)
	if _, err := worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	var content int
	if err := x.f.DB.QueryRow(`SELECT count(*) FROM careeros_draft_content WHERE action_id=$1`, pending.Action.ID).Scan(&content); err != nil || content != 0 {
		t.Fatal("terminal content survived without GET", content, err)
	}
	if err := x.f.DB.QueryRow(`SELECT count(*) FROM careeros_draft_content WHERE action_id=$1`, delivered.Action.ID).Scan(&content); err != nil || content != 1 {
		t.Fatal("prior delivery erased early", content, err)
	}
	var effect bool
	x.f.DB.QueryRow(`SELECT customer_effect FROM ai_fixture_action WHERE id=$1`, delivered.Action.ID).Scan(&effect)
	if !effect {
		t.Fatal("delivered effect reversed")
	}
	if _, err := x.f.DB.Exec(`DELETE FROM careeros_profiles WHERE account_id=$1`, x.f.Principal.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := x.f.DB.Exec(`UPDATE careeros_pilot_access SET activated_at=clock_timestamp()-interval '45 days',expires_at=clock_timestamp()-interval '31 days' WHERE account_id=$1`, x.f.Principal.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if err := x.f.DB.QueryRow(`SELECT count(*) FROM careeros_draft_content WHERE action_id=$1`, delivered.Action.ID).Scan(&content); err != nil || content != 0 {
		t.Fatal("no-current-Profile delivered content missed", content, err)
	}
	x.f.DB.QueryRow(`SELECT customer_effect FROM ai_fixture_action WHERE id=$1`, delivered.Action.ID).Scan(&effect)
	if !effect {
		t.Fatal("expiry refunded delivery")
	}
	w := x.request(t, http.MethodGet, "/api/drafts/lifecycle-delivered", "", nil)
	var result draftrecovery.Result
	if w.Code != 410 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Bundle != nil {
		t.Fatal("late result access", w.Code, w.Body.String())
	}
	if x.generation.Load() != 2 || x.check.Load() != 2 {
		t.Fatal("cleanup caused inference", x.generation.Load(), x.check.Load())
	}
}

func TestLifecycleInvitationDenialAlsoFencesSharedManagedAdmission(t *testing.T) {
	f, h, _, _, _, _ := managedDraftFixture(t)
	control, err := h.Open(context.Background(), f.Principal, "valid-before-invitation-denial", "qualification_gaps", f.Body(t, "/api/qualification-gaps"))
	if err != nil {
		t.Fatal("positive managed-admission control", err)
	}
	if err = control.FinishQualification(context.Background(), true); err != nil {
		t.Fatal("known pre-dispatch local failure control", err)
	}
	if _, err := f.DB.Exec(`UPDATE careeros_pilot_access SET invitation_id='old-cleaned-invitation' WHERE account_id=$1`, f.Principal.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.Exec(`INSERT INTO careeros_invitation_cleanup SELECT account_id,invitation_id,expires_at,clock_timestamp()-interval '1 day',clock_timestamp() FROM careeros_pilot_access WHERE account_id=$1`, f.Principal.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Open(context.Background(), f.Principal, "denied-invitation-action", "qualification_gaps", f.Body(t, "/api/qualification-gaps")); !errors.Is(err, aicontrol.ErrDenied) {
		t.Fatal("shared admission trusted restored active row over invitation denial", err)
	}
}

func TestLifecycleTerminalCleanupDoesNotReleaseUnknownProviderHold(t *testing.T) {
	f, h, _, _, router, input := terminalDraftFixture(t)
	calls := 0
	if err := h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
		calls++
		return nil, false, errors.New("synthetic unknown provider")
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(input)
	w := qualificationRequest(t, f, router, "/api/drafts", "lifecycle-unknown", "", body)
	if w.Code < 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	var action string
	if err := f.DB.QueryRow(`SELECT action_id FROM careeros_draft_actions WHERE owner=$1 AND creation_key='lifecycle-unknown'`, f.Principal.AccountID).Scan(&action); err != nil {
		t.Fatal(err)
	}
	before, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || before.Counted != 1344 || before.Occupied != 1 {
		t.Fatal("unknown floor", before, err)
	}
	if _, err = f.DB.Exec(`UPDATE careeros_draft_actions SET deadline=clock_timestamp()-interval '1 second' WHERE action_id=$1`, action); err != nil {
		t.Fatal(err)
	}
	worker, _ := accountstore.NewLifecycleWorker(f.Store, nil)
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	after, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || after.Counted != before.Counted || after.Occupied != before.Occupied {
		t.Fatal("cleanup released unknown remote/expense", before, after, err)
	}
	replay := qualificationRequest(t, f, router, "/api/drafts", "lifecycle-unknown", "", body)
	if replay.Code != 410 || calls != 1 {
		t.Fatal("closed content/replay caused send", replay.Code, replay.Body.String(), calls)
	}
}

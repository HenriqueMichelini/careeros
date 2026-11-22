package aicontrol

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"professional-information-repo/internal/auth"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRestrictedReconciliationUsagePreviewReplay(t *testing.T) {
	f := setup(t, func(c *SyntheticConfig) { c.Limit = 10000 })
	s := f.open(t, "recon_usage")
	d := f.sent(t, s)
	operator, _ := login(t, f.svc, "synthetic-reconciliation-owner")
	if err := InstallLocalReconciliation(context.Background(), f.db, f.cfg.Namespace, operator.AccountID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	control := f.h.Reconciliation()
	command := ReconciliationCommand{ID: "usage_1", Kind: "usage", Dispatch: d.ID, ExpectedVersion: 0, Evidence: ReconciliationEvidence{
		Source: "fixture_usage_1", Revision: 1, Reason: "normal_settlement", Provider: "openai", Model: "gpt-6-luna", Tier: "standard", Rate: "synthetic-unit-v1", Currency: "SYN", BillingNamespace: "synthetic", RequestID: "request_1", Complete: true,
		Usage: &Usage{Input: pointer(10), Output: pointer(3)}, Difference: pointer(7),
	}}
	preview, err := control.Preview(context.Background(), operator, command)
	if err != nil || preview.After != 20 || preview.Before != 60 || preview.Version != 0 {
		t.Fatal(preview, err)
	}
	command.Preview = preview.Token
	for i := 0; i < 2; i++ {
		result, err := control.Apply(context.Background(), operator, command)
		if err != nil || result.After != 20 {
			t.Fatal(result, err)
		}
	}
	state, err := control.Status(context.Background(), operator, d.ID)
	if err != nil || state.Counted != 20 || state.Remote != "possibly_active" || state.Version != 1 || state.Category != "supported_estimate" {
		t.Fatal(state, err)
	}
	for _, scope := range []string{"global", "provider:openai", "account:" + f.p.AccountID} {
		got, err := f.h.ScopeStatus(context.Background(), f.p, scope)
		if err != nil || got.Counted != 20 || got.Occupied != 1 {
			t.Fatal(scope, got, err)
		}
	}
	if f.status(t, s).CustomerEffect {
		t.Fatal("financial settlement changed customer allowance")
	}
	command.Evidence.Difference = pointer(8)
	if _, err = control.Apply(context.Background(), operator, command); !errors.Is(err, ErrConflict) {
		t.Fatal("changed replay accepted", err)
	}
	if _, err = control.Status(context.Background(), f.p, d.ID); err == nil {
		t.Fatal("ordinary runtime owner assumed operator authority")
	}
}

func TestRestrictedReconciliationCLIAndLostAck(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "cli")
	d := f.sent(t, s)
	operator, token := login(t, f.svc, "synthetic-reconciliation-owner")
	if err := InstallLocalReconciliation(context.Background(), f.db, f.cfg.Namespace, operator.AccountID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var role string
	if err := f.h.reviewerDB.QueryRow(`SELECT current_user`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(f.dsn)
	u.User = url.User(role)
	command := ReconciliationCommand{ID: "cli_usage", Kind: "usage", Dispatch: d.ID, Evidence: ReconciliationEvidence{Source: "fixture_usage", Revision: 1, Reason: "normal_settlement", Provider: "openai", Model: "gpt-6-luna", Tier: "standard", Rate: "synthetic-unit-v1", Currency: "SYN", BillingNamespace: "synthetic", RequestID: "request_1", Complete: true, Usage: &Usage{Input: pointer(10), Output: pointer(3)}, Difference: pointer(7)}}
	cli := func(mode string, c ReconciliationCommand) ReconciliationPreview {
		t.Helper()
		raw, _ := json.Marshal(c)
		child := exec.Command("rtk", "proxy", "go", "run", "../../cmd/reconciliation-local", "--synthetic-only", "--namespace", f.cfg.Namespace, "--mode", mode)
		child.Env = append(os.Environ(), "CAREEROS_RECONCILIATION_LOCAL_DATABASE_URL="+u.String(), "CAREEROS_RECONCILIATION_LOCAL_SESSION_TOKEN="+token)
		child.Stdin = bytes.NewReader(raw)
		out, err := child.CombinedOutput()
		if err != nil {
			t.Fatalf("CLI %s failed: %v %s", mode, err, out)
		}
		var result ReconciliationPreview
		if json.Unmarshal(out, &result) != nil {
			t.Fatalf("invalid bounded CLI output: %s", out)
		}
		return result
	}
	preview := cli("preview", command)
	command.Preview = preview.Token
	first := cli("apply", command)
	second := cli("apply", command)
	if first.After != 20 || !reflect.DeepEqual(first, second) {
		t.Fatal(first, second)
	}
	// A lost acknowledgement after commit recovers exactly the same recorded result.
	command.ID = "lost_ack"
	command.ExpectedVersion = 1
	command.Evidence.Source = "fixture_bill"
	command.Kind = "bill"
	command.Evidence.Gross = pointer(20)
	command.Evidence.Usage = nil
	command.Evidence.Difference = pointer(0)
	preview, err := f.h.Reconciliation().Preview(context.Background(), operator, command)
	if err != nil {
		t.Fatal(err)
	}
	command.Preview = preview.Token
	f.h.SetFault("after_reconciliation_commit", func() error { return errors.New("synthetic lost acknowledgement") })
	lost, err := f.h.Reconciliation().Apply(context.Background(), operator, command)
	if err == nil {
		t.Fatal("fault missing")
	}
	f.h.SetFault("after_reconciliation_commit", nil)
	recovered, err := f.h.Reconciliation().Apply(context.Background(), operator, command)
	if err != nil || !reflect.DeepEqual(lost, recovered) {
		t.Fatal(lost, recovered, err)
	}
	for _, db := range []*sql.DB{f.h.db, f.h.reviewerDB} {
		if _, err := db.Exec(`UPDATE ai_recon_operator SET revoked=true`); err == nil {
			t.Fatal("runtime/reconciliation elevated itself")
		}
	}
	if _, err := f.h.db.Exec(`UPDATE ai_fixture_namespace SET epoch=epoch+1`); err == nil {
		t.Fatal("row-lock grant enabled namespace mutation")
	}
	if _, err := f.h.db.Exec(`UPDATE ai_fixture_namespace SET retired=true`); err == nil {
		t.Fatal("runtime retired namespace")
	}
}

func reconciliationOperator(t *testing.T, f *fixture) auth.Principal {
	t.Helper()
	p, _ := login(t, f.svc, "synthetic-reconciliation-owner")
	if err := InstallLocalReconciliation(context.Background(), f.db, f.cfg.Namespace, p.AccountID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	return p
}
func reconciliationUsage(d Dispatch, id string) ReconciliationCommand {
	return ReconciliationCommand{ID: id, Kind: "usage", Dispatch: d.ID, Evidence: ReconciliationEvidence{Source: id, Revision: 1, Reason: "normal_settlement", Provider: d.Provider, Model: "gpt-6-luna", Tier: "standard", Rate: "synthetic-unit-v1", Currency: "SYN", BillingNamespace: "synthetic", RequestID: "request_1", Complete: true, Usage: &Usage{Input: pointer(10), Output: pointer(3)}, Difference: pointer(7)}}
}
func applyReconciliation(t *testing.T, r *Reconciliation, p auth.Principal, c ReconciliationCommand) ReconciliationPreview {
	t.Helper()
	preview, err := r.Preview(context.Background(), p, c)
	if err != nil {
		t.Fatal(preview, err)
	}
	c.Preview = preview.Token
	result, err := r.Apply(context.Background(), p, c)
	if err != nil {
		t.Fatal(result, err)
	}
	return result
}
func TestRestrictedReconciliationCorrectionAndEvidenceIdentity(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "correction")
	d := f.sent(t, s)
	operator := reconciliationOperator(t, f)
	r := f.h.Reconciliation()
	c := reconciliationUsage(d, "usage")
	applyReconciliation(t, r, operator, c)
	bill := c
	bill.ID = "bill"
	bill.Kind = "bill"
	bill.ExpectedVersion = 1
	bill.Evidence.Source = "gross_bill"
	bill.Evidence.Supersedes = "usage"
	bill.Evidence.Usage = nil
	bill.Evidence.Gross = pointer(15)
	bill.Evidence.Difference = pointer(0)
	applyReconciliation(t, r, operator, bill)
	correction := bill
	correction.ID = "correction"
	correction.Kind = "corrected_bill"
	correction.ExpectedVersion = 2
	correction.Evidence.Revision = 2
	correction.Evidence.Gross = pointer(10)
	correction.Evidence.Supersedes = "bill"
	applyReconciliation(t, r, operator, correction)
	// Re-labeling an immutable source revision cannot change its effect, even
	// with a fresh command ID and current expected-version preview.
	conflicting := correction
	conflicting.ID = "same_revision_changed"
	conflicting.ExpectedVersion = 3
	conflicting.Evidence.Gross = pointer(11)
	if _, err := r.Preview(context.Background(), operator, conflicting); !errors.Is(err, ErrConflict) {
		t.Fatal("same source revision changed", err)
	}
	blind := correction
	blind.ID = "blind_local_version"
	blind.ExpectedVersion = 3
	blind.Evidence.Revision = 999
	blind.Evidence.Supersedes = ""
	blind.Evidence.Gross = pointer(1)
	if _, err := r.Preview(context.Background(), operator, blind); err == nil {
		t.Fatal("local version authorized downward correction")
	}
	replay := correction
	replay.ID = "duplicate_source"
	replay.ExpectedVersion = 3
	same := applyReconciliation(t, r, operator, replay)
	if same.After != 10 || same.Version != 3 {
		t.Fatal(same)
	}
	status, err := r.Status(context.Background(), operator, d.ID)
	if err != nil || status.Counted != 10 || status.Remote != "possibly_active" {
		t.Fatal(status, err)
	}
}
func TestRestrictedReconciliationBreachContainsButLateEvidenceSurvives(t *testing.T) {
	f := setup(t, func(c *SyntheticConfig) { c.Limit = 10000 })
	s := f.open(t, "breach_recon")
	d := f.sent(t, s)
	operator := reconciliationOperator(t, f)
	r := f.h.Reconciliation()
	// Customer grant loss and terminal customer state cannot erase a provider debt.
	if err := f.h.SyntheticReviewer().SetGrantState(context.Background(), f.p, time.Now().Add(time.Hour), true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE ai_fixture_action SET state='closed' WHERE id=$1`, s.ID()); err != nil {
		t.Fatal(err)
	}
	bill := reconciliationUsage(d, "over_max")
	bill.Kind = "bill"
	bill.Evidence.Usage = nil
	bill.Evidence.Gross = pointer(120)
	bill.Evidence.Difference = pointer(0)
	applyReconciliation(t, r, operator, bill)
	status, err := r.Status(context.Background(), operator, d.ID)
	if err != nil || !status.AdmissionStopped || status.Counted != 120 || status.Remote != "possibly_active" {
		t.Fatal(status, err)
	}
	if _, err = f.h.Open(context.Background(), f.p, "after_breach", "cv_generation", []byte(`{"synthetic":"root"}`)); err == nil {
		t.Fatal("breach reopened admission")
	}
	f.cfg.Gate.Close(f.cfg.Namespace)
	late := bill
	late.ID = "late_higher"
	late.ExpectedVersion = 1
	late.Evidence.Source = "late_gross_bill"
	late.Evidence.Gross = pointer(140)
	applyReconciliation(t, r, operator, late)
	status, err = r.Status(context.Background(), operator, d.ID)
	if err != nil || status.Counted != 140 || !status.AdmissionStopped {
		t.Fatal(status, err)
	}
	for _, scope := range status.Scopes {
		if scope.Counted != 140 || scope.Occupied != 1 {
			t.Fatal(scope)
		}
	}
	events, err := r.Events(context.Background(), operator)
	if err != nil || len(events) != 2 || !events[0].OwnerReviewRequired {
		t.Fatal(events, err)
	}
	signaled := 0
	if err = r.Signal(context.Background(), operator, func(event ReconciliationEvent) error {
		signaled++
		if !event.OwnerReviewRequired || event.Obligation <= event.Reserved {
			t.Fatal(event)
		}
		return errors.New("synthetic sink unavailable")
	}); err == nil || signaled != 1 {
		t.Fatal("fake signal missing", err, signaled)
	}
	state, err := r.Status(context.Background(), operator, d.ID)
	if err != nil || !state.AdmissionStopped {
		t.Fatal("failed notification reopened admission", state, err)
	}

}

func TestRestrictedReconciliationRuntimeCoverageIsSeparate(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "runtime_settle")
	f.h.SetResponder(func(_ context.Context, _ string, _ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Request-Id": []string{"request_1"}}, Body: io.NopCloser(strings.NewReader(`{"model":"gpt-6-luna","service_tier":"standard","usage":{"prompt_tokens":10,"completion_tokens":3}}`))}, nil
	})
	resp, err := send(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	closeResp(resp)
	d := f.status(t, s).Dispatches[0]
	if err := f.h.SettleSavedObservation(context.Background(), f.p, d.ID); err == nil {
		t.Fatal("unverified coverage settled")
	}
	f.h.SetSyntheticUsageCoverage(func(contract BillingContract, o ProviderObservation) (UnaccountedBound, error) {
		if contract.Provider != "openai" || contract.Currency != "SYN" || o.RequestID != "request_1" {
			return UnaccountedBound{}, ErrDenied
		}
		return UnaccountedBound{Amount: 7, Reference: "fixture_other_charges_bound"}, nil
	})
	for i := 0; i < 2; i++ {
		if err := f.h.SettleSavedObservation(context.Background(), f.p, d.ID); err != nil {
			t.Fatal(err)
		}
	}
	status := f.status(t, s)
	if status.Dispatches[0].Counted != 20 || status.Dispatches[0].Expense == nil || *status.Dispatches[0].Expense != 13 || f.scope(t).Occupied != 1 {
		t.Fatal(status)
	}
	other, _ := login(t, f.svc, "another_runtime_account")
	if err := f.h.SettleSavedObservation(context.Background(), other, d.ID); err == nil {
		t.Fatal("runtime reconciled another account")
	}
}

func TestRestrictedReconciliationReviewedCompactionAndHigherEvidence(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "detail_retirement")
	d := f.sent(t, s)
	operator := reconciliationOperator(t, f)
	r := f.h.Reconciliation()
	closure := reconciliationUsage(d, "maximum_review")
	closure.Kind = "maximum"
	closure.Evidence.Usage = nil
	closure.Evidence.Difference = nil
	f.clock.set(d.IntentAt.Add(168*time.Hour-time.Nanosecond), true)
	if _, err := r.Preview(context.Background(), operator, closure); err == nil {
		t.Fatal("early financial closure")
	}
	f.clock.set(d.IntentAt.Add(168*time.Hour), true)
	applyReconciliation(t, r, operator, closure)
	compact := reconciliationUsage(d, "detail_review")
	compact.Kind = "compact"
	compact.ExpectedVersion = 1
	compact.Evidence.Usage = nil
	compact.Evidence.Difference = nil
	compact.Retention = &DetailRetirementReview{Reference: "review_all_purposes"}
	if _, err := r.Preview(context.Background(), operator, compact); err == nil {
		t.Fatal("168h became detail TTL")
	}
	f.clock.set(d.IntentAt.Add(30*24*time.Hour), true)
	next := d.IntentAt.Add(37 * 24 * time.Hour)
	compact.Retention.Needed = []DetailPurpose{{Field: "request_id", Purpose: "capacity"}}
	compact.Retention.NextReview = &next
	applyReconciliation(t, r, operator, compact)
	state, err := r.Status(context.Background(), operator, d.ID)
	if err != nil || !state.Compacted || state.Counted != 60 || state.Remote != "possibly_active" || state.NextReview == nil || !state.NextReview.Equal(next) {
		t.Fatal(state, err)
	}
	// A reviewed detail retirement never drops the obligation or evidence markers.
	higher := reconciliationUsage(d, "late_after_compact")
	higher.ExpectedVersion = 2
	higher.Kind = "bill"
	higher.Evidence.Usage = nil
	higher.Evidence.Gross = pointer(90)
	higher.Evidence.Difference = pointer(0)
	applied := applyReconciliation(t, r, operator, higher)
	again, err := r.Apply(context.Background(), operator, func() ReconciliationCommand { p := higher; p.Preview = "lost_response_token"; return p }())
	if err != nil || !reflect.DeepEqual(applied, again) {
		t.Fatal(applied, again, err)
	}
	lower := higher
	lower.ID = "late_lower"
	lower.Kind = "corrected_bill"
	lower.ExpectedVersion = 3
	lower.Evidence.Revision = 2
	lower.Evidence.Supersedes = "late_after_compact"
	lower.Evidence.Gross = pointer(1)
	if _, err = r.Preview(context.Background(), operator, lower); err == nil {
		t.Fatal("retired attribution authorized downward claim")
	}
	state, err = r.Status(context.Background(), operator, d.ID)
	if err != nil || state.Counted != 90 || !state.AdmissionStopped || state.Remote != "possibly_active" {
		t.Fatal(state, err)
	}
}

func TestRestrictedReconciliationInputKeyRotationKeepsOldAction(t *testing.T) {
	f := setup(t, func(c *SyntheticConfig) { c.Limit = 10000; c.Capacity = 3 })
	old := f.open(t, "old_key")
	d := f.sent(t, old)
	operator := reconciliationOperator(t, f)
	cfg := f.cfg
	cfg.BindingKeyVersion = "input_v2"
	cfg.BindingKey = sha256.Sum256([]byte("synthetic-next-input-key"))
	cfg.BindingVerificationKeys = map[string][32]byte{"input_v1": f.cfg.BindingKey}
	rotated, err := NewSynthetic(context.Background(), f.dsn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rotated.Close()
	rotated.SetResponder(func(context.Context, string, *http.Request) (*http.Response, error) {
		t.Error("old durable intent resent under rotated key")
		return nil, ErrDenied
	})
	replay, err := rotated.Open(context.Background(), f.p, "old_key", "cv_generation", []byte(`{"synthetic":"root"}`))
	if err != nil || replay.ID() != old.ID() {
		t.Fatal(replay, err)
	}
	resp, err := send(replay)
	closeResp(resp)
	if !errors.Is(err, ErrOutputUnavailable) || f.calls.Load() != 1 {
		t.Fatal(err, f.calls.Load())
	}
	fresh, err := rotated.Open(context.Background(), f.p, "new_key", "cv_generation", []byte(`{"synthetic":"root"}`))
	if err != nil || fresh.ID() == old.ID() {
		t.Fatal(fresh, err)
	}
	bad := cfg
	bad.BindingVerificationKeys = nil
	if h, err := NewSynthetic(context.Background(), f.dsn, bad); err == nil {
		h.Close()
		t.Fatal("discarded live old verification key")
	}
	// Finance/replay commands have an independent audit binding and do not use
	// whichever input key happened to be current during operator invocation.
	command := reconciliationUsage(d, "audit_after_rotation")
	applyReconciliation(t, rotated.Reconciliation(), operator, command)
	if _, err := rotated.Reconciliation().RetireInputKey(context.Background(), operator, "input_v1", "arbitrary_checkpoint"); err == nil {
		t.Fatal("operator label destroyed live input verifier")
	}
}

func TestRestrictedReconciliationTerminalProofIsIndependent(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "terminal_authority")
	d := f.sent(t, s)
	operator := reconciliationOperator(t, f)
	r := f.h.Reconciliation()
	maximum := reconciliationUsage(d, "reviewed_maximum")
	maximum.Kind = "maximum"
	maximum.Evidence.Usage = nil
	maximum.Evidence.Difference = nil
	f.clock.set(d.IntentAt.Add(168*time.Hour), true)
	applyReconciliation(t, r, operator, maximum)
	terminal := reconciliationUsage(d, "terminal_proof_command")
	terminal.Kind = "terminal"
	terminal.ExpectedVersion = 1
	terminal.Evidence.Source = "code_owned_fake_proof"
	terminal.Evidence.Usage = nil
	terminal.Evidence.Difference = nil
	terminal.Evidence.Complete = false
	if _, err := r.Preview(context.Background(), operator, terminal); err == nil {
		t.Fatal("operator terminal assertion released remote work")
	}
	if err := f.h.IssueCodeOwnedFakeTerminal(context.Background(), f.h.reviewerDB, d.ID, "code_owned_fake_proof", "request_1"); err == nil {
		t.Fatal("operator installed its own terminal proof")
	}
	if err := f.h.IssueCodeOwnedFakeTerminal(context.Background(), f.db, d.ID, "code_owned_fake_proof", "request_1"); err != nil {
		t.Fatal(err)
	}
	wrong := terminal
	wrong.Evidence.RequestID = "different_request"
	if _, err := r.Preview(context.Background(), operator, wrong); err == nil {
		t.Fatal("proof attributed to wrong provider request")
	}
	result := applyReconciliation(t, r, operator, terminal)
	terminal.Preview = "lost_ack"
	again, err := r.Apply(context.Background(), operator, terminal)
	if err != nil || !reflect.DeepEqual(result, again) {
		t.Fatal(result, again, err)
	}
	status, err := r.Status(context.Background(), operator, d.ID)
	if err != nil || status.Remote != "terminal_proven" || status.Counted != 60 {
		t.Fatal(status, err)
	}
	for _, scope := range status.Scopes {
		if scope.Occupied != 0 || scope.Counted != 60 {
			t.Fatal(scope)
		}
	}
}

func TestRestrictedReconciliationUnpricedEvidenceContainsWithoutFX(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "unpriced")
	d := f.sent(t, s)
	operator := reconciliationOperator(t, f)
	r := f.h.Reconciliation()
	malformed := reconciliationUsage(d, "malformed_currency")
	malformed.Evidence.Currency = "EUR"
	if _, err := r.Preview(context.Background(), operator, malformed); err == nil {
		t.Fatal("malformed normal command repriced old dispatch")
	}
	state, err := r.Status(context.Background(), operator, d.ID)
	if err != nil || state.AdmissionStopped {
		t.Fatal(state, err)
	}
	evidence := reconciliationUsage(d, "attributable_eur_bill")
	evidence.Kind = "unpriced"
	evidence.Evidence.Currency = "EUR"
	evidence.Evidence.Rate = "external_unvalued_revision"
	evidence.Evidence.Usage = nil
	evidence.Evidence.Difference = nil
	evidence.Evidence.Gross = pointer(900)
	preview, err := r.Preview(context.Background(), operator, evidence)
	if err != nil || preview.After != 60 {
		t.Fatal("attributable unpriced evidence must be retained without conversion", preview, err)
	}
	evidence.Preview = preview.Token
	if _, err = r.Apply(context.Background(), operator, evidence); err != nil {
		t.Fatal(err)
	}
	state, err = r.Status(context.Background(), operator, d.ID)
	if err != nil || !state.AdmissionStopped || state.Counted != 60 || state.Remote != "possibly_active" {
		t.Fatal(state, err)
	}
	events, err := r.Events(context.Background(), operator)
	if err != nil || len(events) != 1 || events[0].Contract != "openai/gpt-6-luna/standard/external_unvalued_revision/EUR" {
		t.Fatal(events, err)
	}
}

func TestRestrictedReconciliationReopenRequiresCorrectedEvidenceAndBounds(t *testing.T) {
	f := setup(t, func(c *SyntheticConfig) { c.Limit = 10000; c.Capacity = 3 })
	s := f.open(t, "reopen")
	d := f.sent(t, s)
	operator := reconciliationOperator(t, f)
	r := f.h.Reconciliation()
	bill := reconciliationUsage(d, "bad_bill")
	bill.Kind = "bill"
	bill.Evidence.Usage = nil
	bill.Evidence.Gross = pointer(120)
	bill.Evidence.Difference = pointer(0)
	bill.Evidence.Source = "bill_source"
	applyReconciliation(t, r, operator, bill)
	reopen := reconciliationUsage(d, "reopen_record")
	reopen.Kind = "reopen"
	reopen.ExpectedVersion = 1
	reopen.Evidence.Usage = nil
	reopen.Evidence.Difference = nil
	reopen.StopVersion = 1
	reopen.BoundsReference = "verified_bounds"
	reopen.ReconciliationReference = "reviewed_correction"
	if _, err := r.Preview(context.Background(), operator, reopen); err == nil {
		t.Fatal("owner assertion reopened breached contract")
	}
	correction := bill
	correction.Kind = "corrected_bill"
	correction.ID = "reviewed_correction"
	correction.ExpectedVersion = 1
	correction.Evidence.Revision = 2
	correction.Evidence.Supersedes = "bad_bill"
	correction.Evidence.Gross = pointer(50)
	applyReconciliation(t, r, operator, correction)
	reopen.ExpectedVersion = 2
	if _, err := r.Preview(context.Background(), operator, reopen); err == nil {
		t.Fatal("financial correction alone reopened inference")
	}
	if err := f.h.IssueCodeOwnedFakeBoundsReview(context.Background(), f.db, d.ID, "verified_bounds"); err != nil {
		t.Fatal(err)
	}
	applyReconciliation(t, r, operator, reopen)
	state, err := r.Status(context.Background(), operator, d.ID)
	if err != nil || state.AdmissionStopped || state.Counted != 50 || state.Remote != "possibly_active" {
		t.Fatal(state, err)
	}
	// The prior event and high amount remain in immutable audit, and no pool or
	// remote hold is replenished by the owner reopening record.
	events, err := r.Events(context.Background(), operator)
	if err != nil || len(events) != 1 || events[0].Obligation != 120 {
		t.Fatal(events, err)
	}
	if _, err = f.h.Open(context.Background(), f.p, "after_reviewed_reopen", "cv_generation", []byte(`{"synthetic":"root"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestRestrictedReconciliationExplicitKeyRetirementKeepsIndependentSurvivors(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "retire_old_input")
	d := f.sent(t, s)
	operator := reconciliationOperator(t, f)
	legacy := Evidence{ID: "inherited_estimate", Dispatch: d.ID, Provider: "openai", Kind: "estimate", Amount: pointer(35)}
	if err := f.h.SyntheticReviewer().ApplyEvidence(context.Background(), f.p, legacy); err != nil {
		t.Fatal(err)
	}
	r := f.h.Reconciliation()
	command := reconciliationUsage(d, "new_settlement")
	original := applyReconciliation(t, r, operator, command)
	f.clock.set(d.IntentAt.Add(30*24*time.Hour), true)
	compact := reconciliationUsage(d, "retire_detail")
	compact.Kind = "compact"
	compact.ExpectedVersion = 1
	compact.Evidence.Usage = nil
	compact.Evidence.Difference = nil
	compact.Retention = &DetailRetirementReview{Reference: "reviewed_no_detail_purpose"}
	applyReconciliation(t, r, operator, compact)
	cfg := f.cfg
	cfg.BindingKeyVersion = "input_v2"
	cfg.BindingKey = sha256.Sum256([]byte("synthetic-retirement-next-key"))
	cfg.BindingVerificationKeys = map[string][32]byte{"input_v1": f.cfg.BindingKey}
	rotated, err := NewSynthetic(context.Background(), f.dsn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rotated.Close()
	if err = f.h.SyntheticReviewer().DeleteAction(context.Background(), f.p, s.ID()); err != nil {
		t.Fatal(err)
	}
	if err = f.h.SyntheticReviewer().RetireNamespace(context.Background(), f.p); err != nil {
		t.Fatal(err)
	}
	checkpoint := rotated.Reconciliation().WithLocalCheckpointVerifier(func(_ context.Context, namespace, ref string) error {
		// This explicit test-owned evidence is the local counterpart of excluding
		// every old entrypoint/sender and expiring capabilities/recovery copies.
		if namespace != f.cfg.Namespace || ref != "whole_namespace_closed_capabilities_expired" {
			return ErrDenied
		}
		return nil
	})
	if _, err = checkpoint.RetireInputKey(context.Background(), operator, "input_v1", "whole_namespace_closed_capabilities_expired"); err != nil {
		t.Fatal(err)
	}
	_, operatorToken := login(t, f.svc, "synthetic-reconciliation-owner")
	cfg.BindingVerificationKeys = nil
	if os.Getenv("CAREEROS_RECONCILIATION_PHYSICAL_RESTART") == "1" {
		data := filepath.Clean(os.Getenv("CAREEROS_DRAFT_TEST_PGDATA"))
		pgctl := os.Getenv("CAREEROS_DRAFT_TEST_PGCTL")
		if !strings.HasPrefix(data, "/tmp/careeros-draft-pg-") || pgctl == "" {
			t.Fatal("dedicated private PostgreSQL required")
		}
		original, _ := url.Parse(f.dsn)
		options := "-h 127.0.0.1 -k " + filepath.Join(filepath.Dir(data), "socket") + " -p " + original.Port()
		for _, arguments := range [][]string{{"-D", data, "-m", "immediate", "-w", "stop"}, {"-D", data, "-l", filepath.Join(t.TempDir(), "restart.log"), "-o", options, "-w", "start"}} {
			child := exec.Command("rtk", append([]string{"proxy", pgctl}, arguments...)...)
			if out, err := child.CombinedOutput(); err != nil {
				t.Fatalf("private PostgreSQL restart failed: %v %s", err, out)
			}
		}
	}
	restarted, err := NewSynthetic(context.Background(), f.dsn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	result, err := restarted.Reconciliation().Apply(context.Background(), operator, command)
	if err != nil || !reflect.DeepEqual(original, result) {
		t.Fatal(result, original, err)
	}
	// An actual fresh CLI process has zero input keys and still verifies this
	// exact audit/replay result after retirement (and physical PG restart when enabled).
	var role string
	if err = restarted.reviewerDB.QueryRow(`SELECT current_user`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(f.dsn)
	u.User = url.User(role)
	rawCommand, _ := json.Marshal(command)
	cli := exec.Command("rtk", "proxy", "go", "run", "../../cmd/reconciliation-local", "--synthetic-only", "--namespace", f.cfg.Namespace, "--mode", "apply")
	cli.Env = append(os.Environ(), "CAREEROS_RECONCILIATION_LOCAL_DATABASE_URL="+u.String(), "CAREEROS_RECONCILIATION_LOCAL_SESSION_TOKEN="+operatorToken)
	cli.Stdin = bytes.NewReader(rawCommand)
	output, childErr := cli.CombinedOutput()
	if childErr != nil {
		t.Fatalf("fresh key-free CLI recovery: %v %s", childErr, output)
	}
	var fresh ReconciliationPreview
	if json.Unmarshal(output, &fresh) != nil || !reflect.DeepEqual(fresh, original) {
		t.Fatal("fresh CLI changed recorded result")
	}
	changed := command
	changed.Evidence.Difference = pointer(8)
	if _, err = restarted.Reconciliation().Apply(context.Background(), operator, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("changed replay survived key retirement", err)
	}
	if err = restarted.SyntheticReviewer().ApplyEvidence(context.Background(), f.p, legacy); err != nil {
		t.Fatal("old evidence lost independent verifier", err)
	}
	legacy.Amount = pointer(34)
	if err = restarted.SyntheticReviewer().ApplyEvidence(context.Background(), f.p, legacy); !errors.Is(err, ErrConflict) {
		t.Fatal("changed inherited evidence accepted", err)
	}
	higher := reconciliationUsage(d, "late_after_key_retirement")
	higher.Kind = "bill"
	higher.ExpectedVersion = 2
	higher.Evidence.Usage = nil
	higher.Evidence.Gross = pointer(90)
	higher.Evidence.Difference = pointer(0)
	applyReconciliation(t, restarted.Reconciliation(), operator, higher)
	state, err := restarted.Reconciliation().Status(context.Background(), operator, d.ID)
	if err != nil || state.Counted != 90 || state.Remote != "possibly_active" || !state.AdmissionStopped {
		t.Fatal(state, err)
	}
	freshAdmin, openErr := sql.Open("pgx", f.dsn)
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer freshAdmin.Close()
	var deleted, denied bool
	if err = freshAdmin.QueryRow(`SELECT a.state='deleted',n.retired FROM ai_fixture_action a JOIN ai_fixture_namespace n ON n.id=a.namespace WHERE a.id=$1`, s.ID()).Scan(&deleted, &denied); err != nil || !deleted || !denied {
		t.Fatal(deleted, denied, err)
	}
	if _, err = restarted.Open(context.Background(), f.p, "retire_old_input", "cv_generation", []byte(`{"synthetic":"root"}`)); err == nil || f.calls.Load() != 1 {
		t.Fatal("retired namespace revived", err, f.calls.Load())
	}
}

func TestRestrictedReconciliationRateRevisionNeverRepricesDispatch(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "rate_revision")
	d := f.sent(t, s)
	operator := reconciliationOperator(t, f)
	r := f.h.Reconciliation()
	command := reconciliationUsage(d, "new_rate_record")
	command.Kind = "rate_revision"
	command.Evidence.Usage = nil
	command.Evidence.Difference = nil
	command.RateRevision = &RecordedRateRevision{ID: "historical_rate_revision_2", Replaces: "synthetic-unit-v1", Revision: 2, Provider: "openai", Model: "gpt-6-luna", Tier: "standard", Currency: "EUR", Input: p64(200000), Cached: p64(10000), CacheWrite: p64(125000), Output: p64(500000)}
	applyReconciliation(t, r, operator, command)
	rates, err := r.RateRevisions(context.Background(), operator, d.ID)
	if err != nil || len(rates) != 1 || rates[0].Currency != "EUR" || rates[0].ID != "historical_rate_revision_2" {
		t.Fatal(rates, err)
	}
	state, err := r.Status(context.Background(), operator, d.ID)
	if err != nil || state.Counted != 60 || state.AdmissionStopped || state.Remote != "possibly_active" {
		t.Fatal(state, err)
	}
	// Explicit reference records never silently reprice existing obligations.
	changed := command
	changed.ID = "changed_rate"
	changed.ExpectedVersion = 1
	changed.Evidence.Source = "another_rate_source"
	copy := *command.RateRevision
	changed.RateRevision = &copy
	changed.RateRevision.Input = p64(300000)
	if _, err := r.Preview(context.Background(), operator, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("immutable rate record changed", err)
	}
}
func p64(n int64) *int64 { return &n }

func TestRestrictedReconciliationConcurrentAndStaleAuthority(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "concurrent_adjust")
	d := f.sent(t, s)
	operator := reconciliationOperator(t, f)
	r := f.h.Reconciliation()
	first := reconciliationUsage(d, "adjust_1")
	first.Kind = "bill"
	first.Evidence.Usage = nil
	first.Evidence.Gross = pointer(40)
	first.Evidence.Difference = pointer(0)
	second := first
	second.ID = "adjust_2"
	second.Evidence.Source = "adjust_2"
	second.Evidence.Gross = pointer(50)
	p1, err := r.Preview(context.Background(), operator, first)
	if err != nil {
		t.Fatal(err)
	}
	first.Preview = p1.Token
	p2, err := r.Preview(context.Background(), operator, second)
	if err != nil {
		t.Fatal(err)
	}
	second.Preview = p2.Token
	type outcome struct {
		after int64
		err   error
	}
	done := make(chan outcome, 2)
	start := make(chan struct{})
	for _, c := range []ReconciliationCommand{first, second} {
		go func(c ReconciliationCommand) {
			<-start
			result, err := r.Apply(context.Background(), operator, c)
			done <- outcome{result.After, err}
		}(c)
	}
	close(start)
	a, b := <-done, <-done
	if (a.err == nil) == (b.err == nil) {
		t.Fatal(a, b)
	}
	if a.err != nil && !errors.Is(a.err, ErrConflict) || b.err != nil && !errors.Is(b.err, ErrConflict) {
		t.Fatal(a, b)
	}
	winner := a.after
	if a.err != nil {
		winner = b.after
	}
	status, err := r.Status(context.Background(), operator, d.ID)
	if err != nil || status.Version != 1 || status.Counted != winner {
		t.Fatal(status, err)
	}
	for _, scope := range status.Scopes {
		if scope.Counted != winner || scope.Occupied != 1 {
			t.Fatal(scope)
		}
	}
	stale := first
	stale.ID = "stale_role"
	stale.ExpectedVersion = 1
	stale.Evidence.Source = "stale_role"
	stale.Evidence.Gross = pointer(60)
	preview, err := r.Preview(context.Background(), operator, stale)
	if err != nil {
		t.Fatal(err)
	}
	stale.Preview = preview.Token
	if _, err = f.db.Exec(`UPDATE ai_recon_operator SET revoked=true,version=version+1`); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Apply(context.Background(), operator, stale); err == nil {
		t.Fatal("revoked operator applied old preview")
	}
	if _, err = f.db.Exec(`UPDATE ai_recon_operator SET revoked=false,version=version+1`); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Apply(context.Background(), operator, stale); !errors.Is(err, ErrConflict) {
		t.Fatal("old elevation preview survived revoke/restore", err)
	}
	applyReconciliation(t, r, operator, stale)
}
func TestRestrictedReconciliationPostLockSessionExpiryRollsBackAllScopes(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "operator_expiry")
	d := f.sent(t, s)
	operator := reconciliationOperator(t, f)
	r := f.h.Reconciliation()
	c := reconciliationUsage(d, "expiry_adjustment")
	preview, err := r.Preview(context.Background(), operator, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Preview = preview.Token
	if _, err = f.db.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '60 milliseconds' WHERE id=$1`, operator.SessionID); err != nil {
		t.Fatal(err)
	}
	f.h.SetFault("before_reconciliation_commit", func() error { time.Sleep(100 * time.Millisecond); return nil })
	if _, err = r.Apply(context.Background(), operator, c); err == nil {
		t.Fatal("post-lock expired session settled")
	}
	f.h.SetFault("before_reconciliation_commit", nil)
	fresh, _ := login(t, f.svc, "synthetic-reconciliation-owner")
	state, err := r.Status(context.Background(), fresh, d.ID)
	if err != nil || state.Version != 0 || state.Counted != 60 {
		t.Fatal(state, err)
	}
	for _, scope := range state.Scopes {
		if scope.Counted != 60 || scope.Occupied != 1 {
			t.Fatal(scope)
		}
	}
	applyReconciliation(t, r, fresh, c)
	f.clock.set(d.IntentAt.Add(168*time.Hour), true)
	maximum := reconciliationUsage(d, "no_age_upgrade")
	maximum.Kind = "maximum"
	maximum.ExpectedVersion = 1
	maximum.Evidence.Usage = nil
	maximum.Evidence.Difference = nil
	if _, err = r.Preview(context.Background(), fresh, maximum); err == nil {
		t.Fatal("age replaced supported estimate and D")
	}
	state, err = r.Status(context.Background(), fresh, d.ID)
	if err != nil || state.Category != "supported_estimate" || state.Counted != 20 || state.Expense == nil || *state.Expense != 13 {
		t.Fatal(state, err)
	}
}
func TestRestrictedReconciliationInheritedLockRoleAndContentDenials(t *testing.T) {
	f := setup(t, nil)
	var runtimeRole string
	if err := f.h.db.QueryRow(`SELECT current_user`).Scan(&runtimeRole); err != nil {
		t.Fatal(err)
	}
	childRole := "ai_lock_child_" + opaque()
	if _, err := f.db.Exec("CREATE ROLE " + childRole + " LOGIN INHERIT NOSUPERUSER NOCREATEROLE NOCREATEDB"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("GRANT " + runtimeRole + " TO " + childRole); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(f.dsn)
	u.User = url.User(childRole)
	child, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	var epoch int64
	if err = child.QueryRow(`SELECT epoch FROM ai_fixture_namespace FOR SHARE`).Scan(&epoch); err != nil {
		t.Fatal("inherited lock privilege unavailable", err)
	}
	if _, err = child.Exec(`UPDATE ai_fixture_namespace SET epoch=epoch+1`); err == nil {
		t.Fatal("inherited lock grant enabled namespace mutation")
	}
	for _, db := range []*sql.DB{child, f.h.db} {
		for _, table := range []string{"ai_recon_operator", "ai_recon_command", "ai_recon_state", "ai_recon_event", "ai_recon_legacy_marker", "ai_recon_terminal_proof"} {
			rows, err := db.Query("SELECT * FROM " + table)
			if err == nil {
				rows.Close()
				t.Fatal("runtime inherited restricted evidence access", table)
			}
		}
	}
	if _, err = f.h.reviewerDB.Query(`SELECT document FROM careeros_profiles`); err == nil {
		t.Fatal("reconciliation read career content")
	}
}

func TestRestrictedReconciliationContainmentPreservesAuthorizedRecovery(t *testing.T) {
	f := setup(t, func(c *SyntheticConfig) { c.Limit = 10000; c.Capacity = 3 })
	s := f.open(t, "contained_recovery")
	d := f.sent(t, s)
	operator := reconciliationOperator(t, f)
	bill := reconciliationUsage(d, "breached_recovery")
	bill.Kind = "bill"
	bill.Evidence.Usage = nil
	bill.Evidence.Gross = pointer(120)
	bill.Evidence.Difference = pointer(0)
	applyReconciliation(t, f.h.Reconciliation(), operator, bill)
	status, err := f.h.Status(context.Background(), f.p, s.ID())
	if err != nil || status.ID != s.ID() || status.Dispatches[0].Counted != 120 {
		t.Fatal("admission containment changed authorized recovery rights", status, err)
	}
	recovered, err := f.h.Open(context.Background(), f.p, "contained_recovery", "cv_generation", []byte(`{"synthetic":"root"}`))
	if err != nil || recovered.ID() != s.ID() {
		t.Fatal("provider-free identity recovery blocked", err)
	}
	if _, err = f.h.Open(context.Background(), f.p, "new_after_containment", "cv_generation", []byte(`{"synthetic":"root"}`)); err == nil {
		t.Fatal("new Action admitted under containment")
	}
	resp, err := send(recovered)
	closeResp(resp)
	if err == nil || f.calls.Load() != 1 {
		t.Fatal("contained dispatch resent", err, f.calls.Load())
	}
}

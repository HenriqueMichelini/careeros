package accountapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestDraftOpeningReceiptAndSettlementSurvivePostgresAndProcessRestart(t *testing.T) {
	dsn, pgctl, data, log := privateRestartPostgres(t)
	t.Setenv("CAREEROS_TEST_DATABASE_URL", dsn)
	x := newOpeningFixture(t)
	path := filepath.Join(t.TempDir(), "opening-inference-witness")
	x.config.Review = func(ctx context.Context, r draftrecovery.ReviewRequest) (draftrecovery.Completion, error) {
		x.check.Add(1)
		witness(t, path, "completion-check")
		return controlledRecoveryReview(ctx, r)
	}
	if err := x.store.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	x.store, err = draftrecovery.OpenLocalOpening(context.Background(), x.f.DSN, x.config)
	if err != nil {
		t.Fatal(err)
	}
	x.router = accountapi.NewControlledDraftRecovery(x.f.Service, x.store, func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
		x.generation.Add(1)
		witness(t, path, "generation")
		return completeRecoveryBundle(g), nil
	})
	const key = "physical-restart"
	x.create(t, key)
	issued := x.request(t, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening", "", struct{}{})
	var receipt draftrecovery.OpeningReceipt
	if issued.Code != 200 || json.Unmarshal(issued.Body.Bytes(), &receipt) != nil {
		t.Fatalf("pre-restart receipt: %d %s", issued.Code, issued.Body.String())
	}
	var lose atomic.Bool
	lose.Store(true)
	x.fault = func(stage string) error {
		if stage == "after_opening_commit" && lose.Swap(false) {
			return errors.New("synthetic lost committed confirmation response")
		}
		return nil
	}
	if w := x.confirm(t, key, receipt); w.Code != 503 {
		t.Fatalf("lost committed opening reply: %d", w.Code)
	}
	settled := x.request(t, http.MethodGet, "/api/drafts/"+key+"/versions/1", "", nil)
	allowance := x.request(t, http.MethodGet, "/api/draft-allowance", "", nil)
	if settled.Code != 200 || x.allowance(t) != (openingAllowance{5, 0, 1, 4}) {
		t.Fatalf("pre-restart settlement: %d %s", settled.Code, settled.Body.String())
	}
	command, _ := json.Marshal(draftrecovery.OpeningConfirmation{Receipt: receipt.Receipt, BundleDigest: receipt.BundleDigest, Manifest: receipt.Manifest})
	handoff := restartHandoff{DSN: x.f.DSN, Account: x.f.Principal.AccountID, Session: x.f.Login.SessionToken, CSRF: x.f.Login.CSRFToken, Witness: path, Expected: settled.Body.Bytes(), Opening: true, OpeningReceipt: issued.Body.Bytes(), OpeningCommand: command, Allowance: allowance.Body.Bytes()}
	raw, _ := json.Marshal(handoff)
	handoffPath := filepath.Join(t.TempDir(), "opening-handoff.json")
	if err = os.WriteFile(handoffPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	x.store.Close()
	x.actions.Close()
	x.f.Store.Close()
	x.f.DB.Close()
	restartCommand(t, pgctl, "-D", data, "-l", log, "-m", "immediate", "-w", "restart")
	cmd := exec.Command(os.Args[0], "-test.run=^TestControlledDraftRestartChild$", "-test.v", "-test.timeout=45s")
	cmd.Env = append(os.Environ(), "CAREEROS_DRAFT_RESTART_HANDOFF="+handoffPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fresh opening server process: %v\n%s", err, out)
	}
	observed, err := os.ReadFile(path)
	if err != nil || string(observed) != "generation\ncompletion-check\n" || x.generation.Load() != 1 || x.check.Load() != 1 {
		t.Fatalf("restart repeated inference: %q %v", observed, err)
	}
	t.Log("real fsync PostgreSQL immediate restart and fresh process recovered exact pilot reservation/receipt/settlement/result; generation/check witness1/1 unchanged")
}

func checkOpeningRestartChild(t *testing.T, service *auth.Service, in restartHandoff) {
	t.Helper()
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("opening-edge", 1)
	actions, err := aicontrol.NewSynthetic(context.Background(), in.DSN, aicontrol.DefaultSyntheticConfig("opening-edge", gate))
	if err != nil {
		t.Fatal(err)
	}
	defer actions.Close()
	decisions := draftrecovery.DecisionFunc(func(ctx context.Context, a draftrecovery.Authority) (draftrecovery.Decision, error) {
		var d draftrecovery.Decision
		var active bool
		err := a.Transaction.QueryRowContext(ctx, `SELECT expires_at,(extract(epoch FROM activated_at)*1000000)::bigint,state='active' AND age_eligible AND NOT invitation_revoked FROM careeros_pilot_access WHERE account_id=$1`, a.Principal.AccountID).Scan(&d.GrantExpiresAt, &d.Epoch, &active)
		d.Generate, d.Publish, d.Read, d.Revoked = active, active, active, !active
		return d, err
	})
	recovery, err := draftrecovery.OpenLocalOpening(context.Background(), in.DSN, draftrecovery.Config{Actions: actions, BindingKey: []byte("synthetic-opening-edge-binding-key"), Decisions: decisions, Review: func(ctx context.Context, r draftrecovery.ReviewRequest) (draftrecovery.Completion, error) {
		witness(t, in.Witness, "unexpected-completion-check")
		return controlledRecoveryReview(ctx, r)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	router := accountapi.NewControlledDraftRecovery(service, recovery, func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
		witness(t, in.Witness, "unexpected-generation")
		return completeRecoveryBundle(g), nil
	})
	checkRestartReads(t, router, in)
	call := func(method, path string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Origin", aiadmission.Origin)
		r.Header.Set("X-CareerOS-Account", in.Account)
		r.Header.Set("X-CSRF-Token", in.CSRF)
		r.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: in.Session})
		r.AddCookie(&http.Cookie{Name: "__Host-careeros_csrf", Value: in.CSRF})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	issued := call(http.MethodPost, "/api/drafts/physical-restart/versions/1/opening", []byte(`{}`))
	if issued.Code != 200 || !bytes.Equal(issued.Body.Bytes(), in.OpeningReceipt) {
		t.Fatalf("fresh process changed issued receipt: %d %s", issued.Code, issued.Body.String())
	}
	confirmed := call(http.MethodPost, "/api/drafts/physical-restart/versions/1/opening/confirm", in.OpeningCommand)
	if confirmed.Code != 200 || !bytes.Equal(confirmed.Body.Bytes(), in.Expected) {
		t.Fatalf("fresh process duplicate ACK changed exact settlement: %d %s", confirmed.Code, confirmed.Body.String())
	}
	status := call(http.MethodGet, "/api/draft-allowance", nil)
	if status.Code != 200 || !bytes.Equal(status.Body.Bytes(), in.Allowance) {
		t.Fatalf("fresh process consumed or released another right: %d %s", status.Code, status.Body.String())
	}
}

func TestDraftOpeningRestoredRowsCannotOverrideNonRestoredTerminalAuthority(t *testing.T) {
	x := newOpeningFixture(t)
	const key = "restored-opening"
	created := x.create(t, key)
	receipt := x.receipt(t, key)
	ctx := context.Background()
	conn, err := x.f.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, table := range []string{"ai_fixture_action", "careeros_draft_actions", "careeros_draft_admissions", "careeros_draft_allowance"} {
		if _, err = conn.ExecContext(ctx, `CREATE TEMP TABLE opening_before_`+table+` AS TABLE `+table); err != nil {
			t.Fatal(err)
		}
	}
	x.cancelled.Store(true) // Kept outside the older SQL image.
	if w := x.confirm(t, key, receipt); w.Code != 403 || x.allowance(t) != (openingAllowance{5, 0, 0, 5}) {
		t.Fatalf("initial terminal winner: %d %s", w.Code, w.Body.String())
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, query := range []string{
		`UPDATE ai_fixture_action a SET state=b.state,customer_reserved=b.customer_reserved,customer_effect=b.customer_effect FROM opening_before_ai_fixture_action b WHERE a.namespace=b.namespace AND a.id=b.id`,
		`UPDATE careeros_draft_admissions a SET state=b.state FROM opening_before_careeros_draft_admissions b WHERE a.owner=b.owner AND a.creation_key=b.creation_key`,
		`UPDATE careeros_draft_actions a SET state=b.state,cleanup_queued=b.cleanup_queued FROM opening_before_careeros_draft_actions b WHERE a.namespace=b.namespace AND a.action_id=b.action_id`,
		`UPDATE careeros_draft_allowance a SET state=b.state,confirmed_at=b.confirmed_at,released_at=b.released_at FROM opening_before_careeros_draft_allowance b WHERE a.namespace=b.namespace AND a.action_id=b.action_id`,
	} {
		if _, err = tx.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if w := x.confirm(t, key, receipt); w.Code != 403 {
		t.Fatalf("older customer rows overrode current terminal authority: %d %s", w.Code, w.Body.String())
	}
	x.cancelled.Store(false)
	for range 2 {
		if w := x.confirm(t, key, receipt); w.Code != 403 {
			t.Fatalf("re-observed terminal result revived: %d", w.Code)
		}
	}
	closed := x.request(t, http.MethodGet, "/api/drafts/"+key+"/versions/1", "", nil)
	var result draftrecovery.Result
	if closed.Code != 410 || json.Unmarshal(closed.Body.Bytes(), &result) != nil || result.Action != created.Action || result.Bundle != nil || result.State != "closed" || result.AdmittedAt != created.AdmittedAt || result.Deadline != created.Deadline || x.allowance(t) != (openingAllowance{5, 0, 0, 5}) || x.generation.Load() != 1 || x.check.Load() != 1 {
		t.Fatalf("restored customer state changed identity/time/effect/callbacks: %d %s allowance%+v", closed.Code, closed.Body.String(), x.allowance(t))
	}
	t.Log("older local SQL customer/purpose rows remain closed under an independently kept injected terminal decision; this is not hosted restore execution")
}

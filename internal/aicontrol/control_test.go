package aicontrol

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/auth"
)

type fixtureClock struct {
	mu      sync.Mutex
	now     time.Time
	healthy bool
}

func (c *fixtureClock) Sample() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now, c.healthy
}
func (c *fixtureClock) set(now time.Time, healthy bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
	c.healthy = healthy
}

type fixtureOIDC struct{}

func (fixtureOIDC) ID() string { return "google" }
func (fixtureOIDC) AuthorizationURL(state, nonce, challenge, redirect string) string {
	return "https://synthetic.invalid/?state=" + state
}
func (fixtureOIDC) Exchange(_ context.Context, code, verifier, redirect, nonce string) (auth.Identity, error) {
	return auth.Identity{Provider: "google", Issuer: "https://accounts.google.com", Subject: code, EmailVerified: true}, nil
}
func authService(t *testing.T, s *accountstore.Store) *auth.Service {
	t.Helper()
	origin := "http://127.0.0.1:8787"
	reg, e := auth.NewRegistry(fixtureOIDC{})
	if e != nil {
		t.Fatal(e)
	}
	svc, e := auth.NewService(auth.Config{PublicOrigin: origin, SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(origin, "fixture-client", "fixture-secret")}}, reg, s.AuthStores())
	if e != nil {
		t.Fatal(e)
	}
	return svc
}
func login(t *testing.T, svc *auth.Service, subject string) (auth.Principal, string) {
	t.Helper()
	begin, e := svc.BeginLogin(context.Background(), "google", "/", nil)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(begin.URL)
	done, e := svc.CompleteLogin(context.Background(), "google", u.Query().Get("state"), subject, begin.BindingToken)
	if e != nil {
		t.Fatal(e)
	}
	p, e := svc.Authenticate(context.Background(), done.SessionToken)
	if e != nil {
		t.Fatal(e)
	}
	return p, done.SessionToken
}

type fixture struct {
	h     *Harness
	db    *sql.DB
	dsn   string
	p     auth.Principal
	token string
	svc   *auth.Service
	store *accountstore.Store
	clock *fixtureClock
	calls atomic.Int64
	cfg   SyntheticConfig
}

func setup(t *testing.T, change func(*SyntheticConfig)) *fixture {
	t.Helper()
	dsn := os.Getenv("CAREEROS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated synthetic PostgreSQL required")
	}
	admin, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := "careeros_ai_test_" + opaque()
	if _, e = admin.Exec("CREATE SCHEMA " + schema); e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	dsn = u.String()
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	store, e := accountstore.Open(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	if e = store.Migrate(context.Background()); e != nil {
		t.Fatal(e)
	}
	svc := authService(t, store)
	p, token := login(t, svc, "fixture-owner")
	gate := NewSyntheticGate()
	cfg := DefaultSyntheticConfig("fixture", gate)
	clock := &fixtureClock{now: time.Now().UTC(), healthy: true}
	cfg.Clock = clock
	if change != nil {
		change(&cfg)
	}
	gate.Reconcile(cfg.Namespace, 1)
	h, e := NewSynthetic(context.Background(), dsn, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { h.Close() })
	if e = h.SyntheticReviewer().SeedGrant(context.Background(), p, cfg.ExpiresAt, cfg.CustomerUnits); e != nil {
		t.Fatal(e)
	}
	f := &fixture{h: h, db: db, dsn: dsn, p: p, token: token, svc: svc, store: store, clock: clock, cfg: cfg}
	h.SetResponder(func(ctx context.Context, provider string, req *http.Request) (*http.Response, error) {
		f.calls.Add(1)
		if req.Header.Get("Authorization") != "" {
			t.Error("credential reached fixture")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"synthetic":true}`))}, nil
	})
	if _, e = db.Exec(`CREATE TABLE ai_test_invocations(id bigserial PRIMARY KEY)`); e != nil {
		t.Fatal(e)
	}
	return f
}

var fixtureBody = []byte(`{"model":"gpt-6-luna","max_completion_tokens":4000,"messages":[{"role":"user","content":"synthetic-only"}]}`)

func (f *fixture) open(t *testing.T, key string) *Session {
	t.Helper()
	s, e := f.h.Open(context.Background(), f.p, key, "cv_generation", []byte(`{"synthetic":"root"}`))
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func req() *http.Request {
	r, _ := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(fixtureBody))
	r.Header.Set("Authorization", "Bearer synthetic-must-not-reach-fixture")
	return r
}
func send(s *Session) (*http.Response, error) {
	ctx, _ := aidiagnostics.Start(aidispatch.WithWorkflowRoot(context.Background(), s.workflow, s.rootInput), "generation", "synthetic-v1")
	return s.Send(ctx, "openai", req())
}
func closeResp(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
}
func (f *fixture) status(t *testing.T, s *Session) ActionStatus {
	t.Helper()
	v, e := f.h.Status(context.Background(), f.p, s.ID())
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func (f *fixture) scope(t *testing.T) ScopeStatus {
	t.Helper()
	v, e := f.h.ScopeStatus(context.Background(), f.p, "global")
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func (f *fixture) sent(t *testing.T, s *Session) Dispatch {
	t.Helper()
	r, e := send(s)
	closeResp(r)
	if e != nil {
		t.Fatal(e)
	}
	state := f.status(t, s)
	if len(state.Dispatches) != 1 {
		t.Fatal(state)
	}
	return state.Dispatches[0]
}
func pointer(v int64) *int64 { return &v }
func (f *fixture) terminal(t *testing.T, d Dispatch) {
	t.Helper()
	if e := f.h.SyntheticReviewer().ApplyEvidence(context.Background(), f.p, Evidence{ID: "terminal_" + d.ID, Dispatch: d.ID, Provider: d.Provider, Kind: "terminal"}); e != nil {
		t.Fatal(e)
	}
}

// These children are real independently reconstructed processes sharing only
// PostgreSQL and explicitly synthetic identity/config. No network provider exists.
func TestSyntheticChild(t *testing.T) {
	if os.Getenv("AI_SYNTHETIC_CHILD") != "1" {
		return
	}
	dsn := os.Getenv("AI_SYNTHETIC_DSN")
	store, e := accountstore.Open(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	svc := authService(t, store)
	p, e := svc.Authenticate(context.Background(), os.Getenv("AI_SYNTHETIC_TOKEN"))
	if e != nil {
		t.Fatal(e)
	}
	gate := NewSyntheticGate()
	gate.Reconcile("fixture", 1)
	cfg := DefaultSyntheticConfig("fixture", gate)
	h, e := NewSynthetic(context.Background(), dsn, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	h.SetResponder(func(context.Context, string, *http.Request) (*http.Response, error) {
		// This counter belongs to the fake provider, outside runtime/reviewer
		// ledger privileges. It is independent persisted evidence of invocation.
		observer, e := sql.Open("pgx", dsn)
		if e != nil {
			return nil, e
		}
		defer observer.Close()
		_, e = observer.Exec(`INSERT INTO ai_test_invocations DEFAULT VALUES`)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`))}, e
	})
	mode := os.Getenv("AI_SYNTHETIC_MODE")
	if mode == "crash_intent" {
		h.SetFault("after_intent_commit", func() error { os.Exit(13); return nil })
	}
	s, e := h.Open(context.Background(), p, os.Getenv("AI_SYNTHETIC_KEY"), "cv_generation", []byte(`{"synthetic":"root"}`))
	if e != nil {
		t.Fatal(e)
	}
	if mode == "create" {
		return
	}
	r, e := send(s)
	closeResp(r)
	if e != nil && !errors.Is(e, ErrCapacity) && !errors.Is(e, ErrOutputUnavailable) {
		t.Fatal(e)
	}
}
func (f *fixture) child(key, mode string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestSyntheticChild$", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "AI_SYNTHETIC_CHILD=1", "AI_SYNTHETIC_DSN="+f.dsn, "AI_SYNTHETIC_TOKEN="+f.token, "AI_SYNTHETIC_KEY="+key, "AI_SYNTHETIC_MODE="+mode)
	return cmd
}
func runPair(t *testing.T, a, b *exec.Cmd) {
	t.Helper()
	if e := a.Start(); e != nil {
		t.Fatal(e)
	}
	if e := b.Start(); e != nil {
		t.Fatal(e)
	}
	if e := a.Wait(); e != nil {
		t.Fatal(e)
	}
	if e := b.Wait(); e != nil {
		t.Fatal(e)
	}
}

func TestA01_DuplicateAcrossProcesses(t *testing.T) {
	f := setup(t, nil)
	runPair(t, f.child("duplicate", "send"), f.child("duplicate", "send"))
	var calls, dispatches int
	f.db.QueryRow(`SELECT count(*) FROM ai_test_invocations`).Scan(&calls)
	f.db.QueryRow(`SELECT count(*) FROM ai_fixture_dispatch`).Scan(&dispatches)
	scope := f.scope(t)
	if calls != 1 || dispatches != 1 || scope.Counted != 60 || scope.Limit-scope.Counted != 40 {
		t.Fatalf("calls=%d dispatches=%d scope=%+v", calls, dispatches, scope)
	}
	s := f.open(t, "duplicate")
	r, e := send(s)
	closeResp(r)
	if !errors.Is(e, ErrOutputUnavailable) || f.calls.Load() != 0 {
		t.Fatalf("same pointer replay: %v %d", e, f.calls.Load())
	}
}

func TestA02_ChangedReplay(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "binding")
	_, e := f.h.Open(context.Background(), f.p, "binding", "cv_generation", []byte(`{"synthetic":"changed"}`))
	if !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	leg := Leg{Provider: "openai", Coordinate: "request", Stage: "generation", Number: 1, Body: fixtureBody}
	if _, e = s.Reserve(context.Background(), leg); e != nil {
		t.Fatal(e)
	}
	leg.Body = []byte(`{"model":"gpt-6-luna","max_completion_tokens":4000,"messages":[{"content":"changed"}]}`)
	if _, e = s.Reserve(context.Background(), leg); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if f.calls.Load() != 0 {
		t.Fatal("send on changed binding")
	}
}

func TestA03_PrivatePrincipalAndSavedProfile(t *testing.T) {
	f := setup(t, nil)
	forged := auth.Principal{AccountID: f.p.AccountID, SessionID: f.p.SessionID, Generation: f.p.Generation}
	if _, e := f.h.Open(context.Background(), forged, "forged", "cv_generation", []byte(`{}`)); e == nil {
		t.Fatal("fabricated public principal fields authorized")
	}
	_, e := f.db.Exec(`INSERT INTO careeros_profiles VALUES($1,$2,1,$3)`, f.p.AccountID, []byte("synthetic"), []byte(`{"revision":1}`))
	if e != nil {
		t.Fatal(e)
	}
	s, e := f.h.Open(context.Background(), f.p, "profile", "application_draft", []byte(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	f.db.Exec(`UPDATE careeros_profiles SET document=$2 WHERE account_id=$1`, f.p.AccountID, []byte(`{"revision":2}`))
	ctx, _ := aidiagnostics.Start(aidispatch.WithWorkflowRoot(context.Background(), s.workflow, s.rootInput), "field_validation", "fixture")
	providerRequest, _ := http.NewRequest(http.MethodPost, "https://api.typesafe.ai/v1/systemone", strings.NewReader(`{"model":"jev-1.13.0","state":{"field":"job_posting","submission":"synthetic"}}`))
	r, e := s.Send(ctx, "typesafe", providerRequest)
	closeResp(r)
	if !errors.Is(e, ErrConflict) || f.calls.Load() != 0 {
		t.Fatalf("stale before intent: %v", e)
	}
	other, _ := login(t, f.svc, "other-owner")
	if _, e = f.h.Status(context.Background(), other, s.ID()); e == nil {
		t.Fatal("foreign action read")
	}
}

func TestA04_AtomicFinalBudgetAcrossProcesses(t *testing.T) {
	f := setup(t, nil)
	runPair(t, f.child("first", "send"), f.child("second", "send"))
	var calls, dispatches, allocations int
	f.db.QueryRow(`SELECT count(*) FROM ai_test_invocations`).Scan(&calls)
	f.db.QueryRow(`SELECT count(*) FROM ai_fixture_dispatch`).Scan(&dispatches)
	f.db.QueryRow(`SELECT count(*) FROM ai_fixture_allocation`).Scan(&allocations)
	v := f.scope(t)
	if calls != 1 || dispatches != 1 || allocations != 3 || v.Counted != 60 || v.Occupied != 1 {
		t.Fatalf("%d %d %d %+v", calls, dispatches, allocations, v)
	}
}

func TestA05_FiniteFailClosedConfiguration(t *testing.T) {
	if _, e := checkedSum(math.MaxInt64, 1); e == nil {
		t.Fatal("overflow")
	}
	for _, body := range []string{`{"model":"other","max_completion_tokens":1}`, `{"model":"gpt-6-luna"}`, `{"model":"gpt-6-luna","max_completion_tokens":0}`, `{"model":"gpt-6-luna","max_completion_tokens":99999}`} {
		if _, e := maximum("openai", []byte(body)); e == nil {
			t.Fatal(body)
		}
	}
	cfg := DefaultSyntheticConfig("fixture", NewSyntheticGate())
	if _, e := NewSynthetic(context.Background(), "postgresql://hosted.invalid/careeros?search_path=careeros_ai_test_bad", cfg); e == nil {
		t.Fatal("hosted constructor")
	}
	f := setup(t, nil)
	s := f.open(t, "valid")
	f.h.cfg.Gate.Close("fixture")
	r, e := send(s)
	closeResp(r)
	if e == nil || f.calls.Load() != 0 {
		t.Fatal("missing recovery authority sent")
	}
	f.h.cfg.Gate.Reconcile("fixture", 1)
	r, e = send(s)
	closeResp(r)
	if e == nil {
		t.Fatal("pre-close authority revived after reopening")
	}
	f.sent(t, f.open(t, "valid"))
}

func TestA06_ReservationRollback(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "rollback")
	f.h.SetFault("before_reserve_commit", func() error { return errors.New("synthetic failure") })
	r, e := send(s)
	closeResp(r)
	if e == nil {
		t.Fatal("expected failure")
	}
	var n int
	f.db.QueryRow(`SELECT count(*) FROM ai_fixture_dispatch`).Scan(&n)
	if n != 0 || f.scope(t).Counted != 0 || f.calls.Load() != 0 {
		t.Fatal("partial reservation survived")
	}
	f.h.SetFault("before_reserve_commit", nil)
	f.sent(t, s)
}

func TestA07_AbandonClaimRace(t *testing.T) {
	for i := 0; i < 4; i++ {
		f := setup(t, nil)
		s := f.open(t, fmt.Sprintf("race%d", i))
		d, e := s.Reserve(context.Background(), Leg{Provider: "openai", Coordinate: "request", Stage: "generation", Number: 1, Body: fixtureBody})
		if e != nil {
			t.Fatal(e)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; _ = s.Abandon(context.Background(), d.ID) }()
		go func() {
			defer wg.Done()
			<-start
			r, _ := s.invoke(context.Background(), d, req(), fixtureBody)
			closeResp(r)
		}()
		close(start)
		wg.Wait()
		state := f.status(t, s).Dispatches[0]
		scope := f.scope(t)
		if state.State == "aborted" {
			if f.calls.Load() != 0 || scope.Counted != 0 || scope.Occupied != 0 {
				t.Fatalf("abandon %+v %+v", state, scope)
			}
		} else if f.calls.Load() != 1 || scope.Counted != 60 || scope.Occupied != 1 {
			t.Fatalf("claim %+v %+v", state, scope)
		}
	}
}

func TestA08_CrashAfterIntentBeforeInvocation(t *testing.T) {
	f := setup(t, nil)
	cmd := f.child("crash", "crash_intent")
	e := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(e, &exit) || exit.ExitCode() != 13 {
		t.Fatalf("unexpected child result %v", e)
	}
	s := f.open(t, "crash")
	state := f.status(t, s).Dispatches[0]
	if state.State != "intent" || state.Counted != 60 || state.DurableBound != nil {
		t.Fatalf("%+v", state)
	}
	cmd = f.child("crash", "send")
	if e = cmd.Run(); e != nil {
		t.Fatal(e)
	}
	var n int
	f.db.QueryRow(`SELECT count(*) FROM ai_test_invocations`).Scan(&n)
	if n != 0 || f.scope(t).Occupied != 1 {
		t.Fatal("restart recreated permit")
	}
	if e = f.h.SyntheticReviewer().ReviewConservativeClosure(context.Background(), f.p, state.ID); e == nil {
		t.Fatal("missing durability witness allowed closure")
	}
}

func TestA09_UncertainOutcomesRetainExposure(t *testing.T) {
	for _, status := range []int{0, 429, 500, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := setup(t, nil)
			s := f.open(t, "unknown")
			f.h.SetResponder(func(context.Context, string, *http.Request) (*http.Response, error) {
				f.calls.Add(1)
				if status == 0 {
					return nil, context.DeadlineExceeded
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("malformed/refused/truncated"))}, nil
			})
			r, _ := send(s)
			closeResp(r)
			d := f.status(t, s).Dispatches[0]
			if d.Counted != 60 || d.Expense != nil || d.Remote != "possibly_active" || f.scope(t).Occupied != 1 || f.status(t, s).CustomerEffect {
				t.Fatalf("%+v", d)
			}
		})
	}
}

func TestA10_NoRedirectOrTransportRetry(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "redirect")
	f.h.SetResponder(func(context.Context, string, *http.Request) (*http.Response, error) {
		f.calls.Add(1)
		return &http.Response{StatusCode: 307, Header: http.Header{"Location": []string{"https://provider.invalid/retry"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	r, e := send(s)
	closeResp(r)
	if e != nil || f.calls.Load() != 1 {
		t.Fatal("unexpected retry")
	}
	r, e = send(s)
	closeResp(r)
	if !errors.Is(e, ErrOutputUnavailable) || f.calls.Load() != 1 {
		t.Fatal("duplicate resent")
	}
}

func TestA11_RevocationOrdering(t *testing.T) {
	f := setup(t, func(c *SyntheticConfig) { c.Limit = 1000; c.Capacity = 10 })
	s := f.open(t, "revoked")
	first := f.sent(t, s)
	if e := f.store.AuthStores().Sessions.Revoke(context.Background(), f.p.SessionID); e != nil {
		t.Fatal(e)
	}
	ctx, _ := aidiagnostics.Start(aidispatch.WithWorkflowRoot(context.Background(), s.workflow, s.rootInput), "statement_support_assessment", "fixture")
	r, e := s.Send(ctx, "openai", req())
	closeResp(r)
	if e == nil || f.calls.Load() != 1 {
		t.Fatal("revoked second leg sent")
	}
	var count int64
	f.db.QueryRow(`SELECT counted FROM ai_fixture_dispatch WHERE id=$1`, first.ID).Scan(&count)
	if count != 60 {
		t.Fatal("revocation erased expense")
	}
}

func TestA12_DurableRetryAndStaleAttempt(t *testing.T) {
	f := setup(t, func(c *SyntheticConfig) { c.Limit = 1000; c.Capacity = 10 })
	s := f.open(t, "retry")
	d := f.sent(t, s)
	review := f.h.SyntheticReviewer()
	if _, e := s.Retry(context.Background()); e == nil {
		t.Fatal("unknown became failed")
	}
	f.terminal(t, d)
	if e := review.MarkFailed(context.Background(), f.p, s.ID()); e != nil {
		t.Fatal(e)
	}
	reopened := f.open(t, "retry")
	var a, b *Session
	var e1, e2 error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a, e1 = reopened.Retry(context.Background()) }()
	go func() { defer wg.Done(); b, e2 = reopened.Retry(context.Background()) }()
	wg.Wait()
	if e1 != nil || e2 != nil || a.attempt != 2 || b.attempt != 2 {
		t.Fatalf("retry %v %v", e1, e2)
	}
	ctx, _ := aidiagnostics.Start(aidispatch.WithWorkflowRoot(context.Background(), s.workflow, s.rootInput), "statement_support_assessment", "fixture")
	if r, e := s.Send(ctx, "openai", req()); e == nil {
		closeResp(r)
		t.Fatal("stale attempt new stage")
	}
	resp, err := send(a)
	closeResp(resp)
	if err != nil {
		t.Fatal(err)
	}
	if f.calls.Load() != 2 || f.scope(t).Counted != 120 {
		t.Fatal("retry lost prior expense")
	}
	if f.status(t, a).CustomerEffect {
		t.Fatal("retry deducted")
	}
}

func TestA13_ProviderSpecificUsageMissingness(t *testing.T) {
	u := Usage{Input: pointer(10), Output: pointer(6), Cached: pointer(4), Reasoning: pointer(2)}
	n, e := Estimate("openai", "gpt-6-luna", "standard", u)
	if e != nil || n != 16 {
		t.Fatalf("subset accounting %d %v", n, e)
	}
	parts, e := EstimateParts("openai", "gpt-6-luna", "standard", "synthetic-unit-v1", u)
	if e != nil || parts.UncachedInput != 6 || parts.CachedInput != 4 || parts.Output != 6 || parts.ReasoningSubset != 2 || parts.Total != 16 {
		t.Fatalf("category decomposition %+v %v", parts, e)
	}
	if _, e = EstimateParts("openai", "gpt-6-luna", "standard", "", u); e == nil {
		t.Fatal("missing rate silently priced")
	}
	for _, bad := range []Usage{{Input: pointer(10)}, {Input: pointer(10), Output: pointer(6), CacheWrite: pointer(1)}, {Input: pointer(10), Output: pointer(6), Cached: pointer(11)}, {Input: pointer(10), Output: pointer(6), Unsupported: true}} {
		if _, e = Estimate("openai", "gpt-6-luna", "standard", bad); e == nil {
			t.Fatal("missing/unsupported became zero")
		}
	}
	if _, e = Estimate("typesafe", "jev-1.13.0", "standard", u); e == nil {
		t.Fatal("cross-provider categories")
	}
	if _, e = Estimate("openai", "gpt-6-luna", "unknown", u); e == nil {
		t.Fatal("unknown tier")
	}
}

func TestA14_EstimateAndResidualFloor(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "estimate")
	d := f.sent(t, s)
	r := f.h.SyntheticReviewer()
	e := Evidence{ID: "estimate", Dispatch: d.ID, Provider: "openai", Kind: "estimate", Amount: pointer(35)}
	if err := r.ApplyEvidence(context.Background(), f.p, e); err != nil {
		t.Fatal(err)
	}
	v := f.status(t, s).Dispatches[0]
	if *v.Expense != 35 || v.Counted != 60 {
		t.Fatal(v)
	}
	f.clock.set(v.DurableBound.Add(168*time.Hour), true)
	if err := r.ReviewConservativeClosure(context.Background(), f.p, d.ID); err == nil {
		t.Fatal("estimate overwritten by age")
	}
	e.ID = "accepted_estimate"
	e.ReleaseFloor = true
	if err := r.ApplyEvidence(context.Background(), f.p, e); err != nil {
		t.Fatal(err)
	}
	v = f.status(t, s).Dispatches[0]
	if v.Counted != 35 || f.scope(t).Limit-f.scope(t).Counted != 65 {
		t.Fatal(v)
	}
}

func TestA15_ConfirmedEvidenceIdempotency(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "bill")
	d := f.sent(t, s)
	r := f.h.SyntheticReviewer()
	e := Evidence{ID: "bill", Dispatch: d.ID, Provider: "openai", Kind: "bill", Amount: pointer(40), ReleaseFloor: true}
	for i := 0; i < 2; i++ {
		if err := r.ApplyEvidence(context.Background(), f.p, e); err != nil {
			t.Fatal(err)
		}
	}
	if f.scope(t).Counted != 40 {
		t.Fatal("duplicate delta")
	}
	e.Amount = pointer(30)
	if err := r.ApplyEvidence(context.Background(), f.p, e); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	e.ID = "wrong_provider"
	e.Provider = "typesafe"
	if err := r.ApplyEvidence(context.Background(), f.p, e); err == nil {
		t.Fatal("foreign provider bill")
	}
	e.ID = "older_estimate"
	e.Provider = "openai"
	e.Kind = "estimate"
	if err := r.ApplyEvidence(context.Background(), f.p, e); err == nil {
		t.Fatal("weaker evidence replaced bill")
	}
	e.ID = "new_bill"
	e.Kind = "bill"
	e.Version = 2
	if err := r.ApplyEvidence(context.Background(), f.p, e); err != nil {
		t.Fatal(err)
	}
	if f.scope(t).Counted != 30 {
		t.Fatal("reviewed correction not applied")
	}
	e.ID = "reordered_old_bill"
	e.Amount = pointer(40)
	e.Version = 1
	if err := r.ApplyEvidence(context.Background(), f.p, e); err == nil {
		t.Fatal("reordered old bill replaced new bill")
	}
	e.ID = "changed_same_version"
	e.Version = 2
	if err := r.ApplyEvidence(context.Background(), f.p, e); err == nil {
		t.Fatal("same grade/version accepted conflicting amount")
	}
	if f.scope(t).Counted != 30 {
		t.Fatal("reordered evidence changed contribution")
	}
}

func TestA16_ReservationBreach(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "breach")
	d := f.sent(t, s)
	e := Evidence{ID: "breach", Dispatch: d.ID, Provider: "openai", Kind: "bill", Amount: pointer(120), ReleaseFloor: true}
	if err := f.h.SyntheticReviewer().ApplyEvidence(context.Background(), f.p, e); err != nil {
		t.Fatal(err)
	}
	v := f.scope(t)
	if v.Counted != 120 || v.Limit-v.Counted != -20 {
		t.Fatal(v)
	}
	if !f.status(t, s).Dispatches[0].MaximumBreached {
		t.Fatal("bound breach not recorded")
	}
	next := f.open(t, "next")
	if r, e := send(next); e == nil {
		closeResp(r)
		t.Fatal("breach allowed new send")
	}
}

func TestA17_SevenDaysIsEligibilityOnly(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "window")
	d := f.sent(t, s)
	r := f.h.SyntheticReviewer()
	f.clock.set(d.DurableBound.Add(168*time.Hour-time.Nanosecond), true)
	if e := r.ReviewConservativeClosure(context.Background(), f.p, d.ID); e == nil {
		t.Fatal("early closure")
	}
	f.clock.set(d.DurableBound.Add(168*time.Hour), true)
	v := f.status(t, s).Dispatches[0]
	if v.Category != "unknown_maximum" || v.Counted != 60 || f.scope(t).Occupied != 1 {
		t.Fatal("timer changed state")
	}
	if e := r.ReviewConservativeClosure(context.Background(), f.p, d.ID); e != nil {
		t.Fatal(e)
	}
	v = f.status(t, s).Dispatches[0]
	if v.Category != "reviewed_conservative_maximum" || v.Counted != 60 || f.scope(t).Occupied != 1 || f.status(t, s).CustomerEffect {
		t.Fatal(v)
	}
}

func TestA18_PeriodAndStageDoNotRenewAnchor(t *testing.T) {
	f := setup(t, func(c *SyntheticConfig) { c.Limit = 1000; c.Capacity = 10 })
	s := f.open(t, "anchor")
	d := f.sent(t, s)
	f.clock.set(d.DurableBound.Add(167*time.Hour), true)
	ctx, _ := aidiagnostics.Start(aidispatch.WithWorkflowRoot(context.Background(), s.workflow, s.rootInput), "statement_support_assessment", "fixture")
	resp, e := s.Send(ctx, "openai", req())
	closeResp(resp)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.h.SyntheticReviewer().AdvanceSyntheticPeriod(context.Background(), f.p, 1000); e != nil {
		t.Fatal(e)
	}
	v := f.status(t, s)
	if len(v.Dispatches) != 2 || f.scope(t).Counted != 120 {
		t.Fatal(v)
	}
	for _, item := range v.Dispatches {
		if item.ID == d.ID && !item.DurableBound.Equal(*d.DurableBound) {
			t.Fatal("anchor slid")
		}
	}
	f.clock.set(d.DurableBound.Add(168*time.Hour), true)
	if e = f.h.SyntheticReviewer().ReviewConservativeClosure(context.Background(), f.p, d.ID); e != nil {
		t.Fatal(e)
	}
}

func TestA19_ClockUncertaintyDeniesClosure(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "clock")
	d := f.sent(t, s)
	for _, sample := range []struct {
		now     time.Time
		healthy bool
	}{{d.DurableBound.Add(-time.Hour), true}, {d.DurableBound.Add(1000 * time.Hour), false}} {
		f.clock.set(sample.now, sample.healthy)
		if e := f.h.SyntheticReviewer().ReviewConservativeClosure(context.Background(), f.p, d.ID); e == nil {
			t.Fatal("untrusted clock closed")
		}
	}
	if f.scope(t).Counted != 60 || f.scope(t).Occupied != 1 {
		t.Fatal("uncertain clock released")
	}
}

func TestA20_PhysicalReleaseNeedsIndependentTerminalProof(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "capacity")
	d := f.sent(t, s)
	f.clock.set(d.DurableBound.Add(168*time.Hour), true)
	r := f.h.SyntheticReviewer()
	if e := r.ReviewConservativeClosure(context.Background(), f.p, d.ID); e != nil {
		t.Fatal(e)
	}
	if f.scope(t).Occupied != 1 {
		t.Fatal("closure released capacity")
	}
	f.terminal(t, d)
	f.terminal(t, d)
	v := f.scope(t)
	if v.Occupied != 0 || v.Counted != 60 {
		t.Fatal(v)
	}
}

func TestA21_PreDeliveryCancelSeparatesExpense(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "cancel")
	if e := s.ReserveCustomer(context.Background()); e != nil {
		t.Fatal(e)
	}
	d := f.sent(t, s)
	r := f.h.SyntheticReviewer()
	if e := r.Customer(context.Background(), f.p, s.ID(), CustomerEvent{Kind: "cancel"}); e != nil {
		t.Fatal(e)
	}
	v := f.status(t, s)
	if v.CustomerReserved || v.CustomerEffect || v.State != "canceled" || f.scope(t).Counted != 60 || f.scope(t).Occupied != 1 {
		t.Fatal(v, d)
	}
	if e := r.Customer(context.Background(), f.p, s.ID(), CustomerEvent{Kind: "open", ResultRef: "late", Complete: true, Recoverable: true}); e == nil {
		t.Fatal("late debit")
	}
}

func TestA22_DeliveryEffectOnceIndependentOfAccept(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "delivery")
	if e := s.ReserveCustomer(context.Background()); e != nil {
		t.Fatal(e)
	}
	r := f.h.SyntheticReviewer()
	for _, kind := range []string{"candidate", "open", "open", "accept", "export", "cancel"} {
		if e := r.Customer(context.Background(), f.p, s.ID(), CustomerEvent{Kind: kind, ResultRef: "fixture_result", Complete: true, Recoverable: true}); e != nil {
			t.Fatal(kind, e)
		}
	}
	v := f.status(t, s)
	if !v.CustomerEffect || v.CustomerReserved || v.State != "delivered" || len(v.Dispatches) != 0 {
		t.Fatal(v)
	}
}

func TestA23_IncompleteBundleCannotConsume(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "incomplete")
	s.ReserveCustomer(context.Background())
	r := f.h.SyntheticReviewer()
	for _, e := range []CustomerEvent{{Kind: "candidate", ResultRef: "cv_only", Complete: false, Recoverable: true}, {Kind: "candidate", ResultRef: "lost", Complete: true, Recoverable: false}, {Kind: "open", ResultRef: "http200", Complete: true, Recoverable: true}} {
		if err := r.Customer(context.Background(), f.p, s.ID(), e); err == nil {
			t.Fatal("incomplete eligible")
		}
	}
	if f.status(t, s).CustomerEffect {
		t.Fatal("partial deduction")
	}
	if e := r.Customer(context.Background(), f.p, s.ID(), CustomerEvent{Kind: "candidate", ResultRef: "honest_gap", Complete: true, Recoverable: true}); e != nil {
		t.Fatal(e)
	}
}

func TestA24_CustomerDeadlineIndependentFromFinancialWindow(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "deadline")
	s.ReserveCustomer(context.Background())
	d := f.sent(t, s)
	f.clock.set(d.DurableBound.Add(169*time.Hour), true)
	r := f.h.SyntheticReviewer()
	if e := r.Customer(context.Background(), f.p, s.ID(), CustomerEvent{Kind: "close"}); e == nil {
		t.Fatal("W became customer deadline")
	}
	deadline := d.DurableBound.Add(200 * time.Hour)
	if e := r.Customer(context.Background(), f.p, s.ID(), CustomerEvent{Kind: "close", Deadline: &deadline}); e == nil {
		t.Fatal("early customer closure")
	}
	f.clock.set(deadline, true)
	if e := r.Customer(context.Background(), f.p, s.ID(), CustomerEvent{Kind: "close", Deadline: &deadline}); e != nil {
		t.Fatal(e)
	}
	v := f.status(t, s)
	if v.State != "closed" || v.CustomerEffect || v.CustomerReserved || f.scope(t).Counted != 60 {
		t.Fatal(v)
	}
}

func TestA25_LostOutputLookupNeverInfers(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "lookup")
	f.sent(t, s)
	for i := 0; i < 3; i++ {
		v := f.status(t, s)
		if len(v.Dispatches) != 1 || v.CustomerEffect {
			t.Fatal(v)
		}
	}
	r, e := send(s)
	closeResp(r)
	if !errors.Is(e, ErrOutputUnavailable) || f.calls.Load() != 1 {
		t.Fatal("lookup replay regenerated")
	}
}

func TestA30_CompactionKeepsReplayAndExpense(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "compact")
	d := f.sent(t, s)
	f.clock.set(d.DurableBound.Add(168*time.Hour), true)
	r := f.h.SyntheticReviewer()
	if e := r.Compact(context.Background(), f.p, d.ID); e == nil {
		t.Fatal("unreviewed compaction")
	}
	if e := r.ReviewConservativeClosure(context.Background(), f.p, d.ID); e != nil {
		t.Fatal(e)
	}
	if e := r.Compact(context.Background(), f.p, d.ID); e != nil {
		t.Fatal(e)
	}
	resp, e := send(s)
	closeResp(resp)
	if !errors.Is(e, ErrOutputUnavailable) || f.calls.Load() != 1 || f.scope(t).Counted != 60 {
		t.Fatal("compaction revived")
	}
	f.h.cfg.BindingKey = [32]byte{}
	if _, e = f.h.Open(context.Background(), f.p, "compact", "cv_generation", []byte(`{"synthetic":"root"}`)); e == nil {
		t.Fatal("retired key reinterpreted replay")
	}
}

func TestA35_ExpiryDuringRealSQLScopeContention(t *testing.T) {
	for _, authority := range []string{"session", "grant", "policy"} {
		for _, phase := range []string{"reserve", "claim"} {
			t.Run(authority+"/"+phase, func(t *testing.T) {
				f := setup(t, nil)
				s := f.open(t, "expires")
				var d Dispatch
				if phase == "claim" {
					var e error
					d, e = s.Reserve(context.Background(), Leg{Provider: "openai", Coordinate: "request", Stage: "generation", Number: 1, Body: fixtureBody})
					if e != nil {
						t.Fatal(e)
					}
				}
				var e error
				switch authority {
				case "session":
					_, e = f.db.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '400 milliseconds' WHERE id=$1`, f.p.SessionID)
				case "grant":
					_, e = f.db.Exec(`UPDATE ai_fixture_grant SET expires_at=clock_timestamp()+interval '400 milliseconds'`)
				case "policy":
					_, e = f.db.Exec(`UPDATE ai_fixture_policy SET expires_at=clock_timestamp()+interval '400 milliseconds'`)
				}
				if e != nil {
					t.Fatal(e)
				}
				lock, e := f.db.Begin()
				if e != nil {
					t.Fatal(e)
				}
				defer lock.Rollback()
				if _, e = lock.Exec(`SELECT scope FROM ai_fixture_scope WHERE scope='global' FOR UPDATE`); e != nil {
					t.Fatal(e)
				}
				done := make(chan error, 1)
				go func() {
					var r *http.Response
					var err error
					if phase == "claim" {
						r, err = s.invoke(context.Background(), d, req(), fixtureBody)
					} else {
						r, err = send(s)
					}
					closeResp(r)
					done <- err
				}()
				deadline := time.Now().Add(3 * time.Second)
				for {
					var blocked int
					if e = f.db.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%ai_fixture_scope%'`).Scan(&blocked); e != nil {
						t.Fatal(e)
					}
					if blocked > 0 {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("worker never reached shared SQL contention")
					}
					time.Sleep(5 * time.Millisecond)
				}
				time.Sleep(450 * time.Millisecond)
				if e = lock.Commit(); e != nil {
					t.Fatal(e)
				}
				if e = <-done; e == nil {
					t.Fatal("pre-wait expiry authorized")
				}
				var intents, dispatches int
				if e = f.db.QueryRow(`SELECT count(*),count(*) FILTER(WHERE state IN ('intent','observed')) FROM ai_fixture_dispatch`).Scan(&dispatches, &intents); e != nil {
					t.Fatal(e)
				}
				want := 0
				if phase == "claim" {
					want = 1
				}
				if dispatches != want || intents != 0 || f.calls.Load() != 0 {
					t.Fatal("expired admission/intent committed", dispatches, intents, f.calls.Load())
				}
			})
		}
	}
}

func TestA36_CreationHandshakeLostAcknowledgment(t *testing.T) {
	f := setup(t, nil)
	runPair(t, f.child("create", "create"), f.child("create", "create"))
	var actions, calls int
	f.db.QueryRow(`SELECT count(*) FROM ai_fixture_action`).Scan(&actions)
	f.db.QueryRow(`SELECT count(*) FROM ai_test_invocations`).Scan(&calls)
	if actions != 1 || calls != 0 {
		t.Fatal("creation inferred", actions, calls)
	}
	f.h.SetFault("after_create_commit", func() error { return errors.New("lost acknowledgment") })
	if _, e := f.h.Open(context.Background(), f.p, "lost_create", "cv_generation", []byte(`{"synthetic":"root"}`)); e == nil {
		t.Fatal("missing injected failure")
	}
	f.h.SetFault("after_create_commit", nil)
	a := f.open(t, "lost_create")
	b := f.open(t, "lost_create")
	if a.ID() != b.ID() {
		t.Fatal("creation replay changed id")
	}
	f.sent(t, a)
	r, e := send(b)
	closeResp(r)
	if !errors.Is(e, ErrOutputUnavailable) || f.calls.Load() != 1 {
		t.Fatal("lost create duplicated dispatch")
	}
}

func TestA37_CommitDelayCannotShortenWindow(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "delayed_commit")
	f.h.SetFault("before_intent_commit", func() error { time.Sleep(60 * time.Millisecond); return nil })
	d := f.sent(t, s)
	if !d.DurableBound.After(*d.IntentAt) {
		t.Fatal("postcommit witness not after claim")
	}
	f.clock.set(d.IntentAt.Add(168*time.Hour), true)
	if e := f.h.SyntheticReviewer().ReviewConservativeClosure(context.Background(), f.p, d.ID); e == nil {
		t.Fatal("precommit time shortened W")
	}
	f.clock.set(d.DurableBound.Add(168*time.Hour), true)
	if e := f.h.SyntheticReviewer().ReviewConservativeClosure(context.Background(), f.p, d.ID); e != nil {
		t.Fatal(e)
	}
}

func TestBindingCanonicalAndRestrictedError(t *testing.T) {
	f := setup(t, nil)
	a, e := f.h.fingerprint(f.p.AccountID, "test", []byte(`{"opaque":9007199254740991,"unicode":"ação😀","a":1}`))
	if e != nil {
		t.Fatal(e)
	}
	b, e := f.h.fingerprint(f.p.AccountID, "test", []byte(`{"a":1,"unicode":"ação😀","opaque":9007199254740991}`))
	if e != nil || !bytes.Equal(a, b) {
		t.Fatal("object ordering changed binding")
	}
	if _, e = f.h.fingerprint(f.p.AccountID, "test", []byte(`{"a":1,"a":2}`)); e == nil {
		t.Fatal("duplicate keys")
	}
	if strings.Contains(ErrUnavailable.Error(), f.p.AccountID) {
		t.Fatal("identity in error")
	}
	raw, _ := json.Marshal(Evidence{ID: "synthetic", Dispatch: "synthetic", Provider: "openai", Kind: "bill", Amount: pointer(1)})
	if bytes.Contains(raw, []byte("synthetic-only")) {
		t.Fatal("content in evidence")
	}
}

var _ = aidispatch.RequireManaged

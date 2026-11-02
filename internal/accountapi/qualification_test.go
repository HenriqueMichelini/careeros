package accountapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func qualificationFixture(t *testing.T) (*aiadmission.Fixture, *aicontrol.Harness, *aiadmission.Provider, http.Handler) {
	return qualificationBoundFixture(t, func(_ string, raw []byte) (int64, bool) { return int64(len(raw)) + 64, true })
}
func qualificationBoundFixture(t *testing.T, bound func(string, []byte) (int64, bool)) (*aiadmission.Fixture, *aicontrol.Harness, *aiadmission.Provider, http.Handler) {
	t.Helper()
	f := aiadmission.NewFixture(t)
	ctx := context.Background()
	if _, err := f.DB.Exec(`INSERT INTO careeros_pilot_operators(account_id) VALUES($1)`, f.Principal.AccountID); err != nil {
		t.Fatal(err)
	}
	identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "fixture-account"}
	invite, err := f.Store.PilotCommand(ctx, f.Principal, "invite", accountstore.PilotCommand{Kind: "invite", Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.PilotCommand(ctx, f.Principal, "clear", accountstore.PilotCommand{Kind: "clear", Identity: identity}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.ActivatePilot(ctx, f.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("qualification", 1)
	cfg := aicontrol.QualificationSyntheticConfig("qualification", gate, bound)
	cfg.ExpiresAt = time.Now().Add(time.Hour)
	h, err := aicontrol.NewSynthetic(ctx, f.DSN, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	provider := &aiadmission.Provider{T: t, Workflow: "qualification_gaps"}
	if err = h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
		response, err := provider.Respond(ctx, p, r)
		return response, true, err
	}); err != nil {
		t.Fatal(err)
	}
	return f, h, provider, accountapi.NewSyntheticQualification(f.Service, f.Store, h)
}

func TestManagedQualificationKnownFailureRetryHasOneNewIdentity(t *testing.T) {
	f, h, p, router := qualificationFixture(t)
	n := 0
	_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
		n++
		if n == 1 {
			w := httptest.NewRecorder()
			w.WriteHeader(429)
			_, _ = w.WriteString(`{"error":"synthetic_rate_limit"}`)
			return w.Result(), true, nil
		}
		resp, err := p.Respond(ctx, provider, r)
		return resp, true, err
	})
	body := f.Body(t, "/api/qualification-gaps")
	first := qualificationRequest(t, f, router, "/api/qualification-gaps", "retry-action", "", body)
	if first.Code < 400 || first.Header().Get("X-CareerOS-Retry-Allowed") != "true" || n != 1 {
		t.Fatalf("known failure: %d %s %d", first.Code, first.Body.String(), n)
	}
	retry := qualificationRequest(t, f, router, "/api/qualification-gaps", "retry-action", "retry-operation", body)
	if retry.Code != 200 || n != 4 {
		t.Fatalf("deliberate bounded retry: %d %s calls=%d", retry.Code, retry.Body.String(), n)
	}
	for _, key := range []string{"retry-operation", "changed-retry"} {
		w := qualificationRequest(t, f, router, "/api/qualification-gaps", "retry-action", key, body)
		if w.Code == 200 || n != 4 {
			t.Fatalf("retry replay allocated another dispatch: %d %d", w.Code, n)
		}
	}
}

func TestManagedQualificationCompleteRequestBoundsDenyBeforeSend(t *testing.T) {
	for _, kind := range []string{"typesafe", "openai", "unavailable"} {
		t.Run(kind, func(t *testing.T) {
			f, _, p, router := qualificationBoundFixture(t, func(provider string, raw []byte) (int64, bool) {
				if !bytes.Contains(raw, []byte("Java required.")) {
					t.Error("complete original posting missing from full serialized request")
				}
				if kind == "unavailable" {
					return 0, false
				}
				if provider == kind {
					if kind == "typesafe" {
						return 32001, true
					}
					return 64001, true
				}
				return int64(len(raw)) + 64, true
			})
			w := qualificationRequest(t, f, router, "/api/qualification-gaps", "bounded", "", f.Body(t, "/api/qualification-gaps"))
			want := 0
			if kind == "openai" {
				want = 1
			}
			if w.Code < 400 || len(p.Calls) != want {
				t.Fatalf("untrustworthy/oversized full envelope sent: %d %v", w.Code, p.Calls)
			}
		})
	}
}

func TestManagedQualificationPostLockExpiryKeepsProviderClosed(t *testing.T) {
	f, h, p, router := qualificationFixture(t)
	h.SetFault("before_claim", func() error { time.Sleep(120 * time.Millisecond); return nil })
	if _, err := f.DB.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '80 milliseconds' WHERE id=$1`, f.Principal.SessionID); err != nil {
		t.Fatal(err)
	}
	w := qualificationRequest(t, f, router, "/api/qualification-gaps", "expiry", "", f.Body(t, "/api/qualification-gaps"))
	if w.Code < 400 || len(p.Calls) != 0 {
		t.Fatalf("post-lock expiry sent: %d %v", w.Code, p.Calls)
	}
}

func qualificationParticipant(t *testing.T, operator *aiadmission.Fixture, subject string) *aiadmission.Fixture {
	t.Helper()
	ctx := context.Background()
	f := *operator
	start, err := f.Service.BeginLogin(ctx, "google", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(start.URL)
	f.Login, err = f.Service.CompleteLogin(ctx, "google", u.Query().Get("state"), subject, start.BindingToken)
	if err != nil {
		t.Fatal(err)
	}
	f.Principal = f.Login.Principal
	if _, err = f.Store.SaveProfile(ctx, f.Principal, "initial", 0, f.Document); err != nil {
		t.Fatal(err)
	}
	identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: subject}
	invite, err := f.Store.PilotCommand(ctx, operator.Principal, "invite-"+subject, accountstore.PilotCommand{Kind: "invite", Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.PilotCommand(ctx, operator.Principal, "clear-"+subject, accountstore.PilotCommand{Kind: "clear", Identity: identity}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.ActivatePilot(ctx, f.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	return &f
}

func TestManagedQualificationGlobalAndAccountCapacityRaces(t *testing.T) {
	f, h, p, router := qualificationFixture(t)
	second := qualificationParticipant(t, f, "second")
	third := qualificationParticipant(t, f, "third")
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var sends atomic.Int32
	var mu sync.Mutex
	_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
		sends.Add(1)
		entered <- struct{}{}
		<-release
		mu.Lock()
		defer mu.Unlock()
		resp, err := p.Respond(ctx, provider, r)
		return resp, false, err
	})
	done := make(chan *httptest.ResponseRecorder, 2)
	for _, who := range []*aiadmission.Fixture{f, second} {
		who := who
		go func() {
			done <- qualificationRequest(t, who, router, "/api/qualification-gaps", "active", "", who.Body(t, "/api/qualification-gaps"))
		}()
	}
	for n := 0; n < 2; n++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("capacity race did not enter fake provider")
		}
	}
	for _, who := range []*aiadmission.Fixture{f, third} {
		w := qualificationRequest(t, who, router, "/api/qualification-gaps", "another-action", "", who.Body(t, "/api/qualification-gaps"))
		if w.Code != 429 || sends.Load() != 2 {
			close(release)
			t.Fatalf("selected account/global active cap: %d sends=%d", w.Code, sends.Load())
		}
	}
	close(release)
	<-done
	<-done
	scope, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || scope.Occupied != 2 || scope.Counted != 2688 {
		t.Fatalf("two unknown calls retain across race: %+v %v", scope, err)
	}
}

func TestManagedQualificationRestartDoesNotResendDurableIntent(t *testing.T) {
	f, h, p, router := qualificationFixture(t)
	h.SetFault("after_intent_commit", func() error { return aicontrol.ErrUnavailable })
	body := f.Body(t, "/api/qualification-gaps")
	_ = qualificationRequest(t, f, router, "/api/qualification-gaps", "restart", "", body)
	_ = h.Close()
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("qualification", 1) // explicit external synthetic authority, not production recovery
	cfg := aicontrol.QualificationSyntheticConfig("qualification", gate, func(_ string, raw []byte) (int64, bool) { return int64(len(raw)) + 64, true })
	restarted, err := aicontrol.NewSynthetic(context.Background(), f.DSN, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	_ = restarted.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
		resp, err := p.Respond(ctx, provider, r)
		return resp, true, err
	})
	w := qualificationRequest(t, f, accountapi.NewSyntheticQualification(f.Service, f.Store, restarted), "/api/qualification-gaps", "restart", "", body)
	if w.Code != 202 || len(p.Calls) != 0 {
		t.Fatalf("restart re-sent intent: %d %v", w.Code, p.Calls)
	}
	status, err := restarted.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || status.Counted != 1344 || status.Occupied != 1 {
		t.Fatalf("restart dropped obligation: %+v %v", status, err)
	}
}

func TestManagedQualificationGlobalMoneyRaceCountsGrossCarriedExposure(t *testing.T) {
	f, h, p, router := qualificationFixture(t)
	body := f.Body(t, "/api/qualification-gaps")
	if w := qualificationRequest(t, f, router, "/api/qualification-gaps", "historical", "", body); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	session, err := h.Open(context.Background(), f.Principal, "historical", "qualification_gaps", body)
	if err != nil {
		t.Fatal(err)
	}
	status, err := h.Status(context.Background(), f.Principal, session.ID())
	if err != nil {
		t.Fatal(err)
	}
	amount := int64(9960000)
	// Deliberately artificial historical reviewed gross charge. It exceeds the
	// estimate and therefore must remain counted; no discount/credit is applied.
	if err = h.SyntheticReviewer().ApplyEvidence(context.Background(), f.Principal, aicontrol.Evidence{ID: "historical-bill", Dispatch: status.Dispatches[0].ID, Provider: status.Dispatches[0].Provider, Kind: "bill", Amount: &amount, ReleaseFloor: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.DB.Exec(`UPDATE ai_fixture_dispatch SET intent_at=clock_timestamp()-interval '25 hours',expense=COALESCE(expense,0) WHERE namespace='qualification'`); err != nil {
		t.Fatal(err)
	}
	second := qualificationParticipant(t, f, "second")
	third := qualificationParticipant(t, f, "third")
	var providerLock sync.Mutex
	_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
		providerLock.Lock()
		defer providerLock.Unlock()
		resp, err := p.Respond(ctx, provider, r)
		return resp, true, err
	})
	start := make(chan struct{})
	done := make(chan int, 2)
	for _, who := range []*aiadmission.Fixture{second, third} {
		who := who
		go func() {
			<-start
			w := qualificationRequest(t, who, router, "/api/qualification-gaps", "last-money", "", who.Body(t, "/api/qualification-gaps"))
			done <- w.Code
		}()
	}
	close(start)
	a, b := <-done, <-done
	scope, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || scope.Counted > 10000000 || (a == 200 && b == 200) {
		t.Fatalf("atomic last-money race: %d/%d %+v %v", a, b, scope, err)
	}
}

func qualificationRequest(t *testing.T, f *aiadmission.Fixture, router http.Handler, path, key, retry string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := f.Request(t, context.Background(), path, body)
	r.Header.Del("X-OpenAI-Api-Key")
	r.Header.Del("X-TypeSafe-Api-Key")
	r.Header.Set("X-CareerOS-Action-Key", key)
	if retry != "" {
		r.Header.Set("X-CareerOS-Retry-Key", retry)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestManagedQualificationAccountCompositionAndLostAcknowledgment(t *testing.T) {
	f, h, p, router := qualificationFixture(t)
	body := f.Body(t, "/api/qualification-gaps")
	first := qualificationRequest(t, f, router, "/api/qualification-gaps", "stable-create", "", body)
	if first.Code != 200 {
		t.Fatalf("qualification: %d %s", first.Code, first.Body.String())
	}
	if len(p.Calls) != 3 {
		t.Fatalf("reached legs %v; want actual three-stage qualification", p.Calls)
	}
	for _, path := range []string{"/api/qualification-gaps", "/.netlify/functions/qualification-gaps", "/.netlify/functions/account/api/qualification-gaps"} {
		w := qualificationRequest(t, f, router, path, "stable-create", "", body)
		if w.Code != 202 || len(p.Calls) != 3 {
			t.Fatalf("lost-ack recovery %s: %d %s calls=%v", path, w.Code, w.Body.String(), p.Calls)
		}
	}
	status, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || status.Counted != 25344 || status.Occupied != 0 {
		t.Fatalf("selected reservation/explicit fake terminals: %+v %v", status, err)
	}
}

func TestManagedQualificationClosedAliasesAndCallerCredentials(t *testing.T) {
	f, _, p, router := qualificationFixture(t)
	body := f.Body(t, "/api/qualification-gaps")
	for _, path := range []string{"/api/application-draft", "/.netlify/functions/application-draft", "/api/cv/generate", "/.netlify/functions/cv-generation", "/api/profile/ingest", "/.netlify/functions/profile-ingestion", "/api/profile/review", "/.netlify/functions/profile-review"} {
		w := qualificationRequest(t, f, router, path, "closed", "", body)
		if w.Code < 400 || len(p.Calls) != 0 {
			t.Fatalf("unsupported managed alias %s: %d %v", path, w.Code, p.Calls)
		}
	}
	for _, name := range []string{"X-OpenAI-Api-Key", "X-TypeSafe-Api-Key", "Authorization"} {
		r := f.Request(t, context.Background(), "/api/qualification-gaps", body)
		r.Header.Del("X-OpenAI-Api-Key")
		r.Header.Del("X-TypeSafe-Api-Key")
		r.Header.Set(name, "caller")
		r.Header.Set("X-CareerOS-Action-Key", "credentials")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 400 || len(p.Calls) != 0 {
			t.Fatalf("caller credential %s: %d %v", name, w.Code, p.Calls)
		}
	}
	var fields map[string]any
	_ = json.Unmarshal(body, &fields)
	fields["apiKey"] = "caller"
	raw, _ := json.Marshal(fields)
	if w := qualificationRequest(t, f, router, "/api/qualification-gaps", "body-credentials", "", raw); w.Code != 400 || len(p.Calls) != 0 {
		t.Fatalf("body credential: %d", w.Code)
	}
}

func TestManagedQualificationImmutableBindingAndAuthorityDenials(t *testing.T) {
	for _, kind := range []string{"changed_input", "stale_profile", "expired", "suspended", "revoked", "session", "wrong_account", "missing_session"} {
		t.Run(kind, func(t *testing.T) {
			f, _, p, router := qualificationFixture(t)
			body := f.Body(t, "/api/qualification-gaps")
			if kind == "changed_input" {
				if w := qualificationRequest(t, f, router, "/api/qualification-gaps", "same", "", body); w.Code != 200 {
					t.Fatal(w.Body.String())
				}
				body = bytes.Replace(body, []byte("Java required."), []byte("Java required. Extra duty."), 1)
			}
			switch kind {
			case "stale_profile":
				body = bytes.Replace(body, []byte(`"Java"`), []byte(`"Changed"`), 1)
			case "expired":
				_, _ = f.DB.Exec(`UPDATE careeros_pilot_access SET expires_at=clock_timestamp()-interval '1 second' WHERE account_id=$1`, f.Principal.AccountID)
			case "suspended", "revoked":
				_, _ = f.DB.Exec(`UPDATE careeros_pilot_access SET state=$2 WHERE account_id=$1`, f.Principal.AccountID, kind)
			case "session":
				_, _ = f.DB.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, f.Principal.SessionID)
			}
			before := len(p.Calls)
			for _, path := range []string{"/api/qualification-gaps", "/.netlify/functions/qualification-gaps", "/.netlify/functions/account/api/qualification-gaps"} {
				r := f.Request(t, context.Background(), path, body)
				r.Header.Del("X-OpenAI-Api-Key")
				r.Header.Del("X-TypeSafe-Api-Key")
				r.Header.Set("X-CareerOS-Action-Key", "same")
				if kind == "wrong_account" {
					r.Header.Set("X-CareerOS-Account", "another-account")
				}
				if kind == "missing_session" {
					r.Header.Del("Cookie")
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code < 400 || len(p.Calls) != before {
					t.Fatalf("%s %s: %d %s calls=%v", kind, path, w.Code, w.Body.String(), p.Calls)
				}
			}
		})
	}
}

func TestManagedQualificationUnknownIntentCannotRetryOrResend(t *testing.T) {
	f, h, p, router := qualificationFixture(t)
	h.SetFault("after_intent_commit", func() error { return aicontrol.ErrUnavailable })
	body := f.Body(t, "/api/qualification-gaps")
	if w := qualificationRequest(t, f, router, "/api/qualification-gaps", "crashed", "", body); w.Code < 400 {
		t.Fatalf("fault was hidden: %d", w.Code)
	}
	for _, retry := range []string{"", "deliberate-retry"} {
		w := qualificationRequest(t, f, router, "/api/qualification-gaps", "crashed", retry, body)
		if w.Code == 200 || len(p.Calls) != 0 {
			t.Fatalf("unknown redispatched: %d %v", w.Code, p.Calls)
		}
	}
	status, err := h.QualificationStatus(context.Background(), f.Principal, "crashed")
	if err != nil || status.State != "unknown" || status.RetryAllowed || status.ResultAvailable {
		t.Fatalf("unknown status: %+v %v", status, err)
	}
	scope, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || scope.Counted != 1344 || scope.Occupied != 1 {
		t.Fatalf("lost intent hold: %+v %v", scope, err)
	}
}

func TestManagedQualificationRollingActionMaximumPreservesDraftAllowance(t *testing.T) {
	f, _, p, router := qualificationFixture(t)
	body := f.Body(t, "/api/qualification-gaps")
	for n := 0; n < 5; n++ {
		w := qualificationRequest(t, f, router, "/api/qualification-gaps", string(rune('a'+n)), "", body)
		if w.Code != 200 {
			t.Fatalf("action %d: %d %s", n, w.Code, w.Body.String())
		}
	}
	w := qualificationRequest(t, f, router, "/api/qualification-gaps", "sixth", "", body)
	if w.Code != 429 || len(p.Calls) != 15 {
		t.Fatalf("rolling count not bounded: %d %v", w.Code, p.Calls)
	}
	status, err := f.Store.PilotStatus(context.Background(), f.Principal)
	if err != nil || status.DraftAllowance != 5 {
		t.Fatalf("qualification changed Draft allowance: %+v %v", status, err)
	}
}

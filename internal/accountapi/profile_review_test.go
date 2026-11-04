package accountapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	ingestion "professional-information-repo/backend/profile-ingestion"
	"strings"
	"sync"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func profileReviewFixture(t *testing.T) (*aiadmission.Fixture, *aicontrol.Harness, *aiadmission.Provider, http.Handler) {
	return profileReviewBoundFixture(t, func(_ string, raw []byte) (int64, bool) { return int64(len(raw)) + 64, true })
}
func profileReviewBoundFixture(t *testing.T, bound func(string, []byte) (int64, bool)) (*aiadmission.Fixture, *aicontrol.Harness, *aiadmission.Provider, http.Handler) {
	t.Helper()
	f, _, _, _ := qualificationFixture(t)
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("qualification", 1)
	cfg := aicontrol.ProfileReviewSyntheticConfig("qualification", gate, bound)
	h, e := aicontrol.NewSynthetic(context.Background(), f.DSN, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { h.Close() })
	p := &aiadmission.Provider{T: t, Workflow: "profile_review"}
	_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
		v, e := p.Respond(ctx, provider, r)
		return v, true, e
	})
	return f, h, p, accountapi.NewSyntheticProfileReview(f.Service, f.Store, h, []byte("synthetic-ingestion-signing-key-32bytes"))
}
func TestManagedProfileReviewUsesTwoOpenAILegsAndReplay(t *testing.T) {
	f, _, p, router := profileReviewFixture(t)
	body := f.Body(t, "/api/profile/review")
	first := qualificationRequest(t, f, router, "/api/profile/review", "review-action", "", body)
	if first.Code != 200 || len(p.Calls) != 2 {
		t.Fatalf("proposal: %d %s %+v", first.Code, first.Body.String(), p.Calls)
	}
	for _, c := range p.Calls {
		if c.Provider != "openai" {
			t.Fatalf("unexpected provider %+v", c)
		}
	}
	replay := qualificationRequest(t, f, router, "/.netlify/functions/profile-review", "review-action", "", body)
	if replay.Code != 202 || len(p.Calls) != 2 {
		t.Fatalf("replay: %d %s %+v", replay.Code, replay.Body.String(), p.Calls)
	}
	var saved []byte
	if e := f.DB.QueryRow(`SELECT document FROM careeros_profiles WHERE account_id=$1`, f.Principal.AccountID).Scan(&saved); e != nil || !bytes.Equal(saved, f.Document) {
		t.Fatalf("implicit Profile write %v", e)
	}
}

func TestManagedProfileReviewFullSecondRequestBoundAndExpense(t *testing.T) {
	for _, mode := range []string{"max", "oversized_second", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			n := 0
			f, h, p, router := profileReviewBoundFixture(t, func(provider string, raw []byte) (int64, bool) {
				n++
				if provider != "openai" {
					t.Fatal("TypeSafe forbidden")
				}
				if !bytes.Contains(raw, []byte(`"max_completion_tokens":6000`)) || !bytes.Contains(raw, []byte(`"service_tier":"standard"`)) {
					t.Fatal("full output/model envelope missing")
				}
				if n == 2 && (!bytes.Contains(raw, []byte(`proposal`)) || !bytes.Contains(raw, []byte(`I use Java`))) {
					t.Fatal("second request missing proposal")
				}
				if mode == "unavailable" {
					return 0, false
				}
				if mode == "oversized_second" && n == 2 {
					return 64001, true
				}
				return 64000, true
			})
			w := qualificationRequest(t, f, router, "/api/profile/review", "bounds", "", f.Body(t, "/api/profile/review"))
			scope, e := h.ScopeStatus(context.Background(), f.Principal, "global")
			if e != nil {
				t.Fatal(e)
			}
			want := 2
			money := int64(22000)
			if mode == "oversized_second" {
				want = 1
				money = 11000
			}
			if mode == "unavailable" {
				want = 0
				money = 0
			}
			if len(p.Calls) != want || scope.Counted != money || (mode == "max" && w.Code != 200) || (mode != "max" && w.Code < 400) {
				t.Fatalf("%d %s calls=%+v scope=%+v", w.Code, w.Body.String(), p.Calls, scope)
			}
		})
	}
}
func TestManagedProfileReviewKnownFailureExplicitRetry(t *testing.T) {
	f, h, p, router := profileReviewFixture(t)
	n := 0
	_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
		n++
		if n == 1 {
			return &http.Response{StatusCode: 429, Header: make(http.Header), Body: http.NoBody}, true, nil
		}
		v, e := p.Respond(ctx, provider, r)
		return v, true, e
	})
	body := f.Body(t, "/api/profile/review")
	w := qualificationRequest(t, f, router, "/api/profile/review", "retry", "", body)
	if w.Code < 400 || w.Header().Get("X-CareerOS-Retry-Allowed") != "true" || n != 1 {
		t.Fatalf("failure %d %s %d", w.Code, w.Body.String(), n)
	}
	w = qualificationRequest(t, f, router, "/api/profile/review", "retry", "", body)
	if w.Code != 202 || n != 1 {
		t.Fatal("implicit retry")
	}
	w = qualificationRequest(t, f, router, "/api/profile/review", "retry", "explicit-retry", body)
	if w.Code != 200 || n != 3 {
		t.Fatalf("retry %d %s %d", w.Code, w.Body.String(), n)
	}
	for _, key := range []string{"explicit-retry", "changed-retry"} {
		w = qualificationRequest(t, f, router, "/api/profile/review", "retry", key, body)
		if w.Code == 200 || n != 3 {
			t.Fatal("retry resend")
		}
	}
}
func TestManagedProfileReviewCurrentnessAndNoCallerAuthority(t *testing.T) {
	for _, mode := range []string{"no_profile", "changed_document", "changed_section", "changed_locale", "credential", "envelope", "account", "expired_session", "expired_grant", "postlock", "changed_before_result", "policy_before_result"} {
		t.Run(mode, func(t *testing.T) {
			f, h, p, router := profileReviewFixture(t)
			body := f.Body(t, "/api/profile/review")
			path := "/api/profile/review"
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			switch mode {
			case "no_profile":
				_, _ = f.DB.Exec(`DELETE FROM careeros_profiles WHERE account_id=$1`, f.Principal.AccountID)
			case "changed_document":
				doc := in["document"].(map[string]any)
				doc["revision"] = float64(2)
			case "changed_section":
				in["section"] = "goals"
			case "changed_locale":
				in["locale"] = "fr"
			case "envelope":
				in["apiKey"] = "not-authority"
			case "expired_session":
				_, _ = f.DB.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second'`)
			case "expired_grant":
				_, _ = f.DB.Exec(`UPDATE careeros_pilot_access SET expires_at=clock_timestamp()-interval '1 second'`)
			case "postlock":
				h.SetFault("before_claim", func() error { time.Sleep(100 * time.Millisecond); return nil })
				_, _ = f.DB.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '70 milliseconds'`)
			case "changed_before_result", "policy_before_result":
				_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
					v, e := p.Respond(ctx, provider, r)
					if len(p.Calls) == 2 {
						if mode == "policy_before_result" {
							_, _ = f.DB.Exec(`UPDATE ai_fixture_policy SET units=units+1`)
						} else {
							_, _ = f.DB.Exec(`UPDATE careeros_profiles SET document=convert_to(replace(convert_from(document,'UTF8'),'"revision":1','"revision":2'),'UTF8') WHERE account_id=$1`, f.Principal.AccountID)
						}
					}
					return v, true, e
				})
			}
			body, _ = json.Marshal(in)
			r := f.Request(t, context.Background(), path, body)
			r.Header.Del("X-OpenAI-Api-Key")
			r.Header.Del("X-TypeSafe-Api-Key")
			r.Header.Set("X-CareerOS-Action-Key", "authority")
			if mode == "credential" {
				r.Header.Set("X-OpenAI-Api-Key", "caller")
			}
			if mode == "account" {
				r.Header.Set("X-CareerOS-Account", "foreign")
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			want := 0
			if mode == "changed_before_result" || mode == "policy_before_result" {
				want = 2
			}
			if w.Code < 400 || len(p.Calls) != want || strings.Contains(w.Body.String(), "Clarified wording") {
				t.Fatalf("authority %d %s sends=%+v", w.Code, w.Body.String(), p.Calls)
			}
		})
	}
}

func TestManagedProfileReviewDurableDeadlineAtEveryHandoff(t *testing.T) {
	for _, mode := range []string{"claim_zero_dispatch_replay", "postlock", "between_legs", "outer_result"} {
		t.Run(mode, func(t *testing.T) {
			f, h, p, router := profileReviewFixture(t)
			body := f.Body(t, "/api/profile/review")
			expire := func() {
				if _, e := f.DB.Exec(`UPDATE ai_fixture_stage SET created_at=clock_timestamp()-interval '26 seconds' WHERE stage='managed_request_review'`); e != nil {
					t.Fatal(e)
				}
			}
			switch mode {
			case "claim_zero_dispatch_replay":
				s, e := h.Open(context.Background(), f.Principal, "deadline", "profile_review", body)
				if e != nil {
					t.Fatal(e)
				}
				if e = s.BeginProfileReview(context.Background()); e != nil {
					t.Fatal(e)
				}
				expire()
			case "postlock":
				h.SetFault("before_claim", func() error { expire(); return nil })
			case "between_legs":
				_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
					v, e := p.Respond(ctx, provider, r)
					expire()
					return v, true, e
				})
			case "outer_result":
				n := 0
				h.SetFault("before_ingestion_result_lock", func() error {
					n++
					if n == 2 {
						expire()
					}
					return nil
				})
			}
			w := qualificationRequest(t, f, router, "/api/profile/review", "deadline", "", body)
			want := 0
			if mode == "between_legs" {
				want = 1
			}
			if mode == "outer_result" {
				want = 2
			}
			if w.Code < 400 || len(p.Calls) != want || strings.Contains(w.Body.String(), "Clarified wording") {
				t.Fatalf("deadline %d %s calls=%+v", w.Code, w.Body.String(), p.Calls)
			}
		})
	}
}

func TestManagedProfileReviewInheritsBothWorkflowHistories(t *testing.T) {
	f, prior, _, qualification := qualificationFixture(t)
	qbody := f.Body(t, "/api/qualification-gaps")
	if w := qualificationRequest(t, f, qualification, "/api/qualification-gaps", "old-qualification", "", qbody); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	second := qualificationParticipant(t, f, "old-ingestion")
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("qualification", 1)
	cfg := aicontrol.IngestionSyntheticConfig("qualification", gate, func(_ string, raw []byte) (int64, bool) { return int64(len(raw)) + 64, true })
	old, e := aicontrol.NewSynthetic(context.Background(), f.DSN, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer old.Close()
	old.SetFault("after_intent_commit", func() error { return aicontrol.ErrUnavailable })
	oldRouter := accountapi.NewSyntheticIngestion(f.Service, f.Store, old, []byte("synthetic-ingestion-signing-key-32bytes"))
	w := qualificationRequest(t, second, oldRouter, "/api/profile/ingest", "old-ingestion", "", ingestionBody(t, second, "plan", nil))
	var plan ingestion.SubmissionPlanResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
		t.Fatal(w.Body.String())
	}
	extract := ingestionBody(t, second, "extract", map[string]any{"planReceipt": plan.PlanReceipt, "batchIndex": 0})
	_ = qualificationRequest(t, second, oldRouter, "/api/profile/ingest", "old-ingestion", "", extract)
	before, e := prior.ScopeStatus(context.Background(), f.Principal, "global")
	if e != nil || before.Occupied != 1 || before.Counted <= 1344 {
		t.Fatalf("old history %+v %v", before, e)
	}
	reviewGate := aicontrol.NewSyntheticGate()
	reviewGate.Reconcile("qualification", 1)
	h, e := aicontrol.NewSynthetic(context.Background(), f.DSN, aicontrol.ProfileReviewSyntheticConfig("qualification", reviewGate, func(_ string, raw []byte) (int64, bool) { return int64(len(raw)) + 64, true }))
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	provider := &aiadmission.Provider{T: t, Workflow: "profile_review"}
	_ = h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
		v, e := provider.Respond(ctx, p, r)
		return v, true, e
	})
	router := accountapi.NewSyntheticProfileReview(f.Service, f.Store, h, []byte("synthetic-ingestion-signing-key-32bytes"))
	for _, run := range []struct {
		f         *aiadmission.Fixture
		path, key string
		body      []byte
	}{{f, "/api/qualification-gaps", "old-qualification", qbody}, {second, "/api/profile/ingest", "old-ingestion", extract}} {
		w = qualificationRequest(t, run.f, router, run.path, run.key, "", run.body)
		if w.Code != 202 || len(provider.Calls) != 0 {
			t.Fatalf("old identity changed: %d %s %+v", w.Code, w.Body.String(), provider.Calls)
		}
	}
	w = qualificationRequest(t, second, router, "/api/profile/review", "blocked-review", "", second.Body(t, "/api/profile/review"))
	if w.Code != 429 || len(provider.Calls) != 0 {
		t.Fatalf("old hold bypass %d %s", w.Code, w.Body.String())
	}
	after, e := h.ScopeStatus(context.Background(), f.Principal, "global")
	if e != nil || after != before {
		t.Fatalf("copied pool %+v %+v %v", before, after, e)
	}
	w = qualificationRequest(t, f, router, "/api/profile/review", "new-review", "", f.Body(t, "/api/profile/review"))
	if w.Code != 200 || len(provider.Calls) != 2 {
		t.Fatalf("review %d %s", w.Code, w.Body.String())
	}
	after, e = h.ScopeStatus(context.Background(), f.Principal, "global")
	if e != nil || after.Occupied != before.Occupied || after.Counted <= before.Counted {
		t.Fatalf("new exposure not shared %+v %+v %v", before, after, e)
	}
}
func TestManagedProfileReviewActionLimitsCountFailuresAndDoNotReplenish(t *testing.T) {
	f, h, p, router := profileReviewFixture(t)
	body := f.Body(t, "/api/profile/review")
	// Explicit fake terminal failure counts as admitted work; no usage does not
	// imply zero expense. It avoids making sixty synthetic successful calls.
	_ = h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
		return &http.Response{StatusCode: 429, Header: make(http.Header), Body: http.NoBody}, true, nil
	})
	for i := 0; i < 30; i++ {
		if i == 6 {
			w := qualificationRequest(t, f, router, "/api/profile/review", "seventh-day", "", body)
			if w.Code != 429 {
				t.Fatalf("rolling Action cap bypass %d", w.Code)
			}
		}
		if i >= 6 {
			if _, e := f.DB.Exec(`UPDATE ai_fixture_action SET created_at=clock_timestamp()-interval '25 hours'`); e != nil {
				t.Fatal(e)
			}
		}
		w := qualificationRequest(t, f, router, "/api/profile/review", fmt.Sprintf("action-%d", i), "", body)
		if w.Code < 400 || w.Header().Get("X-CareerOS-Retry-Allowed") != "true" {
			t.Fatalf("admission %d %d %s", i, w.Code, w.Body.String())
		}
	}
	w := qualificationRequest(t, f, router, "/api/profile/review", "thirty-first", "", body)
	var n int
	if e := f.DB.QueryRow(`SELECT count(*) FROM ai_fixture_action WHERE workflow='profile_review'`).Scan(&n); e != nil || n != 30 || w.Code != 429 || len(p.Calls) != 0 {
		t.Fatalf("grant cap %d %d %v", w.Code, n, e)
	}
}
func TestManagedProfileReviewUnknownRestartAndRootChanges(t *testing.T) {
	f, h, p, router := profileReviewFixture(t)
	body := f.Body(t, "/api/profile/review")
	h.SetFault("after_intent_commit", func() error { return aicontrol.ErrUnavailable })
	_ = qualificationRequest(t, f, router, "/api/profile/review", "unknown", "", body)
	before, e := h.ScopeStatus(context.Background(), f.Principal, "global")
	if e != nil || before.Occupied != 1 || before.Counted <= 0 {
		t.Fatalf("unknown %+v %v", before, e)
	}
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("qualification", 1)
	recovered, e := aicontrol.NewSynthetic(context.Background(), f.DSN, aicontrol.ProfileReviewSyntheticConfig("qualification", gate, func(_ string, raw []byte) (int64, bool) { return int64(len(raw)) + 64, true }))
	if e != nil {
		t.Fatal(e)
	}
	defer recovered.Close()
	_ = recovered.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
		v, e := p.Respond(ctx, provider, r)
		return v, true, e
	})
	resumed := accountapi.NewSyntheticProfileReview(f.Service, f.Store, recovered, []byte("synthetic-ingestion-signing-key-32bytes"))
	for _, retry := range []string{"", "deliberate-but-unknown"} {
		w := qualificationRequest(t, f, resumed, "/.netlify/functions/account/api/profile/review", "unknown", retry, body)
		if w.Code == 200 || len(p.Calls) != 0 {
			t.Fatal("unknown restart resend")
		}
	}
	if _, e = f.DB.Exec(`UPDATE ai_fixture_dispatch SET intent_at=clock_timestamp()-interval '169 hours'`); e != nil {
		t.Fatal(e)
	}
	after, e := recovered.ScopeStatus(context.Background(), f.Principal, "global")
	if e != nil || after != before {
		t.Fatalf("unknown released %+v %+v", before, after)
	}
	var root map[string]any
	_ = json.Unmarshal(body, &root)
	root["locale"] = "pt-BR"
	changed, _ := json.Marshal(root)
	w := qualificationRequest(t, f, resumed, "/api/profile/review", "unknown", "", changed)
	if w.Code != 409 || len(p.Calls) != 0 {
		t.Fatal("changed root reused")
	}
	w = qualificationRequest(t, f, resumed, "/api/profile/review", "new-after-unknown", "", body)
	if w.Code != 429 || len(p.Calls) != 0 {
		t.Fatal("newAction bypassed unknown capacity")
	}
}
func TestManagedProfileReviewLastGlobalMoneyRace(t *testing.T) {
	f, h, p, router := profileReviewBoundFixture(t, func(string, []byte) (int64, bool) { return 64000, true })
	body := f.Body(t, "/api/profile/review")
	w := qualificationRequest(t, f, router, "/api/profile/review", "history", "", body)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	s, e := h.Open(context.Background(), f.Principal, "history", "profile_review", body)
	if e != nil {
		t.Fatal(e)
	}
	status, e := h.Status(context.Background(), f.Principal, s.ID())
	if e != nil {
		t.Fatal(e)
	}
	amount := int64(9978000) // second11000 + new11000 = exactly10USD shared pool.
	if e = h.SyntheticReviewer().ApplyEvidence(context.Background(), f.Principal, aicontrol.Evidence{ID: "historical-bill", Dispatch: status.Dispatches[0].ID, Provider: "openai", Kind: "bill", Amount: &amount, ReleaseFloor: true}); e != nil {
		t.Fatal(e)
	}
	_, _ = f.DB.Exec(`UPDATE ai_fixture_dispatch SET intent_at=clock_timestamp()-interval '25 hours'`)
	second := qualificationParticipant(t, f, "race-review-second")
	third := qualificationParticipant(t, f, "race-review-third")
	var mu sync.Mutex
	_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
		mu.Lock()
		defer mu.Unlock()
		v, e := p.Respond(ctx, provider, r)
		return v, true, e
	})
	start := make(chan struct{})
	done := make(chan int, 2)
	for _, who := range []*aiadmission.Fixture{second, third} {
		who := who
		body := who.Body(t, "/api/profile/review")
		go func() {
			<-start
			w := qualificationRequest(t, who, router, "/api/profile/review", "last-money", "", body)
			done <- w.Code
		}()
	}
	close(start)
	a, b := <-done, <-done
	scope, e := h.ScopeStatus(context.Background(), f.Principal, "global")
	if e != nil || scope.Counted != 10000000 || len(p.Calls) != 3 || a != 429 || b != 429 {
		t.Fatalf("race %d/%d %+v calls%+v %v", a, b, scope, p.Calls, e)
	}
}

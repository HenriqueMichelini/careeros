package accountapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	ingestion "professional-information-repo/backend/profile-ingestion"
	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/cvsource"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestManagedCVPreservesPreEnableWorkflowHistories(t *testing.T) {
	f, prior, _, qrouter := qualificationFixture(t)
	qbody := f.Body(t, "/api/qualification-gaps")
	if w := qualificationRequest(t, f, qrouter, "/api/qualification-gaps", "old-q", "", qbody); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("qualification", 1)
	old, e := aicontrol.NewSynthetic(context.Background(), f.DSN, aicontrol.ProfileReviewSyntheticConfig("qualification", gate, func(_ string, raw []byte) (int64, bool) { return int64(len(raw)) + 64, true }))
	if e != nil {
		t.Fatal(e)
	}
	defer old.Close()
	provider := &aiadmission.Provider{T: t, Workflow: "profile_review"}
	_ = old.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
		v, e := provider.Respond(ctx, p, r)
		return v, true, e
	})
	oldRouter := accountapi.NewSyntheticProfileReview(f.Service, f.Store, old, []byte("synthetic-ingestion-signing-key-32bytes"))
	rbody := f.Body(t, "/api/profile/review")
	if w := qualificationRequest(t, f, oldRouter, "/api/profile/review", "old-review", "", rbody); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	second := qualificationParticipant(t, f, "old-ingestion-cv")
	w := qualificationRequest(t, second, oldRouter, "/api/profile/ingest", "old-ingest", "", ingestionBody(t, second, "plan", nil))
	var plan ingestion.SubmissionPlanResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
		t.Fatal(w.Body.String())
	}
	old.SetFault("after_intent_commit", func() error { return aicontrol.ErrUnavailable })
	extract := ingestionBody(t, second, "extract", map[string]any{"planReceipt": plan.PlanReceipt, "batchIndex": 0})
	_ = qualificationRequest(t, second, oldRouter, "/api/profile/ingest", "old-ingest", "", extract)
	before, e := prior.ScopeStatus(context.Background(), f.Principal, "global")
	if e != nil || before.Occupied != 1 || before.Counted <= 1344 {
		t.Fatal(before, e)
	}
	newGate := aicontrol.NewSyntheticGate()
	newGate.Reconcile("qualification", 1)
	h, e := aicontrol.NewSynthetic(context.Background(), f.DSN, aicontrol.StandaloneCVSyntheticConfig("qualification", newGate, func(_ string, raw []byte) (int64, bool) { return int64(len(raw)) + 64, true }))
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	p := &cvProvider{}
	_ = h.SetQualificationResponder(p.respond)
	r := accountapi.NewSyntheticStandaloneCV(f.Service, f.Store, h, []byte("synthetic-ingestion-signing-key-32bytes"))
	for _, run := range []struct {
		f         *aiadmission.Fixture
		path, key string
		body      []byte
	}{{f, "/api/qualification-gaps", "old-q", qbody}, {f, "/api/profile/review", "old-review", rbody}, {second, "/api/profile/ingest", "old-ingest", extract}} {
		w = qualificationRequest(t, run.f, r, run.path, run.key, "", run.body)
		if w.Code != 202 || len(p.calls) != 0 {
			t.Fatalf("old identity %d %s", w.Code, w.Body.String())
		}
	}
	w = qualificationRequest(t, second, r, "/api/cv/generate", "blocked", "", managedCVBody())
	if w.Code != 429 || len(p.calls) != 0 {
		t.Fatal("old unknown bypass", w.Code, w.Body.String())
	}
	after, e := h.ScopeStatus(context.Background(), f.Principal, "global")
	if e != nil || after != before {
		t.Fatal("copied pools", before, after, e)
	}
	w = qualificationRequest(t, f, r, "/api/cv/generate", "cv", "", managedCVBody())
	if w.Code != 200 || len(p.calls) != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
	after, e = h.ScopeStatus(context.Background(), f.Principal, "global")
	if e != nil || after.Occupied != before.Occupied || after.Counted <= before.Counted {
		t.Fatal("unshared CV exposure", before, after, e)
	}
}

func TestManagedCVUnknownRestartAndSharedLastMoneyRace(t *testing.T) {
	t.Run("unknown restart", func(t *testing.T) {
		f, h, p, r := managedCVFixture(t, nil)
		h.SetFault("after_intent_commit", func() error { return aicontrol.ErrUnavailable })
		_ = qualificationRequest(t, f, r, "/api/cv/generate", "unknown", "", managedCVBody())
		before, e := h.ScopeStatus(context.Background(), f.Principal, "global")
		if e != nil || before.Occupied != 1 || before.Counted <= 0 {
			t.Fatal(before, e)
		}
		gate := aicontrol.NewSyntheticGate()
		gate.Reconcile("qualification", 1)
		resumed, e := aicontrol.NewSynthetic(context.Background(), f.DSN, aicontrol.StandaloneCVSyntheticConfig("qualification", gate, func(_ string, raw []byte) (int64, bool) { return int64(len(raw)) + 64, true }))
		if e != nil {
			t.Fatal(e)
		}
		defer resumed.Close()
		_ = resumed.SetQualificationResponder(p.respond)
		r = accountapi.NewSyntheticStandaloneCV(f.Service, f.Store, resumed, []byte("synthetic-ingestion-signing-key-32bytes"))
		for _, retry := range []string{"", "not-known-failure"} {
			w := qualificationRequest(t, f, r, "/.netlify/functions/account/api/cv/generate", "unknown", retry, managedCVBody())
			if w.Code == 200 || len(p.calls) != 0 {
				t.Fatal("unknown resend")
			}
		}
		_, _ = f.DB.Exec(`UPDATE ai_fixture_dispatch SET intent_at=clock_timestamp()-interval '169 hours'`)
		after, e := resumed.ScopeStatus(context.Background(), f.Principal, "global")
		if e != nil || after != before {
			t.Fatal("unknown release", before, after, e)
		}
		w := qualificationRequest(t, f, r, "/api/cv/generate", "new-after-unknown", "", managedCVBody())
		if w.Code != 429 || len(p.calls) != 0 {
			t.Fatal("unknown newAction capacity bypass")
		}
	})
	t.Run("global money", func(t *testing.T) {
		f, h, p, r := managedCVFixture(t, func(string, []byte) (int64, bool) { return 64000, true })
		w := qualificationRequest(t, f, r, "/api/cv/generate", "history", "", managedCVBody())
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		normalized, _ := cvsource.Normalize(managedCVBody())
		s, e := h.Open(context.Background(), f.Principal, "history", "cv_generation", normalized)
		if e != nil {
			t.Fatal(e)
		}
		status, e := h.Status(context.Background(), f.Principal, s.ID())
		if e != nil {
			t.Fatal(e)
		}
		amount := int64(9980000)
		if e = h.SyntheticReviewer().ApplyEvidence(context.Background(), f.Principal, aicontrol.Evidence{ID: "historical-bill", Dispatch: status.Dispatches[0].ID, Provider: "openai", Kind: "bill", Amount: &amount, ReleaseFloor: true}); e != nil {
			t.Fatal(e)
		}
		_, _ = f.DB.Exec(`UPDATE ai_fixture_dispatch SET intent_at=clock_timestamp()-interval '25 hours'`)
		second := qualificationParticipant(t, f, "cv-race2")
		third := qualificationParticipant(t, f, "cv-race3")
		var mu sync.Mutex
		_ = h.SetQualificationResponder(func(ctx context.Context, provider string, req *http.Request) (*http.Response, bool, error) {
			mu.Lock()
			defer mu.Unlock()
			return p.respond(ctx, provider, req)
		})
		start := make(chan struct{})
		done := make(chan int, 2)
		for _, who := range []*aiadmission.Fixture{second, third} {
			go func(who *aiadmission.Fixture) {
				<-start
				w := qualificationRequest(t, who, r, "/api/cv/generate", "last", "", managedCVBody())
				done <- w.Code
			}(who)
		}
		close(start)
		a, b := <-done, <-done
		scope, e := h.ScopeStatus(context.Background(), f.Principal, "global")
		if e != nil || scope.Counted != 10000000 || len(p.calls) != 3 || !((a == 200 && b == 429) || (a == 429 && b == 200)) {
			t.Fatalf("race%d/%d scope%+v calls%v %v", a, b, scope, p.calls, e)
		}
	})
}

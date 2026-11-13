package accountapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestManagedDraftPublicAuthorityAndOverrideNegatives(t *testing.T) {
	f, _, provider, _, router, input := managedDraftFixture(t)
	body, _ := json.Marshal(input)
	other := *f
	// A separate signed-in owner with no pilot grant cannot generate.
	ctx := context.Background()
	start, err := f.Service.BeginLogin(ctx, "google", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(start.URL)
	if err != nil {
		t.Fatal(err)
	}
	other.Login, err = f.Service.CompleteLogin(ctx, "google", parsed.Query().Get("state"), "draft-ungranted", start.BindingToken)
	if err != nil {
		t.Fatal(err)
	}
	other.Principal = other.Login.Principal
	for i, alias := range []string{"/api/drafts", "/api/application-draft", "/.netlify/functions/application-draft", "/.netlify/functions/account/api/application-draft"} {
		key := "alias-" + string(rune('a'+i))
		overrides := []string{`"reviewResume":false`, `"reviewArtifacts":false`, `"dispatchId":"forged"`, `"apiKey":"not-a-key"`}
		for _, override := range overrides {
			raw := append(append([]byte{}, body[:len(body)-1]...), []byte(","+override+"}")...)
			if got := qualificationRequest(t, f, router, alias, key, "", raw); got.Code != 422 {
				t.Fatalf("%s accepted override %s: %d", alias, override, got.Code)
			}
		}
		withKeys := f.Request(t, ctx, alias, body)
		withKeys.Header.Set("X-CareerOS-Action-Key", key)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, withKeys)
		if response.Code != 400 {
			t.Fatalf("%s accepted caller credentials: %d", alias, response.Code)
		}
		if got := qualificationRequest(t, &other, router, alias, key, "", body); got.Code != 403 {
			t.Fatalf("%s permitted ungranted owner: %d", alias, got.Code)
		}
	}
	var count int
	if err = f.DB.QueryRow(`SELECT count(*) FROM ai_fixture_action WHERE workflow='application_draft'`).Scan(&count); err != nil || count != 0 || len(provider.Calls) != 0 {
		t.Fatalf("invalid requests admitted work: actions=%d sends=%v err=%v", count, provider.Calls, err)
	}
}

func TestManagedDraftCompleteRequestBoundsAndLocalReviewDisposition(t *testing.T) {
	for _, test := range []struct {
		name  string
		sends int
		code  int
	}{
		{"unavailable_bound", 0, 403},
		{"typesafe_32001", 0, 429},
		{"generation_64001", 1, 429},
		{"artifact_64001", 3, 429},
		{"resume_local_not_needed", 3, 422},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, _, p, _, router, in := managedDraftBoundFixture(t, func(provider string, raw []byte) (int64, bool) {
				if test.name == "unavailable_bound" {
					return 0, false
				}
				if provider == "typesafe" {
					if !strings.Contains(string(raw), inPosting) {
						t.Error("gate omitted the complete original")
					}
					if test.name == "typesafe_32001" {
						return 32_001, true
					}
					return 32_000, true
				}
				var envelope struct {
					Format struct {
						Schema struct{ Name string } `json:"json_schema"`
					} `json:"response_format"`
				}
				_ = json.Unmarshal(raw, &envelope)
				stage := envelope.Format.Schema.Name
				if stage == "artifact_support" && (!strings.Contains(string(raw), "I use Java.") || !strings.Contains(string(raw), inPosting)) {
					t.Error("artifact bound omitted response-dependent or original material")
				}
				if test.name == "generation_64001" && stage == "application_draft" || test.name == "artifact_64001" && stage == "artifact_support" {
					return 64_001, true
				}
				return 64_000, true
			})
			if test.name == "resume_local_not_needed" {
				p.NoResumeClaims = true
			}
			body, _ := json.Marshal(in)
			got := qualificationRequest(t, f, router, "/api/drafts", test.name, "", body)
			if got.Code != test.code || len(p.Calls) != test.sends {
				t.Fatalf("bound/disposition: %d %s sends=%v", got.Code, got.Body.String(), p.Calls)
			}
			if test.name == "resume_local_not_needed" && p.Calls[2].Stage != "artifact_support" {
				t.Fatalf("local disposition suppressed the wrong required leg: %v", p.Calls)
			}
			status := managedDraftStatus(t, f, router, test.name)
			if status.Code != 202 || strings.Contains(status.Body.String(), `"bundle"`) || strings.Contains(status.Body.String(), `"retryAllowed":true`) {
				t.Fatalf("incomplete work became ready or retryable: %d %s", status.Code, status.Body.String())
			}
		})
	}
}

const inPosting = "Java required."

func managedDraftStatus(t *testing.T, f *aiadmission.Fixture, router http.Handler, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := f.Request(t, context.Background(), "/api/drafts/"+key, nil)
	req.Method = http.MethodGet
	req.Header.Del("X-OpenAI-Api-Key")
	req.Header.Del("X-TypeSafe-Api-Key")
	got := httptest.NewRecorder()
	router.ServeHTTP(got, req)
	return got
}

func TestManagedDraftDurableIntentSurvivesReopenAndCustomerClosure(t *testing.T) {
	f, h, p, _, router, in := managedDraftFixture(t)
	h.SetFault("after_intent_commit", func() error { return errors.New("synthetic crash before provider handoff") })
	body, _ := json.Marshal(in)
	const key = "lost-draft-intent"
	first := qualificationRequest(t, f, router, "/api/drafts", key, "", body)
	if first.Code < 400 || len(p.Calls) != 0 {
		t.Fatalf("interrupted permit sent: %d %v", first.Code, p.Calls)
	}
	reopened, recovery := openManagedDraftFixture(t, f.DSN, h.Namespace(), managedDraftFullBound)
	var sends atomic.Int32
	if err := reopened.SetQualificationResponder(func(_ context.Context, _ string, _ *http.Request) (*http.Response, bool, error) {
		sends.Add(1)
		return nil, false, errors.New("unexpected handoff")
	}); err != nil {
		t.Fatal(err)
	}
	next := accountapi.NewSyntheticDraft(f.Service, f.Store, reopened, recovery)
	status := managedDraftStatus(t, f, next, key)
	var before draftrecovery.Result
	if status.Code != 202 || json.Unmarshal(status.Body.Bytes(), &before) != nil || before.State != "incomplete" || before.RetryAllowed || before.Bundle != nil {
		t.Fatalf("unknown status: %d %s", status.Code, status.Body.String())
	}
	if got := qualificationRequest(t, f, next, "/api/drafts", key, "", body); got.Code != 202 || got.Body.String() != status.Body.String() {
		t.Fatalf("duplicate changed identity: %d %s", got.Code, got.Body.String())
	}
	if got := qualificationRequest(t, f, next, "/api/drafts", key, "unknown-retry", body); got.Code != 403 {
		t.Fatalf("unknown became retryable: %d", got.Code)
	}
	if _, err := f.DB.Exec(`UPDATE careeros_draft_actions SET deadline=clock_timestamp()-interval '1 second' WHERE action_id=$1`, before.Action.ID); err != nil {
		t.Fatal(err)
	}
	closed := managedDraftStatus(t, f, next, key)
	if closed.Code != 410 || !strings.Contains(closed.Body.String(), `"cleanupQueued":true`) {
		t.Fatalf("expired customer result did not close: %d %s", closed.Code, closed.Body.String())
	}
	if got := qualificationRequest(t, f, next, "/api/drafts", "new-while-unknown", "", body); got.Code != 429 {
		t.Fatalf("closed unknown released its active Action capacity: %d %s", got.Code, got.Body.String())
	}
	for _, scope := range []string{"account:" + f.Principal.AccountID, "global", "provider:typesafe"} {
		state, err := reopened.ScopeStatus(context.Background(), f.Principal, scope)
		if err != nil || state.Counted != 1344 || state.Occupied != 1 {
			t.Fatalf("closed unknown changed %s hold: %+v %v", scope, state, err)
		}
	}
	if sends.Load() != 0 {
		t.Fatalf("reopened authority resent %d times", sends.Load())
	}
}

func TestManagedDraftTimeoutActuallyClampsTheInFlightProvider(t *testing.T) {
	f, h, _, _, router, in := managedDraftFixture(t)
	const key = "remaining-draft-foreground"
	h.SetFault("after_create_commit", func() error {
		_, err := f.DB.Exec(`UPDATE ai_fixture_action SET created_at=clock_timestamp()-interval '24 seconds' WHERE namespace=$1 AND creation_key=$2`, h.Namespace(), key)
		return err
	})
	var reached atomic.Int32
	if err := h.SetQualificationResponder(func(ctx context.Context, _ string, _ *http.Request) (*http.Response, bool, error) {
		reached.Add(1)
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > time.Second {
			t.Error("actual provider context exceeds remaining durable 25-second budget")
		}
		<-ctx.Done()
		return nil, false, ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(in)
	response := qualificationRequest(t, f, router, "/api/drafts", key, "", body)
	if response.Code != 504 || reached.Load() != 1 {
		t.Fatalf("near-expiry attempt did not reach and time out its actual provider once: %d %s reached=%d", response.Code, response.Body.String(), reached.Load())
	}
	status := managedDraftStatus(t, f, router, key)
	if status.Code != 202 || strings.Contains(status.Body.String(), `"retryAllowed":true`) {
		t.Fatalf("timed-out attempt advertised Retry: %d %s", status.Code, status.Body.String())
	}
	state, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || state.Counted != 1344 || state.Occupied != 1 {
		t.Fatalf("in-flight timeout released hold: %+v %v", state, err)
	}
}

func TestManagedDraftGrantRevocationWinsBeforePublication(t *testing.T) {
	f, h, p, _, router, in := managedDraftFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	if err := h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
		if aidiagnostics.StageName(ctx) == "artifact_support" {
			close(entered)
			<-release
		}
		response, err := p.Respond(ctx, provider, r)
		return response, true, err
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(in)
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- qualificationRequest(t, f, router, "/api/drafts", "revoked-draft", "", body) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("artifact support was not reached")
	}
	identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "fixture-account"}
	_, err := f.Store.PilotCommand(context.Background(), f.Principal, "revoke-draft-owner", accountstore.PilotCommand{Kind: "revoke", Identity: identity})
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	got := <-result
	status := managedDraftStatus(t, f, router, "revoked-draft")
	if got.Code < 400 || status.Code != 410 || strings.Contains(status.Body.String(), `"bundle"`) {
		t.Fatalf("revocation failed to fence publication: POST %d %s GET %d %s", got.Code, got.Body.String(), status.Code, status.Body.String())
	}
	if len(p.Calls) != 4 {
		t.Fatalf("wrong reached schedule: %v", p.Calls)
	}
}

func TestManagedDraftCrossInstanceSharesPreexistingQualificationCapacity(t *testing.T) {
	f, prior, _, old := qualificationFixture(t)
	var oldSends atomic.Int32
	if err := prior.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
		oldSends.Add(1)
		return nil, false, errors.New("unacknowledged synthetic qualification send")
	}); err != nil {
		t.Fatal(err)
	}
	const qualKey = "qualification-before-draft"
	qualBody := f.Body(t, "/api/qualification-gaps")
	if first := qualificationRequest(t, f, old, "/api/qualification-gaps", qualKey, "", qualBody); first.Code < 400 || oldSends.Load() != 1 {
		t.Fatalf("old qualification did not leave the expected unknown: %d sends=%d", first.Code, oldSends.Load())
	}
	second := qualificationParticipant(t, f, "draft-second")
	third := qualificationParticipant(t, f, "draft-third")
	entered, release := make(chan struct{}, 2), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	makeRouter := func() (*aicontrol.Harness, http.Handler, *aiadmission.Provider) {
		h, store := openManagedDraftFixture(t, f.DSN, prior.Namespace(), managedDraftFullBound)
		p := &aiadmission.Provider{T: t, Workflow: "managed_application_draft"}
		if err := h.SetQualificationResponder(func(ctx context.Context, provider string, req *http.Request) (*http.Response, bool, error) {
			if provider == "typesafe" {
				entered <- struct{}{}
				<-release
			}
			response, err := p.Respond(ctx, provider, req)
			return response, true, err
		}); err != nil {
			t.Fatal(err)
		}
		return h, accountapi.NewSyntheticDraft(f.Service, f.Store, h, store), p
	}
	h, first, firstProvider := makeRouter()
	_, secondRouter, secondProvider := makeRouter()
	var doc profiledocument.Document
	if err := json.Unmarshal(f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	input := draftrecovery.Input{Profile: draftrecovery.Source{ID: doc.ID, Revision: doc.Revision}, Job: draftrecovery.Job{Posting: inPosting}, Language: "en"}
	body, _ := json.Marshal(input)
	dup := qualificationRequest(t, f, first, "/api/qualification-gaps", qualKey, "", qualBody)
	var oldStatus aicontrol.QualificationStatus
	if dup.Code != 202 || json.Unmarshal(dup.Body.Bytes(), &oldStatus) != nil || oldStatus.State != "unknown" {
		t.Fatalf("enabling Draft changed old qualification identity: %d %s", dup.Code, dup.Body.String())
	}
	if blocked := qualificationRequest(t, f, first, "/api/drafts", "old-owner-draft", "", body); blocked.Code != 429 {
		t.Fatalf("old qualification no longer occupied its account slot: %d %s", blocked.Code, blocked.Body.String())
	}
	done := make(chan int, 2)
	go func() { done <- qualificationRequest(t, second, first, "/api/drafts", "last-slot", "", body).Code }()
	go func() {
		done <- qualificationRequest(t, third, secondRouter, "/api/drafts", "last-slot", "", body).Code
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("last available Action did not reach the real gate")
	}
	select {
	case code := <-done:
		if code != 429 {
			t.Fatalf("second instance did not lose the shared active capacity race: %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("both instances may have entered the single remaining slot")
	}
	close(release)
	released = true
	// Wait for the sole admitted Action before reading the providers' witness.
	if code := <-done; code != 201 {
		t.Fatalf("winner did not publish: %d", code)
	}
	scope, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || scope.Counted != 36_688 || scope.Occupied != 1 || len(firstProvider.Calls)+len(secondProvider.Calls) != 4 {
		t.Fatalf("cross-instance history or exposure changed: %+v %v", scope, err)
	}
}

func TestManagedDraftCountsFailedActionsAgainstDayAndGrantCaps(t *testing.T) {
	f, h, _, _, router, in := managedDraftFixture(t)
	var sends atomic.Int32
	if err := h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
		sends.Add(1)
		w := httptest.NewRecorder()
		w.WriteHeader(429)
		return w.Result(), true, nil
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(in)
	for i := 0; i < 10; i++ {
		if i > 0 && i%3 == 0 {
			daily := qualificationRequest(t, f, router, "/api/drafts", fmt.Sprintf("day-denied-%d", i), "", body)
			if daily.Code != 429 || sends.Load() != int32(i) {
				t.Fatalf("rolling day cap at %d: %d sends=%d", i, daily.Code, sends.Load())
			}
			if _, err := f.DB.Exec(`UPDATE ai_fixture_action SET created_at=clock_timestamp()-interval '25 hours' WHERE namespace=$1 AND workflow='application_draft'`, h.Namespace()); err != nil {
				t.Fatal(err)
			}
		}
		first := qualificationRequest(t, f, router, "/api/drafts", fmt.Sprintf("failed-draft-%d", i), "", body)
		if first.Code < 400 || sends.Load() != int32(i+1) {
			t.Fatalf("expected one known failed gate for admitted Action %d: %d sends=%d", i, first.Code, sends.Load())
		}
	}
	grant := qualificationRequest(t, f, router, "/api/drafts", "grant-eleven", "", body)
	var counted int
	if err := f.DB.QueryRow(`SELECT count(*) FROM ai_fixture_action WHERE namespace=$1 AND workflow='application_draft'`, h.Namespace()).Scan(&counted); err != nil || grant.Code != 429 || sends.Load() != 10 || counted != 10 {
		t.Fatalf("grant cap: %d sends=%d admitted=%d err=%v", grant.Code, sends.Load(), counted, err)
	}
}

func TestManagedDraftRequiredAnswerCatalogAndPartialReplacement(t *testing.T) {
	f, _, provider, _, router, in := managedDraftFixture(t)
	const question = "Explain your Java experience."
	provider.DraftAnswerRequest = question
	in.Job.Posting = "Java required. " + question
	in.Answers = []draftrecovery.Answer{{Requirement: question, UserContext: "I use Java."}}
	in.Confirmations = []draftrecovery.Answer{{Kind: "experience", Requirement: "Java use"}}
	body, _ := json.Marshal(in)
	const initial = "original-answer-draft"
	first := qualificationRequest(t, f, router, "/api/drafts", initial, "", body)
	var original draftrecovery.Result
	if first.Code != 201 || json.Unmarshal(first.Body.Bytes(), &original) != nil || original.Bundle == nil {
		t.Fatalf("complete required answer was withheld: %d %s", first.Code, first.Body.String())
	}
	answers := original.Bundle.Answers
	if answers.State != "complete" || len(answers.Requests) != 1 || answers.Requests[0].Source.Quote != question ||
		answers.Requests[0].Source.Start != 15 || answers.Text != "I use Java." ||
		len(original.Bundle.Support.Completion.AnswerRequests) != 1 ||
		original.Bundle.Support.Completion.AnswerRequests[0].Source != answers.Requests[0].Source {
		t.Fatalf("answer catalog did not preserve complete original correspondence: %+v", answers)
	}
	foundTemporary := false
	for _, fact := range original.Bundle.Support.Facts {
		if fact.Reference.Kind == "answer" {
			foundTemporary = true
		}
	}
	if !foundTemporary {
		t.Fatal("exact support omitted the used application-only source")
	}
	if original.Context == nil || len(original.Context.Confirmations) != 1 || original.Context.Confirmations[0] != in.Confirmations[0] {
		t.Fatal("application-only confirmation lost its explicit kind or optional empty context")
	}
	for i, mode := range []string{"missing-required-answer", "missing-independent-catalog"} {
		provider.OmitDraftAnswer = i == 0
		provider.OmitDraftCatalog = i == 1
		failed := qualificationRequest(t, f, router, "/api/drafts", mode, "", body)
		if failed.Code != 422 || len(provider.Calls) != (i+2)*4 {
			t.Fatalf("partial answer/capability became ready or changed required calls: %d %s %v", failed.Code, failed.Body.String(), provider.Calls)
		}
		status := managedDraftStatus(t, f, router, mode)
		if status.Code != 202 || strings.Contains(status.Body.String(), `"bundle"`) || strings.Contains(status.Body.String(), `"retryAllowed":true`) {
			t.Fatalf("partial replacement retained content or enabled Retry: %d %s", status.Code, status.Body.String())
		}
	}
	if got := managedDraftStatus(t, f, router, initial); got.Code != 200 || got.Body.String() != first.Body.String() || len(provider.Calls) != 12 {
		t.Fatalf("partial replacements disturbed the previous complete version: %d %s", got.Code, got.Body.String())
	}
	var effects, bundles int
	if err := f.DB.QueryRow(`SELECT count(*) FILTER(WHERE a.customer_effect OR a.customer_reserved),count(*) FILTER(WHERE c.bundle IS NOT NULL) FROM ai_fixture_action a LEFT JOIN careeros_draft_content c ON c.namespace=a.namespace AND c.action_id=a.id`).Scan(&effects, &bundles); err != nil || effects != 0 || bundles != 1 {
		t.Fatalf("partial replacement created retained results or allowance effects: effects=%d bundles=%d err=%v", effects, bundles, err)
	}
}

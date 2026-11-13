package accountapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func managedDraftFixture(t *testing.T) (*aiadmission.Fixture, *aicontrol.Harness, *aiadmission.Provider, *draftrecovery.Store, http.Handler, draftrecovery.Input) {
	t.Helper()
	return managedDraftBoundFixture(t, managedDraftFullBound)
}

func managedDraftFullBound(provider string, raw []byte) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	if provider == "typesafe" {
		return 32_000, true
	}
	return 64_000, true
}

func openManagedDraftFixture(t *testing.T, dsn, namespace string, bound func(string, []byte) (int64, bool)) (*aicontrol.Harness, *draftrecovery.Store) {
	t.Helper()
	ctx := context.Background()
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile(namespace, 1)
	cfg := aicontrol.ManagedDraftSyntheticConfig(namespace, gate, bound)
	cfg.ExpiresAt = time.Now().Add(time.Hour)
	h, err := aicontrol.NewSynthetic(ctx, dsn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	bindingKey := []byte("synthetic-managed-draft-binding-key")
	store, err := draftrecovery.OpenManaged(ctx, dsn, h, bindingKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return h, store
}

func managedDraftBoundFixture(t *testing.T, bound func(string, []byte) (int64, bool)) (*aiadmission.Fixture, *aicontrol.Harness, *aiadmission.Provider, *draftrecovery.Store, http.Handler, draftrecovery.Input) {
	t.Helper()
	f, _, _, _ := qualificationFixture(t)
	h, store := openManagedDraftFixture(t, f.DSN, "managed-draft", bound)
	provider := &aiadmission.Provider{T: t, Workflow: "managed_application_draft"}
	if err := h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
		response, err := provider.Respond(ctx, p, r)
		return response, true, err
	}); err != nil {
		t.Fatal(err)
	}
	router := accountapi.NewSyntheticDraft(f.Service, f.Store, h, store)
	var doc profiledocument.Document
	if err := json.Unmarshal(f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	input := draftrecovery.Input{
		Profile:  draftrecovery.Source{ID: doc.ID, Revision: doc.Revision},
		Job:      draftrecovery.Job{Posting: "Java required."},
		Language: "en",
	}
	return f, h, provider, store, router, input
}

func TestManagedDraftPublishesActualWholeVersionAndRecoversWithoutSending(t *testing.T) {
	f, h, provider, _, router, input := managedDraftFixture(t)
	ctx := context.Background()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	const key = "managed-draft-whole-version"
	first := qualificationRequest(t, f, router, "/api/drafts", key, "", body)
	var created draftrecovery.Result
	if first.Code != http.StatusCreated || json.Unmarshal(first.Body.Bytes(), &created) != nil ||
		created.State != "ready" || created.Version != 1 || created.Bundle == nil ||
		created.Bundle.Resume == "" || created.Bundle.CoverLetter == "" || created.Bundle.RoleSummary == "" {
		t.Fatalf("actual managed Draft did not publish a complete version: %d %s", first.Code, first.Body.String())
	}
	wantCalls := []aiadmission.Call{
		{Provider: "typesafe", Stage: "field_validation"},
		{Provider: "openai", Stage: "generation"},
		{Provider: "openai", Stage: "resume_support"},
		{Provider: "openai", Stage: "artifact_support"},
	}
	if !reflect.DeepEqual(provider.Calls, wantCalls) {
		t.Fatalf("actual sends = %v, want %v", provider.Calls, wantCalls)
	}
	for _, scope := range []string{"account:" + f.Principal.AccountID, "global"} {
		got, err := h.ScopeStatus(ctx, f.Principal, scope)
		if err != nil || got.Counted != 35_344 || got.Occupied != 0 {
			t.Fatalf("scope %s = %+v, %v", scope, got, err)
		}
	}
	for _, path := range []string{"/api/drafts/" + key, "/.netlify/functions/account/api/drafts/" + key + "/versions/1"} {
		r := f.Request(t, ctx, path, nil)
		r.Method = http.MethodGet
		r.Header.Del("X-OpenAI-Api-Key")
		r.Header.Del("X-TypeSafe-Api-Key")
		got := httptest.NewRecorder()
		router.ServeHTTP(got, r)
		if got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), first.Body.Bytes()) {
			t.Fatalf("exact recovery %s: %d %s", path, got.Code, got.Body.String())
		}
	}
	replay := qualificationRequest(t, f, router, "/api/drafts", key, "", body)
	if replay.Code != http.StatusOK || !bytes.Equal(replay.Body.Bytes(), first.Body.Bytes()) ||
		!reflect.DeepEqual(provider.Calls, wantCalls) {
		t.Fatalf("replay changed the version or resent: %d %s, sends=%v", replay.Code, replay.Body.String(), provider.Calls)
	}
}

func TestManagedDraftAuthenticQualificationReuseKeepsSeparateActions(t *testing.T) {
	f, h, p, _, router, input := managedDraftFixture(t)
	analysis := qualificationRequest(t, f, router, "/api/qualification-gaps", "analysis", "", f.Body(t, "/api/qualification-gaps"))
	var output struct {
		JobContext json.RawMessage `json:"jobContext"`
	}
	proof := analysis.Header().Get("X-CareerOS-Draft-Context-Proof")
	if analysis.Code != 200 || json.Unmarshal(analysis.Body.Bytes(), &output) != nil ||
		len(output.JobContext) == 0 || proof == "" || len(p.Calls) != 3 {
		t.Fatalf("completed qualification proof: %d %s proof=%t calls=%v", analysis.Code, analysis.Body.String(), proof != "", p.Calls)
	}
	makeBody := func(context json.RawMessage, proof string) []byte {
		t.Helper()
		body, err := json.Marshal(struct {
			draftrecovery.Input
			JobContext         json.RawMessage `json:"jobContext"`
			QualificationProof string          `json:"qualificationProof"`
		}{input, context, proof})
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	tampered := json.RawMessage(strings.Replace(string(output.JobContext), `"required"`, `"preferred"`, 1))
	for key, body := range map[string][]byte{
		"forged":   makeBody(output.JobContext, proof+"x"),
		"changed":  makeBody(tampered, proof),
		"unsigned": makeBody(output.JobContext, ""),
	} {
		got := qualificationRequest(t, f, router, "/api/application-draft", key, "", body)
		if got.Code < 400 || len(p.Calls) != 3 {
			t.Fatalf("%s reused unauthenticated context: %d sends=%v", key, got.Code, p.Calls)
		}
	}
	body := makeBody(output.JobContext, proof)
	other := qualificationParticipant(t, f, "foreign-draft-reuse")
	if got := qualificationRequest(t, other, router, "/api/drafts", "foreign", "", body); got.Code < 400 || len(p.Calls) != 3 {
		t.Fatalf("foreign reuse sent: %d sends=%v", got.Code, p.Calls)
	}
	draft := qualificationRequest(t, f, router, "/.netlify/functions/application-draft", "draft-from-analysis", "", body)
	var result draftrecovery.Result
	if draft.Code != 201 || json.Unmarshal(draft.Body.Bytes(), &result) != nil || result.State != "ready" ||
		result.Action.ID == analysis.Header().Get("X-CareerOS-Action") || len(p.Calls) != 7 {
		t.Fatalf("reused Draft did not run only its separate four legs: %d %s sends=%v", draft.Code, draft.Body.String(), p.Calls)
	}
	scope, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || scope.Counted != 60_688 {
		t.Fatalf("shared qualification + Draft exposure = %+v, %v", scope, err)
	}
	// A valid-looking changed context can still use the inherited qualification
	// rerun's independent assessment. That pass-through cannot mint provenance.
	var rerun map[string]json.RawMessage
	_ = json.Unmarshal(f.Body(t, "/api/qualification-gaps"), &rerun)
	rerun["jobContext"] = tampered
	rerunBody, _ := json.Marshal(rerun)
	rerunResult := qualificationRequest(t, f, router, "/api/qualification-gaps", "supplied-rerun", "", rerunBody)
	if rerunResult.Code != 200 || rerunResult.Header().Get("X-CareerOS-Draft-Context-Proof") != "" || len(p.Calls) != 9 {
		t.Fatalf("supplied-context qualification rerun changed behavior or minted a proof: %d sends=%v", rerunResult.Code, p.Calls)
	}
}

func TestManagedDraftKnownFailureRetryKeepsOriginalActionAndDeadline(t *testing.T) {
	f, h, provider, _, router, input := managedDraftFixture(t)
	var sends int
	if err := h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
		sends++
		if sends == 2 {
			failure := httptest.NewRecorder()
			failure.WriteHeader(429)
			_, _ = failure.WriteString(`{"error":"synthetic_rate_limit"}`)
			return failure.Result(), true, nil
		}
		response, err := provider.Respond(ctx, p, r)
		return response, true, err
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(input)
	const key = "known-draft-failure"
	first := qualificationRequest(t, f, router, "/api/drafts", key, "", body)
	if first.Code < 400 || sends != 2 {
		t.Fatalf("known failure: %d %s sends=%d", first.Code, first.Body.String(), sends)
	}
	duplicate := qualificationRequest(t, f, router, "/api/drafts", key, "", body)
	var before, after draftrecovery.Result
	if duplicate.Code != 202 || json.Unmarshal(duplicate.Body.Bytes(), &before) != nil || !before.RetryAllowed || before.Attempt != 1 || sends != 2 {
		t.Fatalf("known failure replay: %d %s sends=%d", duplicate.Code, duplicate.Body.String(), sends)
	}
	retried := qualificationRequest(t, f, router, "/api/drafts", key, "known-failure-retry", body)
	if retried.Code != 201 || json.Unmarshal(retried.Body.Bytes(), &after) != nil || after.State != "ready" ||
		after.Attempt != 2 || after.Action != before.Action || after.Deadline != before.Deadline || after.AdmittedAt != before.AdmittedAt || sends != 6 {
		t.Fatalf("deliberate Retry did not publish the original Action: %d %s sends=%d", retried.Code, retried.Body.String(), sends)
	}
	for _, retry := range []string{"known-failure-retry", "third-attempt"} {
		got := qualificationRequest(t, f, router, "/api/drafts", key, retry, body)
		if sends != 6 || (got.Code != 200 && got.Code < 400) {
			t.Fatalf("Retry replay %s: %d sends=%d", retry, got.Code, sends)
		}
	}
}

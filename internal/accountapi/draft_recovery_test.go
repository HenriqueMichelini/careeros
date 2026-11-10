package accountapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestControlledDraftPublishesAndReopensExactWholeVersion(t *testing.T) {
	f := aiadmission.NewFixture(t)
	ctx := context.Background()
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("draft-recovery", 1)
	actions, err := aicontrol.NewSynthetic(ctx, f.DSN, aicontrol.DefaultSyntheticConfig("draft-recovery", gate))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { actions.Close() })
	if err = actions.SyntheticReviewer().SeedGrant(ctx, f.Principal, time.Now().Add(time.Hour), 3); err != nil {
		t.Fatal(err)
	}
	var checks atomic.Int32
	store, err := draftrecovery.Open(ctx, f.DSN, draftrecovery.Config{Actions: actions, Review: func(ctx context.Context, r draftrecovery.ReviewRequest) (draftrecovery.Completion, error) {
		checks.Add(1)
		return controlledRecoveryReview(ctx, r)
	}, BindingKey: []byte("synthetic-draft-binding-key-32-bytes"), Decisions: draftrecovery.DecisionFunc(func(context.Context, draftrecovery.Authority) (draftrecovery.Decision, error) {
		return draftrecovery.Decision{Generate: true, Publish: true, Read: true, GrantExpiresAt: time.Now().Add(time.Hour)}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var sends atomic.Int32
	router := accountapi.NewControlledDraftRecovery(f.Service, store, draftrecovery.ControlledProvider(func(_ context.Context, in draftrecovery.Generation) (draftrecovery.Bundle, error) {
		sends.Add(1)
		return completeRecoveryBundle(in), nil
	}))
	var doc profiledocument.Document
	if err = json.Unmarshal(f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	input := draftrecovery.Input{Profile: draftrecovery.Source{ID: doc.ID, Revision: doc.Revision}, Job: draftrecovery.Job{Posting: "Java required."}, Language: "en"}
	body, _ := json.Marshal(input)
	request := f.Request(t, ctx, "/api/drafts", body)
	request.Header.Del("X-OpenAI-Api-Key")
	request.Header.Del("X-TypeSafe-Api-Key")
	request.Header.Set("X-CareerOS-Action-Key", "whole-draft")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, request)
	if w.Code != http.StatusCreated {
		t.Fatalf("creation: %d %s", w.Code, w.Body.String())
	}
	var created draftrecovery.Result
	if err = json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.State != "ready" || created.Version != 1 || created.Bundle == nil || sends.Load() != 1 || checks.Load() != 1 {
		t.Fatalf("not a complete durable version: %+v sends=%d", created, sends.Load())
	}
	for _, path := range []string{"/api/drafts/whole-draft", "/api/drafts/whole-draft/versions/1", "/.netlify/functions/account/api/drafts/whole-draft/versions/1"} {
		r := f.Request(t, ctx, path, nil)
		r.Method = http.MethodGet
		r.Header.Del("X-OpenAI-Api-Key")
		r.Header.Del("X-TypeSafe-Api-Key")
		got := httptest.NewRecorder()
		router.ServeHTTP(got, r)
		var recovered draftrecovery.Result
		if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &recovered) != nil || !reflect.DeepEqual(created, recovered) || sends.Load() != 1 || checks.Load() != 1 {
			t.Fatalf("exact recovery %s: %d %s sends=%d", path, got.Code, got.Body.String(), sends.Load())
		}
	}
}

// Fixture expectations are independent of optional caller analysis and the
// generator's labels. This is not a live semantic checker or arbitrary-job proof.
func controlledRecoveryReview(_ context.Context, r draftrecovery.ReviewRequest) (draftrecovery.Completion, error) {
	requests := []draftrecovery.AnswerRequirement{}
	concerns := []string{}
	switch r.Generation.Input.Job.Posting {
	case "Java required.":
	case "Java required. Explain your Java experience.":
		requests = append(requests, draftrecovery.AnswerRequirement{ID: "java-experience", Source: draftrecovery.JobRange{Start: 15, End: len(r.Generation.Input.Job.Posting), Quote: "Explain your Java experience."}})
	default:
		concerns = append(concerns, "outside controlled fixture expectations")
	}
	expectedSummary, expectedResume, expectedLetter := "Java role.", "## Technical Skills\n\n- Java", "Dear hiring team,\n\nI use Java.\n\nSincerely,"
	if r.Generation.Input.Language == "pt-BR" {
		expectedSummary = "Vaga Java."
		expectedResume = "## Competências Técnicas\n\n- Java"
		expectedLetter = "Prezada equipe de contratação,\n\nTenho experiência com Java.\n\nAtenciosamente,"
	}
	if name := profiledocument.TrimText(r.Generation.Identity.FullName); name != "" {
		expectedLetter += "\n" + name
	}
	if r.Candidate.RoleSummary != expectedSummary || r.Candidate.Resume != expectedResume || r.Candidate.CoverLetter != expectedLetter {
		concerns = append(concerns, "generated wording disagrees with independent controlled expectations")
	}
	return draftrecovery.Completion{Version: draftrecovery.CompletionVersion, State: "complete", CandidateDigest: r.CandidateDigest, InputDigest: r.Generation.Binding.InputDigest, AnswerRequests: requests, Concerns: concerns}, nil
}

func completeRecoveryBundle(in draftrecovery.Generation) draftrecovery.Bundle {
	fact := in.Facts[0]
	letter := "Dear hiring team,\n\nI use Java.\n\nSincerely,"
	role, resume := "Java role.", "## Technical Skills\n\n- Java"
	if in.Input.Language == "pt-BR" {
		role = "Vaga Java."
		resume = "## Competências Técnicas\n\n- Java"
		letter = "Prezada equipe de contratação,\n\nTenho experiência com Java.\n\nAtenciosamente,"
	}
	if name := profiledocument.TrimText(in.Identity.FullName); name != "" {
		letter += "\n" + name
	}
	b := draftrecovery.Bundle{Schema: draftrecovery.BundleVersion, Binding: in.Binding, RoleSummary: role, Resume: resume, CoverLetter: letter, Identity: in.Identity, Answers: draftrecovery.EmployerAnswers{State: "none_requested", Requests: []draftrecovery.AnswerRequest{}}, Support: draftrecovery.Support{Version: draftrecovery.ReviewVersion, Check: "complete", Facts: []draftrecovery.Fact{fact}, Concerns: []string{}, Resume: []draftrecovery.Claim{{Text: resume, State: "supported", Sources: []draftrecovery.Reference{fact.Reference}, JobSources: []draftrecovery.JobRange{}, Concerns: []string{}}}, Artifacts: []draftrecovery.Claim{{Field: "roleSummary", Text: role, State: "supported", Sources: []draftrecovery.Reference{}, JobSources: []draftrecovery.JobRange{{Start: 0, End: len(in.Input.Job.Posting), Quote: in.Input.Job.Posting}}, Concerns: []string{}}, {Field: "coverLetter", Text: letter, State: "supported", Sources: []draftrecovery.Reference{fact.Reference}, JobSources: []draftrecovery.JobRange{}, Concerns: []string{}}}}}
	if strings.Contains(in.Input.Job.Posting, "Explain your Java experience.") && len(in.Input.Answers) > 0 {
		answer := in.Input.Answers[0].UserContext
		b.Answers = draftrecovery.EmployerAnswers{State: "complete", Text: answer, Requests: []draftrecovery.AnswerRequest{{ID: "java-experience", Source: draftrecovery.JobRange{Start: 15, End: len(in.Input.Job.Posting), Quote: "Explain your Java experience."}, Answer: answer}}}
		for _, f := range in.Facts {
			if f.Reference.Kind == "answer" {
				b.Support.Facts = append(b.Support.Facts, f)
				b.Support.Artifacts = append(b.Support.Artifacts, draftrecovery.Claim{Field: "applicationAnswers", Text: answer, State: "supported", Sources: []draftrecovery.Reference{f.Reference}, JobSources: []draftrecovery.JobRange{b.Answers.Requests[0].Source}, Concerns: []string{}})
				break
			}
		}
	}
	return b
}

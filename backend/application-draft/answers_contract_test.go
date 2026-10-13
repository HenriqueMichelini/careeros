package applicationdraft_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	applicationdraft "professional-information-repo/backend/application-draft"
	"strings"
	"testing"
)

func TestDraftAnswersPrioritizeEmployerRequestsAndLabelSuggestions(t *testing.T) {
	for _, tc := range []struct{ language, requested, suggested string }{
		{"en", "Employer-requested answers", "Additional suggestions"},
		{"pt-BR", "Respostas solicitadas pela empresa", "Sugestões adicionais"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			req := artifactRequest(t, map[string]any{"cvLanguage": tc.language, "reviewResume": false, "reviewArtifacts": false})
			client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				policy, _ := answerEnvelope(t, r)
				assertAnswerGenerationPolicy(t, policy, tc.requested, tc.suggested)
				draft := structuredDraft("## Technical Skills\n- Java")
				draft["applicationAnswers"] = "## " + tc.requested + "\nI use Java.\n\n## " + tc.suggested + "\nWhat would you like to learn about the team?"
				return completionResponse(draft), nil
			})}
			w := httptest.NewRecorder()
			applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
			if w.Code != 200 || !strings.Contains(w.Body.String(), tc.requested) || !strings.Contains(w.Body.String(), tc.suggested) {
				t.Fatalf("separate requested and suggested sections must survive delivery: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func assertAnswerGenerationPolicy(t *testing.T, policy, requested, suggested string) {
	t.Helper()
	if strings.Contains(policy, "5-6 useful application answers") {
		t.Fatal("a fixed answer quota can invent employer requests")
	}
	for _, requirement := range []string{
		"Answer every explicit employer Application Requirement first",
		"## " + requested, "## " + suggested,
		"Never present a suggested question as an employer request",
	} {
		if !strings.Contains(policy, requirement) {
			t.Fatalf("outbound generation contract omits %q", requirement)
		}
	}
}

type fixtureArtifactBlock struct{ Field, Text string }
type answerReviewPayload struct {
	CVLanguage string `json:"cvLanguage"`
	Blocks     []fixtureArtifactBlock
}

func answerEnvelope(t *testing.T, r *http.Request) (string, answerReviewPayload) {
	t.Helper()
	var envelope struct{ Messages []struct{ Content string } }
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Messages) != 2 {
		t.Fatal("expected system and user messages")
	}
	var payload answerReviewPayload
	if err := json.Unmarshal([]byte(envelope.Messages[1].Content), &payload); err != nil {
		t.Fatal(err)
	}
	return envelope.Messages[0].Content, payload
}

func TestAnswerReviewDistinguishesRequestsFromOptionalSuggestions(t *testing.T) {
	posting := "Java developer. Explique em português como você usa Java."
	for _, concerns := range [][]string{{}, {"Additional questions are unlabeled."}, {"A required answer appears only as an optional suggestion."}} {
		req := artifactRequest(t, map[string]any{"cvLanguage": "pt-BR", "jobPosting": posting})
		calls := 0
		client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			switch calls {
			case 1:
				return completionResponse(structuredDraft([]any{citedClaim("Java")})), nil
			case 2:
				return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": "supported", "reason": "Fonte aprovada."}}}), nil
			default:
				return answerReviewResponse(t, r, posting, concerns), nil
			}
		})}
		w := httptest.NewRecorder()
		applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
		assertAnswerConcerns(t, w, concerns)
	}
}

func answerReviewResponse(t *testing.T, r *http.Request, posting string, concerns []string) *http.Response {
	t.Helper()
	policy, payload := answerEnvelope(t, r)
	for _, rule := range []string{"Optional suggestions must be clearly separated", "cannot satisfy a missing employer-requested answer"} {
		if !strings.Contains(policy, rule) {
			t.Fatalf("answer review contract omits %q", rule)
		}
	}
	if payload.CVLanguage != "pt-BR" {
		t.Fatal("review needs the requested output language to assess the Q&A labels")
	}
	jobs := []any{map[string]any{"start": 0, "end": len(posting), "quote": posting}}
	judgments := artifactFixtureJudgments(payload.Blocks, jobs, "Controlled review verdict.")
	return completionResponse(map[string]any{"judgments": judgments, "answerConcerns": concerns})
}

func artifactFixtureJudgments(blocks []fixtureArtifactBlock, jobSources []any, reason string) []any {
	judgments := []any{}
	for i, b := range blocks {
		refs, jobs := artifactFixtureSources(b.Field, jobSources)
		judgments = append(judgments, map[string]any{"index": i, "state": "supported", "reason": reason, "scope": artifactScope(b.Field), "nonfactual": b.Field == "greeting", "sources": refs, "jobSources": jobs})
	}
	return judgments
}

func artifactFixtureSources(field string, jobSources []any) ([]any, []any) {
	refs, jobs := []any{}, []any{}
	switch field {
	case "jobTitle":
		jobs = jobSources[:1]
	case "jobSummary":
		jobs = jobSources
	case "body", "applicationAnswers":
		refs = append(refs, map[string]any{"profileId": "p", "id": "skill", "revision": 1})
	}
	return refs, jobs
}

func assertAnswerConcerns(t *testing.T, w *httptest.ResponseRecorder, concerns []string) {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var output struct {
		ArtifactReview struct {
			Claims []struct {
				Field, State string
				Concerns     []string
			}
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	answers := 0
	for _, c := range output.ArtifactReview.Claims {
		if c.Field != "applicationAnswers" {
			continue
		}
		answers++
		if len(c.Concerns) != len(concerns) {
			t.Fatalf("answer concerns were lost: %+v", c)
		}
		if len(concerns) > 0 && c.State != "unsupported" {
			t.Fatalf("answer concerns must block supported status: %+v", c)
		}
	}
	if answers != 1 {
		t.Fatal("the required answer review must remain present")
	}
}

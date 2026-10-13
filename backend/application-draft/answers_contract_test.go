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
				var envelope struct{ Messages []struct{ Content string } }
				if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
					t.Fatal(err)
				}
				policy := envelope.Messages[0].Content
				if strings.Contains(policy, "5-6 useful application answers") {
					t.Fatal("a fixed answer quota can invent employer requests")
				}
				for _, requirement := range []string{
					"Answer every explicit employer Application Requirement first",
					"## " + tc.requested, "## " + tc.suggested,
					"Never present a suggested question as an employer request",
				} {
					if !strings.Contains(policy, requirement) {
						t.Fatalf("outbound generation contract omits %q", requirement)
					}
				}
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

func TestAnswerReviewDistinguishesRequestsFromOptionalSuggestions(t *testing.T) {
	posting := "Java developer. Explique em português como você usa Java."
	for _, concerns := range [][]string{{}, {"Additional questions are unlabeled."}, {"A required answer appears only as an optional suggestion."}} {
		req := artifactRequest(t, map[string]any{"cvLanguage": "pt-BR", "jobPosting": posting})
		calls := 0
		client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return completionResponse(structuredDraft([]any{citedClaim("Java")})), nil
			}
			if calls == 2 {
				return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": "supported", "reason": "Fonte aprovada."}}}), nil
			}
			var envelope struct{ Messages []struct{ Content string } }
			json.NewDecoder(r.Body).Decode(&envelope)
			for _, rule := range []string{"Optional suggestions must be clearly separated", "cannot satisfy a missing employer-requested answer"} {
				if !strings.Contains(envelope.Messages[0].Content, rule) {
					t.Fatalf("answer review contract omits %q", rule)
				}
			}
			var payload struct {
				CVLanguage string `json:"cvLanguage"`
				Blocks     []struct{ Field, Text string }
			}
			json.Unmarshal([]byte(envelope.Messages[1].Content), &payload)
			if payload.CVLanguage != "pt-BR" {
				t.Fatal("review needs the requested output language to assess the Q&A labels")
			}
			judgments := []any{}
			for i, b := range payload.Blocks {
				refs, jobs := []any{}, []any{}
				if b.Field == "body" || b.Field == "applicationAnswers" {
					refs = append(refs, map[string]any{"profileId": "p", "id": "skill", "revision": 1})
				}
				if b.Field == "jobSummary" {
					jobs = append(jobs, map[string]any{"start": 0, "end": len(posting), "quote": posting})
				}
				judgments = append(judgments, map[string]any{"index": i, "state": "supported", "reason": "Controlled review verdict.", "scope": artifactScope(b.Field), "nonfactual": b.Field == "greeting", "sources": refs, "jobSources": jobs})
			}
			return completionResponse(map[string]any{"judgments": judgments, "answerConcerns": concerns}), nil
		})}
		w := httptest.NewRecorder()
		applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
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
		json.Unmarshal(w.Body.Bytes(), &output)
		answers := 0
		for _, c := range output.ArtifactReview.Claims {
			if c.Field == "applicationAnswers" {
				answers++
			}
			if c.Field == "applicationAnswers" && (len(c.Concerns) != len(concerns) || len(concerns) > 0 && c.State != "unsupported") {
				t.Fatalf("answer concerns must block supported status: %+v", c)
			}
		}
		if answers != 1 {
			t.Fatal("the required answer review must remain present")
		}
	}
}

package applicationdraft_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	applicationdraft "professional-information-repo/backend/application-draft"
	"strings"
	"testing"
)

func TestRemainingArtifactsHaveIndependentFieldSupport(t *testing.T) {
	req := reviewedRequest(t)
	var input map[string]any
	json.NewDecoder(req.Body).Decode(&input)
	input["reviewArtifacts"] = true
	data, _ := json.Marshal(input)
	req.Body = http.NoBody
	req = httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(data)))
	req.Header.Set("X-OpenAI-Api-Key", "sk-synthetic")
	req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	calls := 0
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return completionResponse(structuredDraft([]any{citedClaim("I use Java.")})), nil
		}
		if calls == 2 {
			return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": "supported", "reason": "Faithful."}}}), nil
		}
		// A required answer is omitted despite Java appearing in the resume and letter.
		return completionResponse(map[string]any{"judgments": []any{}, "answerConcerns": []string{"Required Portuguese answer omits Java."}}), nil
	})}
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"artifactReview"`) || !strings.Contains(w.Body.String(), `"check":"unavailable"`) {
		t.Fatalf("missing independent fail-closed artifact review: %d %s", w.Code, w.Body.String())
	}
	if calls != 3 {
		t.Fatalf("want three bounded calls, got %d", calls)
	}
}

func TestArtifactSupportGuardsAndAnswerOmission(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		refs       bool
		jobQuote   string
		concerns   []string
		want       string
	}{
		{"leadership", "I led ten engineers.", true, "", nil, "stronger_claim"},
		{"metric", "I improved throughput by 40%.", true, "", nil, "invented_number"},
		{"proficiency", "I am fluent in Java.", true, "", nil, "stronger_claim"},
		{"missing career support", "I use Java.", false, "", nil, "missing_citations"},
		{"forged job excerpt", "I use Java.", true, "Invented employer", nil, "invalid_job_citation"},
		{"Portuguese field omission", "Eu uso Java.", true, "", []string{"A resposta obrigatória omite Java."}, "A resposta obrigatória omite Java."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := reviewedRequest(t)
			var input map[string]any
			json.NewDecoder(req.Body).Decode(&input)
			input["reviewArtifacts"] = true
			input["jobPosting"] = "Java developer. AWS required."
			input["qualificationAnswers"] = []any{map[string]string{"requirement": "Describe Java experience", "userContext": "I use Java."}}
			data, _ := json.Marshal(input)
			req = httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(data)))
			req.Header.Set("X-OpenAI-Api-Key", "sk-synthetic")
			req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
			calls := 0
			client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					draft := structuredDraft([]any{citedClaim("Java")})
					draft["coverLetter"].(map[string]string)["body"] = tc.body
					return completionResponse(draft), nil
				}
				if calls == 2 {
					return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": "supported", "reason": "Exact skill."}}}), nil
				}
				var envelope struct{ Messages []struct{ Content string } }
				json.NewDecoder(r.Body).Decode(&envelope)
				var payload struct {
					Blocks []struct{ Field, Text string }
				}
				json.Unmarshal([]byte(envelope.Messages[1].Content), &payload)
				judgments := []any{}
				for i, b := range payload.Blocks {
					refs := []any{}
					jobs := []any{}
					nonfactual := b.Field == "greeting"
					if b.Field == "body" || b.Field == "applicationAnswers" {
						if tc.refs {
							refs = append(refs, map[string]any{"profileId": "p", "id": "skill", "revision": 1})
						}
					}
					if b.Field == "jobSummary" || (b.Field == "body" && tc.jobQuote != "") {
						quote := "Java developer."
						if tc.jobQuote != "" {
							quote = tc.jobQuote
						}
						jobs = append(jobs, map[string]any{"start": 0, "end": 15, "quote": quote})
					}
					judgments = append(judgments, map[string]any{"index": i, "state": "supported", "reason": "Controlled judgment.", "nonfactual": nonfactual, "sources": refs, "jobSources": jobs})
				}
				concerns := tc.concerns
				if concerns == nil {
					concerns = []string{}
				}
				return completionResponse(map[string]any{"judgments": judgments, "answerConcerns": concerns}), nil
			})}
			w := httptest.NewRecorder()
			applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
			if w.Code != 200 || !strings.Contains(w.Body.String(), tc.want) {
				t.Fatalf("guard missing: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestApplicationAnswerEvidenceStaysRequestLocal(t *testing.T) {
	for _, text := range []string{"I have not used AWS.", "Eu não usei AWS.", "My desired salary is BRL 9000.", "Minha pretensão salarial é BRL 9000."} {
		t.Run(text, func(t *testing.T) {
			req := reviewedRequest(t)
			var input map[string]any
			json.NewDecoder(req.Body).Decode(&input)
			input["reviewArtifacts"] = true
			input["qualificationAnswers"] = []any{map[string]string{"requirement": "Application question", "userContext": text}}
			original, _ := json.Marshal(input["profileEvidence"])
			data, _ := json.Marshal(input)
			req = httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(data)))
			req.Header.Set("X-OpenAI-Api-Key", "sk-synthetic")
			req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
			calls := 0
			client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					draft := structuredDraft([]any{citedClaim("Java")})
					draft["applicationAnswers"] = text
					draft["coverLetter"].(map[string]string)["body"] = text
					return completionResponse(draft), nil
				}
				if calls == 2 {
					return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": "supported", "reason": "Exact skill."}}}), nil
				}
				var envelope struct{ Messages []struct{ Content string } }
				json.NewDecoder(r.Body).Decode(&envelope)
				var payload struct {
					Blocks []struct{ Field, Text string }
					Facts  []map[string]any
				}
				json.Unmarshal([]byte(envelope.Messages[1].Content), &payload)
				found := false
				for _, f := range payload.Facts {
					if f["id"] == "application-answer-0" && f["value"] == text {
						found = true
					}
				}
				if !found {
					t.Fatal("raw answer must be available as its own request-local source")
				}
				judgments := []any{}
				for i, b := range payload.Blocks {
					refs := []any{}
					jobs := []any{}
					if b.Field == "body" || b.Field == "applicationAnswers" {
						refs = append(refs, map[string]any{"profileId": "p", "id": "application-answer-0", "revision": 1})
					}
					if b.Field == "jobSummary" {
						posting := input["jobPosting"].(string)
						jobs = append(jobs, map[string]any{"start": 0, "end": len(posting), "quote": posting})
					}
					judgments = append(judgments, map[string]any{"index": i, "state": "supported", "reason": "Exact supplied answer.", "nonfactual": b.Field == "greeting", "sources": refs, "jobSources": jobs})
				}
				return completionResponse(map[string]any{"judgments": judgments, "answerConcerns": []string{}}), nil
			})}
			w := httptest.NewRecorder()
			applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
			if w.Code != 200 {
				t.Fatalf("answer failed %d %s", w.Code, w.Body.String())
			}
			var result struct {
				ArtifactReview struct {
					Claims []struct {
						Field, State string
						Concerns     []string
					}
				}
			}
			json.Unmarshal(w.Body.Bytes(), &result)
			for _, c := range result.ArtifactReview.Claims {
				if (c.Field == "body" || c.Field == "applicationAnswers") && c.State != "supported" {
					t.Fatalf("literal supplied answer lost support: %+v", c)
				}
			}
			after, _ := json.Marshal(input["profileEvidence"])
			if string(after) != string(original) {
				t.Fatal("saved Profile changed")
			}
		})
	}
}

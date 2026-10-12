package applicationdraft_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	applicationdraft "professional-information-repo/backend/application-draft"
	"strings"
	"testing"
)

func TestStrengthGuardDistinguishesResponsibilityFromSeniority(t *testing.T) {
	for _, tc := range []struct {
		name, source, text, want, concern string
		reject                            bool
	}{
		{"Portuguese responsibility", "Uso Java para desenvolver APIs.", "Uso Java para desenvolver APIs. Qual é a principal responsabilidade da vaga? Desenvolver aplicações em Java.", "supported", "", false},
		{"Portuguese declarative responsibility", "Uso Java.", "A principal responsabilidade da vaga envolve Java. Uso Java.", "supported", "", false},
		{"Principal engineer English", "I use Java.", "I am a principal engineer using Java.", "unsupported", "stronger_claim", false},
		{"Principal engineer Portuguese", "Uso Java.", "Sou engenheiro principal e uso Java.", "unsupported", "stronger_claim", false},
		{"Seniority before role", "Uso Java.", "Sou principal engenheiro e uso Java.", "unsupported", "stronger_claim", false},
		{"Responsibility cannot hide another rank", "Uso Java.", "A principal responsabilidade da vaga é Java. Sou engenheiro principal.", "unsupported", "stronger_claim", false},
		{"Responsibility cannot supply career rank", "Minha principal responsabilidade é usar Java.", "Sou engenheiro principal e uso Java.", "unsupported", "stronger_claim", false},
		{"Literal supported rank", "I am a principal engineer using Java.", "I am a principal engineer using Java.", "supported", "", false},
		{"Other unsupported strength", "Uso Java.", "A principal responsabilidade é Java. Sou especialista fluente em Java.", "unsupported", "stronger_claim", false},
		{"Metrics remain guarded", "Uso Java.", "A principal responsabilidade é Java. Melhorei APIs em 40%.", "unsupported", "invented_number", false},
		{"Independent review rejection retained", "Uso Java.", "A principal responsabilidade é Java. Uso Java.", "unsupported", "Controlled semantic rejection.", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := reviewedRequest(t)
			var input map[string]any
			json.NewDecoder(req.Body).Decode(&input)
			input["reviewArtifacts"] = true
			posting := "Desenvolvedor Java. Responsabilidades: desenvolver aplicações em Java."
			input["jobPosting"] = posting
			doc := input["profileEvidence"].(map[string]any)
			doc["facts"].([]any)[0].(map[string]any)["value"] = tc.source
			doc["evidence"].([]any)[0].(map[string]any)["excerpt"] = tc.source
			data, _ := json.Marshal(input)
			req = httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(data)))
			req.Header.Set("X-OpenAI-Api-Key", "sk-synthetic")
			req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
			supportState, supportReason := "supported", "Controlled judgment, not semantic proof."
			if tc.reject {
				supportState, supportReason = "unsupported", "Controlled semantic rejection."
			}
			calls := 0
			client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					d := structuredDraft([]any{citedClaim(tc.text)})
					d["applicationAnswers"] = tc.text
					d["coverLetter"].(map[string]string)["body"] = "Eu uso a linguagem Java."
					return completionResponse(d), nil
				}
				var envelope struct {
					Messages       []struct{ Content string }
					ResponseFormat struct {
						JSONSchema struct {
							Name string `json:"name"`
						} `json:"json_schema"`
					} `json:"response_format"`
				}
				json.NewDecoder(r.Body).Decode(&envelope)
				if envelope.ResponseFormat.JSONSchema.Name == "resume_support" {
					return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": supportState, "reason": supportReason}}}), nil
				}
				var payload struct {
					Blocks []struct{ Field, Text string }
				}
				json.Unmarshal([]byte(envelope.Messages[1].Content), &payload)
				judgments := []any{}
				for i, b := range payload.Blocks {
					refs := []any{}
					jobs := []any{}
					scope := "career"
					if b.Field == "jobSummary" || b.Field == "applicationAnswers" {
						jobs = append(jobs, map[string]any{"start": 0, "end": len(posting), "quote": posting})
						scope = "job"
					}
					if b.Field == "body" || b.Field == "applicationAnswers" {
						refs = append(refs, map[string]any{"profileId": "p", "id": "skill", "revision": 1})
						scope = "career"
						if b.Field == "applicationAnswers" {
							scope = "mixed"
						}
					}
					if b.Field == "greeting" {
						scope = "nonfactual"
					}
					judgments = append(judgments, map[string]any{"index": i, "state": supportState, "reason": supportReason, "scope": scope, "nonfactual": scope == "nonfactual", "sources": refs, "jobSources": jobs})
				}
				return completionResponse(map[string]any{"judgments": judgments, "answerConcerns": []string{}}), nil
			})}
			w := httptest.NewRecorder()
			applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
			if w.Code != 200 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			var output struct {
				ResumeReview struct {
					Claims []struct {
						State    string
						Concerns []string
					}
				}
				ArtifactReview struct {
					Claims []struct {
						Field, State string
						Concerns     []string
					}
				}
			}
			json.Unmarshal(w.Body.Bytes(), &output)
			if len(output.ResumeReview.Claims) != 1 {
				t.Fatalf("missing resume review: %s", w.Body.String())
			}
			if tc.concern != "" && !strings.Contains(strings.Join(output.ResumeReview.Claims[0].Concerns, "|"), tc.concern) {
				t.Fatalf("resume concern missing: %+v", output.ResumeReview.Claims[0])
			}
			if output.ResumeReview.Claims[0].State != tc.want {
				t.Fatalf("resume: want %s, got %+v", tc.want, output.ResumeReview.Claims[0])
			}
			found := false
			for _, c := range output.ArtifactReview.Claims {
				if c.Field == "applicationAnswers" {
					found = true
					if tc.concern != "" && !strings.Contains(strings.Join(c.Concerns, "|"), tc.concern) {
						t.Fatalf("answer concern missing: %+v", c)
					}
					if c.State != tc.want {
						t.Fatalf("answer: want %s, got %+v", tc.want, c)
					}
				}
			}
			if !found {
				t.Fatal("missing answer review")
			}
		})
	}
}

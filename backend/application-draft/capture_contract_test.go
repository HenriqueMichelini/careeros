package applicationdraft_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	applicationdraft "professional-information-repo/backend/application-draft"
	"professional-information-repo/internal/testsupport"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type releaseCaptureTransport func(*http.Request) (*http.Response, error)

func (f releaseCaptureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// This transport has no network delegate. It inspects the actual serialized
// envelopes emitted by the public handler, including response-dependent stages.
// It is preparation evidence only, not a deployed egress-capture mechanism.
func TestControlledReleaseCapturesEachProviderEnvelopeBeforeAdmission(t *testing.T) {
	posting := "Desenvolvedor Ruby.\nMantenha ferramentas CLI internas usando Ruby.\nRuby é obrigatório.\nExplique como você usa Ruby em português."
	var baseline []string
	for _, variant := range []string{"literal", "paraphrase", "deny artifact envelope"} {
		t.Run(variant, func(t *testing.T) {
			req := artifactRequest(t, map[string]any{"jobPosting": posting, "cvLanguage": "pt-BR"})
			var input map[string]any
			json.NewDecoder(req.Body).Decode(&input)
			doc := input["profileEvidence"].(map[string]any)
			fact := doc["facts"].([]any)[0].(map[string]any)
			fact["value"] = "Ruby"
			fact["normalization"].(map[string]any)["observed"] = "Ruby"
			fact["normalization"].(map[string]any)["canonical"] = nil
			doc["evidence"].([]any)[0].(map[string]any)["excerpt"] = "Uso Ruby para manter ferramentas CLI internas."
			input["repository"].(map[string]any)["skills"] = "Ruby"
			raw, _ := json.Marshal(input)
			req = httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(raw)))
			req.Header.Set("X-OpenAI-Api-Key", "sk-capture-only")
			req.Header.Set("X-TypeSafe-Api-Key", "capture-only")
			hashes := []string{}
			sizes := []int{}
			client := &http.Client{Transport: releaseCaptureTransport(func(r *http.Request) (*http.Response, error) {
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(raw), "sk-capture-only") || strings.Contains(string(raw), "capture-only") {
					t.Fatal("credential leaked into provider body")
				}
				digest := sha256.Sum256(raw)
				hashes = append(hashes, hex.EncodeToString(digest[:]))
				sizes = append(sizes, len(raw))
				stage := len(hashes) - 1
				if stage > 3 {
					t.Fatal("unplanned dispatch or automatic retry")
				}
				var envelope map[string]json.RawMessage
				if json.Unmarshal(raw, &envelope) != nil {
					t.Fatal("invalid serialized envelope")
				}
				if stage == 0 {
					if r.URL.Host != "api.typesafe.ai" {
						t.Fatal("original field decision must run first")
					}
					assertCaptureFields(t, envelope, "model", "questions", "state")
					if string(envelope["model"]) != `"jev-1.13.0"` {
						t.Fatal("fixed field-validation model changed")
					}
					var state struct{ Submission string }
					json.Unmarshal(envelope["state"], &state)
					if state.Submission != posting {
						t.Fatal("Jev must receive the complete original posting")
					}
					return testsupport.AcceptedJob(), nil
				}
				if r.URL.Host != "api.openai.com" {
					t.Fatal("unexpected provider host")
				}
				assertCaptureFields(t, envelope, "max_completion_tokens", "messages", "model", "reasoning_effort", "response_format")
				if string(envelope["model"]) != `"gpt-6-luna"` || string(envelope["reasoning_effort"]) != `"none"` {
					t.Fatal("fixed workflow settings changed")
				}
				ceilings := []string{"", "8000", "4000", "8000"}
				if string(envelope["max_completion_tokens"]) != ceilings[stage] {
					t.Fatal("stage output ceiling changed")
				}
				var messages []struct{ Content string }
				json.Unmarshal(envelope["messages"], &messages)
				var payload map[string]json.RawMessage
				json.Unmarshal([]byte(messages[1].Content), &payload)
				if stage == 1 {
					assertCaptureFields(t, payload, "careerEvidence", "confirmedQualifications", "jobPosting", "qualificationAnswers", "resumeApplicationEvidence", "selection")
					text := "Ruby"
					if variant != "literal" {
						text = "Ruby: manutenção de ferramentas CLI internas."
					}
					draft := structuredDraft([]any{citedClaim(text)})
					draft["jobTitle"], draft["jobSummary"] = "Desenvolvedor Ruby", posting
					draft["coverLetter"].(map[string]string)["body"] = "Uso Ruby para manter ferramentas CLI internas."
					draft["applicationAnswers"] = "Uso Ruby para manter ferramentas CLI internas."
					if variant != "literal" {
						draft["applicationAnswers"] = "Uso Ruby em ferramentas CLI internas."
					}
					return completionResponse(draft), nil
				}
				if stage == 2 {
					assertCaptureFields(t, payload, "claims", "cvLanguage", "facts", "qualificationAnswers")
					return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": "supported", "reason": "Fonte sintética aprovada."}}}), nil
				}
				assertCaptureFields(t, payload, "applicationAnswers", "blocks", "careerCitations", "cvLanguage", "facts", "jobCitations", "jobPosting", "qualificationAnswers")
				if variant == "deny artifact envelope" {
					return nil, errors.New("dependent envelope not admitted")
				}
				var blocks []struct{ Field, Text string }
				json.Unmarshal(payload["blocks"], &blocks)
				jobSources := []any{}
				start := 0
				for _, line := range strings.Split(posting, "\n") {
					jobSources = append(jobSources, map[string]any{"start": start, "end": start + len(line), "quote": line})
					start += len(line) + 1
				}
				judgments := []any{}
				for i, block := range blocks {
					refs, jobs := []any{}, []any{}
					if block.Field == "jobTitle" {
						jobs = jobSources[:1]
					}
					if block.Field == "jobSummary" {
						jobs = jobSources
					}
					if block.Field == "body" || block.Field == "applicationAnswers" {
						refs = append(refs, map[string]any{"profileId": "p", "id": "skill", "revision": 1})
					}
					judgments = append(judgments, map[string]any{"index": i, "state": "supported", "reason": "Reconstructed source-reviewed fixture.", "scope": artifactScope(block.Field), "nonfactual": block.Field == "greeting", "sources": refs, "jobSources": jobs})
				}
				return completionResponse(map[string]any{"judgments": judgments, "answerConcerns": []string{}}), nil
			})}
			w := httptest.NewRecorder()
			applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
			if len(hashes) != 4 || w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected bounded result: calls=%d status=%d", len(hashes), w.Code)
			}
			var result struct {
				ArtifactReview struct {
					Check  string
					Claims []struct {
						Field, State string
						JobSources   []json.RawMessage
					}
				}
			}
			json.Unmarshal(w.Body.Bytes(), &result)
			if variant == "deny artifact envelope" {
				if result.ArtifactReview.Check != "unavailable" {
					t.Fatal("denied support envelope must remain unavailable")
				}
			} else {
				if result.ArtifactReview.Check != "complete" {
					t.Fatalf("controlled support unavailable: %s", w.Body.String())
				}
				for _, claim := range result.ArtifactReview.Claims {
					if claim.Field == "jobSummary" && (claim.State != "supported" || len(claim.JobSources) != 4) {
						t.Fatal("multi-subclaim summary must preserve its four distinct supplied excerpts")
					}
				}
			}
			if variant == "literal" {
				baseline = append([]string{}, hashes...)
			}
			if variant == "paraphrase" && (hashes[1] != baseline[1] || hashes[2] == baseline[2] || hashes[3] == baseline[3]) {
				t.Fatal("a fixed initial envelope cannot preapprove a response-dependent support envelope")
			}
			t.Logf("controlled only: 4 inspected envelopes; envelopeBytes=%v; hashes=%v", sizes, hashes)
		})
	}
}

func assertCaptureFields(t *testing.T, value map[string]json.RawMessage, allowed ...string) {
	t.Helper()
	keys := []string{}
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sort.Strings(allowed)
	if !reflect.DeepEqual(keys, allowed) {
		t.Fatalf("outbound field allowlist mismatch: %v", keys)
	}
}

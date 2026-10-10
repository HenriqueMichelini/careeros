package applicationdraft_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	applicationdraft "professional-information-repo/backend/application-draft"
	"strings"
	"testing"
)

func completionResponse(value any) *http.Response {
	content, _ := json.Marshal(value)
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(content)}}}})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}
}

func reviewedRequest(t *testing.T) *http.Request {
	t.Helper()
	raw, err := os.ReadFile("../../internal/profiledocument/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct{ Document map[string]any }
	json.Unmarshal(raw, &fixtures)
	doc := fixtures[0].Document
	// Keep the canonical graph, but use a literal approved affirmative statement.
	fact := doc["facts"].([]any)[0].(map[string]any)
	fact["value"] = "Java"
	fact["assertion"] = "affirmed"
	fact["intent"] = "actual"
	fact["certainty"] = "certain"
	var input map[string]any
	json.Unmarshal([]byte(syntheticDraftInput), &input)
	input["profileEvidence"] = doc
	input["reviewResume"] = true
	data, _ := json.Marshal(input)
	req := httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(data)))
	req.Header.Set("X-OpenAI-Api-Key", "sk-synthetic")
	req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	return req
}

func TestReviewedResumeRequiresCurrentCitations(t *testing.T) {
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		var envelope struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&envelope)
		// The fixture reference is intentionally stale. Even a provider claiming support must not authorize it.
		return completionResponse(map[string]any{"jobTitle": nil, "company": nil, "jobSummary": "Java role.", "resume": []any{map[string]any{"section": "skills", "kind": "bullet", "text": "Java", "sources": []any{map[string]any{"profileId": "profile", "id": "skill", "revision": 999}}}}, "applicationAnswers": "I use Java.", "coverLetter": map[string]string{"greeting": "Dear team,", "body": "I use Java.", "closing": "Sincerely,"}}), nil
	})}
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, reviewedRequest(t))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"unsupported"`) || !strings.Contains(w.Body.String(), `"resumeReview"`) {
		t.Fatalf("stale citations must yield an unresolved review, got %d %s", w.Code, w.Body.String())
	}
}

func structuredDraft(claims any) map[string]any {
	return map[string]any{"jobTitle": nil, "company": nil, "jobSummary": "Java role.", "resume": claims, "applicationAnswers": "I use Java.", "coverLetter": map[string]string{"greeting": "Dear team,", "body": "I use Java.", "closing": "Sincerely,"}}
}
func citedClaim(text string) map[string]any {
	return map[string]any{"section": "skills", "kind": "bullet", "text": text, "sources": []any{map[string]any{"profileId": "p", "id": "skill", "revision": 1}}}
}
func TestReviewedResumeRequiresIndependentSupportAssessment(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return completionResponse(structuredDraft([]any{citedClaim("I use Java.")})), nil
		}
		return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": "supported", "reason": "Faithful paraphrase."}}}), nil
	})}
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, reviewedRequest(t))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"supported"`) || !strings.Contains(w.Body.String(), `"check":"complete"`) || !strings.Contains(w.Body.String(), `"resume":"## Technical Skills\n- I use Java."`) {
		t.Fatalf("support assessment missing: %d %s", w.Code, w.Body.String())
	}
	if calls != 2 {
		t.Fatalf("expected generation and one bounded assessment, got %d", calls)
	}
}

func TestReviewedResumePreservesLiteralSkillPunctuation(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return completionResponse(structuredDraft([]any{citedClaim("C# and my_variable tools")})), nil
		}
		return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": "uncertain", "reason": "Source states Java only."}}}), nil
	})}
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, reviewedRequest(t))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "C# and my_variable tools") {
		t.Fatalf("literal punctuation was rejected: %d %s", w.Code, w.Body.String())
	}
}

func TestReviewedResumeStopsMaterialInventions(t *testing.T) {
	raw, _ := os.ReadFile("../../docs/evaluations/qualification-evidence/cases.json")
	var corpus struct {
		Cases []struct {
			ID       string
			Document json.RawMessage
		}
	}
	json.Unmarshal(raw, &corpus)
	cases := []struct {
		name, document, text, kind, concern string
		ids                                 []string
	}{
		{"invented metric", "explicit-support", "Reduced latency by 40% using Java.", "bullet", "invented_number", []string{"java"}},
		{"stronger seniority", "explicit-support", "Senior Java engineer", "bullet", "stronger_claim", []string{"java"}},
		{"cross role combination", "contrasting-employers", "Used Java and AWS together.", "bullet", "cross_owner_combination", []string{"a-java", "b-aws"}},
		{"removed negation", "negation", "I use Java.", "bullet", "protected_fact_or_qualifier_changed", []string{"no"}},
		{"exact duration upgrade", "approximate-duration", "Used Java for three years.", "bullet", "protected_fact_or_qualifier_changed", []string{"java"}},
		{"changed employer", "contrasting-employers", "Engineer at Invented Corp", "subheading", "protected_fact_or_qualifier_changed", []string{"a-title", "a-company"}},
		{"missing citations", "explicit-support", "I use Java.", "bullet", "missing_citations", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var input map[string]any
			json.Unmarshal([]byte(syntheticDraftInput), &input)
			for _, c := range corpus.Cases {
				if c.ID == tc.document {
					input["profileEvidence"] = c.Document
				}
			}
			input["reviewResume"] = true
			data, _ := json.Marshal(input)
			refs := []any{}
			for _, id := range tc.ids {
				refs = append(refs, map[string]any{"profileId": "p", "id": id, "revision": 1})
			}
			claim := map[string]any{"section": "experience", "kind": tc.kind, "text": tc.text, "sources": refs}
			calls := 0
			client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return completionResponse(structuredDraft([]any{claim})), nil
			})}
			req := httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(data)))
			req.Header.Set("X-OpenAI-Api-Key", "sk-test")
			req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
			w := httptest.NewRecorder()
			applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"unsupported"`) || !strings.Contains(w.Body.String(), tc.concern) {
				t.Fatalf("material invention escaped bounds: %d %s", w.Code, w.Body.String())
			}
			if calls != 1 {
				t.Fatalf("deterministic rejection needs no additional provider call: %d", calls)
			}
		})
	}
}

func TestSupportFailureDoesNotDeliverUsableResumeOrRetry(t *testing.T) {
	for _, failure := range []string{"outage", "truncated", "malformed", "missing_index"} {
		t.Run(failure, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return completionResponse(structuredDraft([]any{citedClaim("I use Java.")})), nil
				}
				switch failure {
				case "outage":
					return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
				case "truncated":
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"length","message":{"content":"{}"}}]}`))}, nil
				case "missing_index":
					return completionResponse(map[string]any{"judgments": []any{map[string]any{"state": "supported", "reason": "Okay."}}}), nil
				default:
					return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": "unknown", "reason": "Okay."}}}), nil
				}
			})}
			w := httptest.NewRecorder()
			applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, reviewedRequest(t))
			if w.Code != 502 || !strings.Contains(w.Body.String(), `"error":"invalid_output"`) || w.Header().Get("Cache-Control") != "no-store" || calls != 2 {
				t.Fatalf("failed support must stop with no retry: %d %s calls=%d", w.Code, w.Body.String(), calls)
			}
		})
	}
}

func TestResumeSemanticConcernIsNotSilentlyAccepted(t *testing.T) {
	for _, state := range []string{"uncertain", "unsupported"} {
		t.Run(state, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return completionResponse(structuredDraft([]any{citedClaim("Used Java and PostgreSQL together at work.")})), nil
				}
				return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": state, "reason": "Combined use in one role is not established."}}}), nil
			})}
			w := httptest.NewRecorder()
			applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, reviewedRequest(t))
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"`+state+`"`) || !strings.Contains(w.Body.String(), "Combined use") {
				t.Fatalf("semantic concern missing: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestTemporaryQualificationHasApplicationOwnedSupport(t *testing.T) {
	req := reviewedRequest(t)
	raw, _ := io.ReadAll(req.Body)
	var input map[string]any
	json.Unmarshal(raw, &input)
	input["confirmedQualifications"] = []any{map[string]string{"kind": "skill", "requirement": "AWS", "userContext": "Used AWS for a personal demo."}}
	data, _ := json.Marshal(input)
	req.Body = io.NopCloser(strings.NewReader(string(data)))
	calls := 0
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), "application-confirmation-0") {
			t.Error("temporary qualification lost request-local identity")
		}
		if calls == 1 {
			c := citedClaim("Used AWS for a personal demo.")
			c["sources"] = []any{map[string]any{"profileId": "p", "id": "application-confirmation-0", "revision": 1}}
			return completionResponse(structuredDraft([]any{c})), nil
		}
		return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": "supported", "reason": "User confirmed a personal demo only."}}}), nil
	})}
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"supported"`) {
		t.Fatalf("temporary qualification not reviewable: %d %s", w.Code, w.Body.String())
	}
}

func TestIndentedMarkdownCannotDisappearFromAcceptedResume(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			c := citedClaim("  ## Acme")
			c["kind"] = "paragraph"
			return completionResponse(structuredDraft([]any{c})), nil
		}
		return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": "supported", "reason": "Controlled semantic approval cannot bypass the syntax bound."}}}), nil
	})}
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, reviewedRequest(t))
	if w.Code != 502 || !strings.Contains(w.Body.String(), `"error":"invalid_output"`) {
		t.Fatalf("indented Markdown escaped plain statement boundary: %d %s", w.Code, w.Body.String())
	}
}

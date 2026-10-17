package cvgeneration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestGenerationContract(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "currentSalary") {
			t.Fatal("private field outbound")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"summary\":[{\"sourceId\":\"f0\",\"text\":\"Reduced wait by 20%.\"}],\"selected\":[\"f0\"]}"}}]}`))}, nil
	})}}
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"locale":"en","facts":[{"id":"f0","section":"experience","entryId":"e","field":"achievements","text":"Reduced wait by 20%."}]}`, 200},
		{`{"locale":"en","facts":[],"currentSalary":"secret"}`, 400},
		{`{"locale":"en","facts":[{"id":"f0","section":"experience","entryId":"e","field":"title","text":"CEO"}]}`, 400},
		{`{"locale":"en","facts":[{"id":"f0","section":"other","entryId":"e","field":"salary","text":"secret"}]}`, 400},
	} {
		r := httptest.NewRequest("POST", "/api/cv/generate", strings.NewReader(tc.body))
		r.Header.Set("X-OpenAI-Api-Key", "sk-test")
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cache")
		}
	}
	if calls != 1 {
		t.Fatalf("calls %d", calls)
	}
}
func TestGrounding(t *testing.T) {
	facts := []fact{{ID: "f0", Section: "experience", EntryID: "e", Field: "achievements", Text: "Reduced wait by 20%."}}
	for _, raw := range []string{`{"summary":[{"sourceId":"f0","text":"Reduced wait by 90%."}],"selected":["f0"]}`, `{"summary":[{"sourceId":"f0","text":"Reduced wait by 20%."}],"selected":["unknown"]}`, `{"summary":[{"sourceId":"f0","text":"Reduced wait by 20%."}],"selected":["f0","f0"]}`} {
		var result result
		json.Unmarshal([]byte(raw), &result)
		if validResult(result, facts) {
			t.Fatal("accepted ungrounded", raw)
		}
	}
}

func TestProviderFailuresPreserveContract(t *testing.T) {
	input := `{"locale":"en","facts":[{"id":"f0","section":"skills","entryId":"","field":"skills","text":"Research"}]}`
	for _, tc := range []struct {
		status  int
		content string
		want    int
		code    string
	}{
		{401, "", 401, "key"}, {429, "", 429, "rate_limit"}, {500, "", 502, "outage"}, {200, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"summary\":[{\"sourceId\":\"f0\",\"text\":\"CEO\"}],\"selected\":[\"f0\"]}"}}]}`, 502, "invalid_output"},
	} {
		a := app{client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.content))}, nil
		})}}
		r := httptest.NewRequest("POST", "/api/cv/generate", strings.NewReader(input))
		r.Header.Set("X-OpenAI-Api-Key", "sk-test")
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, r)
		if w.Code != tc.want || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
}
func TestBoundsAndNegation(t *testing.T) {
	in := request{Locale: "pt-BR", Facts: []fact{{ID: "f0", Section: "education", EntryID: "e", Field: "degree", Text: "Bacharelado em Design"}}}
	if !validRequest(in) {
		t.Fatal("sparse qualification rejected")
	}
	in.Facts[0].Text = strings.Repeat("x", (12<<10)+1)
	if validRequest(in) {
		t.Fatal("oversize accepted")
	}
	facts := []fact{{ID: "f0", Section: "experience", EntryID: "e", Field: "achievements", Text: "Did not reduce wait by 20%."}}
	if validResult(result{Summary: []excerpt{{SourceID: "f0", Text: "reduce wait by 20%."}}, Selected: []string{"f0"}}, facts) {
		t.Fatal("negation stripped")
	}
}

func TestGroundedParaphraseAndConsolidation(t *testing.T) {
	facts := []fact{{ID: "f0", Section: "experience", EntryID: "e", Field: "description", Text: "Designed customer services. Designed services for customers. Used research to improve services by 20%."}}
	out := result{Summary: []excerpt{{SourceID: "f0", Text: "Designed customer services using research."}}, Selected: []string{"f0"}, Wording: map[string]string{"f0": "Improved customer services by 20% using research."}}
	if !validResult(out, facts) {
		t.Fatal("grounded concise paraphrase rejected")
	}
	for _, wording := range []string{"Improved services by 90%.", "Certified expert in research.", "Managed customer services."} {
		out.Wording["f0"] = wording
		if validResult(out, facts) {
			t.Fatal("unsupported claim accepted", wording)
		}
	}
	out.Wording = map[string]string{"f0": "Improved customer services by 20% using research."}
	facts[0].Field = "title"
	if validResult(out, facts) {
		t.Fatal("metadata wording accepted")
	}
}

func TestDensityRequestPolicy(t *testing.T) {
	for _, density := range []string{"", "compact", "balanced", "detailed", "invalid"} {
		calls := 0
		a := app{client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			calls++
			var body struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			var input map[string]any
			json.Unmarshal([]byte(body.Messages[1].Content), &input)
			want := density
			if want == "" {
				want = "balanced"
			}
			if input["density"] != want {
				t.Fatalf("density %v, want %s", input["density"], want)
			}
			if !strings.Contains(body.Messages[0].Content, "Density "+want+":") {
				t.Fatal("missing chosen density policy")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"summary\":[{\"sourceId\":\"f0\",\"text\":\"Research\"}],\"selected\":[\"f0\"]}"}}]}`))}, nil
		})}}
		input := map[string]any{"locale": "en", "facts": []fact{{ID: "f0", Section: "skills", Field: "skills", Text: "Research"}}}
		if density != "" {
			input["density"] = density
		}
		raw, _ := json.Marshal(input)
		r := httptest.NewRequest("POST", "/api/cv/generate", strings.NewReader(string(raw)))
		r.Header.Set("X-OpenAI-Api-Key", "sk-test")
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, r)
		wantStatus, wantCalls := 200, 1
		if density == "invalid" {
			wantStatus, wantCalls = 400, 0
		}
		if w.Code != wantStatus || calls != wantCalls {
			t.Fatalf("%s: status %d, calls %d", density, w.Code, calls)
		}
	}
}

func TestCvLanguageIsExplicitAndValidated(t *testing.T) {
	facts := []fact{{ID: "f0", Section: "skills", Field: "skills", Text: "Research"}}
	for _, tc := range []struct {
		language, legacy string
		want             bool
	}{
		{"en", "", true}, {"pt-BR", "", true}, {"", "en", true},
		{"fr", "", false}, {"", "", false}, {"pt-BR", "en", false},
	} {
		if validRequest(request{CvLanguage: tc.language, Locale: tc.legacy, Facts: facts}) != tc.want {
			t.Fatalf("language=%q legacy=%q", tc.language, tc.legacy)
		}
	}
	a := app{client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var input map[string]any
		_ = json.Unmarshal([]byte(body.Messages[1].Content), &input)
		if input["cvLanguage"] != "pt-BR" || input["locale"] != nil || input["uiLocale"] != nil {
			t.Fatalf("unexpected provider input: %v", input)
		}
		if !strings.Contains(body.Messages[0].Content, "independently of the site language") {
			t.Fatal("missing language policy")
		}
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	_, _ = a.call(t.Context(), "sk-test", request{Locale: "pt-BR", Facts: facts})
}

func TestStableGenerationThroughHandler(t *testing.T) {
	for _, language := range []string{"en", "pt-BR"} {
		t.Run(language, func(t *testing.T) {
			input := `{"cvLanguage":"` + language + `","facts":[{"id":"stable-skill","section":"skills","entryId":"","field":"skills","text":"Research and Design","reference":{"profileId":"profile-a","id":"stable-skill","revision":3},"owner":{"profileId":"profile-a","id":"profile-a","revision":1},"kind":"legacy_block","assertion":"unknown","intent":"unknown","certainty":"unknown","support":"unsupported"}]}`
			handler := NewHandlerWithClient(&http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				var payload map[string]any
				json.NewDecoder(r.Body).Decode(&payload)
				format := payload["response_format"].(map[string]any)
				if format["type"] != "json_schema" {
					t.Error("provider-enforced schema missing")
				}
				data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": `{"summary":[{"sourceIds":["stable-skill"],"text":"Research and Design"}],"selected":["stable-skill"],"wording":[]}`}}}})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
			})})
			req := httptest.NewRequest("POST", "/api/cv/generate", strings.NewReader(input))
			req.Header.Set("X-OpenAI-Api-Key", "sk-test")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"sourceIds":["stable-skill"]`) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestSparseSummaryIsRequiredByOutboundProviderContract(t *testing.T) {
	// Observe the public handler's actual outbound request, rather than invoking
	// its schema builder. Restoring the baseline JSON-object contract fails here.
	handler := NewHandlerWithClient(&http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Format struct {
				Type   string `json:"type"`
				Schema struct {
					Strict bool `json:"strict"`
					Schema struct {
						Properties map[string]struct {
							MinItems int `json:"minItems"`
							Items    struct {
								Required   []string `json:"required"`
								Properties map[string]struct {
									MinItems int `json:"minItems"`
								} `json:"properties"`
							} `json:"items"`
						} `json:"properties"`
					} `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		summary := body.Format.Schema.Schema.Properties["summary"]
		if body.Format.Type != "json_schema" || !body.Format.Schema.Strict || summary.MinItems != 1 || summary.Items.Properties["sourceIds"].MinItems != 1 {
			t.Fatal("provider contract permits the baseline empty-summary/missing-citation failure")
		}
		if strings.Join(summary.Items.Required, ",") != "sourceIds,text" {
			t.Fatalf("missing required citation/text: %v", summary.Items.Required)
		}
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/cv/generate", strings.NewReader(`{"cvLanguage":"en","facts":[{"id":"java","section":"skills","entryId":"","field":"skills","text":"Java"}]}`))
	req.Header.Set("X-OpenAI-Api-Key", "sk-controlled")
	handler.ServeHTTP(rec, req)
	if rec.Code != 502 {
		t.Fatalf("handler did not reach controlled provider: %d", rec.Code)
	}
}

// ---- ADR 0003 (#124) statement-support handler contract ----

// providerScript.status 0 simulates a provider transport failure (timeout).
type providerScript struct {
	status int
	body   string
}

func rawEnvelope(finish, refusal, content string) string {
	raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]any{"content": content, "refusal": refusal}}}})
	return string(raw)
}
func envelope(content string) string { return rawEnvelope("stop", "", content) }

// scripted captures every outbound provider body so tests can assert call counts
// and bounds. Exhausted scripts fail the call instead of silently reusing data.
func scripted(script ...providerScript) (roundTrip, *[]string) {
	calls := &[]string{}
	return func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		*calls = append(*calls, string(raw))
		index := len(*calls) - 1
		if index >= len(script) {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		if script[index].status == 0 {
			return nil, context.DeadlineExceeded
		}
		return &http.Response{StatusCode: script[index].status, Body: io.NopCloser(strings.NewReader(script[index].body))}, nil
	}, calls
}

func generationContent(claim string, ids ...string) string {
	sources := make([]any, 0, len(ids))
	for _, id := range ids {
		sources = append(sources, id)
	}
	raw, _ := json.Marshal(map[string]any{"summary": []any{map[string]any{"sourceIds": sources, "text": claim}}, "selected": sources, "wording": []any{}})
	return string(raw)
}
func judgmentContent(id, status, reason string) string {
	raw, _ := json.Marshal(map[string]any{"judgments": []any{map[string]any{"id": id, "status": status, "reason": reason}}})
	return string(raw)
}

type v3Envelope struct {
	SchemaVersion int `json:"schemaVersion"`
	Summary       []struct {
		SourceID  string   `json:"sourceId"`
		SourceIDs []string `json:"sourceIds"`
		Text      string   `json:"text"`
	} `json:"summary"`
	Selected []string `json:"selected"`
	Support  struct {
		PolicyVersion string `json:"policyVersion"`
		Assessment    string `json:"assessment"`
		Statements    []struct {
			ID        string   `json:"id"`
			Text      string   `json:"text"`
			SourceIDs []string `json:"sourceIds"`
			Status    string   `json:"status"`
			Reason    string   `json:"reason"`
		} `json:"statements"`
	} `json:"support"`
}

func generateThroughHandler(t *testing.T, transport roundTrip, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/cv/generate", strings.NewReader(string(raw)))
	req.Header.Set("X-OpenAI-Api-Key", "sk-controlled")
	rec := httptest.NewRecorder()
	NewHandlerWithClient(&http.Client{Transport: transport}).ServeHTTP(rec, req)
	return rec
}
func decodeV3(t *testing.T, rec *httptest.ResponseRecorder) v3Envelope {
	t.Helper()
	var out v3Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not v3: %v %s", err, rec.Body.String())
	}
	return out
}
func providerRequest(t *testing.T, raw string) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("provider body not json: %v", err)
	}
	return body
}

// A well-formed generated claim with valid citations but no semantic support must
// never be marked supported, in EN or PT-BR, and the backend must not certify it
// from citations alone. The retained Java -> Kubernetes reproduction is included.
func TestStatementSupportRequiresSemanticSupportNotCitations(t *testing.T) {
	facts := []fact{{ID: "java", Section: "skills", Field: "skills", Text: "Java"}}
	for _, tc := range []struct {
		id, language, claim string
	}{
		{"en-kubernetes", "en", "Builds production Kubernetes platforms."},
		{"pt-kubernetes", "pt-BR", "Desenvolve plataformas Kubernetes em produção."},
		{"en-tool", "en", "Operates Kubernetes clusters in production."},
		{"pt-responsibility", "pt-BR", "Gerencia a contratação e a mentoria de engenheiros."},
		{"en-qualification", "en", "Maintains Kubernetes administration credentials."},
	} {
		t.Run(tc.id, func(t *testing.T) {
			transport, calls := scripted(
				providerScript{200, envelope(generationContent(tc.claim, "java"))},
				providerScript{200, envelope(judgmentContent("summary:0", "unsupported", "Cited Java fact does not support this claim."))},
			)
			rec := generateThroughHandler(t, transport, map[string]any{"cvLanguage": tc.language, "statementSupport": true, "facts": facts})
			if rec.Code != 200 {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if len(*calls) != 2 {
				t.Fatalf("provider calls %d, want exactly generation + one assessment", len(*calls))
			}
			out := decodeV3(t, rec)
			if out.SchemaVersion != 3 || out.Support.PolicyVersion != supportPolicyVersion || out.Support.Assessment != "complete" {
				t.Fatalf("support envelope: %+v", out)
			}
			if len(out.Support.Statements) != 1 {
				t.Fatalf("statements: %+v", out.Support.Statements)
			}
			statement := out.Support.Statements[0]
			if statement.Status == "supported" {
				t.Fatalf("valid citation was certified as supported: %+v", statement)
			}
			if statement.Status != "unsupported" || statement.ID != "summary:0" || statement.Text != tc.claim || len(statement.SourceIDs) != 1 || statement.SourceIDs[0] != "java" {
				t.Fatalf("statement: %+v", statement)
			}
		})
	}
}

// Any assessment failure leaves generated wording uncertain and recoverable; it
// must never be upgraded to supported and must never fail the request.
func TestUnavailableAssessmentFailsClosed(t *testing.T) {
	facts := []fact{{ID: "java", Section: "skills", Field: "skills", Text: "Java"}}
	generation := generationContent("Builds production Kubernetes platforms.", "java")
	valid := judgmentContent("summary:0", "supported", "ok")
	for _, tc := range []struct {
		name     string
		response providerScript
	}{
		{"outage", providerScript{500, ""}},
		{"rate_limit", providerScript{429, ""}},
		{"timeout", providerScript{}},
		{"refusal", providerScript{200, rawEnvelope("stop", "declined", "")}},
		{"truncated", providerScript{200, rawEnvelope("length", "", valid)}},
		{"malformed", providerScript{200, envelope("{bad")}},
		{"no_judgments", providerScript{200, envelope(`{"judgments":[]}`)}},
		{"unknown_id", providerScript{200, envelope(judgmentContent("summary:9", "supported", "ok"))}},
		{"unknown_status", providerScript{200, envelope(judgmentContent("summary:0", "maybe", "ok"))}},
		{"unknown_field", providerScript{200, envelope(strings.Replace(valid, `}]}`, `,"extra":1}]}`, 1))}},
		{"duplicate", providerScript{200, envelope(`{"judgments":[{"id":"summary:0","status":"supported","reason":"ok"},{"id":"summary:0","status":"supported","reason":"ok"}]}`)}},
		{"extra_judgment", providerScript{200, envelope(`{"judgments":[{"id":"summary:0","status":"supported","reason":"ok"},{"id":"summary:1","status":"supported","reason":"ok"}]}`)}},
		{"oversize_reason", providerScript{200, envelope(judgmentContent("summary:0", "supported", strings.Repeat("x", maxReasonBytes+1)))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport, calls := scripted(providerScript{200, envelope(generation)}, tc.response)
			rec := generateThroughHandler(t, transport, map[string]any{"cvLanguage": "en", "statementSupport": true, "facts": facts})
			if rec.Code != 200 {
				t.Fatalf("recoverable candidate lost: %d %s", rec.Code, rec.Body.String())
			}
			if len(*calls) != 2 {
				t.Fatalf("provider calls %d, want 2", len(*calls))
			}
			out := decodeV3(t, rec)
			if out.Support.Assessment != "unavailable" {
				t.Fatalf("assessment %q: %+v", out.Support.Assessment, out.Support)
			}
			if len(out.Support.Statements) != 1 || out.Support.Statements[0].Status != "uncertain" || out.Support.Statements[0].Reason != supportUnavailableReason {
				t.Fatalf("statements: %+v", out.Support.Statements)
			}
		})
	}
}

// Faithful paraphrases/translations that the assessor supports stay usable; the
// existing heuristic guards are not a substitute for the assessment.
func TestSupportedParaphraseRemainsUsable(t *testing.T) {
	facts := []fact{{ID: "d", Section: "experience", EntryID: "e", Field: "description", Text: "Used research to improve services by 20%."}}
	transport, calls := scripted(
		providerScript{200, envelope(generationContent("Improved services by 20% using research.", "d"))},
		providerScript{200, envelope(judgmentContent("summary:0", "supported", "Faithful paraphrase of the cited fact."))},
	)
	rec := generateThroughHandler(t, transport, map[string]any{"cvLanguage": "pt-BR", "statementSupport": true, "facts": facts})
	if rec.Code != 200 || len(*calls) != 2 {
		t.Fatalf("status %d calls %d", rec.Code, len(*calls))
	}
	out := decodeV3(t, rec)
	if out.Support.Assessment != "complete" || out.Support.Statements[0].Status != "supported" {
		t.Fatalf("supported paraphrase rejected: %+v", out.Support)
	}
}

// Rewrites are separate review units with server-assigned IDs and a judgment is
// required for each one.
func TestWordingUnitsAreAssessedSeparately(t *testing.T) {
	facts := []fact{{ID: "d", Section: "experience", EntryID: "e", Field: "description", Text: "Designed customer services."}}
	generation := `{"summary":[{"sourceIds":["d"],"text":"Designed customer services."}],"selected":["d"],"wording":[{"sourceId":"d","text":"Designed customer services for clients."}]}`
	t.Run("complete", func(t *testing.T) {
		transport, calls := scripted(
			providerScript{200, envelope(generation)},
			providerScript{200, envelope(`{"judgments":[{"id":"summary:0","status":"supported","reason":"ok"},{"id":"wording:d","status":"supported","reason":"ok"}]}`)},
		)
		rec := generateThroughHandler(t, transport, map[string]any{"cvLanguage": "en", "statementSupport": true, "facts": facts})
		if rec.Code != 200 || len(*calls) != 2 {
			t.Fatalf("status %d calls %d: %s", rec.Code, len(*calls), rec.Body.String())
		}
		out := decodeV3(t, rec)
		if len(out.Support.Statements) != 2 || out.Support.Statements[0].ID != "summary:0" || out.Support.Statements[1].ID != "wording:d" {
			t.Fatalf("units: %+v", out.Support.Statements)
		}
		if out.Support.Statements[1].SourceIDs[0] != "d" {
			t.Fatalf("rewrite citation: %+v", out.Support.Statements[1])
		}
	})
	t.Run("missing_wording_judgment", func(t *testing.T) {
		transport, _ := scripted(
			providerScript{200, envelope(generation)},
			providerScript{200, envelope(judgmentContent("summary:0", "supported", "ok"))},
		)
		rec := generateThroughHandler(t, transport, map[string]any{"cvLanguage": "en", "statementSupport": true, "facts": facts})
		out := decodeV3(t, rec)
		if rec.Code != 200 || out.Support.Assessment != "unavailable" {
			t.Fatalf("incomplete judgments not rejected: %d %+v", rec.Code, out.Support)
		}
	})
}

// The assessment is opt-in: without it the legacy v2 response and a single
// provider call are preserved, and the control flag never reaches a provider.
func TestStatementSupportIsOptIn(t *testing.T) {
	facts := []fact{{ID: "java", Section: "skills", Field: "skills", Text: "Java"}}
	transport, calls := scripted(providerScript{200, envelope(generationContent("Java", "java"))})
	rec := generateThroughHandler(t, transport, map[string]any{"cvLanguage": "en", "facts": facts})
	if rec.Code != 200 || len(*calls) != 1 {
		t.Fatalf("legacy status %d calls %d: %s", rec.Code, len(*calls), rec.Body.String())
	}
	var legacy map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &legacy); err != nil {
		t.Fatal(err)
	}
	if _, ok := legacy["schemaVersion"]; ok {
		t.Fatal("legacy client received schemaVersion")
	}
	if _, ok := legacy["support"]; ok {
		t.Fatal("legacy client received support metadata")
	}
	if strings.Contains((*calls)[0], "statementSupport") {
		t.Fatal("control flag reached the generation provider")
	}
}

// The bounded assessment request carries only cited facts, keeps the approved
// ceilings, and never adds retries, a second model or unrelated data.
func TestAssessmentRequestIsBoundedAndCited(t *testing.T) {
	facts := []fact{
		{ID: "java", Section: "skills", Field: "skills", Text: "Java"},
		{ID: "d", Section: "experience", EntryID: "e", Field: "achievements", Text: "Reduced wait by 20%."},
	}
	transport, calls := scripted(
		providerScript{200, envelope(generationContent("Reduced wait by 20%.", "d"))},
		providerScript{200, envelope(judgmentContent("summary:0", "supported", "ok"))},
	)
	rec := generateThroughHandler(t, transport, map[string]any{"cvLanguage": "en", "statementSupport": true, "facts": facts})
	if rec.Code != 200 || len(*calls) != 2 {
		t.Fatalf("status %d calls %d", rec.Code, len(*calls))
	}
	for index, want := range []string{"curated_cv_v2", "cv_statement_support_v1"} {
		body := providerRequest(t, (*calls)[index])
		if body["model"] != "gpt-6-luna" || body["reasoning_effort"] != "none" || body["max_completion_tokens"] != float64(4000) {
			t.Fatalf("call %d policy: %v", index, body)
		}
		format := body["response_format"].(map[string]any)["json_schema"].(map[string]any)
		if format["name"] != want || format["strict"] != true {
			t.Fatalf("call %d schema: %v", index, format["name"])
		}
		if len((*calls)[index]) > maxAssessmentPayload {
			t.Fatalf("call %d exceeded payload bound", index)
		}
	}
	assessment := providerRequest(t, (*calls)[1])
	messages := assessment["messages"].([]any)
	if !strings.Contains(messages[0].(map[string]any)["content"].(string), "Valid source IDs establish traceability only") {
		t.Fatal("assessment policy missing")
	}
	var payload assessmentPayload
	if err := json.Unmarshal([]byte(messages[1].(map[string]any)["content"].(string)), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Statements) != 1 || payload.Statements[0].ID != "summary:0" {
		t.Fatalf("statements: %+v", payload.Statements)
	}
	if len(payload.Facts) != 1 || payload.Facts[0].ID != "d" {
		t.Fatalf("assessment received uncited or unrelated facts: %+v", payload.Facts)
	}
}

// Deterministic hard guards still fail before any assessment; no assessment can
// override them and no extra provider call is made.
func TestHardGuardsFailBeforeAssessment(t *testing.T) {
	for _, tc := range []struct {
		name       string
		facts      []fact
		generation string
	}{
		{"invented_number", []fact{{ID: "f0", Section: "experience", EntryID: "e", Field: "achievements", Text: "Reduced wait by 20%."}}, generationContent("Reduced wait by 90%.", "f0")},
		{"negation_stripped", []fact{{ID: "f0", Section: "experience", EntryID: "e", Field: "achievements", Text: "Did not reduce wait by 20%."}}, generationContent("Reduced wait by 20%.", "f0")},
		{"cross_owner_mix", []fact{{ID: "a", Section: "experience", EntryID: "e1", Field: "achievements", Text: "Reduced wait by 20%."}, {ID: "b", Section: "experience", EntryID: "e2", Field: "achievements", Text: "Designed services."}}, generationContent("Designed services and reduced wait by 20%.", "a", "b")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport, calls := scripted(
				providerScript{200, envelope(tc.generation)},
				providerScript{200, envelope(judgmentContent("summary:0", "supported", "ok"))},
			)
			rec := generateThroughHandler(t, transport, map[string]any{"cvLanguage": "en", "statementSupport": true, "facts": tc.facts})
			if rec.Code != 502 || !strings.Contains(rec.Body.String(), "invalid_output") {
				t.Fatalf("guard not enforced: %d %s", rec.Code, rec.Body.String())
			}
			if len(*calls) != 1 {
				t.Fatalf("assessment called after a hard failure: %d", len(*calls))
			}
		})
	}
}

// Assessment is skipped, never truncated, when it cannot fit the approved
// serialized capacity: the candidate stays available with uncertain status.
func TestAssessmentCapacityIsBounded(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	in := request{CvLanguage: "en", Facts: []fact{{ID: "big", Section: "experience", EntryID: "e", Field: "achievements", Text: strings.Repeat("x", maxAssessmentPayload+1)}}}
	out := result{Summary: []excerpt{{SourceID: "big", Text: strings.Repeat("x", maxAssessmentPayload+1)}}, Selected: []string{"big"}}
	support := a.assess(t.Context(), "sk-test", in, out)
	if calls != 0 || support.Assessment != "unavailable" || len(support.Statements) != 1 || support.Statements[0].Status != "uncertain" || support.Statements[0].Reason != supportUnavailableReason {
		t.Fatalf("capacity guard: calls %d support %+v", calls, support)
	}
}

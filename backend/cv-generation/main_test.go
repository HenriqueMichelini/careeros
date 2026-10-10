package cvgeneration

import (
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

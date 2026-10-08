package applicationdraft

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	profilevalidation "professional-information-repo/internal/profilevalidation"
	"professional-information-repo/internal/testsupport"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "api.typesafe.ai" {
		return testsupport.AcceptedJob(), nil
	}
	return f(r)
}

type timeoutReadError struct{}

func (timeoutReadError) Error() string   { return "timed out" }
func (timeoutReadError) Timeout() bool   { return true }
func (timeoutReadError) Temporary() bool { return true }

type timeoutReader struct{}

func (timeoutReader) Read([]byte) (int, error) { return 0, timeoutReadError{} }
func draftResponse() string {
	content := `{"jobTitle":"Engineer","company":"Example","jobSummary":"Build systems","resume":"# Resume","coverLetter":{"greeting":"Dear team,","body":"I build systems.","closing":"Sincerely,"},"applicationAnswers":"Q&A"}`
	encoded, _ := json.Marshal(content)
	return `{"choices":[{"message":{"content":` + string(encoded) + `}}]}`
}

func TestProviderUsesOpenAIAndFullPopulatedProfile(t *testing.T) {
	in := request{JobPosting: "Senior engineer role", Profile: profilevalidation.Profile{CareerGoals: "leadership", Skills: "Go", Competencies: "systems", Tools: "Docker", EmploymentStatus: "employed", CurrentSalary: "salary", DesiredSalary: "target", AdditionalInfo: "extra", Experience: []profilevalidation.Experience{{ID: "id", Company: "company", Title: "engineer", StartDate: "2020", EndDate: "2022", Location: "location", Description: "description", Responsibilities: "responsibilities", Achievements: "achievement"}}, Projects: []profilevalidation.Project{{ID: "project-id", Name: "project", Description: "project detail", Technologies: "Go", URL: "url", Highlights: "highlight"}}}, Confirmed: []qualification{{Kind: "skill", Requirement: "Kubernetes", UserContext: "used it"}}}
	in.Qualifications = &profilevalidation.Qualifications{Education: []profilevalidation.Education{{ID: "private-education-id", Degree: "BSc", Institution: "Example University"}}}
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "api.openai.com" || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer sk-valid" {
			t.Fatal("incorrect provider endpoint or auth")
		}
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal(body, &payload) != nil || payload.Model != model || len(payload.Messages) != 1 {
			t.Fatal("incorrect provider request")
		}
		for _, want := range []string{"leadership", "Go", "company", "location", "salary", "target", "extra", "project-id", "Kubernetes", "used it", "BSc", "Example University"} {
			if !strings.Contains(payload.Messages[0].Content, want) {
				t.Errorf("full profile or confirmed context missing %q", want)
			}
		}
		if strings.Contains(payload.Messages[0].Content, "private-education-id") {
			t.Error("local qualification ID reached provider")
		}
		previous := -1
		for _, heading := range []string{"Professional Summary", "Technical Skills", "Professional Experience", "Education", "Certifications", "Languages"} {
			index := strings.Index(payload.Messages[0].Content, heading)
			if index <= previous {
				t.Errorf("resume heading %q missing or out of order", heading)
			}
			previous = index
		}
		if !strings.Contains(payload.Messages[0].Content, "Do not add a name or contact header") {
			t.Error("resume must leave identity and contact to saved Profile facts")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(draftResponse())), Header: make(http.Header)}, nil
	})}}
	out, code, err := a.call(t.Context(), "sk-valid", in)
	if err != nil || code != "" || calls != 1 || !valid(out) {
		t.Fatalf("result=%+v code=%s err=%v calls=%d", out, code, err, calls)
	}
}

func TestOmitEmptyProfileFieldsKeepsPopulatedFields(t *testing.T) {
	raw := omitEmptyProfileFields([]byte(`{"careerGoals":"","skills":"Go","competencies":"","tools":"Docker","experience":[],"projects":[],"employmentStatus":"","currentSalary":"","desiredSalary":"","additionalInfo":""}`))
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		t.Fatal("invalid JSON")
	}
	if len(fields) != 2 || string(fields["skills"]) != `"Go"` || string(fields["tools"]) != `"Docker"` {
		t.Fatalf("unexpected fields: %s", raw)
	}
}

func TestQualificationFactsOmitLocalIDs(t *testing.T) {
	facts := qualificationFacts(profilevalidation.Qualifications{
		Education: []profilevalidation.Education{{ID: "private-local-id", Degree: "BSc", Institution: "Example University", GraduationDate: "2018"}},
		Languages: []profilevalidation.Language{{ID: "another-local-id", Name: "English", Proficiency: "Fluent"}},
	})
	serialized, _ := json.Marshal(facts)
	for _, want := range []string{"BSc", "Example University", "2018", "English", "Fluent"} {
		if !strings.Contains(string(serialized), want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(string(serialized), "local-id") || strings.Contains(string(serialized), `"id"`) {
		t.Fatal("local ID reached provider projection")
	}
}

func TestValidRejectsIncompleteDraft(t *testing.T) {
	if valid(result{JobTitle: "Engineer", Company: "Example"}) {
		t.Fatal("incomplete result accepted")
	}
}

func TestRateLimitMapsWithoutRetryAndDoesNotCache(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})}}
	body := `{"repository":{"careerGoals":"","skills":"Go","competencies":"","experience":[],"tools":"Docker","projects":[],"employmentStatus":"","currentSalary":"","desiredSalary":"","additionalInfo":""},"jobPosting":"Engineer","confirmedQualifications":[]}`
	r := httptest.NewRequest(http.MethodPost, "/api/application-draft", strings.NewReader(body))
	r.Header.Set("X-OpenAI-Api-Key", "sk-valid")
	r.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests || calls != 1 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"error":"rate_limit"`) {
		t.Fatalf("status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Header(), w.Body.String())
	}
}

func TestMissingKeyRejectedBeforeProvider(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, nil })}}
	r := httptest.NewRequest(http.MethodPost, "/api/application-draft", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized || calls != 0 || !strings.Contains(w.Body.String(), `"error":"key"`) {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}
}

func TestSyntheticDraftOutboundBodyUsesApprovedProfileAndConfirmation(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoning_effort"`
			MaxTokens       int    `json:"max_completion_tokens"`
			ResponseFormat  struct {
				Type string `json:"type"`
			} `json:"response_format"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		var topLevel map[string]json.RawMessage
		if json.Unmarshal(body, &payload) != nil || json.Unmarshal(body, &topLevel) != nil ||
			len(topLevel) != 5 || payload.Model != model || payload.ReasoningEffort != "none" ||
			payload.MaxTokens != 8000 || payload.ResponseFormat.Type != "json_schema" ||
			len(payload.Messages) != 1 || payload.Messages[0].Role != "user" {
			t.Fatal("bad provider body")
		}
		prompt := payload.Messages[0].Content
		for _, forbidden := range []string{`"currentSalary"`, `"desiredSalary"`, `"additionalInfo"`, `"projects"`, `"tools"`, `"location":"San Francisco"`, `"url"`} {
			if strings.Contains(prompt, forbidden) {
				t.Errorf("forbidden field %s in provider prompt", forbidden)
			}
		}
		for _, allowed := range []string{`"careerGoals":"Lead data projects"`, `"skills":"SQL, dashboard design"`, `"competencies":"Clear communication"`, `"employmentStatus":"employed-full-time"`, `"title":"Data Analyst"`, `"company":"Northstar Analytics"`, `"achievements":"Reduced report preparation time"`, "QUALIFICATION ONLY SYNTHETIC POSTING", `"kind":"skill"`, `"requirement":"Kubernetes"`, `"userContext":"Confirmed for this role only"`} {
			if !strings.Contains(prompt, allowed) {
				t.Errorf("approved content missing: %s", allowed)
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(draftResponse())), Header: make(http.Header)}, nil
	})}}
	body := `{"repository":{"careerGoals":"Lead data projects","skills":"SQL, dashboard design","competencies":"Clear communication","experience":[{"id":"synthetic-1","company":"Northstar Analytics","title":"Data Analyst","startDate":"Jan 2022","endDate":"Jun 2024","current":false,"location":"","description":"Built dashboards","responsibilities":"Translated reporting needs","achievements":"Reduced report preparation time"}],"tools":"","projects":[],"employmentStatus":"employed-full-time","currentSalary":"","desiredSalary":"","additionalInfo":""},"jobPosting":"QUALIFICATION ONLY SYNTHETIC POSTING","confirmedQualifications":[{"kind":"skill","requirement":"Kubernetes","userContext":"Confirmed for this role only"}]}`
	r := httptest.NewRequest(http.MethodPost, "/api/application-draft", strings.NewReader(body))
	r.Header.Set("X-OpenAI-Api-Key", "sk-valid")
	r.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || calls != 1 || !strings.Contains(w.Body.String(), `"jobTitle":"Engineer"`) {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}
}

func TestInvalidInputAndTrailingJSONRejectedBeforeProvider(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, nil })}}
	validBody := `{"repository":{"careerGoals":"","skills":"Go","competencies":"","experience":[],"tools":"Docker","projects":[],"employmentStatus":"","currentSalary":"","desiredSalary":"","additionalInfo":""},"jobPosting":"Engineer","confirmedQualifications":[]}`
	for _, body := range []string{
		`{}`,
		validBody + ` {}`,
		strings.Replace(validBody, `"jobPosting":"Engineer"`, `"jobPosting":""`, 1),
		strings.Replace(validBody, `"repository":{`, `"repository":null`, 1),
		strings.Replace(validBody, `"careerGoals":"","skills":"Go",`, `"skills":"Go",`, 1),
		strings.Replace(validBody, `"experience":[]`, `"experience":null`, 1),
		strings.Replace(validBody, `"projects":[]`, `"projects":null`, 1),
		strings.Replace(validBody, `"confirmedQualifications":[]`, `"confirmedQualifications":null`, 1),
		strings.Replace(validBody, `"skills":"Go"`, `"skills":null`, 1),
		strings.Replace(validBody, `"jobPosting":"Engineer"`, `"jobPosting":null`, 1),
		strings.Replace(validBody, `"confirmedQualifications":[]`, `"confirmedQualifications":[{"kind":"skill","requirement":"Go","userContext":null}]`, 1),
		strings.Replace(validBody, `"experience":[]`, `"experience":[{"id":"x","company":"","title":"","startDate":"","endDate":"","current":null,"location":"","description":"","responsibilities":"","achievements":""}]`, 1),
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/application-draft", strings.NewReader(body))
		r.Header.Set("X-OpenAI-Api-Key", "sk-valid")
		r.Header.Set("X-TypeSafe-Api-Key", "synthetic")
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"error":"input"`) {
			t.Errorf("body=%s status=%d response=%s", body, w.Code, w.Body.String())
		}
	}
	if calls != 0 {
		t.Fatalf("provider called %d times for invalid input", calls)
	}
}

func TestInputLimitsRejectedBeforeProvider(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, nil })}}
	validBody := `{"repository":{"careerGoals":"","skills":"Go","competencies":"","experience":[],"tools":"Docker","projects":[],"employmentStatus":"","currentSalary":"","desiredSalary":"","additionalInfo":""},"jobPosting":"Engineer","confirmedQualifications":[]}`
	profileTooLong := strings.Replace(validBody, `"skills":"Go"`, `"skills":"`+strings.Repeat("x", 12<<10+1)+`"`, 1)
	postingTooLong := strings.Replace(validBody, `"jobPosting":"Engineer"`, `"jobPosting":"`+strings.Repeat("x", 30<<10+1)+`"`, 1)
	qualificationTooLong := strings.Replace(validBody, `"confirmedQualifications":[]`, `"confirmedQualifications":[{"kind":"skill","requirement":"Go","userContext":"`+strings.Repeat("x", 2001)+`"}]`, 1)
	tooManyQualifications := strings.Replace(validBody, `"confirmedQualifications":[]`, `"confirmedQualifications":[`+strings.TrimSuffix(strings.Repeat(`{"kind":"skill","requirement":"Go","userContext":""},`, 26), ",")+`]`, 1)
	for _, body := range []string{profileTooLong, postingTooLong, qualificationTooLong, tooManyQualifications, strings.Repeat(" ", maxRequestBytes) + validBody} {
		r := httptest.NewRequest(http.MethodPost, "/api/application-draft", strings.NewReader(body))
		r.Header.Set("X-OpenAI-Api-Key", "sk-valid")
		r.Header.Set("X-TypeSafe-Api-Key", "synthetic")
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"error":"input"`) {
			t.Errorf("body length=%d status=%d response=%s", len(body), w.Code, w.Body.String())
		}
	}
	if calls != 0 {
		t.Fatalf("provider called %d times for oversized input", calls)
	}
}

func TestTimeoutDuringProviderBodyReadMapsToTimeout(t *testing.T) {
	a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(timeoutReader{}), Header: make(http.Header)}, nil
	})}}
	body := `{"repository":{"careerGoals":"","skills":"Go","competencies":"","experience":[],"tools":"Docker","projects":[],"employmentStatus":"","currentSalary":"","desiredSalary":"","additionalInfo":""},"jobPosting":"Engineer","confirmedQualifications":[]}`
	r := httptest.NewRequest(http.MethodPost, "/api/application-draft", strings.NewReader(body))
	r.Header.Set("X-OpenAI-Api-Key", "sk-valid")
	r.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r)
	if w.Code != http.StatusGatewayTimeout || !strings.Contains(w.Body.String(), `"error":"timeout"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestProviderKeyAndOutageErrorsAreCategorized(t *testing.T) {
	for _, tc := range []struct {
		status     int
		want       string
		httpStatus int
	}{{401, "key", 401}, {403, "key", 401}, {503, "outage", 502}} {
		t.Run(tc.want+http.StatusText(tc.status), func(t *testing.T) {
			calls := 0
			a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
			})}}
			body := `{"repository":{"careerGoals":"","skills":"Go","competencies":"","experience":[],"tools":"Docker","projects":[],"employmentStatus":"","currentSalary":"","desiredSalary":"","additionalInfo":""},"jobPosting":"Engineer","confirmedQualifications":[]}`
			r := httptest.NewRequest(http.MethodPost, "/api/application-draft", strings.NewReader(body))
			r.Header.Set("X-OpenAI-Api-Key", "sk-valid")
			r.Header.Set("X-TypeSafe-Api-Key", "synthetic")
			w := httptest.NewRecorder()
			a.handler().ServeHTTP(w, r)
			if w.Code != tc.httpStatus || calls != 1 || !strings.Contains(w.Body.String(), `"error":"`+tc.want+`"`) {
				t.Fatalf("status=%d calls=%d response=%s", w.Code, calls, w.Body.String())
			}
		})
	}
}

func TestIncompleteAndMalformedProviderDraftsAreRejected(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "punctuated signature", content: `{"jobTitle":"Engineer","company":"Example","jobSummary":"Build systems","resume":"# Resume","coverLetter":{"greeting":"Dear team,","body":"I build systems.\n\nJane Doe, Ph.D.","closing":"Sincerely,"},"applicationAnswers":"Q&A"}`},
		{name: "missing closing", content: `{"jobTitle":"Engineer","company":"Example","jobSummary":"Build systems","resume":"# Resume","coverLetter":{"greeting":"Dear team,","body":"I build systems."},"applicationAnswers":"Q&A"}`},
		{name: "unexpected signature", content: `{"jobTitle":"Engineer","company":"Example","jobSummary":"Build systems","resume":"# Resume","coverLetter":{"greeting":"Dear team,","body":"I build systems.","closing":"Sincerely,\nJane Doe"},"applicationAnswers":"Q&A"}`},
		{name: "signature in body", content: `{"jobTitle":"Engineer","company":"Example","jobSummary":"Build systems","resume":"# Resume","coverLetter":{"greeting":"Dear team,","body":"I build systems.\n\nSincerely,\nJane Doe","closing":"Sincerely,"},"applicationAnswers":"Q&A"}`},
		{name: "missing field", content: `{"jobTitle":"Engineer","company":"Example","jobSummary":"Build systems","resume":"# Resume","coverLetter":{"greeting":"Dear team,","body":"I build systems.","closing":"Sincerely,"}}`},
		{name: "blank field", content: `{"jobTitle":"Engineer","company":"Example","jobSummary":"Build systems","resume":"# Resume","coverLetter":{"greeting":"Dear team,","body":"I build systems.","closing":"Sincerely,"},"applicationAnswers":"  "}`},
		{name: "unknown field", content: `{"jobTitle":"Engineer","company":"Example","jobSummary":"Build systems","resume":"# Resume","coverLetter":{"greeting":"Dear team,","body":"I build systems.","closing":"Sincerely,"},"applicationAnswers":"Q&A","extra":"value"}`},
		{name: "trailing data", content: `{"jobTitle":"Engineer","company":"Example","jobSummary":"Build systems","resume":"# Resume","coverLetter":{"greeting":"Dear team,","body":"I build systems.","closing":"Sincerely,"},"applicationAnswers":"Q&A"} trailing`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":` + mustJSONString(t, tc.content) + `}}]}`)), Header: make(http.Header)}, nil
			})}}
			body := `{"repository":{"careerGoals":"","skills":"Go","competencies":"","experience":[],"tools":"Docker","projects":[],"employmentStatus":"","currentSalary":"","desiredSalary":"","additionalInfo":""},"jobPosting":"Engineer","confirmedQualifications":[]}`
			r := httptest.NewRequest(http.MethodPost, "/api/application-draft", strings.NewReader(body))
			r.Header.Set("X-OpenAI-Api-Key", "sk-valid")
			r.Header.Set("X-TypeSafe-Api-Key", "synthetic")
			w := httptest.NewRecorder()
			a.handler().ServeHTTP(w, r)
			if w.Code != http.StatusBadGateway || calls != 1 || !strings.Contains(w.Body.String(), `"error":"invalid_output"`) {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
			}
		})
	}
}

func mustJSONString(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestProviderAcceptsProseWithEnglishAndPortugueseTransitions(t *testing.T) {
	for _, body := range []string{"Additionally, I build reliable systems.", "Atualmente, desenvolvo sistemas confiáveis.", "I am AWS certified.", "At Harbor Works, I build reliable systems."} {
		t.Run(body, func(t *testing.T) {
			a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				response := strings.Replace(draftResponse(), "I build systems.", body, 1)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(response)), Header: make(http.Header)}, nil
			})}}
			out, code, err := a.call(t.Context(), "sk-valid", request{JobPosting: "Engineer"})
			if err != nil || code != "" || out.CoverLetter.Body != body {
				t.Fatalf("body=%q code=%s err=%v", out.CoverLetter.Body, code, err)
			}
		})
	}
}

func TestCvLanguageControlsApplicationProse(t *testing.T) {
	for _, language := range []string{"en", "pt-BR"} {
		a := app{client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			var body struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			prompt := body.Messages[0].Content
			if !strings.Contains(prompt, "CV language: "+language) || !strings.Contains(prompt, "independent of the site and job posting languages") {
				t.Fatal("missing explicit language policy")
			}
			if language == "pt-BR" && !strings.Contains(prompt, "Resumo Profissional, Competências Técnicas, Experiência Profissional") {
				t.Fatal("missing Portuguese headings")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(draftResponse()))}, nil
		})}}
		_, code, err := a.call(t.Context(), "sk-test", request{CvLanguage: language})
		if err != nil || code != "" {
			t.Fatal(code, err)
		}
	}
	body := `{"repository":{"careerGoals":"","skills":"Go","competencies":"","experience":[],"tools":"Docker","projects":[],"employmentStatus":"","currentSalary":"","desiredSalary":"","additionalInfo":""},"jobPosting":"Engineer","confirmedQualifications":[]}`
	for _, tc := range []struct {
		language string
		want     bool
	}{{`"en"`, true}, {`"pt-BR"`, true}, {`"fr"`, false}, {`null`, false}, {`42`, false}, {`""`, false}} {
		var raw map[string]json.RawMessage
		_ = json.Unmarshal([]byte(body), &raw)
		raw["cvLanguage"] = json.RawMessage(tc.language)
		if completeInputShape(raw) != tc.want {
			t.Fatalf("cvLanguage=%s", tc.language)
		}
		raw["uiLocale"] = json.RawMessage(`"en"`)
		if completeInputShape(raw) {
			t.Fatal("site language accepted as generation input")
		}
	}
}

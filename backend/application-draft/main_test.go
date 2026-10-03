package applicationdraft

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	profilevalidation "professional-information-repo/internal/profilevalidation"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type timeoutReadError struct{}

func (timeoutReadError) Error() string   { return "timed out" }
func (timeoutReadError) Timeout() bool   { return true }
func (timeoutReadError) Temporary() bool { return true }

type timeoutReader struct{}

func (timeoutReader) Read([]byte) (int, error) { return 0, timeoutReadError{} }
func draftResponse() string {
	return `{"choices":[{"message":{"content":"{\"jobTitle\":\"Engineer\",\"company\":\"Example\",\"jobSummary\":\"Build systems\",\"resume\":\"# Resume\",\"coverLetter\":\"Hello\",\"applicationAnswers\":\"Q&A\"}"}}]}`
}
func TestProviderUsesOpenAIAndFullPopulatedProfile(t *testing.T) {
	in := request{JobPosting: "Senior engineer role", Repository: profilevalidation.Profile{CareerGoals: "leadership", Skills: "Go", Competencies: "systems", Tools: "Docker", EmploymentStatus: "employed", CurrentSalary: "salary", DesiredSalary: "target", AdditionalInfo: "extra", Experience: []profilevalidation.Experience{{ID: "id", Company: "company", Title: "engineer", StartDate: "2020", EndDate: "2022", Location: "location", Description: "description", Responsibilities: "responsibilities", Achievements: "achievement"}}, Projects: []profilevalidation.Project{{ID: "project-id", Name: "project", Description: "project detail", Technologies: "Go", URL: "url", Highlights: "highlight"}}}, Confirmed: []qualification{{Kind: "skill", Requirement: "Kubernetes", UserContext: "used it"}}}
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
		for _, want := range []string{"leadership", "Go", "company", "location", "salary", "target", "extra", "project-id", "Kubernetes", "used it"} {
			if !strings.Contains(payload.Messages[0].Content, want) {
				t.Errorf("full profile or confirmed context missing %q", want)
			}
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

func TestSyntheticHandlerOutboundBodyHasOnlyApprovedQualificationFields(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal(body, &payload) != nil || len(payload.Messages) != 1 {
			t.Fatal("bad provider body")
		}
		prompt := payload.Messages[0].Content
		for _, forbidden := range []string{`"careerGoals"`, `"employmentStatus"`, `"currentSalary"`, `"desiredSalary"`, `"additionalInfo"`, `"experience"`, `"projects"`, `"id"`, `"company"`, `"location"`, `"url"`} {
			if strings.Contains(prompt, forbidden) {
				t.Errorf("forbidden field %s in provider prompt", forbidden)
			}
		}
		for _, allowed := range []string{`"skills":"Go"`, `"competencies":"systems"`, `"tools":"Docker"`, "QUALIFICATION ONLY SYNTHETIC POSTING"} {
			if !strings.Contains(prompt, allowed) {
				t.Errorf("approved content missing: %s", allowed)
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(draftResponse())), Header: make(http.Header)}, nil
	})}}
	body := `{"repository":{"careerGoals":"","skills":"Go","competencies":"systems","experience":[],"tools":"Docker","projects":[],"employmentStatus":"","currentSalary":"","desiredSalary":"","additionalInfo":""},"jobPosting":"QUALIFICATION ONLY SYNTHETIC POSTING","confirmedQualifications":[]}`
	r := httptest.NewRequest(http.MethodPost, "/api/application-draft", strings.NewReader(body))
	r.Header.Set("X-OpenAI-Api-Key", "sk-valid")
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
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/application-draft", strings.NewReader(body))
		r.Header.Set("X-OpenAI-Api-Key", "sk-valid")
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

func TestTimeoutDuringProviderBodyReadMapsToTimeout(t *testing.T) {
	a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(timeoutReader{}), Header: make(http.Header)}, nil
	})}}
	body := `{"repository":{"careerGoals":"","skills":"Go","competencies":"","experience":[],"tools":"Docker","projects":[],"employmentStatus":"","currentSalary":"","desiredSalary":"","additionalInfo":""},"jobPosting":"Engineer","confirmedQualifications":[]}`
	r := httptest.NewRequest(http.MethodPost, "/api/application-draft", strings.NewReader(body))
	r.Header.Set("X-OpenAI-Api-Key", "sk-valid")
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
		{name: "missing field", content: `{"jobTitle":"Engineer","company":"Example","jobSummary":"Build systems","resume":"# Resume","coverLetter":"Hello"}`},
		{name: "blank field", content: `{"jobTitle":"Engineer","company":"Example","jobSummary":"Build systems","resume":"# Resume","coverLetter":"Hello","applicationAnswers":"  "}`},
		{name: "unknown field", content: `{"jobTitle":"Engineer","company":"Example","jobSummary":"Build systems","resume":"# Resume","coverLetter":"Hello","applicationAnswers":"Q&A","extra":"value"}`},
		{name: "trailing data", content: `{"jobTitle":"Engineer","company":"Example","jobSummary":"Build systems","resume":"# Resume","coverLetter":"Hello","applicationAnswers":"Q&A"} trailing`},
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

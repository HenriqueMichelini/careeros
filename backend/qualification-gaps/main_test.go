package qualificationgaps

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func sampleRequest() gapRequest {
	return gapRequest{JobPosting: "Senior engineer required. Must know distributed systems and Go.", Repository: repository{
		CareerGoals: "SENTINEL_CAREER_GOALS", Skills: "Go and SQL", Competencies: "systems thinking", Tools: "Docker",
		Experience:       []experience{{ID: "SENTINEL_ID", Company: "SENTINEL_COMPANY", Title: "Backend engineer", StartDate: "2021-01", EndDate: "2023-01", Location: "SENTINEL_LOCATION", Description: "Built services", Responsibilities: "Owned APIs", Achievements: "Reduced latency"}},
		Projects:         []project{{ID: "SENTINEL_PROJECT_ID", Name: "SENTINEL_PROJECT_NAME", Description: "Open source service", Technologies: "Go, Redis", URL: "SENTINEL_URL", Highlights: "Added queue processing"}},
		EmploymentStatus: "SENTINEL_EMPLOYMENT", CurrentSalary: "SENTINEL_CURRENT_SALARY", DesiredSalary: "SENTINEL_DESIRED_SALARY", AdditionalInfo: "SENTINEL_ADDITIONAL_INFO",
	}}
}

type blockingReader struct{ done <-chan struct{} }

func (r blockingReader) Read([]byte) (int, error) { <-r.done; return 0, errors.New("read canceled") }
func providerOK() string {
	return `{"choices":[{"message":{"content":"{\"gaps\":[{\"kind\":\"skill\",\"requirement\":\"Distributed systems\",\"details\":\"The posting asks for distributed systems experience.\"}] }"}}]}`
}

func TestProviderRequestUsesOnlyQualificationAllowlist(t *testing.T) {
	input := sampleRequest()
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		assertQualificationProviderRequest(t, r)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(providerOK())), Header: make(http.Header)}, nil
	})}}
	result, code, err := a.callProvider(t.Context(), "sk-valid", input)
	if err != nil || code != "" || calls != 1 || !validResult(result) || len(result.Gaps) != 1 {
		t.Fatalf("result=%+v code=%s err=%v calls=%d", result, code, err, calls)
	}
}

func assertQualificationProviderRequest(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") != "Bearer sk-valid" {
		t.Fatal("key not forwarded")
	}
	body, _ := io.ReadAll(r.Body)
	var request struct {
		Model    string `json:"model"`
		Effort   string `json:"reasoning_effort"`
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &request) != nil || request.Model != model || request.Effort != "none" || len(request.Messages) != 1 {
		t.Fatal("incorrect provider request contract")
	}
	assertPromptAllowlist(t, request.Messages[0].Content)
}

func assertPromptAllowlist(t *testing.T, prompt string) {
	t.Helper()
	for _, want := range []string{"Senior engineer required", "Go and SQL", "systems thinking", "Docker", "Backend engineer", "Built services", "Owned APIs", "Reduced latency", "Open source service", "Go, Redis", "Added queue processing", "2 years"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("allowlisted content %q missing", want)
		}
	}
	for _, forbidden := range []string{"SENTINEL_CAREER_GOALS", "SENTINEL_COMPANY", "SENTINEL_ID", "SENTINEL_LOCATION", "SENTINEL_EMPLOYMENT", "SENTINEL_CURRENT_SALARY", "SENTINEL_DESIRED_SALARY", "SENTINEL_ADDITIONAL_INFO", "SENTINEL_PROJECT_ID", "SENTINEL_PROJECT_NAME", "SENTINEL_URL", `"recency"`, "ago", "current"} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("excluded profile field %q reached provider", forbidden)
		}
	}
	marker := "CANDIDATE QUALIFICATION PROFILE (JSON):\n"
	start := strings.Index(prompt, marker) + len(marker)
	end := strings.Index(prompt[start:], "\n\nFind at most")
	if start < len(marker) || end < 0 {
		t.Fatal("provider profile JSON missing or invalid")
	}
	var serializedFields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(prompt[start:start+end]), &serializedFields); err != nil {
		t.Fatalf("provider profile JSON invalid: %v", err)
	}
	var experienceFields []map[string]json.RawMessage
	if json.Unmarshal(serializedFields["experience"], &experienceFields) != nil {
		t.Fatal("provider profile serialization invalid")
	}
	if len(experienceFields) != 1 || string(experienceFields[0]["duration"]) != `"2 years"` {
		t.Fatalf("derived duration missing from provider profile: %s", serializedFields["experience"])
	}
	for _, field := range []string{"title", "description", "responsibilities", "achievements", "duration"} {
		if _, ok := experienceFields[0][field]; !ok {
			t.Errorf("approved experience field %q missing from provider profile", field)
		}
	}
	if len(experienceFields[0]) != 5 {
		t.Errorf("provider experience has unexpected fields: %v", experienceFields[0])
	}
	for _, field := range []string{"skills", "competencies", "tools", "experience", "projects"} {
		if _, ok := serializedFields[field]; !ok {
			t.Errorf("approved provider profile field %q missing", field)
		}
	}
	if len(serializedFields) != 5 {
		t.Errorf("provider profile has unexpected fields: %v", serializedFields)
	}
}

func TestHandlerKeepsFullBrowserContractAndReturnsGapsWithoutCaching(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(providerOK())), Header: make(http.Header)}, nil
	})}}
	body, _ := json.Marshal(sampleRequest())
	req := httptest.NewRequest("POST", "/api/qualification-gaps", strings.NewReader(string(body)))
	req.Header.Set("X-OpenAI-Api-Key", "sk-valid")
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, req)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || calls != 1 {
		t.Fatalf("status=%d cache=%q calls=%d body=%s", w.Code, w.Header().Get("Cache-Control"), calls, w.Body.String())
	}
	var result gapResult
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Gaps) != 1 || result.Gaps[0].Requirement != "Distributed systems" {
		t.Fatalf("bad result: %s", w.Body.String())
	}
}

func TestIncompleteFullProfileRejectedBeforeProvider(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, nil })}}
	request := httptest.NewRequest("POST", "/api/qualification-gaps", strings.NewReader(`{"jobPosting":"A sufficiently long posting","repository":{}}`))
	request.Header.Set("X-OpenAI-Api-Key", "sk-valid")
	response := httptest.NewRecorder()
	a.handler().ServeHTTP(response, request)
	if response.Code != 400 || calls != 0 || !strings.Contains(response.Body.String(), `"error":"input"`) {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
}

func TestInvalidProviderOutputPreservesErrorCategory(t *testing.T) {
	for _, content := range []string{"not json", `{"gaps":[{"kind":"skill","requirement":"x","details":"y"},{"kind":"skill","requirement":"x","details":"y"},{"kind":"skill","requirement":"x","details":"y"},{"kind":"skill","requirement":"x","details":"y"},{"kind":"skill","requirement":"x","details":"y"},{"kind":"skill","requirement":"x","details":"y"}]}`} {
		providerBody, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
		a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(providerBody))), Header: make(http.Header)}, nil
		})}}
		_, code, err := a.callProvider(t.Context(), "sk-valid", sampleRequest())
		if err == nil || code != "invalid_output" {
			t.Fatalf("code=%q err=%v", code, err)
		}
	}
}

func TestProviderFailureCategories(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   string
	}{{401, "key"}, {429, "rate_limit"}, {503, "outage"}} {
		a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
		})}}
		_, code, err := a.callProvider(t.Context(), "sk-valid", sampleRequest())
		if err == nil || code != tc.want {
			t.Errorf("status %d: code=%q err=%v", tc.status, code, err)
		}
	}
}

func TestTimeoutCategory(t *testing.T) {
	parent, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	a := app{client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })}}
	_, code, err := a.callProvider(parent, "sk-valid", sampleRequest())
	if err == nil || code != "timeout" {
		t.Fatalf("code=%q err=%v", code, err)
	}
}

func TestTimeoutWhileReadingProviderResponseBody(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	a := app{client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(blockingReader{done: r.Context().Done()}), Header: make(http.Header)}, nil
	})}}
	body, _ := json.Marshal(sampleRequest())
	req := httptest.NewRequest(http.MethodPost, "/api/qualification-gaps", strings.NewReader(string(body))).WithContext(ctx)
	req.Header.Set("X-OpenAI-Api-Key", "sk-valid")
	response := httptest.NewRecorder()
	a.handler().ServeHTTP(response, req)
	if response.Code != http.StatusGatewayTimeout || !strings.Contains(response.Body.String(), `"error":"timeout"`) {
		t.Fatalf("status=%d body=%s; want timeout response", response.Code, response.Body.String())
	}
}

func TestExperienceTimingParsesUserFacingEnglishAndPortugueseDates(t *testing.T) {
	now := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	for _, dates := range [][2]string{{"Jan 2021", "Dec 2023"}, {"jan. 2021", "dez. 2023"}, {"2021-01", "2023-12"}} {
		duration := experienceDuration(dates[0], dates[1], false, now)
		if duration != "2 years 11 months" {
			t.Errorf("experienceDuration(%q, %q) = %q", dates[0], dates[1], duration)
		}
	}
	duration := experienceDuration("jan. 2021", "", true, now)
	if duration != "5 years 9 months" {
		t.Errorf("current experience duration = %q", duration)
	}
	if duration := experienceDuration("Not a date", "Dec 2023", false, now); duration != "" {
		t.Errorf("unparseable dates produced duration %q", duration)
	}
	if duration := experienceDuration("Jan 2021", "Nov 2026", false, now); duration != "" {
		t.Errorf("future-ended experience produced duration %q", duration)
	}
}

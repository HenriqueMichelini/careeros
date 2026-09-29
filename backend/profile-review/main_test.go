package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func sampleRequest() reviewRequest {
	return reviewRequest{ChangedSection: "Skills", Repository: repository{CareerGoals: "Build useful software", CurrentSalary: "$100k", Experience: []experience{{ID: "e1", Company: "Example", Title: "Engineer"}}, Projects: []project{{ID: "p1", Name: "Project"}}}}
}

func validProviderResponse() string {
	return `{"choices":[{"message":{"content":"{\"updatedRepository\":{\"careerGoals\":\"Build better software\",\"skills\":\"Go\",\"competencies\":\"\",\"experience\":[{\"id\":\"e1\",\"company\":\"Example\",\"title\":\"Engineer\",\"startDate\":\"\",\"endDate\":\"\",\"current\":false,\"location\":\"\",\"description\":\"\",\"responsibilities\":\"\",\"achievements\":\"\"}],\"tools\":\"\",\"projects\":[{\"id\":\"p1\",\"name\":\"Project\",\"description\":\"\",\"technologies\":\"\",\"url\":\"\",\"highlights\":\"\"}],\"employmentStatus\":\"\",\"currentSalary\":\"$100k\",\"desiredSalary\":\"\",\"additionalInfo\":\"\"},\"summary\":\"Clarified the profile.\"}"}}]}`
}

func TestReviewValidOutputUsesSingleProviderCallAndPreservesProfileShape(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer sk-valid" {
			t.Fatal("provider key was not forwarded")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), model) || !strings.Contains(string(body), `"reasoning_effort":"none"`) || strings.Contains(string(body), "thinking") {
			t.Fatal("unexpected model settings")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(validProviderResponse())), Header: make(http.Header)}, nil
	})}}
	result, code, err := a.callProvider(t.Context(), "sk-valid", sampleRequest())
	if err != nil || code != "" {
		t.Fatalf("call failed: code=%s err=%v", code, err)
	}
	if calls != 1 || !validResult(result, sampleRequest().Repository) {
		t.Fatalf("calls=%d result invalid", calls)
	}
}

func TestSuccessfulHandlerResponseIsNotCached(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(validProviderResponse())), Header: make(http.Header)}, nil
	})}}
	body, err := json.Marshal(sampleRequest())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/profile/review", strings.NewReader(string(body)))
	request.Header.Set("X-OpenAI-Api-Key", "sk-valid")
	response := httptest.NewRecorder()
	a.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control=%q, want no-store", response.Header().Get("Cache-Control"))
	}
	var result reviewResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || !validResult(result, sampleRequest().Repository) {
		t.Fatalf("invalid review result: err=%v result=%+v", err, result)
	}
	if calls != 1 {
		t.Fatalf("provider calls=%d, want 1", calls)
	}
}

func TestInvalidInputIsRejectedBeforeProviderCall(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, nil })}}
	bad := sampleRequest()
	bad.Repository.CurrentSalary = strings.Repeat("x", maxProfileText+1)
	body, err := json.Marshal(bad)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/profile/review", strings.NewReader(string(body)))
	request.Header.Set("X-OpenAI-Api-Key", "sk-valid")
	response := httptest.NewRecorder()
	a.handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusBadRequest)
	}
	if calls != 0 {
		t.Fatalf("provider was called %d times for invalid input", calls)
	}
}

func TestNonOpenAIKeyIsRejectedBeforeProviderCall(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, nil
	})}}
	body, err := json.Marshal(sampleRequest())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/profile/review", strings.NewReader(string(body)))
	request.Header.Set("X-OpenAI-Api-Key", "sk-ant-not-an-openai-key")
	response := httptest.NewRecorder()
	a.handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"error":"key"`) {
		t.Fatalf("status=%d body=%s; want key error", response.Code, response.Body.String())
	}
	if calls != 0 {
		t.Fatalf("provider was called %d times with a non-OpenAI key", calls)
	}
}

func TestIncompleteOutputCannotReplaceProfile(t *testing.T) {
	original := sampleRequest().Repository
	updated := original
	updated.Experience = nil
	if validResult(reviewResult{UpdatedRepository: updated, Summary: "done"}, original) {
		t.Fatal("incomplete experience list was accepted")
	}
	updated = original
	updated.Projects = append([]project(nil), original.Projects...)
	updated.Projects[0].ID = "different"
	if validResult(reviewResult{UpdatedRepository: updated, Summary: "done"}, original) {
		t.Fatal("changed project identity was accepted")
	}
}

func TestHandlerRejectsMalformedAndIncompleteProviderOutput(t *testing.T) {
	validResult, err := json.Marshal(reviewResult{UpdatedRepository: sampleRequest().Repository, Summary: "Reviewed the profile."})
	if err != nil {
		t.Fatal(err)
	}
	var incomplete map[string]json.RawMessage
	if err := json.Unmarshal(validResult, &incomplete); err != nil {
		t.Fatal(err)
	}
	var repositoryFields map[string]json.RawMessage
	if err := json.Unmarshal(incomplete["updatedRepository"], &repositoryFields); err != nil {
		t.Fatal(err)
	}
	delete(repositoryFields, "desiredSalary")
	incompleteRepo, err := json.Marshal(repositoryFields)
	if err != nil {
		t.Fatal(err)
	}
	incomplete["updatedRepository"] = incompleteRepo
	incompleteOutput, err := json.Marshal(incomplete)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		content string
	}{{"malformed JSON", "not JSON"}, {"incomplete repository", string(incompleteOutput)}} {
		t.Run(tc.name, func(t *testing.T) {
			providerBody, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": tc.content}}}})
			if err != nil {
				t.Fatal(err)
			}
			a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(providerBody))), Header: make(http.Header)}, nil
			})}}
			body, err := json.Marshal(sampleRequest())
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/profile/review", strings.NewReader(string(body)))
			request.Header.Set("X-OpenAI-Api-Key", "sk-valid")
			response := httptest.NewRecorder()
			a.handler().ServeHTTP(response, request)
			if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), `"error":"invalid_output"`) || strings.Contains(response.Body.String(), `"updatedRepository"`) {
				t.Fatalf("status=%d body=%s; want invalid_output without a profile", response.Code, response.Body.String())
			}
		})
	}
}

func TestMalformedInputIsRejectedBeforeProviderCall(t *testing.T) {
	calls := 0
	a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, nil })}}
	request := httptest.NewRequest(http.MethodPost, "/api/profile/review", strings.NewReader(`{"repository":{}}`))
	request.Header.Set("X-OpenAI-Api-Key", "sk-valid")
	response := httptest.NewRecorder()
	a.handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusBadRequest)
	}
	if calls != 0 {
		t.Fatalf("provider was called %d times for malformed input", calls)
	}
}

func TestProviderFailureClassification(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   string
	}{{401, "key"}, {429, "rate_limit"}, {503, "outage"}} {
		a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
		})}}
		_, code, err := a.callProvider(t.Context(), "sk-valid", sampleRequest())
		if err == nil || code != tc.want {
			t.Fatalf("status %d: got %q, %v", tc.status, code, err)
		}
	}
}

func TestHandlerMapsProviderFailures(t *testing.T) {
	for _, tc := range []struct {
		providerStatus int
		wantStatus     int
		wantCode       string
	}{{401, http.StatusUnauthorized, "key"}, {429, http.StatusTooManyRequests, "rate_limit"}, {503, http.StatusBadGateway, "outage"}} {
		t.Run(tc.wantCode, func(t *testing.T) {
			a := app{client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.providerStatus, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
			})}}
			body, err := json.Marshal(sampleRequest())
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/profile/review", strings.NewReader(string(body)))
			request.Header.Set("X-OpenAI-Api-Key", "sk-valid")
			response := httptest.NewRecorder()
			a.handler().ServeHTTP(response, request)
			if response.Code != tc.wantStatus || !strings.Contains(response.Body.String(), `"error":"`+tc.wantCode+`"`) {
				t.Fatalf("status=%d body=%s; want status=%d and error=%q", response.Code, response.Body.String(), tc.wantStatus, tc.wantCode)
			}
		})
	}
}

func TestHandlerMapsSimulatedProviderTimeout(t *testing.T) {
	const simulatedDeadline = 25 * time.Millisecond
	a := app{client: &http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}}
	body, err := json.Marshal(sampleRequest())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), simulatedDeadline)
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/api/profile/review", strings.NewReader(string(body))).WithContext(ctx)
	request.Header.Set("X-OpenAI-Api-Key", "sk-valid")
	response := httptest.NewRecorder()
	started := time.Now()
	a.handler().ServeHTTP(response, request)
	measured := time.Since(started)
	t.Logf("simulated_provider_timeout_ms=%d", measured.Milliseconds())
	if response.Code != http.StatusGatewayTimeout || !strings.Contains(response.Body.String(), `"error":"timeout"`) {
		t.Fatalf("status=%d body=%s; want timeout response", response.Code, response.Body.String())
	}
	if measured < simulatedDeadline || measured > time.Second {
		t.Fatalf("simulated timeout took %s, want between %s and 1s", measured, simulatedDeadline)
	}
}

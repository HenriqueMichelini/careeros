package aidiagnostics_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	eval "professional-information-repo/internal/semanticeval"
	"strings"
	"testing"

	cv "professional-information-repo/backend/cv-generation"
	gaps "professional-information-repo/backend/qualification-gaps"
	d "professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/testsupport"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCompletedWorkflowReportsObservedUsageWithoutContent(t *testing.T) {
	var report d.Attempt
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"model":"gpt-6-luna-2026-01-01","usage":{"prompt_tokens":100,"completion_tokens":20,"completion_tokens_details":{"reasoning_tokens":5},"prompt_tokens_details":{"cached_tokens":40}},"choices":[{"finish_reason":"stop","message":{"content":"{\"summary\":[{\"sourceId\":\"s1\",\"text\":\"Uses Java.\"}],\"selected\":[\"s1\"],\"wording\":{}}"}}]}`))}, nil
	})}
	req := httptest.NewRequest("POST", "/api/cv/generate", bytes.NewBufferString(`{"cvLanguage":"en","density":"balanced","facts":[{"id":"s1","section":"skills","field":"skills","entryId":"","text":"PRIVATE_SOURCE_SENTINEL"}]}`))
	req.Header.Set("X-OpenAI-Api-Key", "sk-PRIVATE_KEY_SENTINEL")
	req = req.WithContext(d.WithSink(context.Background(), func(a d.Attempt) { report = a }))
	rec := httptest.NewRecorder()
	cv.NewHandlerWithClient(client).ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if report.Outcome != "completed" || report.ID == "" || len(report.Stages) != 2 {
		t.Fatalf("%+v", report)
	}
	stage := report.Stages[0]
	if stage.Provider == nil || stage.Provider.Model == nil || *stage.Provider.Model != "gpt-6-luna-2026-01-01" || *stage.Provider.InputTokens != 100 || *stage.Provider.OutputTokens != 20 || *stage.Provider.ReasoningTokens != 5 || *stage.Provider.CachedTokens != 40 || *stage.Provider.OutputCeiling != 4000 {
		t.Fatalf("%+v", stage)
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), "PRIVATE_") {
		t.Fatalf("content leaked: %s", raw)
	}
}

func TestIncompleteOutputsNeverBecomeCompletedResults(t *testing.T) {
	for _, tc := range []struct{ name, finish, refusal, usage, want string }{
		{"absent_usage", "stop", "", "", "completed"},
		{"partial_usage", "stop", "", `,"usage":{"prompt_tokens":12}`, "completed"},
		{"refusal", "stop", "declined", "", "refused"},
		{"parseable_truncation", "length", "", "", "truncated"},
		{"missing_finish", "", "", "", "incomplete_output"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var report d.Attempt
			client := &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
				raw := `{"choices":[{"finish_reason":"` + tc.finish + `","message":{"refusal":"` + tc.refusal + `","content":"{\"summary\":[{\"sourceId\":\"s1\",\"text\":\"Uses Java.\"}],\"selected\":[\"s1\"],\"wording\":{}}"}}]` + tc.usage + `}`
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw))}, nil
			})}
			req := httptest.NewRequest("POST", "/api/cv/generate", strings.NewReader(`{"cvLanguage":"en","facts":[{"id":"s1","section":"skills","field":"skills","entryId":"","text":"Java"}]}`))
			req.Header.Set("X-OpenAI-Api-Key", "sk-controlled")
			req = req.WithContext(d.WithSink(req.Context(), func(a d.Attempt) { report = a }))
			rec := httptest.NewRecorder()
			cv.NewHandlerWithClient(client).ServeHTTP(rec, req)
			wantStatus := 502
			if tc.want == "completed" {
				wantStatus = 200
			}
			if rec.Code != wantStatus || report.Outcome != tc.want || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%d %+v", rec.Code, report)
			}
			p := report.Stages[0].Provider
			if p.OutputTokens != nil || p.ReasoningTokens != nil || p.CachedTokens != nil {
				t.Fatal("invented usage")
			}
			if tc.name == "partial_usage" && (p.InputTokens == nil || *p.InputTokens != 12) {
				t.Fatal("partial usage lost")
			}
		})
	}
}

func TestIngestionSeparatesLocalPreparationExtractionAndReconciliation(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/cases.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []eval.Case `json:"cases"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("corpus")
	}
	var c eval.Case
	for _, item := range corpus.Cases {
		if item.ID == "en-fact" {
			c = item
		}
	}
	run, err := eval.Run(c, "controlled", "", "")
	if err != nil {
		t.Fatal(err)
	}
	a := run.Diagnostics
	if a == nil || a.Outcome != "completed" {
		t.Fatalf("%+v", a)
	}
	names := []string{}
	requests := 0
	for _, s := range a.Stages {
		names = append(names, s.Name)
		if s.Provider != nil {
			requests++
		}
	}
	if strings.Join(names, ",") != "mechanical_preparation,field_validation,extraction,reconciliation,candidate_retrieval" || requests != 3 {
		t.Fatalf("%v requests=%d", names, requests)
	}
	if a.Stages[0].Provider != nil || a.Stages[4].Provider != nil {
		t.Fatal("local stages imply provider consumption")
	}
}

func TestFailuresAndRejectedContentRemainDistinct(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
		err              error
	}{
		{name: "transport", want: "transport_failure", err: io.ErrUnexpectedEOF},
		{name: "provider", status: 503, body: `{}`, want: "provider_failure"},
		{name: "malformed", status: 200, body: `{"choices":`, want: "malformed_output"},
		{name: "rejected", status: 200, body: `{"choices":[{"finish_reason":"stop","message":{"content":"{\"summary\":[{\"sourceId\":\"unknown\",\"text\":\"Java\"}],\"selected\":[\"unknown\"]}"}}]}`, want: "rejected_output"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var a d.Attempt
			calls := 0
			client := &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
				calls++
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			req := httptest.NewRequest("POST", "/api/cv/generate", strings.NewReader(`{"cvLanguage":"en","facts":[{"id":"s1","section":"skills","entryId":"","field":"skills","text":"Java"}]}`))
			req.Header.Set("X-OpenAI-Api-Key", "sk-controlled")
			req = req.WithContext(d.WithSink(req.Context(), func(report d.Attempt) { a = report }))
			rec := httptest.NewRecorder()
			cv.NewHandlerWithClient(client).ServeHTTP(rec, req)
			if rec.Code != 502 || a.Outcome != tc.want || calls != 1 || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("calls=%d status=%d report=%+v", calls, rec.Code, a)
			}
		})
	}
}

func TestUntrustedModelAndInvalidUsageNeverLeakOrInventConsumption(t *testing.T) {
	var report d.Attempt
	client := &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"model":"PRIVATE_SECRET_MODEL","usage":{"prompt_tokens":"bad","completion_tokens":-2},"choices":[{"finish_reason":"stop","message":{"content":"{\"summary\":[{\"sourceId\":\"s1\",\"text\":\"Java\"}],\"selected\":[\"s1\"]}"}}]}`))}, nil
	})}
	req := httptest.NewRequest("POST", "/api/cv/generate", strings.NewReader(`{"cvLanguage":"en","facts":[{"id":"s1","section":"skills","entryId":"","field":"skills","text":"Java"}]}`))
	req.Header.Set("X-OpenAI-Api-Key", "sk-controlled")
	req = req.WithContext(d.WithSink(req.Context(), func(a d.Attempt) { report = a }))
	rec := httptest.NewRecorder()
	cv.NewHandlerWithClient(client).ServeHTTP(rec, req)
	if rec.Code != 200 || report.Stages[0].Provider.Model != nil || report.Stages[0].Provider.InputTokens != nil || report.Stages[0].Provider.OutputTokens != nil {
		t.Fatalf("%d %+v", rec.Code, report)
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), "PRIVATE_") {
		t.Fatal("untrusted model leaked")
	}
}

func TestFieldRejectionIsNotCompletionAndRetainsJevUsage(t *testing.T) {
	var report d.Attempt
	calls := 0
	client := &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		calls++
		response := testsupport.AcceptedJob()
		raw, _ := io.ReadAll(response.Body)
		response.Body.Close()
		text := strings.ReplaceAll(string(raw), `"choice":"job_with_context"`, `"choice":"irrelevant"`)
		text = strings.ReplaceAll(text, `"job_with_context":1`, `"job_with_context":0`)
		text = strings.ReplaceAll(text, `"irrelevant":0`, `"irrelevant":1`)
		text = strings.Replace(text, `"model":"jev-1.13.0"`, `"model":"jev-1.13.0","usage":{"input_tokens":635,"output_tokens":97}`, 1)
		response.Body = io.NopCloser(strings.NewReader(text))
		return response, nil
	})}
	req := httptest.NewRequest("POST", "/api/qualification-gaps", strings.NewReader(`{"repository":{"careerGoals":"","skills":"Java","competencies":"","experience":[],"tools":"","projects":[],"employmentStatus":"","currentSalary":"","desiredSalary":"","additionalInfo":""},"jobPosting":"Weather today."}`))
	req.Header.Set("X-OpenAI-Api-Key", "sk-controlled")
	req.Header.Set("X-TypeSafe-Api-Key", "controlled")
	req = req.WithContext(d.WithSink(req.Context(), func(a d.Attempt) { report = a }))
	rec := httptest.NewRecorder()
	gaps.NewHandlerWithClient(client).ServeHTTP(rec, req)
	if rec.Code != 200 || report.Outcome != "irrelevant" || calls != 1 || len(report.Stages) != 1 {
		t.Fatalf("calls=%d status=%d report=%+v", calls, rec.Code, report)
	}
	provider := report.Stages[0].Provider
	if provider == nil || provider.InputTokens == nil || *provider.InputTokens != 635 || provider.OutputTokens == nil || *provider.OutputTokens != 97 || provider.CachedTokens != nil {
		t.Fatalf("%+v", provider)
	}
}

package profilereview

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSectionRewriteSemanticFixtures(t *testing.T) {
	raw, err := os.ReadFile("testdata/section-rewrite.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			ID             string          `json:"id"`
			Request        json.RawMessage `json:"request"`
			Proposal       json.RawMessage `json:"proposal"`
			Verification   json.RawMessage `json:"verification"`
			Finish         string          `json:"finish"`
			Refusal        string          `json:"refusal"`
			Status         int             `json:"status"`
			ProviderStatus int             `json:"providerStatus"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("invalid corpus")
	}
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				body, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(body), `"strict":true`) {
					t.Fatal("missing strict provider schema")
				}
				content := c.Proposal
				if calls > 0 {
					content = c.Verification
				}
				calls++
				encoded, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": c.Finish, "message": map[string]string{"content": string(content), "refusal": c.Refusal}}}})
				return &http.Response{StatusCode: c.ProviderStatus, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(encoded)))}, nil
			})}
			req := httptest.NewRequest("POST", "/api/profile/review", strings.NewReader(string(c.Request)))
			req.Header.Set("X-OpenAI-Api-Key", "sk-test")
			rec := httptest.NewRecorder()
			NewHandlerWithClient(client).ServeHTTP(rec, req)
			if rec.Code != c.Status {
				t.Fatalf("%d %s; want %d", rec.Code, rec.Body.String(), c.Status)
			}
			if rec.Header().Get("Cache-Control") != "no-store" || calls > 2 {
				t.Fatal("missing no-store or unbounded calls")
			}
			if c.Status != 200 && strings.Contains(rec.Body.String(), "patches") {
				t.Fatal("invalid proposal leaked")
			}
			if c.ID == "truncated" && !strings.Contains(rec.Body.String(), "truncated") {
				t.Fatal("truncation not explicit")
			}
			if c.ID == "refused" && !strings.Contains(rec.Body.String(), "refused") {
				t.Fatal("refusal not explicit")
			}
		})
	}
}
func TestMalformedAndLegacyRequestsNeverReachProvider(t *testing.T) {
	for _, raw := range []string{`{}`, `{"repository":{},"changedSection":"skills"}`, `{"document":{}}`, `not JSON`} {
		client := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("provider called"); return nil, nil })}
		req := httptest.NewRequest("POST", "/api/profile/review", strings.NewReader(raw))
		req.Header.Set("X-OpenAI-Api-Key", "sk-test")
		rec := httptest.NewRecorder()
		NewHandlerWithClient(client).ServeHTTP(rec, req)
		if rec.Code != 400 {
			t.Fatalf("%d", rec.Code)
		}
	}
}

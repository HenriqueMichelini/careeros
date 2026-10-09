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

func TestCanonicalSectionReviewIsBoundedAndDoesNotReturnASavedProfile(t *testing.T) {
	raw := `{"document":{"version":2,"id":"p","revision":1,"normalizationPolicy":"exact-alias-v1","entities":[],"facts":[{"id":"f","revision":1,"owner":{"profileId":"p","id":"p","revision":1},"context":[],"field":"skills","order":0,"kind":"legacy_block","value":"I use Go","assertion":"unknown","intent":"unknown","certainty":"unknown","temporal":{"wording":"","precision":"unknown"},"normalization":{"observed":"I use Go","canonical":null,"policy":"exact-alias-v1"},"origin":{"kind":"existing_profile","original":"unknown"},"approval":"unreviewed","support":"unsupported"}],"evidence":[],"links":[]},"section":"skills","locale":"en"}`
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `\"strict\":true`) && !strings.Contains(string(body), `"strict":true`) {
			t.Fatal("provider schema is not strict")
		}
		content := `{"profileId":"p","revision":1,"section":"skills","summary":"Clarified wording","patches":[{"factId":"f","revision":1,"wording":"I work with Go","supporting":[{"id":"f","revision":1}]}]}`
		if calls > 0 {
			content = `{"checks":[{"factId":"f","supported":true,"complete":true}]}`
		}
		calls++
		encoded, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": content}}}})
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(encoded)))}, nil
	})}
	request := httptest.NewRequest("POST", "/api/profile/review", strings.NewReader(raw))
	request.Header.Set("X-OpenAI-Api-Key", "sk-test")
	response := httptest.NewRecorder()
	NewHandlerWithClient(client).ServeHTTP(response, request)
	if response.Code != 200 || strings.Contains(response.Body.String(), "updatedRepository") || !strings.Contains(response.Body.String(), "patches") {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	if calls != 2 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing bounded verifier/no-store")
	}
}
func TestProviderNeverReceivesUnrelatedCanonicalProfileFacts(t *testing.T) {
	raw, _ := os.ReadFile("testdata/section-rewrite.v1.json")
	var corpus struct {
		Cases []struct {
			Request      reviewRequest   `json:"request"`
			Proposal     json.RawMessage `json:"proposal"`
			Verification json.RawMessage `json:"verification"`
		} `json:"cases"`
	}
	json.Unmarshal(raw, &corpus)
	c := corpus.Cases[0]
	private := c.Request.Document.Facts[0]
	private.ID = "private"
	private.Field = "email"
	private.Order = 1
	private.Value = json.RawMessage(`"private-sentinel@example.test"`)
	private.Normalization.Observed = "private-sentinel@example.test"
	c.Request.Document.Facts = append(c.Request.Document.Facts, private)
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "private-sentinel") {
			t.Fatal("unrelated Profile fact reached provider")
		}
		content := c.Proposal
		if calls > 0 {
			content = c.Verification
		}
		calls++
		encoded, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(content)}}}})
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(encoded)))}, nil
	})}
	requestRaw, _ := json.Marshal(c.Request)
	req := httptest.NewRequest("POST", "/api/profile/review", strings.NewReader(string(requestRaw)))
	req.Header.Set("X-OpenAI-Api-Key", "sk-test")
	rec := httptest.NewRecorder()
	NewHandlerWithClient(client).ServeHTTP(rec, req)
	if rec.Code != 200 || calls != 2 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

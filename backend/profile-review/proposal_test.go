package profilereview

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

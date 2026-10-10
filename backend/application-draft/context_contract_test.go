package applicationdraft_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	applicationdraft "professional-information-repo/backend/application-draft"
	"strings"
	"testing"
)

func TestDraftProjectsPrivateFieldsOutOfProviderContext(t *testing.T) {
	input := strings.Replace(syntheticDraftInput, `"currentSalary":""`, `"currentSalary":"SECRET_SALARY"`, 1)
	input = strings.Replace(input, `"additionalInfo":""`, `"additionalInfo":"SECRET_PRIVATE"`, 1)
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		var payload struct {
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Messages) != 2 || payload.Messages[0].Role != "system" || payload.Messages[1].Role != "user" {
			t.Error("instructions must be separate from data")
		}
		raw, _ := json.Marshal(payload)
		for _, private := range []string{"SECRET_SALARY", "SECRET_PRIVATE", "employmentStatus", "currentSalary"} {
			if strings.Contains(string(raw), private) {
				t.Errorf("disclosed %s", private)
			}
		}
		if !strings.Contains(string(raw), "Java") {
			t.Error("lost relevant skill")
		}
		return providerDraft("I use Java."), nil
	})}
	req := draftRequest()
	req.Body = http.NoBody
	req = httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(input))
	req.Header.Set("X-OpenAI-Api-Key", "sk-synthetic")
	req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"contextSelection"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestDraftRetainsCanonicalNegativeEvidenceAndContext(t *testing.T) {
	raw, err := os.ReadFile("../../internal/profiledocument/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct{ Document json.RawMessage }
	json.Unmarshal(raw, &fixtures)
	var input map[string]any
	json.Unmarshal([]byte(syntheticDraftInput), &input)
	input["profileEvidence"] = fixtures[0].Document
	data, _ := json.Marshal(input)
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		for _, value := range []string{"aspiration", "uncertain", "role", "skill"} {
			if !strings.Contains(string(body), value) {
				t.Errorf("missing %s", value)
			}
		}
		return providerDraft("I use Java."), nil
	})}
	req := httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(data)))
	req.Header.Set("X-OpenAI-Api-Key", "sk-test")
	req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestDraftBudgetDisclosesOmittedWholeUnits(t *testing.T) {
	var input map[string]any
	json.Unmarshal([]byte(syntheticDraftInput), &input)
	repo := input["repository"].(map[string]any)
	repo["skills"] = strings.Repeat("Java ", 2000)
	repo["competencies"] = strings.Repeat("AWS ", 2500)
	repo["tools"] = strings.Repeat("Docker ", 1400)
	data, _ := json.Marshal(input)
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) { return providerDraft("I use Java."), nil })}
	req := httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(data)))
	req.Header.Set("X-OpenAI-Api-Key", "sk-test")
	req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"complete":false`) || !strings.Contains(w.Body.String(), `"budgetExcluded":1`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestDraftRejectsPrivateCanonicalEvidenceBeforeProvider(t *testing.T) {
	raw, _ := os.ReadFile("../../internal/profiledocument/fixtures.json")
	var fixtures []struct{ Document map[string]any }
	json.Unmarshal(raw, &fixtures)
	doc := fixtures[0].Document
	doc["facts"].([]any)[0].(map[string]any)["field"] = "currentSalary"
	var input map[string]any
	json.Unmarshal([]byte(syntheticDraftInput), &input)
	input["profileEvidence"] = doc
	data, _ := json.Marshal(input)
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		t.Error("private projection reached provider")
		return providerDraft("I use Java."), nil
	})}
	req := httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(data)))
	req.Header.Set("X-OpenAI-Api-Key", "sk-test")
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
	if w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestDraftRetainsAssessedParaphraseWithoutLexicalOverlap(t *testing.T) {
	raw, _ := os.ReadFile("../../docs/evaluations/qualification-evidence/cases.json")
	var corpus struct {
		Cases []struct {
			ID       string
			Document json.RawMessage
		}
	}
	json.Unmarshal(raw, &corpus)
	var input map[string]any
	json.Unmarshal([]byte(syntheticDraftInput), &input)
	for _, c := range corpus.Cases {
		if c.ID == "project-not-employment" {
			input["profileEvidence"] = c.Document
		}
	}
	input["jobPosting"] = "Ruby development required."
	input["selectedFactIds"] = []string{"p-java"}
	data, _ := json.Marshal(input)
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "p-java") || !strings.Contains(string(body), "p-name") {
			t.Error("assessed evidence and project context lost")
		}
		return providerDraft("I use Java."), nil
	})}
	req := httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(data)))
	req.Header.Set("X-OpenAI-Api-Key", "sk-test")
	req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

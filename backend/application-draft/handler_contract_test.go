package applicationdraft_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"professional-information-repo/internal/testsupport"
	"reflect"
	"strings"
	"testing"

	applicationdraft "professional-information-repo/backend/application-draft"
)

type providerTransport func(*http.Request) (*http.Response, error)

func (f providerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "api.typesafe.ai" {
		return testsupport.AcceptedJob(), nil
	}
	return f(r)
}

const syntheticDraftInput = `{"repository":{"careerGoals":"","skills":"Java","competencies":"","experience":[],"tools":"","projects":[],"employmentStatus":"looking","currentSalary":"","desiredSalary":"","additionalInfo":""},"jobPosting":"Example Labs hires a Java Developer to build Java APIs. Java and AWS required.","confirmedQualifications":[],"cvLanguage":"en"}`

// The literal describes the agreed provider contract, independently of its builder.
const strictDraftFormat = `{"type":"json_schema","json_schema":{"name":"application_draft","strict":true,"schema":{"type":"object","properties":{"jobTitle":{"type":["string","null"]},"company":{"type":["string","null"]},"jobSummary":{"type":"string"},"resume":{"type":"string"},"applicationAnswers":{"type":"string"},"coverLetter":{"type":"object","properties":{"greeting":{"type":"string"},"body":{"type":"string"},"closing":{"type":"string","enum":["Sincerely,","Kind regards,","Best regards,","Atenciosamente,","Cordialmente,"]}},"required":["greeting","body","closing"],"additionalProperties":false}},"required":["jobTitle","company","jobSummary","resume","applicationAnswers","coverLetter"],"additionalProperties":false}}}`

func providerDraft(answers any) *http.Response {
	content, _ := json.Marshal(map[string]any{
		"jobTitle": "Java Developer", "company": "Example Labs", "jobSummary": "Build Java APIs.",
		"resume": "## Technical Skills\n- Java", "applicationAnswers": answers,
		"coverLetter": map[string]string{"greeting": "Dear hiring team,", "body": "I use Java.", "closing": "Sincerely,"},
	})
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(content)}}}})
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}
}

func draftRequest() *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/application-draft", strings.NewReader(syntheticDraftInput))
	r.Header.Set("X-OpenAI-Api-Key", "sk-synthetic-test-key")
	r.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	return r
}

func TestApplicationDraftUsesStringAnswerContract(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	calls := 0
	http.DefaultTransport = providerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var payload struct {
			ResponseFormat any `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		var expected any
		if err := json.Unmarshal([]byte(strictDraftFormat), &expected); err != nil {
			t.Fatal(err)
		}
		// Without a strict contract the provider can emit the observed array shape.
		if !reflect.DeepEqual(payload.ResponseFormat, expected) {
			return providerDraft([]string{"I use Java."}), nil
		}
		return providerDraft("### Java skills\nI use Java."), nil
	})
	w := httptest.NewRecorder()
	applicationdraft.NewHandler().ServeHTTP(w, draftRequest())
	var response struct {
		Answers string `json:"applicationAnswers"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Answers != "### Java skills\nI use Java." {
		t.Fatalf("draft did not preserve the string answer contract: status=%d", w.Code)
	}
	if calls != 1 {
		t.Fatalf("expected one deliberate provider request, got %d", calls)
	}
}

func TestApplicationDraftRejectsArrayAnswersWithoutRetry(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	calls := 0
	http.DefaultTransport = providerTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return providerDraft([]string{"I use Java."}), nil
	})
	w := httptest.NewRecorder()
	applicationdraft.NewHandler().ServeHTTP(w, draftRequest())
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), `"error":"invalid_output"`) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("malformed provider answers must stop the draft: status=%d", w.Code)
	}
	if calls != 1 {
		t.Fatalf("invalid output must not trigger another billable request: got %d calls", calls)
	}
}

func TestShortDraftPreservesUnknownMetadata(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, metadata := range []string{`"jobTitle":"Java developer","company":null`, `"jobTitle":null,"company":null`} {
		t.Run(metadata, func(t *testing.T) {
			http.DefaultTransport = providerTransport(func(r *http.Request) (*http.Response, error) {
				content := `{` + metadata + `,"jobSummary":"AWS required.","resume":"## Technical Skills\n- Java","applicationAnswers":"I use Java.","coverLetter":{"greeting":"Dear hiring team,","body":"I use Java.","closing":"Sincerely,"}}`
				raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": content}}}})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			})
			r := draftRequest()
			r.Body = io.NopCloser(strings.NewReader(strings.Replace(syntheticDraftInput, "Example Labs hires a Java Developer to build Java APIs. Java and AWS required.", "Java developer. AWS required.", 1)))
			w := httptest.NewRecorder()
			applicationdraft.NewHandler().ServeHTTP(w, r)
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"company":null`) || !strings.Contains(w.Body.String(), "AWS required.") {
				t.Fatalf("unknown metadata must survive drafting: status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestDraftRejectsMalformedUnknownMetadata(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, metadata := range []string{`"jobTitle":"Java developer"`, `"jobTitle":"Java developer","company":" "`, `"jobTitle":"","company":null`, `"jobTitle":false,"company":null`} {
		t.Run(metadata, func(t *testing.T) {
			calls := 0
			http.DefaultTransport = providerTransport(func(*http.Request) (*http.Response, error) {
				calls++
				content := `{` + metadata + `,"jobSummary":"AWS required.","resume":"Java","applicationAnswers":"I use Java.","coverLetter":{"greeting":"Dear team,","body":"I use Java.","closing":"Sincerely,"}}`
				raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": content}}}})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			})
			w := httptest.NewRecorder()
			applicationdraft.NewHandler().ServeHTTP(w, draftRequest())
			if w.Code != 502 || calls != 1 || !strings.Contains(w.Body.String(), `"error":"invalid_output"`) {
				t.Fatalf("malformed metadata accepted: %d", w.Code)
			}
		})
	}
}

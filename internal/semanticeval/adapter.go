package semanticeval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	draft "professional-information-repo/backend/application-draft"
	cv "professional-information-repo/backend/cv-generation"
	ingestion "professional-information-repo/backend/profile-ingestion"
	gaps "professional-information-repo/backend/qualification-gaps"
	"professional-information-repo/internal/fieldvalidation"
)

func Hash(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }

type Call struct {
	Provider            string `json:"provider"`
	Model               string `json:"model"`
	Reasoning           any    `json:"reasoning"`
	MaxCompletionTokens any    `json:"maxCompletionTokens,omitempty"`
	PromptHash          string `json:"promptHash"`
	SchemaHash          string `json:"schemaHash"`
	RequestHash         string `json:"requestHash"`
	Status              int    `json:"status"`
	ReturnedModel       string `json:"returnedModel,omitempty"`
}
type RunResult struct {
	ID              string            `json:"id"`
	Task            string            `json:"task"`
	Split           string            `json:"split"`
	Language        string            `json:"language"`
	Mode            string            `json:"mode"`
	Role            string            `json:"role"`
	Status          int               `json:"status"`
	NoStore         bool              `json:"noStore"`
	Calls           []Call            `json:"calls"`
	Output          json.RawMessage   `json:"output"`
	ProviderOutputs []json.RawMessage `json:"providerOutputs,omitempty"`
	Score           Result            `json:"score"`
}
type transport struct {
	mode      string
	responses []json.RawMessage
	calls     []Call
	index     int
	outputs   []json.RawMessage
}

const openAIHost = "api.openai.com"
const typeSafeHost = "api.typesafe.ai"

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	if host != openAIHost && host != typeSafeHost {
		return nil, fmt.Errorf("unexpected provider host")
	}
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(raw))
	var body map[string]any
	if err = json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	t.calls = append(t.calls, providerCall(host, raw, body))
	if t.mode == "live" {
		return t.liveResponse(req, host)
	}
	return t.controlledResponse(host, body)
}
func providerCall(host string, raw []byte, body map[string]any) Call {
	digest := func(value any) string { encoded, _ := json.Marshal(value); return Hash(encoded) }
	model, _ := body["model"].(string)
	call := Call{Provider: host, Model: model, Reasoning: body["reasoning_effort"], MaxCompletionTokens: body["max_completion_tokens"], PromptHash: digest(body["messages"]), SchemaHash: digest(body["response_format"]), RequestHash: Hash(raw)}
	if host == typeSafeHost {
		call.PromptHash = digest(body["questions"])
		call.SchemaHash = call.PromptHash
	}
	return call
}
func (t *transport) liveResponse(req *http.Request, host string) (*http.Response, error) {
	response, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	t.calls[len(t.calls)-1].Status = response.StatusCode
	if response.StatusCode == http.StatusOK && host == openAIHost {
		if err = t.captureCompletion(response); err != nil {
			return nil, err
		}
	}
	return response, nil
}
func (t *transport) captureCompletion(response *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	response.Body.Close()
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	var envelope struct {
		Model   string          `json:"model"`
		Choices json.RawMessage `json:"choices"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return nil
	}
	t.calls[len(t.calls)-1].ReturnedModel = envelope.Model
	if len(envelope.Choices) > 0 {
		t.outputs = append(t.outputs, envelope.Choices)
	}
	return nil
}
func (t *transport) controlledResponse(host string, body map[string]any) (*http.Response, error) {
	var response any
	if host == typeSafeHost {
		response = controlledDecision(body)
	} else {
		if t.index >= len(t.responses) {
			return nil, fmt.Errorf("controlled response sequence exhausted")
		}
		response = map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(t.responses[t.index])}}}}
		t.index++
	}
	encoded, _ := json.Marshal(response)
	t.calls[len(t.calls)-1].Status = http.StatusOK
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(encoded))}, nil
}
func controlledDecision(body map[string]any) any {
	questions := body["questions"].(map[string]any)
	state := body["state"].(map[string]any)
	content := "professional_fact"
	if state["field"] == "job_posting" {
		content = "job_with_context"
	}
	answers := map[string]any{}
	for key, value := range questions {
		chosen := "none"
		if key == "content" {
			chosen = content
		}
		probabilities := map[string]float64{}
		for option := range value.(map[string]any)["criteria"].(map[string]any) {
			probabilities[option] = 0
		}
		probabilities[chosen] = 1
		answers[key] = map[string]any{"type": "choice", "choice": chosen, "confidence": 1, "probabilities": probabilities}
	}
	return map[string]any{"model": fieldvalidation.Model, "answers": answers}
}

// Run invokes the same public HTTP contracts used by the application. Controlled
// responses exercise parsing, grounding and filtering; they measure no model.
func Run(c Case, mode, openAIKey, typeSafeKey string) (RunResult, error) {
	result := RunResult{ID: c.ID, Task: c.Task, Split: c.Split, Language: c.Language, Mode: mode, Role: c.Role}
	if mode != "controlled" && mode != "live" {
		return result, fmt.Errorf("unknown evidence mode")
	}
	if mode == "live" && (openAIKey == "" || (c.Task != "cv_generation" && typeSafeKey == "")) {
		return result, fmt.Errorf("live provider credentials required")
	}
	t := &transport{mode: mode, responses: c.Provider}
	client := &http.Client{Transport: t, Timeout: 55 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var handler http.Handler
	var path string
	switch c.Task {
	case "profile_ingestion":
		handler = ingestion.NewHandlerWithClient(client)
		path = "/api/profile/ingest"
	case "qualification_gaps":
		handler = gaps.NewHandlerWithClient(client)
		path = "/api/qualification-gaps"
	case "cv_generation":
		handler = cv.NewHandlerWithClient(client)
		path = "/api/cv/generate"
	case "application_draft":
		handler = draft.NewHandlerWithClient(client)
		path = "/api/application-draft"
	default:
		return result, fmt.Errorf("unknown task %q", c.Task)
	}
	if mode == "controlled" {
		openAIKey = "sk-controlled"
		typeSafeKey = "controlled"
	}
	req := httptest.NewRequest("POST", path, bytes.NewReader(c.Request))
	req.Header.Set("X-OpenAI-Api-Key", openAIKey)
	req.Header.Set("X-TypeSafe-Api-Key", typeSafeKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	result.Status = rec.Code
	result.NoStore = rec.Header().Get("Cache-Control") == "no-store"
	result.Calls = t.calls
	result.ProviderOutputs = t.outputs
	result.Output = append(json.RawMessage{}, rec.Body.Bytes()...)
	return Assess(c, result)
}

// Assess scores a saved production response without invoking any provider. The
// original calls/output remain evidence; only derived scores are recalculated.
func Assess(c Case, result RunResult) (RunResult, error) {
	score, err := Score(c, result.Output)
	if err != nil {
		return result, err
	}
	result.Score = score
	if result.Status != 200 {
		result.Score.Failures = append(result.Score.Failures, fmt.Sprintf("HTTP %d", result.Status))
	}
	var output map[string]any
	json.Unmarshal(result.Output, &output)
	if d, ok := output["decision"].(map[string]any); ok {
		if o, ok := d["outcome"].(map[string]any); ok && o["kind"] != "accept" {
			result.Score.Failures = append(result.Score.Failures, "field decision did not accept")
		}
	}
	if !result.NoStore {
		result.Score.Failures = append(result.Score.Failures, "missing no-store")
	}
	return result, nil
}

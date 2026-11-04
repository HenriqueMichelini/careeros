package semanticeval

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"professional-information-repo/internal/aidiagnostics"
	"time"

	draft "professional-information-repo/backend/application-draft"
	cv "professional-information-repo/backend/cv-generation"
	ingestion "professional-information-repo/backend/profile-ingestion"
	review "professional-information-repo/backend/profile-review"
	gaps "professional-information-repo/backend/qualification-gaps"
	"professional-information-repo/internal/fieldvalidation"
)

func Hash(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }

type Call struct {
	DispatchID            string  `json:"dispatchId,omitempty"`
	InstructionPrefixHash string  `json:"first256InstructionCharactersHash"`
	Provider              string  `json:"provider"`
	Model                 string  `json:"model"`
	Reasoning             any     `json:"reasoning"`
	MaxCompletionTokens   any     `json:"maxCompletionTokens,omitempty"`
	PromptHash            string  `json:"promptHash"`
	SchemaHash            string  `json:"schemaHash"`
	RequestHash           string  `json:"requestHash"`
	Status                int     `json:"status"`
	ReturnedModel         string  `json:"returnedModel,omitempty"`
	DurationMS            float64 `json:"durationMs"`
	InputTokens           *int64  `json:"inputTokens"`
	OutputTokens          *int64  `json:"outputTokens"`
	ReasoningTokens       *int64  `json:"reasoningTokensSubsetOfOutput"`
	CachedTokens          *int64  `json:"cachedTokensSubsetOfInput"`
	CacheWriteTokens      *int64  `json:"cacheWriteTokensSubsetOfInput"`
}
type RunResult struct {
	LabelFacetScores map[string]Result      `json:"labelFacetScores,omitempty"`
	RequestEnvelopes []json.RawMessage      `json:"controlledRequestEnvelopes,omitempty"`
	RequiredSteps    int                    `json:"requiredSteps,omitempty"`
	Steps            []RunResult            `json:"steps,omitempty"`
	Diagnostics      *aidiagnostics.Attempt `json:"diagnostics,omitempty"`
	ID               string                 `json:"id"`
	Task             string                 `json:"task"`
	Split            string                 `json:"split"`
	Language         string                 `json:"language"`
	Mode             string                 `json:"mode"`
	Role             string                 `json:"role"`
	Status           int                    `json:"status"`
	NoStore          bool                   `json:"noStore"`
	Calls            []Call                 `json:"calls"`
	Output           json.RawMessage        `json:"output"`
	ProviderOutputs  []json.RawMessage      `json:"providerOutputs,omitempty"`
	Score            Result                 `json:"score"`
}
type transport struct {
	captureRequests bool
	statuses        []int
	requests        []json.RawMessage
	failure         error
	mode            string
	responses       []json.RawMessage
	calls           []Call
	index           int
	outputs         []json.RawMessage
	config          *Configuration
	budget          *Budget
	envelopes       []json.RawMessage
}

const openAIHost = "api.openai.com"
const typeSafeHost = "api.typesafe.ai"

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	if host != openAIHost && host != typeSafeHost {
		return nil, fmt.Errorf("unexpected provider host")
	}
	if err := req.Context().Err(); err != nil {
		t.failure = err
		return nil, err
	}
	policy := executionPolicy(req.Context())
	if policy.BeforeDispatch != nil {
		if err := policy.BeforeDispatch(req.Context()); err != nil {
			t.failure = err
			return nil, err
		}
	}
	if err := req.Context().Err(); err != nil {
		t.failure = err
		return nil, err
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
	if t.budget != nil {
		if t.budget.Used >= t.budget.Limit {
			return nil, fmt.Errorf("provider call budget exhausted")
		}
		t.budget.Used++
	}
	if t.config != nil && host == openAIHost {
		body["model"] = t.config.Model
		body["reasoning_effort"] = t.config.Reasoning
		raw, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
		req = req.Clone(req.Context())
		req.Body = io.NopCloser(bytes.NewReader(raw))
		req.ContentLength = int64(len(raw))
	}
	if t.captureRequests {
		t.requests = append(t.requests, bytes.Clone(raw))
	}
	t.calls = append(t.calls, providerCall(host, raw, body))
	observeRequest(req.Context(), host, len(raw))
	if policy.NewDispatchID != nil {
		t.calls[len(t.calls)-1].DispatchID = policy.NewDispatchID()
	}
	if policy.ObserveDispatch != nil {
		policy.ObserveDispatch(DispatchObservation{Phase: DispatchStarted, Call: t.calls[len(t.calls)-1]})
	}
	started := time.Now()
	var response *http.Response
	if t.mode == "live" {
		response, err = http.DefaultTransport.RoundTrip(req)
	} else {
		response, err = t.controlledResponse(host, body)
		if err == nil && policy.ControlledBody != nil {
			response.Body = policy.ControlledBody(req.Context(), response.Body)
		}
	}
	if err == nil {
		err = t.captureResponse(req.Context(), response, host)
	}
	t.calls[len(t.calls)-1].DurationMS = float64(time.Since(started).Microseconds()) / 1000
	if policy.ObserveDispatch != nil {
		policy.ObserveDispatch(DispatchObservation{Phase: DispatchFinished, Call: t.calls[len(t.calls)-1], Err: err})
	}
	return response, err
}
func providerCall(host string, raw []byte, body map[string]any) Call {
	digest := func(value any) string { encoded, _ := json.Marshal(value); return Hash(encoded) }
	model, _ := body["model"].(string)
	call := Call{Provider: host, Model: model, Reasoning: body["reasoning_effort"], MaxCompletionTokens: body["max_completion_tokens"], PromptHash: digest(body["messages"]), SchemaHash: digest(body["response_format"]), RequestHash: Hash(raw)}
	if messages, ok := body["messages"].([]any); ok && len(messages) > 0 {
		if message, ok := messages[0].(map[string]any); ok {
			if content, ok := message["content"].(string); ok {
				prefix := []rune(content)
				call.InstructionPrefixHash = Hash([]byte(string(prefix[:min(256, len(prefix))])))
			}
		}
	}
	if host == typeSafeHost {
		call.PromptHash = digest(body["questions"])
		call.SchemaHash = call.PromptHash
	}
	return call
}
func (t *transport) captureResponse(ctx context.Context, response *http.Response, host string) error {
	t.calls[len(t.calls)-1].Status = response.StatusCode
	originalBody := response.Body
	stop := context.AfterFunc(ctx, func() { originalBody.Close() })
	body, err := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, reader: originalBody}, (1<<20)+1))
	stop()
	originalBody.Close()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	var envelope struct {
		Model   string          `json:"model"`
		Usage   json.RawMessage `json:"usage"`
		Choices json.RawMessage `json:"choices"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return nil
	}
	call := &t.calls[len(t.calls)-1]
	call.ReturnedModel = safeEvaluationModel(envelope.Model)
	inputKey, outputKey := "prompt_tokens", "completion_tokens"
	if host == typeSafeHost {
		inputKey, outputKey = "input_tokens", "output_tokens"
	}
	call.InputTokens, call.OutputTokens, call.ReasoningTokens, call.CachedTokens = aidiagnostics.Usage(envelope.Usage, inputKey, outputKey)
	var usage struct {
		Prompt struct {
			Writes *int64 `json:"cache_write_tokens"`
		} `json:"prompt_tokens_details"`
	}
	if json.Unmarshal(envelope.Usage, &usage) == nil && usage.Prompt.Writes != nil && *usage.Prompt.Writes >= 0 {
		call.CacheWriteTokens = usage.Prompt.Writes
	}
	if len(envelope.Choices) > 0 && (t.mode == "live" || t.captureRequests) {
		t.outputs = append(t.outputs, envelope.Choices)
	}
	return nil
}
func (t *transport) controlledResponse(host string, body map[string]any) (*http.Response, error) {
	var response any
	status := http.StatusOK
	if host == typeSafeHost {
		response = controlledDecision(body)
	} else {
		if t.index >= len(t.responses) {
			return nil, fmt.Errorf("controlled response sequence exhausted")
		}
		envelope := map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(t.responses[t.index])}}}}
		if t.index < len(t.envelopes) {
			_ = json.Unmarshal(t.envelopes[t.index], &envelope)
		}
		response = envelope
		if t.index < len(t.statuses) {
			status = t.statuses[t.index]
		}
		t.index++
	}
	encoded, _ := json.Marshal(response)
	t.calls[len(t.calls)-1].Status = status
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(encoded))}, nil
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
	return run(c, mode, openAIKey, typeSafeKey, nil, nil)
}

func RunContext(ctx context.Context, c Case, mode, openAIKey, typeSafeKey string) (RunResult, error) {
	return runContext(ctx, c, mode, openAIKey, typeSafeKey, nil, nil)
}

func run(c Case, mode, openAIKey, typeSafeKey string, config *Configuration, budget *Budget) (RunResult, error) {
	return runContext(context.Background(), c, mode, openAIKey, typeSafeKey, config, budget)
}

func runContext(ctx context.Context, c Case, mode, openAIKey, typeSafeKey string, config *Configuration, budget *Budget) (RunResult, error) {
	return runContextWithAssessment(ctx, c, mode, openAIKey, typeSafeKey, config, budget, true)
}

// runProposalContext executes the same production handler contract without
// calling Assess. Proposal-only variants must never receive a Case's gold,
// forbidden patterns or expected failures, so no scoring path is reachable.
func runProposalContext(ctx context.Context, c Case, mode, openAIKey, typeSafeKey string, config *Configuration, budget *Budget) (RunResult, error) {
	return runContextWithAssessment(ctx, c, mode, openAIKey, typeSafeKey, config, budget, false)
}

func runContextWithAssessment(ctx context.Context, c Case, mode, openAIKey, typeSafeKey string, config *Configuration, budget *Budget, assess bool) (RunResult, error) {
	result := RunResult{ID: c.ID, Task: c.Task, Split: c.Split, Language: c.Language, Mode: mode, Role: c.Role}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if mode != "controlled" && mode != "live" {
		return result, fmt.Errorf("unknown evidence mode")
	}
	if c.Task == "profile_review" && mode != "controlled" {
		return result, fmt.Errorf("profile_review evaluation is controlled only")
	}
	if mode == "live" && (openAIKey == "" || (c.Task != "cv_generation" && typeSafeKey == "")) {
		return result, fmt.Errorf("live provider credentials required")
	}
	t := &transport{captureRequests: c.Task == "profile_review" && mode == "controlled", statuses: c.ProviderStatuses, mode: mode, responses: c.Provider, config: config, budget: budget, envelopes: c.ProviderEnvelopes}
	client := &http.Client{Transport: t, Timeout: 55 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var handler http.Handler
	var path string
	switch c.Task {
	case "profile_review":
		handler = review.NewHandlerWithClient(client)
		path = "/api/profile/review"
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
	if budget != nil && (budget.Used >= budget.Limit || budget.WorkflowLimit > 0 && budget.Workflows >= budget.WorkflowLimit) {
		return result, fmt.Errorf("evaluation budget exhausted")
	}
	policy := executionPolicy(ctx)
	if policy.BeforeWorkflow != nil {
		if err := policy.BeforeWorkflow(ctx, c); err != nil {
			return result, err
		}
	}
	if budget != nil {
		budget.Workflows++
	}
	if policy.ObserveWorkflow != nil {
		defer func() { policy.ObserveWorkflow(c, result) }()
	}
	if mode == "controlled" {
		openAIKey = "sk-controlled"
		typeSafeKey = "controlled"
	}
	req := httptest.NewRequest("POST", path, bytes.NewReader(c.Request))
	req.Header.Set("X-OpenAI-Api-Key", openAIKey)
	req.Header.Set("X-TypeSafe-Api-Key", typeSafeKey)
	rec := httptest.NewRecorder()
	req = req.WithContext(aidiagnostics.WithSink(ctx, func(a aidiagnostics.Attempt) { result.Diagnostics = &a }))
	handler.ServeHTTP(rec, req)
	result.Status = rec.Code
	result.NoStore = rec.Header().Get("Cache-Control") == "no-store"
	result.Calls = t.calls
	result.ProviderOutputs = t.outputs
	result.RequestEnvelopes = t.requests
	result.Output = append(json.RawMessage{}, rec.Body.Bytes()...)
	if t.failure != nil {
		return result, t.failure
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if !assess {
		return result, nil
	}
	assessed, err := Assess(c, result)
	result = assessed
	return result, err
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
	if c.Task == "profile_review" {
		facets, facetErr := sectionLabelFacets(c, output)
		if facetErr != nil {
			return result, facetErr
		}
		result.LabelFacetScores = facets
	}
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

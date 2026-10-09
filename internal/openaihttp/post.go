package openaihttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"professional-information-repo/internal/aidiagnostics"
	"time"
)

const completionURL = "https://api.openai.com/v1/chat/completions"

// Post sends one bounded chat-completion request. The caller owns cancel so it
// can keep the deadline active while it reads and validates the response body.
func Post(parent context.Context, client *http.Client, timeout time.Duration, key string, body []byte) (context.Context, context.CancelFunc, *http.Response, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	var settings struct {
		Ceiling *int64 `json:"max_completion_tokens"`
	}
	_ = json.Unmarshal(body, &settings)
	aidiagnostics.BeginProvider(ctx, "openai", settings.Ceiling)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, completionURL, bytes.NewReader(body))
	if err != nil {
		return ctx, cancel, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := client.Do(req)
	if err != nil {
		aidiagnostics.Observe(ctx, aidiagnostics.Observation{Status: 0, Completion: "transport_failure"})
	} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		aidiagnostics.Observe(ctx, aidiagnostics.Observation{Status: resp.StatusCode, Completion: "provider_failure"})
	}
	return ctx, cancel, resp, err
}

package openaihttp

import (
	"bytes"
	"context"
	"net/http"
	"time"
)

const completionURL = "https://api.openai.com/v1/chat/completions"

// Post sends one bounded chat-completion request. The caller owns cancel so it
// can keep the deadline active while it reads and validates the response body.
func Post(parent context.Context, client *http.Client, timeout time.Duration, key string, body []byte) (context.Context, context.CancelFunc, *http.Response, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, completionURL, bytes.NewReader(body))
	if err != nil {
		return ctx, cancel, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := client.Do(req)
	return ctx, cancel, resp, err
}

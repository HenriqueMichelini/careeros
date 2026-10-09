package openaihttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"professional-information-repo/internal/aidiagnostics"
	"strings"
)

var ErrTruncated = errors.New("truncated")

// Completion decodes the full bounded envelope before any workflow consumes prose.
// A parsable content object is insufficient: stop and no refusal are required.
func Completion(ctx context.Context, body io.Reader, limit int64, requiredFields ...string) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	var envelope struct {
		Model   string          `json:"model"`
		Usage   json.RawMessage `json:"usage"`
		Choices []struct {
			Finish  string `json:"finish_reason"`
			Message struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	outcome := "malformed_output"
	observe := func() {
		input, output, reasoning, cached := aidiagnostics.Usage(envelope.Usage, "prompt_tokens", "completion_tokens")
		aidiagnostics.Observe(ctx, aidiagnostics.Observation{Status: 200, Completion: outcome, Model: SafeModel(envelope.Model), TokenUsage: aidiagnostics.TokenUsage{InputTokens: input, OutputTokens: output, ReasoningTokens: reasoning, CachedTokens: cached}})
	}
	defer observe()
	if err != nil {
		outcome = "transport_failure"
		return "", err
	}
	if int64(len(raw)) > limit || json.Unmarshal(raw, &envelope) != nil || len(envelope.Choices) != 1 {
		return "", errors.New("invalid provider envelope")
	}
	c := envelope.Choices[0]
	switch {
	case c.Message.Refusal != "" || c.Finish == "content_filter":
		outcome = "refused"
	case c.Finish == "length":
		outcome = "truncated"
		return "", ErrTruncated
	case c.Finish != "stop":
		outcome = "incomplete_output"
	case strings.TrimSpace(c.Message.Content) == "":
		outcome = "incomplete_output"
	default:
		if !json.Valid([]byte(c.Message.Content)) {
			return "", errors.New("malformed content")
		}
		var fields map[string]json.RawMessage
		if len(requiredFields) > 0 && json.Unmarshal([]byte(c.Message.Content), &fields) == nil {
			for _, field := range requiredFields {
				if _, ok := fields[field]; !ok {
					outcome = "incomplete_output"
					return "", errors.New(outcome)
				}
			}
		}
		outcome = "completed"
		return c.Message.Content, nil
	}
	return "", errors.New(outcome)
}

// Only known application model aliases or dated snapshots are emitted; arbitrary
// provider-returned strings might contain secrets or echoed source content.
func SafeModel(model string) *string {
	for _, base := range []string{"gpt-6-luna", "jev-1.13.0"} {
		if model == base {
			return &model
		}
		if base == "gpt-6-luna" && strings.HasPrefix(model, base+"-") {
			suffix := strings.TrimPrefix(model, base+"-")
			if len(suffix) == 10 {
				valid := true
				for i, c := range suffix {
					if i == 4 || i == 7 {
						valid = valid && c == '-'
					} else {
						valid = valid && c >= '0' && c <= '9'
					}
				}
				if valid {
					return &model
				}
			}
		}
	}
	return nil
}

func IsTimeout(err error) bool {
	var ne net.Error
	return errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) && ne.Timeout()
}

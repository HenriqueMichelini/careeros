package aicontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"professional-information-repo/internal/accountstore"
)

// NumericObservation preserves absence and invalidity. Unsupported categories
// stay explicit; they are never silently priced as zero.
type NumericObservation struct {
	State string `json:"state"`
	Value *int64 `json:"value,omitempty"`
}

// ProviderObservation is an allowlisted, non-content ledger record. It carries
// no provider envelope, error text, content, transport headers or credentials.
type ProviderObservation struct {
	Provider       string             `json:"provider"`
	RequestID      string             `json:"request_id,omitempty"`
	RequestIDState string             `json:"request_id_state"`
	Model          string             `json:"model,omitempty"`
	ModelState     string             `json:"model_state"`
	Tier           string             `json:"tier,omitempty"`
	TierState      string             `json:"tier_state"`
	BodyState      string             `json:"body_state"`
	Input          NumericObservation `json:"input"`
	Output         NumericObservation `json:"output"`
	Cached         NumericObservation `json:"cached"`
	Reasoning      NumericObservation `json:"reasoning"`
	CacheWrite     NumericObservation `json:"cache_write"`
	Unsupported    bool               `json:"unsupported"`
	RateID         string             `json:"rate_id,omitempty"`
	RateState      string             `json:"rate_state"`
	Estimate       *int64             `json:"estimate,omitempty"`
}

var metadataLabel = regexp.MustCompile(`^[A-Za-z0-9_.:/-]{1,128}$`)

func metadataString(raw json.RawMessage) (string, string) {
	if len(raw) == 0 {
		return "", "missing"
	}
	var s string
	if json.Unmarshal(raw, &s) != nil || !metadataLabel.MatchString(s) {
		return "", "invalid"
	}
	return s, "valid"
}
func numeric(raw json.RawMessage) NumericObservation {
	if len(raw) == 0 {
		return NumericObservation{State: "missing"}
	}
	var n int64
	if json.Unmarshal(raw, &n) != nil || n < 0 {
		return NumericObservation{State: "invalid"}
	}
	return NumericObservation{State: "valid", Value: &n}
}
func observation(provider, requestID string, raw []byte, complete bool) ProviderObservation {
	o := ProviderObservation{Provider: provider, RequestIDState: "missing", ModelState: "missing", TierState: "missing", BodyState: "unread", RateState: "missing"}
	o.Input = numeric(nil)
	o.Output = numeric(nil)
	o.Cached = numeric(nil)
	o.Reasoning = numeric(nil)
	o.CacheWrite = numeric(nil)
	if requestID != "" {
		if metadataLabel.MatchString(requestID) {
			o.RequestID = requestID
			o.RequestIDState = "valid"
		} else {
			o.RequestIDState = "invalid"
		}
	}
	if !complete {
		return o
	}
	if !accountstore.StrictJSON(raw) {
		o.BodyState = "invalid"
		return o
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		o.BodyState = "invalid"
		return o
	}
	o.BodyState = "complete"
	o.Model, o.ModelState = metadataString(fields["model"])
	o.Tier, o.TierState = metadataString(fields["service_tier"])
	if o.ModelState == "valid" && o.TierState == "valid" {
		o.RateState = "unsupported"
		if o.Tier == "standard" && (provider == "openai" && o.Model == "gpt-6-luna" || provider == "typesafe" && o.Model == "jev-1.13.0") {
			o.RateState = "supported"
			o.RateID = "synthetic-unit-v1"
		}
	}
	var usage map[string]json.RawMessage
	if len(fields["usage"]) == 0 {
		return o
	}
	if json.Unmarshal(fields["usage"], &usage) != nil || usage == nil {
		o.BodyState = "invalid_usage"
		return o
	}
	if provider == "openai" {
		o.Input = numeric(usage["prompt_tokens"])
		o.Output = numeric(usage["completion_tokens"])
		var input, output map[string]json.RawMessage
		if v, ok := usage["prompt_tokens_details"]; ok && json.Unmarshal(v, &input) != nil {
			o.Unsupported = true
		}
		if v, ok := usage["completion_tokens_details"]; ok && json.Unmarshal(v, &output) != nil {
			o.Unsupported = true
		}
		o.Cached = numeric(input["cached_tokens"])
		o.Reasoning = numeric(output["reasoning_tokens"])
		o.CacheWrite = numeric(input["cache_write_tokens"])
		for key := range usage {
			switch key {
			case "prompt_tokens", "completion_tokens", "total_tokens", "prompt_tokens_details", "completion_tokens_details":
			default:
				o.Unsupported = true
			}
		}
		for key := range input {
			if key != "cached_tokens" {
				o.Unsupported = true
			}
		}
		for key := range output {
			if key != "reasoning_tokens" {
				o.Unsupported = true
			}
		}
	} else if provider == "typesafe" {
		o.Input = numeric(usage["input_tokens"])
		o.Output = numeric(usage["output_tokens"])
		for key := range usage {
			if key != "input_tokens" && key != "output_tokens" {
				o.Unsupported = true
			}
		}
	} else {
		o.Unsupported = true
	}
	for _, category := range []NumericObservation{o.Input, o.Output, o.Cached, o.Reasoning, o.CacheWrite} {
		if category.State == "invalid" {
			o.Unsupported = true
		}
	}
	u := Usage{Input: o.Input.Value, Output: o.Output.Value, Cached: o.Cached.Value, Reasoning: o.Reasoning.Value, CacheWrite: o.CacheWrite.Value, Unsupported: o.Unsupported}
	if n, e := Estimate(provider, o.Model, o.Tier, u); e == nil {
		o.Estimate = &n
	}
	return o
}

func (h *Harness) appendObservation(ctx context.Context, d Dispatch, sequence int, o ProviderObservation) error {
	raw, e := json.Marshal(o)
	if e != nil {
		return ErrUnavailable
	}
	// Observation failure never drops the already committed maximum. Keep this
	// best-effort write bounded even when the originating client disappeared.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_, e = h.db.ExecContext(writeCtx, `INSERT INTO ai_fixture_observation(namespace,dispatch,sequence,data) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, h.cfg.Namespace, d.ID, sequence, raw)
	return safe(e)
}
func (h *Harness) observeResponse(ctx context.Context, d Dispatch, resp *http.Response, callErr error) (*http.Response, error) {
	requestID := ""
	if resp != nil {
		requestID = resp.Header.Get("X-Request-ID")
	}
	initial := observation(d.Provider, requestID, nil, false)
	if callErr != nil {
		initial.BodyState = "transport_error"
	}
	if e := h.appendObservation(ctx, d, 0, initial); e != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return nil, e
	}
	if resp != nil && resp.Body != nil {
		resp.Body = &observedBody{ReadCloser: resp.Body, finish: func(raw []byte, complete bool) {
			_ = h.appendObservation(ctx, d, 1, observation(d.Provider, requestID, raw, complete))
		}}
	}
	return resp, callErr
}

type observedBody struct {
	io.ReadCloser
	buf      bytes.Buffer
	overflow bool
	once     sync.Once
	finish   func([]byte, bool)
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, e := b.ReadCloser.Read(p)
	if n > 0 && !b.overflow {
		if b.buf.Len()+n > 1<<20 {
			b.overflow = true
			b.buf.Reset()
		} else {
			b.buf.Write(p[:n])
		}
	}
	if e != nil {
		b.once.Do(func() { b.finish(b.buf.Bytes(), e == io.EOF && !b.overflow) })
	}
	return n, e
}
func (b *observedBody) Close() error {
	e := b.ReadCloser.Close()
	b.once.Do(func() { b.finish(nil, false) })
	return e
}

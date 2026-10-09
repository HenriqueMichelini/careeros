// Package aidiagnostics records only application-owned labels and numeric metadata.
// It never accepts source text, provider identifiers, prompts, keys or their hashes.
package aidiagnostics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Provider struct {
	Name            string  `json:"name"`
	Model           *string `json:"returnedModel"`
	InputTokens     *int64  `json:"inputTokens"`
	OutputTokens    *int64  `json:"outputTokens"`
	ReasoningTokens *int64  `json:"reasoningTokens"`
	CachedTokens    *int64  `json:"prefixCachedTokens"`
	OutputCeiling   *int64  `json:"outputTokenCeiling"`
	DurationMS      float64 `json:"durationMs"`
	Status          int     `json:"httpStatus"`
	Completion      string  `json:"completion"`
	started         time.Time
}
type Stage struct {
	Name       string    `json:"name"`
	Version    string    `json:"version"`
	Outcome    string    `json:"outcome"`
	DurationMS float64   `json:"durationMs"`
	Provider   *Provider `json:"provider"`
}
type Attempt struct {
	Version     string   `json:"version"`
	ID          string   `json:"attemptId"`
	Workflow    string   `json:"workflow"`
	Outcome     string   `json:"outcome"`
	Status      int      `json:"httpStatus"`
	DurationMS  float64  `json:"durationMs"`
	ResultReuse bool     `json:"applicationResultReuse"`
	Stages      []*Stage `json:"stages"`
	// Acceptance is external evidence, never inferred from a successful HTTP result.
	Accepted *bool `json:"accepted"`
}
type key int

const (
	sinkKey key = iota
	attemptKey
	stageKey
)

type Sink func(Attempt)

func WithSink(ctx context.Context, sink Sink) context.Context {
	return context.WithValue(ctx, sinkKey, sink)
}
func milliseconds(start time.Time) float64 {
	return float64(time.Since(start)) / float64(time.Millisecond)
}
func Log(a Attempt) {
	raw, err := json.Marshal(a)
	if err == nil {
		log.Printf("ai_diagnostic %s", raw)
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *responseWriter) Write(raw []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(raw)
}
func Workflow(name string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		var id [16]byte
		_, err := rand.Read(id[:])
		identity := ""
		if err == nil {
			identity = hex.EncodeToString(id[:])
		}
		a := &Attempt{Version: "ai-diagnostics-v1", ID: identity, Workflow: name, Stages: []*Stage{}}
		writer := &responseWriter{ResponseWriter: w}
		ctx := context.WithValue(r.Context(), attemptKey, a)
		next.ServeHTTP(writer, r.WithContext(ctx))
		a.Status = writer.status
		if a.Status == 0 {
			a.Status = 200
		}
		a.DurationMS = milliseconds(started)
		if a.Outcome == "" {
			a.Outcome = "completed"
			if a.Status >= 400 {
				a.Outcome = "input_rejected"
			}
		}
		for _, stage := range a.Stages {
			if stage.Outcome != "completed" && stage.Outcome != "" {
				a.Outcome = stage.Outcome
			}
		}
		sink, _ := ctx.Value(sinkKey).(Sink)
		if sink == nil {
			sink = Log
		}
		sink(*a)
	})
}

// Start starts a synchronous stage. Names and versions must be code constants.
func Start(ctx context.Context, name, version string) (context.Context, func(string)) {
	s := &Stage{Name: name, Version: version}
	started := time.Now()
	if a, ok := ctx.Value(attemptKey).(*Attempt); ok {
		a.Stages = append(a.Stages, s)
	}
	return context.WithValue(ctx, stageKey, s), func(outcome string) {
		s.DurationMS = milliseconds(started)
		if outcome == "invalid_output" {
			outcome = "rejected_output"
		}
		s.Outcome = outcome
		if outcome == "" {
			s.Outcome = "completed"
		}
		if s.Provider != nil && s.Provider.Completion != "completed" && s.Provider.Completion != "" && outcome != "timeout" {
			s.Outcome = s.Provider.Completion
		}
	}
}
func BeginProvider(ctx context.Context, name string, ceiling *int64) {
	if s, ok := ctx.Value(stageKey).(*Stage); ok {
		s.Provider = &Provider{Name: name, OutputCeiling: ceiling, started: time.Now()}
	}
}
func Observe(ctx context.Context, status int, completion string, model *string, input, output, reasoning, cached *int64) {
	if s, ok := ctx.Value(stageKey).(*Stage); ok && s.Provider != nil {
		p := s.Provider
		p.Status = status
		p.DurationMS = milliseconds(p.started)
		p.Completion = completion
		p.Model = model
		p.InputTokens = input
		p.OutputTokens = output
		p.ReasoningTokens = reasoning
		p.CachedTokens = cached
	}
}

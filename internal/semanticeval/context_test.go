package semanticeval_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	eval "professional-information-repo/internal/semanticeval"
	"sync"
	"testing"
	"time"
)

func controlledFact(t *testing.T) eval.Case {
	t.Helper()
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/cases.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []eval.Case `json:"cases"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	for _, c := range corpus.Cases {
		if c.ID == "en-fact" {
			return c
		}
	}
	t.Fatal("missing retained fact")
	return eval.Case{}
}

func TestStorageHealthFailureStopsBeforeControlledProviderDispatch(t *testing.T) {
	failure := errors.New("storage unavailable")
	ctx := eval.WithExecutionPolicy(context.Background(), eval.ExecutionPolicy{BeforeDispatch: func(context.Context) error { return failure }})
	budget := &eval.Budget{Limit: 10, WorkflowLimit: 2}
	r, err := eval.RunConfiguredContext(ctx, controlledFact(t), "controlled", "", "", "current", budget)
	if !errors.Is(err, failure) || len(r.Calls) != 0 || budget.Used != 0 {
		t.Fatalf("storage failure dispatched provider: calls=%d used=%d err=%v", len(r.Calls), budget.Used, err)
	}
}

type blockedBody struct {
	closed chan struct{}
	once   sync.Once
}

func (b *blockedBody) Read([]byte) (int, error) { <-b.closed; return 0, io.ErrClosedPipe }
func (b *blockedBody) Close() error             { b.once.Do(func() { close(b.closed) }); return nil }

func TestCancellationClosesBlockedResponseReadAndRetainsDispatchedAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reading := make(chan struct{})
	ctx = eval.WithExecutionPolicy(ctx, eval.ExecutionPolicy{ControlledBody: func(context.Context, io.ReadCloser) io.ReadCloser {
		close(reading)
		return &blockedBody{closed: make(chan struct{})}
	}})
	type outcome struct {
		run eval.RunResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		r, err := eval.RunConfiguredContext(ctx, controlledFact(t), "controlled", "", "", "current", &eval.Budget{Limit: 10, WorkflowLimit: 2})
		done <- outcome{r, err}
	}()
	select {
	case <-reading:
	case <-time.After(time.Second):
		t.Fatal("controlled body never reached")
	}
	cancel()
	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) || len(got.run.Calls) != 1 || got.run.Calls[0].InputTokens != nil {
			t.Fatalf("lost canceled dispatch: calls=%d err=%v", len(got.run.Calls), got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("body read ignored cancellation")
	}
}

func TestCanceledControlledExecutionStopsBeforeAnyDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	budget := &eval.Budget{Limit: 10, WorkflowLimit: 2}
	r, err := eval.RunConfiguredContext(ctx, controlledFact(t), "controlled", "", "", "current", budget)
	if !errors.Is(err, context.Canceled) || len(r.Calls) != 0 || budget.Used != 0 || budget.Workflows != 0 {
		t.Fatalf("cancellation dispatched work: calls=%d budget=%+v err=%v", len(r.Calls), budget, err)
	}
}

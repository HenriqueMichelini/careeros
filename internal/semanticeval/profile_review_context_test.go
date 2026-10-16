package semanticeval_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	eval "professional-information-repo/internal/semanticeval"
	"testing"
	"time"
)

func sectionCase(t *testing.T, id string) eval.Case {
	t.Helper()
	raw, err := os.ReadFile("../../docs/evaluations/section-review/cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []eval.Case }
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	for _, c := range corpus.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatal("missing section fixture")
	return eval.Case{}
}
func TestSectionReviewCancellationAndDeadlineRetainPartialStagesWithoutRetry(t *testing.T) {
	for _, lang := range []string{"en", "pt"} {
		for _, fault := range []string{"cancel-rewrite", "cancel-support", "deadline"} {
			t.Run(lang+"-"+fault, func(t *testing.T) {
				c := sectionCase(t, lang+"-grounded")
				limit := time.Second
				if fault == "deadline" {
					limit = 100 * time.Millisecond
				}
				ctx, cancel := context.WithTimeout(context.Background(), limit)
				defer cancel()
				stopAt := 1
				if fault == "cancel-support" {
					stopAt = 2
				}
				reading := make(chan struct{})
				calls := 0
				ctx = eval.WithExecutionPolicy(ctx, eval.ExecutionPolicy{ControlledBody: func(_ context.Context, body io.ReadCloser) io.ReadCloser {
					calls++
					if calls == stopAt {
						close(reading)
						return &blockedBody{closed: make(chan struct{})}
					}
					return body
				}})
				type outcome struct {
					run eval.RunResult
					err error
				}
				done := make(chan outcome, 1)
				go func() { run, err := eval.RunContext(ctx, c, "controlled", "", ""); done <- outcome{run, err} }()
				select {
				case <-reading:
				case <-time.After(time.Second):
					t.Fatal("controlled section body was not reached")
				}
				expected := context.DeadlineExceeded
				if fault != "deadline" {
					expected = context.Canceled
					cancel()
				}
				select {
				case got := <-done:
					if !errors.Is(got.err, expected) || got.run.Status != 504 || !got.run.NoStore || len(got.run.Calls) != stopAt || len(got.run.ProviderOutputs) != stopAt-1 || got.run.Calls[stopAt-1].InputTokens != nil {
						t.Fatalf("partial/cancel contract: status%d calls%d outputs%d err%v", got.run.Status, len(got.run.Calls), len(got.run.ProviderOutputs), got.err)
					}
					if got.run.Diagnostics == nil || len(got.run.Diagnostics.Stages) != stopAt {
						t.Fatal("lost observed terminal stages")
					}
				case <-time.After(time.Second):
					t.Fatal("section response ignored cancellation")
				}
			})
		}
	}
}
func TestSectionReviewAdapterDoesNotEnableLiveExecution(t *testing.T) {
	run, err := eval.Run(sectionCase(t, "en-grounded"), "live", "sk-test", "test")
	if err == nil || len(run.Calls) != 0 || len(run.RequestEnvelopes) != 0 {
		t.Fatal("section adapter enabled live dispatch")
	}
}

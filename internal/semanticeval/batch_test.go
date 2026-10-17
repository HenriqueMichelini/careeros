package semanticeval_test

import (
	"context"
	"errors"
	eval "professional-information-repo/internal/semanticeval"
	"testing"
)

func TestBatchStopsAfterResultPersistenceFailure(t *testing.T) {
	c := controlledFact(t)
	failure := errors.New("cannot persist completed case")
	observed := 0
	results := 0
	ctx := eval.WithExecutionPolicy(context.Background(), eval.ExecutionPolicy{ObserveDispatch: func(o eval.DispatchObservation) {
		if o.Phase == eval.DispatchStarted {
			observed++
		}
	}})
	err := eval.ExecuteBatch(ctx, eval.Batch{Cases: []eval.Case{c, c}, Mode: "controlled"}, func(eval.Case, string, eval.RunResult, error) error { results++; return failure })
	if !errors.Is(err, failure) || results != 1 || observed != 3 {
		t.Fatalf("batch ignored persistence failure: results=%d calls=%d err=%v", results, observed, err)
	}
}

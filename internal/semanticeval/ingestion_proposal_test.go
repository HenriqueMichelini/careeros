package semanticeval_test

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	eval "professional-information-repo/internal/semanticeval"
)

type proposalDeniedTransport struct{ calls *atomic.Int64 }

func (d proposalDeniedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	d.calls.Add(1)
	return nil, errors.New("external network denied by controlled test")
}

func proposalOnlyCase(t *testing.T) eval.Case {
	t.Helper()
	c := controlledFact(t)
	c.Gold = nil
	c.Forbidden = nil
	c.Entities = nil
	c.ExpectedFailures = nil
	c.ExpectedStatus = 0
	c.ExpectedCalls = 0
	c.AssessorScenario = ""
	c.Adjudication = ""
	c.SourceSpans = nil
	c.SequenceOutput = ""
	c.Steps = nil
	return c
}

func TestExecuteProposalBatchRunsProductionHandlerWithoutAssessmentOrGold(t *testing.T) {
	var external atomic.Int64
	previous := http.DefaultTransport
	http.DefaultTransport = proposalDeniedTransport{calls: &external}
	t.Cleanup(func() { http.DefaultTransport = previous })

	c := proposalOnlyCase(t)
	var observed eval.Case
	ctx := eval.WithExecutionPolicy(context.Background(), eval.ExecutionPolicy{BeforeWorkflow: func(_ context.Context, c eval.Case) error {
		observed = c
		return nil
	}})
	var run eval.RunResult
	retained := 0
	err := eval.ExecuteProposalBatch(ctx, eval.Batch{Cases: []eval.Case{c}, Configurations: []string{"current"}, Mode: "controlled", Budget: &eval.Budget{Limit: 10, WorkflowLimit: 1}}, func(got eval.Case, configuration string, result eval.RunResult, evaluationErr error) error {
		if got.ID != c.ID || configuration != "current" {
			t.Fatalf("retention received the wrong case/configuration: %s %s", got.ID, configuration)
		}
		retained++
		run = result
		return evaluationErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if retained != 1 || run.Status != 200 || len(run.Calls) != 3 || len(run.Output) == 0 {
		t.Fatalf("proposal batch did not execute the production handler: retained=%d status=%d calls=%d output=%d", retained, run.Status, len(run.Calls), len(run.Output))
	}
	if !reflect.DeepEqual(run.Score, eval.Result{}) || run.LabelFacetScores != nil {
		t.Fatalf("proposal-only execution was assessed: score=%+v facets=%+v", run.Score, run.LabelFacetScores)
	}
	if observed.Gold != nil || observed.Forbidden != nil || observed.ExpectedFailures != nil || observed.ExpectedStatus != 0 || observed.ExpectedCalls != 0 || observed.Adjudication != "" || observed.SequenceOutput != "" || observed.Steps != nil {
		t.Fatalf("execution policy observed expectation fields: %+v", observed)
	}
	if external.Load() != 0 {
		t.Fatalf("proposal execution attempted %d external requests", external.Load())
	}
}

func TestExecuteProposalBatchRefusesEveryExpectationFieldBeforeExecution(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*eval.Case)
	}{
		{"gold", func(c *eval.Case) { c.Gold = []eval.Atom{{ID: "gold"}} }},
		{"forbidden", func(c *eval.Case) { c.Forbidden = []string{"expert"} }},
		{"expected failures", func(c *eval.Case) { c.ExpectedFailures = []string{"HTTP 500"} }},
		{"source spans", func(c *eval.Case) { c.SourceSpans = []eval.SourceSpan{{ID: "span"}} }},
		{"entities", func(c *eval.Case) { c.Entities = []string{"Java"} }},
		{"expected status", func(c *eval.Case) { c.ExpectedStatus = 500 }},
		{"expected calls", func(c *eval.Case) { c.ExpectedCalls = 3 }},
		{"assessor scenario", func(c *eval.Case) { c.AssessorScenario = "scenario" }},
		{"adjudication", func(c *eval.Case) { c.Adjudication = "accepted" }},
		{"sequence output", func(c *eval.Case) { c.SequenceOutput = "final" }},
		{"staged sequence", func(c *eval.Case) { c.Steps = []eval.Case{proposalOnlyCase(t)} }},
		{"wrong task", func(c *eval.Case) { c.Task = "profile_review" }},
		{"empty identity", func(c *eval.Case) { c.ID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := proposalOnlyCase(t)
			tc.mutate(&c)
			retained := 0
			err := eval.ExecuteProposalBatch(context.Background(), eval.Batch{Cases: []eval.Case{c}, Configurations: []string{"current"}, Mode: "controlled", Budget: &eval.Budget{Limit: 10, WorkflowLimit: 1}}, func(eval.Case, string, eval.RunResult, error) error {
				retained++
				return nil
			})
			if err == nil || retained != 0 {
				t.Fatalf("proposal execution accepted %s: retained=%d err=%v", tc.name, retained, err)
			}
		})
	}
	if _, err := eval.RunConfiguredProposalContext(context.Background(), controlledFact(t), "", "", "current", &eval.Budget{Limit: 10, WorkflowLimit: 1}); err == nil {
		t.Fatal("direct proposal execution accepted a fully labelled fixture case")
	}
}

func TestExecuteProposalBatchRefusesCredentialsUnsupportedModesAndUnpinnedConfiguration(t *testing.T) {
	c := proposalOnlyCase(t)
	for _, tc := range []struct {
		name  string
		batch eval.Batch
	}{
		{"OpenAI credential", eval.Batch{Cases: []eval.Case{c}, Configurations: []string{"current"}, Mode: "controlled", OpenAIKey: "sk-test", Budget: &eval.Budget{Limit: 10, WorkflowLimit: 1}}},
		{"TypeSafe credential", eval.Batch{Cases: []eval.Case{c}, Configurations: []string{"current"}, Mode: "controlled", TypeSafeKey: "test", Budget: &eval.Budget{Limit: 10, WorkflowLimit: 1}}},
		{"live mode", eval.Batch{Cases: []eval.Case{c}, Configurations: []string{"current"}, Mode: "live", Budget: &eval.Budget{Limit: 10, WorkflowLimit: 1}}},
		{"unknown configuration", eval.Batch{Cases: []eval.Case{c}, Configurations: []string{"arbitrary"}, Mode: "controlled", Budget: &eval.Budget{Limit: 10, WorkflowLimit: 1}}},
		{"unpinned configuration", eval.Batch{Cases: []eval.Case{c}, Mode: "controlled", Budget: &eval.Budget{Limit: 10, WorkflowLimit: 1}}},
		{"missing budget", eval.Batch{Cases: []eval.Case{c}, Configurations: []string{"current"}, Mode: "controlled"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := eval.ExecuteProposalBatch(context.Background(), tc.batch, func(eval.Case, string, eval.RunResult, error) error { return nil }); err == nil {
				t.Fatalf("proposal batch accepted %s", tc.name)
			}
		})
	}
	if err := eval.ExecuteProposalBatch(context.Background(), eval.Batch{Cases: []eval.Case{c}, Configurations: []string{"current"}, Mode: "controlled", Budget: &eval.Budget{Limit: 10, WorkflowLimit: 1}}, nil); err == nil {
		t.Fatal("proposal batch accepted a nil retention callback")
	}
	if _, err := eval.RunConfiguredProposalContext(context.Background(), c, "sk-test", "test", "current", &eval.Budget{Limit: 10, WorkflowLimit: 1}); err == nil {
		t.Fatal("direct proposal execution accepted credentials")
	}
}

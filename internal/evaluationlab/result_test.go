package evaluationlab_test

import (
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func TestResultContractAllowsCompletedEngineWithFailedAssertions(t *testing.T) {
	r := lab.RunRecord{Version: lab.ResultVersion, ID: "run-1", EngineState: "completed", AssertionVerdict: "fail", Context: lab.ExecutionContext{Mode: "controlled", Concurrency: 1}, Selection: lab.Selection{Suite: "semantic-quality", Mode: "controlled", Configuration: "current"}}
	if err := lab.ValidateResult(r); err != nil {
		t.Fatal(err)
	}
	r.EngineState = "trusted-supported"
	if err := lab.ValidateResult(r); err == nil {
		t.Fatal("imported trust label accepted as engine state")
	}
	r.EngineState = "completed"
	r.Context.Concurrency = 2
	if err := lab.ValidateResult(r); err == nil {
		t.Fatal("unsupported parallel execution accepted")
	}
}

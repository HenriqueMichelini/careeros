package evaluationlab_test

import (
	"os"
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func TestFrozenPlanBindsSelectionOrderFixtureBytesAndExecutionCaps(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/cases.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	planner, err := lab.NewPlanner(map[string][]byte{"semantic-quality": raw})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planner.Plan(lab.Selection{Suite: "semantic-quality", Mode: "controlled", Configuration: "current", CaseIDs: []string{"en-fact"}}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 5, MaxArtifactBytes: 1 << 20, DeadlineMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ID == "" || plan.Hash == "" || len(plan.CaseOrder) != 1 || plan.CaseOrder[0] != "en-fact" || plan.RequiredHTTPAttempts != 1 {
		t.Fatalf("incomplete frozen plan: %+v", plan)
	}
	if err := planner.ValidatePlan(plan); err != nil {
		t.Fatal(err)
	}
	plan.Caps.MaxProviderCalls++
	if err := planner.ValidatePlan(plan); err == nil {
		t.Fatal("altered cap accepted")
	}
	raw[0] = '!'
	if _, err := lab.NewPlanner(map[string][]byte{"semantic-quality": raw}); err == nil {
		t.Fatal("changed fixture accepted")
	}
}

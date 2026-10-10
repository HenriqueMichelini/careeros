package semanticeval

import (
	"encoding/json"
	"os"
	"testing"
)

func factFixture(t *testing.T) Case {
	t.Helper()
	var corpus struct {
		Cases []Case `json:"cases"`
	}
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/cases.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	var fixture Case
	for _, c := range corpus.Cases {
		if c.ID == "en-fact" {
			fixture = c
		}
	}
	return fixture
}

func TestConfiguredEvaluationPreservesFieldGateAndBounds(t *testing.T) {
	fixture := factFixture(t)
	baseline, err := Run(fixture, "controlled", "", "")
	if err != nil {
		t.Fatal(err)
	}
	run, err := RunConfigured(fixture, "controlled", "", "", "luna-low", &Budget{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != 200 || len(run.Calls) != 3 {
		t.Fatalf("incomplete workflow: %+v", run)
	}
	if run.Calls[0].Model != "jev-1.13.0" {
		t.Fatal("changed approved field gate")
	}
	for _, call := range run.Calls[1:] {
		wantBound := float64(6000)
		if call.Model != "gpt-6-luna" || call.Reasoning != "low" || call.MaxCompletionTokens != wantBound {
			t.Fatalf("wrong bounded candidate: %+v", call)
		}
	}
	for i, call := range run.Calls {
		if call.PromptHash != baseline.Calls[i].PromptHash || call.SchemaHash != baseline.Calls[i].SchemaHash || call.InstructionPrefixHash != baseline.Calls[i].InstructionPrefixHash {
			t.Fatal("comparison changed prompts/schema or padded instruction prefix")
		}
	}
}

func TestEvaluationBudgetAndUnknownSettingsFailClosed(t *testing.T) {
	c := factFixture(t)
	budget := &Budget{Limit: 1}
	run, err := RunConfigured(c, "controlled", "", "", "current", budget)
	if err != nil {
		t.Fatal(err)
	}
	if budget.Used != 1 || len(run.Calls) != 1 || run.Status != 502 {
		t.Fatalf("budget did not stop downstream calls: %+v, %+v", budget, run)
	}
	if _, err := RunConfigured(c, "controlled", "", "", "current", budget); err == nil {
		t.Fatal("exhausted budget accepted another workflow")
	}
	if _, err := RunConfigured(c, "controlled", "", "", "arbitrary-model", &Budget{Limit: 3}); err == nil {
		t.Fatal("arbitrary configuration accepted")
	}
}

func TestEvaluationRecordsReturnedCacheWritesWithoutInventingUsage(t *testing.T) {
	c := factFixture(t)
	c.ProviderEnvelopes = []json.RawMessage{json.RawMessage(`{"model":"gpt-6-luna","usage":{"prompt_tokens":1200,"completion_tokens":40,"prompt_tokens_details":{"cached_tokens":1024,"cache_write_tokens":128},"completion_tokens_details":{"reasoning_tokens":10}}}`)}
	run, err := RunConfigured(c, "controlled", "", "", "current", &Budget{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	call := run.Calls[1]
	if call.InputTokens == nil || *call.InputTokens != 1200 || call.CacheWriteTokens == nil || *call.CacheWriteTokens != 128 || call.CachedTokens == nil || *call.CachedTokens != 1024 {
		t.Fatalf("provider usage lost: %+v", call)
	}
	if run.Calls[2].InputTokens != nil {
		t.Fatal("absent usage became zero")
	}
}

func TestFrozenCompletedWorkflowMatrix(t *testing.T) {
	var data struct {
		Cases []Case `json:"cases"`
	}
	raw, err := os.ReadFile("../../docs/evaluations/workflow-settings/cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if err = Validate(data.Cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range data.Cases {
		t.Run(c.ID, func(t *testing.T) {
			run, err := RunConfigured(c, "controlled", "", "", "current", &Budget{Limit: 20})
			if err != nil {
				t.Fatal(err)
			}
			if run.Status != 200 || !run.NoStore || len(run.Score.Failures) > 0 {
				t.Fatalf("incomplete acceptance fixture: status=%d score=%+v output=%s", run.Status, run.Score, run.Output)
			}
			if len(c.Steps) > 0 && len(run.Steps) != len(c.Steps) {
				t.Fatal("continuation stopped before complete original coverage")
			}
		})
	}
}

func TestHTTPWorkflowCapIncludesContinuationAndRejectsBeforeDispatch(t *testing.T) {
	c := factFixture(t)
	budget := &Budget{Limit: 20, WorkflowLimit: 1}
	if _, err := RunConfigured(c, "controlled", "", "", "sol-low", budget); err != nil {
		t.Fatal(err)
	}
	if budget.Workflows != 1 || budget.Used != 3 {
		t.Fatalf("missing workflow/call accounting: %+v", budget)
	}
	if _, err := RunConfigured(c, "controlled", "", "", "current", budget); err == nil {
		t.Fatal("HTTP workflow cap was bypassed")
	}
	if budget.Used != 3 {
		t.Fatal("exhausted workflow dispatched a provider call")
	}
}

func TestCurrentReviewedDraftControlsExposeInventedLeadershipAndPortugueseAnswerOmission(t *testing.T) {
	var data struct {
		Cases []Case `json:"cases"`
	}
	raw, err := os.ReadFile("../../docs/evaluations/workflow-settings/cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ id, field, text, missing string }{{"en-reviewed-draft", "body", "I led ten engineers.", "cover-java"}, {"pt-reviewed-draft", "applicationAnswers", "AWS é obrigatório.", "answers-java"}} {
		t.Run(tc.missing, func(t *testing.T) {
			var c Case
			for _, fixture := range data.Cases {
				if fixture.ID == tc.id {
					c = fixture
				}
			}
			var generated map[string]any
			if err = json.Unmarshal(c.Provider[0], &generated); err != nil {
				t.Fatal(err)
			}
			if tc.field == "body" {
				generated["coverLetter"].(map[string]any)["body"] = tc.text
			} else {
				generated[tc.field] = tc.text
			}
			c.Provider[0], _ = json.Marshal(generated)
			run, err := RunConfigured(c, "controlled", "", "", "current", &Budget{Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, missing := range run.Score.Missing {
				found = found || missing == tc.missing
			}
			if !found || run.Score.Completeness == 1 {
				t.Fatalf("field defect hidden by supported Java elsewhere: %+v", run.Score)
			}
		})
	}
}

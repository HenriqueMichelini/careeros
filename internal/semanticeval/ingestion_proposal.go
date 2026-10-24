package semanticeval

import (
	"context"
	"fmt"
)

// RunConfiguredProposalContext executes one controlled Profile ingestion
// proposal through the production handler without calling Assess. It refuses a
// Case that still carries any expectation field: proposal-only execution must
// not receive gold, forbidden patterns, expected failures or expected calls.
func RunConfiguredProposalContext(ctx context.Context, c Case, openAIKey, typeSafeKey, id string, budget *Budget) (RunResult, error) {
	if err := validateProposalCase(c); err != nil {
		return RunResult{}, err
	}
	if openAIKey != "" || typeSafeKey != "" {
		return RunResult{}, fmt.Errorf("controlled proposal execution refuses credentials")
	}
	if err := ctx.Err(); err != nil {
		return RunResult{}, err
	}
	config, err := configuration(id)
	if err != nil {
		return RunResult{}, err
	}
	if budget == nil || budget.Limit < 1 || budget.Used >= budget.Limit {
		return RunResult{}, fmt.Errorf("provider call budget exhausted")
	}
	return runProposalContext(ctx, c, "controlled", openAIKey, typeSafeKey, &config, budget)
}

// ExecuteProposalBatch is the proposal-only counterpart of ExecuteBatch. The
// controlled transport and production handler stay identical; assessment and
// every gold-dependent code path are deliberately absent.
func ExecuteProposalBatch(ctx context.Context, batch Batch, retain func(Case, string, RunResult, error) error) error {
	if retain == nil {
		return fmt.Errorf("result retention callback required")
	}
	if batch.Mode != "controlled" {
		return fmt.Errorf("proposal-only batch requires controlled mode")
	}
	if batch.OpenAIKey != "" || batch.TypeSafeKey != "" {
		return fmt.Errorf("controlled proposal batch refuses credentials")
	}
	configurations := batch.Configurations
	if len(configurations) == 0 {
		configurations = []string{""}
	}
	for _, id := range configurations {
		if id != "" {
			if _, err := configuration(id); err != nil {
				return err
			}
		}
	}
	for _, c := range batch.Cases {
		if err := validateProposalCase(c); err != nil {
			return err
		}
	}
	for _, c := range batch.Cases {
		for _, id := range configurations {
			if err := ctx.Err(); err != nil {
				return err
			}
			var run RunResult
			var err error
			if id == "" {
				return fmt.Errorf("proposal-only execution requires a pinned configuration")
			}
			run, err = RunConfiguredProposalContext(ctx, c, batch.OpenAIKey, batch.TypeSafeKey, id, batch.Budget)
			if retentionErr := retain(c, id, run, err); retentionErr != nil {
				return retentionErr
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func validateProposalCase(c Case) error {
	if c.ID == "" || c.Task != "profile_ingestion" {
		return fmt.Errorf("proposal-only execution requires a Profile ingestion case")
	}
	if len(c.Steps) > 0 {
		return fmt.Errorf("proposal-only adapter does not accept a staged sequence")
	}
	if len(c.Gold) > 0 || len(c.Forbidden) > 0 || len(c.ExpectedFailures) > 0 || len(c.SourceSpans) > 0 || len(c.Entities) > 0 || c.ExpectedStatus != 0 || c.ExpectedCalls != 0 || c.AssessorScenario != "" || c.Adjudication != "" || c.SequenceOutput != "" {
		return fmt.Errorf("proposal-only execution refuses expectation fields")
	}
	return nil
}

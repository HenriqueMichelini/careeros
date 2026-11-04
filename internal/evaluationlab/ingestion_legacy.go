package evaluationlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"professional-information-repo/internal/semanticeval"
)

const (
	LegacyIngestionVariantID       = "ingestion-legacy-proposal-v1"
	LegacyIngestionAdapterVersion  = "lab-ingestion-legacy-proposal-v1"
	LegacyIngestionEvidenceVersion = "lab-ingestion-legacy-execution-v1"
	LegacyIngestionScope           = "reviewable proposal only: before user Apply/Save, canonical mutation, browser acceptance or real-model quality evidence"
)

// The documented five-case cohort is fixed for variant A. The order is part of
// the frozen execution plan, not a caller preference.
var legacyIngestionCaseIDs = []string{
	"en-fact",
	"pt-fact",
	"en-negation-aspiration",
	"en-existing-duplicate",
	"en-proficiency-conflict",
}

// LegacyIngestionProposalCaseIDs returns an owned copy of the documented cohort.
func LegacyIngestionProposalCaseIDs() []string {
	return append([]string{}, legacyIngestionCaseIDs...)
}

// LegacyIngestionExecutionEvidence wraps the real handler response and the
// mechanical proposal observation. It contains no Case, gold, score,
// expectation or Apply/Save claim.
type LegacyIngestionExecutionEvidence struct {
	Version     string                                    `json:"version"`
	VariantID   string                                    `json:"variantId"`
	CaseID      string                                    `json:"caseId"`
	Language    string                                    `json:"language"`
	RequestHash string                                    `json:"requestHash"`
	Status      int                                       `json:"status"`
	Response    semanticeval.IngestionResponseEvidence    `json:"response"`
	Observation semanticeval.IngestionProposalObservation `json:"observation"`
	Scope       string                                    `json:"scope"`
}

func planLegacyIngestion(planner *Planner, selection Selection, caps ExecutionCaps) (Plan, error) {
	if selection.Suite != "semantic-quality" || selection.Mode != "controlled" || selection.Configuration != "current" {
		return Plan{}, fmt.Errorf("legacy ingestion variant requires controlled semantic-quality/current")
	}
	order, err := legacyIngestionCohort(selection.CaseIDs)
	if err != nil {
		return Plan{}, err
	}
	selection.CaseIDs = order
	return planner.Plan(selection, caps)
}

// legacyIngestionCohort accepts either no case IDs or exactly the documented
// five unique IDs. It normalizes both forms to the same frozen order.
func legacyIngestionCohort(caseIDs []string) ([]string, error) {
	if len(caseIDs) == 0 {
		return LegacyIngestionProposalCaseIDs(), nil
	}
	if len(caseIDs) != len(legacyIngestionCaseIDs) {
		return nil, fmt.Errorf("legacy ingestion variant requires all five documented cases")
	}
	seen := map[string]bool{}
	for _, id := range caseIDs {
		if seen[id] {
			return nil, fmt.Errorf("duplicate legacy ingestion case")
		}
		seen[id] = true
		if !contains(legacyIngestionCaseIDs, id) {
			return nil, fmt.Errorf("unsupported legacy ingestion case")
		}
	}
	return LegacyIngestionProposalCaseIDs(), nil
}

func (r *Runner) startLegacyIngestion(parent context.Context, plan Plan) (string, error) {
	if plan.Selection.Suite != "semantic-quality" || plan.Selection.Mode != "controlled" || plan.Selection.Configuration != "current" {
		return "", fmt.Errorf("legacy ingestion plan selection is unsupported")
	}
	if _, err := legacyIngestionCohort(plan.Selection.CaseIDs); err != nil {
		return "", err
	}
	if !sameCaseOrder(plan.Selection.CaseIDs, legacyIngestionCaseIDs) || !sameCaseOrder(plan.CaseOrder, legacyIngestionCaseIDs) {
		return "", fmt.Errorf("legacy ingestion plan does not contain the frozen five-case cohort")
	}
	return r.start(parent, plan, LegacyIngestionVariantID)
}

func sameCaseOrder(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// executeLegacyIngestionCase sanitizes the frozen fixture before execution, then
// retains the real handler response and its mechanical observation. The
// sanitizer is intentionally explicit: expectation fields are never copied.
func (r *Runner) executeLegacyIngestionCase(ctx context.Context, caseID string, budget *semanticeval.Budget) (json.RawMessage, error) {
	full, err := r.semanticCase("semantic-quality", caseID)
	if err != nil {
		return nil, err
	}
	executionCase := legacyIngestionProposalCase(full)
	var run semanticeval.RunResult
	err = semanticeval.ExecuteProposalBatch(ctx, semanticeval.Batch{
		Cases:          []semanticeval.Case{executionCase},
		Configurations: []string{"current"},
		Mode:           "controlled",
		Budget:         budget,
	}, func(_ semanticeval.Case, _ string, result semanticeval.RunResult, evaluationErr error) error {
		run = result
		return evaluationErr
	})
	response := semanticeval.IngestionResponseEvidence{ID: caseID, Status: run.Status, Body: bytes.Clone(run.Output)}
	observation := semanticeval.NormalizeLegacyIngestion(response)
	evidence := LegacyIngestionExecutionEvidence{
		Version:     LegacyIngestionEvidenceVersion,
		VariantID:   LegacyIngestionVariantID,
		CaseID:      caseID,
		Language:    full.Language,
		RequestHash: semanticeval.Hash(full.Request),
		Status:      run.Status,
		Response:    response,
		Observation: observation,
		Scope:       LegacyIngestionScope,
	}
	actual, marshalErr := json.Marshal(evidence)
	if marshalErr != nil {
		return nil, marshalErr
	}
	return actual, err
}

// legacyIngestionProposalCase constructs the execution Case from allowed
// fields only. In particular, Gold, Forbidden, ExpectedFailures, expected
// counts/status, sequence scoring and adjudication metadata are absent rather
// than zeroed copies.
func legacyIngestionProposalCase(c semanticeval.Case) semanticeval.Case {
	return semanticeval.Case{
		ProviderStatuses:  append([]int{}, c.ProviderStatuses...),
		ID:                c.ID,
		Task:              c.Task,
		Language:          c.Language,
		Split:             c.Split,
		Request:           bytes.Clone(c.Request),
		Provider:          cloneRawMessages(c.Provider),
		ProviderEnvelopes: cloneRawMessages(c.ProviderEnvelopes),
	}
}

func cloneRawMessages(values []json.RawMessage) []json.RawMessage {
	if values == nil {
		return nil
	}
	result := make([]json.RawMessage, len(values))
	for i := range values {
		result[i] = bytes.Clone(values[i])
	}
	return result
}

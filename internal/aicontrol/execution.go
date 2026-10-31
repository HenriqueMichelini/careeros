package aicontrol

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"time"

	"professional-information-repo/internal/aidiagnostics"
)

// ExecutionIdentity is a server-owned version registry for this synthetic slice.
// The labels describe the actual handler prompt/schema contracts. Prices and
// production policy remain unselected; the sole rate identity is artificial SYN.
type ExecutionIdentity struct {
	Version, Workflow, WorkflowProtocol, RatePolicy string
	Stages                                          map[string]StageIdentity
}
type StageIdentity struct{ Provider, Model, Tier, PromptSchema string }

func executionIdentity(workflow, version, rate string) (ExecutionIdentity, error) {
	e := ExecutionIdentity{Version: version, Workflow: workflow, WorkflowProtocol: "workflow-v1", RatePolicy: rate, Stages: map[string]StageIdentity{}}
	if !tokenLabel.MatchString(version) || rate != "synthetic-unit-v1" {
		return e, ErrDenied
	}
	oa := func(stage, prompt string) {
		e.Stages[stage] = StageIdentity{"openai", "gpt-6-luna", "standard", prompt}
	}
	gate := func() {
		e.Stages["field_validation"] = StageIdentity{"typesafe", "jev-1.13.0", "standard", "jev-rubric-v1/policy-v1"}
	}
	switch workflow {
	case "cv_generation":
		oa("generation", "cv-policy-v2/schema-v2")
		oa("statement_support_assessment", "cv-statement-support-v1")
	case "profile_review":
		oa("section_review", "profile-section-proposal-v1")
		oa("section_support_check", "profile-section-proposal-v1")
	case "application_draft":
		gate()
		oa("generation", "application-draft-policy-v3/context-v1/schema-v1")
		oa("resume_support", "resume-support-openai-v1/number-guard-v2/strength-guard-v2/review-language-v1")
		oa("artifact_support", "artifact-support-openai-v8/prepared-citations-v1/number-guard-v2/answer-input-v1/career-citations-v1/neutral-blocks-v1/range-schema-v2/strength-guard-v2")
	case "qualification_gaps":
		gate()
		oa("qualification_matching", "job-context-v2/prompt-v1/schema-v1/gpt-6-luna/none/jev-rubric-v1/policy-v1")
		oa("requirement_evidence", "qualification-evidence-v2")
	case "profile_ingestion":
		e.WorkflowProtocol = "ingestion-legacy-v1-v2"
		gate()
		oa("extraction", "profile-claims-prompt-v6/schema-v2/source-resolution-v1/portions-v1/clarifications-v1/apply-answer-v1")
		oa("reconciliation", "profile-operations-prompt-v6/schema-v3/dispositions-v1/clarifications-v1/apply-answer-v1")
	default:
		return e, ErrDenied
	}
	return e, nil
}

// executionBinding freezes the persisted operating envelope, not a caller's
// mutable config copy. Scope counters/occupancy are deliberately not policy.
func (h *Harness) executionBinding(ctx context.Context, tx *sql.Tx, owner, workflow string) ([]byte, error) {
	e, err := executionIdentity(workflow, h.cfg.ExecutionPolicyVersion, h.cfg.RatePolicyVersion)
	if err != nil {
		return nil, err
	}
	var policy struct {
		Expires                      time.Time
		Dispatches, Attempts, Stages int
		Units, Input, Output         int64
	}
	if tx.QueryRowContext(ctx, `SELECT expires_at,max_dispatches,max_attempts,max_stages,units,max_input_bytes,max_output_tokens FROM ai_fixture_policy WHERE namespace=$1 FOR UPDATE`, h.cfg.Namespace).Scan(&policy.Expires, &policy.Dispatches, &policy.Attempts, &policy.Stages, &policy.Units, &policy.Input, &policy.Output) != nil {
		return nil, ErrUnavailable
	}
	type scope struct {
		Name     string
		Limit    int64
		Capacity int
	}
	var scopes []scope
	rows, err := tx.QueryContext(ctx, `SELECT scope,limit_amount,capacity FROM ai_fixture_scope WHERE namespace=$1 AND (scope IN ('global','provider:openai','provider:typesafe') OR scope=$2) ORDER BY scope FOR SHARE`, h.cfg.Namespace, "account:"+owner)
	if err != nil {
		return nil, ErrUnavailable
	}
	for rows.Next() {
		var s scope
		if rows.Scan(&s.Name, &s.Limit, &s.Capacity) != nil {
			rows.Close()
			return nil, ErrUnavailable
		}
		scopes = append(scopes, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(scopes) != 4 {
		return nil, ErrUnavailable
	}
	raw, err := json.Marshal(struct {
		Identity ExecutionIdentity
		Policy   any
		Scopes   []scope
	}{e, policy, scopes})
	if err != nil {
		return nil, ErrUnavailable
	}
	digest := sha256.Sum256(raw)
	return digest[:], nil
}

// Caller holds the action lock before policy/scope locks; all admission paths
// use that order. Neither a newer binary nor policy edit reinterprets an Action.
func (s *Session) checkExecution(ctx context.Context, tx *sql.Tx) error {
	var frozen []byte
	var version string
	if tx.QueryRowContext(ctx, `SELECT execution_binding,execution_version FROM ai_fixture_action WHERE namespace=$1 AND id=$2 AND owner=$3`, s.h.cfg.Namespace, s.id, s.p.AccountID).Scan(&frozen, &version) != nil {
		return ErrDenied
	}
	current, err := s.h.executionBinding(ctx, tx, s.p.AccountID, s.workflow)
	if err != nil {
		return err
	}
	if version != s.h.cfg.ExecutionPolicyVersion || !hmac.Equal(frozen, s.execution) || !hmac.Equal(current, frozen) {
		return ErrConflict
	}
	return nil
}

func (s *Session) validateExecutionStage(ctx context.Context, provider, stage string, raw []byte) error {
	e, err := executionIdentity(s.workflow, s.h.cfg.ExecutionPolicyVersion, s.h.cfg.RatePolicyVersion)
	if err != nil {
		return err
	}
	identity, ok := e.Stages[stage]
	if !ok || identity.Provider != provider || aidiagnostics.StageVersion(ctx) != identity.PromptSchema {
		return ErrConflict
	}
	return validateRequestIdentity(identity, raw)
}
func validateRequestIdentity(identity StageIdentity, raw []byte) error {
	var request struct {
		Model string  `json:"model"`
		Tier  *string `json:"service_tier"`
	}
	if json.Unmarshal(raw, &request) != nil || request.Model != identity.Model {
		return ErrConflict
	}
	// These handlers omit service_tier. The synthetic registry explicitly binds
	// that default to standard; any explicit differing tier is denied.
	if request.Tier != nil && *request.Tier != identity.Tier {
		return ErrConflict
	}
	return nil
}

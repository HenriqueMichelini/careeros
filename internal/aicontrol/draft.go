package aicontrol

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"time"

	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/auth"
)

const ManagedDraftPolicyVersion = "application-draft-local-20261104-v1"
const ManagedDraftGenerationVersion = "application-draft-managed-v1/generation-v1"
const ManagedDraftArtifactVersion = "application-draft-managed-v1/artifact-answer-catalog-v1"

var managedDraftPolicy = struct {
	Actions, RollingActions, Attempts, TypeSafeCalls, OpenAICalls int
	TypeSafeInput, OpenAIInput, OpenAIInputTotal, OutputTotal     int64
	AttemptMoney, ActionMoney                                     int64
	GenerationOutput, ResumeOutput, ArtifactOutput                int64
}{
	Actions: 10, RollingActions: 3, Attempts: 2, TypeSafeCalls: 1, OpenAICalls: 3,
	TypeSafeInput: 32_000, OpenAIInput: 64_000, OpenAIInputTotal: 192_000, OutputTotal: 20_000,
	AttemptMoney: 40_000, ActionMoney: 80_000,
	GenerationOutput: 8_000, ResumeOutput: 4_000, ArtifactOutput: 8_000,
}

func ManagedDraftSyntheticConfig(namespace string, gate *SyntheticGate, bound func(string, []byte) (int64, bool)) SyntheticConfig {
	cfg := StandaloneCVSyntheticConfig(namespace, gate, bound)
	cfg.ManagedDraft = true
	return cfg
}

func (h *Harness) ManagedDraftEnabled() bool {
	return h != nil && h.cfg.Qualification && h.cfg.ManagedDraft
}

func (s *Session) managedDraft() bool {
	return s.h.ManagedDraftEnabled() && s.workflow == "application_draft"
}

func draftOutput(stage string) int64 {
	switch stage {
	case "generation":
		return managedDraftPolicy.GenerationOutput
	case "resume_support":
		return managedDraftPolicy.ResumeOutput
	case "artifact_support":
		return managedDraftPolicy.ArtifactOutput
	default:
		return 0
	}
}

func (h *Harness) draftMaximum(leg Leg) (int64, int64, error) {
	if h.cfg.TokenBound == nil || leg.Number != 1 || leg.Coordinate != "request" {
		return 0, 0, ErrDenied
	}
	n, ok := h.cfg.TokenBound(leg.Provider, append([]byte(nil), leg.Body...))
	if !ok || n <= 0 {
		return 0, 0, ErrDenied
	}
	rates := selectedQualificationEnvelope()
	if leg.Provider == "typesafe" && leg.Stage == "field_validation" {
		if n > managedDraftPolicy.TypeSafeInput {
			return 0, 0, ErrCapacity
		}
		return (n*rates.TypeSafeRate + 999) / 1000, n, nil
	}
	var body struct {
		Model     string `json:"model"`
		Tier      string `json:"service_tier"`
		Reasoning string `json:"reasoning_effort"`
		Output    int64  `json:"max_completion_tokens"`
	}
	output := draftOutput(leg.Stage)
	if leg.Provider != "openai" || output <= 0 || json.Unmarshal(leg.Body, &body) != nil ||
		body.Model != "gpt-6-luna" || body.Tier != "standard" || body.Reasoning != "none" || body.Output != output {
		return 0, 0, ErrDenied
	}
	if n > managedDraftPolicy.OpenAIInput {
		return 0, 0, ErrCapacity
	}
	return (n*rates.CacheWriteRate + output*rates.OutputRate + 999) / 1000, n, nil
}

func (s *Session) draftLimits(ctx context.Context, tx *sql.Tx, leg Leg, input, output int64) error {
	if leg.Provider != "openai" {
		return nil
	}
	var usedInput, usedOutput int64
	if tx.QueryRowContext(ctx, `SELECT COALESCE(sum(input_tokens),0),COALESCE(sum(output_tokens),0) FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND attempt=$3 AND provider='openai'`, s.h.cfg.Namespace, s.id, s.attempt).Scan(&usedInput, &usedOutput) != nil {
		return ErrUnavailable
	}
	if usedInput+input > managedDraftPolicy.OpenAIInputTotal || usedOutput+output > managedDraftPolicy.OutputTotal {
		return ErrCapacity
	}
	return nil
}

type draftQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Session) draftRemaining(ctx context.Context, db draftQuery, customerOnly bool) (int64, error) {
	var customer, foreground sql.NullInt64
	if db.QueryRowContext(ctx, `
		SELECT (extract(epoch FROM (a.created_at+interval '30 minutes'-clock_timestamp()))*1000000)::bigint,
		       (extract(epoch FROM ((CASE WHEN $3=1 THEN a.created_at ELSE r.created_at END)+interval '25 seconds'-clock_timestamp()))*1000000)::bigint
		FROM ai_fixture_action a
		LEFT JOIN ai_fixture_stage r ON r.namespace=a.namespace AND r.action=a.id AND r.attempt=$3 AND r.coordinate='request' AND r.stage='managed_request_draft'
		WHERE a.namespace=$1 AND a.id=$2 AND a.owner=$4`, s.h.cfg.Namespace, s.id, s.attempt, s.p.AccountID).Scan(&customer, &foreground) != nil {
		return 0, ErrUnavailable
	}
	if !customer.Valid || customer.Int64 <= 0 {
		return 0, ErrDenied
	}
	if customerOnly {
		return customer.Int64, nil
	}
	if !foreground.Valid || foreground.Int64 <= 0 || foreground.Int64 > 25_000_000 {
		return 0, ErrCapacity
	}
	return min(customer.Int64, foreground.Int64), nil
}

// ManagedDraftContext binds the existing pipeline to the exact admitted root
// and clamps in-flight work to the remaining durable attempt budget.
func (s *Session) ManagedDraftContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if !s.managedDraft() {
		return nil, nil, ErrDenied
	}
	started := time.Now()
	micros, err := s.draftRemaining(ctx, s.h.db, false)
	if err != nil {
		return nil, nil, err
	}
	remaining := time.Duration(micros)*time.Microsecond - time.Since(started)
	if remaining <= 0 {
		return nil, nil, ErrCapacity
	}
	ctx = aidispatch.WithController(aidispatch.RequireManaged(ctx), s)
	ctx = aidispatch.WithWorkflowRoot(ctx, s.workflow, s.rootInput)
	bounded, cancel := context.WithTimeout(ctx, remaining)
	return bounded, cancel, nil
}

// MarkDraftResumeNotNeeded records the existing handler's local disposition.
// It cannot release a held dispatch or turn that draft into a complete version.
func (s *Session) MarkDraftResumeNotNeeded(ctx context.Context) error {
	if !s.managedDraft() {
		return ErrDenied
	}
	tx, err := s.h.begin(ctx, s.p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.stageAction(ctx, tx); err != nil {
		return err
	}
	if err = s.checkCurrent(ctx, tx); err != nil {
		return err
	}
	var generation, resume int
	if tx.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE stage='generation' AND state='observed'),count(*) FILTER(WHERE stage='resume_support') FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND attempt=$3`, s.h.cfg.Namespace, s.id, s.attempt).Scan(&generation, &resume) != nil {
		return ErrUnavailable
	}
	if generation != 1 || resume != 0 {
		return ErrDenied
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_stage(namespace,action,coordinate,stage,attempt,binding) VALUES($1,$2,'request','draft_resume_not_needed',$3,$4) ON CONFLICT DO NOTHING`, s.h.cfg.Namespace, s.id, s.attempt, s.root)
	if err != nil {
		return ErrUnavailable
	}
	return safe(tx.Commit())
}

// AuthorizeDraftPublication runs inside the recovery store's account and
// Action transaction. Every required stage must be reached and terminal, on
// exactly the attempt whose candidate the store is sealing.
func (s *Session) AuthorizeDraftPublication(ctx context.Context, tx *sql.Tx) error {
	if err := s.AuthorizeDraftAttempt(ctx, tx); err != nil {
		return err
	}
	if s.h.hosted != nil {
		if err := s.h.admissionReady(ctx, tx); err != nil {
			return err
		}
		if err := s.checkInferenceEpoch(ctx, tx); err != nil {
			return err
		}
	}
	var total, terminal int
	if tx.QueryRowContext(ctx, `SELECT count(*),count(DISTINCT stage) FILTER(WHERE state='observed' AND remote_state='terminal_proven') FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND attempt=$3`, s.h.cfg.Namespace, s.id, s.attempt).Scan(&total, &terminal) != nil {
		return ErrUnavailable
	}
	if total != 4 || terminal != total {
		return ErrOutputUnavailable
	}
	return nil
}

func (s *Session) AuthorizeDraftAttempt(ctx context.Context, tx *sql.Tx) error {
	if !s.managedDraft() {
		return ErrDenied
	}
	if err := s.stageAction(ctx, tx); err != nil {
		return err
	}
	return s.checkCurrent(ctx, tx)
}

func (s *Session) DraftAttempt() int {
	if !s.managedDraft() {
		return 0
	}
	return s.attempt
}

// FinishDraftFailure never erases a dispatch hold or overwrites a terminal
// Action. Known provider failure needs terminal evidence for every reached
// call before it can enable a single deliberate Retry.
func (s *Session) FinishDraftFailure(ctx context.Context, tx *sql.Tx, retryable bool) error {
	if !s.managedDraft() {
		return ErrDenied
	}
	if err := s.stageAction(ctx, tx); err != nil {
		return err
	}
	finishing := *s
	finishing.foregroundIndependent = true
	if err := finishing.checkCurrent(ctx, tx); err != nil {
		return err
	}
	var reached, invalid int
	if tx.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE state<>'observed' OR remote_state<>'terminal_proven') FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2`, s.h.cfg.Namespace, s.id).Scan(&reached, &invalid) != nil {
		return ErrUnavailable
	}
	if invalid > 0 {
		return nil
	}
	state := "recoverable"
	if retryable && reached > 0 && s.attempt == 1 {
		state = "failed"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ai_fixture_action SET execution_finished=true,state=$3 WHERE namespace=$1 AND id=$2 AND owner=$4 AND attempt=$5 AND state='recoverable'`,
		s.h.cfg.Namespace, s.id, state, s.p.AccountID, s.attempt); err != nil {
		return ErrUnavailable
	}
	return nil
}

type DraftQualificationBinding struct {
	Namespace, Owner, Action string
	Attempt                  int
	GrantVersion             int64
	Profile, Execution       []byte
}

// CompletedDraftQualification verifies ledger provenance in the recovery
// store's existing account/session transaction. No job or Profile content is
// kept in the ledger or returned by this method.
func (h *Harness) CompletedDraftQualification(ctx context.Context, tx *sql.Tx, p auth.Principal, id string) (DraftQualificationBinding, error) {
	out := DraftQualificationBinding{Namespace: h.Namespace(), Owner: p.AccountID, Action: id}
	if h.hosted != nil || !h.ManagedDraftEnabled() || !tokenLabel.MatchString(id) {
		return out, ErrDenied
	}
	if err := h.ready(ctx, tx); err != nil {
		return out, err
	}
	lease, err := h.cfg.Gate.capture(h.cfg.Namespace)
	if err != nil {
		return out, err
	}
	var version, state string
	var finished, compacted bool
	if tx.QueryRowContext(ctx, `SELECT state,execution_finished,compacted,attempt,grant_version,profile_binding,execution_binding,execution_version FROM ai_fixture_action WHERE namespace=$1 AND id=$2 AND owner=$3 AND workflow='qualification_gaps' FOR UPDATE`, h.cfg.Namespace, id, p.AccountID).Scan(&state, &finished, &compacted, &out.Attempt, &out.GrantVersion, &out.Profile, &out.Execution, &version) != nil ||
		state != "recoverable" || !finished || compacted || version != h.executionVersion("qualification_gaps") {
		return out, ErrDenied
	}
	if _, err = h.checkGrant(ctx, tx, p, out.GrantVersion); err != nil {
		return out, err
	}
	profile, err := h.savedProfileBinding(ctx, tx, p.AccountID, false)
	if err != nil || !hmac.Equal(profile, out.Profile) {
		return out, ErrConflict
	}
	execution, err := h.executionBinding(ctx, tx, p.AccountID, "qualification_gaps")
	if err != nil || !hmac.Equal(execution, out.Execution) {
		return out, ErrConflict
	}
	var reached, understood, invalid int
	var policyValid bool
	if tx.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE stage='qualification_matching'),count(*) FILTER(WHERE state<>'observed' OR remote_state<>'terminal_proven') FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND attempt=$3`, h.cfg.Namespace, id, out.Attempt).Scan(&reached, &understood, &invalid) != nil ||
		tx.QueryRowContext(ctx, `SELECT expires_at>clock_timestamp() FROM ai_fixture_policy WHERE namespace=$1`, h.cfg.Namespace).Scan(&policyValid) != nil ||
		reached < 2 || understood != 1 || invalid != 0 || !policyValid {
		return out, ErrDenied
	}
	return out, lease.allowed()
}

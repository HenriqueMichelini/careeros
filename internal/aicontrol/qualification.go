package aicontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

// QualificationSyntheticConfig reuses the restricted admission laboratory with
// selected microUSD limits. It can invoke only a code-installed fake responder.
// No environment flag selects this constructor or supplies real credentials.
func QualificationSyntheticConfig(namespace string, gate *SyntheticGate, bound func(string, []byte) (int64, bool)) SyntheticConfig {
	c := DefaultSyntheticConfig(namespace, gate)
	c.Qualification = true
	c.TokenBound = bound
	c.ExecutionPolicyVersion = "qualification-local-20261102-v1"
	c.RatePolicyVersion = QualificationRateID
	c.Limit = selectedQualificationEnvelope().AccountTotal
	c.Capacity = 1
	c.MaxDispatches = 6
	c.MaxStages = 6
	c.MaxAttempts = 2
	return c
}

func (h *Harness) qualificationGrant(ctx context.Context, tx authorityReader, p auth.Principal, expected int64) (int64, error) {
	var version int64
	var valid bool
	// Account/session locks are already held by begin. #276 operator commands
	// take the same account lock, so eligibility cannot change behind this read.
	err := tx.QueryRowContext(ctx, `SELECT COALESCE((extract(epoch FROM activated_at)*1000000)::bigint,0), state='active' AND age_eligible AND NOT invitation_revoked AND NOT retrieval_held AND NOT control_compromise_held AND NOT careeros_invitation_denied($1) AND expires_at>clock_timestamp() AND EXISTS(SELECT 1 FROM careeros_identities i WHERE i.account_id=$1 AND i.provider=p.provider AND i.issuer=p.issuer AND i.subject=p.subject) FROM careeros_pilot_access p WHERE account_id=$1`, p.AccountID).Scan(&version, &valid)
	if err == sql.ErrNoRows {
		return 0, ErrDenied
	}
	if err != nil {
		return 0, ErrUnavailable
	}
	if !valid || version <= 0 || expected > 0 && expected != version {
		return 0, ErrDenied
	}
	return version, nil
}

func (h *Harness) managedOpen(ctx context.Context, tx *sql.Tx, owner, workflow, exclude string, newAction bool) error {
	// The namespace policy lock orders global last-slot and rolling action races.
	var ignored int
	if tx.QueryRowContext(ctx, `SELECT max_attempts FROM ai_fixture_policy WHERE namespace=$1 FOR UPDATE`, h.cfg.Namespace).Scan(&ignored) != nil {
		return ErrUnavailable
	}
	var total, day, accountActive, globalActive int
	err := tx.QueryRowContext(ctx, `
		WITH actions AS (
			SELECT a.*, (
				(a.state='recoverable' AND NOT a.execution_finished) OR
				(a.workflow<>'application_draft' AND (
					(NOT a.execution_finished AND NOT a.local_execution_tracking) OR EXISTS(
						SELECT 1 FROM ai_fixture_stage l WHERE l.namespace=a.namespace AND l.action=a.id
						AND l.stage='local_execution' AND l.receipt_id=''
					)
				)) OR
				(a.workflow='application_draft' AND a.execution_version=$5 AND EXISTS(
					SELECT 1 FROM ai_fixture_dispatch d WHERE d.namespace=a.namespace AND d.action=a.id
					AND d.state IN ('intent','observed') AND d.remote_state<>'terminal_proven'
				))
			) AS active FROM ai_fixture_action a WHERE namespace=$1
		)
		SELECT count(*) FILTER(WHERE owner=$2 AND workflow=$3),
			count(*) FILTER(WHERE owner=$2 AND workflow=$3 AND created_at>clock_timestamp()-interval '24 hours'),
			count(*) FILTER(WHERE id<>$4 AND owner=$2 AND active),
			count(*) FILTER(WHERE id<>$4 AND active)
		FROM actions`, h.cfg.Namespace, owner, workflow, exclude, ManagedDraftPolicyVersion).Scan(&total, &day, &accountActive, &globalActive)
	if err != nil {
		return ErrUnavailable
	}
	p := selectedQualificationEnvelope()
	if workflow == "profile_ingestion" {
		p.Actions, p.RollingActions = 10, 3
	}
	if workflow == "profile_review" {
		r := selectedProfileReviewEnvelope()
		p.Actions, p.RollingActions = r.Actions, r.RollingActions
	}
	if workflow == "cv_generation" && h.cfg.StandaloneCV {
		r := selectedStandaloneCVEnvelope()
		p.Actions, p.RollingActions = r.Actions, r.RollingActions
	}
	if workflow == "application_draft" && h.cfg.ManagedDraft {
		p.Actions, p.RollingActions = managedDraftPolicy.Actions, managedDraftPolicy.RollingActions
	}
	if (newAction && (total >= p.Actions || day >= p.RollingActions)) || accountActive >= p.AccountActive || globalActive >= p.GlobalActive {
		return ErrCapacity
	}
	return nil
}

func (s *Session) qualificationLimits(ctx context.Context, tx *sql.Tx, leg Leg, m int64) error {
	var attempt, action, accountDay, globalDay int64
	var typesafe, openai, output int
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(sum(counted),0),count(*) FILTER(WHERE provider='typesafe'),count(*) FILTER(WHERE provider='openai'),COALESCE(sum(output_tokens) FILTER(WHERE provider='openai'),0) FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND attempt=$3`, s.h.cfg.Namespace, s.id, s.attempt).Scan(&attempt, &typesafe, &openai, &output)
	if err != nil {
		return ErrUnavailable
	}
	p := selectedQualificationEnvelope()
	if s.ingestion() {
		p.TypeSafeCalls, p.OpenAICalls, p.AttemptOutput, p.AttemptMoney, p.ActionMoney = 48, 48, 240000, 300000, 600000
		p.OpenAIOutput = 6000
	}
	if s.profileReview() {
		r := selectedProfileReviewEnvelope()
		p.TypeSafeCalls, p.OpenAICalls, p.OpenAIOutput, p.AttemptOutput, p.AttemptMoney, p.ActionMoney = 0, r.OpenAICalls, r.Output, r.OutputTotal, r.AttemptMoney, r.ActionMoney
	}
	if s.standaloneCV() {
		r := selectedStandaloneCVEnvelope()
		p.TypeSafeCalls, p.OpenAICalls, p.OpenAIOutput, p.AttemptOutput, p.AttemptMoney, p.ActionMoney = 0, r.OpenAICalls, r.Output, r.OutputTotal, r.AttemptMoney, r.ActionMoney
	}
	if s.managedDraft() {
		p.TypeSafeCalls, p.OpenAICalls = managedDraftPolicy.TypeSafeCalls, managedDraftPolicy.OpenAICalls
		p.OpenAIOutput, p.AttemptOutput = draftOutput(leg.Stage), managedDraftPolicy.OutputTotal
		p.AttemptMoney, p.ActionMoney = managedDraftPolicy.AttemptMoney, managedDraftPolicy.ActionMoney
	}
	if (leg.Provider == "typesafe" && typesafe >= p.TypeSafeCalls) || (leg.Provider == "openai" && (openai >= p.OpenAICalls || int64(output)+p.OpenAIOutput > p.AttemptOutput)) {
		return ErrCapacity
	}
	if tx.QueryRowContext(ctx, `SELECT COALESCE(sum(counted),0) FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2`, s.h.cfg.Namespace, s.id).Scan(&action) != nil {
		return ErrUnavailable
	}
	// Unresolved obligations stay in every rolling pool even after 24 hours.
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(sum(d.counted) FILTER(WHERE a.owner=$2),0),COALESCE(sum(d.counted),0) FROM ai_fixture_dispatch d JOIN ai_fixture_action a ON a.namespace=d.namespace AND a.id=d.action WHERE d.namespace=$1 AND (COALESCE(d.intent_at,a.created_at)>clock_timestamp()-interval '24 hours' OR d.remote_state<>'terminal_proven' OR d.expense IS NULL OR d.counted>d.expense)`, s.h.cfg.Namespace, s.p.AccountID).Scan(&accountDay, &globalDay)
	if err != nil {
		return ErrUnavailable
	}
	if attempt+m > p.AttemptMoney || action+m > p.ActionMoney || accountDay+m > p.AccountDay || globalDay+m > p.GlobalDay {
		return ErrCapacity
	}
	return nil
}

func (h *Harness) qualificationMaximum(provider string, raw []byte) (int64, error) {
	if h.cfg.TokenBound == nil {
		return 0, ErrDenied
	}
	p := selectedQualificationEnvelope()
	n, ok := h.cfg.TokenBound(provider, append([]byte(nil), raw...))
	if !ok || n < 0 {
		return 0, ErrDenied
	}
	if provider == "typesafe" {
		if n > p.TypeSafeInput {
			return 0, ErrCapacity
		}
		return 1344, nil
	}
	var body struct {
		Model, Tier, Reasoning string
		Output                 int64 `json:"max_completion_tokens"`
	}
	var fields map[string]json.RawMessage
	if provider != "openai" || json.Unmarshal(raw, &fields) != nil {
		return 0, ErrDenied
	}
	_ = json.Unmarshal(fields["model"], &body.Model)
	_ = json.Unmarshal(fields["service_tier"], &body.Tier)
	_ = json.Unmarshal(fields["reasoning_effort"], &body.Reasoning)
	_ = json.Unmarshal(fields["max_completion_tokens"], &body.Output)
	if body.Model != "gpt-6-luna" || body.Tier != "standard" || body.Reasoning != "none" || body.Output <= 0 || body.Output > p.OpenAIOutput {
		return 0, ErrDenied
	}
	if n > p.OpenAIInput {
		return 0, ErrCapacity
	}
	// Full selected bound; do not assume cache savings or partial output.
	return 12000, nil
}

// SetQualificationResponder accepts explicit synthetic terminal authority from
// an in-memory fake. An ordinary HTTP success/usage record never releases a slot.
// This helper does not exist in production composition or read provider keys.
func (h *Harness) SetQualificationResponder(fn func(context.Context, string, *http.Request) (*http.Response, bool, error)) error {
	if !h.cfg.Qualification || fn == nil {
		return ErrDenied
	}
	h.SetResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, error) {
		response, terminal, err := fn(ctx, p, r)
		if response != nil {
			response.Header = response.Header.Clone()
			if response.Header == nil {
				response.Header = make(http.Header)
			}
			response.Header.Del("X-CareerOS-Fake-Terminal")
			if terminal {
				response.Header.Set("X-CareerOS-Fake-Terminal", "1")
			}
		}
		return response, err
	})
	return nil
}

// FinishQualification records workflow completion only after all intended fake
// calls are authoritatively terminal. Local invocation leases independently hold
// capacity through handler return, including after content/customer terminality.
// Neither completion mechanism reconciles unknown remote work or expense.
func (s *Session) FinishQualification(ctx context.Context, knownFailure bool) error {
	return s.finishManaged(ctx, knownFailure, false)
}
func (s *Session) finishManaged(ctx context.Context, knownFailure, failedChild bool) error {
	if !s.h.cfg.Qualification {
		return ErrDenied
	}
	h := s.h.SyntheticReviewer().h
	tx, err := h.begin(ctx, s.p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.stageAction(ctx, tx); err != nil {
		return err
	}
	var invalid int
	if tx.QueryRowContext(ctx, `SELECT count(*) FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND (state<>'observed' OR remote_state<>'terminal_proven')`, h.cfg.Namespace, s.id).Scan(&invalid) != nil {
		return ErrUnavailable
	}
	if invalid > 0 {
		return ErrOutputUnavailable
	}
	state := "recoverable"
	if knownFailure {
		state = "failed"
	}
	current := *s
	// Finalization records already reached work, including an allowed
	// connected CV candidate after its inference deadline. It cannot send.
	current.foregroundIndependent = true
	if err = current.checkCurrentWithSource(ctx, tx, s.h.db); err != nil {
		return err
	}
	if err = accountstore.AuthorizeTransaction(ctx, tx, s.p); err != nil {
		return err
	}
	updated, err := tx.ExecContext(ctx, `UPDATE ai_fixture_action SET execution_finished=true,state=$3 WHERE namespace=$1 AND id=$2 AND owner=$4 AND state='recoverable' AND attempt=$5 AND NOT compacted`, h.cfg.Namespace, s.id, state, s.p.AccountID, s.attempt)
	if err != nil {
		return ErrUnavailable
	}
	if n, err := updated.RowsAffected(); err != nil || n != 1 {
		return ErrDenied
	}
	if failedChild {
		_, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_stage(namespace,action,coordinate,stage,attempt,binding) SELECT namespace,action,parent_coordinate||'/revision/'||id,'known_failed_clarification',attempt,binding FROM ai_fixture_child WHERE namespace=$1 AND action=$2 AND id=$3 AND attempt=$4 ON CONFLICT DO NOTHING`, h.cfg.Namespace, s.id, s.childID, s.attempt)
		if err != nil {
			return ErrUnavailable
		}
	}
	return safe(tx.Commit())
}

func (h *Harness) QualificationEnabled() bool { return h != nil && h.cfg.Qualification }

type QualificationStatus struct {
	ActionID        string `json:"actionId"`
	State           string `json:"state"`
	Attempt         int    `json:"attempt"`
	RetryAllowed    bool   `json:"retryAllowed"`
	ResultAvailable bool   `json:"resultAvailable"`          // content is deliberately transient
	ReviewRequired  bool   `json:"reviewRequired,omitempty"` // noncontent CV outcome; never an acceptance claim
}

func (h *Harness) QualificationStatus(ctx context.Context, p auth.Principal, key string) (QualificationStatus, error) {
	var out QualificationStatus
	if !h.QualificationEnabled() || !tokenLabel.MatchString(key) {
		return out, ErrDenied
	}
	tx, err := h.begin(ctx, p)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var state string
	var finished bool
	var reached, unknown int
	if tx.QueryRowContext(ctx, `SELECT id,state,attempt,execution_finished FROM ai_fixture_action WHERE namespace=$1 AND owner=$2 AND creation_key=$3`, h.cfg.Namespace, p.AccountID, key).Scan(&out.ActionID, &state, &out.Attempt, &finished) != nil {
		return out, ErrDenied
	}
	if tx.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE state IN ('intent','observed') AND remote_state<>'terminal_proven') FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND attempt=$3`, h.cfg.Namespace, out.ActionID, out.Attempt).Scan(&reached, &unknown) != nil {
		return out, ErrUnavailable
	}
	out.State = "ready"
	if reached > 0 {
		out.State = "incomplete"
	}
	if unknown > 0 {
		out.State = "unknown"
	}
	if finished {
		out.State = "result_unavailable"
	}
	if state == "failed" {
		out.State = "known_failure"
		out.RetryAllowed = out.Attempt < 2
	}
	return out, safe(tx.Commit())
}

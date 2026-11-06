package aicontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/cvsource"
	"time"
)

const StandaloneCVPolicyVersion = "standalone-cv-local-20261105-v1"
const StandaloneCVAttemptMaximum int64 = 20000

func selectedStandaloneCVEnvelope() profileReviewEnvelope {
	r := selectedQualificationEnvelope()
	return profileReviewEnvelope{8, 2, 2, 2, 64000, 128000, 4000, 8000, 25000, 50000, r.OrdinaryRate, r.CachedRate, r.CacheWriteRate, r.OutputRate}
}
func StandaloneCVSyntheticConfig(namespace string, gate *SyntheticGate, bound func(string, []byte) (int64, bool)) SyntheticConfig {
	c := ProfileReviewSyntheticConfig(namespace, gate, bound)
	c.StandaloneCV = true
	return c
}
func (h *Harness) StandaloneCVEnabled() bool {
	return h != nil && h.cfg.Qualification && h.cfg.StandaloneCV
}
func (s *Session) standaloneCV() bool {
	return s.h.StandaloneCVEnabled() && s.workflow == "cv_generation"
}

func (h *Harness) cvSavedSnapshot(ctx context.Context, tx *sql.Tx, owner string, root []byte) error {
	var saved []byte
	if tx.QueryRowContext(ctx, `SELECT document FROM careeros_profiles WHERE account_id=$1`, owner).Scan(&saved) != nil {
		return ErrDenied
	}
	if !cvsource.Matches(root, saved) {
		return ErrConflict
	}
	return nil
}
func (h *Harness) cvMaximum(provider string, raw []byte) (int64, int64, error) {
	if provider != "openai" || h.cfg.TokenBound == nil {
		return 0, 0, ErrDenied
	}
	n, ok := h.cfg.TokenBound(provider, append([]byte(nil), raw...))
	if !ok || n < 0 {
		return 0, 0, ErrDenied
	}
	var v struct {
		Model     string `json:"model"`
		Tier      string `json:"service_tier"`
		Reasoning string `json:"reasoning_effort"`
		Output    int64  `json:"max_completion_tokens"`
	}
	p := selectedStandaloneCVEnvelope()
	if json.Unmarshal(raw, &v) != nil || v.Model != "gpt-6-luna" || v.Tier != "standard" || v.Reasoning != "none" || v.Output != p.Output {
		return 0, 0, ErrDenied
	}
	if n > p.Input {
		return 0, 0, ErrCapacity
	}
	return (n*p.CacheWriteRate + v.Output*p.OutputRate + 999) / 1000, n, nil
}
func (s *Session) cvLimits(ctx context.Context, tx *sql.Tx, leg Leg, input, output int64) error {
	p := selectedStandaloneCVEnvelope()
	var calls int
	var totalInput, totalOutput int64
	if leg.Provider != "openai" {
		return ErrDenied
	}
	if tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(input_tokens),0),COALESCE(sum(output_tokens),0) FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND attempt=$3`, s.h.cfg.Namespace, s.id, s.attempt).Scan(&calls, &totalInput, &totalOutput) != nil {
		return ErrUnavailable
	}
	if calls >= p.OpenAICalls || totalInput+input > p.InputTotal || totalOutput+output > p.OutputTotal {
		return ErrCapacity
	}
	return nil
}
func (s *Session) BeginStandaloneCV(ctx context.Context) error {
	if !s.standaloneCV() {
		return ErrDenied
	}
	tx, e := s.h.begin(ctx, s.p)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = s.stageAction(ctx, tx); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO ai_fixture_stage(namespace,action,coordinate,stage,attempt,binding) VALUES($1,$2,'request','managed_request_cv',$3,$4) ON CONFLICT DO NOTHING`, s.h.cfg.Namespace, s.id, s.attempt, s.root); e != nil {
		return ErrUnavailable
	}
	if e = s.checkCurrent(ctx, tx); e != nil {
		return e
	}
	return safe(tx.Commit())
}
func (s *Session) cvDeadline(ctx context.Context, tx *sql.Tx) error {
	var valid bool
	if tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_fixture_stage WHERE namespace=$1 AND action=$2 AND attempt=$3 AND stage='managed_request_cv' AND created_at+interval '25 seconds'>clock_timestamp())`, s.h.cfg.Namespace, s.id, s.attempt).Scan(&valid) != nil {
		return ErrUnavailable
	}
	if !valid {
		return ErrCapacity
	}
	return nil
}

// StandaloneCVContext bounds actual handler/provider work by the durable
// attempt witness. A replay cannot borrow a fresh handler-relative 25 seconds.
// Subtracting the complete local query round trip is conservative even when
// the database and process wall clocks differ. Final candidate return uses the
// original request context and its separate current-authority result fence.
func (s *Session) StandaloneCVContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if !s.standaloneCV() {
		return nil, nil, ErrDenied
	}
	started := time.Now()
	var remainingMicros int64
	if s.h.db.QueryRowContext(ctx, `SELECT (extract(epoch FROM (created_at+interval '25 seconds'-clock_timestamp()))*1000000)::bigint FROM ai_fixture_stage WHERE namespace=$1 AND action=$2 AND attempt=$3 AND stage='managed_request_cv'`, s.h.cfg.Namespace, s.id, s.attempt).Scan(&remainingMicros) != nil {
		return nil, nil, ErrUnavailable
	}
	if remainingMicros > int64((25*time.Second)/time.Microsecond) {
		return nil, nil, ErrDenied
	}
	remaining := time.Duration(remainingMicros)*time.Microsecond - time.Since(started)
	if remaining <= 0 {
		return nil, nil, ErrCapacity
	}
	bounded, cancel := context.WithTimeout(ctx, remaining)
	return bounded, cancel, nil
}

// An unresolved assessment cannot be laundered into a favorable new generation
// by changing only a creation key. Only keyed root/policy metadata is retained.
func (h *Harness) cvReviewBlocked(ctx context.Context, tx *sql.Tx, owner string, binding, execution, profile []byte) error {
	var blocked bool
	if tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_fixture_action a JOIN ai_fixture_stage s ON s.namespace=a.namespace AND s.action=a.id WHERE a.namespace=$1 AND a.owner=$2 AND a.workflow='cv_generation' AND a.binding=$3 AND a.execution_binding=$4 AND a.profile_binding=$5 AND s.stage='cv_review_required')`, h.cfg.Namespace, owner, binding, execution, profile).Scan(&blocked) != nil {
		return ErrUnavailable
	}
	if blocked {
		return ErrDenied
	}
	return nil
}

func (s *Session) FinishStandaloneCV(ctx context.Context, candidate, unresolved, failed bool) error {
	if !s.standaloneCV() {
		return ErrDenied
	}
	// Candidate presence is supplied only by the actual handler's validated v3
	// response, not by a caller flag. Keep the latch even if remote work is unknown.
	if candidate && unresolved {
		tx, e := s.h.begin(ctx, s.p)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if e = s.stageAction(ctx, tx); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO ai_fixture_stage(namespace,action,coordinate,stage,attempt,binding) VALUES($1,$2,'request','cv_review_required',$3,$4) ON CONFLICT DO NOTHING`, s.h.cfg.Namespace, s.id, s.attempt, s.root); e != nil {
			return ErrUnavailable
		}
		if e = tx.Commit(); e != nil {
			return ErrUnavailable
		}
	}
	s.cvCandidateResult = candidate
	return s.FinishQualification(ctx, !candidate && failed)
}
func (h *Harness) StandaloneCVStatus(ctx context.Context, p auth.Principal, key string) (QualificationStatus, error) {
	if !h.StandaloneCVEnabled() {
		return QualificationStatus{}, ErrDenied
	}
	out, e := h.QualificationStatus(ctx, p, key)
	if e != nil {
		return out, e
	}
	var workflow string
	if h.db.QueryRowContext(ctx, `SELECT workflow FROM ai_fixture_action WHERE namespace=$1 AND owner=$2 AND creation_key=$3`, h.cfg.Namespace, p.AccountID, key).Scan(&workflow) != nil || workflow != "cv_generation" {
		return QualificationStatus{}, ErrDenied
	}
	if h.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_fixture_stage WHERE namespace=$1 AND action=$2 AND stage='cv_review_required')`, h.cfg.Namespace, out.ActionID).Scan(&out.ReviewRequired) != nil {
		return out, ErrUnavailable
	}
	return out, nil
}

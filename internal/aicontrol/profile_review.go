package aicontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	profilereview "professional-information-repo/backend/profile-review"
	"professional-information-repo/internal/auth"
)

const ProfileReviewPolicyVersion = "profile-review-local-20261104-v1"
const ProfileReviewAttemptMaximum int64 = 22000

type profileReviewEnvelope struct {
	Actions, RollingActions, Attempts, OpenAICalls                    int
	Input, InputTotal, Output, OutputTotal, AttemptMoney, ActionMoney int64
	OrdinaryRate, CachedRate, CacheWriteRate, OutputRate              int64
}

func selectedProfileReviewEnvelope() profileReviewEnvelope {
	r := selectedQualificationEnvelope()
	return profileReviewEnvelope{30, 6, 2, 2, 64000, 128000, 6000, 12000, 30000, 60000, r.OrdinaryRate, r.CachedRate, r.CacheWriteRate, r.OutputRate}
}

// ProfileReviewSyntheticConfig adds the selected review capability to the SAME
// namespace. Existing qualification and ingestion execution identities survive.
// No environment or production composition selects this fake-only constructor.
func ProfileReviewSyntheticConfig(namespace string, gate *SyntheticGate, bound func(string, []byte) (int64, bool)) SyntheticConfig {
	c := IngestionSyntheticConfig(namespace, gate, bound)
	c.ProfileReview = true
	return c
}
func (h *Harness) ProfileReviewEnabled() bool {
	return h != nil && h.cfg.Qualification && h.cfg.ProfileReview
}
func (s *Session) profileReview() bool {
	return s.h.ProfileReviewEnabled() && s.workflow == "profile_review"
}
func (h *Harness) reviewSavedSnapshot(ctx context.Context, tx *sql.Tx, owner string, root []byte) error {
	var saved []byte
	if tx.QueryRowContext(ctx, `SELECT document FROM careeros_profiles WHERE account_id=$1`, owner).Scan(&saved) != nil {
		return ErrDenied
	}
	if !profilereview.MatchesSavedRequest(root, saved) {
		return ErrConflict
	}
	return nil
}
func (h *Harness) reviewMaximum(provider string, raw []byte) (int64, int64, error) {
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
	p := selectedProfileReviewEnvelope()
	if json.Unmarshal(raw, &v) != nil || v.Model != "gpt-6-luna" || v.Tier != "standard" || v.Reasoning != "none" || v.Output != p.Output {
		return 0, 0, ErrDenied
	}
	if n > p.Input {
		return 0, 0, ErrCapacity
	}
	m := (n*p.CacheWriteRate + v.Output*p.OutputRate + 999) / 1000
	return m, n, nil
}
func (s *Session) reviewLimits(ctx context.Context, tx *sql.Tx, leg Leg, input, output int64) error {
	p := selectedProfileReviewEnvelope()
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

// The persisted invocation witness precedes provider work, so replay after a
// crash with zero dispatches cannot reset the selected 25-second attempt budget.
func (s *Session) BeginProfileReview(ctx context.Context) error {
	if !s.profileReview() {
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
	if _, e = tx.ExecContext(ctx, `INSERT INTO ai_fixture_stage(namespace,action,coordinate,stage,attempt,binding) VALUES($1,$2,'request','managed_request_review',$3,$4) ON CONFLICT DO NOTHING`, s.h.cfg.Namespace, s.id, s.attempt, s.root); e != nil {
		return ErrUnavailable
	}
	if e = s.checkCurrent(ctx, tx); e != nil {
		return e
	}
	return safe(tx.Commit())
}
func (s *Session) reviewDeadline(ctx context.Context, tx *sql.Tx) error {
	var valid bool
	if tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_fixture_stage WHERE namespace=$1 AND action=$2 AND attempt=$3 AND stage='managed_request_review' AND created_at+interval '25 seconds'>clock_timestamp())`, s.h.cfg.Namespace, s.id, s.attempt).Scan(&valid) != nil {
		return ErrUnavailable
	}
	if !valid {
		return ErrCapacity
	}
	return nil
}
func (h *Harness) ProfileReviewStatus(ctx context.Context, p auth.Principal, key string) (QualificationStatus, error) {
	if !h.ProfileReviewEnabled() {
		return QualificationStatus{}, ErrDenied
	}
	out, e := h.QualificationStatus(ctx, p, key)
	if e != nil {
		return out, e
	}
	var workflow string
	if h.db.QueryRowContext(ctx, `SELECT workflow FROM ai_fixture_action WHERE namespace=$1 AND owner=$2 AND creation_key=$3`, h.cfg.Namespace, p.AccountID, key).Scan(&workflow) != nil || workflow != "profile_review" {
		return QualificationStatus{}, ErrDenied
	}
	return out, nil
}

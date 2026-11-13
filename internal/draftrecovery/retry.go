package draftrecovery

import (
	"context"
	"database/sql"
	"encoding/json"

	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
)

type draftRetry struct {
	record
	kernelState string
	retryKey    string
	source      Generation
}

// RetryManaged uses two durable witnesses. The shared ledger allocates attempt
// two with its original retry key and foreground clock. Only one purpose-row
// transition hands that attempt to generation, even across the commit gap.
func (s *Store) RetryManaged(ctx context.Context, p auth.Principal, key, retryKey string, input Input, rawContext json.RawMessage, proof string) (Generation, bool, error) {
	var empty Generation
	if !s.managed || !validToken(key) || !validToken(retryKey) {
		return empty, false, ErrInvalid
	}
	reuse, err := s.prepareDraftReuse(&input, rawContext, proof)
	if err != nil {
		return empty, false, err
	}
	in, raw, err := normalize(input, true)
	if err != nil {
		return empty, false, err
	}
	digest := s.digest("input", raw)
	tx, err := s.begin(ctx, p)
	if err != nil {
		return empty, false, err
	}
	defer tx.Rollback()
	r, err := s.draftRetryState(ctx, tx, p, key, in, digest, reuse)
	if err != nil {
		return empty, false, err
	}
	if r.Attempt == 2 && r.retryKey != retryKey {
		return empty, false, ErrConflict
	}
	if r.generationAttempt == 2 {
		return empty, false, tx.Commit()
	}
	if r.State != "incomplete" || !(r.Attempt == 1 && r.kernelState == "failed" ||
		r.Attempt == 2 && r.kernelState == "recoverable") {
		return empty, false, ErrDenied
	}
	if err = tx.Commit(); err != nil {
		return empty, false, ErrDenied
	}
	session, err := s.cfg.Actions.Open(ctx, p, key, "application_draft", raw)
	if err != nil {
		return empty, false, err
	}
	session, err = session.RetryQualification(ctx, retryKey)
	if err != nil {
		return empty, false, err
	}
	if session.ID() != r.Action.ID || session.DraftAttempt() != 2 {
		return empty, false, ErrDenied
	}
	if err = s.fault("after_shared_retry_commit"); err != nil {
		return empty, false, err
	}
	return s.claimDraftRetry(ctx, p, key, retryKey, in, digest, reuse, session)
}

func (s *Store) draftRetryState(ctx context.Context, tx *sql.Tx, p auth.Principal, key string, in Input, digest string, reuse draftReuse) (draftRetry, error) {
	r, d, _, err := s.load(ctx, tx, p, key)
	result := draftRetry{record: r}
	if err != nil {
		return result, err
	}
	if r.State == "closed" || r.State == "deleted" {
		// The caller rolls back on error. The protected read path commits any
		// terminal closure before reporting it.
		return result, ErrDenied
	}
	if !d.Generate || r.generationAttempt == 0 || r.binding.GenerationVersion != aicontrol.ManagedDraftPolicyVersion {
		return result, ErrDenied
	}
	if r.inputDigest != digest {
		return result, ErrConflict
	}
	if err = s.authorizeDraftReuse(ctx, tx, p, in.Job.Posting, reuse); err != nil {
		return result, err
	}
	g, err := s.current(ctx, tx, p, in)
	if err != nil || !r.sourceMatches(g) {
		return result, ErrConflict
	}
	if tx.QueryRowContext(ctx, `SELECT state,retry_creation_key FROM ai_fixture_action WHERE namespace=$1 AND id=$2 AND owner=$3`, r.Action.Namespace, r.Action.ID, p.AccountID).Scan(&result.kernelState, &result.retryKey) != nil {
		return result, ErrDenied
	}
	result.source = g
	return result, nil
}

func (s *Store) claimDraftRetry(ctx context.Context, p auth.Principal, key, retryKey string, in Input, digest string, reuse draftReuse, session *aicontrol.Session) (Generation, bool, error) {
	var empty Generation
	tx, err := s.begin(ctx, p)
	if err != nil {
		return empty, false, err
	}
	defer tx.Rollback()
	r, err := s.draftRetryState(ctx, tx, p, key, in, digest, reuse)
	if err != nil {
		return empty, false, err
	}
	if r.Attempt != 2 || r.retryKey != retryKey || r.Action.ID != session.ID() {
		return empty, false, ErrDenied
	}
	if r.generationAttempt == 2 {
		return empty, false, tx.Commit()
	}
	if r.State != "incomplete" || r.kernelState != "recoverable" {
		return empty, false, ErrDenied
	}
	if err = session.AuthorizeDraftAttempt(ctx, tx); err != nil {
		return empty, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE careeros_draft_actions SET generation_attempt=2,state='pending' WHERE namespace=$1 AND action_id=$2 AND generation_attempt=1 AND state='incomplete'`, r.Action.Namespace, r.Action.ID); err != nil {
		return empty, false, ErrDenied
	}
	if err = tx.Commit(); err != nil {
		return empty, false, ErrDenied
	}
	g := r.source
	g.Binding, g.session, g.jobContext = r.binding, session, reuse.raw
	return g, true, nil
}

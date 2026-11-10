package draftrecovery

import (
	"context"
	"database/sql"
	"errors"
	"professional-information-repo/internal/auth"
	"time"
)

// The guard contains keyed bindings and authority metadata only. It precedes the
// restricted ledger commit, so a crash between commits cannot select a new epoch.
func (s *Store) admission(ctx context.Context, tx *sql.Tx, p auth.Principal, key, digest string, in Input, g Generation, d Decision) (record, error) {
	var r record
	err := tx.QueryRowContext(ctx, `SELECT namespace,input_digest,source_digest,profile_id,profile_revision,language,authority_epoch,grant_expires_at,state FROM careeros_draft_admissions WHERE owner=$1 AND creation_key=$2 FOR UPDATE`, p.AccountID, key).Scan(&r.Action.Namespace, &r.inputDigest, &r.binding.SourceDigest, &r.binding.Profile.ID, &r.binding.Profile.Revision, &r.binding.Language, &r.epoch, &r.grantEnd, &r.State)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `INSERT INTO careeros_draft_admissions(namespace,owner,creation_key,input_digest,source_digest,profile_id,profile_revision,language,authority_epoch,grant_expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, s.cfg.Actions.Namespace(), p.AccountID, key, digest, g.Binding.SourceDigest, in.Profile.ID, in.Profile.Revision, in.Language, d.Epoch, d.GrantExpiresAt)
		r.Action.Namespace = s.cfg.Actions.Namespace()
		r.inputDigest = digest
		r.binding = g.Binding
		r.epoch = d.Epoch
		r.grantEnd = d.GrantExpiresAt
		r.State = "pending"
	}
	if err != nil {
		return r, ErrDenied
	}
	if r.Action.Namespace != s.cfg.Actions.Namespace() || r.inputDigest != digest || !r.sourceMatches(g) {
		return r, ErrConflict
	}
	return r, nil
}
func guardClosed(r record, d Decision, now time.Time) bool {
	return r.State == "closed" || r.State == "deleted" || d.Epoch != r.epoch || !d.Generate || d.Deleted || d.Revoked || d.Cancelled || !now.Before(r.grantEnd) || !now.Before(d.GrantExpiresAt)
}
func (s *Store) closeAdmission(ctx context.Context, tx *sql.Tx, p auth.Principal, key string, d Decision) error {
	state := "closed"
	if d.Deleted {
		state = "deleted"
	}
	_, err := tx.ExecContext(ctx, `UPDATE careeros_draft_admissions SET state=CASE WHEN state='deleted' THEN state ELSE $3 END WHERE owner=$1 AND creation_key=$2`, p.AccountID, key, state)
	return err
}

// gapRecord recovers only non-content status for an admitted Action whose
// generation witness has not committed. It NEVER claims generation or checking.
func (s *Store) gapRecord(ctx context.Context, tx *sql.Tx, p auth.Principal, key string) (record, error) {
	var r record
	var sharedState string
	err := tx.QueryRowContext(ctx, `SELECT a.namespace,a.id,g.input_digest,g.source_digest,g.profile_id,g.profile_revision,g.language,g.authority_epoch,a.created_at,g.grant_expires_at,g.state,a.state FROM careeros_draft_admissions g JOIN ai_fixture_action a ON a.namespace=g.namespace AND a.owner=g.owner AND a.creation_key=g.creation_key AND a.workflow='application_draft' WHERE g.owner=$1 AND g.creation_key=$2 FOR UPDATE OF g,a`, p.AccountID, key).Scan(&r.Action.Namespace, &r.Action.ID, &r.inputDigest, &r.binding.SourceDigest, &r.binding.Profile.ID, &r.binding.Profile.Revision, &r.binding.Language, &r.epoch, &r.AdmittedAt, &r.grantEnd, &r.State, &sharedState)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, ErrDenied
	}
	if sharedState == "closed" || sharedState == "canceled" || sharedState == "deleted" {
		r.State = "closed"
		if sharedState == "deleted" {
			r.State = "deleted"
		}
	}
	r.Action.Owner = p.AccountID
	r.Action.Workflow = "application_draft"
	r.CreationKey = key
	r.Deadline = r.AdmittedAt.Add(CustomerWindow)
	if r.grantEnd.Before(r.Deadline) {
		r.Deadline = r.grantEnd
	}
	r.binding.Action = r.Action
	r.binding.InputDigest = r.inputDigest
	r.binding.GenerationVersion = GenerationVersion
	r.binding.ReviewVersion = ReviewVersion
	return r, nil
}

package aicontrol

import (
	"context"
	"database/sql"
	"time"

	"professional-information-repo/internal/auth"
)

// SeedGrant is explicitly synthetic fixture authority. No production grant,
// payment, invitation, environment or HTTP route calls it. Reseeding never
// resets rights or revives a revoked/expired grant.
func (r *Reviewer) SeedGrant(ctx context.Context, p auth.Principal, expires time.Time, units int64) error {
	if r == nil || !r.authorized || r.h == nil || units <= 0 || units > 1000000 {
		return ErrDenied
	}
	tx, e := r.h.begin(ctx, p)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var valid bool
	if tx.QueryRowContext(ctx, `SELECT $1::timestamptz>clock_timestamp()`, expires).Scan(&valid) != nil || !valid {
		return ErrDenied
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO ai_fixture_grant(namespace,owner,expires_at,units) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, r.h.cfg.Namespace, p.AccountID, expires, units)
	if e != nil {
		return ErrUnavailable
	}
	return safe(tx.Commit())
}

func (h *Harness) checkGrant(ctx context.Context, tx *sql.Tx, p auth.Principal, expected int64) (int64, error) {
	var version int64
	var valid bool
	e := tx.QueryRowContext(ctx, `SELECT version,NOT revoked AND expires_at>clock_timestamp() FROM ai_fixture_grant WHERE namespace=$1 AND owner=$2 FOR UPDATE`, h.cfg.Namespace, p.AccountID).Scan(&version, &valid)
	if e == sql.ErrNoRows {
		return 0, ErrDenied
	}
	if e != nil {
		return 0, ErrUnavailable
	}
	if !valid || expected > 0 && expected != version {
		return 0, ErrDenied
	}
	return version, nil
}

// SetGrantState is a reviewed local fixture event, independent of session and
// global policy expiry. Version advancement permanently fences prior bindings.
func (r *Reviewer) SetGrantState(ctx context.Context, p auth.Principal, expires time.Time, revoked bool) error {
	if r == nil || !r.authorized || r.h == nil {
		return ErrDenied
	}
	tx, e := r.h.begin(ctx, p)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	result, e := tx.ExecContext(ctx, `UPDATE ai_fixture_grant SET expires_at=$3,revoked=$4,version=version+1 WHERE namespace=$1 AND owner=$2 AND version<9223372036854775807`, r.h.cfg.Namespace, p.AccountID, expires, revoked)
	if e != nil {
		return ErrUnavailable
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrDenied
	}
	return safe(tx.Commit())
}

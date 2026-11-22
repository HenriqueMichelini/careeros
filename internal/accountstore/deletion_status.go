package accountstore

import (
	"context"
	"database/sql"
	"errors"
	"professional-information-repo/internal/auth"
	"time"
)

func (s sessions) CreateClosedStatus(ctx context.Context, v auth.Session) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT status='deleted' AND generation=$2 AND EXISTS(SELECT 1 FROM careeros_deletion_operations d WHERE d.account_id=a.id AND d.kind='account') FROM careeros_accounts a WHERE id=$1 FOR UPDATE`, v.AccountID, v.Generation).Scan(&valid)
	if err != nil || !valid {
		return auth.ErrForbidden
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM careeros_deletion_status_sessions WHERE account_id=$1 OR expires_at<=clock_timestamp()`, v.AccountID)
	if err != nil {
		return unavailable(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO careeros_deletion_status_sessions VALUES($1,$2,$3,$4,$5,$6,$7)`, v.ID, v.AccountID, v.TokenHash[:], v.CSRFHash[:], v.Generation, v.CreatedAt, v.ExpiresAt)
	if err != nil {
		return unavailable(err)
	}
	return unavailable(tx.Commit())
}
func (s sessions) FindClosedStatusByTokenHash(ctx context.Context, hash [32]byte, _ time.Time) (auth.Session, bool, error) {
	v, err := scanSession(s.db.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM careeros_deletion_status_sessions WHERE token_hash=$1 AND expires_at>clock_timestamp()`, hash[:]))
	if errors.Is(err, sql.ErrNoRows) {
		return v, false, nil
	}
	return v, err == nil, unavailable(err)
}
func (s sessions) RevokeClosedStatus(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM careeros_deletion_status_sessions WHERE id=$1`, id)
	return unavailable(err)
}

func authorizeDeletionStatus(ctx context.Context, tx *sql.Tx, p auth.Principal) error {
	var state string
	var generation uint64
	if err := tx.QueryRowContext(ctx, `SELECT status,generation FROM careeros_accounts WHERE id=$1 FOR UPDATE`, p.AccountID).Scan(&state, &generation); err != nil {
		return auth.ErrAuthenticationRequired
	}
	if state == "active" {
		return authorizedAccount(ctx, tx, p)
	}
	if state != "deleted" || generation != p.Generation {
		return auth.ErrAuthenticationRequired
	}
	v, err := scanSession(tx.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM careeros_deletion_status_sessions WHERE id=$1 AND account_id=$2 AND expires_at>clock_timestamp() FOR UPDATE`, p.SessionID, p.AccountID))
	if err != nil || !p.MatchesSession(v) {
		return auth.ErrAuthenticationRequired
	}
	return nil
}

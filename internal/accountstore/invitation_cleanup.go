package accountstore

import (
	"context"
	"database/sql"
	"time"
)

func recordInvitationCleanup(ctx context.Context, tx *sql.Tx, owner, id string, expiry, fenced time.Time, cleaned *time.Time) error {
	if expiry.Before(fenced) {
		fenced = expiry
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO careeros_invitation_cleanup VALUES($1,$2,$3,$4,$5) ON CONFLICT(account_id,invitation_id) DO UPDATE SET fenced_at=LEAST(careeros_invitation_cleanup.fenced_at,EXCLUDED.fenced_at),cleaned_at=CASE WHEN careeros_invitation_cleanup.cleaned_at IS NULL THEN EXCLUDED.cleaned_at WHEN EXCLUDED.cleaned_at IS NULL THEN careeros_invitation_cleanup.cleaned_at ELSE LEAST(careeros_invitation_cleanup.cleaned_at,EXCLUDED.cleaned_at) END WHERE careeros_invitation_cleanup.expires_at=EXCLUDED.expires_at`, owner, id, expiry, fenced, cleaned)
	if err != nil {
		return unavailable(err)
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	return nil
}

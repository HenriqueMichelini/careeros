package accountstore

import (
	"context"
	"database/sql"
	"professional-information-repo/internal/auth"
)

type RestrictedRightsStatus struct {
	AccountID          string `json:"accountId"`
	State              string `json:"state"`
	CanRequestDeletion bool   `json:"canRequestDeletion"`
}

func authorizeRestrictedRights(ctx context.Context, tx *sql.Tx, p auth.Principal) error {
	var state string
	var generation uint64
	if err := tx.QueryRowContext(ctx, `SELECT status,generation FROM careeros_accounts WHERE id=$1 FOR UPDATE`, p.AccountID).Scan(&state, &generation); err != nil {
		return auth.ErrAuthenticationRequired
	}
	if state != "active" || generation != p.Generation {
		return auth.ErrAuthenticationRequired
	}
	var denied bool
	if err := tx.QueryRowContext(ctx, `SELECT careeros_control_account_denied($1)`, p.AccountID).Scan(&denied); err != nil {
		return unavailable(err)
	}
	if !denied {
		return auth.ErrForbidden
	}
	v, err := scanSession(tx.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM careeros_deletion_status_sessions WHERE id=$1 AND account_id=$2 FOR UPDATE`, p.SessionID, p.AccountID))
	if err != nil || !p.MatchesSession(v) {
		return auth.ErrAuthenticationRequired
	}
	now, err := pilotNow(ctx, tx)
	if err != nil {
		return err
	}
	if !now.Before(v.ExpiresAt) {
		return auth.ErrAuthenticationRequired
	}
	return nil
}

func (s *Store) WithRestrictedRightsStatus(ctx context.Context, p auth.Principal, write func(RestrictedRightsStatus) error) error {
	if write == nil {
		return ErrValidation
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if err = authorizeRestrictedStatus(ctx, tx, p); err != nil {
		return err
	}
	var state string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM careeros_accounts WHERE id=$1`, p.AccountID).Scan(&state); err != nil {
		return unavailable(err)
	}
	result := RestrictedRightsStatus{AccountID: p.AccountID, State: "closed"}
	if state == "active" {
		result.State = "restricted"
		result.CanRequestDeletion = true
	}
	if err = authorizeRestrictedStatus(ctx, tx, p); err != nil {
		return err
	}
	return write(result)
}

func authorizeRestrictedStatus(ctx context.Context, tx *sql.Tx, p auth.Principal) error {
	var state string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM careeros_accounts WHERE id=$1 FOR UPDATE`, p.AccountID).Scan(&state); err != nil {
		return auth.ErrAuthenticationRequired
	}
	if state == "active" {
		var denied bool
		if err := tx.QueryRowContext(ctx, `SELECT careeros_control_account_denied($1)`, p.AccountID).Scan(&denied); err != nil {
			return unavailable(err)
		}
		if !denied {
			return auth.ErrForbidden
		}
		return authorizeRestrictedRights(ctx, tx, p)
	}
	return authorizeDeletionStatus(ctx, tx, p)
}

// StartRestrictedDeletion accepts only explicit full-account deletion under a
// fresh restricted session. It cannot read content or request grant admission.
func (s *Store) StartRestrictedDeletion(ctx context.Context, p auth.Principal, in DeletionRequest) (DeletionStatus, error) {
	if in.Kind != "account" {
		return DeletionStatus{}, auth.ErrForbidden
	}
	return s.startDeletion(ctx, p, in, true)
}

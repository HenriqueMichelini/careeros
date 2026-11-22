package accountstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"professional-information-repo/internal/auth"
)

// PilotStatus contains no eligibility evidence, operator identity or provider subject.
// Customer content expiry and financial reconciliation are distinct clocks.
type PilotStatus struct {
	AccountID           string     `json:"accountId"`
	State               string     `json:"state"`
	InvitationID        string     `json:"invitationId,omitempty"`
	InvitationExpiresAt *time.Time `json:"invitationExpiresAt,omitempty"`
	ExpiresAt           *time.Time `json:"expiresAt,omitempty"`
	RetrieveUntil       *time.Time `json:"retrieveUntil,omitempty"`
	CanActivate         bool       `json:"canActivate"`
	CanDraft            bool       `json:"canDraft"`
	CanStandalone       bool       `json:"canStandalone"`
	CanRetrieve         bool       `json:"canRetrieve"`
	DraftAllowance      int        `json:"draftAllowance"`
}

// PilotCommand is a trusted operator decision, never an age checking method.
// Only the 18+ result, deciding operator and DB check time are retained.
// There is deliberately no proof text, birthdate, email or document field.
type PilotCommand struct {
	Kind     string           `json:"kind"`
	Identity auth.IdentityRef `json:"identity"`
}

type pilotAccess struct {
	allowance                                     int
	invitation                                    sql.NullString
	invited, invitationExpiry, activated, expires sql.NullTime
	revoked                                       bool
	eligible                                      sql.NullBool
	state                                         string
	accessEnded                                   sql.NullTime
	retrievalHeld                                 bool
}

func lockPilot(ctx context.Context, tx *sql.Tx) error {
	// Five participants: a single short authority lock makes slot assignment,
	// command replay, suspension and redemption share one linearization order.
	_, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(72481306)")
	return unavailable(err)
}

func (s *Store) pilotTransaction(ctx context.Context, p auth.Principal, target string) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, unavailable(err)
	}
	if err = lockPilot(ctx, tx); err != nil {
		tx.Rollback()
		return nil, err
	}
	// Same sorted account order as session replacement, including operator commands.
	rows, err := tx.QueryContext(ctx, "SELECT id FROM careeros_accounts WHERE id=$1 OR id=$2 ORDER BY id FOR UPDATE", p.AccountID, target)
	if err == nil {
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				break
			}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
	}
	if err != nil {
		tx.Rollback()
		return nil, unavailable(err)
	}
	if err = authorized(ctx, tx, p); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func pilotNow(ctx context.Context, tx *sql.Tx) (time.Time, error) {
	var now time.Time
	err := tx.QueryRowContext(ctx, "SELECT clock_timestamp()").Scan(&now)
	return now, unavailable(err)
}
func readPilot(ctx context.Context, tx *sql.Tx, owner string) (pilotAccess, error) {
	var a pilotAccess
	err := tx.QueryRowContext(ctx, `SELECT invitation_id,invited_at,invitation_expires_at,invitation_revoked,age_eligible,activated_at,expires_at,state,draft_allowance,access_ended_at,retrieval_held FROM careeros_pilot_access WHERE account_id=$1`, owner).Scan(&a.invitation, &a.invited, &a.invitationExpiry, &a.revoked, &a.eligible, &a.activated, &a.expires, &a.state, &a.allowance, &a.accessEnded, &a.retrievalHeld)
	if err == nil && a.invitation.Valid {
		var denied bool
		if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_invitation_cleanup WHERE account_id=$1 AND invitation_id=$2)`, owner, a.invitation.String).Scan(&denied); e != nil {
			return a, e
		}
		if denied {
			a.revoked = true
			if a.activated.Valid {
				a.state = "revoked"
				a.retrievalHeld = true
				a.accessEnded = sql.NullTime{}
			}
		}
	}
	return a, err
}
func pilotStatus(owner string, a pilotAccess, now time.Time) PilotStatus {
	s := PilotStatus{AccountID: owner, State: "uninvited", DraftAllowance: 5}
	if a.state == "" {
		return s
	}
	s.State = a.state
	s.DraftAllowance = a.allowance
	if a.invitation.Valid {
		s.InvitationID = a.invitation.String
	}
	if a.invitationExpiry.Valid {
		s.InvitationExpiresAt = &a.invitationExpiry.Time
	}
	if !a.activated.Valid {
		if a.revoked {
			s.State = "invitation_revoked"
		} else if a.invitationExpiry.Valid && !now.Before(a.invitationExpiry.Time) {
			s.State = "invitation_expired"
		} else if a.invitation.Valid {
			s.CanActivate = a.eligible.Valid && a.eligible.Bool
		} else {
			s.State = "uninvited"
		}
		return s
	}
	s.ExpiresAt = &a.expires.Time
	end := a.expires.Time
	if a.accessEnded.Valid && a.accessEnded.Time.Before(end) {
		end = a.accessEnded.Time
	}
	until := end.Add(30 * 24 * time.Hour)
	s.RetrieveUntil = &until
	if a.state == "revoked" && !a.accessEnded.Valid {
		s.RetrieveUntil = nil
	}
	if a.state == "active" {
		if now.Before(a.expires.Time) {
			s.CanDraft = true
			s.CanStandalone = true
			s.CanRetrieve = true
		} else if now.Before(until) {
			s.State = "retrieval_only"
			s.CanRetrieve = true
		} else {
			s.State = "expired"
		}
	}
	if a.state == "revoked" && a.accessEnded.Valid && !a.retrievalHeld && !a.revoked && now.Before(until) {
		s.CanRetrieve = true
	}
	if a.retrievalHeld {
		s.CanRetrieve = false
		s.CanDraft = false
		s.CanStandalone = false
	}
	return s
}
func statusInTransaction(ctx context.Context, tx *sql.Tx, owner string) (PilotStatus, error) {
	a, err := readPilot(ctx, tx, owner)
	if errors.Is(err, sql.ErrNoRows) {
		a = pilotAccess{}
	} else if err != nil {
		return PilotStatus{}, unavailable(err)
	}
	now, err := pilotNow(ctx, tx)
	return pilotStatus(owner, a, now), err
}
func (s *Store) PilotStatus(ctx context.Context, p auth.Principal) (PilotStatus, error) {
	tx, err := s.pilotTransaction(ctx, p, "")
	if err != nil {
		return PilotStatus{}, err
	}
	defer tx.Rollback()
	status, err := statusInTransaction(ctx, tx, p.AccountID)
	if err != nil {
		return status, err
	}
	return status, unavailable(tx.Commit())
}

func pilotReplay(ctx context.Context, tx *sql.Tx, actor, operation string, hash []byte) (string, bool, error) {
	var stored []byte
	var invitation string
	err := tx.QueryRowContext(ctx, "SELECT request_hash,invitation_id FROM careeros_pilot_commands WHERE actor_id=$1 AND operation_id=$2", actor, operation).Scan(&stored, &invitation)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, unavailable(err)
	}
	if !bytes.Equal(hash, stored) {
		return "", false, ErrConflict
	}
	return invitation, true, nil
}
func pilotReceipt(ctx context.Context, tx *sql.Tx, actor, operation string, hash []byte, invitation string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO careeros_pilot_commands VALUES($1,$2,$3,$4)", actor, operation, hash, invitation)
	return unavailable(err)
}

func (s *Store) PilotCommand(ctx context.Context, p auth.Principal, operation string, command PilotCommand) (PilotStatus, error) {
	if !validOperation(operation) || command.Identity.Provider == "" || command.Identity.Issuer == "" || command.Identity.Subject == "" || len(command.Identity.Provider) > 128 || len(command.Identity.Issuer) > 2048 || len(command.Identity.Subject) > 2048 {
		return PilotStatus{}, ErrValidation
	}
	switch command.Kind {
	case "invite", "clear", "revoke", "suspend":
	default:
		return PilotStatus{}, ErrValidation
	}
	// Resolve only a verified mapped subject. Display email is never authority.
	var target string
	err := s.db.QueryRowContext(ctx, "SELECT account_id FROM careeros_identities WHERE provider=$1 AND issuer=$2 AND subject=$3", []byte(command.Identity.Provider), []byte(command.Identity.Issuer), []byte(command.Identity.Subject)).Scan(&target)
	if errors.Is(err, sql.ErrNoRows) {
		return PilotStatus{}, auth.ErrForbidden
	}
	if err != nil {
		return PilotStatus{}, unavailable(err)
	}
	tx, err := s.pilotTransaction(ctx, p, target)
	if err != nil {
		return PilotStatus{}, err
	}
	defer tx.Rollback()
	var permitted bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM careeros_pilot_operators WHERE account_id=$1)", p.AccountID).Scan(&permitted); err != nil {
		return PilotStatus{}, unavailable(err)
	}
	if !permitted {
		return PilotStatus{}, auth.ErrForbidden
	}
	// The identity must still belong to the same locked account.
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM careeros_identities WHERE provider=$1 AND issuer=$2 AND subject=$3 AND account_id=$4)", []byte(command.Identity.Provider), []byte(command.Identity.Issuer), []byte(command.Identity.Subject), target).Scan(&permitted); err != nil {
		return PilotStatus{}, unavailable(err)
	}
	if !permitted {
		return PilotStatus{}, auth.ErrForbidden
	}
	command.Identity.CreatedAt = time.Time{}
	raw, _ := json.Marshal(command)
	hash := sha256.Sum256(raw)
	old, replay, err := pilotReplay(ctx, tx, p.AccountID, operation, hash[:])
	if err != nil {
		return PilotStatus{}, err
	}
	if replay {
		status, e := statusInTransaction(ctx, tx, target)
		status.InvitationID = old
		if e != nil {
			return status, e
		}
		return status, unavailable(tx.Commit())
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_pilot_access(account_id,provider,issuer,subject) VALUES($1,$2,$3,$4) ON CONFLICT(account_id) DO NOTHING`, target, []byte(command.Identity.Provider), []byte(command.Identity.Issuer), []byte(command.Identity.Subject)); err != nil {
		return PilotStatus{}, unavailable(err)
	}
	a, err := readPilot(ctx, tx, target)
	if err != nil {
		return PilotStatus{}, unavailable(err)
	}
	now, err := pilotNow(ctx, tx)
	if err != nil {
		return PilotStatus{}, err
	}
	invitation := a.invitation.String
	switch command.Kind {
	case "invite":
		if a.activated.Valid {
			return PilotStatus{}, ErrConflict
		}
		if a.invitation.Valid && a.invitationExpiry.Valid {
			if err = recordInvitationCleanup(ctx, tx, target, a.invitation.String, a.invitationExpiry.Time, now, &now); err != nil {
				return PilotStatus{}, err
			}
		}
		invitation, err = newID()
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE careeros_pilot_access SET invitation_id=$2,invited_at=$3,invitation_expires_at=$4,invitation_revoked=false,provider=$5,issuer=$6,subject=$7,age_eligible=NULL,eligibility_checked_at=NULL,eligibility_checked_by=NULL,invitation_cleaned_at=NULL WHERE account_id=$1`, target, invitation, now, now.Add(7*24*time.Hour), []byte(command.Identity.Provider), []byte(command.Identity.Issuer), []byte(command.Identity.Subject))
		}
	case "clear":
		if !a.activated.Valid && (!a.invitation.Valid || a.revoked || !a.invitationExpiry.Valid || !now.Before(a.invitationExpiry.Time)) {
			return PilotStatus{}, auth.ErrForbidden
		}
		_, err = tx.ExecContext(ctx, `UPDATE careeros_pilot_access SET age_eligible=true,eligibility_checked_at=$2,eligibility_checked_by=$3 WHERE account_id=$1`, target, now, p.AccountID)
	case "suspend":
		if !a.activated.Valid || a.state == "revoked" {
			return PilotStatus{}, ErrConflict
		}
		_, err = tx.ExecContext(ctx, `UPDATE careeros_pilot_access SET state='suspended',retrieval_held=true WHERE account_id=$1`, target)
	case "revoke":
		if a.activated.Valid {
			_, err = tx.ExecContext(ctx, `UPDATE careeros_pilot_access SET state='revoked',access_ended_at=CASE WHEN state='revoked' AND access_ended_at IS NULL THEN NULL WHEN access_ended_at IS NULL THEN $2 ELSE LEAST(access_ended_at,$2) END,retrieval_held=retrieval_held OR state='suspended' WHERE account_id=$1`, target, now)
		} else {
			if a.invitation.Valid && a.invitationExpiry.Valid {
				if err = recordInvitationCleanup(ctx, tx, target, a.invitation.String, a.invitationExpiry.Time, now, nil); err != nil {
					return PilotStatus{}, err
				}
			}
			_, err = tx.ExecContext(ctx, `UPDATE careeros_pilot_access SET invitation_revoked=true WHERE account_id=$1`, target)
		}
	}
	if err != nil {
		return PilotStatus{}, unavailable(err)
	}
	if err = pilotReceipt(ctx, tx, p.AccountID, operation, hash[:], invitation); err != nil {
		return PilotStatus{}, err
	}
	status, err := statusInTransaction(ctx, tx, target)
	if err != nil {
		return status, err
	}
	return status, unavailable(tx.Commit())
}

// ActivatePilot is explicit redemption and activation in one atomic operation.
// A status GET, sign-in and replay never start or extend access.
func (s *Store) ActivatePilot(ctx context.Context, p auth.Principal, operation, invitation string, confirm bool) (PilotStatus, error) {
	if !validOperation(operation) || !validOperation(invitation) || !confirm {
		return PilotStatus{}, ErrValidation
	}
	tx, err := s.pilotTransaction(ctx, p, "")
	if err != nil {
		return PilotStatus{}, err
	}
	defer tx.Rollback()
	hash := sha256.Sum256([]byte("activate-v1:" + invitation))
	_, replay, err := pilotReplay(ctx, tx, p.AccountID, operation, hash[:])
	if err != nil {
		return PilotStatus{}, err
	}
	if replay {
		status, e := statusInTransaction(ctx, tx, p.AccountID)
		if e != nil {
			return status, e
		}
		return status, unavailable(tx.Commit())
	}
	a, err := readPilot(ctx, tx, p.AccountID)
	if errors.Is(err, sql.ErrNoRows) {
		return PilotStatus{}, auth.ErrForbidden
	}
	if err != nil {
		return PilotStatus{}, unavailable(err)
	}
	now, err := pilotNow(ctx, tx)
	if err != nil {
		return PilotStatus{}, err
	}
	if !a.invitation.Valid || a.invitation.String != invitation || a.revoked || !a.eligible.Valid || !a.eligible.Bool {
		return PilotStatus{}, auth.ErrForbidden
	}
	// Binding survives account substitutions and mutable display information.
	var bound bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_pilot_access p JOIN careeros_identities i USING(provider,issuer,subject) WHERE p.account_id=$1 AND i.account_id=$1)`, p.AccountID).Scan(&bound); err != nil {
		return PilotStatus{}, unavailable(err)
	}
	if !bound {
		return PilotStatus{}, auth.ErrForbidden
	}
	if !a.activated.Valid {
		if !now.Before(a.invitationExpiry.Time) {
			return PilotStatus{}, auth.ErrForbidden
		}
		var occupied int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM careeros_pilot_access WHERE activated_at IS NOT NULL").Scan(&occupied); err != nil {
			return PilotStatus{}, unavailable(err)
		}
		if occupied >= 5 {
			return PilotStatus{}, auth.ErrCapacity
		}
		_, err = tx.ExecContext(ctx, "UPDATE careeros_pilot_access SET activated_at=$2,expires_at=$3,state='active',participant_slot=$4 WHERE account_id=$1", p.AccountID, now, now.Add(14*24*time.Hour), occupied+1)
		if err != nil {
			return PilotStatus{}, unavailable(err)
		}
	}
	if err = pilotReceipt(ctx, tx, p.AccountID, operation, hash[:], invitation); err != nil {
		return PilotStatus{}, err
	}
	status, err := statusInTransaction(ctx, tx, p.AccountID)
	if err != nil {
		return status, err
	}
	return status, unavailable(tx.Commit())
}

// PilotProfiles scopes existing Profile transactions to pilot authority without
// changing the legacy account composition. Both share the same database pool.
func (s *Store) PilotProfiles() *Store { return &Store{db: s.db, pilot: true} }

type PilotRight string

const (
	PilotRetrieve   PilotRight = "retrieve"
	PilotStandalone PilotRight = "standalone"
	PilotDraft      PilotRight = "draft"
)

func requirePilot(ctx context.Context, tx *sql.Tx, owner string, right PilotRight) error {
	status, err := statusInTransaction(ctx, tx, owner)
	if err != nil {
		return err
	}
	allowed := right == PilotRetrieve && status.CanRetrieve || right == PilotStandalone && status.CanStandalone || right == PilotDraft && status.CanDraft
	if !allowed {
		return auth.ErrForbidden
	}
	return nil
}

// AuthorizePilot revalidates session and grant after every blocking lock.
// It admits local operations only; this is not managed dispatch, allowance
// consumption or durable delivery. Those need their own atomic authority.
func (s *Store) AuthorizePilot(ctx context.Context, p auth.Principal, right PilotRight) error {
	tx, err := s.pilotTransaction(ctx, p, "")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = requirePilot(ctx, tx, p.AccountID, right); err != nil {
		return err
	}
	return unavailable(tx.Commit())
}

// BootstrapLocalPilotOperator is a privileged database-owner operation for a
// disposable synthetic database only. It is not a runtime HTTP capability.
// Runtime decisions additionally require a live session and this explicit role.
func (s *Store) BootstrapLocalPilotOperator(ctx context.Context, identity auth.IdentityRef) error {
	var allowed bool
	if err := s.db.QueryRowContext(ctx, `SELECT current_database() LIKE '%\_test' ESCAPE '\'`).Scan(&allowed); err != nil {
		return unavailable(err)
	}
	if !allowed {
		return auth.ErrForbidden
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO careeros_pilot_operators(account_id) SELECT account_id FROM careeros_identities WHERE provider=$1 AND issuer=$2 AND subject=$3 ON CONFLICT DO NOTHING`, []byte(identity.Provider), []byte(identity.Issuer), []byte(identity.Subject))
	if err != nil {
		return unavailable(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return unavailable(err)
	}
	if n == 0 {
		var exists bool
		err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_pilot_operators o JOIN careeros_identities i ON o.account_id=i.account_id WHERE i.provider=$1 AND i.issuer=$2 AND i.subject=$3)`, []byte(identity.Provider), []byte(identity.Issuer), []byte(identity.Subject)).Scan(&exists)
		if err != nil {
			return unavailable(err)
		}
		if !exists {
			return auth.ErrForbidden
		}
	}
	return nil
}

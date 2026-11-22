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

const controlFence int64 = 72481312

// ControlStatus contains authority metadata, never recovery proof or content.
type ControlStatus struct {
	Version            int64  `json:"version"`
	InvitationStopped  bool   `json:"invitationStopped"`
	InferenceStopped   bool   `json:"inferenceStopped"`
	AccountID          string `json:"accountId,omitempty"`
	Compromised        bool   `json:"compromised,omitempty"`
	InferenceEpoch     int64  `json:"inferenceEpoch"`
	SystemGeneration   int64  `json:"systemGeneration"`
	operationsRequired bool
}

type ControlCommand struct {
	Control           string `json:"control"`
	Stop              bool   `json:"stop"`
	ExpectedVersion   int64  `json:"expectedVersion"`
	AccountID         string `json:"accountId,omitempty"`
	RecoveryReference string `json:"recoveryReference,omitempty"`
}

// ControlRecovery is an injected code-owned verifier, not a request flag.
// Verify may perform local observations outside locks. Recheck is read-only
// under the final authority/namespace fence and must not reacquire that fence.
type ControlRecovery interface {
	VerifyControlRecovery(context.Context, auth.Principal, ControlCommand) (ControlRecoveryEvidence, error)
	RecheckControlRecovery(context.Context, auth.Principal, ControlCommand, ControlRecoveryEvidence) error
}
type ControlRecoveryEvidence struct {
	Reference           string `json:"reference"`
	ActorID             string `json:"actorId"`
	Control             string `json:"control"`
	AccountID           string `json:"accountId,omitempty"`
	StopVersion         int64  `json:"stopVersion"`
	Recovery            string `json:"recovery"`
	Reconciliation      string `json:"reconciliation"`
	ObservationDigest   string `json:"observationDigest"`
	WholeSystemExcluded bool   `json:"wholeSystemExcluded"`
	Exclusion           string `json:"exclusion,omitempty"`
	Generation          int64  `json:"generation"`
}
type OperatorControls struct {
	store    *Store
	recovery ControlRecovery
}

func (s *Store) LocalControls(recovery ControlRecovery) *OperatorControls {
	return &OperatorControls{store: s, recovery: recovery}
}

// LockControlTransaction orders managed admission before account/namespace
// locks. It is not an inference stop check: authorized reads remain possible.
func LockControlTransaction(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock_shared($1)", controlFence)
	return unavailable(err)
}

func controlStatus(ctx context.Context, tx *sql.Tx) (ControlStatus, error) {
	var s ControlStatus
	var initialized bool
	err := tx.QueryRowContext(ctx, `SELECT version,initialized,invitation_stopped,inference_stopped,inference_epoch,system_generation,operations_binding<>'' FROM careeros_control_state WHERE singleton`).Scan(&s.Version, &initialized, &s.InvitationStopped, &s.InferenceStopped, &s.InferenceEpoch, &s.SystemGeneration, &s.operationsRequired)
	if err != nil {
		return s, unavailable(err)
	}
	if !initialized {
		s.InvitationStopped, s.InferenceStopped = true, true
	}
	return s, nil
}

func controlActor(ctx context.Context, tx *sql.Tx, p auth.Principal) error {
	if err := authorizedAccount(ctx, tx, p); err != nil {
		if !errors.Is(err, auth.ErrAuthenticationRequired) && !errors.Is(err, auth.ErrForbidden) {
			return err
		}
		if restrictedErr := authorizeRestrictedRights(ctx, tx, p); restrictedErr != nil {
			return err
		}
	}
	var allowed bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_control_operators WHERE account_id=$1)`, p.AccountID).Scan(&allowed); err != nil {
		return unavailable(err)
	}
	if !allowed {
		return auth.ErrForbidden
	}
	return nil
}

// This private capability exists only after exact persisted session matching
// under the actor lock. It permits the final clock check after an intentional
// self-revocation; exported Principal expiry fields are not trusted evidence.
type capturedControlActor struct {
	account string
	expires time.Time
}

func captureControlActor(ctx context.Context, tx *sql.Tx, p auth.Principal) (capturedControlActor, error) {
	if err := controlActor(ctx, tx, p); err != nil {
		return capturedControlActor{}, err
	}
	v, err := scanSession(tx.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM careeros_sessions WHERE id=$1 AND account_id=$2 FOR UPDATE`, p.SessionID, p.AccountID))
	if errors.Is(err, sql.ErrNoRows) {
		v, err = scanSession(tx.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM careeros_deletion_status_sessions WHERE id=$1 AND account_id=$2 FOR UPDATE`, p.SessionID, p.AccountID))
	}
	if err != nil || !p.MatchesSession(v) {
		return capturedControlActor{}, auth.ErrAuthenticationRequired
	}
	return capturedControlActor{account: p.AccountID, expires: v.ExpiresAt}, nil
}
func (c capturedControlActor) check(ctx context.Context, tx *sql.Tx, p auth.Principal) error {
	if c.account != p.AccountID || c.expires.IsZero() {
		return auth.ErrForbidden
	}
	var valid bool
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()<$2 AND EXISTS(SELECT 1 FROM careeros_control_operators o JOIN careeros_accounts a ON a.id=o.account_id WHERE o.account_id=$1 AND a.status='active')`, c.account, c.expires).Scan(&valid); err != nil {
		return unavailable(err)
	}
	if !valid {
		return auth.ErrAuthenticationRequired
	}
	return nil
}

func localControlDatabase(ctx context.Context, tx *sql.Tx) error {
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT current_database() LIKE '%\_test' ESCAPE '\' AND current_schema() ~ '^careeros_(ai_)?test_[A-Za-z0-9_]+$' AND (inet_server_addr() IS NULL OR inet_server_addr()<<='127.0.0.0/8'::inet OR inet_server_addr()='::1'::inet)`).Scan(&allowed)
	if err != nil {
		return unavailable(err)
	}
	if !allowed {
		return auth.ErrForbidden
	}
	return nil
}

// BootstrapLocalControlOperator is an explicit database-owner fixture action.
// At most one identity is installed; pilot/support/reconciliation roles do not
// imply this capability. There is no HTTP bootstrap surface.
func (s *Store) BootstrapLocalControlOperator(ctx context.Context, identity auth.IdentityRef) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if err = localControlDatabase(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", controlFence); err != nil {
		return unavailable(err)
	}
	var target string
	err = tx.QueryRowContext(ctx, `SELECT account_id FROM careeros_identities WHERE provider=$1 AND issuer=$2 AND subject=$3`, []byte(identity.Provider), []byte(identity.Issuer), []byte(identity.Subject)).Scan(&target)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.ErrForbidden
	}
	if err != nil {
		return unavailable(err)
	}
	var other bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_control_operators WHERE account_id<>$1)`, target).Scan(&other); err != nil {
		return unavailable(err)
	}
	if other {
		return auth.ErrForbidden
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_control_operators(account_id) VALUES($1) ON CONFLICT DO NOTHING`, target); err != nil {
		return unavailable(err)
	}
	return unavailable(tx.Commit())
}

func (c *OperatorControls) transaction(ctx context.Context, p auth.Principal, target string, exclusive bool) (*sql.Tx, error) {
	if c == nil || c.store == nil {
		return nil, auth.ErrUnavailable
	}
	tx, err := c.store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, unavailable(err)
	}
	lock := "SELECT pg_advisory_xact_lock_shared($1)"
	if exclusive {
		lock = "SELECT pg_advisory_xact_lock($1)"
	}
	if _, err = tx.ExecContext(ctx, lock, controlFence); err == nil {
		err = lockPilot(ctx, tx)
	}
	if err == nil {
		rows, e := tx.QueryContext(ctx, `SELECT id FROM careeros_accounts WHERE id=$1 OR id=$2 ORDER BY id FOR UPDATE`, p.AccountID, target)
		if e != nil {
			err = e
		} else {
			for rows.Next() {
				var id string
				if e = rows.Scan(&id); e != nil {
					break
				}
			}
			if e == nil {
				e = rows.Err()
			}
			rows.Close()
			err = e
		}
	}
	if err == nil {
		err = controlActor(ctx, tx, p)
	}
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func controlReplay(ctx context.Context, tx *sql.Tx, p auth.Principal, operation string, request []byte) (ControlStatus, bool, error) {
	var hash, raw []byte
	err := tx.QueryRowContext(ctx, `SELECT request_hash,result FROM careeros_control_commands WHERE actor_id=$1 AND operation_id=$2`, p.AccountID, operation).Scan(&hash, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ControlStatus{}, false, nil
	}
	if err != nil {
		return ControlStatus{}, false, unavailable(err)
	}
	if !bytes.Equal(hash, request) {
		return ControlStatus{}, false, ErrConflict
	}
	var result ControlStatus
	if json.Unmarshal(raw, &result) != nil {
		return result, false, auth.ErrUnavailable
	}
	return result, true, nil
}
func controlReceipt(ctx context.Context, tx *sql.Tx, p auth.Principal, operation string, hash []byte, target string, captured *capturedControlActor) (ControlStatus, error) {
	var actorErr error
	if captured != nil {
		actorErr = captured.check(ctx, tx, p)
	} else {
		actorErr = controlActor(ctx, tx, p)
	}
	if actorErr != nil {
		return ControlStatus{}, actorErr
	}
	result, err := controlStatus(ctx, tx)
	if err != nil {
		return result, err
	}
	if target != "" {
		result.AccountID = target
		if err = tx.QueryRowContext(ctx, `SELECT compromised FROM careeros_control_accounts WHERE account_id=$1`, target).Scan(&result.Compromised); err != nil {
			return result, unavailable(err)
		}
	}
	raw, err := json.Marshal(result)
	if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO careeros_control_commands(actor_id,operation_id,request_hash,result) VALUES($1,$2,$3,$4)`, p.AccountID, operation, hash, raw)
	}
	if err == nil {
		if captured != nil {
			err = captured.check(ctx, tx, p)
		} else {
			err = controlActor(ctx, tx, p)
		}
	}
	return result, unavailable(err)
}

func (c *OperatorControls) InitializeSynthetic(ctx context.Context, p auth.Principal, operation string) (ControlStatus, error) {
	if !validOperation(operation) {
		return ControlStatus{}, ErrValidation
	}
	tx, err := c.transaction(ctx, p, "", true)
	if err != nil {
		return ControlStatus{}, err
	}
	defer tx.Rollback()
	if err = localControlDatabase(ctx, tx); err != nil {
		return ControlStatus{}, err
	}
	hash := sha256.Sum256([]byte("fresh-synthetic-controls-v1"))
	prior, found, err := controlReplay(ctx, tx, p, operation, hash[:])
	if err != nil {
		return prior, err
	}
	if found {
		return prior, unavailable(tx.Commit())
	}
	var fresh bool
	if err = tx.QueryRowContext(ctx, `SELECT NOT initialized AND NOT EXISTS(SELECT 1 FROM careeros_control_commands) AND NOT EXISTS(SELECT 1 FROM careeros_pilot_access WHERE invitation_id IS NOT NULL OR activated_at IS NOT NULL) FROM careeros_control_state WHERE singleton FOR UPDATE`).Scan(&fresh); err != nil {
		return ControlStatus{}, unavailable(err)
	}
	if !fresh {
		return ControlStatus{}, auth.ErrForbidden
	}
	for _, table := range []string{"ai_fixture_namespace", "ai_fixture_action", "ai_fixture_grant", "ai_fixture_dispatch"} {
		present, e := installed(ctx, tx, table)
		if e != nil {
			return ControlStatus{}, e
		}
		if present {
			var exists bool
			// All table names are this fixed source allowlist, never input.
			if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+")").Scan(&exists); e != nil {
				return ControlStatus{}, unavailable(e)
			}
			if exists {
				return ControlStatus{}, auth.ErrForbidden
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE careeros_control_state SET initialized=true,invitation_stopped=false,inference_stopped=false,changed_at=clock_timestamp() WHERE singleton`); err != nil {
		return ControlStatus{}, unavailable(err)
	}
	result, err := controlReceipt(ctx, tx, p, operation, hash[:], "", nil)
	if err != nil {
		return result, err
	}
	return result, unavailable(tx.Commit())
}

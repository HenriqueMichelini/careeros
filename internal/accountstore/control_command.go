package accountstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"

	"professional-information-repo/internal/auth"
)

// WithStatus retains the current operator/session and control fence through a
// bounded code-owned callback. It exposes no proof, ledger or recipient fields.
func (c *OperatorControls) WithStatus(ctx context.Context, p auth.Principal, target string, write func(ControlStatus) error) error {
	if write == nil || target != "" && !validOperation(target) {
		return ErrValidation
	}
	tx, err := c.transaction(ctx, p, target, false)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	status, err := controlStatus(ctx, tx)
	if err != nil {
		return err
	}
	if target != "" {
		status.AccountID = target
		if err = tx.QueryRowContext(ctx, `SELECT careeros_control_account_denied($1)`, target).Scan(&status.Compromised); err != nil {
			return unavailable(err)
		}
	}
	if err = controlActor(ctx, tx, p); err != nil {
		return err
	}
	return write(status)
}
func (c *OperatorControls) Status(ctx context.Context, p auth.Principal, target string) (ControlStatus, error) {
	var result ControlStatus
	err := c.WithStatus(ctx, p, target, func(v ControlStatus) error { result = v; return nil })
	return result, err
}

func validControlCommand(command ControlCommand) bool {
	if command.ExpectedVersion <= 0 || command.Control != "invitations" && command.Control != "inference" && command.Control != "compromise" {
		return false
	}
	if command.Control == "compromise" && !validOperation(command.AccountID) || command.Control != "compromise" && command.AccountID != "" {
		return false
	}
	return command.Stop && command.RecoveryReference == "" || !command.Stop && validOperation(command.RecoveryReference)
}

func (c *OperatorControls) Apply(ctx context.Context, p auth.Principal, operation string, command ControlCommand) (ControlStatus, error) {
	if !validOperation(operation) || !validControlCommand(command) {
		return ControlStatus{}, ErrValidation
	}
	tx, err := c.transaction(ctx, p, command.AccountID, true)
	if err != nil {
		return ControlStatus{}, err
	}
	defer tx.Rollback()
	raw, _ := json.Marshal(command)
	hash := sha256.Sum256(raw)
	prior, found, err := controlReplay(ctx, tx, p, operation, hash[:])
	if err != nil {
		return prior, err
	}
	if found {
		return prior, unavailable(tx.Commit())
	}
	var evidence ControlRecoveryEvidence
	if !command.Stop {
		if c.recovery == nil {
			return ControlStatus{}, auth.ErrForbidden
		}
		// Verifier observations are outside the authority transaction. The
		// exact checkpoint is rechecked below after final namespace locks.
		_ = tx.Rollback()
		evidence, err = c.recovery.VerifyControlRecovery(ctx, p, command)
		if err != nil {
			return ControlStatus{}, err
		}
		tx, err = c.transaction(ctx, p, command.AccountID, true)
		if err != nil {
			return ControlStatus{}, err
		}
		defer tx.Rollback()
		prior, found, err = controlReplay(ctx, tx, p, operation, hash[:])
		if err != nil {
			return prior, err
		}
		if found {
			return prior, unavailable(tx.Commit())
		}
		if err = lockRecoveryNamespaces(ctx, tx); err != nil {
			return ControlStatus{}, err
		}
		current, e := controlStatus(ctx, tx)
		if e != nil {
			return ControlStatus{}, e
		}
		if current.Version != command.ExpectedVersion {
			return ControlStatus{}, ErrConflict
		}
		if command.Control != "compromise" {
			if current.operationsFault {
				return ControlStatus{}, auth.ErrForbidden
			}
			release, err := c.store.holdOperations(ctx, current.operationsRequired, command.Control)
			if err != nil {
				return ControlStatus{}, err
			}
			defer release()
			var unresolved bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_operations_containment
		  WHERE NOT resolved AND ($1='inference' OR $1='invitations' AND invitation_stopped))
		  OR ($1='inference' AND (EXISTS(SELECT 1 FROM careeros_operations_pending WHERE critical_event)
		    OR EXISTS(SELECT 1 FROM careeros_operations_deferred)))`, command.Control).Scan(&unresolved); err != nil {
				return ControlStatus{}, unavailable(err)
			}
			if unresolved {
				return ControlStatus{}, auth.ErrForbidden
			}
		}
		if err = validRecoveryEvidence(p, command, current, evidence); err != nil {
			return ControlStatus{}, err
		}
		if err = c.recovery.RecheckControlRecovery(ctx, p, command, evidence); err != nil {
			return ControlStatus{}, err
		}
	}
	actor, err := captureControlActor(ctx, tx, p)
	if err != nil {
		return ControlStatus{}, err
	}
	if err = applyGlobalControl(ctx, tx, command); err != nil {
		return ControlStatus{}, err
	}
	if command.Control == "compromise" {
		if command.Stop {
			err = stopCompromisedAccount(ctx, tx, p, command.AccountID)
		} else {
			err = reopenCompromisedAccount(ctx, tx, command.AccountID)
		}
		if err != nil {
			return ControlStatus{}, err
		}
	}
	if !command.Stop {
		if evidence.WholeSystemExcluded {
			if _, err = tx.ExecContext(ctx, `UPDATE careeros_control_state SET system_generation=$1,inference_epoch=inference_epoch+CASE WHEN $2='inference' THEN 0 ELSE 1 END WHERE singleton AND inference_epoch<9223372036854775807`, evidence.Generation, command.Control); err != nil {
				return ControlStatus{}, unavailable(err)
			}
			present, e := installed(ctx, tx, "ai_fixture_namespace")
			if e != nil {
				return ControlStatus{}, e
			}
			if present {
				var overflow bool
				if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_fixture_namespace WHERE epoch>=9223372036854775807)`).Scan(&overflow); err != nil {
					return ControlStatus{}, unavailable(err)
				}
				if overflow {
					return ControlStatus{}, ErrConflict
				}
				if _, err = tx.ExecContext(ctx, `UPDATE ai_fixture_namespace SET epoch=epoch+1`); err != nil {
					return ControlStatus{}, unavailable(err)
				}
			}
		}
		evidenceRaw, _ := json.Marshal(evidence)
		if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_control_recovery(reference,actor_id,operation_id,evidence) VALUES($1,$2,$3,$4)`, evidence.Reference, p.AccountID, operation, evidenceRaw); err != nil {
			return ControlStatus{}, ErrConflict
		}
	}
	var self *capturedControlActor
	if command.Control == "compromise" && command.AccountID == p.AccountID {
		self = &actor
	}
	result, err := controlReceipt(ctx, tx, p, operation, hash[:], command.AccountID, self)
	if err != nil {
		return result, err
	}
	return result, unavailable(tx.Commit())
}

func applyGlobalControl(ctx context.Context, tx *sql.Tx, command ControlCommand) error {
	var result sql.Result
	var err error
	switch command.Control {
	case "invitations":
		result, err = tx.ExecContext(ctx, `UPDATE careeros_control_state SET initialized=true,invitation_stopped=$2,version=version+1,changed_at=clock_timestamp() WHERE singleton AND (initialized OR NOT $2) AND version=$1 AND version<9223372036854775807`, command.ExpectedVersion, command.Stop)
	case "inference":
		result, err = tx.ExecContext(ctx, `UPDATE careeros_control_state SET initialized=true,inference_stopped=$2,version=version+1,inference_epoch=inference_epoch+1,changed_at=clock_timestamp() WHERE singleton AND (initialized OR NOT $2) AND version=$1 AND version<9223372036854775807 AND inference_epoch<9223372036854775807`, command.ExpectedVersion, command.Stop)
	case "compromise":
		result, err = tx.ExecContext(ctx, `UPDATE careeros_control_state SET version=version+1,changed_at=clock_timestamp() WHERE singleton AND initialized AND version=$1 AND version<9223372036854775807`, command.ExpectedVersion)
	}
	if err != nil {
		return unavailable(err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrConflict
	}
	return nil
}

func lockRecoveryNamespaces(ctx context.Context, tx *sql.Tx) error {
	present, err := installed(ctx, tx, "ai_fixture_namespace")
	if err != nil || !present {
		return err
	}
	if _, err = tx.ExecContext(ctx, `LOCK TABLE ai_fixture_namespace IN EXCLUSIVE MODE`); err != nil {
		return unavailable(err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM ai_fixture_namespace ORDER BY id FOR UPDATE`)
	if err != nil {
		return unavailable(err)
	}
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
	return unavailable(err)
}

func validRecoveryEvidence(p auth.Principal, command ControlCommand, current ControlStatus, e ControlRecoveryEvidence) error {
	if e.ActorID != p.AccountID || e.Control != command.Control || e.AccountID != command.AccountID || e.StopVersion != command.ExpectedVersion || e.Reference != command.RecoveryReference || !validOperation(e.Reference) || !validOperation(e.Recovery) || !validOperation(e.Reconciliation) || len(e.ObservationDigest) != 64 {
		return auth.ErrForbidden
	}
	if e.WholeSystemExcluded {
		if !validOperation(e.Exclusion) || current.SystemGeneration >= 9223372036854775807 || e.Generation != current.SystemGeneration+1 {
			return auth.ErrForbidden
		}
	} else if e.Exclusion != "" || e.Generation != current.SystemGeneration {
		return auth.ErrForbidden
	}
	return nil
}

package accountstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"professional-information-repo/internal/auth"
)

type OperationsFault string

const (
	OperationsDatabaseFault  OperationsFault = "database_fault"
	OperationsAuthorityFault OperationsFault = "authority_fault"
	OperationsControlFault   OperationsFault = "control_fault"
	OperationsAuthFault      OperationsFault = "auth_fault"
	OperationsProviderFault  OperationsFault = "provider_fault"
	OperationsBackupFault    OperationsFault = "backup_fault"
)

// ObserveFault is for the explicitly composed local runtime observer. Expected
// sign-in denial and normal grant expiry/exhaustion are not accepted faults.
func (o *Operations) ObserveFault(ctx context.Context, fault OperationsFault) error {
	// Both actions are attempted even when the primary authority is unavailable.
	replay, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	created, err := o.journal.recordFault(replay, fault)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			return err
		}
		return errors.Join(err, o.synchronizeEmergencyStop(replay))
	}
	deliveryCtx, deliveryCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer deliveryCancel()
	deliveryErr := o.Deliver(deliveryCtx)
	persistenceCtx, persistenceCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer persistenceCancel()
	if created {
		// Diagnostics have their own ordinary TTL. The restricted incident
		// remains durable when the diagnostic database cannot be reached.
		_ = o.RecordDiagnostic(persistenceCtx, fault)
	}
	return errors.Join(deliveryErr, o.synchronizeContainment(persistenceCtx))
}

// SignalOfflineFault is an independently owned local observer command. A
// registered protected path checks this journal until the SQL stop is replayed.
func (j *OperationsJournal) SignalOfflineFault(ctx context.Context, fault OperationsFault) error {
	if _, err := j.recordFault(ctx, fault); err != nil {
		return err
	}
	return j.deliver(ctx)
}

func (j *OperationsJournal) recordFault(ctx context.Context, fault OperationsFault) (bool, error) {
	var response string
	switch fault {
	case OperationsDatabaseFault, OperationsAuthorityFault, OperationsControlFault:
		response = "keep_protected_operations_closed_and_verify_authority"
	case OperationsAuthFault:
		response = "contain_and_review_unexpected_auth_failure"
	case OperationsProviderFault:
		response = "pause_inference_and_review_provider_health"
	case OperationsBackupFault:
		response = "pause_inference_and_review_declared_backup_failure"
	default:
		return false, ErrValidation
	}
	incident := OperationsIncident{Kind: string(fault), EventAt: j.cfg.Clock().UTC(),
		Severity: "critical", Owner: "Henrique", Response: response, State: "unacknowledged"}
	var created bool
	err := j.containmentTransaction(ctx, func(state *operationsJournalState) error {
		created = j.upsert(state, "fault/"+string(fault), incident, true)
		switch fault {
		case OperationsDatabaseFault, OperationsAuthorityFault:
			state.Persistence = "unavailable_at_observation"
		case OperationsBackupFault:
			state.Backup = "declared_failure"
		}
		return nil
	})
	return created, err
}

type containment struct {
	id          string
	revision    int
	invitations bool
}

func operationsStop(record operationsJournalRecord, now time.Time) (stop, invitations bool) {
	if record.Incident.State == "resolved" {
		return false, false
	}
	switch record.Incident.Kind {
	case "provider_fault", "backup_fault", "maximum_breach", "invariant_breach", "spend_breach":
		return true, false
	case "database_fault", "authority_fault", "control_fault", "auth_fault", "operator_absence":
		return true, true
	default:
		due := record.Incident.State == "unacknowledged" && record.Incident.Severity == "warning" && !now.Before(record.Incident.EventAt.Add(24*time.Hour))
		return record.Contained || due, record.Contained || due
	}
}

// ReviewIncident records the named operations owner's acknowledgment or
// resolution. The separate #290 control still requires verified reopening.
func (o *Operations) ReviewIncident(ctx context.Context, p auth.Principal, id string, revision int, resolve bool) error {
	return o.reviewIncident(ctx, p, id, revision, resolve, "")
}

func (o *Operations) ResolveIncident(ctx context.Context, p auth.Principal, id string, revision int, recovery string) error {
	return o.reviewIncident(ctx, p, id, revision, true, recovery)
}

func (o *Operations) reviewIncident(ctx context.Context, p auth.Principal, id string, revision int, resolve bool, recovery string) error {
	if len(id) != 64 || !validOperation(id) || revision <= 0 {
		return ErrValidation
	}
	tx, err := o.store.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", controlFence); err != nil {
		return unavailable(err)
	}
	if err = operationsActor(ctx, tx, p, "owner"); err != nil {
		return err
	}
	return o.journal.transaction(ctx, true, func(state *operationsJournalState) error {
		record, ok := state.Records[id]
		if !ok || record.Revision != revision && !(resolve && record.Incident.State == "resolved" && record.Revision == revision+1) {
			return ErrConflict
		}
		if err := operationsActor(ctx, tx, p, "owner"); err != nil {
			return err
		}
		if record.Incident.State == "resolved" {
			return nil
		}
		if resolve {
			switch record.Incident.Kind {
			case "backup_fault":
				if o.cfg.VerifyBackupRecovery == nil || !validOperation(recovery) {
					return auth.ErrForbidden
				}
				proof, err := o.cfg.VerifyBackupRecovery(ctx, recovery, record.Incident)
				if err != nil || proof.Before(record.Incident.EventAt) || proof.After(o.journal.cfg.Clock().UTC()) {
					return auth.ErrForbidden
				}
				proof = proof.UTC()
				record.RecoveryAt = &proof
				record.RecoveryBinding = o.journal.binding([]byte("recovery\x00" + id + "\x00" + recovery))
				state.Backup = "verified_recovery"
			case "operator_absence":
				if record.AbsenceUntil.IsZero() || o.journal.cfg.Clock().UTC().Before(record.AbsenceUntil) {
					return auth.ErrForbidden
				}
			}
			if err := operationsActor(ctx, tx, p, "owner"); err != nil {
				return err
			}
		}
		stop, invitations := operationsStop(record, o.journal.cfg.Clock().UTC())
		if stop {
			if err := applyOperationsContainment(ctx, tx, containment{id, record.Revision, invitations}); err != nil {
				return err
			}
			record.Contained = true
		}
		if resolve {
			if _, err := tx.ExecContext(ctx, `UPDATE careeros_operations_containment SET resolved=true
			  WHERE incident_id=$1 AND incident_revision=$2`, id, revision); err != nil {
				return unavailable(err)
			}
			record.Incident.State = "resolved"
			record.Incident.Response = "incident_resolved_review_control_reopening"
			record.Revision++
			record.Delivery, record.Attempt, record.FirstAttempt = "pending", 0, time.Time{}
		} else {
			record.Incident.State = "acknowledged"
		}
		if err := tx.Commit(); err != nil {
			return unavailable(err)
		}
		state.Records[id] = record
		return nil
	})
}

// HoldProtected orders an independently observed stop against invite commit
// or actual fake provider entry, including when PostgreSQL is recovering.
// Only the three protected control kinds consult this lease.
func (o *Operations) HoldProtected(ctx context.Context, control string) (func(), error) {
	unlock, err := o.journal.lock(ctx, false)
	if err != nil {
		return nil, auth.ErrUnavailable
	}
	if err = o.journal.checkFence(); err != nil {
		unlock()
		return nil, err
	}
	state, err := o.journal.read()
	if err == nil {
		now := o.journal.cfg.Clock().UTC()
		for _, record := range state.Records {
			stop, invitation := operationsStop(record, now)
			if stop && (control == "inference" || control == "invitations" && invitation) {
				err = auth.ErrForbidden
				break
			}
		}
	}
	if err != nil {
		unlock()
		return nil, err
	}
	return unlock, nil
}

func (s *Store) holdOperations(ctx context.Context, required bool, control string) (func(), error) {
	if o := s.operations.Load(); o != nil {
		return o.HoldProtected(ctx, control)
	}
	if required {
		return nil, auth.ErrUnavailable
	}
	return func() {}, nil
}

// This is an add-only authority separate from the ordinary operator adapter.
// An incident first wins in the independent journal, then is applied once to
// the shared PostgreSQL stop and fence. No delivery outcome controls a stop.
func (o *Operations) synchronizeContainment(ctx context.Context) error {
	var pending []containment
	err := o.journal.transaction(ctx, true, func(state *operationsJournalState) error {
		if err := o.journal.checkFence(); err != nil {
			return err
		}
		for id, record := range state.Records {
			if stop, invitation := operationsStop(record, o.journal.cfg.Clock().UTC()); stop {
				record.Contained = true
				state.Records[id] = record
				pending = append(pending, containment{id, record.Revision, invitation})
			}
		}
		return nil
	})
	if err != nil {
		return errors.Join(err, o.synchronizeEmergencyStop(ctx))
	}
	if len(pending) == 0 {
		return err
	}
	tx, err := o.store.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", controlFence); err != nil {
		return unavailable(err)
	}
	for _, event := range pending {
		if err := applyOperationsContainment(ctx, tx, event); err != nil {
			return err
		}
	}
	return unavailable(tx.Commit())
}

func (o *Operations) synchronizeEmergencyStop(ctx context.Context) error {
	replay, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	tx, err := o.store.db.BeginTx(replay, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(replay, "SELECT pg_advisory_xact_lock($1)", controlFence); err != nil {
		return unavailable(err)
	}
	event := containment{o.journal.binding([]byte("operations_persistence_unavailable")), 1, true}
	if err = applyOperationsContainment(replay, tx, event); err != nil {
		return err
	}
	return unavailable(tx.Commit())
}

func applyOperationsContainment(ctx context.Context, tx *sql.Tx, event containment) error {
	result, err := tx.ExecContext(ctx, `INSERT INTO careeros_operations_containment(incident_id,incident_revision,invitation_stopped)
		  VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, event.id, event.revision, event.invitations)
	if err != nil {
		return unavailable(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return unavailable(err)
	}
	if count == 0 {
		return nil
	}
	result, err = tx.ExecContext(ctx, `UPDATE careeros_control_state SET inference_stopped=true,
		  invitation_stopped=invitation_stopped OR $1,version=version+1,
		  inference_epoch=inference_epoch+1,changed_at=clock_timestamp()
		  WHERE singleton AND version<9223372036854775807 AND inference_epoch<9223372036854775807`, event.invitations)
	if err != nil {
		return unavailable(err)
	}
	if count, err = result.RowsAffected(); err != nil || count != 1 {
		return auth.ErrUnavailable
	}
	return nil
}

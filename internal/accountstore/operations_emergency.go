package accountstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"professional-information-repo/internal/auth"
)

// OperationsEmergency identifies the current private interval requiring review.
// StartedAt is the first durable fallback receipt, which can follow the onset.
type OperationsEmergency struct {
	ID           string            `json:"id"`
	StartedAt    time.Time         `json:"startedAt"`
	RecordedAt   time.Time         `json:"recordedAt"`
	Causes       []OperationsFault `json:"causes"`
	ReceiptSet   string            `json:"receiptSet,omitempty"`
	observations map[OperationsFault]struct {
		At       time.Time `json:"at"`
		LatestAt time.Time `json:"latestAt"`
	}
}

type OperationsEmergencyReview struct {
	ObservedAt       time.Time
	ReceiptSet       string
	ClassifiedCauses []OperationsFault
}

func (j *OperationsJournal) observerCapability() string {
	return j.binding([]byte("operation-sql-observer"))
}

// CaptureReconciliation binds the enrolled observer to the current source
// transition. Neither the caller nor the finance role supplies the event data.
func (o *Operations) CaptureReconciliation(ctx context.Context, tx *sql.Tx, namespace, command string, includeEvent bool) (int, error) {
	var outcome int
	if err := tx.QueryRowContext(ctx, `SELECT careeros_capture_reconciliation_operations($1,$2,$3,$4)`,
		o.journal.observerCapability(), namespace, command, includeEvent).Scan(&outcome); err != nil {
		return 0, unavailable(err)
	}
	return outcome, nil
}

// AcknowledgeCapture discards only the exact source record the journal has
// consumed. The acknowledgement uses a separate enrolled observer capability.
func (o *Operations) AcknowledgeCapture(ctx context.Context, db *sql.DB, namespace string, receipt int64, fingerprint [32]byte) error {
	var removed bool
	if err := db.QueryRowContext(ctx, `SELECT careeros_ack_operations_capture($1,$2,$3,$4)`,
		o.journal.observerCapability(), namespace, receipt, fingerprint[:]).Scan(&removed); err != nil || !removed {
		return auth.ErrUnavailable
	}
	return nil
}

// SignalLocalOperationsFallback uses the previously enrolled observer key
// when the independent filesystem cannot durably record the fault. It does
// not open the journal, grant database reads, or install an observer.
func (s *Store) SignalLocalOperationsFallback(ctx context.Context, cfg OperationsJournalConfig, kind OperationsFault) error {
	if s == nil || cfg.BindingKey == ([32]byte{}) {
		return ErrValidation
	}
	if _, err := operationsFaultIncident(kind, time.Time{}); err != nil {
		return err
	}
	j := OperationsJournal{cfg: cfg}
	return s.recordOperationsEmergency(ctx, j.observerCapability(), kind)
}

func (o *Operations) synchronizeEmergencyStop(ctx context.Context, kind OperationsFault) error {
	return o.store.recordOperationsEmergency(ctx, o.journal.observerCapability(), kind)
}

func (s *Store) recordOperationsEmergency(ctx context.Context, capability string, kind OperationsFault) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if err = localControlDatabase(ctx, tx); err != nil {
		return err
	}
	if err = markOperationsEmergency(ctx, tx, capability, kind); err != nil {
		return err
	}
	return unavailable(tx.Commit())
}

func markOperationsEmergency(ctx context.Context, tx *sql.Tx, capability string, kind OperationsFault) error {
	var marked bool
	if err := tx.QueryRowContext(ctx, `SELECT careeros_operations_emergency($1,$2)`, capability, kind).Scan(&marked); err != nil {
		return unavailable(err)
	}
	if !marked {
		return auth.ErrForbidden
	}
	return nil
}

func readOperationsEmergency(ctx context.Context, tx *sql.Tx) (*OperationsEmergency, bool, error) {
	var fault bool
	var e OperationsEmergency
	var started, recorded sql.NullTime
	var raw []byte
	var observations []byte
	err := tx.QueryRowContext(ctx, `SELECT operations_fault,operations_fault_id,operations_fault_since,
	  operations_fault_at,to_json(operations_fault_kinds),operations_fault_observations,operations_fault_receipt_set
	  FROM careeros_control_state WHERE singleton`).
		Scan(&fault, &e.ID, &started, &recorded, &raw, &observations, &e.ReceiptSet)
	if err != nil || !fault {
		return nil, false, unavailable(err)
	}
	if e.ID == "" {
		return nil, true, nil // A persisted latch from the earlier composition.
	}
	if len(e.ID) != 64 || !validOperation(e.ID) || !started.Valid || !recorded.Valid || started.Time.After(recorded.Time) ||
		e.ReceiptSet != "" && (len(e.ReceiptSet) != 64 || !validOperation(e.ReceiptSet)) ||
		json.Unmarshal(raw, &e.Causes) != nil || len(e.Causes) < 1 || len(e.Causes) > 8 ||
		json.Unmarshal(observations, &e.observations) != nil || len(e.observations) > 8 {
		return nil, false, auth.ErrUnavailable
	}
	e.StartedAt, e.RecordedAt = started.Time.UTC(), recorded.Time.UTC()
	for _, observed := range e.observations {
		if observed.At.IsZero() || observed.LatestAt.Before(observed.At) || observed.LatestAt.After(e.RecordedAt) {
			return nil, false, auth.ErrUnavailable
		}
	}
	return &e, false, nil
}

// ReplayEmergency mirrors the exact current emergency into the journal. A
// separately verified interval review may then clear the emergency latch.
// Invitation and inference stops remain for the existing #290 authority.
func (o *Operations) ReplayEmergency(ctx context.Context, p auth.Principal, reference string) error {
	if reference != "" && (!validOperation(reference) || o.cfg.VerifyEmergencyReview == nil) {
		return auth.ErrForbidden
	}
	if err := o.replayEmergency(ctx, p, reference); err != nil {
		return err
	}
	if reference != "" {
		return o.DrainCommittedRisk(ctx)
	}
	return o.Deliver(ctx)
}

func (o *Operations) replayEmergency(ctx context.Context, p auth.Principal, reference string) error {
	tx, err := o.store.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if err = LockControlStopTransaction(ctx, tx); err != nil {
		return err
	}
	if err = operationsActor(ctx, tx, p, "owner"); err != nil {
		return err
	}
	unlock, err := o.journal.lock(ctx, true)
	if err != nil {
		return err
	}
	defer unlock()
	endDecision, err := o.journal.lockAdmission(ctx, true)
	if err != nil {
		return err
	}
	defer endDecision()
	if err = operationsActor(ctx, tx, p, "owner"); err != nil {
		return err
	}
	state, err := o.journal.read()
	if err != nil || state.ID != o.journal.id {
		return auth.ErrUnavailable
	}
	faultFiles, err := o.journal.faultFiles()
	if err != nil {
		return err
	}
	if err = o.journal.mirrorFaultReceipts(&state, faultFiles); err != nil {
		return err
	}
	e, legacy, err := readOperationsEmergency(ctx, tx)
	if err != nil {
		return err
	}
	oldHold := o.journal.binding([]byte("operations_persistence_unavailable"))
	if e == nil {
		if reference != "" {
			return ErrConflict
		}
		var orphan bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_operations_containment
		  WHERE incident_id=$1 AND incident_revision=1 AND NOT resolved)`, oldHold).Scan(&orphan); err != nil {
			return unavailable(err)
		}
		if !legacy && !orphan && o.journal.verifyWriteHealth() == nil {
			return ErrNotFound
		}
	}
	if e == nil || e.ReceiptSet != faultFiles.set || faultFiles.unclassified && !slices.Contains(e.Causes, operationsUnclassifiedFault) {
		if reference != "" {
			return auth.ErrForbidden
		}
		reason := operationsPersistenceFault
		if faultFiles.unclassified || e == nil && !faultFiles.known {
			reason = operationsUnclassifiedFault
		}
		if err = markOperationsEmergency(ctx, tx, o.journal.observerCapability(), reason); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE careeros_control_state SET operations_fault_receipt_set=$1 WHERE singleton`, faultFiles.set); err != nil {
			return unavailable(err)
		}
		e, _, err = readOperationsEmergency(ctx, tx)
		if err != nil || e == nil {
			return auth.ErrUnavailable
		}
	}
	if err = o.journal.mirrorEmergency(&state, *e); err != nil {
		return err
	}
	if reference != "" {
		if err = o.reviewEmergency(ctx, tx, p, reference, e, &state, oldHold, faultFiles); err != nil {
			return err
		}
	} else if err = o.journal.write(state); err != nil {
		return err
	}
	return unavailable(tx.Commit())
}

func (j *OperationsJournal) mirrorEmergency(state *operationsJournalState, e OperationsEmergency) error {
	for _, kind := range e.Causes {
		if kind != operationsPersistenceFault && kind != operationsUnclassifiedFault {
			observed := e.observations[kind]
			if observed.At.IsZero() {
				observed.At, observed.LatestAt = e.RecordedAt, e.RecordedAt
			}
			incident, err := operationsFaultIncident(kind, observed.At)
			if err != nil {
				return auth.ErrUnavailable
			}
			incident.LatestObservedAt = &observed.LatestAt
			j.upsertFault(state, kind, incident)
		}
	}
	if _, ok := state.Records[e.ID]; !ok {
		incident := OperationsIncident{ID: e.ID, Kind: string(operationsPersistenceFault), EventAt: e.StartedAt,
			Owner: "Henrique", Severity: "critical", State: "unacknowledged", Response: "verify_unrecorded_containment_interval"}
		state.Records[e.ID] = operationsJournalRecord{Incident: incident, Revision: 1, Delivery: "pending"}
	}
	return nil
}

func (o *Operations) reviewEmergency(ctx context.Context, tx *sql.Tx, p auth.Principal, reference string, e *OperationsEmergency, state *operationsJournalState, oldHold string, faultFiles operationsFaultFiles) error {
	review, err := o.cfg.VerifyEmergencyReview(ctx, reference, *e)
	at := review.ObservedAt
	if err != nil || review.ReceiptSet != e.ReceiptSet || !at.After(e.RecordedAt) || at.Before(faultFiles.latest) || at.After(time.Now().UTC()) {
		return auth.ErrForbidden
	}
	if err = o.journal.classifyEmergency(state, *e, review); err != nil {
		return err
	}
	sample, err := o.source.OperationsSample(ctx)
	if err != nil || !sample.Complete || sample.At.IsZero() || !validOperation(sample.Namespace) {
		return auth.ErrUnavailable
	}
	// A readable snapshot alone does not prove the capture writer recovered.
	// Commit this source-owned observation together with the verified clearance.
	var captured bool
	if err = tx.QueryRowContext(ctx, `SELECT careeros_replay_operations_captures($1)`, sample.Namespace).Scan(&captured); err != nil || !captured {
		return auth.ErrUnavailable
	}
	if err = operationsActor(ctx, tx, p, "owner"); err != nil {
		return err
	}
	for id, record := range state.Records {
		if record.Incident.Kind == string(operationsPersistenceFault) && record.Incident.State != "resolved" {
			proof := at.UTC()
			record.RecoveryAt = &proof
			record.RecoveryBinding = o.journal.binding([]byte("emergency-review\x00" + e.ID + "\x00" + reference))
			record.Incident.State = "resolved"
			record.Incident.Response = "interval_reviewed_verify_control_reopening"
			record.Revision++
			record.Delivery, record.Attempt, record.FirstAttempt = "pending", 0, time.Time{}
			state.Records[id] = record
		}
	}
	if err = o.journal.write(*state); err != nil {
		return err
	}
	if err = o.journal.clearReviewedFence(faultFiles); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE careeros_operations_containment SET resolved=true
	  WHERE incident_id=$1 AND incident_revision=1`, oldHold); err != nil {
		return unavailable(err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE careeros_control_state SET operations_fault=false,
	  operations_fault_id='',operations_fault_since=NULL,operations_fault_at=NULL,operations_fault_kinds='{}',
	  operations_fault_observations='{}',operations_fault_receipt_set=''
	  WHERE singleton AND operations_fault AND operations_fault_id=$1 AND operations_fault_receipt_set=$2`, e.ID, e.ReceiptSet)
	if err != nil {
		return unavailable(err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return ErrConflict
	}
	return operationsActor(ctx, tx, p, "owner")
}

func (j *OperationsJournal) classifyEmergency(state *operationsJournalState, e OperationsEmergency, review OperationsEmergencyReview) error {
	if !slices.Contains(e.Causes, operationsUnclassifiedFault) {
		if len(review.ClassifiedCauses) != 0 {
			return auth.ErrForbidden
		}
		return nil
	}
	if len(review.ClassifiedCauses) == 1 && review.ClassifiedCauses[0] == "no_reportable_fault" {
		return nil
	}
	if len(review.ClassifiedCauses) == 0 || len(review.ClassifiedCauses) > len(recoverableOperationsFaults) {
		return auth.ErrForbidden
	}
	seen := make(map[OperationsFault]bool, len(review.ClassifiedCauses))
	for _, kind := range review.ClassifiedCauses {
		incident, err := operationsFaultIncident(kind, e.StartedAt)
		if err != nil || seen[kind] {
			return auth.ErrForbidden
		}
		seen[kind] = true
		if kind == OperationsBackupFault || kind == OperationsProviderFault {
			// The signed review gives a conservative latest bound when the
			// newly classified occurrence's precise onset was not recorded.
			observed := review.ObservedAt.UTC()
			incident.LatestObservedAt = &observed
		}
		j.upsertFault(state, kind, incident)
	}
	return nil
}

// The exclusive journal lease and SQL stop remain held until recovery commits.
func (j *OperationsJournal) clearReviewedFence(reviewed operationsFaultFiles) error {
	current, err := j.faultFiles()
	if err != nil || current.set != reviewed.set {
		return auth.ErrUnavailable
	}
	for _, file := range reviewed.files {
		current, err := j.readFaultFile(file.kind)
		if err != nil || current.stamp != file.stamp || os.Remove(j.faultReceiptPath(file.kind)) != nil {
			return auth.ErrUnavailable
		}
	}
	if current, err := j.faultFiles(); err != nil || len(current.files) != 0 {
		return auth.ErrUnavailable
	}
	for _, name := range []string{"journal.pending", "journal.uncertain", "journal.queued"} {
		if err := os.Remove(filepath.Join(j.cfg.Directory, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return auth.ErrUnavailable
		}
	}
	if err := j.syncDirectory(); err != nil {
		return err
	}
	for _, offset := range []int64{0, 1} {
		if err := j.writeFence(offset, 0); err != nil {
			return err
		}
	}
	return j.verifyWriteHealth()
}

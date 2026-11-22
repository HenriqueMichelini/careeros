package accountstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// SyntheticResidualObserver is an explicit fake-evidence seam. No environment,
// customer route or production recipient contract constructs it.
type SyntheticResidualObserver func(context.Context, LifecycleResidualObligation) (LifecycleResidualEvidence, error)
type LifecycleResidualObligation struct {
	AccountID        string     `json:"accountId"`
	OperationID      string     `json:"operationId"`
	Class            string     `json:"class"`
	OriginalFencedAt time.Time  `json:"originalFencedAt"`
	Deadline         *time.Time `json:"deadline,omitempty"`
	State            string     `json:"state"`
	Revision         int64      `json:"revision"`
	EvidenceID       string     `json:"evidenceId"`
}
type LifecycleResidualEvidence struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	State    string `json:"state"`
}
type ResidualEvidenceFence struct {
	AccountID   string                    `json:"accountId"`
	OperationID string                    `json:"operationId"`
	Class       string                    `json:"class"`
	Evidence    LifecycleResidualEvidence `json:"evidence"`
	Binding     []byte                    `json:"binding"`
}

func residualEvidenceBinding(owner, operation, class string, e LifecycleResidualEvidence) []byte {
	raw, _ := json.Marshal(struct {
		Owner, Operation, Class string
		Evidence                LifecycleResidualEvidence
	}{owner, operation, class, e})
	hash := sha256.Sum256(raw)
	return hash[:]
}
func validResidualEvidence(e LifecycleResidualEvidence) bool {
	return validOperation(e.ID) && e.Revision >= 1 && e.Revision <= 9007199254740991 && (e.State == "pending" || e.State == "failed" || e.State == "verified_unrecoverable")
}
func (w *LifecycleWorker) observeResiduals(ctx context.Context, owner string) error {
	if w.residual == nil {
		return nil
	}
	rows, err := w.store.db.QueryContext(ctx, `SELECT account_id,operation_id,class,original_fenced_at,deadline,state,revision,evidence_id FROM careeros_lifecycle_residuals WHERE account_id=$1 AND state<>'verified_unrecoverable' ORDER BY operation_id,class LIMIT 16`, owner)
	if err != nil {
		return unavailable(err)
	}
	var obligations []LifecycleResidualObligation
	for rows.Next() {
		var o LifecycleResidualObligation
		var deadline sql.NullTime
		if err = rows.Scan(&o.AccountID, &o.OperationID, &o.Class, &o.OriginalFencedAt, &deadline, &o.State, &o.Revision, &o.EvidenceID); err != nil {
			break
		}
		if deadline.Valid {
			o.Deadline = &deadline.Time
		}
		obligations = append(obligations, o)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return unavailable(err)
	}
	for _, o := range obligations {
		e, err := w.residual(ctx, o)
		if err != nil {
			continue
		}
		if !validResidualEvidence(e) {
			return ErrValidation
		}
		if err = w.recordResidual(ctx, o, e); err != nil {
			return err
		}
	}
	return nil
}
func (w *LifecycleWorker) recordResidual(ctx context.Context, o LifecycleResidualObligation, e LifecycleResidualEvidence) error {
	tx, err := w.store.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if err = lockPilot(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE`, o.AccountID); err != nil {
		return unavailable(err)
	}
	var anchor time.Time
	var revision int64
	var oldEvidence string
	if err = tx.QueryRowContext(ctx, `SELECT original_fenced_at,revision,evidence_id FROM careeros_lifecycle_residuals WHERE account_id=$1 AND operation_id=$2 AND class=$3 FOR UPDATE`, o.AccountID, o.OperationID, o.Class).Scan(&anchor, &revision, &oldEvidence); err != nil {
		return unavailable(err)
	}
	if !anchor.Equal(o.OriginalFencedAt) {
		return ErrConflict
	}
	hash := residualEvidenceBinding(o.AccountID, o.OperationID, o.Class, e)
	var prior []byte
	err = tx.QueryRowContext(ctx, `SELECT request_hash FROM careeros_lifecycle_residual_evidence WHERE account_id=$1 AND operation_id=$2 AND class=$3 AND evidence_id=$4`, o.AccountID, o.OperationID, o.Class, e.ID).Scan(&prior)
	if err == nil {
		if !bytes.Equal(prior, hash) {
			return ErrConflict
		}
		return unavailable(tx.Commit())
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return unavailable(err)
	}
	if e.Revision < revision || e.Revision == revision && oldEvidence != "" {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_lifecycle_residual_evidence VALUES($1,$2,$3,$4,$5,$6,$7)`, o.AccountID, o.OperationID, o.Class, e.ID, e.Revision, e.State, hash); err != nil {
		return unavailable(err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE careeros_lifecycle_residuals SET state=$4,revision=$5,evidence_id=$6 WHERE account_id=$1 AND operation_id=$2 AND class=$3`, o.AccountID, o.OperationID, o.Class, e.State, e.Revision, e.ID); err != nil {
		return unavailable(err)
	}
	return unavailable(tx.Commit())
}

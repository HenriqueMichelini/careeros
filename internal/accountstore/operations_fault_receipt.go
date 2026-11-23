package accountstore

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"professional-information-repo/internal/auth"
)

var recoverableOperationsFaults = []OperationsFault{
	OperationsDatabaseFault, OperationsAuthorityFault, OperationsControlFault,
	OperationsAuthFault, OperationsProviderFault, OperationsBackupFault,
}

type operationsFaultReceipt struct {
	Kind         OperationsFault `json:"kind"`
	At           time.Time       `json:"at"`
	LatestAt     time.Time       `json:"latestAt"`
	Unclassified bool            `json:"unclassified,omitempty"`
}

func (j *OperationsJournal) faultReceiptPath(kind OperationsFault) string {
	return filepath.Join(j.cfg.Directory, "journal.fault."+string(kind))
}

func (j *OperationsJournal) faultReceiptBinding(raw []byte) string {
	return j.binding(append([]byte("fault-receipt\x00"+j.id+"\x00"), raw...))
}

// A bounded per-cause file survives a failed main journal update. Holding
// the entry barrier orders it against future leases and emergency clearance.
func (j *OperationsJournal) retainFault(ctx context.Context, receipt operationsFaultReceipt) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	unlock, err := j.lockAdmission(ctx, true)
	if err != nil {
		return err
	}
	defer unlock()
	receipt.LatestAt = receipt.At
	prior, err := j.readFaultReceipt(receipt.Kind)
	switch {
	case err == nil:
		receipt.Unclassified = prior.Unclassified
		if prior.At.Before(receipt.At) {
			receipt.At = prior.At
		}
		if prior.LatestAt.After(receipt.LatestAt) {
			receipt.LatestAt = prior.LatestAt
		}
	case !errors.Is(err, os.ErrNotExist):
		// A newer complete receipt preserves uncertainty about older bytes.
		receipt.Unclassified = true
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		return auth.ErrUnavailable
	}
	raw, err := json.Marshal(operationsEnvelope{Payload: payload, Binding: j.faultReceiptBinding(payload)})
	if err != nil || len(raw) > 512 {
		return auth.ErrUnavailable
	}
	return j.writeAtomic(j.faultReceiptPath(receipt.Kind), raw)
}

func (j *OperationsJournal) readFaultReceipt(kind OperationsFault) (operationsFaultReceipt, error) {
	var receipt operationsFaultReceipt
	f, err := os.OpenFile(j.faultReceiptPath(kind), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return receipt, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 512 {
		return receipt, auth.ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(f, 513))
	var envelope operationsEnvelope
	if err != nil || len(raw) > 512 || json.Unmarshal(raw, &envelope) != nil ||
		!hmac.Equal([]byte(envelope.Binding), []byte(j.faultReceiptBinding(envelope.Payload))) ||
		json.Unmarshal(envelope.Payload, &receipt) != nil || receipt.Kind != kind || receipt.At.IsZero() {
		return operationsFaultReceipt{}, auth.ErrUnavailable
	}
	if receipt.LatestAt.IsZero() {
		receipt.LatestAt = receipt.At // Complete receipts from the earlier format.
	}
	if receipt.LatestAt.Before(receipt.At) {
		return operationsFaultReceipt{}, auth.ErrUnavailable
	}
	return receipt, nil
}

func (j *OperationsJournal) mirrorFaultReceipts(state *operationsJournalState) (known, unclassified bool, latest time.Time) {
	for _, kind := range recoverableOperationsFaults {
		receipt, err := j.readFaultReceipt(kind)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			unclassified = true
			continue
		}
		incident, err := operationsFaultIncident(kind, receipt.At)
		if err != nil {
			unclassified = true
			continue
		}
		incident.LatestObservedAt = &receipt.LatestAt
		j.upsertFault(state, kind, incident)
		known = true
		unclassified = unclassified || receipt.Unclassified
		if receipt.LatestAt.After(latest) {
			latest = receipt.LatestAt
		}
	}
	return known, unclassified, latest
}

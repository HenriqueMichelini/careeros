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
	Kind OperationsFault `json:"kind"`
	At   time.Time       `json:"at"`
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
	payload, err := json.Marshal(receipt)
	if err != nil {
		return auth.ErrUnavailable
	}
	file := j.faultReceiptPath(receipt.Kind)
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if errors.Is(err, os.ErrExist) {
		_, err = j.readFaultReceipt(receipt.Kind)
		return err
	}
	if err != nil {
		return auth.ErrUnavailable
	}
	err = json.NewEncoder(f).Encode(operationsEnvelope{Payload: payload, Binding: j.faultReceiptBinding(payload)})
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return auth.ErrUnavailable
	}
	return j.syncDirectory()
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
	return receipt, nil
}

func (j *OperationsJournal) mirrorFaultReceipts(state *operationsJournalState) (bool, error) {
	var known bool
	for _, kind := range recoverableOperationsFaults {
		receipt, err := j.readFaultReceipt(kind)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, auth.ErrUnavailable
		}
		incident, err := operationsFaultIncident(kind, receipt.At)
		if err != nil {
			return false, err
		}
		j.upsertFault(state, kind, incident)
		known = true
	}
	return known, nil
}

package accountstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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

var operationsFallbackMarkers = []string{"journal.pending", "journal.uncertain", "journal.queued"}

type operationsFaultReceipt struct {
	Kind         OperationsFault `json:"kind"`
	At           time.Time       `json:"at"`
	LatestAt     time.Time       `json:"latestAt"`
	Unclassified bool            `json:"unclassified,omitempty"`
}

type operationsFaultFile struct {
	name    string
	stamp   string
	receipt operationsFaultReceipt
}

type operationsFaultFiles struct {
	files        []operationsFaultFile
	set          string
	known        bool
	unclassified bool
	latest       time.Time
}

func (j *OperationsJournal) faultReceiptPath(kind OperationsFault) string {
	return filepath.Join(j.cfg.Directory, faultReceiptName(kind))
}

func faultReceiptName(kind OperationsFault) string { return "journal.fault." + string(kind) }

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
	file, err := j.readFaultFile(kind)
	if err != nil {
		return operationsFaultReceipt{}, err
	}
	if file.receipt.Kind == "" {
		return operationsFaultReceipt{}, auth.ErrUnavailable
	}
	return file.receipt, nil
}

// An unreadable file contributes only its trusted local identity and any
// bounded bytes that can be read. Its filename never assigns the fault cause.
func (j *OperationsJournal) readFaultFile(kind OperationsFault) (operationsFaultFile, error) {
	file, raw, err := j.readRecoveryFile(faultReceiptName(kind))
	if err == nil && raw != nil {
		file.receipt = j.decodeFaultReceipt(kind, raw)
	}
	return file, err
}

func (j *OperationsJournal) readRecoveryFile(name string) (operationsFaultFile, []byte, error) {
	file := operationsFaultFile{name: name}
	path := filepath.Join(j.cfg.Directory, name)
	info, err := os.Lstat(path)
	if err != nil {
		return file, nil, err
	}
	stamp, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return file, nil, auth.ErrUnavailable
	}
	metadata := fmt.Sprintf("recovery-file\x00%s\x00%d\x00%d\x00%d\x00%d\x00%d", name, stamp.Dev, stamp.Ino,
		info.Mode(), info.Size(), info.ModTime().UnixNano())
	file.stamp = j.binding([]byte(metadata))
	if !info.Mode().IsRegular() || info.Size() > 512 {
		return file, nil, nil
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return file, nil, nil
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !sameFaultFile(info, opened) {
		return file, nil, auth.ErrUnavailable
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, 513))
	current, err := os.Lstat(path)
	if err != nil || !sameFaultFile(info, current) {
		return file, nil, auth.ErrUnavailable
	}
	if readErr != nil || len(raw) > 512 {
		return file, nil, nil
	}
	hash := sha256.Sum256(raw)
	file.stamp = j.binding(append([]byte(metadata+"\x00"), hash[:]...))
	if info.Mode().Perm()&0077 != 0 {
		return file, nil, nil
	}
	return file, raw, nil
}

func sameFaultFile(expected, current os.FileInfo) bool {
	return os.SameFile(expected, current) && expected.Mode() == current.Mode() &&
		expected.Size() == current.Size() && expected.ModTime().Equal(current.ModTime())
}

func (j *OperationsJournal) decodeFaultReceipt(kind OperationsFault, raw []byte) operationsFaultReceipt {
	var receipt operationsFaultReceipt
	var envelope operationsEnvelope
	if json.Unmarshal(raw, &envelope) != nil ||
		!hmac.Equal([]byte(envelope.Binding), []byte(j.faultReceiptBinding(envelope.Payload))) ||
		json.Unmarshal(envelope.Payload, &receipt) != nil || receipt.Kind != kind || receipt.At.IsZero() {
		return operationsFaultReceipt{}
	}
	if receipt.LatestAt.IsZero() {
		receipt.LatestAt = receipt.At // Complete receipts from the earlier format.
	}
	if receipt.LatestAt.Before(receipt.At) {
		return operationsFaultReceipt{}
	}
	return receipt
}

func (j *OperationsJournal) faultFiles() (operationsFaultFiles, error) {
	var observed operationsFaultFiles
	var stamps []byte
	for _, name := range append([]string{"journal.fence"}, operationsFallbackMarkers...) {
		file, raw, err := j.readRecoveryFile(name)
		if errors.Is(err, os.ErrNotExist) && name != "journal.fence" ||
			err == nil && name == "journal.fence" && len(raw) == 2 && raw[0] == 0 && raw[1] == 0 {
			continue
		}
		if err != nil {
			return operationsFaultFiles{}, auth.ErrUnavailable
		}
		observed.files = append(observed.files, file)
		stamps = append(stamps, file.stamp...)
		// A marker records loss of certainty, with no authenticated cause.
		// A separate typed receipt cannot prove it covers this generation.
		observed.unclassified = true
	}
	for _, kind := range recoverableOperationsFaults {
		file, err := j.readFaultFile(kind)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return operationsFaultFiles{}, auth.ErrUnavailable
		}
		observed.files = append(observed.files, file)
		stamps = append(stamps, file.stamp...)
		receipt := file.receipt
		observed.known = observed.known || receipt.Kind != ""
		observed.unclassified = observed.unclassified || receipt.Kind == "" || receipt.Unclassified
		if receipt.LatestAt.After(observed.latest) {
			observed.latest = receipt.LatestAt
		}
	}
	if len(stamps) != 0 {
		observed.set = j.binding(append([]byte("recovery-files\x00"), stamps...))
	}
	return observed, nil
}

func (j *OperationsJournal) mirrorFaultReceipts(state *operationsJournalState, observed operationsFaultFiles) error {
	for _, file := range observed.files {
		receipt := file.receipt
		if receipt.Kind == "" {
			continue
		}
		incident, err := operationsFaultIncident(receipt.Kind, receipt.At)
		if err != nil {
			return err
		}
		incident.LatestObservedAt = &receipt.LatestAt
		j.upsertFault(state, receipt.Kind, incident)
	}
	return nil
}

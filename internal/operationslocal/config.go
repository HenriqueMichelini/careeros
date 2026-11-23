// Package operationslocal composes explicit synthetic CLI adapters.
package operationslocal

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

func JournalConfig(output io.Writer) (accountstore.OperationsJournalConfig, bool, error) {
	directory, keyText := os.Getenv("CAREEROS_OPERATIONS_JOURNAL_DIRECTORY"), os.Getenv("CAREEROS_OPERATIONS_JOURNAL_KEY")
	if directory == "" && keyText == "" {
		return accountstore.OperationsJournalConfig{}, false, nil
	}
	raw, err := hex.DecodeString(keyText)
	if directory == "" || err != nil || len(raw) != 32 {
		return accountstore.OperationsJournalConfig{}, true, accountstore.ErrValidation
	}
	var key [32]byte
	copy(key[:], raw)
	return accountstore.OperationsJournalConfig{Directory: directory, BindingKey: key,
		Notify: func(_ context.Context, n accountstore.OperationsNotification) accountstore.OperationsDelivery {
			if json.NewEncoder(output).Encode(n) != nil {
				return accountstore.OperationsFailed
			}
			return accountstore.OperationsDelivered
		}}, true, nil
}

type RecoveryStatement struct {
	Purpose, Reference, Incident, Episode, Result string
	Revision                                      int
	ObservedAt                                    time.Time
}

type RecoveryVerifier struct{ statement RecoveryStatement }

// Recovery requires a bounded artifact signed by the independently configured
// synthetic verification key. The operator's command supplies only a reference.
func Recovery() (RecoveryVerifier, error) {
	path, keyText := os.Getenv("CAREEROS_OPERATIONS_RECOVERY_ARTIFACT"), os.Getenv("CAREEROS_OPERATIONS_RECOVERY_PUBLIC_KEY")
	if path == "" && keyText == "" {
		return RecoveryVerifier{}, nil
	}
	key, err := hex.DecodeString(keyText)
	if !filepath.IsAbs(path) || err != nil || len(key) != ed25519.PublicKeySize {
		return RecoveryVerifier{}, accountstore.ErrValidation
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != filepath.Clean(path) {
		return RecoveryVerifier{}, accountstore.ErrValidation
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return RecoveryVerifier{}, accountstore.ErrValidation
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return RecoveryVerifier{}, accountstore.ErrValidation
	}
	raw, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || len(raw) > 8192 {
		return RecoveryVerifier{}, accountstore.ErrValidation
	}
	var envelope struct {
		Payload   json.RawMessage
		Signature []byte
	}
	if !strictDecode(raw, &envelope) || !ed25519.Verify(ed25519.PublicKey(key), envelope.Payload, envelope.Signature) {
		return RecoveryVerifier{}, auth.ErrForbidden
	}
	var statement RecoveryStatement
	if !strictDecode(envelope.Payload, &statement) || statement.ObservedAt.IsZero() {
		return RecoveryVerifier{}, accountstore.ErrValidation
	}
	return RecoveryVerifier{statement: statement}, nil
}

func strictDecode(raw []byte, dst any) bool {
	if !accountstore.StrictJSON(raw) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst) == nil
}

func (v RecoveryVerifier) VerifyBackup(_ context.Context, ref string, incident accountstore.OperationsIncident) (time.Time, error) {
	s := v.statement
	if s.Purpose != "backup" || s.Reference != ref || s.Incident != incident.ID || s.Revision != incident.Revision ||
		s.Episode != "" || s.Result != "synthetic_restore_check_passed" {
		return time.Time{}, auth.ErrForbidden
	}
	return s.ObservedAt, nil
}

func (v RecoveryVerifier) VerifyService(_ context.Context, ref, episode string) (time.Time, error) {
	s := v.statement
	if s.Purpose != "service" || s.Reference != ref || s.Episode != episode || s.Incident != "" || s.Revision != 0 ||
		s.Result != "synthetic_service_probe_passed" {
		return time.Time{}, auth.ErrForbidden
	}
	return s.ObservedAt, nil
}

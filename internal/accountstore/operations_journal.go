package accountstore

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"professional-information-repo/internal/auth"
)

type OperationsDelivery string

const (
	OperationsDelivered OperationsDelivery = "delivered"
	OperationsFailed    OperationsDelivery = "known_failure"
	OperationsUnknown   OperationsDelivery = "unknown"
)

type OperationsNotification struct {
	ID, Kind, Owner, Response string
	Revision                  int
}
type OperationsJournalConfig struct {
	Directory  string
	BindingKey [32]byte
	Notify     func(context.Context, OperationsNotification) OperationsDelivery
	Clock      func() time.Time
	MaxBytes   int64
}

// OperationsJournal is an explicitly configured, owned local metadata journal.
// It discovers neither addresses nor an external mail adapter.
type OperationsJournal struct {
	cfg  OperationsJournalConfig
	id   string
	name string
}
type operationsJournalRecord struct {
	Incident        OperationsIncident `json:"incident"`
	Delivery        string             `json:"delivery"`
	FirstAttempt    time.Time          `json:"firstAttempt,omitempty"`
	Attempt         int                `json:"attempt,omitempty"`
	Revision        int                `json:"revision"`
	Contained       bool               `json:"contained,omitempty"`
	AbsenceStarts   time.Time          `json:"absenceStarts,omitempty"`
	AbsenceUntil    time.Time          `json:"absenceUntil,omitempty"`
	RecoveryAt      *time.Time         `json:"recoveryAt,omitempty"`
	RecoveryBinding string             `json:"recoveryBinding,omitempty"`
}
type operationsJournalState struct {
	Version             int                                `json:"version"`
	ID                  string                             `json:"id"`
	Records             map[string]operationsJournalRecord `json:"records"`
	Figures             *OperationsFigures                 `json:"figures,omitempty"`
	Persistence         string                             `json:"persistence,omitempty"`
	Backup              string                             `json:"backup,omitempty"`
	Public              *OperationsPublicStatus            `json:"public,omitempty"`
	PublicEpisode       *operationsPublicEpisode           `json:"publicEpisode,omitempty"`
	Rolling             map[string]time.Time               `json:"rolling,omitempty"`
	RollingRecoveries   map[string]rollingRecovery         `json:"rollingRecoveries,omitempty"`
	LastDurableSequence int64                              `json:"lastDurableSequence,omitempty"`
	FenceInstalled      bool                               `json:"fenceInstalled,omitempty"`
	OfflineOwner        []byte                             `json:"offlineOwner,omitempty"`
}
type operationsEnvelope struct {
	Payload json.RawMessage `json:"payload"`
	Binding string          `json:"binding"`
}

func OpenLocalOperationsJournal(cfg OperationsJournalConfig) (*OperationsJournal, error) {
	j, err := openOperationsJournal(cfg, "journal")
	if err != nil {
		return nil, err
	}
	if err = j.transaction(context.Background(), false, func(state *operationsJournalState) error {
		if state.FenceInstalled {
			j.id = state.ID
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if j.id != "" {
		return j, nil
	}
	if err = j.transaction(context.Background(), true, func(state *operationsJournalState) error {
		if state.ID == "" {
			state.ID, err = newID()
		}
		if err == nil && !state.FenceInstalled {
			err = j.installFence()
			state.FenceInstalled = err == nil
		}
		j.id = state.ID
		return err
	}); err != nil {
		return nil, err
	}
	return j, nil
}

func openOperationsJournal(cfg OperationsJournalConfig, name string) (*OperationsJournal, error) {
	if cfg.BindingKey == ([32]byte{}) || !filepath.IsAbs(cfg.Directory) {
		return nil, ErrValidation
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.MaxBytes == 0 {
		cfg.MaxBytes = 16 << 20
	}
	if cfg.MaxBytes < 512 || cfg.MaxBytes > 16<<20 {
		return nil, ErrValidation
	}
	resolved, err := filepath.EvalSymlinks(cfg.Directory)
	if err != nil || resolved != filepath.Clean(cfg.Directory) {
		return nil, ErrValidation
	}
	info, err := os.Lstat(cfg.Directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, ErrValidation
	}
	return &OperationsJournal{cfg: cfg, name: name}, nil
}
func (j *OperationsJournal) binding(raw []byte) string {
	m := hmac.New(sha256.New, j.cfg.BindingKey[:])
	_, _ = m.Write([]byte("local-operations-journal-v1\x00"))
	_, _ = m.Write(raw)
	return hex.EncodeToString(m.Sum(nil))
}
func (j *OperationsJournal) transaction(ctx context.Context, persist bool, apply func(*operationsJournalState) error) error {
	return j.withState(ctx, persist, false, apply)
}

func (j *OperationsJournal) containmentTransaction(ctx context.Context, apply func(*operationsJournalState) error) error {
	return j.withState(ctx, true, true, apply)
}

func (j *OperationsJournal) withState(ctx context.Context, persist, containment bool, apply func(*operationsJournalState) error) error {
	unlock, err := j.lock(ctx, persist)
	if err != nil {
		if containment {
			return errors.Join(err, j.queueEmergency(ctx))
		}
		return err
	}
	defer unlock()
	state, err := j.read()
	if err != nil {
		if containment {
			return errors.Join(err, j.sealFence(), j.sealIndependent())
		}
		return err
	}
	if err = apply(&state); err != nil || !persist {
		return err
	}
	if !containment {
		return j.write(state)
	}
	// Persist uncertainty independently of the in-place latch. A failed
	// latch write can leave its previous bytes readable.
	if err = j.checkPendingFence(); err != nil {
		return errors.Join(err, j.sealIndependent())
	}
	if err = j.createFenceFile("journal.pending"); err != nil {
		return errors.Join(err, j.sealFence(), j.sealIndependent())
	}
	if err = j.setFence(1); err != nil {
		return errors.Join(err, j.sealFence())
	}
	if err = j.write(state); err != nil {
		return errors.Join(err, j.sealFence())
	}
	if err = j.setFence(0); err != nil {
		return errors.Join(err, j.sealFence())
	}
	if err = os.Remove(filepath.Join(j.cfg.Directory, "journal.pending")); err != nil {
		return errors.Join(auth.ErrUnavailable, j.sealFence())
	}
	if err = j.syncDirectory(); err != nil {
		return errors.Join(err, j.sealFence())
	}
	return nil
}

func (j *OperationsJournal) syncDirectory() error {
	dir, err := os.Open(j.cfg.Directory)
	if err != nil {
		return auth.ErrUnavailable
	}
	defer dir.Close()
	if err = dir.Sync(); err != nil {
		return auth.ErrUnavailable
	}
	return nil
}

func (j *OperationsJournal) createFenceFile(name string) error {
	path := filepath.Join(j.cfg.Directory, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return auth.ErrUnavailable
	}
	_, err = f.Write([]byte{1})
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return auth.ErrUnavailable
	}
	return j.syncDirectory()
}

func (j *OperationsJournal) checkPendingFence() error {
	if err := j.checkFence(); err != nil {
		return err
	}
	for _, name := range []string{"journal.pending", "journal.uncertain", "journal.queued"} {
		_, err := os.Lstat(filepath.Join(j.cfg.Directory, name))
		if !errors.Is(err, os.ErrNotExist) {
			return auth.ErrUnavailable
		}
	}
	for _, kind := range recoverableOperationsFaults {
		if _, err := os.Lstat(j.faultReceiptPath(kind)); !errors.Is(err, os.ErrNotExist) {
			return auth.ErrUnavailable
		}
	}
	return nil
}

// Queue a stop against future lease decisions without modifying the latch or
// authenticated state underneath an already ordered protected operation.
func (j *OperationsJournal) queueEmergency(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	unlock, err := j.lockAdmission(ctx, true)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := os.Lstat(filepath.Join(j.cfg.Directory, "journal.queued")); err == nil {
		return nil
	}
	return j.createFenceFile("journal.queued")
}

func (j *OperationsJournal) sealIndependent() error {
	return j.createFenceFile("journal.uncertain")
}

// verifyWriteHealth uses the same data and directory durability as a
// required update without modifying the authenticated incident list.
func (j *OperationsJournal) verifyWriteHealth() error {
	if err := j.checkPendingFence(); err != nil {
		return err
	}
	id, err := newID()
	if err != nil {
		return auth.ErrUnavailable
	}
	name := ".journal-health-" + id
	defer os.Remove(filepath.Join(j.cfg.Directory, name))
	if err = j.createFenceFile(name); err != nil {
		return err
	}
	if err = os.Remove(filepath.Join(j.cfg.Directory, name)); err != nil {
		return auth.ErrUnavailable
	}
	if err = j.syncDirectory(); err != nil {
		return err
	}
	return j.checkPendingFence()
}

func (j *OperationsJournal) installFence() error {
	path := filepath.Join(j.cfg.Directory, "journal.fence")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if errors.Is(err, os.ErrExist) {
		// Replaying an interrupted install must preserve a closed marker.
		return j.checkFence()
	}
	if err != nil {
		return auth.ErrUnavailable
	}
	defer f.Close()
	if _, err = f.Write([]byte{0, 0}); err != nil {
		return auth.ErrUnavailable
	}
	if err = f.Sync(); err != nil {
		return auth.ErrUnavailable
	}
	return nil
}

func (j *OperationsJournal) openFence(flags int) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(j.cfg.Directory, "journal.fence"), flags|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, auth.ErrUnavailable
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != 2 {
		f.Close()
		return nil, auth.ErrUnavailable
	}
	return f, nil
}

func (j *OperationsJournal) checkFence() error {
	f, err := j.openFence(os.O_RDWR)
	if err != nil {
		return err
	}
	defer f.Close()
	var b [2]byte
	if _, err = io.ReadFull(f, b[:]); err != nil || b != ([2]byte{}) {
		return auth.ErrUnavailable
	}
	return nil
}

func (j *OperationsJournal) setFence(value byte) error {
	return j.writeFence(0, value)
}

func (j *OperationsJournal) sealFence() error {
	return j.writeFence(1, 1)
}

func (j *OperationsJournal) writeFence(offset int64, value byte) error {
	f, err := j.openFence(os.O_WRONLY)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.WriteAt([]byte{value}, offset); err != nil {
		return auth.ErrUnavailable
	}
	if err = f.Sync(); err != nil {
		return auth.ErrUnavailable
	}
	return nil
}

func (j *OperationsJournal) lock(ctx context.Context, exclusive bool) (func(), error) {
	return j.lockFile(ctx, j.name+".lock", exclusive)
}

func (j *OperationsJournal) lockAdmission(ctx context.Context, exclusive bool) (func(), error) {
	return j.lockFile(ctx, "journal.admission.lock", exclusive)
}

func (j *OperationsJournal) lockFile(ctx context.Context, name string, exclusive bool) (func(), error) {
	mode := syscall.LOCK_SH
	if exclusive {
		mode = syscall.LOCK_EX
	}
	path := filepath.Join(j.cfg.Directory, name)
	flags := os.O_RDONLY | syscall.O_NOFOLLOW
	if exclusive {
		flags = os.O_CREATE | os.O_RDWR | syscall.O_NOFOLLOW
	}
	lock, err := os.OpenFile(path, flags, 0600)
	if !exclusive && errors.Is(err, os.ErrNotExist) {
		lock, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	}
	if err != nil {
		return nil, auth.ErrUnavailable
	}
	info, err := lock.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		lock.Close()
		return nil, auth.ErrUnavailable
	}
	for {
		err = syscall.Flock(int(lock.Fd()), mode|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			lock.Close()
			return nil, auth.ErrUnavailable
		}
		select {
		case <-ctx.Done():
			lock.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return func() { _ = lock.Close() }, nil
}

func (j *OperationsJournal) read() (operationsJournalState, error) {
	state := operationsJournalState{Version: 1, Records: map[string]operationsJournalRecord{}}
	f, err := os.OpenFile(filepath.Join(j.cfg.Directory, j.name+".json"), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if errors.Is(err, os.ErrNotExist) {
		if j.id != "" && j.name == "journal" {
			return state, auth.ErrUnavailable
		}
		return state, nil
	}
	if err != nil {
		return state, auth.ErrUnavailable
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16<<20 {
		return state, auth.ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(f, 16<<20+1))
	var envelope operationsEnvelope
	if err != nil || len(raw) > 16<<20 || json.Unmarshal(raw, &envelope) != nil || !hmac.Equal([]byte(envelope.Binding), []byte(j.binding(append([]byte(j.name+"\x00"), envelope.Payload...)))) || json.Unmarshal(envelope.Payload, &state) != nil || state.Version != 1 || state.Records == nil || len(state.ID) != 32 || j.id != "" && state.ID != j.id {
		return state, auth.ErrUnavailable
	}
	return state, nil
}

func (j *OperationsJournal) write(state operationsJournalState) error {
	payload, err := json.Marshal(state)
	if err != nil {
		return auth.ErrUnavailable
	}
	raw, err := json.Marshal(operationsEnvelope{Payload: payload, Binding: j.binding(append([]byte(j.name+"\x00"), payload...))})
	if err != nil || int64(len(raw)) > j.cfg.MaxBytes {
		return auth.ErrUnavailable
	}
	var nonce [12]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return auth.ErrUnavailable
	}
	name := filepath.Join(j.cfg.Directory, ".journal-"+hex.EncodeToString(nonce[:]))
	file, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return auth.ErrUnavailable
	}
	defer os.Remove(name)
	if _, err = file.Write(raw); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil || os.Rename(name, filepath.Join(j.cfg.Directory, j.name+".json")) != nil {
		return auth.ErrUnavailable
	}
	return j.syncDirectory()
}
func (j *OperationsJournal) upsert(state *operationsJournalState, key string, incident OperationsIncident, recurring bool) bool {
	id := j.binding([]byte("incident\x00" + key))
	revision := 1
	if prior, ok := state.Records[id]; ok {
		if prior.Incident.State != "resolved" || !recurring {
			prior.Incident.Exposure = incident.Exposure
			state.Records[id] = prior
			return false
		}
		revision = prior.Revision + 1
	}
	incident.ID = id
	state.Records[id] = operationsJournalRecord{Incident: incident, Delivery: "pending", Revision: revision}
	return true
}
func (j *OperationsJournal) status(ctx context.Context) (OperationsStatus, error) {
	status := OperationsStatus{Incidents: []OperationsIncident{}, Persistence: "unobserved", Backup: "unobserved"}
	err := j.transaction(ctx, false, func(state *operationsJournalState) error {
		status = j.restrictedStatus(state)
		return nil
	})
	return status, err
}

func (j *OperationsJournal) restrictedStatus(state *operationsJournalState) OperationsStatus {
	status := OperationsStatus{Incidents: []OperationsIncident{}, Persistence: "unobserved", Backup: "unobserved", ContainmentUnavailable: j.checkPendingFence() != nil}
	status.Figures = state.Figures
	if state.Persistence != "" {
		status.Persistence = state.Persistence
	}
	if state.Backup != "" {
		status.Backup = state.Backup
	}
	for _, record := range state.Records {
		incident := record.Incident
		incident.Revision = record.Revision
		incident.Delivery = record.Delivery
		if incident.Delivery == "sending" {
			incident.Delivery = string(OperationsUnknown)
		}
		status.Incidents = append(status.Incidents, incident)
	}
	sort.Slice(status.Incidents, func(a, b int) bool { return status.Incidents[a].ID < status.Incidents[b].ID })
	return status
}

// WithOfflineOwnerStatus checks a separately provisioned capability while the
// list is locked. Runtime observer capability alone cannot read the list.
func (j *OperationsJournal) WithOfflineOwnerStatus(ctx context.Context, key [32]byte, write func(OperationsStatus) error) error {
	if key == ([32]byte{}) || write == nil {
		return auth.ErrForbidden
	}
	return j.transaction(ctx, false, func(state *operationsJournalState) error {
		digest := sha256.Sum256(key[:])
		if subtle.ConstantTimeCompare(state.OfflineOwner, digest[:]) != 1 {
			return auth.ErrForbidden
		}
		return write(j.restrictedStatus(state))
	})
}
func (j *OperationsJournal) deliver(ctx context.Context) error {
	if j.cfg.Notify == nil {
		return auth.ErrUnavailable
	}
	for {
		var selected operationsJournalRecord
		err := j.transaction(ctx, true, func(state *operationsJournalState) error {
			ids := make([]string, 0, len(state.Records))
			for id := range state.Records {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				record := state.Records[id]
				now := j.cfg.Clock().UTC()
				switch record.Delivery {
				case "pending":
					record.FirstAttempt = now
					record.Attempt = 1
				case string(OperationsFailed):
					slot := 0
					if !now.Before(record.FirstAttempt.Add(30 * time.Minute)) {
						slot = 3
					} else if !now.Before(record.FirstAttempt.Add(5 * time.Minute)) {
						slot = 2
					}
					if slot <= record.Attempt {
						continue
					}
					record.Attempt = slot
				default:
					continue
				}
				record.Delivery = "sending"
				selected = record
				state.Records[id] = record
				break
			}
			return nil
		})
		if err != nil || selected.Incident.ID == "" {
			return err
		}
		i := selected.Incident
		outcome := j.cfg.Notify(ctx, OperationsNotification{ID: i.ID, Kind: i.Kind, Owner: i.Owner, Response: i.Response, Revision: selected.Revision})
		err = j.transaction(context.WithoutCancel(ctx), true, func(state *operationsJournalState) error {
			record := state.Records[i.ID]
			if record.Delivery != "sending" || record.Revision != selected.Revision || record.Attempt != selected.Attempt {
				return nil
			}
			if outcome != OperationsDelivered && outcome != OperationsFailed {
				outcome = OperationsUnknown
			}
			record.Delivery = string(outcome)
			state.Records[i.ID] = record
			return nil
		})
		if err != nil {
			return err
		}
	}
}

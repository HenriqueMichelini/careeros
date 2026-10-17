package evaluationlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"professional-information-repo/internal/semanticeval"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const StoreVersion = "lab-store-v1"
const snapshotPrefix = "snapshot-"
const writerLock = "writer.lock"
const maxStoreFile = 64 << 20
const maxStoredRuns = 4096

var storeID = regexp.MustCompile(`^[a-f0-9]{32}$`)

// HistoryStore separates persistence acknowledgments from execution outcomes.
type HistoryStore interface {
	Save(RunRecord) error
	Health(context.Context) error
	Load(string) (RunRecord, error)
	Case(string, string) (ImportedCase, error)
	List(RunFilter) (RunPage, error)
	Close() error
}
type StoreOptions struct {
	// BeforeWrite is a local storage fault seam, never an execution adapter.
	BeforeWrite      func(operation string) error
	SoftWarningBytes int64
}
type RunSummary struct {
	Workflows        []string `json:"workflows,omitempty"`
	Languages        []string `json:"languages,omitempty"`
	Categories       []string `json:"categories,omitempty"`
	Outcomes         []string `json:"outcomes,omitempty"`
	Models           []string `json:"configuredModels,omitempty"`
	ReturnedModels   []string `json:"returnedModels,omitempty"`
	Versions         []string `json:"versions,omitempty"`
	Configuration    string   `json:"configuration,omitempty"`
	ID               string   `json:"id"`
	EngineState      string   `json:"engineState"`
	AssertionVerdict string   `json:"assertionVerdict"`
	Suite            string   `json:"suite"`
	Mode             string   `json:"mode"`
	Cases            int      `json:"cases"`
	Quarantined      bool     `json:"quarantined"`
}
type RunFilter struct {
	Workflow, Language, Category, Mode, Outcome, Model, ReturnedModel, Version string
	Suite, EngineState                                                         string
	Offset, Limit                                                              int
}
type RunPage struct {
	Runs     []RunSummary `json:"runs"`
	Next     int          `json:"next"`
	Problems []string     `json:"problems"`
}
type storeManifest struct {
	Kind         string            `json:"kind,omitempty"`
	Version      string            `json:"version"`
	Summary      RunSummary        `json:"summary"`
	Snapshot     string            `json:"snapshot"`
	Hash         string            `json:"hash"`
	CaseHashes   map[string]string `json:"caseHashes"`
	OriginalHash string            `json:"originalHash,omitempty"`
}
type FileStore struct {
	recovery  map[string]RunRecord
	mu        sync.Mutex
	root      *os.Root
	dir       string
	lock      *os.File
	options   StoreOptions
	manifests map[string]storeManifest
	problems  []string
	failure   error
	closed    bool
}

// DefaultStoreDirectory is application-owned user data, outside the checkout.
func DefaultStoreDirectory() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("user data directory must be absolute")
	}
	return filepath.Join(base, "careeros", "evaluation-lab"), nil
}
func OpenFileStore(dir string, options StoreOptions) (*FileStore, error) {
	if dir == "" {
		var err error
		dir, err = DefaultStoreDirectory()
		if err != nil {
			return nil, err
		}
	}
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("store directory must be absolute")
	}
	if err := rejectSymlinkParents(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	s := &FileStore{root: root, dir: dir, options: options, manifests: map[string]storeManifest{}, recovery: map[string]RunRecord{}}
	if options.SoftWarningBytes <= 0 {
		s.options.SoftWarningBytes = 512 << 20
	}
	if err = s.acquireLock(); err != nil {
		root.Close()
		return nil, err
	}
	if err = s.rebuild(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
func rejectSymlinkParents(path string) error {
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
			return fmt.Errorf("unsafe store directory")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}
func (s *FileStore) check() error {
	if s.closed {
		return fmt.Errorf("store closed")
	}
	return s.failure
}
func (s *FileStore) fail(err error) error {
	if err != nil && s.failure == nil {
		s.failure = fmt.Errorf("storage failure: %w", err)
	}
	return s.failure
}
func (s *FileStore) Health(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.check()
}
func (s *FileStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	err := s.lock.Close()
	return errors.Join(err, s.root.Close())
}
func (s *FileStore) guard(operation string) error {
	if err := s.check(); err != nil {
		return err
	}
	if s.options.BeforeWrite != nil {
		return s.options.BeforeWrite(operation)
	}
	return nil
}
func (s *FileStore) safe(path string) error {
	if filepath.IsAbs(path) || filepath.Clean(path) != path || path == "." {
		return fmt.Errorf("invalid store path")
	}
	parts := strings.Split(path, string(filepath.Separator))
	for i := range parts {
		if parts[i] == ".." || parts[i] == "" {
			return fmt.Errorf("invalid store path")
		}
		info, err := s.root.Lstat(filepath.Join(parts[:i+1]...))
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in history path")
		}
		if i < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("non-directory in history path")
		}
	}
	return nil
}
func (s *FileStore) read(path string, limit int64) ([]byte, error) {
	if err := s.safe(path); err != nil {
		return nil, err
	}
	f, err := s.root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("history file exceeds bounds or is not regular")
	}
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if len(raw) > int(limit) {
		return nil, fmt.Errorf("history file exceeds bounds")
	}
	return raw, err
}
func (s *FileStore) syncDir(path string) error {
	f, err := s.root.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func (s *FileStore) atomic(path string, raw []byte) error {
	if len(raw) > maxStoreFile {
		return fmt.Errorf("history file exceeds bounds")
	}
	if err := s.guard("write:" + filepath.Base(path)); err != nil {
		return err
	}
	parent := filepath.Dir(path)
	if err := s.safe(parent); err != nil {
		return err
	}
	tmp := filepath.Join(parent, "tmp-"+newLabID())
	f, err := s.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer s.root.Remove(tmp)
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if err = s.guard("rename:" + filepath.Base(path)); err != nil {
		return err
	}
	// Go 1.24 has no Root.Rename; use opened contained parent descriptors.
	if err = s.safe(parent); err != nil {
		return err
	}
	if err = s.rename(tmp, path); err != nil {
		return err
	}
	return s.syncDir(parent)
}
func (s *FileStore) Save(record RunRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(record)
}
func (s *FileStore) save(record RunRecord) error {
	if err := s.check(); err != nil {
		return err
	}
	if !storeID.MatchString(record.ID) {
		return fmt.Errorf("invalid run identity")
	}
	if err := ValidateResult(record); err != nil {
		return err
	}
	if existing, ok := s.manifests[record.ID]; ok && existing.Kind == "import" {
		return fmt.Errorf("immutable import cannot be overwritten")
	}
	if _, ok := s.manifests[record.ID]; !ok {
		if len(s.manifests) >= maxStoredRuns {
			return fmt.Errorf("history catalog capacity reached")
		}
		if err := s.guard("create-run"); err != nil {
			return s.fail(err)
		}
		if err := s.root.Mkdir(record.ID, 0700); err != nil {
			return s.fail(err)
		}
		if err := s.syncDir("."); err != nil {
			return s.fail(err)
		}
	}
	generation := snapshotPrefix + newLabID()
	if err := s.guard("create-snapshot"); err != nil {
		return s.fail(err)
	}
	if err := s.root.Mkdir(filepath.Join(record.ID, generation), 0700); err != nil {
		return s.fail(err)
	}
	record = cloneJSON(record)
	record.Durable = true
	manifest := storeManifest{Version: StoreVersion, Summary: RunSummary{ID: record.ID, EngineState: record.EngineState, AssertionVerdict: record.AssertionVerdict, Suite: record.Selection.Suite, Mode: record.Selection.Mode, Cases: len(record.Cases)}, Snapshot: generation, CaseHashes: map[string]string{}}
	manifest.Summary = summaryMetadata(manifest.Summary, record.Cases, record.Dispatches, []string{record.Version, record.Context.ConfigurationVersion, record.Context.RuntimeVersion, record.Context.PromptVersion, record.Context.SchemaVersion, record.Context.ScorerVersion, record.Context.StrategyVersion, record.Context.LabelHash, record.Provenance.Commit})
	manifest.Summary.Configuration = record.Selection.Configuration
	for _, c := range record.Cases {
		if !storeID.MatchString(c.Identity) {
			return fmt.Errorf("invalid case identity")
		}
		if _, exists := manifest.CaseHashes[c.Identity]; exists {
			return fmt.Errorf("duplicate case identity")
		}
		raw, err := json.Marshal(c)
		if err != nil {
			return err
		}
		manifest.CaseHashes[c.Identity] = semanticeval.Hash(raw)
		if err = s.atomic(filepath.Join(record.ID, generation, c.Identity+".json"), raw); err != nil {
			return s.fail(err)
		}
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	manifest.Hash = semanticeval.Hash(raw)
	if err = s.atomic(filepath.Join(record.ID, generation, "result.json"), raw); err != nil {
		return s.fail(err)
	}
	if err = s.appendCheckpoint(record); err != nil {
		return s.fail(err)
	}
	encoded, _ := json.Marshal(manifest)
	if err = s.atomic(filepath.Join(record.ID, "manifest.json"), encoded); err != nil {
		return s.fail(err)
	}
	s.manifests[record.ID] = manifest
	return nil
}
func (s *FileStore) manifest(id string) (storeManifest, error) {
	if !storeID.MatchString(id) {
		return storeManifest{}, fmt.Errorf("invalid run identity")
	}
	m, ok := s.manifests[id]
	if !ok {
		return m, os.ErrNotExist
	}
	return m, nil
}
func (s *FileStore) load(m storeManifest) (RunRecord, error) {
	if m.Kind != "" {
		return RunRecord{}, fmt.Errorf("history entry is an import")
	}
	raw, err := s.read(filepath.Join(m.Summary.ID, m.Snapshot, "result.json"), maxStoreFile)
	if err != nil {
		return RunRecord{}, err
	}
	if semanticeval.Hash(raw) != m.Hash {
		return RunRecord{}, fmt.Errorf("result hash mismatch")
	}
	var record RunRecord
	if err = json.Unmarshal(raw, &record); err != nil {
		return record, err
	}
	if err = ValidateResult(record); err != nil {
		return record, err
	}
	if record.ID != m.Summary.ID {
		return record, fmt.Errorf("identity mismatch")
	}
	return record, nil
}
func (s *FileStore) Load(id string) (RunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return RunRecord{}, fmt.Errorf("store closed")
	}
	m, err := s.manifest(id)
	if err != nil {
		return RunRecord{}, err
	}
	if recovered, ok := s.recovery[id]; ok {
		return cloneJSON(recovered), nil
	}
	return s.load(m)
}
func (s *FileStore) Case(id, caseID string) (ImportedCase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.caseEvidence(id, caseID)
}

// caseEvidence requires the store lock; private review bindings reuse the same
// immutable case bytes and integrity checks as the public history seam.
func (s *FileStore) caseEvidence(id, caseID string) (ImportedCase, error) {
	var c ImportedCase
	if s.closed {
		return c, fmt.Errorf("store closed")
	}
	if len(caseID) > 256 || caseID == "" {
		return c, fmt.Errorf("invalid case identity")
	}
	m, err := s.manifest(id)
	if err != nil {
		return c, err
	}
	hash, ok := m.CaseHashes[caseID]
	if !ok {
		return c, os.ErrNotExist
	}
	name := caseID
	if m.Kind == "import" {
		name = semanticeval.Hash([]byte(caseID))
	} else if !storeID.MatchString(caseID) {
		return c, fmt.Errorf("invalid case identity")
	}
	raw, err := s.read(filepath.Join(id, m.Snapshot, name+".json"), maxStoreFile)
	if err != nil {
		return c, err
	}
	if semanticeval.Hash(raw) != hash {
		return c, fmt.Errorf("case hash mismatch")
	}
	err = json.Unmarshal(raw, &c)
	return c, err
}
func (s *FileStore) List(filter RunFilter) (RunPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page := RunPage{Runs: []RunSummary{}, Problems: append([]string{}, s.problems...)}
	if s.closed {
		return page, fmt.Errorf("store closed")
	}
	if filter.Offset < 0 || filter.Limit < 1 || filter.Limit > 100 {
		return page, fmt.Errorf("invalid pagination")
	}
	ids := []string{}
	for id, m := range s.manifests {
		if matchesSummary(m.Summary, filter) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	start := min(filter.Offset, len(ids))
	end := min(start+filter.Limit, len(ids))
	for _, id := range ids[start:end] {
		page.Runs = append(page.Runs, s.manifests[id].Summary)
	}
	if end < len(ids) {
		page.Next = end
	}
	return page, nil
}
func (s *FileStore) rebuild() error {
	dir, err := s.root.Open(".")
	if err != nil {
		return err
	}
	entries, err := dir.ReadDir(maxStoredRuns + 3)
	dir.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) > maxStoredRuns+2 {
		return fmt.Errorf("history catalog exceeds bounds")
	}
	for _, entry := range entries {
		if entry.Name() == writerLock || entry.Name() == exposureDirectory {
			continue
		}
		if !storeID.MatchString(entry.Name()) || !entry.IsDir() {
			s.problems = append(s.problems, "unrecognized history entry")
			continue
		}
		s.rebuildEntry(entry.Name())
	}
	return nil
}
func validateManifest(m storeManifest, id string) error {
	if m.Version != StoreVersion || m.Summary.ID != id {
		return fmt.Errorf("invalid manifest identity/version")
	}
	if !strings.HasPrefix(m.Snapshot, snapshotPrefix) || !storeID.MatchString(strings.TrimPrefix(m.Snapshot, snapshotPrefix)) {
		return fmt.Errorf("invalid snapshot identity")
	}
	if len(m.CaseHashes) > 512 || m.Summary.Cases != len(m.CaseHashes) || len(m.Hash) != 64 {
		return fmt.Errorf("invalid manifest bounds")
	}
	if m.Kind != "" && m.Kind != "import" {
		return fmt.Errorf("unknown history kind")
	}
	if !contains([]string{"planned", "running", "completed", "failed", "canceled", "interrupted"}, m.Summary.EngineState) {
		return fmt.Errorf("invalid execution state")
	}
	return nil
}
func (s *FileStore) rebuildEntry(id string) {
	raw, err := s.read(filepath.Join(id, "manifest.json"), 1<<20)
	var m storeManifest
	if err == nil {
		err = json.Unmarshal(raw, &m)
	}
	if err == nil {
		err = validateManifest(m, id)
	}
	if err != nil {
		s.problems = append(s.problems, id+": invalid or incomplete history")
		return
	}
	s.manifests[id] = m
	if m.Kind == "import" {
		return
	}
	if _, err = s.readEvents(id); err != nil {
		s.problems = append(s.problems, id+": damaged checkpoint journal")
	}
	if m.Summary.EngineState != "planned" && m.Summary.EngineState != "running" {
		return
	}
	record, err := s.load(m)
	if err != nil {
		m.Summary.EngineState = "interrupted"
		m.Summary.AssertionVerdict = "blocked"
		s.manifests[id] = m
		s.problems = append(s.problems, id+": interrupted result unavailable")
		s.fail(err)
		return
	}
	s.recoverInterrupted(m, record)
}
func (s *FileStore) recoverInterrupted(m storeManifest, record RunRecord) {
	record.EngineState = "interrupted"
	record.AssertionVerdict = "blocked"
	record.UnavailableReasons = append(record.UnavailableReasons, "process_restart; no auto-resume")
	for i := range record.Dispatches {
		if record.Dispatches[i].State == "attempted" || record.Dispatches[i].State == "planned" {
			record.Dispatches[i].State = "unknown-final"
		}
	}
	if err := s.save(record); err == nil {
		return
	}
	record.Durable = false
	record.UnavailableReasons = append(record.UnavailableReasons, "interruption recovery could not be persisted")
	s.recovery[record.ID] = record
	m.Summary.EngineState = "interrupted"
	m.Summary.AssertionVerdict = "blocked"
	s.manifests[record.ID] = m
	s.problems = append(s.problems, record.ID+": interruption recovery not durable")
}

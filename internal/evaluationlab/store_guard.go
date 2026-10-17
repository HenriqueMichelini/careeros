package evaluationlab

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"professional-information-repo/internal/semanticeval"
	"strings"
)

const guardDirectory = "live-guard"

type guardHead struct {
	Version  string `json:"version"`
	Snapshot string `json:"snapshot"`
	Revision int    `json:"revision"`
}
type guardSnapshot struct {
	Version      string      `json:"version"`
	PreviousHash string      `json:"previousHash,omitempty"`
	Revision     int         `json:"revision"`
	Record       guardRecord `json:"record"`
}

func validateGuardRecord(record guardRecord) error {
	if record.Version != LiveGuardVersion || record.Preflight.Version != LiveGuardVersion || !storeID.MatchString(record.Preflight.ID) || record.Preflight.Hash != livePreflightHash(record.Preflight) || len(record.Calls) > len(record.Preflight.Dispatches) || record.Next < 0 || record.Next > len(record.Preflight.Dispatches) || !contains([]string{"planned", "authorized", "running", "completed", "failed", "canceled", "expired", "unknown-final"}, record.State) {
		return fmt.Errorf("invalid guard evidence")
	}
	if record.Comparison != nil && record.Comparison.Hash != guardComparisonHash(*record.Comparison) {
		return fmt.Errorf("comparison artifact changed")
	}
	for i, call := range record.Calls {
		if call.Spec != record.Preflight.Dispatches[i] {
			return fmt.Errorf("guard journal dispatch mismatch")
		}
	}
	return nil
}
func (s *FileStore) saveGuard(record guardRecord) (returnErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(); err != nil {
		return err
	}
	if err := validateGuardRecord(record); err != nil {
		return err
	}
	base, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if int64(len(base))+512 > record.Preflight.Plan.Caps.MaxArtifactBytes {
		return fmt.Errorf("per-run guard artifact cap exceeded")
	}
	if err := s.guard("guard-evidence"); err != nil {
		return err
	}
	parent := filepath.Join(guardDirectory, record.Preflight.ID)
	createdParent := false
	defer func() {
		if returnErr != nil && createdParent {
			_ = s.root.Remove(parent)
		}
	}()
	for _, directory := range []string{guardDirectory, parent} {
		if directory == parent {
			if _, statErr := s.root.Stat(parent); errors.Is(statErr, os.ErrNotExist) {
				listing, openErr := s.root.Open(guardDirectory)
				if openErr != nil {
					return openErr
				}
				entries, listErr := listing.ReadDir(129)
				listing.Close()
				if listErr != nil && !errors.Is(listErr, io.EOF) {
					return listErr
				}
				if len(entries) >= 128 {
					return fmt.Errorf("guard catalog capacity reached")
				}
			}
		}
		mkdirErr := s.root.Mkdir(directory, 0700)
		if mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
			return mkdirErr
		}
		if directory == parent && mkdirErr == nil {
			createdParent = true
		}
		if err := s.safe(directory); err != nil {
			return err
		}
	}
	head := guardHead{Version: LiveGuardVersion}
	prior, err := s.read(filepath.Join(parent, "head.json"), 1024)
	if err == nil {
		if err = json.Unmarshal(prior, &head); err != nil || head.Version != LiveGuardVersion || !evidenceHash.MatchString(head.Snapshot) || head.Revision < 1 || head.Revision >= 16384 {
			return fmt.Errorf("invalid guard journal head")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	snapshot := guardSnapshot{Version: LiveGuardVersion, PreviousHash: head.Snapshot, Revision: head.Revision + 1, Record: record}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if len(raw) > 1<<20 || snapshot.Revision > 16384 {
		return fmt.Errorf("guard record bounds exceeded")
	}
	var total, runTotal int64
	err = fs.WalkDir(s.root.FS(), guardDirectory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe guard journal entry")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsafe guard evidence")
		}
		total += info.Size()
		if strings.HasPrefix(path, parent+string(filepath.Separator)) {
			runTotal += info.Size()
		}
		if total+int64(len(raw))+1024 > 16<<20 {
			return fmt.Errorf("guard journal capacity reached")
		}
		return nil
	})
	if err != nil {
		return err
	}
	required := int64(len(raw)) + 1024
	if record.State == "running" && len(record.Calls) > 0 && record.Calls[len(record.Calls)-1].State == "unknown-final" {
		required = 3*int64(len(raw)) + 8192
	}
	if runTotal+required > record.Preflight.Plan.Caps.MaxArtifactBytes || total+required > 16<<20 {
		return fmt.Errorf("guard retention reservation exceeds capacity")
	}
	hash := semanticeval.Hash(raw)
	path := filepath.Join(parent, hash+".json")
	if prior, readErr := s.read(path, 1<<20); readErr == nil {
		if string(prior) != string(raw) {
			return fmt.Errorf("immutable guard snapshot mismatch")
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	} else if err = s.atomic(path, raw); err != nil {
		return err
	}
	updated, _ := json.Marshal(guardHead{Version: LiveGuardVersion, Snapshot: hash, Revision: snapshot.Revision})
	if err = s.atomic(filepath.Join(parent, "head.json"), updated); err != nil {
		return err
	}
	return s.syncDir(guardDirectory)
}
func (s *FileStore) loadGuardRecords() (map[string]guardRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := map[string]guardRecord{}
	if err := s.check(); err != nil {
		return nil, err
	}
	directory, err := s.root.Open(guardDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := directory.ReadDir(129)
	directory.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 128 {
		return nil, fmt.Errorf("guard catalog capacity exceeded")
	}
	for _, entry := range entries {
		id := entry.Name()
		if !storeID.MatchString(id) || !entry.IsDir() {
			return nil, fmt.Errorf("invalid guard catalog entry")
		}
		parent := filepath.Join(guardDirectory, id)
		raw, err := s.read(filepath.Join(parent, "head.json"), 1024)
		if errors.Is(err, os.ErrNotExist) {
			harmless, inspectionErr := s.uncommittedGuardPreflight(parent)
			if inspectionErr != nil {
				return nil, inspectionErr
			}
			if harmless {
				s.problems = append(s.problems, "Uncommitted guard preflight retained without executable authority.")
				continue
			}
		}
		if err != nil {
			return nil, err
		}
		var head guardHead
		if err = json.Unmarshal(raw, &head); err != nil || head.Version != LiveGuardVersion || !evidenceHash.MatchString(head.Snapshot) || head.Revision < 1 || head.Revision > 16384 {
			return nil, fmt.Errorf("invalid guard head")
		}
		raw, err = s.read(filepath.Join(parent, head.Snapshot+".json"), 1<<20)
		if err != nil {
			return nil, err
		}
		if semanticeval.Hash(raw) != head.Snapshot {
			return nil, fmt.Errorf("guard journal hash mismatch")
		}
		if err = validateJSON(raw); err != nil {
			return nil, err
		}
		var snapshot guardSnapshot
		if err = json.Unmarshal(raw, &snapshot); err != nil || snapshot.Version != LiveGuardVersion || snapshot.Revision != head.Revision || snapshot.Record.Preflight.ID != id {
			return nil, fmt.Errorf("guard snapshot identity mismatch")
		}
		if err = validateGuardRecord(snapshot.Record); err != nil {
			return nil, err
		}
		result[id] = snapshot.Record
	}
	return result, nil
}

func (s *FileStore) claimGuard(instance string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(); err != nil {
		return err
	}
	if s.guardOwner != "" {
		return fmt.Errorf("private store already has a guard owner")
	}
	s.guardOwner = instance
	return nil
}
func (s *FileStore) releaseGuard(instance string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.guardOwner == instance {
		s.guardOwner = ""
	}
}

func (s *FileStore) uncommittedGuardPreflight(parent string) (bool, error) {
	directory, err := s.root.Open(parent)
	if err != nil {
		return false, err
	}
	entries, err := directory.ReadDir(2)
	directory.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if len(entries) != 1 || entries[0].IsDir() {
		return false, nil
	}
	name := entries[0].Name()
	hash := strings.TrimSuffix(name, ".json")
	if name == hash || !evidenceHash.MatchString(hash) {
		return false, nil
	}
	raw, err := s.read(filepath.Join(parent, name), 1<<20)
	if err != nil {
		return false, err
	}
	if semanticeval.Hash(raw) != hash {
		return false, fmt.Errorf("uncommitted guard snapshot hash mismatch")
	}
	var snapshot guardSnapshot
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		return false, err
	}
	if err = validateGuardRecord(snapshot.Record); err != nil {
		return false, err
	}
	return snapshot.Version == LiveGuardVersion && snapshot.Revision == 1 && snapshot.PreviousHash == "" && snapshot.Record.State == "planned" && len(snapshot.Record.Calls) == 0 && snapshot.Record.Approval == nil && snapshot.Record.Comparison == nil, nil
}

package evaluationlab

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
)

type StoreUsage struct {
	Bytes            int64 `json:"bytes"`
	Files            int   `json:"files"`
	SoftWarning      bool  `json:"softWarning"`
	SoftWarningBytes int64 `json:"softWarningBytes"`
}

func (s *FileStore) Usage() (StoreUsage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	usage := StoreUsage{SoftWarningBytes: s.options.SoftWarningBytes}
	if s.closed {
		return usage, fmt.Errorf("store closed")
	}
	err := fs.WalkDir(s.root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		usage.Files++
		if usage.Files > 100000 {
			return fmt.Errorf("disk inventory exceeds bounds")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in disk inventory")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			usage.Bytes += info.Size()
		}
		return nil
	})
	usage.SoftWarning = usage.Bytes >= usage.SoftWarningBytes
	return usage, err
}

// Delete requires an explicit run identity; there is no automatic retention.
func (s *FileStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(); err != nil {
		return err
	}
	if _, err := s.manifest(id); err != nil {
		return err
	}
	if err := s.safe(id); err != nil {
		return err
	}
	if err := s.guard("delete-run"); err != nil {
		return s.fail(err)
	}
	// Rename first so a crash during recursive deletion cannot expose a partly
	// deleted run as valid. Tombstones remain visibly incomplete after restart.
	tombstone := "deleted-" + newLabID()
	if err := s.rename(id, tombstone); err != nil {
		return s.fail(err)
	}
	if err := s.syncDir("."); err != nil {
		return s.fail(err)
	}
	delete(s.manifests, id)
	if err := s.root.RemoveAll(tombstone); err != nil {
		return s.fail(err)
	}
	return s.fail(s.syncDir("."))
}

// ExportDiagnostics contains bounded metadata only, with no case artifacts,
// source, output, model prompts, original bytes or local paths.
func (s *FileStore) ExportDiagnostics(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("store closed")
	}
	m, err := s.manifest(id)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Version string     `json:"version"`
		Run     RunSummary `json:"run"`
	}{"lab-diagnostics-export-v1", m.Summary})
}

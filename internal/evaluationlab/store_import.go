package evaluationlab

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"professional-information-repo/internal/semanticeval"
	"strings"
)

// Import retains only explicitly supplied bounded bytes. Unknown hashes are
// quarantined; filesystem discovery and private release capture import do not exist.
func (s *FileStore) Import(raw []byte, dependencies [][]byte) (string, error) {
	imported, err := ImportWithDependencies(raw, dependencies)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = s.check(); err != nil {
		return "", err
	}
	if len(s.manifests) >= maxStoredRuns {
		return "", fmt.Errorf("history catalog capacity reached")
	}
	id := newLabID()
	generation := snapshotPrefix + newLabID()
	if err = s.guard("create-import"); err != nil {
		return "", s.fail(err)
	}
	if err = s.root.Mkdir(id, 0700); err != nil {
		return "", s.fail(err)
	}
	if err = s.syncDir("."); err != nil {
		return "", s.fail(err)
	}
	if err = s.root.Mkdir(filepath.Join(id, generation), 0700); err != nil {
		return "", s.fail(err)
	}
	manifest := storeManifest{Version: StoreVersion, Kind: "import", Summary: RunSummary{ID: id, EngineState: "completed", AssertionVerdict: "unavailable", Suite: imported.Format, Mode: "offline-import", Cases: len(imported.Cases), Quarantined: imported.Quarantined}, Snapshot: generation, OriginalHash: imported.OriginalHash, CaseHashes: map[string]string{}}
	manifest.Summary = summaryMetadata(manifest.Summary, imported.Cases, DispatchLedger(imported), []string{imported.Version, imported.OriginalVersion, imported.Derivation.Version, imported.Provenance.Commit})
	for _, c := range imported.Cases {
		encoded, _ := json.Marshal(c)
		manifest.CaseHashes[c.Identity] = semanticeval.Hash(encoded)
		if err = s.atomic(filepath.Join(id, generation, semanticeval.Hash([]byte(c.Identity))+".json"), encoded); err != nil {
			return "", s.fail(err)
		}
	}
	if err = s.atomic(filepath.Join(id, generation, "original.json"), imported.Original); err != nil {
		return "", s.fail(err)
	}
	encoded, err := json.Marshal(imported)
	if err != nil {
		return "", err
	}
	manifest.Hash = semanticeval.Hash(encoded)
	if err = s.atomic(filepath.Join(id, generation, "import.json"), encoded); err != nil {
		return "", s.fail(err)
	}
	encoded, _ = json.Marshal(manifest)
	if err = s.atomic(filepath.Join(id, "manifest.json"), encoded); err != nil {
		return "", s.fail(err)
	}
	s.manifests[id] = manifest
	return id, nil
}
func (s *FileStore) loadImport(m storeManifest) (ImportResult, error) {
	if m.Kind != "import" {
		return ImportResult{}, fmt.Errorf("history entry is a native run")
	}
	raw, err := s.read(filepath.Join(m.Summary.ID, m.Snapshot, "import.json"), maxStoreFile)
	if err != nil {
		return ImportResult{}, err
	}
	if semanticeval.Hash(raw) != m.Hash {
		return ImportResult{}, fmt.Errorf("import hash mismatch")
	}
	var imported ImportResult
	if err = json.Unmarshal(raw, &imported); err != nil {
		return imported, err
	}
	original, err := s.read(filepath.Join(m.Summary.ID, m.Snapshot, "original.json"), MaxImportBytes)
	if err != nil {
		return imported, err
	}
	if semanticeval.Hash(original) != m.OriginalHash || imported.OriginalHash != m.OriginalHash || semanticeval.Hash(imported.Original) != m.OriginalHash {
		return ImportResult{}, fmt.Errorf("original hash mismatch")
	}
	validated, err := revalidateImport(imported)
	if err != nil {
		return ImportResult{}, err
	}
	if validated.Quarantined != m.Summary.Quarantined {
		return ImportResult{}, fmt.Errorf("quarantine mismatch")
	}
	return validated, nil
}
func (s *FileStore) LoadImport(id string) (ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ImportResult{}, fmt.Errorf("store closed")
	}
	m, err := s.manifest(id)
	if err != nil {
		return ImportResult{}, err
	}
	return s.loadImport(m)
}
func (s *FileStore) ExportOriginal(id string) ([]byte, error) {
	imported, err := s.LoadImport(id)
	if err != nil {
		return nil, err
	}
	if imported.Quarantined {
		return nil, fmt.Errorf("quarantined artifacts cannot be exported")
	}
	return imported.Original, nil
}

// ExportMarkdown is a versioned synthetic evidence worksheet, not diagnostics.
func (s *FileStore) ExportMarkdown(id string) ([]byte, error) {
	if record, err := s.Load(id); err == nil {
		if err = validateSyntheticExport(record); err != nil {
			return nil, err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "# Synthetic run export\n\nVersion: lab-markdown-v1\n\nRun: %s\n\nExecution: %s; assertion: %s.\n\n", record.ID, record.EngineState, record.AssertionVerdict)
		for _, c := range record.Cases {
			fmt.Fprintf(&b, "## Case %s\n\nExecution: %s; assertion: %s; artifact SHA-256: %s.\n\n", c.ID, c.EngineState, c.AssertionVerdict, c.ArtifactHash)
		}
		return []byte(b.String()), nil
	}
	imported, err := s.LoadImport(id)
	if err != nil {
		return nil, err
	}
	if imported.Quarantined {
		return nil, fmt.Errorf("quarantined artifacts cannot be exported")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Synthetic evaluation export\n\nVersion: lab-markdown-v1\n\nOriginal SHA-256: %s\n\nDerivation: %s / %s\n\n", imported.OriginalHash, imported.Derivation.Version, imported.Derivation.Mode)
	for _, c := range imported.Cases {
		fmt.Fprintf(&b, "## Case %s\n\nExecution: %s; assertion: %s; operational outcome: %s.\n\n", c.ID, c.EngineState, c.AssertionVerdict, c.OperationalOutcome)
	}
	return []byte(b.String()), nil
}

const ExportVersion = "lab-synthetic-export-v1"

type syntheticExport struct {
	Native       *RunRecord `json:"nativeRun,omitempty"`
	Version      string     `json:"version"`
	Original     []byte     `json:"originalBytes"`
	Dependencies [][]byte   `json:"dependencies"`
}

func (s *FileStore) ExportJSON(id string) ([]byte, error) {
	if record, err := s.Load(id); err == nil {
		if err = validateSyntheticExport(record); err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(syntheticExport{Version: ExportVersion, Native: &record})
		if len(encoded) > MaxImportBytes {
			return nil, fmt.Errorf("native export exceeds round-trip import bounds")
		}
		return encoded, err
	}
	imported, err := s.LoadImport(id)
	if err != nil {
		return nil, err
	}
	if imported.Quarantined {
		return nil, fmt.Errorf("quarantined artifacts cannot be exported")
	}
	bundle := syntheticExport{Version: ExportVersion, Original: imported.Original, Dependencies: [][]byte{}}
	for _, dependency := range imported.Dependencies {
		bundle.Dependencies = append(bundle.Dependencies, dependency.Original)
	}
	return json.Marshal(bundle)
}
func (s *FileStore) ImportExport(raw []byte) (string, error) {
	if len(raw) == 0 || len(raw) > 16<<20 {
		return "", fmt.Errorf("export bundle exceeds bounds")
	}
	if err := validateJSON(raw); err != nil {
		return "", err
	}
	var bundle syntheticExport
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return "", err
	}
	if bundle.Version != ExportVersion {
		return "", fmt.Errorf("unsupported export version")
	}
	if bundle.Native != nil {
		if len(bundle.Original) != 0 || len(bundle.Dependencies) != 0 {
			return "", fmt.Errorf("mixed export kinds")
		}
		return s.Import(raw, nil)
	}
	return s.Import(bundle.Original, bundle.Dependencies)
}

// Native Save is an internal trusted runner seam, not an arbitrary HTTP upload
// interface. Imported native bundles are always quarantined until registered.
func validateSyntheticExport(record RunRecord) error {
	if err := ValidateResult(record); err != nil {
		return err
	}
	if record.Selection.Mode != "controlled" && record.Selection.Mode != "offline" && record.Selection.Mode != "replay" {
		return fmt.Errorf("only local synthetic runs may be exported")
	}
	for _, dispatch := range record.Dispatches {
		if dispatch.Origin != "simulated" {
			return fmt.Errorf("non-synthetic dispatch cannot be exported")
		}
	}
	return nil
}
func (s *FileStore) Artifact(id, caseID string) ([]byte, error) {
	detail, err := s.Case(id, caseID)
	if err != nil {
		return nil, err
	}
	if len(detail.Actual) > MaxImportBytes {
		return nil, fmt.Errorf("artifact exceeds bounds")
	}
	if detail.ArtifactHash != "" && semanticeval.Hash(detail.Actual) != detail.ArtifactHash {
		return nil, fmt.Errorf("artifact hash mismatch")
	}
	return detail.Actual, nil
}

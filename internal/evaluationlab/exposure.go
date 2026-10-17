package evaluationlab

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"professional-information-repo/internal/semanticeval"
	"regexp"
	"time"
)

const exposureDirectory = "exposure-ledger"
const exposureVersion = "lab-exposure-v1"

var evidenceHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

type CohortEntry struct {
	ContentHash    string `json:"contentHash"`
	HistoricalName string `json:"historicalName"`
	Split          string `json:"split"`
	ExposureGroup  string `json:"exposureGroup"`
	Language       string `json:"language"`
	Authorship     string `json:"authorship"`
}
type ExposureEvent struct {
	ContentHash string      `json:"contentHash"`
	Actor       ReviewActor `json:"actor"`
	Use         string      `json:"use"`
	At          string      `json:"at"`
}
type CohortStatus struct {
	Entry          CohortEntry     `json:"entry"`
	EffectiveSplit string          `json:"effectiveSplit"`
	Untouched      bool            `json:"untouched"`
	Events         []ExposureEvent `json:"events"`
}
type exposureLedger struct {
	Version string          `json:"version"`
	Entries []CohortEntry   `json:"entries"`
	Events  []ExposureEvent `json:"events"`
}

func (s *FileStore) loadExposure() (exposureLedger, error) {
	result := exposureLedger{Version: exposureVersion, Entries: []CohortEntry{}, Events: []ExposureEvent{}}
	if err := s.check(); err != nil {
		return result, err
	}
	raw, err := s.read(filepath.Join(exposureDirectory, "ledger.json"), 4<<20)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if err = validateJSON(raw); err != nil {
		return result, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&result); err != nil {
		return result, err
	}
	if result.Version != exposureVersion || len(result.Entries) > 4096 || len(result.Events) > 16384 {
		return result, fmt.Errorf("exposure ledger bounds/version mismatch")
	}
	seen := map[string]bool{}
	for _, entry := range result.Entries {
		if err = validateCohort(entry); err != nil {
			return result, err
		}
		if seen[entry.ContentHash] {
			return result, fmt.Errorf("duplicate cohort identity")
		}
		seen[entry.ContentHash] = true
	}
	for _, event := range result.Events {
		if err = validateExposure(event); err != nil {
			return result, err
		}
	}
	return result, nil
}
func validateCohort(entry CohortEntry) error {
	if !evidenceHash.MatchString(entry.ContentHash) || entry.HistoricalName == "" || len(entry.HistoricalName) > 256 || len(entry.ExposureGroup) > 256 || len(entry.Authorship) > 1024 || !contains([]string{"development", "exposed-regression", "newly-authored-blinded", "unknown"}, entry.Split) || !contains([]string{"en", "pt-BR", "unknown"}, entry.Language) {
		return fmt.Errorf("bounded cohort provenance required")
	}
	if entry.Split == "newly-authored-blinded" && (entry.Authorship == "" || entry.ExposureGroup == "") {
		return fmt.Errorf("newly authored cohort requires origin and exposure group")
	}
	return nil
}
func validateExposure(event ExposureEvent) error {
	if !evidenceHash.MatchString(event.ContentHash) || event.Actor.ID == "" || len(event.Actor.ID) > 128 || !contains([]string{"human", "agent", "model", "unknown"}, event.Actor.Type) || !contains([]string{"view", "export", "worksheet", "prompt-development", "model-development"}, event.Use) {
		return fmt.Errorf("invalid exposure event")
	}
	if _, err := time.Parse(time.RFC3339Nano, event.At); err != nil {
		return err
	}
	return nil
}
func (s *FileStore) saveExposure(ledger exposureLedger) error {
	if err := s.guard("record-exposure"); err != nil {
		return err
	}
	if err := s.root.Mkdir(exposureDirectory, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := s.safe(exposureDirectory); err != nil {
		return err
	}
	raw, err := json.Marshal(ledger)
	if err != nil {
		return err
	}
	if len(raw) > 4<<20 || len(ledger.Entries) > 4096 || len(ledger.Events) > 16384 {
		return fmt.Errorf("exposure ledger capacity reached; content remains blocked")
	}
	if err = s.atomic(filepath.Join(exposureDirectory, "ledger.json"), raw); err != nil {
		return err
	}
	return s.syncDir(".")
}
func (s *FileStore) RegisterCohort(entry CohortEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateCohort(entry); err != nil {
		return err
	}
	ledger, err := s.loadExposure()
	if err != nil {
		return err
	}
	for _, prior := range ledger.Entries {
		if prior.ContentHash == entry.ContentHash {
			if prior != entry {
				return fmt.Errorf("cohort declarations are immutable")
			}
			return nil
		}
	}
	ledger.Entries = append(ledger.Entries, entry)
	return s.saveExposure(ledger)
}
func (s *FileStore) RecordExposure(hash string, actor ReviewActor, use string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recordExposure(hash, actor, use)
}
func (s *FileStore) recordExposure(hash string, actor ReviewActor, use string) error {
	ledger, err := s.loadExposure()
	if err != nil {
		return err
	}
	event := ExposureEvent{ContentHash: hash, Actor: actor, Use: use, At: time.Now().UTC().Format(time.RFC3339Nano)}
	if err = validateExposure(event); err != nil {
		return err
	}
	ledger.Events = append(ledger.Events, event)
	return s.saveExposure(ledger)
}
func (s *FileStore) CohortStatus(hash string) (CohortStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ledger, err := s.loadExposure()
	if err != nil {
		return CohortStatus{}, err
	}
	result := CohortStatus{Entry: CohortEntry{ContentHash: hash, HistoricalName: "undeclared", Split: "unknown", Language: "unknown"}, EffectiveSplit: "unknown", Events: []ExposureEvent{}}
	for _, entry := range ledger.Entries {
		if entry.ContentHash == hash {
			result.Entry = entry
			result.EffectiveSplit = entry.Split
			break
		}
	}
	group := map[string]bool{hash: true}
	if result.Entry.ExposureGroup != "" {
		for _, entry := range ledger.Entries {
			if entry.ExposureGroup == result.Entry.ExposureGroup {
				group[entry.ContentHash] = true
			}
		}
	}
	for _, event := range ledger.Events {
		if group[event.ContentHash] {
			result.Events = append(result.Events, event)
		}
	}
	result.Untouched = result.Entry.Split == "newly-authored-blinded" && len(result.Events) == 0
	if len(result.Events) > 0 {
		result.EffectiveSplit = "exposed-regression"
	}
	return result, nil
}

// ExposeRun records source access before an HTTP response discloses content.
// Metadata-only history views do not call this method. Unknown sessions do not
// become human reviewers merely by opening a browser.
func (s *FileStore) ExposeRun(runID, caseID, use string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	manifest, err := s.manifest(runID)
	if err != nil {
		return err
	}
	ids := []string{caseID}
	if caseID == "" {
		ids = []string{}
		for id := range manifest.CaseHashes {
			ids = append(ids, id)
		}
	}
	hashes := map[string]bool{}
	ledger, err := s.loadExposure()
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, entry := range ledger.Entries {
		known[entry.ContentHash] = true
	}
	changed := false
	var collect func(ImportedCase)
	collect = func(c ImportedCase) {
		if len(c.Source) == 0 && len(c.Actual) > 0 {
			hashes[semanticeval.Hash(c.Actual)] = true
		}
		if len(c.Source) > 0 {
			hash := semanticeval.Hash(c.Source)
			hashes[hash] = true
			if !known[hash] {
				split := "unknown"
				if c.Split == "development" {
					split = "development"
				} else if c.Split != "" {
					split = "exposed-regression"
				}
				group := c.ExposureGroup
				if group == "" {
					group = c.ID
				}
				language := c.Language
				if !contains([]string{"en", "pt-BR"}, language) {
					language = "unknown"
				}
				ledger.Entries = append(ledger.Entries, CohortEntry{ContentHash: hash, HistoricalName: c.Split + ":" + c.ID, Split: split, ExposureGroup: group, Language: language, Authorship: "Historical retained fixture; new authorship is not established."})
				known[hash] = true
				changed = true
			}
		}
		for _, child := range c.Children {
			collect(child)
		}
	}
	for _, id := range ids {
		c, err := s.caseEvidence(runID, id)
		if err != nil {
			return err
		}
		collect(c)
	}
	if caseID == "" && manifest.Kind == "import" {
		imported, loadErr := s.loadImport(manifest)
		if loadErr != nil {
			return loadErr
		}
		hashes[semanticeval.Hash(imported.Original)] = true
		for _, dependency := range imported.Dependencies {
			hashes[dependency.Hash] = true
		}
	}
	if changed {
		if err = s.saveExposure(ledger); err != nil {
			return err
		}
	}
	for hash := range hashes {
		if err = s.recordExposure(hash, ReviewActor{ID: "local-session", Type: "unknown"}, use); err != nil {
			return err
		}
	}
	return nil
}

// Package evaluationlab owns local, synthetic evaluation contracts. Imported
// evidence never authorizes execution or promotes a production trust state.
package evaluationlab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/semanticeval"
)

const ImportVersion = "lab-import-v1"
const MaxImportBytes = 8 << 20
const MaxImportDepth = 32

type Derivation struct {
	Version    string `json:"version"`
	Mode       string `json:"mode"`
	SourceHash string `json:"sourceHash"`
}

type ImportedCase struct {
	LeafIdentity       string          `json:"leafIdentity,omitempty"`
	PlannedSteps       int             `json:"plannedSteps,omitempty"`
	AttemptedSteps     int             `json:"attemptedSteps,omitempty"`
	SkippedSteps       int             `json:"skippedSteps,omitempty"`
	ArtifactHash       string          `json:"artifactHash,omitempty"`
	ExpectedFailures   []string        `json:"expectedFailures,omitempty"`
	AssertionVersion   string          `json:"assertionVersion,omitempty"`
	Split              string          `json:"split,omitempty"`
	ExposureGroup      string          `json:"exposureGroup,omitempty"`
	FailureFamilies    []string        `json:"failureFamilies,omitempty"`
	Source             json.RawMessage `json:"source,omitempty"`
	Expected           json.RawMessage `json:"expected,omitempty"`
	Identity           string          `json:"identity"`
	ParentIdentity     string          `json:"parentIdentity,omitempty"`
	Children           []ImportedCase  `json:"children,omitempty"`
	EngineState        string          `json:"engineState"`
	AssertionVerdict   string          `json:"assertionVerdict"`
	OperationalOutcome string          `json:"operationalOutcome"`
	UnavailableReasons []string        `json:"unavailableReasons"`
	Review             json.RawMessage `json:"review,omitempty"`
	Configuration      string          `json:"configuration,omitempty"`
	Kind               string          `json:"kind"`
	ID                 string          `json:"id"`
	Workflow           string          `json:"workflow"`
	OriginalLanguage   string          `json:"originalLanguage"`
	Language           string          `json:"language"`
	Actual             json.RawMessage `json:"actual"`
}

type ImportResult struct {
	Provenance         Provenance     `json:"provenance"`
	Dependencies       []Dependency   `json:"dependencies,omitempty"`
	ParentHash         string         `json:"parentHash,omitempty"`
	ReviewDerivation   string         `json:"reviewDerivation,omitempty"`
	Format             string         `json:"format"`
	Version            string         `json:"version"`
	Original           []byte         `json:"originalBytes"`
	OriginalHash       string         `json:"originalHash"`
	OriginalVersion    string         `json:"originalVersion"`
	OriginMode         string         `json:"originMode"`
	OriginTimestamp    string         `json:"originTimestamp"`
	Derivation         Derivation     `json:"derivation"`
	Quarantined        bool           `json:"quarantined"`
	UnavailableReasons []string       `json:"unavailableReasons"`
	Cases              []ImportedCase `json:"cases"`
}

// Import accepts bytes, never paths or executable instructions. Approval is an
// exact retained-artifact allowlist: schema recognition cannot prove sensitivity.
// New artifacts need explicit synthetic registration before becoming trusted.
func Import(raw []byte) (ImportResult, error) {
	if len(raw) == 0 || len(raw) > MaxImportBytes {
		return ImportResult{}, fmt.Errorf("import size exceeds bounds")
	}
	if err := validateJSON(raw); err != nil {
		return ImportResult{}, err
	}
	hash := semanticeval.Hash(raw)
	r := ImportResult{Version: ImportVersion, Original: bytes.Clone(raw), OriginalHash: hash, Derivation: Derivation{Version: ImportVersion, Mode: "offline-import", SourceHash: hash}, UnavailableReasons: []string{}, Cases: []ImportedCase{}}
	registration, ok := retainedArtifacts[hash]
	if !ok {
		r.Quarantined = true
		r.UnavailableReasons = append(r.UnavailableReasons, "unregistered artifact: format and synthetic sensitivity are not verified")
		return r, nil
	}
	var document struct {
		ParentHash              string            `json:"derivedFromSHA256"`
		Version                 string            `json:"version"`
		Mode                    string            `json:"mode"`
		Started                 string            `json:"started"`
		Runs                    []json.RawMessage `json:"runs"`
		Attempts                []json.RawMessage `json:"attempts"`
		CandidateRetrieval      []json.RawMessage `json:"candidateRetrieval"`
		ControlledFinalMatching []json.RawMessage `json:"controlledFinalMatching"`
		CandidateRecall         []json.RawMessage `json:"candidateRecall"`
		FieldResults            []json.RawMessage `json:"fieldResults"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return ImportResult{}, err
	}
	r.OriginalVersion = document.Version
	r.Format = registration.Version
	r.OriginMode = document.Mode
	r.OriginTimestamp = document.Started
	r.ParentHash = document.ParentHash
	if document.ParentHash != "" {
		r.ReviewDerivation = "offline-source-review"
	}
	if document.Version != registration.Version {
		return ImportResult{}, fmt.Errorf("registered artifact version mismatch")
	}
	if err := json.Unmarshal(raw, &r.Provenance); err != nil {
		return ImportResult{}, err
	}
	r.Provenance.UnavailableReasons = []string{"historical dirty-tree identity, embedded dependency completeness, runtime constraints and inference-era label exposure may be unavailable"}
	if len(document.Attempts) > 0 {
		r.Format = "diagnostics-legacy-v1"
		for _, item := range document.Attempts {
			var attempt struct {
				ID       string `json:"attemptId"`
				Workflow string `json:"workflow"`
			}
			if err := json.Unmarshal(item, &attempt); err != nil {
				return ImportResult{}, err
			}
			r.Cases = append(r.Cases, ImportedCase{ID: attempt.ID, Workflow: attempt.Workflow, Kind: "diagnostic-attempt", Actual: bytes.Clone(item)})
		}
	}
	appendRows := func(kind, workflow string, rows []json.RawMessage) error {
		for _, item := range rows {
			var row struct {
				ID       string `json:"id"`
				Language string `json:"language"`
			}
			if err := json.Unmarshal(item, &row); err != nil {
				return err
			}
			language := row.Language
			if language == "pt" {
				language = "pt-BR"
			}
			r.Cases = append(r.Cases, ImportedCase{ID: row.ID, Kind: kind, Workflow: workflow, OriginalLanguage: row.Language, Language: language, Actual: bytes.Clone(item)})
		}
		return nil
	}
	if len(document.CandidateRetrieval) > 0 {
		r.Format = "qualification-evidence-legacy-v1"
		if err := appendRows("candidate-retrieval", "qualification_gaps", document.CandidateRetrieval); err != nil {
			return ImportResult{}, err
		}
		if err := appendRows("controlled-final-matching", "qualification_gaps", document.ControlledFinalMatching); err != nil {
			return ImportResult{}, err
		}
	}
	if len(document.CandidateRecall) > 0 {
		r.Format = "application-context-legacy-v1"
		if err := appendRows("candidate-recall", "application_draft", document.CandidateRecall); err != nil {
			return ImportResult{}, err
		}
		if err := appendRows("field-result", "application_draft", document.FieldResults); err != nil {
			return ImportResult{}, err
		}
	}
	for _, item := range document.Runs {
		var run struct {
			Review        json.RawMessage `json:"review"`
			Configuration string          `json:"configuration"`
			ID            string          `json:"id"`
			Task          string          `json:"task"`
			Language      string          `json:"language"`
			Run           json.RawMessage `json:"run"`
		}
		if err := json.Unmarshal(item, &run); err != nil {
			return ImportResult{}, err
		}
		actual := item
		review := bytes.Clone(run.Review)
		configuration := run.Configuration
		if len(run.Run) > 0 {
			actual = run.Run
			if err := json.Unmarshal(actual, &run); err != nil {
				return ImportResult{}, err
			}
		}
		language := run.Language
		if language == "pt" {
			language = "pt-BR"
		}
		r.Cases = append(r.Cases, ImportedCase{ID: run.ID, Kind: "workflow-result", Workflow: run.Task, OriginalLanguage: run.Language, Language: language, Actual: bytes.Clone(actual), Review: review, Configuration: configuration})
	}
	r.UnavailableReasons = append(r.UnavailableReasons, "historical source, fixture, label and scorer completeness requires format-specific interpretation")
	for i := range r.Cases {
		normalizeCaseState(&r.Cases[i], fmt.Sprintf("%s:%d", r.OriginalHash, i), "")
	}
	return r, nil
}

func revalidateImport(imported ImportResult) (ImportResult, error) {
	dependencies := make([][]byte, 0, len(imported.Dependencies))
	for _, dependency := range imported.Dependencies {
		dependencies = append(dependencies, dependency.Original)
	}
	return ImportWithDependencies(imported.Original, dependencies)
}

func normalizeCaseState(c *ImportedCase, identity, parent string) {
	c.Identity = identity
	c.ParentIdentity = parent
	c.EngineState = "unavailable"
	c.AssertionVerdict = "unavailable"
	c.OperationalOutcome = "unknown"
	c.UnavailableReasons = []string{"historical engine terminal state unavailable", "assertion interpretation requires the inference-era fixture and scorer contract"}
	var observed struct {
		Diagnostics *aidiagnostics.Attempt `json:"diagnostics"`
		Outcome     string                 `json:"outcome"`
		Steps       []json.RawMessage      `json:"steps"`
	}
	if json.Unmarshal(c.Actual, &observed) != nil {
		return
	}
	if observed.Diagnostics != nil {
		c.OperationalOutcome = aidiagnostics.OperationalOutcome(observed.Diagnostics.Outcome)
	}
	if c.Kind == "diagnostic-attempt" {
		c.OperationalOutcome = aidiagnostics.OperationalOutcome(observed.Outcome)
	}
	for i, raw := range observed.Steps {
		var step struct {
			ID       string `json:"id"`
			Task     string `json:"task"`
			Language string `json:"language"`
		}
		if json.Unmarshal(raw, &step) != nil {
			continue
		}
		language := step.Language
		if language == "pt" {
			language = "pt-BR"
		}
		child := ImportedCase{ID: step.ID, Kind: "workflow-result", Workflow: step.Task, OriginalLanguage: step.Language, Language: language, Actual: bytes.Clone(raw), Configuration: c.Configuration}
		normalizeCaseState(&child, fmt.Sprintf("%s:%d", identity, i), identity)
		c.Children = append(c.Children, child)
	}
}

func validateJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid import JSON")
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
			if depth > MaxImportDepth {
				return fmt.Errorf("import nesting exceeds bounds")
			}
		}
	}
	if !json.Valid(raw) {
		return fmt.Errorf("invalid import JSON")
	}
	return nil
}

package evaluationlab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"professional-information-repo/internal/semanticeval"
	"strconv"
	"unicode/utf8"
)

const TaskSidecarVersion = "lab-task-sidecar-v1"
const TaskCanonicalVersion = "lab-canonical-json-v1"
const IngestionTaskVersion = "profile-ingestion-task-v1"

// TaskCaseRef identifies an observation independently of its variant or execution ID.
type TaskCaseRef struct {
	SuiteID             string  `json:"suiteId"`
	CaseID              string  `json:"caseId"`
	TaskVersion         string  `json:"taskVersion"`
	InputHash           string  `json:"inputHash"`
	ExpectationHash     string  `json:"expectationHash"`
	Language            string  `json:"language"`
	RepeatOrdinal       int     `json:"repeatOrdinal"`
	Split               string  `json:"split"`
	ExposureGroup       string  `json:"exposureGroup"`
	DependencyGroup     *string `json:"dependencyGroup"`
	OriginalFixtureHash string  `json:"originalFixtureHash"`
}

// TaskSidecar separates shared task/labels from a variant's controlled response tape.
// Derivation is not execution admission: modified or unknown bytes are unregistered.
type TaskSidecar struct {
	Version          string          `json:"version"`
	CanonicalVersion string          `json:"canonicalVersion"`
	Ref              TaskCaseRef     `json:"caseRef"`
	Input            json.RawMessage `json:"input"`
	Expectation      json.RawMessage `json:"expectation"`
	SimulationScript json.RawMessage `json:"simulationScript"`
	ScriptHash       string          `json:"scriptHash"`
	OriginalFixture  Dependency      `json:"originalFixture"`
	SourceAdmission  string          `json:"sourceAdmission"`
}

// ExtractTaskSidecar derives inert identities from bounded synthetic fixture JSON.
// Only exact registered bytes carry approved-synthetic provenance. The initial
// projector supports semantic-quality Profile ingestion; other tasks fail explicitly.
func ExtractTaskSidecar(suiteID string, raw []byte, caseID string) (TaskSidecar, error) {
	if suiteID != "semantic-quality" {
		return TaskSidecar{}, fmt.Errorf("unsupported task sidecar suite")
	}
	if len(raw) > MaxImportBytes {
		return TaskSidecar{}, fmt.Errorf("task fixture exceeds bounds")
	}
	if err := validateJSON(raw); err != nil {
		return TaskSidecar{}, err
	}
	if err := validateTaskJSON(raw); err != nil {
		return TaskSidecar{}, err
	}
	var root map[string]json.RawMessage
	_ = json.Unmarshal(raw, &root)
	for field := range root {
		if !contains([]string{"version", "cases", "labelRevision"}, field) {
			return TaskSidecar{}, fmt.Errorf("unclassified task fixture field %q", field)
		}
	}
	var corpus struct {
		Version       string            `json:"version"`
		LabelRevision json.RawMessage   `json:"labelRevision"`
		Cases         []json.RawMessage `json:"cases"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		return TaskSidecar{}, err
	}
	if corpus.Version != "semantic-quality-v2" || len(corpus.Cases) == 0 || len(corpus.Cases) > 512 {
		return TaskSidecar{}, fmt.Errorf("unsupported task fixture schema or case bounds")
	}
	seen := map[string]bool{}
	var selected map[string]json.RawMessage
	for _, item := range corpus.Cases {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil {
			return TaskSidecar{}, err
		}
		var id string
		if json.Unmarshal(fields["id"], &id) != nil || id == "" || seen[id] {
			return TaskSidecar{}, fmt.Errorf("missing or duplicate fixture case ID")
		}
		seen[id] = true
		if id == caseID {
			selected = fields
		}
	}
	if selected == nil {
		return TaskSidecar{}, fmt.Errorf("unknown task fixture case")
	}
	for field := range selected {
		if !contains([]string{"id", "task", "language", "split", "sourceId", "tags", "notes", "request", "document", "revision", "userAnswers", "gold", "forbidden", "entities", "role", "expectedFailures", "labelRevision", "boundaryInvariants", "provider", "providerEnvelopes", "providerStatuses", "output"}, field) {
			return TaskSidecar{}, fmt.Errorf("unclassified task case field %q", field)
		}
	}
	var metadata struct {
		Task     string  `json:"task"`
		Language string  `json:"language"`
		Split    string  `json:"split"`
		SourceID *string `json:"sourceId"`
	}
	encoded, _ := json.Marshal(selected)
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		return TaskSidecar{}, err
	}
	if metadata.Task != "profile_ingestion" {
		return TaskSidecar{}, fmt.Errorf("unsupported task sidecar workflow")
	}
	if !contains([]string{"en", "pt-BR"}, metadata.Language) || !contains([]string{"development", "heldout"}, metadata.Split) {
		return TaskSidecar{}, fmt.Errorf("invalid task language or split")
	}
	var request map[string]json.RawMessage
	if json.Unmarshal(selected["request"], &request) != nil || request == nil {
		return TaskSidecar{}, fmt.Errorf("task request required")
	}
	var original string
	var profile map[string]json.RawMessage
	if json.Unmarshal(request["input"], &original) != nil || bytes.Equal(request["input"], []byte("null")) || json.Unmarshal(request["profile"], &profile) != nil || profile == nil {
		return TaskSidecar{}, fmt.Errorf("original text and initial Profile required")
	}
	var gold []map[string]json.RawMessage
	if json.Unmarshal(selected["gold"], &gold) != nil || len(gold) == 0 {
		return TaskSidecar{}, fmt.Errorf("independent gold array required")
	}
	for _, field := range []string{"provider", "providerEnvelopes", "providerStatuses", "forbidden", "entities", "expectedFailures"} {
		if value, ok := selected[field]; ok {
			var items []json.RawMessage
			if json.Unmarshal(value, &items) != nil || items == nil {
				return TaskSidecar{}, fmt.Errorf("task field %q must be an array", field)
			}
		}
	}
	input := map[string]json.RawMessage{"request": selected["request"]}
	for _, name := range []string{"document", "revision", "userAnswers"} {
		if value, ok := selected[name]; ok {
			input[name] = value
		}
	}
	expectation := map[string]json.RawMessage{}
	for _, name := range []string{"gold", "forbidden", "entities", "role", "expectedFailures", "labelRevision", "boundaryInvariants"} {
		if value, ok := selected[name]; ok {
			expectation[name] = value
		}
	}
	if corpus.LabelRevision != nil {
		expectation["fixtureLabelRevision"] = corpus.LabelRevision
	}
	script := map[string]json.RawMessage{}
	for _, name := range []string{"provider", "providerEnvelopes", "providerStatuses", "output"} {
		if value, ok := selected[name]; ok {
			script[name] = value
		}
	}
	input["task"], _ = json.Marshal(metadata.Task)
	input["language"], _ = json.Marshal(metadata.Language)
	result := TaskSidecar{Version: TaskSidecarVersion, CanonicalVersion: TaskCanonicalVersion, SourceAdmission: "unregistered"}
	result.Input = canonicalTaskRecord(IngestionTaskVersion, input)
	result.Expectation = canonicalTaskRecord("lab-task-expectation-v1", expectation)
	result.SimulationScript = canonicalTaskRecord("lab-simulation-script-v1", script)
	fixtureHash := semanticeval.Hash(raw)
	result.OriginalFixture = Dependency{Hash: fixtureHash, Version: corpus.Version, Original: bytes.Clone(raw)}
	result.ScriptHash = semanticeval.Hash(result.SimulationScript)
	group := "unknown-historical-pairing"
	if metadata.SourceID != nil && *metadata.SourceID != "" {
		group = *metadata.SourceID
	}
	result.Ref = TaskCaseRef{SuiteID: suiteID, CaseID: caseID, TaskVersion: IngestionTaskVersion, InputHash: semanticeval.Hash(result.Input), ExpectationHash: semanticeval.Hash(result.Expectation), Language: metadata.Language, RepeatOrdinal: 1, Split: metadata.Split, ExposureGroup: group, DependencyGroup: metadata.SourceID, OriginalFixtureHash: fixtureHash}
	for _, suite := range suiteCatalog {
		if suite.ID == suiteID && suite.FixtureHash == fixtureHash && approvedFixtureHashes[fixtureHash] {
			result.SourceAdmission = "approved-synthetic"
		}
	}
	return result, nil
}

func canonicalTaskRecord(version string, fields map[string]json.RawMessage) json.RawMessage {
	raw, _ := json.Marshal(struct {
		Version string                     `json:"version"`
		Data    map[string]json.RawMessage `json:"data"`
	}{version, fields})
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	_ = decoder.Decode(&value)
	canonical, _ := json.Marshal(value)
	return canonical
}

// Historical import validation stays unchanged. Sidecars reject duplicate keys
// before canonicalization so two different source values cannot collapse silently.
func validateTaskJSON(raw []byte) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("invalid task UTF-8")
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) || raw[i] != 'u' {
			continue
		}
		value, _ := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		i += 4
		if value >= 0xDC00 && value <= 0xDFFF {
			return fmt.Errorf("unpaired task Unicode surrogate")
		}
		if value >= 0xD800 && value <= 0xDBFF {
			if i+6 >= len(raw) || string(raw[i+1:i+3]) != `\u` {
				return fmt.Errorf("unpaired task Unicode surrogate")
			}
			low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || low < 0xDC00 || low > 0xDFFF {
				return fmt.Errorf("unpaired task Unicode surrogate")
			}
			i += 6
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return fmt.Errorf("duplicate task JSON key")
				}
				seen[name] = true
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
		case json.Delim('['):
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
		}
		return err
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("task JSON must contain one value")
	}
	return nil
}

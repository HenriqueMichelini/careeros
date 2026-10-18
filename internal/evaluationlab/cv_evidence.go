package evaluationlab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"professional-information-repo/internal/semanticeval"
)

const CVEvidenceVersion = "cv-lab-evidence-v1"

type cvStatement struct {
	ID        string   `json:"id"`
	Text      string   `json:"text"`
	SourceIDs []string `json:"sourceIds"`
	Status    string   `json:"status"`
}
type cvClientObservation struct {
	ID           string          `json:"id"`
	Request      json.RawMessage `json:"request"`
	Units        []cvStatement   `json:"units"`
	AcceptedAsIs *bool           `json:"acceptedAsIs"`
	Snapshot     json.RawMessage `json:"snapshot"`
	Reloaded     json.RawMessage `json:"reloaded"`
	Freshness    string          `json:"freshness"`
	Durable      string          `json:"durable"`
}
type cvEvidenceDocument struct {
	Version          string            `json:"version"`
	Mode             string            `json:"mode"`
	FixtureHash      string            `json:"fixtureHash"`
	FixtureBytes     []byte            `json:"fixtureBytes"`
	OwnerReportHash  string            `json:"ownerReportHash"`
	OwnerReportBytes []byte            `json:"ownerReportBytes"`
	Observations     []json.RawMessage `json:"observations"`
	BrowserBytes     []byte            `json:"browserBytes"`
	BrowserHash      string            `json:"browserHash"`
	Checks           []json.RawMessage `json:"checks"`
}

// DeriveCVEvidence maps owner receipts only. It grants no import registration,
// semantic label approval, product trust, or permission to execute. Missing client
// observations remain missing; identical case names cannot substitute for joins.
func DeriveCVEvidence(raw []byte) ([]ImportedCase, error) {
	if len(raw) == 0 || len(raw) > MaxImportBytes {
		return nil, fmt.Errorf("CV evidence exceeds bounds")
	}
	if err := validateJSON(raw); err != nil {
		return nil, err
	}
	var receipt cvEvidenceDocument
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return nil, err
	}
	if receipt.Version != CVEvidenceVersion || receipt.Mode != "controlled" || semanticeval.Hash(receipt.FixtureBytes) != receipt.FixtureHash || semanticeval.Hash(receipt.OwnerReportBytes) != receipt.OwnerReportHash {
		return nil, fmt.Errorf("invalid CV evidence identity")
	}
	for _, data := range [][]byte{receipt.FixtureBytes, receipt.OwnerReportBytes} {
		if err := validateJSON(data); err != nil {
			return nil, err
		}
	}
	var fixture struct {
		Cases        []json.RawMessage          `json:"cases"`
		Expectations map[string]json.RawMessage `json:"expectations"`
	}
	var report struct {
		Mode        string            `json:"mode"`
		FixtureHash string            `json:"fixtureHash"`
		Runs        []json.RawMessage `json:"runs"`
	}
	if json.Unmarshal(receipt.FixtureBytes, &fixture) != nil || json.Unmarshal(receipt.OwnerReportBytes, &report) != nil || report.Mode != "controlled" || report.FixtureHash != receipt.FixtureHash || len(fixture.Cases) == 0 {
		return nil, fmt.Errorf("CV owner report/fixture mismatch")
	}
	observations := map[string]json.RawMessage{}
	for _, observation := range receipt.Observations {
		var row cvClientObservation
		if json.Unmarshal(observation, &row) != nil || row.ID == "" || observations[row.ID] != nil {
			return nil, fmt.Errorf("duplicate or missing CV client identity")
		}
		observations[row.ID] = observation
	}
	runs := map[string]json.RawMessage{}
	for _, rawRun := range report.Runs {
		var run semanticeval.RunResult
		if json.Unmarshal(rawRun, &run) != nil || run.ID == "" || run.Task != "cv_generation" || run.Mode != "controlled" || runs[run.ID] != nil {
			return nil, fmt.Errorf("duplicate or invalid CV handler identity")
		}
		runs[run.ID] = rawRun
	}
	if len(runs) != len(fixture.Cases) {
		return nil, fmt.Errorf("incomplete CV owner report")
	}
	result := []ImportedCase{}
	seen := map[string]bool{}
	for _, f := range fixture.Cases {
		var source struct {
			ID       string          `json:"id"`
			Language string          `json:"language"`
			Request  json.RawMessage `json:"request"`
		}
		if json.Unmarshal(f, &source) != nil || seen[source.ID] || runs[source.ID] == nil || fixture.Expectations[source.ID] == nil {
			return nil, fmt.Errorf("missing or duplicate CV fixture identity")
		}
		seen[source.ID] = true
		c := ImportedCase{ID: source.ID, Kind: "workflow-result", Workflow: "cv_generation", Language: source.Language, OriginalLanguage: source.Language, Actual: bytes.Clone(runs[source.ID]), Configuration: "owner-retained-v1"}
		normalizeCaseState(&c, receipt.OwnerReportHash+":"+source.ID, "")
		bindFixtureEvidence(&c, f)
		c.Expected, _ = json.Marshal(map[string]any{"fixture": json.RawMessage(f), "contract": fixture.Expectations[source.ID], "adjudication": "pending; scripted expectations are enforcement labels"})
		interpretCVCase(&c)
		if observation, ok := observations[source.ID]; ok {
			if err := joinCVObservation(source.Request, c.Actual, observation, source.Language); err != nil {
				return nil, fmt.Errorf("%s: %w", source.ID, err)
			}
			c.Review = bytes.Clone(observation)
			var stages cvClientObservation
			_ = json.Unmarshal(observation, &stages)
			if len(stages.Snapshot) == 0 {
				c.UnavailableReasons = append(c.UnavailableReasons, "client snapshot stage unavailable")
			}
			if len(stages.Reloaded) == 0 {
				c.UnavailableReasons = append(c.UnavailableReasons, "client serialization/reload stage unavailable")
			}
			delete(observations, source.ID)
		} else {
			c.Review = json.RawMessage(`{"state":"unavailable","reason":"owner client observation missing"}`)
		}
		c.UnavailableReasons = append(c.UnavailableReasons, "scripted judgments; independent semantic adjudication pending", "serialization/reparse is not durable storage; browser evidence is a separate cohort")
		result = append(result, c)
	}
	if len(observations) > 0 {
		return nil, fmt.Errorf("unmatched CV client observation")
	}
	if len(receipt.BrowserBytes) > 0 {
		if semanticeval.Hash(receipt.BrowserBytes) != receipt.BrowserHash {
			return nil, fmt.Errorf("browser receipt hash mismatch")
		}
		if err := validateJSON(receipt.BrowserBytes); err != nil {
			return nil, err
		}
		var browser struct {
			Version      string            `json:"version"`
			Observations []json.RawMessage `json:"observations"`
		}
		if json.Unmarshal(receipt.BrowserBytes, &browser) != nil || browser.Version != "cv-lab-browser-v1" {
			return nil, fmt.Errorf("invalid browser receipt")
		}
		for _, raw := range browser.Observations {
			var row struct {
				ID       string          `json:"id"`
				Language string          `json:"language"`
				Request  json.RawMessage `json:"request"`
				Snapshot struct {
					Sources json.RawMessage `json:"sources"`
				} `json:"snapshot"`
			}
			if json.Unmarshal(raw, &row) != nil || row.ID == "" || seen[row.ID] {
				return nil, fmt.Errorf("duplicate browser case")
			}
			seen[row.ID] = true
			source := row.Request
			if len(source) == 0 {
				source = row.Snapshot.Sources
			}
			result = append(result, ImportedCase{ID: row.ID, Identity: receipt.BrowserHash + ":" + row.ID, Kind: "cv-browser-observation", Workflow: "cv_generation", Language: row.Language, OriginalLanguage: row.Language, Split: "development", ExposureGroup: "cv-owner-browser", Configuration: "owner-retained-v1", Source: bytes.Clone(source), Expected: json.RawMessage(`{"scope":"owner controlled browser assertions; independent semantic labels unavailable"}`), Actual: bytes.Clone(raw), Review: bytes.Clone(raw), EngineState: "completed", AssertionVerdict: "unavailable", OperationalOutcome: "unknown", UnavailableReasons: []string{"separate mocked browser source/output cohort; not a handler-corpus join or semantic proof"}})
		}
	}
	for _, raw := range receipt.Checks {
		var check struct {
			ID       string            `json:"id"`
			Language string            `json:"language"`
			Original []byte            `json:"originalBytes"`
			Hash     string            `json:"hash"`
			Sources  map[string]string `json:"sourceHashes"`
		}
		if json.Unmarshal(raw, &check) != nil || !contains([]string{"owner-client-contracts", "owner-backend-contracts"}, check.ID) || seen[check.ID] || semanticeval.Hash(check.Original) != check.Hash {
			return nil, fmt.Errorf("invalid owner contract receipt")
		}
		seen[check.ID] = true
		source, _ := json.Marshal(check.Sources)
		result = append(result, ImportedCase{ID: check.ID, Identity: check.Hash, Kind: "cv-owner-contract-tests", Workflow: "cv_generation", Language: check.Language, OriginalLanguage: check.Language, Split: "development", ExposureGroup: "cv-owner-contract-tests", Configuration: "owner-retained-v1", Source: source, Expected: json.RawMessage(`{"scope":"original owner test assertions; no new workflow or semantic acceptance"}`), Actual: bytes.Clone(raw), EngineState: "completed", AssertionVerdict: "unavailable", OperationalOutcome: "unknown", UnavailableReasons: []string{"original test output retained; command success is not semantic or user acceptance"}})
	}
	return result, nil
}

func interpretCVCase(c *ImportedCase) {
	interpretRetainedAssertion(c)
	c.AssertionVersion = "cv-lab-mapping-v1"
	if !cvOwnerContractMatches(*c) {
		c.AssertionVerdict = "fail"
	}
}
func cvOwnerContractMatches(c ImportedCase) bool {
	var expected struct {
		Contract struct {
			Status, Calls int
			Assessment    string
			Judgments     map[string]string
		} `json:"contract"`
	}
	var run struct {
		Status int
		Calls  []json.RawMessage
		Output struct {
			Support struct {
				Assessment string
				Statements []cvStatement
			}
		}
	}
	if json.Unmarshal(c.Expected, &expected) != nil || json.Unmarshal(c.Actual, &run) != nil || expected.Contract.Status != run.Status || expected.Contract.Calls != len(run.Calls) || expected.Contract.Assessment != run.Output.Support.Assessment || len(expected.Contract.Judgments) != len(run.Output.Support.Statements) {
		return false
	}
	seen := map[string]bool{}
	for _, statement := range run.Output.Support.Statements {
		if seen[statement.ID] || expected.Contract.Judgments[statement.ID] != statement.Status {
			return false
		}
		seen[statement.ID] = true
	}
	return true
}

func joinCVObservation(request, actual, observation json.RawMessage, language string) error {
	type factIdentity struct{ ID, Section, EntryID, Field, Text string }
	var original struct {
		Facts      []factIdentity
		CVLanguage string `json:"cvLanguage"`
		Density    string `json:"density"`
	}
	var client struct {
		Request struct {
			Facts []struct {
				factIdentity
				Owner struct {
					ID, ProfileID string
					Revision      int
				}
				Reference struct {
					ID, ProfileID string
					Revision      int
				}
			}
			CVLanguage       string `json:"cvLanguage"`
			Density          string `json:"density"`
			StatementSupport bool   `json:"statementSupport"`
		} `json:"request"`
		Units    []cvStatement `json:"units"`
		Document struct {
			ID       string
			Entities []struct{ ID, LegacyID string }
			Facts    []struct {
				ID, Field, Value string
				Revision         int
				Owner            struct{ ID string }
			}
		}
	}
	var run struct {
		Language string `json:"language"`
		Output   struct {
			Support struct {
				Statements []cvStatement `json:"statements"`
			} `json:"support"`
		} `json:"output"`
	}
	if json.Unmarshal(request, &original) != nil || json.Unmarshal(observation, &client) != nil || json.Unmarshal(actual, &run) != nil || original.CVLanguage != language || client.Request.Density != original.Density || client.Request.CVLanguage != language || run.Language != language || !client.Request.StatementSupport || len(original.Facts) != len(client.Request.Facts) {
		return fmt.Errorf("CV request facts/language mismatch")
	}
	// stableCvFacts assigns positional legacy entry aliases. Resolve ownership
	// through the retained canonical document instead of treating aliases as IDs.
	seenFacts := map[string]bool{}
	for _, fact := range original.Facts {
		if fact.ID == "" || seenFacts[fact.ID] {
			return fmt.Errorf("duplicate source fact")
		}
		seenFacts[fact.ID] = true
		owner := client.Document.ID
		if fact.EntryID != "" {
			owner = ""
			for _, entity := range client.Document.Entities {
				if entity.LegacyID == fact.EntryID {
					owner = entity.ID
				}
			}
		}
		matched := false
		for _, projected := range client.Request.Facts {
			if projected.ID != fact.ID {
				continue
			}
			if owner == "" || projected.Owner.ProfileID != client.Document.ID || projected.Reference.ProfileID != client.Document.ID || projected.Owner.ID != owner || projected.Reference.ID != fact.ID || projected.Section != fact.Section || projected.Field != fact.Field || projected.Text != fact.Text {
				return fmt.Errorf("CV source owner/text mismatch")
			}
			for _, canonical := range client.Document.Facts {
				if canonical.ID == fact.ID && canonical.Owner.ID == owner && canonical.Field == fact.Field && canonical.Value == fact.Text && canonical.Revision == projected.Reference.Revision {
					matched = true
				}
			}
		}
		if !matched {
			return fmt.Errorf("CV canonical source reference mismatch")
		}
	}
	if len(run.Output.Support.Statements) != len(client.Units) {
		return fmt.Errorf("CV statement population mismatch")
	}
	units := map[string]cvStatement{}
	for _, unit := range client.Units {
		if _, exists := units[unit.ID]; exists || unit.ID == "" {
			return fmt.Errorf("duplicate CV statement identity")
		}
		units[unit.ID] = unit
	}
	for _, unit := range run.Output.Support.Statements {
		other, ok := units[unit.ID]
		if !ok || !equalJSON(unit, other) {
			return fmt.Errorf("CV statement text/source/status mismatch")
		}
	}
	return joinCVSnapshot(observation, language)
}
func equalJSON(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return bytes.Equal(left, right)
}

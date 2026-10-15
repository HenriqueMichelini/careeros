package evaluationlab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"professional-information-repo/internal/semanticeval"
	"strings"
)

type Provenance struct {
	Commit             string              `json:"commit"`
	DirtyHashes        map[string]string   `json:"dirtyHashes,omitempty"`
	SourceHashes       map[string]string   `json:"sourceHashes,omitempty"`
	FixtureHash        string              `json:"fixtureHash,omitempty"`
	FixtureVersion     string              `json:"fixtureVersion,omitempty"`
	FixtureHashes      map[string]string   `json:"fixtureHashes,omitempty"`
	DerivedFrom        []ArtifactReference `json:"derivedFrom,omitempty"`
	UnavailableReasons []string            `json:"unavailableReasons"`
}

type ArtifactReference struct {
	Hash string `json:"hash"`
	Path string `json:"path,omitempty"`
}
type Dependency struct {
	Hash     string `json:"hash"`
	Version  string `json:"version"`
	Original []byte `json:"originalBytes"`
}

// ImportWithDependencies is still pure: callers supply bounded, approved bytes.
// Fixture paths in historical reports are provenance text, never file requests.
func ImportWithDependencies(raw []byte, dependencies [][]byte) (ImportResult, error) {
	result, err := Import(raw)
	if err != nil {
		return result, err
	}
	if len(dependencies) > 16 {
		return ImportResult{}, fmt.Errorf("too many dependencies")
	}
	seen := map[string]bool{}
	total := len(raw)
	for _, dependency := range dependencies {
		total += len(dependency)
		if total > MaxImportBytes {
			return ImportResult{}, fmt.Errorf("import bundle exceeds bounds")
		}
		if err := validateJSON(dependency); err != nil {
			return ImportResult{}, err
		}
		hash := semanticeval.Hash(dependency)
		if !approvedFixtureHashes[hash] || seen[hash] {
			return ImportResult{}, fmt.Errorf("unregistered or duplicate synthetic dependency")
		}
		seen[hash] = true
		matches := hash == result.Provenance.FixtureHash
		for _, expected := range result.Provenance.FixtureHashes {
			matches = matches || hash == expected
		}
		if result.Quarantined || !matches {
			return ImportResult{}, fmt.Errorf("dependency does not match inference-era report")
		}
		var corpus struct {
			Version string            `json:"version"`
			Cases   []json.RawMessage `json:"cases"`
		}
		if err := json.Unmarshal(dependency, &corpus); err != nil {
			return ImportResult{}, err
		}
		result.Dependencies = append(result.Dependencies, Dependency{Hash: hash, Version: corpus.Version, Original: bytes.Clone(dependency)})
		for i := range result.Cases {
			for _, fixture := range corpus.Cases {
				bindFixtureEvidence(&result.Cases[i], fixture)
			}
		}
	}
	return result, nil
}

func interpretRetainedAssertion(c *ImportedCase) {
	var observed struct {
		Score *semanticeval.Result `json:"score"`
	}
	if json.Unmarshal(c.Actual, &observed) != nil || observed.Score == nil {
		return
	}
	c.AssertionVersion = "lab-retained-assertions-v1"
	c.AssertionVerdict = "pass"
	for _, expected := range c.ExpectedFailures {
		found := false
		for _, actual := range observed.Score.Failures {
			found = found || strings.HasPrefix(actual, expected)
		}
		if !found {
			c.AssertionVerdict = "fail"
		}
	}
	for _, actual := range observed.Score.Failures {
		allowed := false
		for _, expected := range c.ExpectedFailures {
			allowed = allowed || strings.HasPrefix(actual, expected)
		}
		if !allowed {
			c.AssertionVerdict = "fail"
		}
	}
	// An absent score never gets here; an expected rejection may pass its
	// assertion while the operational outcome remains a rejection.
	filtered := []string{}
	for _, reason := range c.UnavailableReasons {
		if !strings.HasPrefix(reason, "assertion interpretation") {
			filtered = append(filtered, reason)
		}
	}
	c.UnavailableReasons = filtered
}

func bindFixtureEvidence(c *ImportedCase, raw json.RawMessage) {
	var fixture struct {
		ID               string            `json:"id"`
		Request          json.RawMessage   `json:"request"`
		Document         json.RawMessage   `json:"document"`
		Gold             json.RawMessage   `json:"gold"`
		Relevant         json.RawMessage   `json:"relevantFactIds"`
		ExpectedFailures []string          `json:"expectedFailures"`
		SourceID         string            `json:"sourceId"`
		Split            string            `json:"split"`
		Tags             []string          `json:"tags"`
		Steps            []json.RawMessage `json:"steps"`
	}
	if json.Unmarshal(raw, &fixture) != nil || fixture.ID != c.ID {
		return
	}
	source := fixture.Request
	if len(source) == 0 {
		source = fixture.Document
	}
	gold := fixture.Gold
	if len(gold) == 0 {
		gold = fixture.Relevant
	}
	c.Source = bytes.Clone(source)
	c.Expected = bytes.Clone(gold)
	c.ExpectedFailures = append([]string{}, fixture.ExpectedFailures...)
	c.Split = fixture.Split
	c.ExposureGroup = fixture.SourceID
	c.FailureFamilies = append([]string{}, fixture.Tags...)
	interpretRetainedAssertion(c)
	for i := range c.Children {
		for _, step := range fixture.Steps {
			bindFixtureEvidence(&c.Children[i], step)
		}
	}
}

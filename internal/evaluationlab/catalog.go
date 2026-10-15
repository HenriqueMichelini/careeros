package evaluationlab

import "fmt"

type FixtureCase struct {
	ID            string `json:"id"`
	Hash          string `json:"hash"`
	Workflow      string `json:"workflow"`
	Language      string `json:"language"`
	Split         string `json:"split"`
	ExposureGroup string `json:"exposureGroup"`
	RequiredSteps int    `json:"requiredSteps"`
	MaximumCalls  int    `json:"maximumCalls"`
}

type Suite struct {
	FixtureHashes      map[string]string `json:"fixtureHashes,omitempty"`
	ID                 string            `json:"id"`
	Version            string            `json:"version"`
	FixtureHash        string            `json:"fixtureHash"`
	Modes              []string          `json:"modes"`
	Configurations     []string          `json:"configurations"`
	RequiredTools      []string          `json:"requiredTools"`
	Sensitivity        string            `json:"sensitivity"`
	AssertionSemantics string            `json:"assertionSemantics"`
	Cases              []FixtureCase     `json:"cases"`
}

type Selection struct {
	Suite         string   `json:"suite"`
	Mode          string   `json:"mode"`
	Configuration string   `json:"configuration"`
	CaseIDs       []string `json:"caseIds"`
}

func Catalog() []Suite {
	result := make([]Suite, len(suiteCatalog))
	for i, s := range suiteCatalog {
		if s.FixtureHashes != nil {
			hashes := map[string]string{}
			for key, value := range s.FixtureHashes {
				hashes[key] = value
			}
			s.FixtureHashes = hashes
		}
		s.Modes = append([]string{}, s.Modes...)
		s.Configurations = append([]string{}, s.Configurations...)
		s.RequiredTools = append([]string{}, s.RequiredTools...)
		s.Cases = append([]FixtureCase{}, s.Cases...)
		result[i] = s
	}
	return result
}

func ValidateSelection(selection Selection) error {
	for _, suite := range suiteCatalog {
		if suite.ID != selection.Suite {
			continue
		}
		if !contains(suite.Modes, selection.Mode) || !contains(suite.Configurations, selection.Configuration) {
			return fmt.Errorf("unsupported suite mode/configuration")
		}
		seen := map[string]bool{}
		for _, id := range selection.CaseIDs {
			if seen[id] {
				return fmt.Errorf("duplicate case ID")
			}
			seen[id] = true
			found := false
			for _, c := range suite.Cases {
				if c.ID == id {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("unknown suite case")
			}
		}
		return nil
	}
	return fmt.Errorf("unknown suite")
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

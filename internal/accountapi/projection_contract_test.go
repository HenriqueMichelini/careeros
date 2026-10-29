package accountapi

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/qualificationmatching"
)

type projectionResult struct {
	Name           string                   `json:"name"`
	Evidence       profiledocument.Document `json:"evidence"`
	Useful         bool                     `json:"useful"`
	Repository     map[string]any           `json:"repository"`
	Qualifications map[string]any           `json:"qualifications"`
}

// Shared inputs have independent expected selections. The Node contract runs
// this exact test and compares every output value with the production TS code.
// No database, provider, server or production-only test endpoint is involved.
func TestProfileProjectionContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/profile-projections.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string          `json:"name"`
		Document json.RawMessage `json:"document"`
		Expected struct {
			Facts    []string            `json:"facts"`
			Evidence []string            `json:"evidence"`
			Links    []string            `json:"links"`
			Useful   bool                `json:"useful"`
			Groups   map[string][]string `json:"groups"`
		} `json:"expected"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil || len(cases) == 0 {
		t.Fatalf("invalid shared corpus: %v", err)
	}
	results := make([]projectionResult, 0, len(cases))
	seen := map[string]bool{}
	for _, c := range cases {
		if c.Name == "" || seen[c.Name] {
			t.Fatalf("empty/duplicate case name %q", c.Name)
		}
		seen[c.Name] = true
		t.Run(c.Name, func(t *testing.T) {
			doc, err := profiledocument.Decode(c.Document)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(doc)
			evidence := qualificationmatching.ApprovedProjection(doc)
			repo, qualifications := workflowProfile(doc)
			useful := qualificationmatching.HasUsefulFacts(evidence)
			facts, excerpts, links := []string{}, []string{}, []string{}
			for _, fact := range evidence.Facts {
				facts = append(facts, fact.ID)
			}
			for _, excerpt := range evidence.Evidence {
				excerpts = append(excerpts, excerpt.ID)
			}
			for _, link := range evidence.Links {
				links = append(links, link.ID)
			}
			for _, comparison := range []struct {
				label     string
				got, want any
			}{
				{"facts", facts, c.Expected.Facts}, {"evidence", excerpts, c.Expected.Evidence},
				{"links", links, c.Expected.Links}, {"useful", useful, c.Expected.Useful},
			} {
				if !reflect.DeepEqual(comparison.got, comparison.want) {
					t.Errorf("%s: got %#v; want %#v", comparison.label, comparison.got, comparison.want)
				}
			}
			for group, expected := range c.Expected.Groups {
				value, exists := repo[group]
				if !exists {
					value = qualifications[group]
				}
				items, ok := value.([]map[string]any)
				if !ok {
					t.Fatalf("missing group %s", group)
				}
				ids := []string{}
				for _, item := range items {
					ids = append(ids, item["id"].(string))
				}
				if !reflect.DeepEqual(ids, expected) {
					t.Errorf("%s: got %v; want %v", group, ids, expected)
				}
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) {
				t.Fatal("projection mutated the saved Profile")
			}
			results = append(results, projectionResult{c.Name, evidence, useful, repo, qualifications})
		})
	}
	if output := os.Getenv("CAREEROS_PROFILE_PROJECTION_OUTPUT"); output != "" && !t.Failed() {
		raw, err := json.Marshal(results)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(output, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

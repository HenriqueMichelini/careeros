package semanticeval

import (
	"encoding/json"
	"os"
	"testing"
)

func TestScoreSeparatesMissingFactsFromUnsupportedAndWrongRelationships(t *testing.T) {
	gold := []Atom{
		{ID: "java", Group: "operations", Match: map[string]string{"value": "^Java$", "target": "^skills$"}},
		{ID: "metric", Group: "operations", Match: map[string]string{"value": "20%", "entryId": "^e1$"}},
		{ID: "python", Group: "operations", Match: map[string]string{"value": "^Python$"}},
	}
	raw := json.RawMessage(`{"operations":[{"target":"skills","value":"Java"},{"target":"skills","value":"Java"},{"entryId":"e2","value":"Reduced latency 20%"},{"value":"AWS expert"}]}`)
	score, err := Score(Case{Gold: gold, Forbidden: []string{"AWS expert"}}, raw)
	if err != nil {
		t.Fatal(err)
	}
	if score.Matched != 1 || score.Expected != 3 || score.Observed != 4 || score.Duplicates != 1 || score.RelationshipErrors != 1 || score.ForbiddenAdditions != 1 || len(score.Missing) != 2 || score.Precision != .25 {
		t.Fatalf("misleading score: %+v", score)
	}
}

func TestCorpusPreservesCapacityOmissionsAndControlledContracts(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []Case `json:"cases"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if err := Validate(corpus.Cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			result, err := Run(c, "controlled", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != 200 || !result.NoStore || result.Score.ForbiddenAdditions != 0 {
				t.Fatalf("contract regression: %+v", result)
			}
			if c.ID == "en-capacity-35" || c.ID == "pt-capacity-35" {
				if len(result.Score.Missing) != 10 || result.Score.Matched != 60 || result.Score.Expected != 70 {
					t.Fatalf("hid capacity loss: %+v", result.Score)
				}
			} else if len(result.Score.Failures) != 0 {
				t.Fatalf("curated control regression: %+v", result.Score)
			}
		})
	}
}

func TestLiveRequiresExplicitCredentials(t *testing.T) {
	if _, err := Run(Case{Task: "cv_generation"}, "live", "", ""); err == nil {
		t.Fatal("live without a credential")
	}
	if _, err := Run(Case{}, "typo", "", ""); err == nil {
		t.Fatal("unknown mode")
	}
}

func TestControlledAdapterExercisesProductionCvGrounding(t *testing.T) {
	c := Case{Task: "cv_generation", Request: json.RawMessage(`{"cvLanguage":"en","facts":[{"id":"f0","section":"experience","entryId":"e1","field":"achievements","text":"Reduced latency by 20%."}]}`), Provider: []json.RawMessage{json.RawMessage(`{"summary":[{"sourceId":"f0","text":"Reduced latency by 90%."}],"selected":["f0"]}`)}}
	run, err := Run(c, "controlled", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != 502 || string(run.Output) != "{\"error\":\"invalid_output\"}\n" || len(run.Calls) != 1 || run.Calls[0].Model == "" || run.Calls[0].SchemaHash == "" {
		t.Fatalf("did not preserve production grounding: %+v", run)
	}
}

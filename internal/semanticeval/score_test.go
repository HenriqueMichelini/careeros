package semanticeval

import (
	"encoding/json"
	"os"
	"strings"
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

func TestDraftScoresInventedClaimsBeyondSupportedKeywords(t *testing.T) {
	c := Case{Task: "application_draft", Gold: []Atom{
		{ID: "java", Group: "draft", Match: map[string]string{"field": "^coverLetter$", "text": "^I use Java$"}},
	}}
	score, err := Score(c, json.RawMessage(`{"coverLetter":{"body":"I use Java. I managed ten engineers."}}`))
	if err != nil {
		t.Fatal(err)
	}
	if score.Observed != 2 || score.Matched != 1 || score.Precision != .5 || len(score.Unmatched) != 1 {
		t.Fatalf("invented management disappeared: %+v", score)
	}
}

func TestDraftDoesNotTransferOutcomeBetweenEntities(t *testing.T) {
	c := Case{Task: "application_draft", Entities: []string{"Acme", "Atlas"}, Gold: []Atom{
		{ID: "metric-owner", Group: "draft", Source: "Profile.experience[e1].achievements", Match: map[string]string{"field": "^resume$", "content": "^At \\{entity\\} I reduced latency by 20%$", "owner": "^Acme$"}},
	}}
	score, err := Score(c, json.RawMessage(`{"resume":"At Atlas I reduced latency by 20%."}`))
	if err != nil {
		t.Fatal(err)
	}
	if score.RelationshipErrors != 1 || score.Matched != 0 || len(score.Missing) != 1 {
		t.Fatalf("transferred metric: %+v", score)
	}
}

func TestCorpusPreservesCapacityOmissionsAndControlledContracts(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/cases.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []Case `json:"cases"`
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if err = Validate(corpus.Cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) { assertControlledCase(t, c) })
	}
}
func assertControlledCase(t *testing.T, c Case) {
	t.Helper()
	result, err := Run(c, "controlled", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != 200 || !result.NoStore {
		t.Fatalf("contract regression: %+v", result)
	}
	if c.Role == "negative_control" {
		assertNegativeControl(t, c, result.Score)
		return
	}
	if result.Score.ForbiddenAdditions != 0 {
		t.Fatalf("forbidden addition: %+v", result.Score)
	}
	if c.ID == "en-capacity-35" || c.ID == "pt-capacity-35" {
		assertCapacityLoss(t, result.Score)
		return
	}
	if len(result.Score.Failures) != 0 {
		t.Fatalf("curated control regression: %+v", result.Score)
	}
}
func assertNegativeControl(t *testing.T, c Case, score Result) {
	t.Helper()
	for _, expected := range c.ExpectedFailures {
		if !hasFailure(score.Failures, expected) {
			t.Fatalf("missed adversarial failure %q: %+v", expected, score)
		}
	}
	if strings.Contains(c.ID, "transferred-metric") && score.RelationshipErrors != 1 {
		t.Fatalf("did not detect transferred metric: %+v", score)
	}
	if (strings.Contains(c.ID, "invented-management") || strings.Contains(c.ID, "requirement-as-skill")) && score.UnsupportedCandidates != 1 {
		t.Fatalf("did not count unsupported claim: %+v", score)
	}
}
func hasFailure(failures []string, expected string) bool {
	for _, failure := range failures {
		if strings.Contains(failure, expected) {
			return true
		}
	}
	return false
}
func assertCapacityLoss(t *testing.T, score Result) {
	t.Helper()
	if len(score.Missing) != 10 || score.Matched != 60 || score.Expected != 70 {
		t.Fatalf("hid capacity loss: %+v", score)
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

func TestOriginalBaselineInputsAreRecoverableByHash(t *testing.T) {
	root := "../../docs/evaluations/semantic-quality/"
	raw, err := os.ReadFile(root + "live.initial.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		FixtureHash  string            `json:"fixtureHash"`
		SourceHashes map[string]string `json:"sourceHashes"`
	}
	if err = json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	for file, expected := range map[string]string{"history/cases.initial.v1.json": report.FixtureHash, "history/adapter.initial.v1.go.txt": report.SourceHashes["internal/semanticeval/adapter.go"], "history/score.v1.go.txt": report.SourceHashes["internal/semanticeval/score.go"]} {
		contents, err := os.ReadFile(root + file)
		if err != nil {
			t.Fatal(err)
		}
		if Hash(contents) != expected {
			t.Fatalf("original evidence snapshot drifted: %s", file)
		}
	}
}

func TestOfflineAssessmentDoesNotEraseARejectedWorkflow(t *testing.T) {
	c := Case{Gold: []Atom{{ID: "metric", Group: "summary", Match: map[string]string{"text": "20%"}}}}
	result, err := Assess(c, RunResult{Mode: "replay", Status: 502, NoStore: true, Output: json.RawMessage(`{"error":"invalid_output"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != 502 || result.Score.Completeness != 0 || len(result.Score.Failures) != 2 {
		t.Fatalf("replay erased failure: %+v", result)
	}
}

func TestInvalidGoldIsRejectedBeforeEvaluation(t *testing.T) {
	c := Case{ID: "invalid-label", Task: "profile_ingestion", Language: "en", Split: "development", Request: json.RawMessage(`{}`), Notes: "Synthetic validation control", Gold: []Atom{{ID: "java", Group: "claims", Match: map[string]string{"text": "["}}}}
	if err := Validate([]Case{c}); err == nil {
		t.Fatal("invalid gold could reach provider evaluation")
	}
}

func TestIdenticalRequestsCannotLeakIntoHeldoutSplit(t *testing.T) {
	c := Case{ID: "development", Task: "cv_generation", Language: "en", Split: "development", Request: json.RawMessage(`{"facts":[],"cvLanguage":"en"}`), Notes: "Split integrity control", Gold: []Atom{{ID: "source", Group: "selected", Match: map[string]string{"text": "f0"}}}}
	heldout := c
	heldout.ID = "heldout"
	heldout.Split = "heldout"
	heldout.Request = json.RawMessage(`{"cvLanguage":"en", "facts":[]}`)
	if err := Validate([]Case{c, heldout}); err == nil {
		t.Fatal("identical request appears on both sides of split")
	}
}

package evaluationlab_test

import (
	"bytes"
	"encoding/json"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func taskFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/cases.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mutateTaskFixture(t *testing.T, raw []byte, change func(map[string]any)) []byte {
	t.Helper()
	var corpus map[string]any
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	for _, value := range corpus["cases"].([]any) {
		c := value.(map[string]any)
		if c["id"] == "en-fact" {
			change(c)
		}
	}
	encoded, err := json.Marshal(corpus)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestTaskSidecarDetectsSharedInputAndIndependentLabelDrift(t *testing.T) {
	raw := taskFixture(t)
	baseline, err := lab.ExtractTaskSidecar("semantic-quality", raw, "en-fact")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		input  bool
		change func(map[string]any)
	}{
		{"original text bytes", true, func(c map[string]any) { c["request"].(map[string]any)["input"] = " I use Java\n" }},
		{"initial Profile", true, func(c map[string]any) { c["request"].(map[string]any)["profile"].(map[string]any)["skills"] = "Go" }},
		{"document", true, func(c map[string]any) { c["document"] = map[string]any{"text": "Java", "revision": 1} }},
		{"revision", true, func(c map[string]any) { c["revision"] = 2 }},
		{"language", true, func(c map[string]any) { c["language"] = "pt-BR" }},
		{"frozen user answers", true, func(c map[string]any) { c["userAnswers"] = []any{map[string]any{"questionId": "q1", "answer": "No"}} }},
		{"null document differs from absent", true, func(c map[string]any) { c["document"] = nil }},
		{"gold", false, func(c map[string]any) { c["gold"].([]any)[0].(map[string]any)["id"] = "revised-java-fact" }},
		{"forbidden", false, func(c map[string]any) { c["forbidden"] = []any{"Python"} }},
		{"entities", false, func(c map[string]any) { c["entities"] = []any{"Alice"} }},
		{"role", false, func(c map[string]any) { c["role"] = "negative_control" }},
		{"label revision", false, func(c map[string]any) { c["labelRevision"] = "curated-v2" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate, err := lab.ExtractTaskSidecar("semantic-quality", mutateTaskFixture(t, raw, test.change), "en-fact")
			if err != nil {
				t.Fatal(err)
			}
			if (candidate.Ref.InputHash != baseline.Ref.InputHash) != test.input || (candidate.Ref.ExpectationHash != baseline.Ref.ExpectationHash) == test.input {
				t.Fatal("drift changed the wrong identity or was lost")
			}
			if candidate.ScriptHash != baseline.ScriptHash {
				t.Fatal("shared contract drift changed simulation identity")
			}
		})
	}
	unknown, err := lab.ExtractTaskSidecar("semantic-quality", raw, "en-proficiency-conflict")
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Ref.DependencyGroup != nil || unknown.Ref.ExposureGroup != "unknown-historical-pairing" {
		t.Fatal("unknown translation relationship was inferred")
	}
}

func TestTaskSidecarSeparatesFrozenExpectationsFromSimulation(t *testing.T) {
	raw := taskFixture(t)
	original := bytes.Clone(raw)
	baseline, err := lab.ExtractTaskSidecar("semantic-quality", raw, "en-fact")
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(raw, []byte(`"text": "Java"`), []byte(`"text": "Invented expertise"`), 1)
	candidate, err := lab.ExtractTaskSidecar("semantic-quality", changed, "en-fact")
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Ref.InputHash != candidate.Ref.InputHash || baseline.Ref.ExpectationHash != candidate.Ref.ExpectationHash || !bytes.Equal(baseline.Expectation, candidate.Expectation) {
		t.Fatal("simulation change regenerated task input or independent expectations")
	}
	if baseline.ScriptHash == candidate.ScriptHash {
		t.Fatal("changed simulation script lost its own identity")
	}
	if baseline.SourceAdmission != "approved-synthetic" || candidate.SourceAdmission != "unregistered" {
		t.Fatal("candidate bytes gained approved provenance")
	}
	if !bytes.Equal(raw, original) || !bytes.Equal(baseline.OriginalFixture.Original, original) {
		t.Fatal("original fixture bytes changed")
	}
	if baseline.Ref.ExposureGroup != "en-fact" || baseline.Ref.RepeatOrdinal != 1 {
		t.Fatal("case metadata missing")
	}
}

func TestTaskSidecarRejectsAmbiguousFixtureShapes(t *testing.T) {
	raw := taskFixture(t)
	for _, candidate := range [][]byte{
		bytes.Replace(raw, []byte(`"version": "semantic-quality-v2"`), []byte(`"version": "semantic-quality-v2", "version": "semantic-quality-v2"`), 1),
		bytes.Replace(raw, []byte(`"input": "I use Java"`), []byte(`"input": "shadow", "input": "I use Java"`), 1),
		mutateTaskFixture(t, raw, func(c map[string]any) { c["gold"] = nil }),
		mutateTaskFixture(t, raw, func(c map[string]any) { c["gold"] = map[string]any{"id": "ambiguous"} }),
		mutateTaskFixture(t, raw, func(c map[string]any) { c["unclassifiedExpectation"] = "must not disappear" }),
		mutateTaskFixture(t, raw, func(c map[string]any) { c["provider"] = map[string]any{"reply": "ambiguous"} }),
		bytes.Replace(raw, []byte(`I use Java`), []byte(`I use \ud800`), 1),
	} {
		if _, err := lab.ExtractTaskSidecar("semantic-quality", candidate, "en-fact"); err == nil {
			t.Fatal("ambiguous or unclassified fixture accepted")
		}
	}
}

func TestTaskSidecarRejectsMalformedGoldAtoms(t *testing.T) {
	raw := taskFixture(t)
	for _, test := range []struct {
		name string
		atom any
	}{
		{"null atom", nil},
		{"empty atom", map[string]any{}},
		{"missing id", map[string]any{"group": "claims", "match": map[string]any{"text": "Java"}}},
		{"null id", map[string]any{"id": nil, "group": "claims", "match": map[string]any{"text": "Java"}}},
		{"missing group", map[string]any{"id": "java", "match": map[string]any{"text": "Java"}}},
		{"empty group", map[string]any{"id": "java", "group": "", "match": map[string]any{"text": "Java"}}},
		{"missing match", map[string]any{"id": "java", "group": "claims"}},
		{"empty match", map[string]any{"id": "java", "group": "claims", "match": map[string]any{}}},
		{"null match", map[string]any{"id": "java", "group": "claims", "match": nil}},
		{"non-string match", map[string]any{"id": "java", "group": "claims", "match": map[string]any{"text": 1}}},
		{"null match member", map[string]any{"id": "java", "group": "claims", "match": map[string]any{"text": nil}}},
		{"non-boolean optional", map[string]any{"id": "java", "group": "claims", "match": map[string]any{"text": "Java"}, "optional": "yes"}},
		{"non-string source", map[string]any{"id": "java", "group": "claims", "match": map[string]any{"text": "Java"}, "source": map[string]any{"bad": "shape"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := mutateTaskFixture(t, raw, func(c map[string]any) { c["gold"] = []any{test.atom} })
			if _, err := lab.ExtractTaskSidecar("semantic-quality", candidate, "en-fact"); err == nil {
				t.Fatal("malformed gold atom received a canonical expectation identity")
			}
		})
	}
}

func TestTaskSidecarRejectsMalformedTypedFixtureMembers(t *testing.T) {
	raw := taskFixture(t)
	for _, test := range []struct {
		field string
		value any
	}{
		{"role", map[string]any{"bad": "shape"}},
		{"notes", []any{"not a scalar"}},
		{"tags", []any{1}},
		{"forbidden", []any{map[string]any{"bad": "shape"}}},
		{"entities", []any{false}},
		{"expectedFailures", []any{[]any{"nested"}}},
		{"providerStatuses", []any{"200"}},
		{"providerStatuses", []any{200.5}},
	} {
		t.Run(test.field, func(t *testing.T) {
			candidate := mutateTaskFixture(t, raw, func(c map[string]any) { c[test.field] = test.value })
			if _, err := lab.ExtractTaskSidecar("semantic-quality", candidate, "en-fact"); err == nil {
				t.Fatal("malformed typed fixture member received a canonical identity")
			}
		})
	}
	for _, field := range []string{"tags", "forbidden", "entities", "expectedFailures", "providerStatuses"} {
		t.Run(field+" null member", func(t *testing.T) {
			candidate := mutateTaskFixture(t, raw, func(c map[string]any) { c[field] = []any{nil} })
			if _, err := lab.ExtractTaskSidecar("semantic-quality", candidate, "en-fact"); err == nil {
				t.Fatal("null array member received a canonical identity")
			}
		})
	}
}

func TestTaskSidecarPreservesRegisteredFixturesAndOptionalNulls(t *testing.T) {
	raw := taskFixture(t)
	var corpus struct {
		Cases []struct{ ID, Task string }
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	for _, c := range corpus.Cases {
		if c.Task != "profile_ingestion" {
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			sidecar, err := lab.ExtractTaskSidecar("semantic-quality", raw, c.ID)
			if err != nil {
				t.Fatal(err)
			}
			if sidecar.SourceAdmission != "approved-synthetic" || !bytes.Equal(sidecar.OriginalFixture.Original, raw) {
				t.Fatal("registered fixture provenance or original bytes changed")
			}
		})
	}
	for _, field := range []string{"role", "labelRevision", "boundaryInvariants", "document", "revision", "userAnswers"} {
		t.Run(field+" absent versus null", func(t *testing.T) {
			absent, err := lab.ExtractTaskSidecar("semantic-quality", mutateTaskFixture(t, raw, func(c map[string]any) { delete(c, field) }), "en-fact")
			if err != nil {
				t.Fatal(err)
			}
			null, err := lab.ExtractTaskSidecar("semantic-quality", mutateTaskFixture(t, raw, func(c map[string]any) { c[field] = nil }), "en-fact")
			if err != nil {
				t.Fatal(err)
			}
			inputField := field == "document" || field == "revision" || field == "userAnswers"
			if (absent.Ref.InputHash != null.Ref.InputHash) != inputField || (absent.Ref.ExpectationHash != null.Ref.ExpectationHash) == inputField || absent.ScriptHash != null.ScriptHash {
				t.Fatal("optional null was lost or changed an independent identity")
			}
		})
	}
	optionalAtomNulls := mutateTaskFixture(t, raw, func(c map[string]any) {
		atom := c["gold"].([]any)[0].(map[string]any)
		atom["optional"], atom["source"] = nil, nil
	})
	if _, err := lab.ExtractTaskSidecar("semantic-quality", optionalAtomNulls, "en-fact"); err != nil {
		t.Fatalf("optional atom nulls rejected: %v", err)
	}
}

func TestTaskSidecarCanonicalIdentityPreservesValuesAndOriginalBytes(t *testing.T) {
	raw := taskFixture(t)
	baseline, err := lab.ExtractTaskSidecar("semantic-quality", raw, "en-fact")
	if err != nil {
		t.Fatal(err)
	}
	reencoded := mutateTaskFixture(t, raw, func(c map[string]any) {})
	canonical, err := lab.ExtractTaskSidecar("semantic-quality", reencoded, "en-fact")
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Ref.InputHash != canonical.Ref.InputHash || baseline.Ref.ExpectationHash != canonical.Ref.ExpectationHash || baseline.ScriptHash != canonical.ScriptHash {
		t.Fatal("JSON layout changed canonical identity")
	}
	if baseline.OriginalFixture.Hash == canonical.OriginalFixture.Hash {
		t.Fatal("exact original fixture identity lost")
	}
	input := []byte(`{"data":{"language":"en","request":{"input":"I use Java","profile":{"additionalInfo":"","careerGoals":"","competencies":"","currentSalary":"","desiredSalary":"","employmentStatus":"","experience":[],"projects":[],"skills":"","tools":""}},"task":"profile_ingestion"},"version":"profile-ingestion-task-v1"}`)
	if !bytes.Equal(baseline.Input, input) {
		t.Fatalf("unexpected canonical task record: %s", baseline.Input)
	}
	for _, change := range []func(map[string]any){
		func(c map[string]any) {
			c["request"].(map[string]any)["profile"].(map[string]any)["projects"] = []any{"first", "second"}
		},
		func(c map[string]any) {
			c["request"].(map[string]any)["profile"].(map[string]any)["projects"] = []any{"second", "first"}
		},
	} {
		candidate, err := lab.ExtractTaskSidecar("semantic-quality", mutateTaskFixture(t, raw, change), "en-fact")
		if err != nil {
			t.Fatal(err)
		}
		if candidate.Ref.InputHash == baseline.Ref.InputHash {
			t.Fatal("array value changes disappeared")
		}
		baseline = candidate
	}
	for _, test := range []struct{ suite, id string }{{"section-review", "en-fact"}, {"semantic-quality", "en-short-job"}, {"semantic-quality", "unknown"}} {
		if _, err := lab.ExtractTaskSidecar(test.suite, raw, test.id); err == nil {
			t.Fatal("unsupported projection silently accepted")
		}
	}
	raw[0] = '!'
	if canonical.OriginalFixture.Original[0] != '{' {
		t.Fatal("caller mutation changed retained original bytes")
	}
}

package evaluationlab_test

import (
	"bytes"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
	"strings"
	"testing"
)

func TestRetainedSemanticHistoryPreservesRawBytesAndOriginalOrigin(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/live.initial.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	result, err := lab.Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Quarantined || !bytes.Equal(raw, result.Original) || result.OriginalVersion != "semantic-quality-report-v1" || result.OriginMode != "live" || result.Derivation.Mode != "offline-import" {
		t.Fatalf("lost historical provenance: %+v", result)
	}
	if len(result.Cases) == 0 {
		t.Fatal("missing case detail")
	}
	for _, c := range result.Cases {
		if c.OriginalLanguage == "pt" && c.Language != "pt-BR" {
			t.Fatalf("lost language mapping: %+v", c)
		}
	}
	raw[0] = '!'
	if result.Original[0] != '{' {
		t.Fatal("import did not take immutable ownership of bytes")
	}
}

func TestImportRejectsMalformedDeepAndOversizeInputsAndQuarantinesUnregisteredData(t *testing.T) {
	for _, raw := range [][]byte{[]byte(`{"x":`), []byte(`{} {}`), []byte(strings.Repeat("[", 33) + "0" + strings.Repeat("]", 33)), bytes.Repeat([]byte(" "), lab.MaxImportBytes+1)} {
		if _, err := lab.Import(raw); err == nil {
			t.Fatal("invalid bounded import accepted")
		}
	}
	for _, raw := range [][]byte{[]byte(`{"version":"future","mode":"live","trusted":true}`), []byte(`{"path":"/etc/passwd","command":"go run","provider":"https://example.com"}`)} {
		r, err := lab.Import(raw)
		if err != nil || !r.Quarantined || len(r.Cases) != 0 || !bytes.Equal(raw, r.Original) {
			t.Fatalf("untrusted input escaped quarantine: %+v %v", r, err)
		}
	}
}

func TestRetainedFormatsExposeEveryEvidenceRowWithoutInventingLanguage(t *testing.T) {
	for _, tc := range []struct {
		path   string
		count  int
		format string
	}{
		{"ai-diagnostics.controlled.v1.json", 34, "diagnostics-legacy-v1"},
		{"qualification-evidence/controlled-report.json", 52, "qualification-evidence-legacy-v1"},
		{"application-context/controlled-report.json", 53, "application-context-legacy-v1"},
		{"workflow-settings/reviewed.v1.json", 42, "workflow-settings-v1"},
	} {
		raw, err := os.ReadFile("../../docs/evaluations/" + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		r, err := lab.Import(raw)
		if err != nil || r.Quarantined || r.Format != tc.format || len(r.Cases) != tc.count {
			t.Fatalf("%s: format %s count %d err %v", tc.path, r.Format, len(r.Cases), err)
		}
		if !bytes.Equal(raw, r.Original) {
			t.Fatal("original report mutated")
		}
	}
}

func TestReviewedSettingsRetainInferenceAndReviewDerivationSeparately(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/workflow-settings/reviewed.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.OriginMode != "live" || r.OriginTimestamp != "2026-10-10T23:48:41Z" || r.ParentHash != "f93f658ea837c0fd52283ad40ebe4fa69ae315c790dc5f1b159a581535f905e9" || r.ReviewDerivation != "offline-source-review" {
		t.Fatalf("lost inference/review chain: %+v", r)
	}
	found := false
	for _, c := range r.Cases {
		if len(c.Review) > 0 && string(c.Review) != "null" {
			found = true
		}
	}
	if !found {
		t.Fatal("review annotations missing")
	}
}

func TestImportedWorkflowKeepsEngineAssertionAndOperationalStatesSeparate(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/workflow-settings/controlled.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range r.Cases {
		if c.Workflow != "application_draft" {
			continue
		}
		if c.EngineState != "unavailable" || c.AssertionVerdict != "unavailable" || c.OperationalOutcome != "completed" || c.Identity == "" {
			t.Fatalf("conflated imported state: %+v", c)
		}
		found = true
	}
	if !found {
		t.Fatal("missing retained application evidence")
	}
}

func TestImportJoinsOnlyExactInferenceEraFixtureDependencies(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/live.initial.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("../../docs/evaluations/semantic-quality/history/cases.initial.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.ImportWithDependencies(raw, [][]byte{fixture})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Dependencies) != 1 || r.Provenance.FixtureHash != "480c03fa24a7f79c12038435a0e6be33b31c45a513fbae57e6406f33a81e0382" {
		t.Fatal("missing exact dependency")
	}
	for _, c := range r.Cases {
		if len(c.Expected) == 0 || len(c.Source) == 0 {
			t.Fatalf("missing source/gold for %s", c.ID)
		}
	}
	fixture[0] = '!'
	if _, err := lab.ImportWithDependencies(raw, [][]byte{fixture}); err == nil {
		t.Fatal("unapproved dependency accepted")
	}
}

func TestExpectedNegativeUsesBoundFixtureAssertionsWithoutUpgradingQuality(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/controlled.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("../../docs/evaluations/semantic-quality/history/cases.inference.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.ImportWithDependencies(raw, [][]byte{fixture})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range r.Cases {
		if c.ID == "en-draft-invented-management" {
			if c.AssertionVerdict != "pass" || len(c.ExpectedFailures) != 2 || c.AssertionVersion == "" {
				t.Fatalf("expected negative not interpreted: %+v", c)
			}
			return
		}
	}
	t.Fatal("missing retained negative control")
}

func TestBoundSequencePreservesSourceAndGoldAtEachObservedLeaf(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/workflow-settings/controlled.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("../../docs/evaluations/workflow-settings/cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.ImportWithDependencies(raw, [][]byte{fixture})
	if err != nil {
		t.Fatal(err)
	}
	leaves := 0
	for _, c := range r.Cases {
		for _, child := range c.Children {
			leaves++
			if len(child.Source) == 0 || len(child.Expected) == 0 || child.ParentIdentity != c.Identity {
				t.Fatalf("lost leaf source or identity: %+v", child)
			}
		}
	}
	if leaves != 36 {
		t.Fatalf("unexpected bound sequence leaves: %d", leaves)
	}
}

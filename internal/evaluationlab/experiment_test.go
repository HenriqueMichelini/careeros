package evaluationlab_test

import (
	"encoding/json"
	lab "professional-information-repo/internal/evaluationlab"
	"strings"
	"testing"
)

func experimentDigest(id string) lab.DigestRef {
	return lab.DigestRef{ID: id, Version: "v1", Hash: strings.Repeat("a", 64)}
}

func experimentDefinition() lab.ExperimentDefinition {
	variant := lab.VariantRef{
		ID: "legacy", AdapterVersion: "adapter-v1",
		ImplementationManifest: experimentDigest("implementation"), BuildReceipt: experimentDigest("build"),
		Configuration: experimentDigest("configuration"), PromptManifest: experimentDigest("prompt"),
		StrategyManifest: experimentDigest("strategy"), ProducerContract: experimentDigest("producer"),
		Normalizer: experimentDigest("normalizer"), SimulationScript: experimentDigest("script"),
		Dependencies: map[string]lab.DigestRef{"source/adapter.go": experimentDigest("adapter-source")},
	}
	right := variant
	right.ID = "staged"
	right.PromptManifest.Hash = strings.Repeat("b", 64)
	right.ProducerContract.Hash = strings.Repeat("c", 64)
	return lab.ExperimentDefinition{
		Version: lab.ExperimentVersion, ID: "synthetic-experiment", Kind: "complete-design",
		TaskContract: experimentDigest("task"), Evaluator: experimentDigest("evaluator"),
		Cases: []lab.TaskCaseRef{{SuiteID: "semantic-quality", CaseID: "en-case", TaskVersion: lab.IngestionTaskVersion,
			InputHash: strings.Repeat("d", 64), ExpectationHash: strings.Repeat("e", 64), Language: "en",
			RepeatOrdinal: 1, Split: "development", ExposureGroup: "unknown-historical-pairing", OriginalFixtureHash: strings.Repeat("f", 64)}},
		Variants: [2]lab.VariantRef{variant, right}, RepeatCount: 1,
		SharedPolicy: lab.ExperimentSharedPolicy{Reference: experimentDigest("policy"), Mode: "controlled", RuntimeVersion: "go-test/linux/amd64",
			ClockBoundary: "task-to-proposal-v1", SchedulePolicy: "alternating-pairs-v1", Concurrency: 1,
			RetryPolicy: "none", FallbackPolicy: "none", ResumePolicy: "none",
			Caps: lab.ExecutionCaps{MaxCases: 32, MaxHTTPAttempts: 100, MaxProviderCalls: 100, MaxArtifactBytes: 10000, DeadlineMS: 60000}},
		Differences: []lab.ExperimentDifference{
			{Field: "promptManifest.hash", Left: strings.Repeat("a", 64), Right: strings.Repeat("b", 64), Rationale: "Different prompt templates"},
			{Field: "producerContract.hash", Left: strings.Repeat("a", 64), Right: strings.Repeat("c", 64), Rationale: "Different intermediate schemas"},
		},
	}
}

func experimentEvidence(def lab.ExperimentDefinition, side int) lab.ExperimentEvidence {
	// Clone as callers would when resolving independent stored evidence.
	raw, _ := json.Marshal(def)
	var copy lab.ExperimentDefinition
	_ = json.Unmarshal(raw, &copy)
	return lab.ExperimentEvidence{TaskContract: copy.TaskContract, Cases: copy.Cases, Evaluator: copy.Evaluator, SharedPolicy: copy.SharedPolicy, Variant: copy.Variants[side]}
}

func TestExperimentCompleteDesignAcceptsExactMultipleDifferences(t *testing.T) {
	def, err := lab.FreezeExperiment(experimentDefinition())
	if err != nil {
		t.Fatal(err)
	}
	result := lab.CheckExperiment(def, experimentEvidence(def, 0), experimentEvidence(def, 1))
	if result.Status != "valid" || len(result.Reasons) != 0 || len(result.Differences) != 2 || len(def.Hash) != 64 {
		t.Fatalf("exact declared design rejected: %+v", result)
	}
}

func TestExperimentRejectsInexactDifferenceDeclarations(t *testing.T) {
	tests := []struct {
		name string
		edit func(*lab.ExperimentDefinition)
	}{
		{"undeclared", func(d *lab.ExperimentDefinition) { d.Differences = d.Differences[:1] }},
		{"wrong value", func(d *lab.ExperimentDefinition) { d.Differences[0].Right = strings.Repeat("f", 64) }},
		{"no rationale", func(d *lab.ExperimentDefinition) { d.Differences[0].Rationale = "   " }},
		{"unused declaration", func(d *lab.ExperimentDefinition) {
			d.Differences = append(d.Differences, lab.ExperimentDifference{Field: "adapterVersion", Left: "adapter-v1", Right: "adapter-v1", Rationale: "unchanged"})
		}},
		{"duplicate declaration", func(d *lab.ExperimentDefinition) { d.Differences = append(d.Differences, d.Differences[0]) }},
		{"source wildcard", func(d *lab.ExperimentDefinition) { d.Differences[0].Field = "dependencies.*" }},
		{"shared evaluator waiver", func(d *lab.ExperimentDefinition) {
			d.Differences = append(d.Differences, lab.ExperimentDifference{Field: "evaluator.hash", Left: "a", Right: "b", Rationale: "changed evaluator"})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			def := experimentDefinition()
			test.edit(&def)
			if _, err := lab.FreezeExperiment(def); err == nil {
				t.Fatal("inexact declarations admitted")
			}
		})
	}
}

func TestExperimentKindsSeparateTreatmentsFromConsequencesAndRepeats(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*lab.ExperimentDefinition)
		valid bool
	}{
		{"single treatment with pinned consequence", func(d *lab.ExperimentDefinition) {
			d.Kind = "single-dimension"
			d.Differences[1].ConsequenceOf = "promptManifest.hash"
		}, true},
		{"two independent treatments", func(d *lab.ExperimentDefinition) { d.Kind = "single-dimension" }, false},
		{"unidentified consequence", func(d *lab.ExperimentDefinition) {
			d.Kind = "single-dimension"
			d.Differences[1].ConsequenceOf = "missing.hash"
		}, false},
		{"consequence cycle", func(d *lab.ExperimentDefinition) {
			d.Kind = "single-dimension"
			d.Differences[0].ConsequenceOf = "producerContract.hash"
			d.Differences[1].ConsequenceOf = "promptManifest.hash"
		}, false},
		{"repeatability", func(d *lab.ExperimentDefinition) {
			d.Kind = "repeatability"
			d.Variants[1] = d.Variants[0]
			d.Differences = nil
		}, true},
		{"repeatability with changes", func(d *lab.ExperimentDefinition) { d.Kind = "repeatability" }, false},
		{"unchanged single treatment", func(d *lab.ExperimentDefinition) {
			d.Kind = "single-dimension"
			d.Variants[1] = d.Variants[0]
			d.Differences = nil
		}, false},
		{"unchanged design", func(d *lab.ExperimentDefinition) { d.Variants[1] = d.Variants[0]; d.Differences = nil }, false},
		{"unknown kind", func(d *lab.ExperimentDefinition) { d.Kind = "architecture-anything" }, false},
		{"two frozen repetitions", func(d *lab.ExperimentDefinition) {
			d.RepeatCount = 2
			ref := d.Cases[0]
			ref.RepeatOrdinal = 2
			d.Cases = append(d.Cases, ref)
		}, true},
		{"missing repetition", func(d *lab.ExperimentDefinition) { d.RepeatCount = 2 }, false},
		{"duplicate repetition", func(d *lab.ExperimentDefinition) { d.Cases = append(d.Cases, d.Cases[0]) }, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			def := experimentDefinition()
			test.edit(&def)
			frozen, err := lab.FreezeExperiment(def)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, err=%v", test.valid, err)
			}
			if err == nil && lab.CheckExperiment(frozen, experimentEvidence(frozen, 0), experimentEvidence(frozen, 1)).Status != "valid" {
				t.Fatal("valid frozen kind rejected")
			}
		})
	}
}

func TestExperimentSharedEvidenceCannotBeWaivedAndMissingnessStaysUnavailable(t *testing.T) {
	def, err := lab.FreezeExperiment(experimentDefinition())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, status string
		edit         func(*lab.ExperimentEvidence)
	}{
		{"task", "invalid", func(e *lab.ExperimentEvidence) { e.TaskContract.Hash = strings.Repeat("1", 64) }},
		{"input", "invalid", func(e *lab.ExperimentEvidence) { e.Cases[0].InputHash = strings.Repeat("1", 64) }},
		{"labels", "invalid", func(e *lab.ExperimentEvidence) { e.Cases[0].ExpectationHash = strings.Repeat("1", 64) }},
		{"evaluator", "invalid", func(e *lab.ExperimentEvidence) { e.Evaluator.Hash = strings.Repeat("1", 64) }},
		{"ceiling", "invalid", func(e *lab.ExperimentEvidence) { e.SharedPolicy.Caps.MaxProviderCalls++ }},
		{"runtime", "invalid", func(e *lab.ExperimentEvidence) { e.SharedPolicy.RuntimeVersion = "other-runtime" }},
		{"source drift", "invalid", func(e *lab.ExperimentEvidence) {
			r := e.Variant.Dependencies["source/adapter.go"]
			r.Hash = strings.Repeat("1", 64)
			e.Variant.Dependencies["source/adapter.go"] = r
		}},
		{"wrong variant", "invalid", func(e *lab.ExperimentEvidence) { e.Variant.ID = "unknown" }},
		{"missing evaluator", "unavailable", func(e *lab.ExperimentEvidence) { e.Evaluator = lab.DigestRef{} }},
		{"missing input", "unavailable", func(e *lab.ExperimentEvidence) { e.Cases[0].InputHash = "" }},
		{"missing cohort", "unavailable", func(e *lab.ExperimentEvidence) { e.Cases = nil }},
		{"missing build", "unavailable", func(e *lab.ExperimentEvidence) { e.Variant.BuildReceipt = lab.DigestRef{} }},
		{"missing runtime", "unavailable", func(e *lab.ExperimentEvidence) { e.SharedPolicy.RuntimeVersion = "" }},
		{"missing dependencies", "unavailable", func(e *lab.ExperimentEvidence) { e.Variant.Dependencies = nil }},
		{"malformed digest", "invalid", func(e *lab.ExperimentEvidence) { e.Variant.BuildReceipt.Hash = "not-a-hash" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			left, right := experimentEvidence(def, 0), experimentEvidence(def, 1)
			test.edit(&right)
			result := lab.CheckExperiment(def, left, right)
			if result.Status != test.status || len(result.Reasons) == 0 {
				t.Fatalf("want %s, got %+v", test.status, result)
			}
		})
	}
	empty := lab.CheckExperiment(lab.ExperimentDefinition{}, lab.ExperimentEvidence{}, lab.ExperimentEvidence{})
	if empty.Status != "unavailable" {
		t.Fatalf("legacy absence became equality or known invalidity: %+v", empty)
	}
	for _, edit := range []func(*lab.ExperimentDefinition){
		func(d *lab.ExperimentDefinition) { d.Evaluator.Hash = "" },
		func(d *lab.ExperimentDefinition) { d.Version = "" },
		func(d *lab.ExperimentDefinition) { d.Variants[0].Dependencies = nil },
		func(d *lab.ExperimentDefinition) { d.SharedPolicy.Caps.DeadlineMS = 0 },
	} {
		candidate := experimentDefinition()
		edit(&candidate)
		if _, err := lab.FreezeExperiment(candidate); err == nil {
			t.Fatal("incomplete definition frozen")
		}
	}
}

func TestExperimentFrozenContentOwnsInputsAndDetectsTampering(t *testing.T) {
	source := experimentDefinition()
	group := "translation-family"
	source.Cases[0].DependencyGroup = &group
	frozen, err := lab.FreezeExperiment(source)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(frozen)
	source.Cases[0].InputHash = strings.Repeat("1", 64)
	group = "changed"
	source.Differences[0].Rationale = "changed"
	source.Variants[0].Dependencies["source/adapter.go"] = experimentDigest("changed")
	unchanged, _ := json.Marshal(frozen)
	if string(original) != string(unchanged) {
		t.Fatal("caller mutated frozen receipt")
	}
	left, right := experimentEvidence(frozen, 0), experimentEvidence(frozen, 1)
	if lab.CheckExperiment(frozen, left, right).Status != "valid" {
		t.Fatal("caller mutation invalidated owned receipt")
	}
	frozen.Differences[0].Rationale = "new reason"
	if lab.CheckExperiment(frozen, left, right).Status != "invalid" {
		t.Fatal("tampered receipt retained frozen authority")
	}
	revised, err := lab.FreezeExperiment(frozen)
	if err != nil {
		t.Fatal(err)
	}
	if revised.Hash == frozen.Hash {
		t.Fatal("changed content kept old hash")
	}
	if lab.CheckExperiment(revised, left, right).Status != "valid" {
		t.Fatal("explicit new receipt rejected")
	}
}

func TestExperimentReportsActualExactDependencyChanges(t *testing.T) {
	def := experimentDefinition()
	def.Variants[1].Dependencies = map[string]lab.DigestRef{"source/adapter.go": experimentDigest("adapter-source")}
	ref := def.Variants[1].Dependencies["source/adapter.go"]
	ref.Hash = strings.Repeat("2", 64)
	def.Variants[1].Dependencies["source/adapter.go"] = ref
	def.Differences = append(def.Differences, lab.ExperimentDifference{Field: "dependencies.source/adapter.go.hash", Left: strings.Repeat("a", 64), Right: strings.Repeat("2", 64), Rationale: "Exact adapter implementation change"})
	frozen, err := lab.FreezeExperiment(def)
	if err != nil {
		t.Fatal(err)
	}
	left, right := experimentEvidence(frozen, 0), experimentEvidence(frozen, 1)
	accepted := lab.CheckExperiment(frozen, left, right)
	if accepted.Status != "valid" || len(accepted.Differences) != 3 {
		t.Fatalf("exact source change rejected: %+v", accepted)
	}
	ref = right.Variant.Dependencies["source/adapter.go"]
	ref.Hash = strings.Repeat("3", 64)
	right.Variant.Dependencies["source/adapter.go"] = ref
	result := lab.CheckExperiment(frozen, left, right)
	found := false
	for _, diff := range result.Differences {
		if diff.Field == "dependencies.source/adapter.go.hash" && diff.Right == strings.Repeat("3", 64) {
			found = true
		}
	}
	if result.Status != "invalid" || !found {
		t.Fatalf("actual source drift hidden behind frozen declaration: %+v", result)
	}
}

func TestExperimentRejectsKnownDriftAlongsideMissingMetadata(t *testing.T) {
	def, err := lab.FreezeExperiment(experimentDefinition())
	if err != nil {
		t.Fatal(err)
	}
	left, right := experimentEvidence(def, 0), experimentEvidence(def, 1)
	right.Cases[0].InputHash = ""
	right.Cases[0].ExpectationHash = strings.Repeat("1", 64)
	if result := lab.CheckExperiment(def, left, right); result.Status != "invalid" {
		t.Fatalf("known label drift hidden by absent input: %+v", result)
	}
	repeated := experimentDefinition()
	repeated.RepeatCount = 2
	ref := repeated.Cases[0]
	ref.RepeatOrdinal = 2
	ref.ExpectationHash = strings.Repeat("1", 64)
	repeated.Cases = append(repeated.Cases, ref)
	if _, err := lab.FreezeExperiment(repeated); err == nil {
		t.Fatal("repeated case changed its fixed labels")
	}
}

func TestExperimentRejectsDuplicateCohortWithMissingFixtureMetadata(t *testing.T) {
	def, err := lab.FreezeExperiment(experimentDefinition())
	if err != nil {
		t.Fatal(err)
	}
	left, right := experimentEvidence(def, 0), experimentEvidence(def, 1)
	duplicate := right.Cases[0]
	duplicate.OriginalFixtureHash = ""
	right.Cases = append(right.Cases, duplicate)
	result := lab.CheckExperiment(def, left, right)
	duplicateFound, missingFound := false, false
	for _, reason := range result.Reasons {
		duplicateFound = duplicateFound || (reason.Code == "case_mismatch" && strings.Contains(reason.Detail, "duplicate"))
		missingFound = missingFound || (reason.Code == "metadata_missing" && reason.Field == "right.cases.originalFixture.hash")
	}
	if result.Status != "invalid" || !duplicateFound || !missingFound {
		t.Fatalf("known duplicate or missing fixture metadata hidden: %+v", result)
	}
}

func TestExperimentRejectsUnexpectedCaseWithMissingInputMetadata(t *testing.T) {
	def, err := lab.FreezeExperiment(experimentDefinition())
	if err != nil {
		t.Fatal(err)
	}
	left, right := experimentEvidence(def, 0), experimentEvidence(def, 1)
	right.Cases[0].CaseID = "unexpected-case"
	right.Cases[0].InputHash = ""
	result := lab.CheckExperiment(def, left, right)
	cohortFound, missingFound := false, false
	for _, reason := range result.Reasons {
		cohortFound = cohortFound || (reason.Code == "case_mismatch" && reason.Field == "right.cases")
		missingFound = missingFound || (reason.Code == "metadata_missing" && reason.Field == "right.cases.input.hash")
	}
	if result.Status != "invalid" || !cohortFound || !missingFound {
		t.Fatalf("known unexpected case or missing input metadata hidden: %+v", result)
	}
}

func TestExperimentRejectsChangedCohortSizeWithMissingMetadata(t *testing.T) {
	source := experimentDefinition()
	second := source.Cases[0]
	second.CaseID = "second-case"
	source.Cases = append(source.Cases, second)
	def, err := lab.FreezeExperiment(source)
	if err != nil {
		t.Fatal(err)
	}
	left, right := experimentEvidence(def, 0), experimentEvidence(def, 1)
	right.Cases = right.Cases[:1]
	right.Cases[0].OriginalFixtureHash = ""
	result := lab.CheckExperiment(def, left, right)
	cohortFound, missingFound := false, false
	for _, reason := range result.Reasons {
		cohortFound = cohortFound || (reason.Code == "case_mismatch" && reason.Field == "right.cases")
		missingFound = missingFound || (reason.Code == "metadata_missing" && reason.Field == "right.cases.originalFixture.hash")
	}
	if result.Status != "invalid" || !cohortFound || !missingFound {
		t.Fatalf("known cohort size change or missing metadata hidden: %+v", result)
	}
}

func TestExperimentIncompleteCohortWithoutKnownMismatchStaysUnavailable(t *testing.T) {
	def, err := lab.FreezeExperiment(experimentDefinition())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		edit func(*lab.ExperimentEvidence)
	}{
		{"fixture identity", func(e *lab.ExperimentEvidence) { e.Cases[0].OriginalFixtureHash = "" }},
		{"input identity", func(e *lab.ExperimentEvidence) { e.Cases[0].InputHash = "" }},
		{"partial pairing identity", func(e *lab.ExperimentEvidence) {
			e.Cases[0].CaseID = ""
			e.Cases[0].InputHash = ""
			e.Cases[0].RepeatOrdinal = 0
		}},
		{"unknown case", func(e *lab.ExperimentEvidence) { e.Cases[0] = lab.TaskCaseRef{} }},
		{"absent cohort", func(e *lab.ExperimentEvidence) { e.Cases = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			left, right := experimentEvidence(def, 0), experimentEvidence(def, 1)
			test.edit(&right)
			result := lab.CheckExperiment(def, left, right)
			if result.Status != "unavailable" || len(result.Reasons) == 0 {
				t.Fatalf("incomplete evidence became equality or known invalidity: %+v", result)
			}
			for _, reason := range result.Reasons {
				if reason.Code != "metadata_missing" {
					t.Fatalf("unknown identity treated as known mismatch: %+v", result)
				}
			}
		})
	}
}

func TestExperimentPreservesStrictComparatorAndRejectsUnsupportedPolicy(t *testing.T) {
	strict := lab.ComparisonContract{FixtureHash: "f", LabelHash: "l", SourceVersion: "s", PromptVersion: "p", SchemaVersion: "j", ScorerVersion: "e", StrategyVersion: "t", BudgetVersion: "b", RuntimeVersion: "r", Mode: "controlled", Configuration: "current"}
	changed := strict
	changed.PromptVersion = "other-prompt"
	changed.SchemaVersion = "other-schema"
	result := lab.CheckComparison(strict, changed, "prompt")
	if result.Compatible || result.Version != "lab-comparison-v1" {
		t.Fatalf("architecture rules weakened strict comparison: %+v", result)
	}
	for _, edit := range []func(*lab.ExperimentDefinition){
		func(d *lab.ExperimentDefinition) { d.SharedPolicy.Mode = "live" },
		func(d *lab.ExperimentDefinition) { d.SharedPolicy.RetryPolicy = "automatic" },
		func(d *lab.ExperimentDefinition) { d.SharedPolicy.Concurrency = 2 },
		func(d *lab.ExperimentDefinition) { d.SharedPolicy.Caps.DeadlineMS = -1 },
		func(d *lab.ExperimentDefinition) { d.Version = "lab-experiment-unknown" },
		func(d *lab.ExperimentDefinition) {
			d.Variants[0].Dependencies = map[string]lab.DigestRef{"source/*": experimentDigest("wildcard")}
		},
	} {
		def := experimentDefinition()
		edit(&def)
		if _, err := lab.FreezeExperiment(def); err == nil {
			t.Fatal("unsupported definition frozen")
		}
	}
}

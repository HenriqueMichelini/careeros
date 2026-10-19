package evaluationlab_test

import (
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func TestTaskCasesPairExactDistinctRepetitionsAndPreserveDependence(t *testing.T) {
	sidecar, err := lab.ExtractTaskSidecar("semantic-quality", taskFixture(t), "en-proficiency-conflict")
	if err != nil {
		t.Fatal(err)
	}
	first := sidecar.Ref
	second := first
	second.RepeatOrdinal = 2
	pairs, err := lab.PairTaskCases([]lab.TaskCaseRef{first, second}, []lab.TaskCaseRef{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 2 || pairs[0].Left.RepeatOrdinal != 1 || pairs[0].Right.RepeatOrdinal != 1 || pairs[1].Left.RepeatOrdinal != 2 || pairs[1].Right.RepeatOrdinal != 2 {
		t.Fatal("distinct repetitions did not pair once each")
	}
	if pairs[0].Left.ExposureGroup != "unknown-historical-pairing" || pairs[0].Right.DependencyGroup != nil || pairs[0].Left.Split != "heldout" {
		t.Fatal("dependent/unknown cohort metadata changed")
	}
	if _, err := lab.PairTaskCases([]lab.TaskCaseRef{first, first}, []lab.TaskCaseRef{first, second}); err == nil {
		t.Fatal("duplicate left pair key accepted")
	}
	if _, err := lab.PairTaskCases([]lab.TaskCaseRef{first, second}, []lab.TaskCaseRef{first, first}); err == nil {
		t.Fatal("duplicate right pair key accepted")
	}
}

func TestTaskCasesRejectSharedContractDriftAndMalformedReferences(t *testing.T) {
	sidecar, err := lab.ExtractTaskSidecar("semantic-quality", taskFixture(t), "en-fact")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*lab.TaskCaseRef)
	}{
		{"task version", func(ref *lab.TaskCaseRef) { ref.TaskVersion = "different-task-v1" }},
		{"case", func(ref *lab.TaskCaseRef) { ref.CaseID = "pt-fact" }},
		{"input", func(ref *lab.TaskCaseRef) {
			ref.InputHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}},
		{"language", func(ref *lab.TaskCaseRef) { ref.Language = "pt-BR" }},
		{"repetition", func(ref *lab.TaskCaseRef) { ref.RepeatOrdinal = 2 }},
		{"expectations", func(ref *lab.TaskCaseRef) {
			ref.ExpectationHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}},
		{"suite", func(ref *lab.TaskCaseRef) { ref.SuiteID = "section-review" }},
		{"split", func(ref *lab.TaskCaseRef) { ref.Split = "heldout" }},
		{"exposure", func(ref *lab.TaskCaseRef) { ref.ExposureGroup = "new-group" }},
		{"dependence", func(ref *lab.TaskCaseRef) { ref.DependencyGroup = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := sidecar.Ref
			test.change(&changed)
			if _, err := lab.PairTaskCases([]lab.TaskCaseRef{sidecar.Ref}, []lab.TaskCaseRef{changed}); err == nil {
				t.Fatal("shared contract drift accepted")
			}
		})
	}
	for _, change := range []func(*lab.TaskCaseRef){
		func(ref *lab.TaskCaseRef) { ref.RepeatOrdinal = 0 },
		func(ref *lab.TaskCaseRef) { ref.InputHash = "" },
		func(ref *lab.TaskCaseRef) { ref.ExpectationHash = "not-a-digest" },
		func(ref *lab.TaskCaseRef) { ref.TaskVersion = "" },
	} {
		ref := sidecar.Ref
		change(&ref)
		if _, err := lab.PairTaskCases([]lab.TaskCaseRef{ref}, []lab.TaskCaseRef{ref}); err == nil {
			t.Fatal("malformed reference accepted")
		}
	}
	if _, err := lab.PairTaskCases(nil, nil); err == nil {
		t.Fatal("empty cohort passed pairing")
	}
}

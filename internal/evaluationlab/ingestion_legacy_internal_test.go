package evaluationlab

import (
	"bytes"
	"encoding/json"
	"professional-information-repo/internal/semanticeval"
	"reflect"
	"testing"
)

func TestLegacyIngestionProposalCaseCopiesOnlyExecutionFields(t *testing.T) {
	request := json.RawMessage(`{"input":"I use Java","profile":{"skills":""}}`)
	provider := []json.RawMessage{json.RawMessage(`{"claims":[]}`), json.RawMessage(`{"operations":[]}`)}
	envelopes := []json.RawMessage{json.RawMessage(`{"choices":[]}`)}
	full := semanticeval.Case{
		ID:                "en-fact",
		Task:              "profile_ingestion",
		Language:          "en",
		Split:             "development",
		Request:           request,
		Provider:          provider,
		ProviderEnvelopes: envelopes,
		ProviderStatuses:  []int{200, 200},
		Gold:              []semanticeval.Atom{{ID: "java-fact", Group: "claims", Match: map[string]string{"text": "Java"}}},
		Forbidden:         []string{"expert"},
		ExpectedFailures:  []string{"HTTP 500"},
		SourceSpans:       []semanticeval.SourceSpan{{ID: "span"}},
		Entities:          []string{"Java"},
		ExpectedStatus:    200,
		ExpectedCalls:     2,
		AssessorScenario:  "scenario",
		Adjudication:      "accepted",
		SequenceOutput:    "final",
		Steps:             []semanticeval.Case{{ID: "step"}},
		Notes:             "notes",
		Tags:              []string{"tag"},
		Role:              "role",
		SourceID:          "source",
	}
	got := legacyIngestionProposalCase(full)
	if got.ID != full.ID || got.Task != full.Task || got.Language != full.Language || got.Split != full.Split {
		t.Fatalf("task identity was not copied: %+v", got)
	}
	if !bytes.Equal(got.Request, request) || !reflect.DeepEqual(got.Provider, provider) || !reflect.DeepEqual(got.ProviderEnvelopes, envelopes) || !reflect.DeepEqual(got.ProviderStatuses, []int{200, 200}) {
		t.Fatalf("execution inputs were not copied exactly: %+v", got)
	}
	if got.Gold != nil || got.Forbidden != nil || got.ExpectedFailures != nil || got.SourceSpans != nil || got.Entities != nil {
		t.Fatalf("expectation arrays leaked into execution: %+v", got)
	}
	if got.ExpectedStatus != 0 || got.ExpectedCalls != 0 || got.AssessorScenario != "" || got.Adjudication != "" || got.SequenceOutput != "" || got.Steps != nil {
		t.Fatalf("expectation scalars or steps leaked into execution: %+v", got)
	}
	if got.Notes != "" || got.Tags != nil || got.Role != "" || got.SourceID != "" {
		t.Fatalf("non-execution metadata was copied: %+v", got)
	}

	// The execution copy owns its mutable slices; changing it cannot alter the
	// frozen fixture-derived case or its provider tape.
	got.ProviderStatuses[0] = 500
	got.Provider[0][0] = '['
	got.Request[0] = '['
	if full.ProviderStatuses[0] != 200 || full.Provider[0][0] != '{' || full.Request[0] != '{' {
		t.Fatal("execution sanitization did not copy mutable inputs")
	}
}

func TestLegacyIngestionCohortNormalizesOnlyTheDocumentedFiveCases(t *testing.T) {
	want := []string{"en-fact", "pt-fact", "en-negation-aspiration", "en-existing-duplicate", "en-proficiency-conflict"}
	got, err := legacyIngestionCohort(nil)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("empty selection did not normalize to the cohort: %v %v", got, err)
	}
	got[0] = "mutated"
	again, err := legacyIngestionCohort(nil)
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("cohort was not independently owned: %v %v", again, err)
	}
	reversed := []string{want[4], want[3], want[2], want[1], want[0]}
	normalized, err := legacyIngestionCohort(reversed)
	if err != nil || !reflect.DeepEqual(normalized, want) {
		t.Fatalf("documented cohort order was not restored: %v %v", normalized, err)
	}
	for _, invalid := range [][]string{
		{"en-fact"},
		want[:4],
		append(append([]string{}, want...), "en-fact"),
		append(append([]string{}, want[:4]...), "en-ambiguous-date"),
		{"en-fact", "pt-fact", "en-negation-aspiration", "en-existing-duplicate", "en-existing-duplicate"},
	} {
		if got, err := legacyIngestionCohort(invalid); err == nil || got != nil {
			t.Fatalf("accepted non-cohort selection %v: %v %v", invalid, got, err)
		}
	}
}

func TestLegacyIngestionPlanFunctionRejectsOtherSelectionsBeforePlanning(t *testing.T) {
	for _, selection := range []Selection{
		{Suite: "workflow-settings", Mode: "controlled", Configuration: "current"},
		{Suite: "semantic-quality", Mode: "live", Configuration: "current"},
		{Suite: "semantic-quality", Mode: "controlled", Configuration: "luna-low"},
	} {
		if plan, err := planLegacyIngestion(nil, selection, ExecutionCaps{}); err == nil {
			t.Fatalf("accepted unsupported selection %+v: %+v", selection, plan.Selection)
		}
	}
}

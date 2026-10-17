package evaluationlab_test

import (
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func TestCatalogRejectsUnsupportedModesConfigurationsAndUnknownCases(t *testing.T) {
	suites := lab.Catalog()
	if len(suites) < 4 {
		t.Fatal("missing offline and controlled suites")
	}
	for _, suite := range suites {
		if suite.ID == "" || suite.Version == "" || suite.FixtureHash == "" || len(suite.Cases) == 0 || suite.Sensitivity != "approved-synthetic" {
			t.Fatalf("incomplete suite: %+v", suite)
		}
	}
	for _, selection := range []lab.Selection{
		{Suite: "semantic-quality", Mode: "live", Configuration: "current"},
		{Suite: "semantic-quality", Mode: "controlled", Configuration: "arbitrary-model"},
		{Suite: "semantic-quality", Mode: "controlled", Configuration: "current", CaseIDs: []string{"unknown"}},
	} {
		if err := lab.ValidateSelection(selection); err == nil {
			t.Fatalf("unsupported selection accepted: %+v", selection)
		}
	}
	if err := lab.ValidateSelection(lab.Selection{Suite: "semantic-quality", Mode: "controlled", Configuration: "current"}); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationCatalogIncludesRetrievalAndOutputReplayCases(t *testing.T) {
	for _, suite := range lab.Catalog() {
		if suite.ID != "application-context" {
			continue
		}
		if len(suite.Cases) != 23 || suite.FixtureHashes["outputs"] == "" || suite.FixtureHashes["baseline"] == "" {
			t.Fatalf("incomplete application replay plan: %+v", suite)
		}
		for _, c := range suite.Cases {
			if c.MaximumCalls < 1 {
				t.Fatal("simulated provider legs omitted from cap")
			}
		}
		return
	}
	t.Fatal("missing application suite")
}

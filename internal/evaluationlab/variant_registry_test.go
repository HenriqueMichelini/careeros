package evaluationlab_test

import (
	"context"
	"net/http"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestResolvedVariantPlansAndExecutesOwnedControlledBehavior(t *testing.T) {
	variant, err := lab.ResolveVariant("semantic-quality", "semantic-quality-native-v1", "current")
	if err != nil {
		t.Fatal(err)
	}
	planner := semanticPlanner(t)
	plan, err := variant.Plan(planner, []string{"en-fact"}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 5, MaxArtifactBytes: 1 << 20, DeadlineMS: 5000})
	if err != nil {
		t.Fatal(err)
	}
	runner := lab.NewRunner(planner, lab.RunnerOptions{})
	id, err := variant.Start(context.Background(), runner, plan)
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, runner, id)
	if record.EngineState != "completed" || record.VariantID != "semantic-quality-native-v1" || len(record.Cases) != 1 || record.Cases[0].ID != "en-fact" || len(record.Dispatches) == 0 {
		t.Fatalf("owned adapter did not execute selected case: %+v", record)
	}
	for _, dispatch := range record.Dispatches {
		if dispatch.Origin != "simulated" {
			t.Fatalf("non-controlled dispatch: %+v", dispatch)
		}
	}
}

func TestResolvedVariantKeepsArchitectureSeparateFromModelConfiguration(t *testing.T) {
	for _, configuration := range []string{"current", "luna-low", "sol-low"} {
		variant, err := lab.ResolveVariant("workflow-settings", "workflow-settings-native-v1", configuration)
		if err != nil {
			t.Fatal(err)
		}
		descriptor := variant.Descriptor()
		if descriptor.ID != "workflow-settings-native-v1" || descriptor.SuiteID != "workflow-settings" || descriptor.AdapterVersion != "lab-native-adapter-v1" {
			t.Fatalf("configuration changed architecture identity: %+v", descriptor)
		}
		raw, err := os.ReadFile("../../docs/evaluations/workflow-settings/cases.v1.json")
		if err != nil {
			t.Fatal(err)
		}
		planner, err := lab.NewPlanner(map[string][]byte{"workflow-settings": raw})
		if err != nil {
			t.Fatal(err)
		}
		plan, err := variant.Plan(planner, []string{"en-fact"}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 5, MaxArtifactBytes: 1 << 20, DeadlineMS: 5000})
		if err != nil {
			t.Fatal(err)
		}
		if plan.Selection.Configuration != configuration || plan.Selection.Mode != "controlled" || plan.Selection.Suite != "workflow-settings" {
			t.Fatalf("configuration was not independently bound: %+v", plan.Selection)
		}
	}
}

func TestResolvedVariantRejectsUnknownAndCrossSuiteSelectors(t *testing.T) {
	for _, selector := range [][3]string{
		{"unknown", "semantic-quality-native-v1", "current"},
		{"workflow-settings", "semantic-quality-native-v1", "current"},
		{"semantic-quality", "semantic-quality-native-v1", "luna-low"},
		{"semantic-quality", "ingestion-submission-proposal-v1", "current"},
		{"semantic-quality", "ingestion-staged-proposal-v1", "current"},
		{"semantic-quality", "go test ./...", "current"},
		{"semantic-quality", "/tmp/adapter", "current"},
		{"semantic-quality", "semantic-quality-native-v1", "OPENAI_API_KEY=sentinel"},
		{"semantic-quality", "semantic-quality-native-v1", "current "},
	} {
		if _, err := lab.ResolveVariant(selector[0], selector[1], selector[2]); err == nil {
			t.Fatalf("accepted unowned selector: %q", selector)
		}
	}
}

func TestResolvedVariantRejectsMismatchedAndAlteredPlansBeforeExecution(t *testing.T) {
	variant, err := lab.ResolveVariant("semantic-quality", "semantic-quality-native-v1", "current")
	if err != nil {
		t.Fatal(err)
	}
	planner := semanticPlanner(t)
	plan := factPlan(t, planner)
	runner := lab.NewRunner(planner, lab.RunnerOptions{})
	for _, field := range []string{"suite", "mode", "configuration", "case", "hash"} {
		altered := plan
		switch field {
		case "suite":
			altered.Selection.Suite = "workflow-settings"
		case "mode":
			altered.Selection.Mode = "live"
		case "configuration":
			altered.Selection.Configuration = "sol-low"
		case "case":
			altered.Selection.CaseIDs = []string{"pt-fact"}
		case "hash":
			altered.Hash = "forged"
		}
		if id, err := variant.Start(context.Background(), runner, altered); err == nil || id != "" {
			t.Fatalf("accepted %s drift: id=%s err=%v", field, id, err)
		}
	}
	if id, err := variant.Start(context.Background(), runner, plan); err != nil || id == "" {
		t.Fatalf("rejected valid frozen plan after rejected mutations: id=%s err=%v", id, err)
	} else {
		awaitRun(t, runner, id)
	}
	var unresolved lab.ResolvedVariant
	if _, err := unresolved.Plan(planner, []string{"en-fact"}, plan.Caps); err == nil {
		t.Fatal("zero handle planned work")
	}
	if _, err := unresolved.Start(context.Background(), runner, plan); err == nil {
		t.Fatal("zero handle started work")
	}
	if _, err := variant.Plan(nil, []string{"en-fact"}, plan.Caps); err == nil {
		t.Fatal("nil planner accepted")
	}
	if _, err := variant.Start(context.Background(), nil, plan); err == nil {
		t.Fatal("nil runner accepted")
	}
}

func TestCatalogDescribesSevenSuiteBoundariesWithoutInventingProjectors(t *testing.T) {
	expected := map[string][3]string{
		"semantic-quality":       {"workflow-handler-output", "curated-label-completeness", "conditional"},
		"workflow-settings":      {"workflow-sequence-output", "curated-label-completeness", "conditional"},
		"qualification-evidence": {"qualification-match-state", "controlled-match-state-agreement", "unavailable"},
		"application-context":    {"application-draft-field", "curated-label-completeness", "unavailable"},
		"section-review":         {"section-review-patches", "curated-label-completeness", "partial"},
		"cv-integrity-evidence":  {"retained-cv-client-boundary", "scripted-unsupported-active-machine-acceptance", "partial"},
		"test-adapters":          {"isolated-test-process", "fixed-test-process-exit-assertion", "unavailable"},
	}
	catalog := lab.Catalog()
	if len(catalog) != 7 {
		t.Fatalf("suite coverage changed: %d", len(catalog))
	}
	for _, suite := range catalog {
		want, ok := expected[suite.ID]
		if !ok {
			t.Fatalf("unknown suite: %s", suite.ID)
		}
		capability := suite.ComparisonCapabilities
		if capability.Version != "lab-comparison-capabilities-v1" || capability.StrictProjector.Status != want[2] || len(capability.StrictProjector.Reasons) == 0 {
			t.Fatalf("missing honest strict capability: %s %+v", suite.ID, capability)
		}
		if capability.ArchitectureProjector.Status != "unavailable" || len(capability.ArchitectureProjector.Measurements) != 0 || len(capability.ArchitectureProjector.Reasons) == 0 {
			t.Fatalf("invented architecture projector: %s %+v", suite.ID, capability)
		}
		found := false
		for _, endpoint := range capability.Endpoints {
			if endpoint.Boundary != want[0] {
				continue
			}
			for _, measurement := range endpoint.Measurements {
				if measurement == want[1] {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("missing supported native endpoint for %s: %+v", suite.ID, capability.Endpoints)
		}
		wantVariants := 1
		if suite.ID == "semantic-quality" {
			wantVariants = 2
		}
		if len(suite.Variants) != wantVariants {
			t.Fatalf("unexpected executable variants: %s %+v", suite.ID, suite.Variants)
		}
		for _, descriptor := range suite.Variants {
			if descriptor.SuiteID != suite.ID {
				t.Fatalf("wrong adapter suite: %+v", descriptor)
			}
			wantAdapter := "lab-native-adapter-v1"
			if descriptor.ID == lab.LegacyIngestionVariantID {
				wantAdapter = lab.LegacyIngestionAdapterVersion
			}
			if descriptor.AdapterVersion != wantAdapter {
				t.Fatalf("wrong adapter version: %+v", descriptor)
			}
			for _, configuration := range suite.Configurations {
				resolved, err := lab.ResolveVariant(suite.ID, descriptor.ID, configuration)
				if err != nil || resolved.Descriptor().ID != descriptor.ID {
					t.Fatalf("catalog adapter cannot resolve: %s %s %v", suite.ID, configuration, err)
				}
			}
		}
		if capability.StrictProjector.Status == "unavailable" && len(capability.StrictProjector.Measurements) != 0 {
			t.Fatalf("unsupported strict projector advertises measurements: %s", suite.ID)
		}
	}
}

func TestCatalogAndResolvedDescriptorsOwnAllNestedMetadata(t *testing.T) {
	original := lab.Catalog()
	changed := lab.Catalog()
	for i := range changed {
		suite := &changed[i]
		suite.Variants[0].ID = "injected-adapter"
		suite.Variants[0].Modes[0] = "live"
		suite.Variants[0].Configurations[0] = "injected-config"
		capability := &suite.ComparisonCapabilities
		capability.Version = "injected-version"
		capability.Endpoints[0].Measurements[0] = "invented-measure"
		capability.Endpoints[0].Limitations[0] = "invented-limit"
		capability.Endpoints[0].Scope = "invented-scope"
		capability.StrictProjector.Reasons[0] = "invented-reason"
		if len(capability.StrictProjector.Measurements) > 0 {
			capability.StrictProjector.Measurements[0] = "invented-delta"
		}
		capability.ArchitectureProjector.Reasons[0] = "invented-authority"
		capability.ArchitectureProjector.Measurements = append(capability.ArchitectureProjector.Measurements, "invented-delta")
	}
	if !reflect.DeepEqual(original, lab.Catalog()) {
		t.Fatal("caller mutation changed server-owned catalog metadata")
	}
	variant, err := lab.ResolveVariant("workflow-settings", "workflow-settings-native-v1", "current")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := variant.Descriptor()
	descriptor.Modes[0] = "live"
	descriptor.Configurations[0] = "injected-config"
	next := variant.Descriptor()
	if next.Modes[0] != "controlled" || next.Configurations[0] != "current" {
		t.Fatal("caller mutation changed resolved execution metadata")
	}
	if _, err := lab.ResolveVariant("workflow-settings", "workflow-settings-native-v1", "injected-config"); err == nil {
		t.Fatal("caller metadata granted configuration authority")
	}
}

func TestResolvedVariantsExecuteAllSixInProcessNativeSuitesWithoutExternalTransport(t *testing.T) {
	read := func(path string) []byte {
		raw, err := os.ReadFile("../../docs/evaluations/" + path)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	labels := read("qualification-evidence/cases.json")
	assets := map[string][]byte{
		"semantic-quality":       read("semantic-quality/cases.v2.json"),
		"workflow-settings":      read("workflow-settings/cases.v1.json"),
		"qualification-evidence": labels,
		"application-context":    labels,
		"application-baseline":   read("semantic-quality/controlled.v2.json"),
		"section-review":         read("section-review/cases.v1.json"),
		"cv-integrity-evidence":  read("cv-lab/evidence.v1.json"),
	}
	var external atomic.Int64
	previous := http.DefaultTransport
	http.DefaultTransport = deniedTransport{calls: &external}
	t.Cleanup(func() { http.DefaultTransport = previous })
	for _, suite := range lab.Catalog() {
		if suite.ID == "test-adapters" {
			continue
		}
		t.Run(suite.ID, func(t *testing.T) {
			variant, err := lab.ResolveVariant(suite.ID, suite.Variants[0].ID, suite.Configurations[0])
			if err != nil {
				t.Fatal(err)
			}
			planner, err := lab.NewPlanner(assets)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := variant.Plan(planner, []string{suite.Cases[0].ID}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 10, MaxProviderCalls: 20, MaxArtifactBytes: 8 << 20, DeadlineMS: 10000})
			if err != nil {
				t.Fatal(err)
			}
			runner := lab.NewRunner(planner, lab.RunnerOptions{})
			id, err := variant.Start(context.Background(), runner, plan)
			if err != nil {
				t.Fatal(err)
			}
			record := awaitRun(t, runner, id)
			if record.EngineState != "completed" || record.Selection.Suite != suite.ID || len(record.Cases) != 1 || record.Cases[0].ID != suite.Cases[0].ID || len(record.Metrics) == 0 {
				t.Fatalf("native adapter did not preserve suite behavior: %+v", record)
			}
			for _, dispatch := range record.Dispatches {
				if dispatch.Origin != "simulated" {
					t.Fatalf("non-simulated dispatch: %+v", dispatch)
				}
			}
		})
	}
	if external.Load() != 0 {
		t.Fatalf("native adapters attempted %d external requests", external.Load())
	}
}

func TestResolvedVariantRejectsAnotherValidFrozenSuiteOrConfigurationPlan(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/workflow-settings/cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	planner, err := lab.NewPlanner(map[string][]byte{"workflow-settings": raw})
	if err != nil {
		t.Fatal(err)
	}
	current, err := lab.ResolveVariant("workflow-settings", "workflow-settings-native-v1", "current")
	if err != nil {
		t.Fatal(err)
	}
	low, err := lab.ResolveVariant("workflow-settings", "workflow-settings-native-v1", "luna-low")
	if err != nil {
		t.Fatal(err)
	}
	otherSuite, err := lab.ResolveVariant("semantic-quality", "semantic-quality-native-v1", "current")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := current.Plan(planner, []string{"en-fact"}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 5, MaxArtifactBytes: 1 << 20, DeadlineMS: 5000})
	if err != nil {
		t.Fatal(err)
	}
	runner := lab.NewRunner(planner, lab.RunnerOptions{})
	for _, mismatched := range []lab.ResolvedVariant{otherSuite, low} {
		if id, err := mismatched.Start(context.Background(), runner, plan); err == nil || id != "" {
			t.Fatalf("another resolved adapter admitted a valid mismatched frozen plan: id=%s err=%v", id, err)
		}
	}
	id, err := current.Start(context.Background(), runner, plan)
	if err != nil {
		t.Fatal(err)
	}
	if record := awaitRun(t, runner, id); record.EngineState != "completed" {
		t.Fatalf("correct adapter could not execute unchanged plan: %+v", record)
	}
}

package evaluationlab

import (
	"context"
	"fmt"
)

// VariantDescriptor is catalog metadata; it contains no executable authority.
type VariantDescriptor struct {
	ID             string   `json:"id"`
	AdapterVersion string   `json:"adapterVersion"`
	SuiteID        string   `json:"suiteId"`
	Modes          []string `json:"modes"`
	Configurations []string `json:"configurations"`
}

type variantRegistration struct {
	id      string
	suiteID string
	plan    func(*Planner, Selection, ExecutionCaps) (Plan, error)
	start   func(*Runner, context.Context, Plan) (string, error)
}

// Entries contain owned code, never names loaded from a request or artifact.
var variantRegistry = []variantRegistration{
	{id: "semantic-quality-native-v1", suiteID: "semantic-quality", plan: (*Planner).Plan, start: (*Runner).Start},
	{id: "workflow-settings-native-v1", suiteID: "workflow-settings", plan: (*Planner).Plan, start: (*Runner).Start},
	{id: "qualification-evidence-native-v1", suiteID: "qualification-evidence", plan: (*Planner).Plan, start: (*Runner).Start},
	{id: "application-context-native-v1", suiteID: "application-context", plan: (*Planner).Plan, start: (*Runner).Start},
	{id: "section-review-native-v1", suiteID: "section-review", plan: (*Planner).Plan, start: (*Runner).Start},
	{id: "cv-integrity-evidence-native-v1", suiteID: "cv-integrity-evidence", plan: (*Planner).Plan, start: (*Runner).Start},
	{id: "test-adapters-native-v1", suiteID: "test-adapters", plan: (*Planner).Plan, start: (*Runner).Start},
}

// ResolvedVariant binds an owned adapter to an independently selected model
// configuration. Its zero value cannot plan or execute work.
type ResolvedVariant struct {
	registration *variantRegistration
	selection    Selection
}

// ResolveVariant accepts exact catalog IDs only. Resolution neither executes
// work nor supplies build attestation or architecture-comparison authority.
func ResolveVariant(suiteID, variantID, configurationID string) (ResolvedVariant, error) {
	for i := range variantRegistry {
		registration := &variantRegistry[i]
		if registration.suiteID != suiteID || registration.id != variantID {
			continue
		}
		for _, suite := range suiteCatalog {
			if suite.ID == suiteID && contains(suite.Configurations, configurationID) {
				return ResolvedVariant{registration: registration, selection: Selection{Suite: suiteID, Mode: suite.Modes[0], Configuration: configurationID}}, nil
			}
		}
	}
	return ResolvedVariant{}, fmt.Errorf("unknown suite, variant or configuration ID")
}

func (v ResolvedVariant) Plan(planner *Planner, caseIDs []string, caps ExecutionCaps) (Plan, error) {
	if v.registration == nil || planner == nil {
		return Plan{}, fmt.Errorf("resolved variant or planner unavailable")
	}
	selection := v.selection
	selection.CaseIDs = append([]string{}, caseIDs...)
	return v.registration.plan(planner, selection, caps)
}

func (v ResolvedVariant) Start(ctx context.Context, runner *Runner, plan Plan) (string, error) {
	if v.registration == nil || runner == nil {
		return "", fmt.Errorf("resolved variant or runner unavailable")
	}
	if plan.Selection.Suite != v.selection.Suite || plan.Selection.Mode != v.selection.Mode || plan.Selection.Configuration != v.selection.Configuration {
		return "", fmt.Errorf("plan does not match resolved variant")
	}
	return v.registration.start(runner, ctx, plan)
}

func (v ResolvedVariant) Descriptor() VariantDescriptor {
	if v.registration == nil {
		return VariantDescriptor{}
	}
	for _, suite := range suiteCatalog {
		if suite.ID == v.registration.suiteID {
			return VariantDescriptor{ID: v.registration.id, AdapterVersion: "lab-native-adapter-v1", SuiteID: suite.ID, Modes: append([]string{}, suite.Modes...), Configurations: append([]string{}, suite.Configurations...)}
		}
	}
	return VariantDescriptor{}
}

func suiteVariants(suite Suite) []VariantDescriptor {
	descriptors := []VariantDescriptor{}
	for _, registration := range variantRegistry {
		if registration.suiteID == suite.ID {
			descriptors = append(descriptors, VariantDescriptor{ID: registration.id, AdapterVersion: "lab-native-adapter-v1", SuiteID: suite.ID, Modes: append([]string{}, suite.Modes...), Configurations: append([]string{}, suite.Configurations...)})
		}
	}
	return descriptors
}

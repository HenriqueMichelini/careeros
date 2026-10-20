package evaluationlab

// ComparisonCapabilities describes implemented observations separately from
// projectors. Catalog support never establishes a particular run's eligibility.
type ComparisonCapabilities struct {
	Version               string               `json:"version"`
	Endpoints             []ComparisonEndpoint `json:"endpoints"`
	StrictProjector       ComparisonProjector  `json:"strictProjector"`
	ArchitectureProjector ComparisonProjector  `json:"architectureProjector"`
}

type ComparisonEndpoint struct {
	Boundary     string   `json:"boundary"`
	Scope        string   `json:"scope"`
	Measurements []string `json:"measurements"`
	Limitations  []string `json:"limitations"`
}

type ComparisonProjector struct {
	Status       string   `json:"status"`
	Measurements []string `json:"measurements"`
	Reasons      []string `json:"reasons"`
}

func suiteComparisonCapabilities(suiteID string) ComparisonCapabilities {
	curated := []string{"curated-label-precision", "curated-label-completeness", "curated-label-omission"}
	deltas := []string{"curatedForbiddenAdditionsDelta", "curatedMissingLabelsDelta", "curatedRelationshipErrorsDelta"}
	capability := ComparisonCapabilities{
		Version:               "lab-comparison-capabilities-v1",
		Endpoints:             []ComparisonEndpoint{},
		StrictProjector:       ComparisonProjector{Status: "unavailable", Measurements: []string{}, Reasons: []string{}},
		ArchitectureProjector: ComparisonProjector{Status: "unavailable", Measurements: []string{}, Reasons: []string{}},
	}
	switch suiteID {
	case "semantic-quality":
		capability.Endpoints = []ComparisonEndpoint{{Boundary: "workflow-handler-output", Scope: "workflow-specific response before Apply/Save", Measurements: curated, Limitations: []string{"Finite curated labels for Profile ingestion, qualification matching, CV generation and Application Draft; not semantic truth or user acceptance."}}}
		capability.StrictProjector = ComparisonProjector{Status: "conditional", Measurements: deltas, Reasons: []string{"Complete strict metadata, exactly one changed dimension and usable paired root scores required; only current configuration is registered."}}
		capability.ArchitectureProjector.Reasons = []string{"Common endpoint normalizers/evaluator and executable ingestion architecture adapters are not registered."}
	case "workflow-settings":
		capability.Endpoints = []ComparisonEndpoint{{Boundary: "workflow-sequence-output", Scope: "selected workflow or frozen user-action sequence, before Apply/Save", Measurements: curated, Limitations: []string{"Sequence roots are scored; child stages are not independently paired. Model/reasoning settings do not select an architecture."}}}
		capability.StrictProjector = ComparisonProjector{Status: "conditional", Measurements: deltas, Reasons: []string{"Complete strict metadata, exactly one changed dimension and usable paired root scores required; configurations share the native adapter."}}
		capability.ArchitectureProjector.Reasons = []string{"No shared endpoint architecture projector or second executable architecture adapter is registered."}
	case "qualification-evidence":
		capability.Endpoints = []ComparisonEndpoint{
			{Boundary: "candidate-retrieval", Scope: "requirement facts per byte budget", Measurements: []string{"post-budget-relevant-retention", "post-budget-negative-retention"}, Limitations: []string{"Unique curated fact IDs per case and budget; not Recall@K."}},
			{Boundary: "qualification-match-state", Scope: "controlled final matching state per case", Measurements: []string{"controlled-match-state-agreement"}, Limitations: []string{"Scripted match-state agreement is not live semantic accuracy."}},
		}
		capability.StrictProjector.Reasons = []string{"Native wrappers have no top-level semantic score; strict comparison does not unpack retrieval or final-state rows."}
		capability.ArchitectureProjector.Reasons = []string{"Retrieval and final-state comparison projectors are not implemented."}
	case "application-context":
		capability.Endpoints = []ComparisonEndpoint{
			{Boundary: "candidate-retrieval", Scope: "application facts per byte budget", Measurements: []string{"post-budget-relevant-retention", "post-budget-negative-retention"}, Limitations: []string{"Candidate recall and output fields have separate opportunity sets."}},
			{Boundary: "application-draft-field", Scope: "jobSummary, resume, coverLetter or applicationAnswers per case", Measurements: curated, Limitations: []string{"Controlled saved-output replay; field denominators remain separate."}},
		}
		capability.StrictProjector.Reasons = []string{"Native wrappers have no top-level semantic score; strict comparison does not unpack field or retrieval rows."}
		capability.ArchitectureProjector.Reasons = []string{"Retrieval and per-field comparison projectors are not implemented."}
	case "section-review":
		capability.Endpoints = []ComparisonEndpoint{
			{Boundary: "section-review-patches", Scope: "material patches facet per reviewed section", Measurements: curated, Limitations: []string{"Material patches use their own facet; generic root scores cannot replace that facet."}},
			{Boundary: "outbound-request", Scope: "scripted private canary per request envelope", Measurements: []string{"scripted-outbound-private-canary-exposure"}, Limitations: []string{"Injected canary exposure is not a full privacy or semantic assessment."}},
		}
		capability.StrictProjector = ComparisonProjector{Status: "partial", Measurements: deltas, Reasons: []string{"Generic complete root-score deltas are possible under strict admission; the strict comparator ignores the material patches facet."}}
		capability.ArchitectureProjector.Reasons = []string{"Material-patch and privacy comparison projectors are not implemented."}
	case "cv-integrity-evidence":
		capability.Endpoints = []ComparisonEndpoint{{Boundary: "retained-cv-client-boundary", Scope: "exact retained owner, machine-origin and client observation joins", Measurements: []string{"scripted-unsupported-active-machine-acceptance", "scripted-unsafe-case-acceptance"}, Limitations: []string{"Replays immutable owner evidence; not another implementation execution. Human semantic judgments remain pending."}}}
		capability.StrictProjector = ComparisonProjector{Status: "partial", Measurements: deltas, Reasons: []string{"Only completed retained handler roots with curated scores can supply generic deltas; browser/contract records and CV boundary measures are unprojected."}}
		capability.ArchitectureProjector.Reasons = []string{"Retained CV boundary comparison projector is not implemented; replay cannot establish an executed architecture variant."}
	case "test-adapters":
		capability.Endpoints = []ComparisonEndpoint{{Boundary: "isolated-test-process", Scope: "fixed allowlisted process selector", Measurements: []string{"fixed-test-process-exit-assertion", "injected-process-isolation-violations", "isolated-process-duration"}, Limitations: []string{"Requires supported isolation, tools and unchanged reviewed sources; process success is not endpoint semantic quality or native-export acceptance."}}}
		capability.StrictProjector.Reasons = []string{"Process assertions have no semantic-score shape and cannot supply generic semantic deltas."}
		capability.ArchitectureProjector.Reasons = []string{"Typed isolated-process comparison projector is not implemented."}
	}
	return capability
}

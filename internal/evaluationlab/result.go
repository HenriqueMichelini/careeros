package evaluationlab

import "fmt"

const ResultVersion = "lab-result-v1"

type ExecutionContext struct {
	Mode                 string   `json:"mode"`
	RuntimeVersion       string   `json:"runtimeVersion"`
	Concurrency          int      `json:"concurrency"`
	Order                []string `json:"caseOrder"`
	MaxLogicalCases      int      `json:"maxLogicalCases"`
	MaxHTTPAttempts      int      `json:"maxHttpAttempts"`
	MaxProviderCalls     int      `json:"maxProviderCalls"`
	MaxArtifactBytes     int64    `json:"maxArtifactBytes"`
	DeadlineMS           int64    `json:"deadlineMs"`
	ConfigurationVersion string   `json:"configurationVersion"`
	StrategyVersion      string   `json:"strategyVersion"`
	PromptVersion        string   `json:"promptVersion"`
	SchemaVersion        string   `json:"schemaVersion"`
	ScorerVersion        string   `json:"scorerVersion"`
	LabelHash            string   `json:"labelHash"`
	UnavailableReasons   []string `json:"unavailableReasons"`
}

// RunRecord is the versioned storage/API contract. Engine execution, assertions,
// operational outcomes and persistence acknowledgments have distinct fields.
type RunRecord struct {
	Progress            RunProgress         `json:"progress"`
	Version             string              `json:"version"`
	ID                  string              `json:"id"`
	EngineState         string              `json:"engineState"`
	AssertionVerdict    string              `json:"assertionVerdict"`
	OperationalOutcomes map[string]int      `json:"operationalOutcomes"`
	Selection           Selection           `json:"selection"`
	Context             ExecutionContext    `json:"context"`
	Provenance          Provenance          `json:"provenance"`
	Derivations         []Derivation        `json:"derivations"`
	Cases               []ImportedCase      `json:"cases"`
	Dispatches          []Dispatch          `json:"dispatches"`
	Artifacts           []ArtifactReference `json:"approvedArtifacts"`
	Metrics             []Metric            `json:"metrics"`
	Durable             bool                `json:"durable"`
	UnavailableReasons  []string            `json:"unavailableReasons"`
}

type RunProgress struct {
	PlannedCases        int `json:"plannedCases"`
	StartedCases        int `json:"startedCases"`
	FinishedCases       int `json:"finishedCases"`
	PlannedHTTP         int `json:"plannedHttpAttempts"`
	AttemptedHTTP       int `json:"attemptedHttpAttempts"`
	FinishedHTTP        int `json:"finishedHttpAttempts"`
	UnreachedHTTP       int `json:"unreachedHttpAttempts"`
	AttemptedDispatches int `json:"attemptedDispatches"`
}

func ValidateResult(r RunRecord) error {
	if r.Version != ResultVersion || r.ID == "" {
		return fmt.Errorf("unsupported result version or missing identity")
	}
	if !contains([]string{"planned", "running", "completed", "failed", "canceled", "interrupted"}, r.EngineState) {
		return fmt.Errorf("unknown engine state")
	}
	if !contains([]string{"pass", "fail", "blocked", "skipped", "unavailable"}, r.AssertionVerdict) {
		return fmt.Errorf("unknown assertion verdict")
	}
	if r.Context.Concurrency != 1 || r.Context.Mode != r.Selection.Mode {
		return fmt.Errorf("invalid execution mode/concurrency")
	}
	if err := ValidateSelection(r.Selection); err != nil {
		return err
	}
	if r.Context.MaxLogicalCases < 0 || r.Context.MaxHTTPAttempts < 0 || r.Context.MaxProviderCalls < 0 || r.Context.MaxArtifactBytes < 0 || r.Context.DeadlineMS < 0 {
		return fmt.Errorf("invalid execution budgets")
	}
	return nil
}

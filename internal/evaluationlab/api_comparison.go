package evaluationlab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"professional-information-repo/internal/semanticeval"
)

func (a *API) comparisonEvidence(id string) (ComparisonContract, []ImportedCase, error) {
	if !storeID.MatchString(id) {
		return ComparisonContract{}, nil, fmt.Errorf("invalid run identity")
	}
	if record, err := a.store.Load(id); err == nil {
		source := ""
		if len(record.Provenance.SourceHashes) > 0 {
			raw, _ := json.Marshal(record.Provenance.SourceHashes)
			source = semanticeval.Hash(raw)
		}
		return ComparisonContract{FixtureHash: record.Provenance.FixtureHash, LabelHash: record.Context.LabelHash, SourceVersion: source, PromptVersion: record.Context.PromptVersion, SchemaVersion: record.Context.SchemaVersion, ScorerVersion: record.Context.ScorerVersion, StrategyVersion: record.Context.StrategyVersion, BudgetVersion: budgetContract(record.Context), RuntimeVersion: record.Context.RuntimeVersion, Mode: record.Selection.Mode, Configuration: record.Selection.Configuration}, record.Cases, nil
	}
	imported, err := a.store.LoadImport(id)
	if err != nil {
		return ComparisonContract{}, nil, err
	}
	if imported.Quarantined {
		return ComparisonContract{}, nil, fmt.Errorf("quarantined evidence")
	}
	// Unavailable historical dimensions remain empty and make the shared strict
	// comparison contract decline pairing. Never accept client-supplied versions.
	return ComparisonContract{FixtureHash: imported.Provenance.FixtureHash, SourceVersion: imported.Provenance.Commit, SchemaVersion: imported.OriginalVersion, Mode: imported.OriginMode}, imported.Cases, nil
}
func budgetContract(ctx ExecutionContext) string {
	raw, _ := json.Marshal(struct {
		Cases, HTTP, Providers int
		Artifacts, Deadline    int64
	}{ctx.MaxLogicalCases, ctx.MaxHTTPAttempts, ctx.MaxProviderCalls, ctx.MaxArtifactBytes, ctx.DeadlineMS})
	return semanticeval.Hash(raw)
}
func (a *API) compare(w http.ResponseWriter, r *http.Request) {
	var input struct {
		LeftID    string `json:"leftId"`
		RightID   string `json:"rightId"`
		Dimension string `json:"changedDimension"`
	}
	if err := a.decode(w, r, &input, 4096); err != nil || !contains([]string{"configuration", "source", "prompt", "schema", "scorer", "strategy", "budget"}, input.Dimension) {
		a.error(w, 400, "invalid_comparison", "Select stored evidence and one supported dimension.")
		return
	}
	left, leftCases, e1 := a.comparisonEvidence(input.LeftID)
	right, rightCases, e2 := a.comparisonEvidence(input.RightID)
	if e1 != nil || e2 != nil {
		a.error(w, 409, "comparison_unavailable", "Trusted stored comparison evidence is unavailable.")
		return
	}
	a.json(w, 200, CompareCases(left, right, input.Dimension, leftCases, rightCases))
}

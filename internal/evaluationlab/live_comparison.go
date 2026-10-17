package evaluationlab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"professional-information-repo/internal/semanticeval"
)

type GuardComparisonGroup struct {
	PreflightID   string `json:"preflightId"`
	PreflightHash string `json:"preflightHash"`
	CaseID        string `json:"caseId"`
	Configuration string `json:"configuration"`
	Language      string `json:"language"`
	ExposureGroup string `json:"exposureGroup"`
}
type GuardComparison struct {
	Version                    string                 `json:"version"`
	ID                         string                 `json:"id"`
	Hash                       string                 `json:"hash"`
	ChangedDimension           string                 `json:"changedDimension"`
	Schedule                   []GuardComparisonGroup `json:"schedule"`
	BudgetPerConfiguration     ExecutionCaps          `json:"budgetPerConfiguration"`
	TokenUnitsPerConfiguration int64                  `json:"tokenUnitsPerConfiguration"`
	MaxOutputTokens            int64                  `json:"maxOutputTokens"`
	Pooling                    string                 `json:"pooling"`
}

func guardComparisonHash(value GuardComparison) string {
	value.Hash = ""
	raw, _ := json.Marshal(value)
	return semanticeval.Hash(raw)
}
func guardComparisonContract(preflight LivePreflight) ComparisonContract {
	return ComparisonContract{FixtureHash: preflight.Identities["fixture"], LabelHash: preflight.Identities["labels"], SourceVersion: preflight.Identities["source"], PromptVersion: preflight.Identities["prompt"], SchemaVersion: preflight.Identities["schema"], ScorerVersion: preflight.Identities["scorer"], StrategyVersion: preflight.Identities["strategy"], BudgetVersion: budgetContract(ExecutionContext{MaxLogicalCases: preflight.Plan.Caps.MaxCases, MaxHTTPAttempts: preflight.Plan.Caps.MaxHTTPAttempts, MaxProviderCalls: preflight.Plan.Caps.MaxProviderCalls, MaxArtifactBytes: preflight.Plan.Caps.MaxArtifactBytes, DeadlineMS: preflight.Plan.Caps.DeadlineMS}), RuntimeVersion: preflight.Identities["runtime"], Mode: preflight.Plan.Selection.Mode, Configuration: preflight.Plan.Selection.Configuration}
}

// FreezeComparison binds single-case preflights into a finite paired schedule.
// Each run still requires its own immediate local human authorization.
func (g *LiveGuard) FreezeComparison(left, right []string, dimension string) (GuardComparison, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || !g.options.Enabled || g.active != "" || len(left) == 0 || len(left) != len(right) || len(left) > 32 {
		return GuardComparison{}, fmt.Errorf("bounded inactive paired preflights required")
	}
	result := GuardComparison{Version: "lab-live-comparison-v1", ID: newLabID(), ChangedDimension: dimension, Schedule: []GuardComparisonGroup{}, Pooling: "none; finite paired cases, languages and exposure groups remain separate"}
	seen := map[string]bool{}
	seenCases := map[string]bool{}
	var baselineLeft, baselineRight ComparisonContract
	for i := range left {
		a, aOK := g.records[left[i]]
		b, bOK := g.records[right[i]]
		if !aOK || !bOK || a.State != "planned" || b.State != "planned" || a.Comparison != nil || b.Comparison != nil || a.Preflight.Instance != g.instance || b.Preflight.Instance != g.instance || len(a.Preflight.Plan.CaseOrder) != 1 || len(b.Preflight.Plan.CaseOrder) != 1 || a.Preflight.Plan.CaseOrder[0] != b.Preflight.Plan.CaseOrder[0] || seen[left[i]] || seen[right[i]] || left[i] == right[i] {
			return GuardComparison{}, fmt.Errorf("distinct owned single-case pairs required")
		}
		caseID := a.Preflight.Plan.CaseOrder[0]
		if seenCases[caseID] {
			return GuardComparison{}, fmt.Errorf("duplicate paired case")
		}
		seenCases[caseID] = true
		seen[left[i]] = true
		seen[right[i]] = true
		ac, bc := guardComparisonContract(a.Preflight), guardComparisonContract(b.Preflight)
		compatibility := CheckComparison(ac, bc, dimension)
		if !compatibility.Compatible || a.Preflight.Plan.Caps != b.Preflight.Plan.Caps || a.Preflight.MaxTokenUnits != b.Preflight.MaxTokenUnits || a.Preflight.MaxOutputTokens != b.Preflight.MaxOutputTokens || a.Preflight.PriceSnapshotVersion != b.Preflight.PriceSnapshotVersion {
			return GuardComparison{}, fmt.Errorf("comparison labels, quality identities or budgets are incompatible")
		}
		if len(a.Preflight.Dispatches) != len(b.Preflight.Dispatches) {
			return GuardComparison{}, fmt.Errorf("comparison workflow coverage differs")
		}
		for j := range a.Preflight.Dispatches {
			leftCall, rightCall := a.Preflight.Dispatches[j], b.Preflight.Dispatches[j]
			leftCall.ID = ""
			rightCall.ID = ""
			leftCall.Model = ""
			rightCall.Model = ""
			leftCall.Reasoning = ""
			rightCall.Reasoning = ""
			if leftCall != rightCall {
				return GuardComparison{}, fmt.Errorf("comparison step inputs, coverage or reservations differ")
			}
		}
		ap, _ := json.Marshal(a.Preflight.Prices)
		bp, _ := json.Marshal(b.Preflight.Prices)
		if !bytes.Equal(ap, bp) {
			return GuardComparison{}, fmt.Errorf("comparison price snapshots differ")
		}
		if i == 0 {
			baselineLeft = ac
			baselineRight = bc
			result.MaxOutputTokens = a.Preflight.MaxOutputTokens
		} else if ac != baselineLeft || bc != baselineRight || result.MaxOutputTokens != a.Preflight.MaxOutputTokens {
			return GuardComparison{}, fmt.Errorf("comparison cohort identities differ")
		}
		groups := []GuardComparisonGroup{}
		for _, record := range []guardRecord{a, b} {
			group := GuardComparisonGroup{PreflightID: record.Preflight.ID, PreflightHash: record.Preflight.Hash, CaseID: caseID, Configuration: record.Preflight.Plan.Selection.Configuration}
			for _, suite := range suiteCatalog {
				if suite.ID == record.Preflight.Plan.Selection.Suite {
					for _, fixture := range suite.Cases {
						if fixture.ID == caseID {
							group.Language = fixture.Language
							group.ExposureGroup = fixture.ExposureGroup
						}
					}
				}
			}
			groups = append(groups, group)
		}
		if i%2 == 1 {
			groups[0], groups[1] = groups[1], groups[0]
		}
		result.Schedule = append(result.Schedule, groups...)
		caps := a.Preflight.Plan.Caps
		result.BudgetPerConfiguration.MaxCases += caps.MaxCases
		result.BudgetPerConfiguration.MaxHTTPAttempts += caps.MaxHTTPAttempts
		result.BudgetPerConfiguration.MaxProviderCalls += caps.MaxProviderCalls
		result.BudgetPerConfiguration.MaxArtifactBytes += caps.MaxArtifactBytes
		result.BudgetPerConfiguration.DeadlineMS += caps.DeadlineMS
		result.TokenUnitsPerConfiguration += a.Preflight.MaxTokenUnits
	}
	caps := result.BudgetPerConfiguration
	if caps.MaxCases*2 > 512 || caps.MaxHTTPAttempts*2 > 2048 || caps.MaxProviderCalls*2 > 4096 || caps.MaxArtifactBytes*2 > 16<<20 || caps.DeadlineMS*2 > 60000 || result.TokenUnitsPerConfiguration*2 > 1000000 {
		return GuardComparison{}, fmt.Errorf("combined comparison caps exceed guard ceilings")
	}
	result.Hash = guardComparisonHash(result)
	raw, _ := json.Marshal(result)
	if len(raw) > 1<<20 {
		return GuardComparison{}, fmt.Errorf("comparison artifact exceeds bounds")
	}
	for _, group := range result.Schedule {
		record := g.records[group.PreflightID]
		record.Comparison = cloneJSON(&result)
		if err := g.store.saveGuard(record); err != nil {
			g.closed = true
			g.store.releaseGuard(g.instance)
			return GuardComparison{}, fmt.Errorf("comparison retention failed; capability revoked")
		}
		g.records[group.PreflightID] = record
	}
	return result, nil
}
func guardApprovalHash(record guardRecord) string {
	if record.Comparison == nil {
		return record.Preflight.Hash
	}
	raw, _ := json.Marshal(struct{ Preflight, Comparison string }{record.Preflight.Hash, record.Comparison.Hash})
	return semanticeval.Hash(raw)
}
func (g *LiveGuard) comparisonAdmission(record guardRecord) error {
	if record.Comparison == nil {
		return nil
	}
	if record.Comparison.Hash != guardComparisonHash(*record.Comparison) {
		return fmt.Errorf("changed comparison")
	}
	for _, group := range record.Comparison.Schedule {
		prior, ok := g.records[group.PreflightID]
		if !ok || prior.Preflight.Hash != group.PreflightHash || prior.Comparison == nil || prior.Comparison.Hash != record.Comparison.Hash {
			return fmt.Errorf("comparison correspondence unavailable")
		}
		if prior.State == "completed" {
			continue
		}
		if group.PreflightID != record.Preflight.ID || prior.State != "planned" {
			return fmt.Errorf("next paired run required; failed or restarted comparisons cannot resume")
		}
		return nil
	}
	return fmt.Errorf("comparison already completed")
}
func (g *LiveGuard) ExportComparison(id string) ([]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, record := range g.records {
		if record.Comparison != nil && record.Comparison.ID == id {
			raw, err := json.Marshal(record.Comparison)
			if len(raw) > 1<<20 {
				return nil, fmt.Errorf("comparison export exceeds bounds")
			}
			return raw, err
		}
	}
	return nil, fmt.Errorf("unknown comparison")
}

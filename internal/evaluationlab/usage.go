package evaluationlab

import (
	"encoding/json"
	"fmt"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/semanticeval"
	"strings"
)

type Dispatch struct {
	State               string            `json:"state,omitempty"`
	LogicalCaseIdentity string            `json:"logicalCaseIdentity,omitempty"`
	Identity            string            `json:"identity"`
	CaseIdentity        string            `json:"caseIdentity"`
	Origin              string            `json:"origin"`
	Call                semanticeval.Call `json:"call"`
	UnavailableReasons  []string          `json:"unavailableReasons"`
}

type UsageSummary struct {
	Calls             int                         `json:"calls"`
	Origins           map[string]int              `json:"origins"`
	InconsistentCalls int                         `json:"inconsistentCalls"`
	Unpriced          int                         `json:"unpricedCalls"`
	KnownUSD          *float64                    `json:"knownPricedSubtotalUSD"`
	TotalUSD          *float64                    `json:"totalEstimatedUSD"`
	Input             semanticeval.TokensObserved `json:"inputTokens"`
	Output            semanticeval.TokensObserved `json:"outputTokens"`
	Reasoning         semanticeval.TokensObserved `json:"reasoningTokensSubsetOfOutput"`
	Cached            semanticeval.TokensObserved `json:"cachedTokensSubsetOfInput"`
	CacheWrite        semanticeval.TokensObserved `json:"cacheWriteTokensSubsetOfInput"`
	Prices            []semanticeval.Rate         `json:"datedExactModelPrices"`
}

type ResultCost struct {
	Cohort             string   `json:"cohort"`
	Completed          int      `json:"completedLogicalResults"`
	Accepted           *int     `json:"acceptedResults"`
	AcceptanceKind     string   `json:"acceptanceEvidenceKind,omitempty"`
	PerCompletedUSD    *float64 `json:"estimatedPerCompletedUSD"`
	PerAcceptedUSD     *float64 `json:"estimatedPerAcceptedUSD"`
	UnavailableReasons []string `json:"unavailableReasons"`
}

// ResultCosts uses the full named dispatch cohort, including failed calls, as
// the numerator. A caller must supply explicit named acceptance evidence.
func ResultCosts(usage UsageSummary, cohort string, completed int, accepted *int, kind string) (ResultCost, error) {
	r := ResultCost{Cohort: cohort, Completed: completed, AcceptanceKind: kind, UnavailableReasons: []string{}}
	if strings.TrimSpace(cohort) == "" || completed < 0 || accepted != nil && (*accepted < 0 || *accepted > completed) {
		return r, fmt.Errorf("invalid named result cohort")
	}
	if usage.TotalUSD == nil {
		r.UnavailableReasons = append(r.UnavailableReasons, "full cohort cost unavailable")
	}
	if usage.TotalUSD != nil && completed > 0 {
		v := *usage.TotalUSD / float64(completed)
		r.PerCompletedUSD = &v
	}
	if accepted != nil && strings.TrimSpace(kind) != "" {
		count := *accepted
		r.Accepted = &count
		if usage.TotalUSD != nil && count > 0 {
			v := *usage.TotalUSD / float64(count)
			r.PerAcceptedUSD = &v
		}
	} else {
		r.UnavailableReasons = append(r.UnavailableReasons, "named acceptance evidence unavailable")
	}
	return r, nil
}

// SummarizeDispatches preserves nullable components and charges every unique
// dispatch, including failure/cancellation. It does not infer accepted results.
func SummarizeDispatches(ledger []Dispatch, rates []semanticeval.Rate) (UsageSummary, error) {
	r := UsageSummary{Origins: map[string]int{}, Prices: append([]semanticeval.Rate{}, rates...)}
	seen := map[string]bool{}
	calls := []semanticeval.Call{}
	for _, dispatch := range ledger {
		if dispatch.Identity == "" || seen[dispatch.Identity] {
			return r, fmt.Errorf("missing or duplicate dispatch identity")
		}
		seen[dispatch.Identity] = true
		if dispatch.State == "planned" || dispatch.State == "not_dispatched" {
			continue
		}
		if !contains([]string{"simulated", "historical-provider-observed", "provider-observed", "unknown-final"}, dispatch.Origin) {
			return r, fmt.Errorf("unknown dispatch origin")
		}
		r.Origins[dispatch.Origin]++
		if len(r.Origins) > 1 {
			return r, fmt.Errorf("different evidence origins require separate usage cohorts")
		}
		validation := validateUsage(dispatch.Call)
		call := validation.Clean
		if validation.Inconsistent {
			r.InconsistentCalls++
			// Raw components remain in Dispatch; contradictory observations
			// cannot be priced even when some components remain valid.
			call.Provider = ""
		}
		calls = append(calls, call)
	}
	// Reuse the existing exact-model, dated-price accounting, without review
	// annotations or invented logical completion/acceptance denominators.
	summaries, err := semanticeval.Compare([]semanticeval.ComparisonRun{{Configuration: "current", Run: semanticeval.RunResult{Calls: calls}}}, rates)
	if err != nil {
		return r, err
	}
	s := summaries["current"]
	r.Calls = s.Calls
	r.Unpriced = s.Unpriced
	r.KnownUSD = s.KnownUSD
	r.TotalUSD = s.TotalUSD
	r.Input = s.InputTokens
	r.Output = s.OutputTokens
	r.Reasoning = s.ReasoningTokens
	r.Cached = s.CachedTokens
	r.CacheWrite = s.WriteTokens
	return r, nil
}

// DispatchLedger keeps the historical flattened call list once. Children still
// carry their original evidence; they do not create additional dispatches.
func DispatchLedger(imported ImportResult) []Dispatch {
	canonical, err := revalidateImport(imported)
	if err != nil || canonical.Quarantined {
		return nil
	}
	origin := "historical-provider-observed"
	if strings.HasPrefix(canonical.OriginMode, "controlled") {
		origin = "simulated"
	}
	var result []Dispatch
	for _, c := range canonical.Cases {
		caseOrigin := origin
		var calls []semanticeval.Call
		if c.Kind == "diagnostic-attempt" {
			var attempt aidiagnostics.Attempt
			if json.Unmarshal(c.Actual, &attempt) != nil {
				continue
			}
			for _, stage := range attempt.Stages {
				if stage.Provider == nil {
					continue
				}
				p := stage.Provider
				model := ""
				if p.Model != nil {
					model = *p.Model
				}
				calls = append(calls, semanticeval.Call{Provider: p.Name, ReturnedModel: model, Status: p.Status, DurationMS: p.DurationMS, InputTokens: p.InputTokens, OutputTokens: p.OutputTokens, ReasoningTokens: p.ReasoningTokens, CachedTokens: p.CachedTokens})
			}
		} else if c.Kind == "workflow-result" {
			var run semanticeval.RunResult
			if json.Unmarshal(c.Actual, &run) != nil {
				continue
			}
			calls = flattenedCalls(run)
			if strings.HasPrefix(run.Mode, "controlled") {
				caseOrigin = "simulated"
			}
		}
		for i, call := range calls {
			result = append(result, Dispatch{Identity: fmt.Sprintf("%s:dispatch:%d", c.Identity, i), CaseIdentity: c.Identity, Origin: caseOrigin, Call: call, UnavailableReasons: validateUsage(call).Reasons})
		}
	}
	return result
}

func flattenedCalls(run semanticeval.RunResult) []semanticeval.Call {
	if len(run.Calls) > 0 || len(run.Steps) == 0 {
		return run.Calls
	}
	var calls []semanticeval.Call
	for _, step := range run.Steps {
		calls = append(calls, flattenedCalls(step)...)
	}
	return calls
}

type usageValidation struct {
	Clean        semanticeval.Call
	Inconsistent bool
	Reasons      []string
}

func validateUsage(original semanticeval.Call) usageValidation {
	r := usageValidation{Clean: original, Reasons: []string{}}
	invalid := func(reason string, component **int64) {
		r.Inconsistent = true
		r.Reasons = append(r.Reasons, reason)
		*component = nil
	}
	for _, component := range []struct {
		name  string
		value **int64
	}{
		{"input", &r.Clean.InputTokens}, {"output", &r.Clean.OutputTokens}, {"cached", &r.Clean.CachedTokens}, {"cache-write", &r.Clean.CacheWriteTokens}, {"reasoning", &r.Clean.ReasoningTokens},
	} {
		if *component.value == nil {
			r.Reasons = append(r.Reasons, component.name+": unavailable")
		} else if **component.value < 0 {
			invalid(component.name+": invalid negative usage", component.value)
		}
	}
	c := &r.Clean
	if c.CachedTokens != nil && c.InputTokens != nil && *c.CachedTokens > *c.InputTokens {
		invalid("cached exceeds input", &c.CachedTokens)
	}
	if c.ReasoningTokens != nil && c.OutputTokens != nil && *c.ReasoningTokens > *c.OutputTokens {
		invalid("reasoning exceeds output", &c.ReasoningTokens)
	}
	if c.CacheWriteTokens != nil && c.InputTokens != nil {
		available := *c.InputTokens
		if c.CachedTokens != nil {
			available -= *c.CachedTokens
		}
		if *c.CacheWriteTokens > available {
			invalid("cache-write exceeds available input", &c.CacheWriteTokens)
		}
	}
	return r
}

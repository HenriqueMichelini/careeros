package semanticeval

import (
	"fmt"
	"math"
	"professional-information-repo/internal/aidiagnostics"
	"strings"
	"time"
)

type FieldReview struct {
	Field      string `json:"field"`
	Assessment string `json:"assessment"`
	Source     string `json:"source"`
	Finding    string `json:"finding"`
}
type Review struct {
	Fields   []FieldReview `json:"fields,omitempty"`
	Accepted bool          `json:"accepted"`
	Method   string        `json:"method"`
	Reason   string        `json:"reason"`
}
type ComparisonRun struct {
	Configuration string    `json:"configuration"`
	Repeat        bool      `json:"cacheObservationRepeat"`
	Run           RunResult `json:"run"`
	Review        *Review   `json:"review"`
}
type Rate struct {
	Model  string  `json:"model"`
	AsOf   string  `json:"asOf"`
	Input  float64 `json:"inputPerMillionUSD"`
	Cached float64 `json:"cachedPerMillionUSD"`
	Write  float64 `json:"cacheWritePerMillionUSD"`
	Output float64 `json:"outputPerMillionUSD"`
}
type ComparisonSummary struct {
	LeafAttempts                      int            `json:"leafAttempts"`
	RequiredUnreached                 int            `json:"requiredUnreached"`
	SequencesWithUnknownRequiredSteps int            `json:"sequencesWithUnknownRequiredSteps"`
	OutcomeNormalizerVersion          string         `json:"outcomeNormalizerVersion"`
	OperationalOutcomes               map[string]int `json:"operationalOutcomes"`
	Attempts                          int            `json:"attempts"`
	Completed                         int            `json:"completed"`
	Failed                            int            `json:"failed"`
	Accepted                          int            `json:"accepted"`
	Reviewed                          int            `json:"reviewed"`
	Calls                             int            `json:"providerCalls"`
	Unpriced                          int            `json:"unpricedCalls"`
	DurationMS                        float64        `json:"workflowDurationMs"`
	KnownUSD                          *float64       `json:"knownPricedComponentsUSD"`
	TotalUSD                          *float64       `json:"totalEstimatedUSD"`
	PerCompletedUSD                   *float64       `json:"estimatedPerCompletedUSD"`
	PerAcceptedUSD                    *float64       `json:"estimatedPerAcceptedUSD"`
	InputTokens                       TokensObserved `json:"inputTokens"`
	OutputTokens                      TokensObserved `json:"outputTokens"`
	ReasoningTokens                   TokensObserved `json:"reasoningTokensSubsetOfOutput"`
	CachedTokens                      TokensObserved `json:"cachedTokensSubsetOfInput"`
	WriteTokens                       TokensObserved `json:"cacheWriteTokensSubsetOfInput"`
}
type TokensObserved struct {
	Total   *int64 `json:"observed"`
	Missing int    `json:"unavailableCalls"`
}

func (t *TokensObserved) add(n *int64) {
	if n == nil {
		t.Missing++
		return
	}
	if t.Total == nil {
		t.Total = new(int64)
	}
	*t.Total += *n
}

// Compare retains failures in cost numerators and never infers review acceptance
// from HTTP status, source references, or curated regex scores.
func Compare(runs []ComparisonRun, rates []Rate) (map[string]*ComparisonSummary, error) {
	for _, r := range rates {
		if _, err := time.Parse("2006-01-02", r.AsOf); err != nil {
			return nil, fmt.Errorf("dated pricing required")
		}
		for _, v := range []float64{r.Input, r.Cached, r.Write, r.Output} {
			if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("invalid pricing")
			}
		}
	}
	summaries := map[string]*ComparisonSummary{}
	for _, r := range runs {
		if _, err := configuration(r.Configuration); err != nil {
			return nil, err
		}
		s := summaries[r.Configuration]
		if s == nil {
			s = &ComparisonSummary{OutcomeNormalizerVersion: aidiagnostics.OutcomeNormalizerVersion, OperationalOutcomes: map[string]int{}}
			summaries[r.Configuration] = s
		}
		s.Attempts++
		category := OperationalOutcome(r.Run)
		s.OperationalOutcomes[category]++
		if Completed(r.Run) {
			s.Completed++
		} else if category == "infrastructure_failure" {
			s.Failed++
		}
		accumulateLeaves(r.Run, s)
		if r.Review != nil {
			if strings.TrimSpace(r.Review.Method) == "" || strings.TrimSpace(r.Review.Reason) == "" {
				return nil, fmt.Errorf("review method and source-grounded reason required")
			}
			if r.Review.Accepted && !Completed(r.Run) {
				return nil, fmt.Errorf("unfinished workflow cannot be accepted")
			}
			s.Reviewed++
			if r.Review.Accepted {
				s.Accepted++
			}
		}
		for _, call := range dispatchCalls(r.Run) {
			s.Calls++
			s.InputTokens.add(call.InputTokens)
			s.OutputTokens.add(call.OutputTokens)
			s.ReasoningTokens.add(call.ReasoningTokens)
			s.CachedTokens.add(call.CachedTokens)
			s.WriteTokens.add(call.CacheWriteTokens)
			cost, ok := priceCall(call, rates)
			if !ok {
				s.Unpriced++
				continue
			}
			if s.KnownUSD == nil {
				s.KnownUSD = new(float64)
			}
			*s.KnownUSD += cost
		}
	}
	for _, s := range summaries {
		if s.Calls > 0 && s.Unpriced == 0 {
			s.TotalUSD = s.KnownUSD
			if s.Completed > 0 {
				v := *s.TotalUSD / float64(s.Completed)
				s.PerCompletedUSD = &v
			}
			if s.Accepted > 0 {
				v := *s.TotalUSD / float64(s.Accepted)
				s.PerAcceptedUSD = &v
			}
		}
	}
	return summaries, nil
}

// OperationalOutcome applies the shared terminal interpretation to a logical
// result and its observed required children; it does not infer quality.
func OperationalOutcome(r RunResult) string {
	if r.Diagnostics != nil {
		category := aidiagnostics.OperationalOutcome(r.Diagnostics.Outcome)
		if category != "completed" {
			return category
		}
	}
	for _, step := range r.Steps {
		if category := OperationalOutcome(step); category != "completed" {
			return category
		}
	}
	if r.RequiredSteps > len(r.Steps) {
		return "incomplete"
	}
	// Sequence status can be copied from a child. Prefer its observed category
	// before the legacy status-only fallback, which supplies no quality evidence.
	if r.Status != 200 {
		if r.Status >= 500 {
			return "infrastructure_failure"
		}
		return "unknown"
	}
	return "completed"
}

func accumulateLeaves(r RunResult, s *ComparisonSummary) {
	if r.RequiredSteps > 0 || len(r.Steps) > 0 {
		if r.RequiredSteps == 0 {
			s.SequencesWithUnknownRequiredSteps++
		}
		if r.RequiredSteps > len(r.Steps) {
			s.RequiredUnreached += r.RequiredSteps - len(r.Steps)
		}
		for _, step := range r.Steps {
			accumulateLeaves(step, s)
		}
		return
	}
	s.LeafAttempts++
	if r.Diagnostics != nil {
		s.DurationMS += r.Diagnostics.DurationMS
	}
}

// Sequence Calls is the existing flattened dispatch ledger. Use it once;
// recurse only when a caller supplies a tree without a flattened ledger.
func dispatchCalls(r RunResult) []Call {
	if len(r.Calls) > 0 || len(r.Steps) == 0 {
		return r.Calls
	}
	var calls []Call
	for _, step := range r.Steps {
		calls = append(calls, dispatchCalls(step)...)
	}
	return calls
}
func priceCall(c Call, rates []Rate) (float64, bool) {
	if c.Provider != "api.openai.com" || c.InputTokens == nil || c.OutputTokens == nil || c.CachedTokens == nil || c.CacheWriteTokens == nil {
		return 0, false
	}
	input, output, cached, write := *c.InputTokens, *c.OutputTokens, *c.CachedTokens, *c.CacheWriteTokens
	if input < 0 || output < 0 || cached < 0 || write < 0 || cached > input || write > input-cached {
		return 0, false
	}
	for _, r := range rates {
		if c.ReturnedModel == r.Model {
			return (float64(input-cached-write)*r.Input + float64(cached)*r.Cached + float64(write)*r.Write + float64(output)*r.Output) / 1e6, true
		}
	}
	return 0, false
}

func Completed(r RunResult) bool {
	return OperationalOutcome(r) == "completed"
}

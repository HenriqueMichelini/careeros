package semanticeval

import (
	"fmt"
	"math"
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
	Attempts        int            `json:"attempts"`
	Completed       int            `json:"completed"`
	Failed          int            `json:"failed"`
	Accepted        int            `json:"accepted"`
	Reviewed        int            `json:"reviewed"`
	Calls           int            `json:"providerCalls"`
	Unpriced        int            `json:"unpricedCalls"`
	DurationMS      float64        `json:"workflowDurationMs"`
	KnownUSD        *float64       `json:"knownPricedComponentsUSD"`
	TotalUSD        *float64       `json:"totalEstimatedUSD"`
	PerCompletedUSD *float64       `json:"estimatedPerCompletedUSD"`
	PerAcceptedUSD  *float64       `json:"estimatedPerAcceptedUSD"`
	InputTokens     TokensObserved `json:"inputTokens"`
	OutputTokens    TokensObserved `json:"outputTokens"`
	ReasoningTokens TokensObserved `json:"reasoningTokensSubsetOfOutput"`
	CachedTokens    TokensObserved `json:"cachedTokensSubsetOfInput"`
	WriteTokens     TokensObserved `json:"cacheWriteTokensSubsetOfInput"`
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
			s = &ComparisonSummary{}
			summaries[r.Configuration] = s
		}
		s.Attempts++
		if Completed(r.Run) {
			s.Completed++
		} else {
			s.Failed++
		}
		if r.Run.Diagnostics != nil {
			s.DurationMS += r.Run.Diagnostics.DurationMS
		}
		for _, step := range r.Run.Steps {
			if step.Diagnostics != nil {
				s.DurationMS += step.Diagnostics.DurationMS
			}
		}
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
		for _, call := range r.Run.Calls {
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
	if r.Status != 200 || r.Diagnostics != nil && (r.Diagnostics.Outcome != "completed" && r.Diagnostics.Outcome != "complete") {
		return false
	}
	for _, step := range r.Steps {
		if !Completed(step) {
			return false
		}
	}
	return true
}

package aidiagnostics

import (
	"errors"
	"math"
	"time"
)

// Observed totals sum available components only. Unavailable counts requests,
// not tokens. A wholly unobserved component has a null sum, never zero.
type Tokens struct {
	Observed    *int64 `json:"observed"`
	Available   int    `json:"availableRequests"`
	Unavailable int    `json:"unavailableRequests"`
}

func (t *Tokens) add(n *int64) {
	if n == nil {
		t.Unavailable++
		return
	}
	t.Available++
	if t.Observed == nil {
		t.Observed = new(int64)
	}
	*t.Observed += *n
}

type Price struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	AsOf             string  `json:"asOf"`
	InputPerMillion  float64 `json:"inputPerMillionUSD"`
	CachedPerMillion float64 `json:"cachedPerMillionUSD"`
	OutputPerMillion float64 `json:"outputPerMillionUSD"`
}
type StageSummary struct {
	Runs               int            `json:"runs"`
	Requests           int            `json:"requests"`
	DurationMS         float64        `json:"totalDurationMs"`
	ProviderDurationMS float64        `json:"totalProviderDurationMs"`
	Input              Tokens         `json:"inputTokens"`
	Output             Tokens         `json:"outputTokens"`
	Reasoning          Tokens         `json:"reasoningTokensSubsetOfOutput"`
	Cached             Tokens         `json:"prefixCachedTokensSubsetOfInput"`
	Outcomes           map[string]int `json:"outcomes"`
}
type Summary struct {
	Stages                   map[string]*StageSummary `json:"stagesByVersion"`
	DurationMS               float64                  `json:"totalWorkflowDurationMs"`
	Attempts                 int                      `json:"attempts"`
	Requests                 int                      `json:"requests"`
	Completed                int                      `json:"completed"`
	Failed                   int                      `json:"failed"`
	Accepted                 int                      `json:"accepted"`
	AcceptanceEvidence       int                      `json:"attemptsWithAcceptanceEvidence"`
	CompletionRate           *float64                 `json:"completionRate"`
	FailureRate              *float64                 `json:"failureRate"`
	AcceptanceRate           *float64                 `json:"acceptanceRate"`
	Input                    Tokens                   `json:"inputTokens"`
	Output                   Tokens                   `json:"outputTokens"`
	Reasoning                Tokens                   `json:"reasoningTokensSubsetOfOutput"`
	Cached                   Tokens                   `json:"prefixCachedTokensSubsetOfInput"`
	Outcomes                 map[string]int           `json:"outcomes"`
	UnpricedRequests         int                      `json:"unpricedRequests"`
	EstimatedKnownUSD        *float64                 `json:"estimatedKnownComponentsUSD"`
	EstimatedTotalUSD        *float64                 `json:"estimatedTotalUSD"`
	EstimatedPerCompletedUSD *float64                 `json:"estimatedCostPerCompletedUSD"`
	EstimatedPerAcceptedUSD  *float64                 `json:"estimatedCostPerAcceptedUSD"`
	Billing                  string                   `json:"billing"`
	Prices                   []Price                  `json:"datedPriceAssumptions"`
}

func ratio(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	v := float64(n) / float64(d)
	return &v
}
func Summarize(attempts []Attempt, prices []Price) (Summary, error) {
	s := Summary{Stages: map[string]*StageSummary{}, Attempts: len(attempts), Outcomes: map[string]int{}, Billing: "unavailable: no invoice evidence; estimates exclude unobserved/provider-specific billing components", Prices: prices}
	for _, p := range prices {
		if _, err := time.Parse("2006-01-02", p.AsOf); err != nil {
			return s, errors.New("pricing requires an ISO date")
		}
		for _, v := range []float64{p.InputPerMillion, p.CachedPerMillion, p.OutputPerMillion} {
			if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				return s, errors.New("invalid price")
			}
		}
	}
	known := 0.0
	priced := 0
	for _, a := range attempts {
		s.DurationMS += a.DurationMS
		s.Outcomes[a.Outcome]++
		if a.Outcome == "completed" {
			s.Completed++
		} else {
			s.Failed++
		}
		if a.Accepted != nil {
			s.AcceptanceEvidence++
			if *a.Accepted {
				s.Accepted++
			}
		}
		for _, stage := range a.Stages {
			group := stage.Name + "/" + stage.Version
			g := s.Stages[group]
			if g == nil {
				g = &StageSummary{Outcomes: map[string]int{}}
				s.Stages[group] = g
			}
			g.Runs++
			g.DurationMS += stage.DurationMS
			g.Outcomes[stage.Outcome]++
			p := stage.Provider
			if p == nil {
				continue
			}
			s.Requests++
			g.Requests++
			g.ProviderDurationMS += p.DurationMS
			g.Input.add(p.InputTokens)
			g.Output.add(p.OutputTokens)
			g.Reasoning.add(p.ReasoningTokens)
			g.Cached.add(p.CachedTokens)
			s.Input.add(p.InputTokens)
			s.Output.add(p.OutputTokens)
			s.Reasoning.add(p.ReasoningTokens)
			s.Cached.add(p.CachedTokens)
			matched := false
			for _, price := range prices {
				if price.Provider != p.Name || p.Model == nil || price.Model != *p.Model {
					continue
				}
				if p.InputTokens == nil || p.OutputTokens == nil || p.CachedTokens == nil || *p.CachedTokens > *p.InputTokens {
					break
				}
				known += (float64(*p.InputTokens-*p.CachedTokens)*price.InputPerMillion + float64(*p.CachedTokens)*price.CachedPerMillion + float64(*p.OutputTokens)*price.OutputPerMillion) / 1e6
				matched = true
				priced++
				break
			}
			if !matched {
				s.UnpricedRequests++
			}
		}
	}
	s.CompletionRate = ratio(s.Completed, s.Attempts)
	s.FailureRate = ratio(s.Failed, s.Attempts)
	s.AcceptanceRate = ratio(s.Accepted, s.AcceptanceEvidence)
	if priced > 0 {
		s.EstimatedKnownUSD = &known
	}
	if s.Requests > 0 && s.UnpricedRequests == 0 {
		s.EstimatedTotalUSD = &known
		if s.Completed > 0 {
			v := known / float64(s.Completed)
			s.EstimatedPerCompletedUSD = &v
		}
		if s.Accepted > 0 {
			v := known / float64(s.Accepted)
			s.EstimatedPerAcceptedUSD = &v
		}
	}
	return s, nil
}

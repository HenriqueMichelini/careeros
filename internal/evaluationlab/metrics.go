package evaluationlab

import (
	"encoding/json"
	"professional-information-repo/internal/semanticeval"
)

type MetricDefinition struct {
	ID         string `json:"id"`
	Version    string `json:"version"`
	Name       string `json:"name"`
	Definition string `json:"definition"`
	Source     string `json:"source"`
}

type Metric struct {
	Split             string   `json:"split,omitempty"`
	ExposureGroup     string   `json:"exposureGroup,omitempty"`
	FailureFamilies   []string `json:"failureFamilies,omitempty"`
	Scope             string   `json:"scope,omitempty"`
	LabelHash         string   `json:"labelHash,omitempty"`
	BudgetExcluded    *int     `json:"budgetExcluded,omitempty"`
	RelevanceExcluded *int     `json:"relevanceExcluded,omitempty"`
	Configuration     string   `json:"configuration,omitempty"`
	BudgetBytes       *int     `json:"budgetBytes,omitempty"`
	ObservationCount  *int     `json:"n,omitempty"`
	ID                string   `json:"id"`
	Version           string   `json:"version"`
	Measure           string   `json:"measure"`
	Workflow          string   `json:"workflow"`
	CaseID            string   `json:"caseId"`
	Field             string   `json:"field"`
	Language          string   `json:"language"`
	Mode              string   `json:"mode"`
	Source            string   `json:"source"`
	Numerator         *int     `json:"numerator"`
	Denominator       *int     `json:"denominator"`
	Eligible          int      `json:"eligible"`
	Evaluated         int      `json:"evaluated"`
	Missing           int      `json:"missing"`
	Units             string   `json:"units"`
	Estimator         string   `json:"estimator"`
	Value             *float64 `json:"value"`
	UnavailableReason string   `json:"unavailableReason,omitempty"`
}

func MetricRegistry() []MetricDefinition { return append([]MetricDefinition{}, metricDefinitions...) }

// CountMetric preserves a zero denominator as zero opportunities, never 100%.
func CountMetric(id, measure string, numerator, denominator int, mode, source string) Metric {
	m := Metric{ID: id, Version: "lab-metrics-v1", Measure: measure, Mode: mode, Source: source, Numerator: &numerator, Denominator: &denominator, Eligible: denominator, Evaluated: denominator, Units: "ratio", Estimator: "finite labeled opportunity count"}
	if denominator < 0 || numerator < 0 || numerator > denominator {
		m.UnavailableReason = "inconsistent count evidence"
		m.Missing = denominator
		m.Evaluated = 0
		return m
	}
	if denominator == 0 {
		m.UnavailableReason = "not applicable: zero opportunities"
		return m
	}
	value := float64(numerator) / float64(denominator)
	m.Value = &value
	return m
}

// Metrics exposes unsupported capabilities rather than inventing a score.
func Metrics(imported ImportResult) []Metric {
	// Revalidate the original bytes: caller-supplied normalized fields cannot
	// turn quarantined or modified data into trusted measurements.
	canonical, err := revalidateImport(imported)
	if err == nil {
		imported = canonical
	} else {
		imported.Quarantined = true
	}
	metrics := make([]Metric, 0, len(metricDefinitions))
	for _, definition := range metricDefinitions {
		reason := "full metric contract unavailable: separately named observed measures may exist; remaining opportunities or reviewed labels are missing"
		if imported.Quarantined {
			reason = "quarantined evidence cannot supply trusted metrics"
		}
		metrics = append(metrics, Metric{ID: definition.ID, Version: definition.Version, Measure: definition.Name, Mode: imported.OriginMode, Source: imported.OriginalHash, Units: "unavailable", Estimator: "unavailable", UnavailableReason: reason})
	}
	if !imported.Quarantined {
		metrics = append(metrics, retrievalMetrics(imported)...)
		metrics = append(metrics, workflowMetrics(imported)...)
		metrics = append(metrics, dispatchMetrics(imported)...)
		metrics = append(metrics, matchingMetrics(imported)...)
	}
	return metrics
}

func dispatchMetrics(imported ImportResult) []Metric {
	ledger := DispatchLedger(imported)
	if len(ledger) == 0 {
		return nil
	}
	var document struct {
		Rates []semanticeval.Rate `json:"datedPriceAssumptions"`
	}
	if json.Unmarshal(imported.Original, &document) != nil {
		return nil
	}
	summary, err := SummarizeDispatches(ledger, document.Rates)
	if err != nil {
		return nil
	}
	base := func(id, measure, units string) Metric {
		return Metric{ID: id, Version: "lab-metrics-v1", Measure: measure, Mode: imported.OriginMode, Source: imported.OriginalHash, Configuration: "all retained report configurations", Eligible: summary.Calls, Units: units, Estimator: "unique dispatch ledger in this report; evidence origins remain separate"}
	}
	m := base("M18", "dispatch-count", "calls")
	n := summary.Calls
	v := float64(n)
	m.ObservationCount = &n
	m.Value = &v
	m.Evaluated = n
	metrics := []Metric{m}
	cacheHits := 0
	cacheOpportunities := 0
	for _, dispatch := range ledger {
		call := dispatch.Call
		if call.CachedTokens != nil && call.InputTokens != nil && *call.CachedTokens >= 0 && *call.InputTokens >= *call.CachedTokens {
			cacheOpportunities++
			if *call.CachedTokens > 0 {
				cacheHits++
			}
		}
		latency := base("M22", "dispatch-duration", "ms")
		latency.CaseID = dispatch.CaseIdentity
		one := 1
		latency.ObservationCount = &one
		latency.Eligible = 1
		latency.Evaluated = 1
		latency.Estimator = "single retained dispatch-through-consumed-response observation; not a population percentile or workflow wall time"
		if call.DurationMS >= 0 {
			value := call.DurationMS
			latency.Value = &value
		} else {
			latency.UnavailableReason = "invalid duration"
			latency.Evaluated = 0
			latency.Missing = 1
		}
		metrics = append(metrics, latency)
	}
	cache := CountMetric("M21", "observed-cache-hit-rate", cacheHits, cacheOpportunities, imported.OriginMode, imported.OriginalHash)
	cache.Estimator = "positive cache reads among calls with valid paired cache-read/input evidence"
	metrics = append(metrics, cache)
	for _, component := range []struct {
		name   string
		tokens semanticeval.TokensObserved
	}{
		{"input-tokens", summary.Input}, {"output-tokens", summary.Output}, {"reasoning-tokens-subset-of-output", summary.Reasoning}, {"cached-tokens-subset-of-input", summary.Cached}, {"cache-write-tokens-subset-of-input", summary.CacheWrite},
	} {
		m := base("M21", component.name, "tokens")
		available := summary.Calls - component.tokens.Missing
		m.ObservationCount = &available
		m.Evaluated = available
		m.Missing = component.tokens.Missing
		if component.tokens.Total != nil {
			v := float64(*component.tokens.Total)
			m.Value = &v
		} else {
			m.UnavailableReason = "no valid observed token component"
		}
		metrics = append(metrics, m)
	}
	for _, cost := range []struct {
		name  string
		value *float64
	}{{"known-priced-subtotal", summary.KnownUSD}, {"full-cohort-cost", summary.TotalUSD}} {
		m := base("M23", cost.name, "USD")
		priced := summary.Calls - summary.Unpriced
		m.ObservationCount = &priced
		m.Evaluated = priced
		m.Missing = summary.Unpriced
		m.Value = cost.value
		m.Estimator = "dated exact-model assumptions in original report; excludes unknown/unpriced calls; not invoice or accepted-result cost"
		if m.Value == nil {
			m.UnavailableReason = "dated price or required usage missing for some or all calls"
		}
		metrics = append(metrics, m)
	}
	return metrics
}

func retrievalMetrics(imported ImportResult) []Metric {
	var provenance struct {
		FixtureHash   string            `json:"fixtureHash"`
		FixtureHashes map[string]string `json:"fixtureHashes"`
	}
	if json.Unmarshal(imported.Original, &provenance) != nil {
		return nil
	}
	if provenance.FixtureHash != retrievalLabelHash && provenance.FixtureHashes["retrieval"] != retrievalLabelHash {
		return nil
	}
	var metrics []Metric
	for _, c := range imported.Cases {
		if c.Kind != "candidate-retrieval" && c.Kind != "candidate-recall" {
			continue
		}
		ids, ok := retrievalRelevantIDs[c.ID]
		if !ok {
			continue
		}
		var row struct {
			Relevant       int      `json:"relevant"`
			Retrieved      int      `json:"retrieved"`
			Misses         []string `json:"materialMisses"`
			Budget         int      `json:"budgetBytes"`
			NegativeMisses []string `json:"negativeMisses"`
			Selection      struct {
				BudgetExcluded    *int `json:"BudgetExcluded"`
				RelevanceExcluded *int `json:"RelevanceExcluded"`
			} `json:"selection"`
		}
		if json.Unmarshal(c.Actual, &row) != nil {
			continue
		}
		missing := map[string]bool{}
		for _, id := range row.Misses {
			missing[id] = true
		}
		kept := 0
		for _, id := range ids {
			if !missing[id] {
				kept++
			}
		}
		m := CountMetric("M08", "post-budget-relevant-retention", kept, len(ids), imported.OriginMode, imported.OriginalHash)
		m.CaseID = c.ID
		if row.Budget > 0 {
			budget := row.Budget
			m.BudgetBytes = &budget
		}
		m.Workflow = c.Workflow
		m.Language = c.Language
		m.Language = retrievalLanguages[c.ID]
		m.LabelHash = retrievalLabelHash
		m.Scope = "requirement-facts"
		if c.Kind == "candidate-recall" {
			m.Scope = "application-facts"
		}
		m.BudgetExcluded = row.Selection.BudgetExcluded
		m.RelevanceExcluded = row.Selection.RelevanceExcluded
		m.Estimator = "unique labeled fact IDs per case and byte budget; not Recall@K"
		if row.Relevant != len(ids) || row.Retrieved != kept || len(missing) > len(ids) {
			m.Value = nil
			m.UnavailableReason = "report counts disagree with unique labeled IDs"
		}
		metrics = append(metrics, m)
		negatives := retrievalNegativeIDs[c.ID]
		negativeMissing := map[string]bool{}
		for _, id := range row.NegativeMisses {
			negativeMissing[id] = true
		}
		negativeKept := 0
		for _, id := range negatives {
			if !negativeMissing[id] {
				negativeKept++
			}
		}
		n := CountMetric("M08", "post-budget-negative-retention", negativeKept, len(negatives), imported.OriginMode, imported.OriginalHash)
		n.CaseID = m.CaseID
		n.Workflow = m.Workflow
		n.Language = m.Language
		n.Scope = m.Scope
		n.LabelHash = m.LabelHash
		n.BudgetBytes = m.BudgetBytes
		n.BudgetExcluded = m.BudgetExcluded
		n.RelevanceExcluded = m.RelevanceExcluded
		n.Estimator = "unique labeled negative fact IDs per case and byte budget; not ranked retrieval"
		metrics = append(metrics, n)
	}
	return metrics
}

func workflowMetrics(imported ImportResult) []Metric {
	var result []Metric
	logicalCases := 0
	httpLeaves := 0
	var visit func(ImportedCase)
	visit = func(c ImportedCase) {
		if c.Kind != "workflow-result" && c.Kind != "field-result" && c.Kind != "diagnostic-attempt" {
			return
		}
		var observed struct {
			Status  int                            `json:"status"`
			Facets  map[string]semanticeval.Result `json:"labelFacetScores"`
			Score   *semanticeval.Result           `json:"score"`
			Current *semanticeval.Result           `json:"currentControlledReplay"`
			Field   string                         `json:"field"`
		}
		if json.Unmarshal(c.Actual, &observed) != nil {
			return
		}
		score := observed.Score
		if score == nil {
			score = observed.Current
		}
		if c.Workflow == "profile_review" {
			if facet, ok := observed.Facets["patches"]; ok {
				score = &facet
			}
			if observed.Field == "" {
				var scope struct {
					Request struct {
						Section string `json:"section"`
					} `json:"request"`
				}
				_ = json.Unmarshal(c.Source, &scope)
				observed.Field = scope.Request.Section
			}
		}
		if score != nil {
			for _, measure := range []struct {
				id, name string
				n, d     int
			}{
				{"M03", "curated-label-precision", score.Matched, score.Observed},
				{"M03", "curated-label-completeness", score.Covered, score.Expected},
				{"M05", "curated-label-omission", len(score.Missing), score.Expected},
			} {
				m := CountMetric(measure.id, measure.name, measure.n, measure.d, imported.OriginMode, imported.OriginalHash)
				m.CaseID = c.ID
				m.Workflow = c.Workflow
				m.Field = observed.Field
				m.Language = c.Language
				m.Configuration = c.Configuration
				m.Split = c.Split
				m.ExposureGroup = c.ExposureGroup
				m.FailureFamilies = append([]string{}, c.FailureFamilies...)
				m.Estimator = "inference-era curated label counts; not semantic correctness or user acceptance"
				var run semanticeval.RunResult
				_ = json.Unmarshal(c.Actual, &run)
				available := observed.Status == 200
				if c.Kind == "workflow-result" {
					available = semanticeval.Completed(run)
				}
				if !available {
					m.Value = nil
					m.UnavailableReason = "failed, incomplete or unreached output cannot receive a passing score"
					m.Missing = m.Eligible
					m.Evaluated = 0
				}
				result = append(result, m)
			}
		}
		if len(c.Children) > 0 {
			for _, child := range c.Children {
				visit(child)
			}
			return
		}
		if c.Kind == "workflow-result" || c.Kind == "diagnostic-attempt" {
			httpLeaves++
			m := CountMetric("M17", "terminal-category:"+c.OperationalOutcome, 1, 1, imported.OriginMode, imported.OriginalHash)
			m.CaseID = c.ID
			m.Workflow = c.Workflow
			m.Language = c.Language
			m.Configuration = c.Configuration
			m.Estimator = "observed leaf HTTP attempt category; unknown remains unknown"
			result = append(result, m)
		}
	}
	for _, c := range imported.Cases {
		if c.Kind == "workflow-result" {
			logicalCases++
		}
		visit(c)
	}
	for _, count := range []struct {
		name  string
		value int
	}{{"logical-case-count", logicalCases}, {"http-leaf-attempt-count", httpLeaves}} {
		if count.value == 0 {
			continue
		}
		n := count.value
		v := float64(n)
		result = append(result, Metric{ID: "M18", Version: "lab-metrics-v1", Measure: count.name, Mode: imported.OriginMode, Source: imported.OriginalHash, ObservationCount: &n, Eligible: n, Evaluated: n, Units: "count", Estimator: "observed logical roots or HTTP leaves; translated/configuration pairs are dependent", Value: &v})
	}
	return result
}

func matchingMetrics(imported ImportResult) []Metric {
	var metrics []Metric
	for _, c := range imported.Cases {
		if c.Kind != "controlled-final-matching" {
			continue
		}
		var row struct {
			Status   int    `json:"status"`
			Expected string `json:"expected"`
			Actual   string `json:"actual"`
		}
		if json.Unmarshal(c.Actual, &row) != nil {
			continue
		}
		correct := 0
		if row.Expected == row.Actual {
			correct = 1
		}
		m := CountMetric("M07", "controlled-match-state-agreement", correct, 1, imported.OriginMode, imported.OriginalHash)
		m.CaseID = c.ID
		m.Workflow = c.Workflow
		m.Language = retrievalLanguages[c.ID]
		m.Scope = "requirement-facts"
		m.LabelHash = retrievalLabelHash
		m.Estimator = "expected versus controlled replay state; not live model semantic accuracy"
		if row.Status != 200 || row.Expected == "" || row.Actual == "" {
			m.Value = nil
			m.UnavailableReason = "controlled match assessment unavailable"
			m.Missing = 1
			m.Evaluated = 0
		}
		metrics = append(metrics, m)
	}
	return metrics
}

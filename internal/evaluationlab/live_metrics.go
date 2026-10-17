package evaluationlab

import (
	"professional-information-repo/internal/semanticeval"
	"sort"
)

func guardMetrics(record guardRecord, usage UsageSummary, logical, httpLegs int) []Metric {
	metrics := []Metric{}
	count := func(id, measure string, value int) {
		v := float64(value)
		n := value
		metrics = append(metrics, Metric{ID: id, Version: "lab-metrics-v1", Measure: measure, Mode: "controlled-guard", Source: record.Preflight.Hash, ObservationCount: &n, Eligible: value, Evaluated: value, Value: &v, Units: "count", Estimator: "finite durable guard admissions; simulated provider legs, not actual provider/network requests or semantic results"})
	}
	count("M18", "controlled-guard-provider-calls", len(record.Calls))
	count("M18", "controlled-guard-logical-cases", logical)
	count("M18", "controlled-guard-workflow-http-legs", httpLegs)
	outcomes := map[string]int{}
	for _, call := range record.Calls {
		outcomes[call.State]++
		metric := Metric{ID: "M22", Version: "lab-metrics-v1", Measure: "controlled-guard-callback-duration", CaseID: call.Spec.CaseID, Mode: "controlled-guard", Source: record.Preflight.Hash, Eligible: 1, Missing: 1, Units: "ms", Estimator: "one guard callback through returned response; not provider latency, full workflow time or a percentile"}
		if call.FinishedAt != "" {
			value := call.Call.DurationMS
			one := 1
			metric.Value = &value
			metric.ObservationCount = &one
			metric.Evaluated = 1
			metric.Missing = 0
		} else {
			metric.UnavailableReason = "Admitted intent has no retained final callback observation."
		}
		metrics = append(metrics, metric)
	}
	keys := []string{}
	for key := range outcomes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, outcome := range keys {
		count("M17", "controlled-guard-terminal/"+outcome, outcomes[outcome])
		if outcome != "completed" {
			count("M19", "controlled-guard-failure/"+outcome, outcomes[outcome])
		}
	}
	for _, component := range []struct {
		name   string
		tokens semanticeval.TokensObserved
	}{{"input", usage.Input}, {"output", usage.Output}, {"reasoning-subset", usage.Reasoning}, {"cache-read-subset", usage.Cached}, {"cache-write-subset", usage.CacheWrite}} {
		metric := Metric{ID: "M21", Version: "lab-metrics-v1", Measure: "controlled-guard-" + component.name, Mode: "controlled-guard", Source: record.Preflight.Hash, Eligible: usage.Calls, Evaluated: usage.Calls - component.tokens.Missing, Missing: component.tokens.Missing, Units: "tokens", Estimator: "sum of explicitly simulated usage observations; absent components stay unavailable"}
		if component.tokens.Total != nil {
			value := float64(*component.tokens.Total)
			metric.Value = &value
		} else {
			metric.UnavailableReason = "No valid controlled component observations."
		}
		metrics = append(metrics, metric)
	}
	metrics = append(metrics, Metric{ID: "M23", Version: "lab-metrics-v1", Measure: "controlled-guard-known-priced-subtotal", Mode: "controlled-guard", Source: record.Preflight.Hash, Value: usage.KnownUSD, Eligible: usage.Calls, Evaluated: usage.Calls - usage.Unpriced, Missing: usage.Unpriced, Units: "USD", Estimator: "dated exact-model simulated usage estimate; not a provider charge"})
	cost := Metric{ID: "M23", Version: "lab-metrics-v1", Measure: "controlled-guard-full-estimate", Mode: "controlled-guard", Source: record.Preflight.Hash, Value: usage.TotalUSD, Eligible: usage.Calls, Evaluated: usage.Calls - usage.Unpriced, Missing: usage.Unpriced, Units: "USD", Estimator: "full total only when every controlled call has valid paired usage and exact dated prices"}
	if usage.TotalUSD == nil {
		cost.UnavailableReason = "Full controlled total unavailable: unknown finality or incomplete usable exact prices/usage."
	}
	metrics = append(metrics, cost)
	for _, missing := range []struct{ id, reason string }{
		{"M10", "The guard observes no independently adjudicated statement-assessor population."},
		{"M11", "False acceptance/rejection and abstention require adjudicated labels and assessor predictions."},
		{"M12", "Guard operational failures are not semantic uncertainty or assessment unavailability populations."},
		{"M20", "No reusable semantic analysis or saved workflow-call baseline is observed."},
		{"M23", "Real provider charges and accepted-result cost remain unavailable; no real experiment occurred."},
		{"M28", "Runtime guard metadata is not an external network/filesystem monitor. Finite sentinel and network-denial verification is retained separately; no universal privacy claim."},
	} {
		metrics = append(metrics, Metric{ID: missing.id, Version: "lab-metrics-v1", Measure: "controlled-guard-unavailable", Mode: "controlled-guard", Source: record.Preflight.Hash, Units: "unavailable", Estimator: "unavailable", UnavailableReason: missing.reason})
	}
	return metrics
}

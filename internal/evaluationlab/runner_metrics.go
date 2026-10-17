package evaluationlab

import "encoding/json"

// Native derivations operate on this runner's owned controlled results. They do
// not normalize arbitrary uploads or grant imported artifacts a trusted origin.
func nativeMetrics(record RunRecord, plan Plan) []Metric {
	metrics := []Metric{}
	for _, count := range []struct {
		name  string
		value int
	}{
		{"planned-logical-cases", record.Progress.PlannedCases}, {"started-logical-cases", record.Progress.StartedCases}, {"finished-logical-cases", record.Progress.FinishedCases},
		{"http-leaf-attempt-count", record.Progress.AttemptedHTTP}, {"unreached-http-leg-count", record.Progress.UnreachedHTTP}, {"simulated-dispatch-count", record.Progress.AttemptedDispatches},
		{"planned-process-attempt-count", record.Progress.PlannedProcesses}, {"attempted-process-count", record.Progress.AttemptedProcesses}, {"finished-process-count", record.Progress.FinishedProcesses}, {"unreached-process-count", record.Progress.UnreachedProcesses},
	} {
		n := count.value
		value := float64(n)
		metrics = append(metrics, Metric{ID: "M18", Version: "lab-metrics-v1", Measure: count.name, Mode: record.Context.Mode, Source: plan.Hash, ObservationCount: &n, Eligible: n, Evaluated: n, Units: "count", Estimator: "frozen-plan progress and observed controlled dispatch ledger", Value: &value})
	}
	if plan.Selection.Suite == "test-adapters" {
		metrics = append(metrics, processMetrics(record, plan)...)
	} else if plan.Selection.Suite == "section-review" {
		metrics = append(metrics, sectionMetrics(record, plan)...)
	} else if plan.Selection.Suite == "semantic-quality" || plan.Selection.Suite == "workflow-settings" {
		for _, m := range workflowMetrics(ImportResult{OriginMode: "controlled", OriginalHash: plan.Hash, Cases: record.Cases}) {
			if m.ID != "M18" {
				metrics = append(metrics, m)
			}
		}
	} else {
		for _, root := range record.Cases {
			if len(root.Actual) == 0 {
				continue
			}
			var report struct {
				Retrieval []json.RawMessage `json:"candidateRetrieval"`
				Final     []json.RawMessage `json:"controlledFinalMatching"`
				Recall    []json.RawMessage `json:"candidateRecall"`
				Fields    []json.RawMessage `json:"fieldResults"`
			}
			if json.Unmarshal(root.Actual, &report) != nil {
				continue
			}
			normalized := ImportResult{OriginMode: "controlled-offline", OriginalHash: root.ArtifactHash, Original: root.Actual}
			for _, group := range []struct {
				kind string
				rows []json.RawMessage
			}{{"candidate-retrieval", report.Retrieval}, {"controlled-final-matching", report.Final}, {"candidate-recall", report.Recall}, {"field-result", report.Fields}} {
				for _, raw := range group.rows {
					var identity struct {
						ID string `json:"id"`
					}
					_ = json.Unmarshal(raw, &identity)
					normalized.Cases = append(normalized.Cases, ImportedCase{ID: identity.ID, Kind: group.kind, Workflow: root.Workflow, Language: root.Language, Actual: raw})
				}
			}
			metrics = append(metrics, retrievalMetrics(normalized)...)
			metrics = append(metrics, matchingMetrics(normalized)...)
			if len(report.Fields) > 0 {
				metrics = append(metrics, workflowMetrics(normalized)...)
			}
		}
	}
	if usage, err := SummarizeDispatches(record.Dispatches, nil); err == nil {
		for _, component := range []struct {
			name     string
			missing  int
			observed *int64
		}{{"input-tokens", usage.Input.Missing, usage.Input.Total}, {"output-tokens", usage.Output.Missing, usage.Output.Total}, {"cached-tokens", usage.Cached.Missing, usage.Cached.Total}, {"reasoning-tokens-subset-of-output", usage.Reasoning.Missing, usage.Reasoning.Total}, {"cache-write-tokens-subset-of-input", usage.CacheWrite.Missing, usage.CacheWrite.Total}} {
			n := usage.Calls - component.missing
			m := Metric{ID: "M21", Version: "lab-metrics-v1", Measure: component.name, Mode: record.Context.Mode, Source: plan.Hash, ObservationCount: &n, Eligible: usage.Calls, Evaluated: n, Missing: component.missing, Units: "tokens", Estimator: "controlled simulation; no provider-observed usage"}
			if component.observed != nil {
				value := float64(*component.observed)
				m.Value = &value
			} else {
				m.UnavailableReason = "simulated response has no observed token component"
			}
			metrics = append(metrics, m)
		}
	}
	metrics = append(metrics, Metric{ID: "M23", Version: "lab-metrics-v1", Measure: "full-cohort-cost", Mode: record.Context.Mode, Source: plan.Hash, Units: "USD", Estimator: "unavailable", UnavailableReason: "controlled simulation is not provider usage, billing or named acceptance evidence"})
	for _, dispatch := range record.Dispatches {
		if dispatch.State == "planned" || dispatch.State == "not_dispatched" {
			continue
		}
		one := 1
		duration := dispatch.Call.DurationMS
		metrics = append(metrics, Metric{ID: "M22", Version: "lab-metrics-v1", Measure: "controlled-dispatch-duration", CaseID: dispatch.CaseIdentity, Mode: record.Context.Mode, Source: plan.Hash, ObservationCount: &one, Eligible: 1, Evaluated: 1, Units: "ms", Estimator: "one local controlled transport observation; not provider speed or a population percentile", Value: &duration})
	}
	for _, missing := range []struct{ id, name, reason string }{{"M19", "stage-causal-failure-rate", "terminal traces are retained; a stage-specific causal denominator is not established"}, {"M20", "client-reuse-effectiveness", "client reuse opportunities are not exercised by this handler-only suite"}} {
		metrics = append(metrics, Metric{ID: missing.id, Version: "lab-metrics-v1", Measure: missing.name, Mode: record.Context.Mode, Source: plan.Hash, Units: "unavailable", Estimator: "unavailable", UnavailableReason: missing.reason})
	}
	return metrics
}

package evaluationlab

import "encoding/json"

func processMetrics(record RunRecord, plan Plan) []Metric {
	metrics := []Metric{}
	for _, c := range record.Cases {
		var result ProcessResult
		if json.Unmarshal(c.Actual, &result) != nil {
			continue
		}
		for _, id := range []string{"M02", "M06", "M09", "M16", "M17", "M19", "M20", "M24", "M25", "M26", "M27", "M28", "M29"} {
			metrics = append(metrics, Metric{ID: id, Version: "lab-metrics-v1", Measure: "nested-test-opportunities-unavailable", CaseID: c.ID, Workflow: c.Workflow, Language: c.Language, Mode: record.Selection.Mode, Source: c.ArtifactHash, Units: "unavailable", Estimator: "unavailable", UnavailableReason: "A fixed test-process result is not an extracted domain assertion population, semantic accuracy, user acceptance or native-export observation."})
		}
		if result.Outcome == "passed" || result.Outcome == "assertion_failed" {
			passed := 0
			if result.Outcome == "passed" {
				passed = 1
			}
			m := CountMetric("M17", "fixed-test-process-exit-assertion", passed, 1, record.Selection.Mode, c.ArtifactHash)
			m.CaseID = c.ID
			m.Workflow = c.Workflow
			m.Language = c.Language
			m.Scope = "one allowlisted test command exit; nested assertions are not counted"
			metrics = append(metrics, m)
		}
		if c.ID == "probe-isolation" && len(result.Evidence) == 4 {
			bad := 0
			valid := true
			for _, key := range []string{"credentialEnvAbsent", "hostFileAbsent", "hostNetworkUnreachable", "privateHome"} {
				value, present := result.Evidence[key]
				if !present {
					valid = false
				}
				if !value {
					bad++
				}
			}
			if valid {
				m := CountMetric("M28", "injected-process-isolation-violations", bad, 4, record.Selection.Mode, c.ArtifactHash)
				m.CaseID = c.ID
				m.Workflow = c.Workflow
				m.Language = c.Language
				m.Scope = "four injected environment/home/host-file/host-loopback sentinels"
				m.Estimator = "finite local guard opportunities; no universal privacy/security claim"
				metrics = append(metrics, m)
			}
		}
		if result.SpawnAttempted {
			one := 1
			duration := result.DurationMS
			metrics = append(metrics, Metric{ID: "M22", Version: "lab-metrics-v1", Measure: "isolated-process-duration", CaseID: c.ID, Workflow: c.Workflow, Language: c.Language, Mode: record.Selection.Mode, Source: c.ArtifactHash, ObservationCount: &one, Eligible: 1, Evaluated: 1, Value: &duration, Units: "ms", Estimator: "one local process observation through supervisor wait; not provider latency, browser interaction time or a percentile"})
		}
	}
	return metrics
}

package evaluationlab

import (
	"encoding/json"
	"professional-information-repo/internal/semanticeval"
	"strings"
)

// Material labels and scripted privacy opportunities have finite denominators.
// Independent semantic/judge/user decisions stay unavailable while unreviewed.
func sectionMetrics(record RunRecord, plan Plan) []Metric {
	normalized := ImportResult{OriginMode: "controlled", OriginalHash: plan.Hash}
	metrics := []Metric{}
	for _, c := range record.Cases {
		var run semanticeval.RunResult
		_ = json.Unmarshal(c.Actual, &run)
		var source struct {
			Request struct {
				Section string `json:"section"`
			} `json:"request"`
		}
		_ = json.Unmarshal(c.Source, &source)
		var atoms []semanticeval.Atom
		_ = json.Unmarshal(c.Expected, &atoms)
		expected := 0
		missing := []string{}
		for _, atom := range atoms {
			if atom.Group == "patches" && !atom.Optional {
				expected++
				missing = append(missing, atom.ID)
			}
		}
		score, ok := run.LabelFacetScores["patches"]
		if !ok {
			score = semanticeval.Result{Expected: expected, Missing: missing}
		}
		var actual map[string]json.RawMessage
		_ = json.Unmarshal(c.Actual, &actual)
		if actual == nil {
			actual = map[string]json.RawMessage{}
		}
		actual["score"], _ = json.Marshal(score)
		actual["field"], _ = json.Marshal(source.Request.Section)
		derived := c
		derived.Actual, _ = json.Marshal(actual)
		normalized.Cases = append(normalized.Cases, derived)
		leaks := 0
		for _, envelope := range run.RequestEnvelopes {
			if strings.Contains(string(envelope), "never-send-section@example.test") || strings.Contains(string(envelope), "sk-controlled") {
				leaks++
			}
		}
		privacy := CountMetric("M28", "scripted-outbound-private-canary-exposure", leaks, len(run.RequestEnvelopes), "controlled", plan.Hash)
		privacy.CaseID = c.ID
		privacy.Workflow = c.Workflow
		privacy.Field = source.Request.Section
		privacy.Language = c.Language
		privacy.Scope = "captured controlled request envelopes"
		privacy.Estimator = "injected synthetic email/key-in-body canaries; no universal privacy or actual-provider disclosure claim"
		metrics = append(metrics, privacy)
		for _, unavailable := range []struct{ id, reason string }{
			{"M04", "meaning/owner/polarity adjudication remains pending; regex mismatches are candidates only"},
			{"M09", "invalid_output conflates structural/reference and support rejection; causal validation denominator unavailable"},
			{"M10", "no independently adjudicated three-class support labels; scripted booleans are not assessor accuracy"},
			{"M11", "false acceptance/rejection requires independent adjudicated support labels"},
			{"M12", "production support schema is binary; malformed uncertain fixtures are unavailability, not semantic uncertainty"},
			{"M16", "client Apply is not executed by this handler; public client regressions are separate evidence"},
			{"M25", "no human client review decisions or review time were observed"},
		} {
			metrics = append(metrics, Metric{ID: unavailable.id, Version: "lab-metrics-v1", Measure: "section-review-unavailable", CaseID: c.ID, Workflow: c.Workflow, Field: source.Request.Section, Language: c.Language, Mode: "controlled", Source: plan.Hash, Eligible: expected, Missing: expected, Units: "unavailable", Estimator: "unavailable", UnavailableReason: unavailable.reason})
		}
	}
	for _, metric := range workflowMetrics(normalized) {
		if metric.ID == "M18" {
			continue
		}
		metric.Scope = "material patch clauses; optional style summary excluded"
		metric.LabelHash = plan.InputHashes["section-review"]
		metric.Estimator = "finite material-clause label matches; semantic adjudication remains pending"
		metrics = append(metrics, metric)
	}
	return metrics
}

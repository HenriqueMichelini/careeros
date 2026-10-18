package evaluationlab

import (
	"encoding/json"
	"professional-information-repo/internal/semanticeval"
)

// DeriveCVEvidenceMetrics is an offline measurement derivation, not registration
// or adjudication. Trusted Lab import metrics additionally require exact receipt
// registration in Metrics; changed receipts remain quarantined there.
func DeriveCVEvidenceMetrics(raw []byte) ([]Metric, error) {
	cases, err := DeriveCVEvidence(raw)
	if err != nil {
		return nil, err
	}
	return cvIntegrityMetrics(cases, semanticeval.Hash(raw)), nil
}

// CV metrics count exact joined owner observations once per machine unit.
// Scripted enforcement labels remain a separate cohort from adjudicated M15.
func cvIntegrityMetrics(cases []ImportedCase, source string) []Metric {
	metrics := []Metric{}
	for _, c := range cases {
		if c.Kind != "workflow-result" {
			continue
		}
		var observed cvClientObservation
		_ = json.Unmarshal(c.Review, &observed)
		var expected struct {
			Contract struct {
				Judgments map[string]string `json:"judgments"`
			} `json:"contract"`
		}
		_ = json.Unmarshal(c.Expected, &expected)
		var run semanticeval.RunResult
		_ = json.Unmarshal(c.Actual, &run)
		unsupported, accepted, missing, userAssertions := 0, 0, 0, 0
		var snapshot struct {
			Provenance []struct {
				ID, Text, Origin, Decision string
				SourceIDs                  []string `json:"sourceIds"`
			} `json:"provenance"`
		}
		_ = json.Unmarshal(observed.Snapshot, &snapshot)
		for _, p := range snapshot.Provenance {
			if p.Origin == "user_edited" || p.Origin == "user_confirmed" {
				userAssertions++
			}
		}
		for _, unit := range observed.Units {
			if expected.Contract.Judgments[unit.ID] != "unsupported" {
				continue
			}
			unsupported++
			active := false
			for _, p := range snapshot.Provenance {
				if p.ID == unit.ID && p.Origin == "generated" && p.Decision == "keep_generated" && p.Text == unit.Text && equalJSON(p.SourceIDs, unit.SourceIDs) {
					active = true
				}
			}
			if active || observed.AcceptedAsIs != nil && *observed.AcceptedAsIs {
				accepted++
			} else if observed.AcceptedAsIs == nil {
				missing++
			}
		}
		m := CountMetric("M15", "scripted-unsupported-active-machine-acceptance", accepted, unsupported, "controlled", source)
		m.Missing = missing
		m.Evaluated = unsupported - missing
		m.Estimator = "scripted owner expectations at the exact joined client acceptance boundary; finite enforcement only, not adjudicated M15"
		if missing > 0 || observed.ID == "" {
			m.Value = nil
			m.UnavailableReason = "client acceptance observation missing"
			if observed.ID == "" {
				for _, status := range expected.Contract.Judgments {
					if status == "unsupported" {
						m.Eligible++
						m.Missing++
					}
				}
			}
		}
		metrics = append(metrics, m)
		unsafe := CountMetric("M15", "scripted-unsafe-case-acceptance", boolCount(accepted > 0), boolCount(unsupported > 0), "controlled", source)
		unsafe.Estimator = "one unsafe case with any accepted unsupported machine unit; scripted enforcement cohort only"
		if m.Value == nil && unsupported > 0 {
			unsafe.Value = nil
			unsafe.UnavailableReason = m.UnavailableReason
			unsafe.Missing = 1
			unsafe.Evaluated = 0
		}
		metrics = append(metrics, unsafe)
		adjudicated := CountMetric("M15", "adjudicated-unsupported-active-machine-acceptance", 0, 0, "controlled", source)
		adjudicated.Missing = unsupported
		adjudicated.UnavailableReason = "independent human adjudication pending; scripted labels and explicit user assertions excluded"
		metrics = append(metrics, adjudicated)
		for _, item := range []struct {
			id, name string
			value    int
		}{
			{"M18", "retained-simulated-dispatch-count", len(run.Calls)},
			{"M17", "retained-blocked-generation", boolCount(run.Status != 200)},
			{"M17", "retained-unavailable-assessment", cvUnavailableAssessment(run.Output)},
		} {
			n := item.value
			value := float64(n)
			metrics = append(metrics, Metric{ID: item.id, Version: "lab-metrics-v1", Measure: item.name, Mode: "controlled-retained", Source: source, Value: &value, ObservationCount: &n, Eligible: n, Evaluated: n, Units: "count", Estimator: "immutable owner observation; replay makes zero generation/assessment dispatches"})
		}
		for _, item := range []struct {
			id, name string
			value    int
		}{{"M15", "pending-scripted-unsupported-statements", unsupported}, {"M27", "explicit-user-assertion-cohort", userAssertions}} {
			n := item.value
			value := float64(n)
			metrics = append(metrics, Metric{ID: item.id, Version: "lab-metrics-v1", Measure: item.name, Mode: "controlled", Source: source, Value: &value, ObservationCount: &n, Eligible: n, Evaluated: n, Units: "count", Estimator: "joined original machine units or separate user-edited/confirmed provenance records"})
		}
		for _, gap := range []struct{ id, reason string }{
			{"M09", "owner structural and causal rejection opportunities are not exhaustively labeled"},
			{"M10", "scripted assessment statuses do not measure assessor accuracy"},
			{"M11", "independent supported/unsupported adjudication pending"},
			{"M12", "scripted uncertain statuses are not calibrated semantic uncertainty"},
			{"M16", "scripted client acceptance is not human acceptance"},
			{"M19", "resource/deadline assertions are owner test evidence, not an observed population failure rate"},
			{"M23", "simulated calls are not provider billing or live cost"},
			{"M24", "serialization/reparse is not a durable browser write; separate browser receipt retains storage observations"},
			{"M25", "scripted review actions do not measure human review burden"},
		} {
			metrics = append(metrics, Metric{ID: gap.id, Version: "lab-metrics-v1", Measure: "cv-integrity-unavailable", Mode: "controlled", Source: source, Units: "unavailable", Estimator: "unavailable", UnavailableReason: gap.reason})
		}
		// Per-case annotations retain language/split pairing; never pool translated cases.
		for i := range metrics {
			if metrics[i].CaseID == "" {
				metrics[i].CaseID = c.ID
				metrics[i].Workflow = c.Workflow
				metrics[i].Language = c.Language
				metrics[i].Split = c.Split
				metrics[i].ExposureGroup = c.ExposureGroup
				metrics[i].Configuration = c.Configuration
			}
		}
	}
	return metrics
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
func cvUnavailableAssessment(output json.RawMessage) int {
	var envelope struct {
		Support struct {
			Assessment string `json:"assessment"`
		} `json:"support"`
	}
	_ = json.Unmarshal(output, &envelope)
	return boolCount(envelope.Support.Assessment == "unavailable")
}

package evaluationlab

import "professional-information-repo/internal/semanticeval"

type SourceReviewReport struct {
	Version               string                    `json:"version"`
	WorksheetID           string                    `json:"worksheetId"`
	ContextHash           string                    `json:"contextHash"`
	OriginalSourceHash    string                    `json:"originalSourceHash"`
	OriginalOutputHash    string                    `json:"originalOutputHash"`
	OriginalGoldHash      string                    `json:"originalGoldHash"`
	OriginalLabelVersion  string                    `json:"originalLabelVersion"`
	DerivedLabelVersion   string                    `json:"derivedLabelVersion,omitempty"`
	MeasurementOnly       bool                      `json:"measurementOnly"`
	FinalGoldEligible     bool                      `json:"finalGoldEligible"`
	Classifications       map[string]int            `json:"classifications"`
	Dimensions            map[string]map[string]int `json:"dimensions"`
	PendingAnnotations    int                       `json:"pendingAnnotations"`
	ProposalDisagreements int                       `json:"proposalDisagreements"`
	Metrics               []Metric                  `json:"metrics"`
	Limits                []string                  `json:"limits"`
}

// ReviewReport derives classifications from an owned append-only receipt. It
// neither re-executes a workflow nor replaces original curated scorer outputs.
func (s *FileStore) ReviewReport(runID, worksheetID string) (SourceReviewReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	worksheet, err := s.loadReviewWorksheet(runID, worksheetID)
	if err != nil {
		return SourceReviewReport{}, err
	}
	result := SourceReviewReport{Version: "lab-source-review-report-v1", WorksheetID: worksheet.ID, ContextHash: worksheet.ContextHash, OriginalSourceHash: semanticeval.Hash(worksheet.Binding.SourceBytes), OriginalOutputHash: semanticeval.Hash(worksheet.Binding.OutputBytes), OriginalGoldHash: semanticeval.Hash(worksheet.Binding.GoldBytes), OriginalLabelVersion: worksheet.Binding.LabelVersion, DerivedLabelVersion: worksheet.DerivedLabelVersion, MeasurementOnly: true, Classifications: map[string]int{}, Dimensions: map[string]map[string]int{}, PendingAnnotations: len(worksheet.Annotations), Metrics: []Metric{}, Limits: []string{"Classification changes are measurement-only: original output, gold, scorer and run stay immutable.", "Annotated spans are not an exhaustive output-atom population or a newly balanced benchmark.", "Retrieval relevance and qualification sufficiency remain separate review dimensions.", "No release thresholds changed; zero forbidden factual additions remains the curated integrity requirement."}}
	annotations := map[string]ReviewAnnotation{}
	for _, a := range worksheet.Annotations {
		annotations[a.ID] = a
	}
	if len(worksheet.Adjudications) > 0 {
		latest := worksheet.Adjudications[len(worksheet.Adjudications)-1]
		if latest.Verified {
			result.FinalGoldEligible = worksheet.State == "reviewed" && latest.Origin == "local-human-attestation"
			for _, decision := range latest.Decisions {
				result.Classifications[decision.Verdict]++
				a := annotations[decision.AnnotationID]
				dimension := a.Dimension
				if dimension == "" {
					dimension = "statement-support"
				}
				if result.Dimensions[dimension] == nil {
					result.Dimensions[dimension] = map[string]int{}
				}
				result.Dimensions[dimension][decision.Verdict]++
				if decision.Verdict != "unresolved" {
					result.PendingAnnotations--
				}
				if a.ProposedVerdict != decision.Verdict {
					result.ProposalDisagreements++
				}
			}
		}
	}
	for _, id := range []string{"M03", "M04", "M05", "M06", "M07", "M10", "M11", "M12", "M13", "M14", "M15", "M25", "M27"} {
		result.Metrics = append(result.Metrics, Metric{ID: id, Version: "lab-metrics-v1", Measure: "source-review-population-unavailable", Source: worksheet.ContextHash, Mode: "review-only", Eligible: len(worksheet.Annotations), Missing: len(worksheet.Annotations), Units: "unavailable", Estimator: "unavailable", UnavailableReason: "Selected review annotations do not establish an exhaustive independently adjudicated population, raw assessor predictions, ranked retrieval, acceptance events or observed user review burden. Use the original run metrics under their original labels; do not pool label versions."})
	}
	return result, nil
}

package profileingestion

import (
	"errors"
	"professional-information-repo/internal/preprocessing"
)

type portionRequest struct {
	Index int `json:"index"`
	Bytes int `json:"bytes"`
}

// Coverage records execution of source regions, separately from claim discovery.
// No state, validation token, complete paste or provider result is persisted.
type continuationProgress struct {
	SourceID     string                  `json:"sourceId"`
	Index        int                     `json:"index"`
	Total        int                     `json:"total"`
	Bytes        int                     `json:"bytes"`
	Regions      [][]preprocessing.Range `json:"regions"`
	Remaining    []preprocessing.Range   `json:"remaining"`
	PlanComplete bool                    `json:"planComplete"`
	Processed    bool                    `json:"processed"`
}

func planContinuation(s preprocessing.Source, request portionRequest) (preprocessing.Plan, error) {
	if request.Index < 0 || request.Bytes < 200 || request.Bytes > 2000 {
		return preprocessing.Plan{}, errors.New("invalid portion")
	}
	plan, err := s.Plan(preprocessing.Budget{PreparedBytes: request.Bytes, PayloadBytes: maxProviderPayload, MaxPortions: 256}, func(v preprocessing.View) ([]byte, error) {
		return providerPayload(extractionPrompt(s, &v), "profile_claims", extractionSchema()), nil
	})
	if err != nil || request.Index >= len(plan.Portions) {
		return plan, errors.New("unavailable portion")
	}
	return plan, nil
}

func portionText(s preprocessing.Source, view *preprocessing.View) string {
	if view == nil {
		return s.Text()
	}
	text := ""
	for _, c := range view.Context {
		text += c.Text + "\n"
	}
	return text + view.Text
}
func portionSegments(s preprocessing.Source, view *preprocessing.View) []extractionSegment {
	segments := extractionSegments(s)
	if view == nil {
		return segments
	}
	selected := []extractionSegment{}
	for _, seg := range segments {
		included := seg.Start < view.Normalized.End && seg.End > view.Normalized.Start
		for _, c := range view.Context {
			included = included || c.SegmentID == seg.ID
		}
		if included {
			// A segment can be split by the planner; expose only selected bytes.
			start, end := seg.Start, seg.End
			if seg.Start < view.Normalized.End && seg.End > view.Normalized.Start {
				if start < view.Normalized.Start {
					start = view.Normalized.Start
				}
				if end > view.Normalized.End {
					end = view.Normalized.End
				}
			}
			seg.Text = s.Text()[start:end]
			seg.Start = start
			seg.End = end
			selected = append(selected, seg)
		}
	}
	return selected
}
func referenceInView(ref *sourceReference, view *preprocessing.View) bool {
	if view == nil {
		return true
	}
	if ref == nil {
		return false
	}
	ranges := append([]preprocessing.Range{}, view.Original...)
	for _, c := range view.Context {
		ranges = append(ranges, c.Original...)
	}
	for _, r := range ranges {
		if ref.OriginalStart >= r.Start && ref.OriginalEnd <= r.End {
			return true
		}
	}
	return false
}

package profileingestion

import (
	"regexp"
	"strings"

	"professional-information-repo/internal/preprocessing"
)

// Byte budgets include normalization expansion and the complete serialized
// provider envelope/schema/context. They are not token estimates. No portion
// execution or automatic continuation is allowed by this integration.
const maxPreparedInput = preprocessing.MaxPreparedWorkflowBytes
const maxProviderPayload = preprocessing.MaxWorkflowPayloadBytes

type sourceReference struct {
	Version            int    `json:"version"`
	SourceID           string `json:"sourceId"`
	PreparationVersion string `json:"preparationVersion"`
	SegmentID          string `json:"segmentId"`
	OccurrenceID       string `json:"occurrenceId"`
	OriginalStart      int    `json:"originalStart"`
	OriginalEnd        int    `json:"originalEnd"`
}

type extractionSegment struct {
	Text     string `json:"text"`
	ID       string `json:"id"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
	ParentID string `json:"parentId"`
}

func extractionSegments(s preprocessing.Source) []extractionSegment {
	segments := s.Segments()
	result := make([]extractionSegment, 0, len(segments))
	for _, seg := range segments {
		result = append(result, extractionSegment{s.Text()[seg.Normalized.Start:seg.Normalized.End], seg.ID, seg.Normalized.Start, seg.Normalized.End, seg.ParentID})
	}
	return result
}

// Resolve an exact or whitespace-tolerant prepared excerpt by its starting
// segment. Missing identity is tolerated only for a globally unique match.
// Multiple matches, unknown segments and split normalization groups fail closed.
func resolveExcerpt(s preprocessing.Source, excerpt, segmentID string) (string, *sourceReference) {
	if strings.TrimSpace(excerpt) == "" {
		return "", nil
	}
	start, end := 0, len(s.Text())
	if segmentID != "" {
		found := false
		for _, seg := range s.Segments() {
			if seg.ID == segmentID {
				start, end, found = seg.Normalized.Start, seg.Normalized.End, true
				break
			}
		}
		if !found {
			return "", nil
		}
	}
	matches := [][2]int{}
	// Look for overlapping exact matches too; two occurrences are ambiguous.
	for offset := start; offset < end; {
		i := strings.Index(s.Text()[offset:], excerpt)
		if i < 0 || offset+i >= end {
			break
		}
		at := offset + i
		matches = append(matches, [2]int{at, at + len(excerpt)})
		if len(matches) > 1 {
			return "", nil
		}
		offset = at + 1
	}
	if len(matches) == 0 {
		fields := strings.Fields(excerpt)
		for i := range fields {
			fields[i] = regexp.QuoteMeta(fields[i])
		}
		pattern, err := regexp.Compile(strings.Join(fields, `\s+`))
		if err != nil {
			return "", nil
		}
		// Advance from each start, so overlapping whitespace matches are ambiguous.
		for offset := start; offset < end; {
			loc := pattern.FindStringIndex(s.Text()[offset:])
			if loc == nil || offset+loc[0] >= end {
				break
			}
			at := offset + loc[0]
			matches = append(matches, [2]int{at, offset + loc[1]})
			if len(matches) > 1 {
				return "", nil
			}
			offset = at + 1
		}
	}
	if len(matches) != 1 {
		return "", nil
	}
	r := preprocessing.Range{Start: matches[0][0], End: matches[0][1]}
	ranges, err := s.Resolve(r)
	if err != nil || len(ranges) != 1 {
		return "", nil
	}
	original := ranges[0]
	if original.End-original.Start > 1000 {
		return "", nil
	}
	for _, seg := range s.Segments() {
		if r.Start >= seg.Normalized.Start && r.Start < seg.Normalized.End {
			segmentID = seg.ID
			break
		}
	}
	occurrence, err := s.RangeID(r)
	if err != nil || segmentID == "" {
		return "", nil
	}
	return s.Original()[original.Start:original.End], &sourceReference{1, s.ID(), s.Version(), segmentID, occurrence, original.Start, original.End}
}

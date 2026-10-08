package preprocessing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// RepetitionVersion pins both the comparison policy and the formatting rules.
const RepetitionVersion = "repetition-v1/" + RulesVersion + "/" + StructureVersion

type RepetitionComparison string

const (
	ExactRepetition       RepetitionComparison = "exact_original"
	FormattingEquivalence RepetitionComparison = "formatting_equivalent"
)

// Repetition annotates text equality, never fact/entity equality or permission
// to discard content. Occurrences retain the complete syntax/source contract.
type Repetition struct {
	ID          string
	SourceID    string
	Version     string
	Comparison  RepetitionComparison
	Occurrences []Segment
}

type repetitionBucket struct {
	original    string
	varied      bool
	occurrences []Segment
}

// Repetitions compares complete segments, including markers and line endings.
// Exact groups precede formatting groups; each class is ordered by first
// occurrence. Formatting groups include all occurrences but are emitted only
// when at least two original byte strings differ. An exact subgroup can thus
// also occur inside a formatting group. No extra cleanup is performed here.
// Normalize-only and empty Sources have no segments and return no groups.
func (s Source) Repetitions() []Repetition {
	var exact, formatted []repetitionBucket
	exactIndex, formattedIndex := map[string]int{}, map[string]int{}
	add := func(index map[string]int, buckets *[]repetitionBucket, key, original string, seg Segment) {
		i, exists := index[key]
		if !exists {
			i = len(*buckets)
			index[key] = i
			*buckets = append(*buckets, repetitionBucket{original: original})
		}
		bucket := &(*buckets)[i]
		bucket.varied = bucket.varied || original != bucket.original
		bucket.occurrences = append(bucket.occurrences, seg)
	}
	for _, seg := range s.Segments() {
		var original strings.Builder
		for _, r := range seg.Original {
			original.WriteString(s.original[r.Start:r.End])
		}
		raw := original.String()
		// Go string-keyed maps use full string equality after hash lookup: a
		// collision cannot establish repetition. No all-pairs comparison occurs.
		add(exactIndex, &exact, raw, raw, seg)
		add(formattedIndex, &formatted, s.text[seg.Normalized.Start:seg.Normalized.End], raw, seg)
	}
	var result []Repetition
	emit := func(buckets []repetitionBucket, comparison RepetitionComparison) {
		for _, bucket := range buckets {
			if len(bucket.occurrences) < 2 || (comparison == FormattingEquivalence && !bucket.varied) {
				continue
			}
			identity, _ := json.Marshal([]string{s.id, RepetitionVersion, string(comparison), bucket.occurrences[0].ID})
			digest := sha256.Sum256(identity)
			result = append(result, Repetition{
				ID: hex.EncodeToString(digest[:]), SourceID: s.id,
				Version: RepetitionVersion, Comparison: comparison,
				Occurrences: bucket.occurrences,
			})
		}
	}
	emit(exact, ExactRepetition)
	emit(formatted, FormattingEquivalence)
	return result
}

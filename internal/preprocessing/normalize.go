// Package preprocessing prepares transient text mechanically. It makes no field
// acceptance, safety, relevance, or semantic decision.
package preprocessing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"professional-information-repo/internal/fieldvalidation"
)

const RulesVersion = "normalization-v1"

// Range is a half-open UTF-8 byte range, never a UTF-16 code-unit range.
type Range struct{ Start, End int }

// Mapping records correspondence at an unchanged character or transformed NFC
// group/line ending. A transformed group cannot be resolved only in part.
type Mapping struct{ Normalized, Original Range }

// Source keeps its original, working view and mappings immutable to callers.
// A zero Source is invalid; construct one with Normalize.
type Source struct {
	original, text, id string
	field              fieldvalidation.Field
	mapping            []Mapping
}

func (s Source) Original() string             { return s.original }
func (s Source) Text() string                 { return s.text }
func (s Source) ID() string                   { return s.id }
func (s Source) Field() fieldvalidation.Field { return s.field }
func (s Source) Version() string              { return RulesVersion }
func (s Source) Mappings() []Mapping          { return append([]Mapping(nil), s.mapping...) }

// Normalize accepts decoded UTF-8, not encoded transport bytes. Field is an
// identity namespace only. There are no configurable rules in version 1.
func Normalize(original string, field fieldvalidation.Field) (Source, error) {
	if !utf8.ValidString(original) {
		return Source{}, errors.New("source is not valid UTF-8")
	}
	if field != fieldvalidation.ProfessionalInformation && field != fieldvalidation.JobPosting {
		return Source{}, errors.New("unknown source field")
	}
	s := Source{original: original, field: field}
	identity, _ := json.Marshal([]string{RulesVersion, string(field), original})
	digest := sha256.Sum256(identity)
	s.id = hex.EncodeToString(digest[:])
	var out strings.Builder
	protected := literalBytes(original)
	add := func(start, end int, value string) {
		n := out.Len()
		out.WriteString(value)
		s.mapping = append(s.mapping, Mapping{Range{n, out.Len()}, Range{start, end}})
	}
	for pos := 0; pos < len(original); {
		if protected[pos] {
			_, size := utf8.DecodeRuneInString(original[pos:])
			add(pos, pos+size, original[pos:pos+size])
			pos += size
			continue
		}
		if original[pos] == '\r' {
			end := pos + 1
			if end < len(original) && original[end] == '\n' {
				end++
			}
			add(pos, end, "\n")
			pos = end
			continue
		}
		end := pos
		for end < len(original) && original[end] != '\r' && !protected[end] {
			end++
		}
		for start := pos; start < end; {
			_, size := utf8.DecodeRuneInString(original[start:end])
			stop := start + size
			for stop < end && !norm.NFC.PropertiesString(original[stop:end]).BoundaryBefore() {
				_, size = utf8.DecodeRuneInString(original[stop:end])
				stop += size
			}
			value := canonicalNFC(original[start:stop])
			if original[start:stop] == value {
				for i, r := range value {
					add(start+i, start+i+utf8.RuneLen(r), string(r))
				}
			} else {
				add(start, stop, value)
			}
			start = stop
		}
		pos = end
	}
	s.text = out.String()
	return s, nil
}

// Resolve recovers exact original ranges. It rejects empty/out-of-bounds ranges,
// split UTF-8 characters and partial transformed groups rather than guessing.
// Adjacent original ranges are coalesced; repeated text resolves by occurrence.
func (s Source) Resolve(r Range) ([]Range, error) {
	if s.id == "" || r.Start < 0 || r.End <= r.Start || r.End > len(s.text) {
		return nil, errors.New("invalid normalized range")
	}
	var result []Range
	for _, m := range s.mapping {
		if m.Normalized.End <= r.Start {
			continue
		}
		if m.Normalized.Start >= r.End {
			break
		}
		if m.Normalized.Start < r.Start || m.Normalized.End > r.End {
			return nil, errors.New("range splits character or transformed group")
		}
		if len(result) > 0 && result[len(result)-1].End == m.Original.Start {
			result[len(result)-1].End = m.Original.End
		} else {
			result = append(result, m.Original)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("unresolvable normalized range")
	}
	return result, nil
}

// RangeID identifies one normalized occurrence within this exact source/version.
func (s Source) RangeID(r Range) (string, error) {
	if _, err := s.Resolve(r); err != nil {
		return "", err
	}
	identity, _ := json.Marshal(struct {
		Source string
		Range  Range
	}{s.id, r})
	digest := sha256.Sum256(identity)
	return hex.EncodeToString(digest[:]), nil
}

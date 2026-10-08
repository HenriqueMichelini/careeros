package preprocessing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"

	"professional-information-repo/internal/fieldvalidation"
)

const StructureVersion = "structure-v1"

type Kind string

const (
	Text             Kind = "text"
	Blank            Kind = "blank"
	Heading          Kind = "heading"
	ListItem         Kind = "list_item"
	ListContinuation Kind = "list_continuation"
	Separator        Kind = "separator"
	Quotation        Kind = "quotation"
	Literal          Kind = "literal"
	Table            Kind = "table"
)

// Segment covers syntax only. ParentID refers to an earlier explicit heading or
// list item; it makes no claim about semantic ownership. Ranges include endings.
type Segment struct {
	ID         string
	Kind       Kind
	Normalized Range
	Original   []Range
	ParentID   string
	Indent     int // leading columns, with tabs advancing to the next multiple of four
	Level      int // explicit ATX heading level, otherwise zero
}

func (s Source) Segments() []Segment {
	result := append([]Segment(nil), s.segments...)
	for i := range result {
		result[i].Original = append([]Range(nil), result[i].Original...)
	}
	return result
}

// Prepare composes normalization, guarded formatting cleanup and syntax-only
// segmentation. It has no I/O or acceptance authority. Normalize remains the
// separate normalization-v1 API for callers that do not need structure.
func Prepare(original string, field fieldvalidation.Field) (Source, error) {
	guards := literalBytes(original)
	for _, line := range scanStructure(original) {
		if line.kind == Literal || line.kind == Table || line.kind == Quotation {
			for i := line.start; i < line.end; i++ {
				guards[i] = true
			}
		}
	}
	s, err := normalizeProtected(original, field, guards)
	if err != nil {
		return Source{}, err
	}
	s.version = StructureVersion
	identity, _ := json.Marshal([]string{RulesVersion, StructureVersion, string(field), original})
	digest := sha256.Sum256(identity)
	s.id = hex.EncodeToString(digest[:])
	lines := scanStructure(s.text)
	protected := literalBytes(s.text)
	var out strings.Builder
	var mapping []Mapping
	// Existing mapping groups are indivisible. Only unprotected horizontal
	// separator groups inside prose/list text are coalesced; nothing is deleted.
	for i := 0; i < len(s.mapping); {
		m := s.mapping[i]
		line := lineAt(lines, m.Normalized.Start)
		eligible := (line.kind == Text || line.kind == ListItem || line.kind == ListContinuation) && !tableLike(s.text[line.content:line.bodyEnd])
		start := m.Normalized.Start
		end, stop := m.Normalized.End, i+1
		if eligible && start >= line.content && horizontal(s.text[start:end]) && !protected[start] {
			for stop < len(s.mapping) {
				next := s.mapping[stop]
				if next.Normalized.End > line.bodyEnd || protected[next.Normalized.Start] || !horizontal(s.text[next.Normalized.Start:next.Normalized.End]) {
					break
				}
				end = next.Normalized.End
				stop++
			}
			// Preserve leading indentation, trailing whitespace/hard breaks and any
			// whitespace adjacent to a protected value when the guard is uncertain.
			if start > line.content && end < line.bodyEnd && !protected[start-1] && !protected[end] {
				n := out.Len()
				out.WriteByte(' ')
				mapping = append(mapping, Mapping{Range{n, out.Len()}, Range{m.Original.Start, s.mapping[stop-1].Original.End}})
				i = stop
				continue
			}
		}
		n := out.Len()
		out.WriteString(s.text[start:m.Normalized.End])
		mapping = append(mapping, Mapping{Range{n, out.Len()}, m.Original})
		i++
	}
	s.text, s.mapping = out.String(), mapping
	lines = scanStructure(s.text)
	type ancestor struct{ index, depth int }
	var headings, lists []ancestor
	for _, line := range lines {
		parent := ""
		if line.kind == Heading {
			lists = nil
			for len(headings) > 0 && headings[len(headings)-1].depth >= line.level {
				headings = headings[:len(headings)-1]
			}
		}
		if len(headings) > 0 {
			parent = s.segments[headings[len(headings)-1].index].ID
		}
		if line.kind == ListItem {
			for len(lists) > 0 && lists[len(lists)-1].depth > line.indent {
				lists = lists[:len(lists)-1]
			}
		} else if line.kind != Blank {
			for len(lists) > 0 && line.indent < lists[len(lists)-1].depth {
				lists = lists[:len(lists)-1]
			}
			if line.kind == Text && len(lists) > 0 {
				line.kind = ListContinuation
			}
		}
		if len(lists) > 0 && line.kind != Heading {
			parent = s.segments[lists[len(lists)-1].index].ID
		}
		r := Range{line.start, line.end}
		id, err := s.RangeID(r)
		if err != nil {
			return Source{}, err
		}
		originalRanges, err := s.Resolve(r)
		if err != nil {
			return Source{}, err
		}
		s.segments = append(s.segments, Segment{id, line.kind, r, originalRanges, parent, line.indent, line.level})
		index := len(s.segments) - 1
		if line.kind == Heading {
			headings = append(headings, ancestor{index, line.level})
		}
		if line.kind == ListItem {
			lists = append(lists, ancestor{index, line.contentColumn})
		}
		if line.kind == Separator {
			lists = nil
		}
	}
	return s, nil
}

var listMarker = regexp.MustCompile(`^(?:[-+*]|[0-9]{1,9}[.)])[ \t]+`)
var atxHeading = regexp.MustCompile(`^#{1,6}(?:[ \t]+|$)`)
var alignmentGaps = regexp.MustCompile(` {2,}`)

type syntaxLine struct {
	start, end, bodyEnd, content, indent, contentColumn, level int
	kind                                                       Kind
	fenced                                                     bool
}

func columns(s string) int {
	n := 0
	for _, c := range s {
		if c == '\t' {
			n += 4 - n%4
		} else {
			n++
		}
	}
	return n
}
func horizontal(s string) bool {
	return s == " " || s == "\t" || s == "\u00a0"
}
func lineAt(lines []syntaxLine, pos int) syntaxLine {
	// Binary search keeps preparation bounded even for many short lines.
	lo, hi := 0, len(lines)
	for lo < hi {
		mid := (lo + hi) / 2
		if lines[mid].end <= pos {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lines[lo]
}
func scanStructure(text string) []syntaxLine {
	var lines []syntaxLine
	var fence byte
	fenceSize := 0
	for pos := 0; pos < len(text); {
		end := lineEnd(text, pos)
		bodyEnd := end
		for bodyEnd > pos && (text[bodyEnd-1] == '\r' || text[bodyEnd-1] == '\n') {
			bodyEnd--
		}
		content := pos
		for content < bodyEnd && (text[content] == ' ' || text[content] == '\t') {
			content++
		}
		body := text[content:bodyEnd]
		line := syntaxLine{start: pos, end: end, bodyEnd: bodyEnd, content: content, indent: columns(text[pos:content]), kind: Text}
		switch {
		case fence != 0:
			line.kind = Literal
			line.fenced = true
			if markerRun(body, 0, fence) >= fenceSize && strings.TrimSpace(body[markerRun(body, 0, fence):]) == "" {
				fence = 0
			}
		case line.indent <= 3 && len(body) >= 3 && (body[0] == '`' || body[0] == '~') && markerRun(body, 0, body[0]) >= 3:
			line.kind = Literal
			line.fenced = true
			fence = body[0]
			fenceSize = markerRun(body, 0, fence)
		case strings.TrimSpace(body) == "":
			line.kind = Blank
		case line.indent >= 4:
			line.kind = Literal
		case strings.HasPrefix(body, ">"):
			line.kind = Quotation
		case isSeparator(body):
			line.kind = Separator
		default:
			if match := atxHeading.FindString(body); match != "" {
				line.kind = Heading
				line.level = strings.Count(strings.TrimSpace(match), "#")
			} else if match := listMarker.FindString(body); match != "" {
				line.kind = ListItem
			} else if tableLike(body) {
				line.kind = Table
			}
		}
		lines = append(lines, line)
		pos = end
	}
	// Explicit list indentation makes continuation syntax recognizable; otherwise
	// four-column indentation remains literal. A blank line retains attachment.
	var listColumns []int
	for i := range lines {
		l := &lines[i]
		if l.kind == ListItem || (l.kind == Literal && !l.fenced && listMarker.MatchString(text[l.content:l.bodyEnd]) && len(listColumns) > 0) {
			l.kind = ListItem
			marker := listMarker.FindString(text[l.content:l.bodyEnd])
			l.contentColumn = l.indent + columns(marker)
			l.content += len(marker)
			for len(listColumns) > 0 && listColumns[len(listColumns)-1] > l.indent {
				listColumns = listColumns[:len(listColumns)-1]
			}
			listColumns = append(listColumns, l.contentColumn)
		} else if l.kind != Blank {
			for len(listColumns) > 0 && l.indent < listColumns[len(listColumns)-1] {
				listColumns = listColumns[:len(listColumns)-1]
			}
			if len(listColumns) > 0 && (l.kind == Text || (l.kind == Literal && !l.fenced)) && l.indent < listColumns[len(listColumns)-1]+4 {
				l.kind = ListContinuation
			}
		}
	}
	return lines
}
func isSeparator(s string) bool {
	s = strings.TrimRight(s, " \t")
	for _, marker := range []rune{'-', '*', '_'} {
		count := 0
		valid := true
		for _, r := range s {
			if r == marker {
				count++
			} else if r != ' ' && r != '\t' {
				valid = false
				break
			}
		}
		if valid && count >= 3 {
			return true
		}
	}
	return false
}

func tableLike(body string) bool {
	return strings.ContainsAny(body, "|\t") || len(alignmentGaps.FindAllStringIndex(strings.TrimRight(body, " \t"), -1)) >= 2
}

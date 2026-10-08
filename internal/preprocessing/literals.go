package preprocessing

import (
	"regexp"
	"strings"
)

// These are lexical guards, not URL/email validation or entity extraction. A
// conservative overmatch preserves bytes rather than modifying a possible value.
var literalValue = regexp.MustCompile("(?i:https?://|mailto:|www\\.)[^\\s<>\"`]+|[\\pL\\pM\\pN.!#$%&'*+/=?^_`{|}~-]+@[\\pL\\pM\\pN_-]+(?:\\.[\\pL\\pM\\pN_-]+)+")

func literalBytes(text string) []bool {
	protected := make([]bool, len(text))
	mark := func(start, end int) {
		for i := start; i < end; i++ {
			protected[i] = true
		}
	}
	for pos := 0; pos < len(text); {
		if text[pos] != '`' && text[pos] != '~' {
			pos++
			continue
		}
		marker := text[pos]
		run := markerRun(text, pos, marker)
		if run >= 3 && lineIndent(text, pos) {
			end := len(text)
			for next := lineEnd(text, pos); next < len(text); {
				content := next
				for content < len(text) && text[content] == ' ' && content-next < 3 {
					content++
				}
				closing := markerRun(text, content, marker)
				stop := lineEnd(text, content)
				if closing >= run && strings.TrimSpace(text[content+closing:stop]) == "" {
					end = stop
					break
				}
				next = stop
			}
			mark(pos, end)
			pos = end
			continue
		}
		if marker == '~' {
			pos += run
			continue
		}
		end := len(text)
		for next := pos + run; next < len(text); {
			if text[next] != '`' {
				next++
				continue
			}
			closing := markerRun(text, next, '`')
			if closing == run {
				end = next + closing
				break
			}
			next += closing
		}
		mark(pos, end)
		pos = end
	}
	for _, match := range literalValue.FindAllStringIndex(text, -1) {
		mark(match[0], match[1])
	}
	return protected
}

func markerRun(text string, start int, marker byte) int {
	end := start
	for end < len(text) && text[end] == marker {
		end++
	}
	return end - start
}

func lineIndent(text string, pos int) bool {
	start := pos
	for start > 0 && text[start-1] == ' ' {
		start--
	}
	return pos-start <= 3 && (start == 0 || text[start-1] == '\n' || text[start-1] == '\r')
}

// lineEnd includes the original LF, CRLF or CR ending, if present.
func lineEnd(text string, start int) int {
	end := start
	for end < len(text) && text[end] != '\n' && text[end] != '\r' {
		end++
	}
	if end < len(text) {
		if text[end] == '\r' && end+1 < len(text) && text[end+1] == '\n' {
			return end + 2
		}
		return end + 1
	}
	return end
}

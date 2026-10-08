package preprocessing

import (
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// x/text enforces stream safety by inserting CGJ after 30 non-starters. That
// insertion is outside our rules. For those rare groups, decompose each rune
// independently, order combining marks, then compose without the stream limit.
func canonicalNFC(text string) string {
	value := norm.NFC.String(text)
	if strings.Count(value, "\u034f") <= strings.Count(text, "\u034f") {
		return value
	}
	type character struct {
		value rune
		ccc   uint8
	}
	var decomposed []character
	for _, r := range text {
		for _, d := range norm.NFD.String(string(r)) {
			decomposed = append(decomposed, character{d, norm.NFD.PropertiesString(string(d)).CCC()})
		}
	}
	for start := 0; start < len(decomposed); {
		if decomposed[start].ccc == 0 {
			start++
		}
		end := start
		for end < len(decomposed) && decomposed[end].ccc != 0 {
			end++
		}
		sort.SliceStable(decomposed[start:end], func(i, j int) bool { return decomposed[start+i].ccc < decomposed[start+j].ccc })
		start = end
	}
	var composed []rune
	starter, lastCCC := -1, uint8(0)
	for _, c := range decomposed {
		if starter >= 0 && (lastCCC == 0 || lastCCC < c.ccc) {
			pair := norm.NFC.String(string([]rune{composed[starter], c.value}))
			if utf8.RuneCountInString(pair) == 1 {
				composed[starter], _ = utf8.DecodeRuneInString(pair)
				continue
			}
		}
		if c.ccc == 0 {
			starter = len(composed)
		}
		composed = append(composed, c.value)
		lastCCC = c.ccc
	}
	return string(composed)
}

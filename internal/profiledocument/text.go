package profiledocument

import "strings"

// HasText matches JavaScript String.prototype.trim() !== "" at the shared
// Profile projection boundary. Go's strings.TrimSpace differs for U+0085 and
// U+FEFF. This predicate never changes the saved or projected wording.
func HasText(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool { return !textWhitespace(r) }) >= 0
}

// TrimText is used when constructing an application-owned signature, never to
// rewrite a saved Profile or its immutable source snapshot.
func TrimText(value string) string { return strings.TrimFunc(value, textWhitespace) }

func textWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', '\u00a0', '\u1680',
		'\u2028', '\u2029', '\u202f', '\u205f', '\u3000', '\ufeff':
		return true
	default:
		return r >= '\u2000' && r <= '\u200a'
	}
}

// WhitespaceSplitCount matches JavaScript value.split(/\s+/).length, including
// leading/trailing empty fields. Callers explicitly trim when their contract does.
func WhitespaceSplitCount(value string) int {
	count, previous := 1, false
	for _, r := range value {
		current := textWhitespace(r)
		if current && !previous {
			count++
		}
		previous = current
	}
	return count
}

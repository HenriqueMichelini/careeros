package profiledocument

import "strings"

// HasText matches JavaScript String.prototype.trim() !== "" at the shared
// Profile projection boundary. Go's strings.TrimSpace differs for U+0085 and
// U+FEFF. This predicate never changes the saved or projected wording.
func HasText(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool {
		switch r {
		case '\t', '\n', '\v', '\f', '\r', ' ', '\u00a0', '\u1680',
			'\u2028', '\u2029', '\u202f', '\u205f', '\u3000', '\ufeff':
			return false
		default:
			return r < '\u2000' || r > '\u200a'
		}
	}) >= 0
}

package applicationdraft

import (
	"professional-information-repo/internal/profiledocument"
	"strings"
)

// ValidCompletedCoverLetter applies the existing structured-letter contract to
// a frozen completed string. It never rebuilds a historical letter from a newer
// Profile, and keeps the signature in its application-owned slot.
func ValidCompletedCoverLetter(completed, fullName string) bool {
	name := profiledocument.TrimText(fullName)
	letter := completed
	if name != "" {
		if !strings.HasSuffix(letter, "\n"+name) {
			return false
		}
		letter = strings.TrimSuffix(letter, "\n"+name)
	}
	parts := strings.Split(letter, "\n\n")
	if len(parts) < 3 {
		return false
	}
	c := coverLetter{Greeting: parts[0], Body: strings.Join(parts[1:len(parts)-1], "\n\n"), Closing: parts[len(parts)-1]}
	if !validCoverLetterParts(c, profiledocument.TrimText, canonicalLetterWhitespace) || !profiledocument.HasText(c.Greeting) || !profiledocument.HasText(c.Body) || !profiledocument.HasText(c.Closing) {
		return false
	}
	if name != "" && strings.Contains(c.Greeting+"\n"+c.Body+"\n"+c.Closing, name) {
		return false
	}
	canonical := strings.Join([]string{profiledocument.TrimText(c.Greeting), profiledocument.TrimText(c.Body), profiledocument.TrimText(c.Closing)}, "\n\n")
	if name != "" {
		canonical += "\n" + name
	}
	return completed == canonical && profiledocument.WhitespaceSplitCount(completed) < 400
}

// Go regex \s is ASCII; JavaScript \s uses the same canonical whitespace set as
// trim(). Normalize ONLY the text inspected by identity regexes, never output.
func canonicalLetterWhitespace(value string) string {
	return strings.Map(func(r rune) rune {
		if profiledocument.TrimText(string(r)) == "" {
			return ' '
		}
		return r
	}, value)
}

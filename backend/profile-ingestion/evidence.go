package profileingestion

import (
	"encoding/json"
	"professional-information-repo/internal/profilevalidation"
	"regexp"
	"strings"
)

// This bounded check protects literal numbers and technology spellings, not
// semantic equivalence, ownership, proficiency or the truth of a claim.
var protectedTokens = regexp.MustCompile(`\b(?:[0-9]+(?:[.,][0-9]+)*(?:%|[kKmM])?|(?:JavaScript|Javascript|TypeScript|Typescript|Java|Python|Go|Rust|Ruby|React|Angular|Vue|Node|PostgreSQL|MySQL|SQL|AWS|Azure|Docker|Kubernetes|Spring|Boot|[A-Z]{2,}[A-Za-z0-9+#]*|[A-Z][a-z]+[A-Z][A-Za-z0-9]*)\b)`)
var sourceWords = regexp.MustCompile(`[\pL\pN_+#]+(?:[.,][0-9]+)*(?:%|[kKmM])?`)

func protectedSupported(value, source string) bool {
	words := map[string]bool{}
	for _, word := range sourceWords.FindAllString(source, -1) {
		words[exactAlias(word)] = true
		if strings.HasPrefix(word, "+") {
			words[strings.TrimPrefix(word, "+")] = true
		}
	}
	for _, token := range protectedTokens.FindAllString(value, -1) {
		if !words[exactAlias(token)] {
			return false
		}
	}
	return true
}
func exactAlias(s string) string {
	if s == "Javascript" {
		return "JavaScript"
	}
	if s == "Typescript" {
		return "TypeScript"
	}
	return s
}
func operationSource(op *operation, claims map[string]claim, p profilevalidation.Profile) string {
	ids := op.SupportingClaimIDs
	if ids == nil {
		ids = []string{op.ClaimID}
	}
	source := ""
	for _, id := range ids {
		c := claims[id]
		source += "\n" + c.Source
		for _, s := range c.SupportingSources {
			source += "\n" + s.Source
		}
	}
	// Existing wording may be preserved in a replacement, but cannot establish
	// new source support or be borrowed from another role/project.
	var view map[string]any
	_ = json.Unmarshal(mustJSON(p), &view)
	if op.EntryID == "" {
		if value, ok := view[op.Field].(string); ok {
			source += "\n" + value
		}
	} else if !strings.HasPrefix(op.EntryID, "new:") {
		if entries, ok := view[op.Target].([]any); ok {
			for _, e := range entries {
				item, _ := e.(map[string]any)
				if item["id"] == op.EntryID {
					if value, ok := item[op.Field].(string); ok {
						source += "\n" + value
					}
				}
			}
		}
	}
	return source
}

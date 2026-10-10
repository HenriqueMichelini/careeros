// Package qualificationmatching selects bounded, identity-preserving candidates.
// Selection is lexical preparation, never proof of semantic support or absence.
package qualificationmatching

import (
	_ "embed" // Embeds the shared qualification field policy.
	"encoding/json"
	"errors"
	"professional-information-repo/internal/jobcontext"
	"professional-information-repo/internal/profiledocument"
	"slices"
	"sort"
	"strings"
	"unicode"
)

const Version = "qualification-evidence-v1"
const DefaultBudget = 16 << 10

//go:embed fields.json
var fieldsJSON []byte
var allowed = func() map[string][]string {
	var fields map[string][]string
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		panic(err)
	}
	return fields
}()

type Fact struct {
	profiledocument.Fact
	Section  string                     `json:"section"`
	Evidence []profiledocument.Evidence `json:"evidence"`
}
type Candidates struct {
	RequirementIndex int    `json:"requirementIndex"`
	Facts            []Fact `json:"facts"`
	Complete         bool   `json:"complete"`
	Excluded         int    `json:"excluded"`
}
type Decision struct {
	RequirementIndex int      `json:"requirementIndex"`
	State            string   `json:"state"`
	FactIDs          []string `json:"factIds"`
	Explanation      string   `json:"explanation"`
	Question         string   `json:"question"`
}
type Match struct {
	Decision
	Requirement jobcontext.ResolvedItem `json:"requirement"`
	Facts       []Fact                  `json:"facts"`
	Complete    bool                    `json:"complete"`
	Excluded    int                     `json:"excluded"`
}

func Projection(raw []byte) (profiledocument.Document, error) {
	doc, err := profiledocument.Decode(raw)
	if err != nil {
		return doc, err
	}
	if len(doc.Facts) > 500 {
		return doc, errors.New("too many facts")
	}
	for _, f := range doc.Facts {
		kind := "profile"
		for _, e := range doc.Entities {
			if e.ID == f.Owner.ID {
				kind = e.Kind
			}
		}
		permitted := slices.Contains(allowed[kind], f.Field)
		if !permitted || (f.Approval != "approved" && !(f.Kind == "legacy_block" && f.Origin.Kind == "existing_profile")) {
			return doc, errors.New("private or unapproved fact")
		}
	}
	return doc, nil
}
func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '+' && r != '#' }) {
		if len(word) > 1 {
			out[word] = true
		}
	}
	return out
}

// Select keeps each employer/project with its dates and facts as one unit. A
// canonical fact is never truncated. Negative/uncertain units rank first.
func Select(doc profiledocument.Document, requirement string, index, budget int) Candidates {
	result := Candidates{RequirementIndex: index, Facts: []Fact{}, Complete: true}
	groups := map[string][]Fact{}
	order := []string{}
	for _, f := range doc.Facts {
		section := "profile"
		for _, e := range doc.Entities {
			if e.ID == f.Owner.ID {
				section = e.Kind
			}
		}
		fact := Fact{Fact: f, Section: section, Evidence: []profiledocument.Evidence{}}
		for _, l := range doc.Links {
			if l.Kind == "supports" && l.State == "active" && l.From.ID == f.ID && l.From.Revision == f.Revision {
				for _, e := range doc.Evidence {
					if e.ID == l.To.ID && e.Revision == l.To.Revision {
						fact.Evidence = append(fact.Evidence, e)
					}
				}
			}
		}
		key := f.Owner.ID
		if key == doc.ID && len(f.Context) == 0 {
			key = f.ID
		}
		if len(f.Context) > 0 {
			key = f.Context[0].ID
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], fact)
	}
	terms := words(requirement)
	score := func(group []Fact) int {
		score := 0
		for _, f := range group {
			if f.Assertion == "negated" || f.Intent == "aspiration" || f.Certainty == "uncertain" {
				score += 10000
			}
			for word := range words(string(f.Value)) {
				if terms[word] {
					score++
				}
			}
		}
		return score
	}
	sort.SliceStable(order, func(i, j int) bool { return score(groups[order[i]]) > score(groups[order[j]]) })
	used := 0
	for _, key := range order {
		group := groups[key]
		// Include every referenced entity's identity/period and contrasting facts.
		owners := map[string]bool{}
		for _, f := range group {
			owners[f.Owner.ID] = true
			for _, c := range f.Context {
				owners[c.ID] = true
			}
		}
		seen := map[string]bool{}
		for _, f := range group {
			seen[f.ID] = true
		}
		for _, otherKey := range order {
			for _, f := range groups[otherKey] {
				if owners[f.Owner.ID] && f.Owner.ID != doc.ID && !seen[f.ID] {
					group = append(group, f)
					seen[f.ID] = true
				}
			}
		}
		encoded, _ := json.Marshal(group)
		if used+len(encoded) > budget {
			result.Complete = false
			continue
		}
		used += len(encoded)
		for _, f := range group {
			exists := false
			for _, kept := range result.Facts {
				if kept.ID == f.ID {
					exists = true
				}
			}
			if !exists {
				result.Facts = append(result.Facts, f)
			}
		}
	}
	result.Excluded = len(doc.Facts) - len(result.Facts)
	result.Complete = result.Excluded == 0
	return result
}
func Resolve(requirements []jobcontext.ResolvedItem, candidates []Candidates, decisions []Decision) ([]Match, error) {
	invalid := errors.New("invalid requirement match")
	if len(decisions) != len(requirements) || len(candidates) != len(requirements) {
		return nil, invalid
	}
	result := make([]Match, len(requirements))
	seen := map[int]bool{}
	for _, d := range decisions {
		i := d.RequirementIndex
		if i < 0 || i >= len(requirements) || seen[i] || len(d.FactIDs) > 100 || strings.TrimSpace(d.Explanation) == "" || len(d.Explanation) > 1500 || len(d.Question) > 500 {
			return nil, invalid
		}
		seen[i] = true
		if d.State != "supported" && d.State != "partially_supported" && d.State != "not_evidenced" && d.State != "needs_clarification" {
			return nil, invalid
		}
		refs := map[string]bool{}
		for _, id := range d.FactIDs {
			if refs[id] {
				return nil, invalid
			}
			refs[id] = true
			found := false
			for _, f := range candidates[i].Facts {
				if f.ID == id {
					found = true
					if (d.State == "supported" || d.State == "partially_supported") && (f.Assertion == "negated" || f.Intent == "aspiration" || f.Certainty == "uncertain" || f.Support == "invalidated") {
						return nil, invalid
					}
				}
			}
			if !found {
				return nil, invalid
			}
		}
		if (d.State == "supported" || d.State == "partially_supported") && len(d.FactIDs) == 0 {
			return nil, invalid
		}
		if d.State == "needs_clarification" && strings.TrimSpace(d.Question) == "" {
			return nil, invalid
		}
		if !candidates[i].Complete && d.State == "not_evidenced" {
			d.State = "needs_clarification"
			d.Question = "Can you supply relevant evidence for this requirement?"
		}
		result[i] = Match{d, requirements[i], candidates[i].Facts, candidates[i].Complete, candidates[i].Excluded}
	}
	return result, nil
}
func ResponseFormat() map[string]any {
	obj := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
	}
	str := map[string]any{"type": "string"}
	decision := obj(map[string]any{"requirementIndex": map[string]any{"type": "integer"}, "state": map[string]any{"type": "string", "enum": []string{"supported", "partially_supported", "not_evidenced", "needs_clarification"}}, "factIds": map[string]any{"type": "array", "items": str}, "explanation": str, "question": str}, "requirementIndex", "state", "factIds", "explanation", "question")
	return map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "requirement_evidence", "strict": true, "schema": obj(map[string]any{"matches": map[string]any{"type": "array", "items": decision}}, "matches")}}
}

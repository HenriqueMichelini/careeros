package applicationdraft

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/qualificationmatching"
)

const draftContextBudget = 24 << 10

type contextSelection struct {
	Version           string   `json:"version"`
	Sources           []string `json:"sources"`
	BudgetExcluded    int      `json:"budgetExcluded"`
	RelevanceExcluded int      `json:"relevanceExcluded"`
	Complete          bool     `json:"complete"`
	Bytes             int      `json:"bytes"`
}
type contextUnit struct {
	refs  []string
	value any
	score int
}

// Lexical selection is a candidate heuristic, never semantic support. Keep
// negative and uncertain evidence conservatively, even without a lexical hit.
func relevance(value, job string) int {
	words := func(s string) map[string]bool {
		out := map[string]bool{}
		for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '+' && r != '#' }) {
			if len(w) > 1 {
				out[w] = true
			}
		}
		return out
	}
	terms := words(job)
	score := 0
	for w := range words(value) {
		if terms[w] {
			score++
		}
	}
	return score
}
func draftProjection(in request, job string) ([]any, contextSelection) {
	units := []contextUnit{}
	report := contextSelection{Version: "application-context-v1", Sources: []string{}, Complete: true}
	if in.ProfileEvidence != nil {
		doc := *in.ProfileEvidence
		// Connected components keep every role/project relationship and all owners'
		// facts together, including multi-context facts and contrasting statements.
		parent := map[string]string{}
		var root func(string) string
		root = func(id string) string {
			if parent[id] == "" {
				parent[id] = id
			}
			if parent[id] != id {
				parent[id] = root(parent[id])
			}
			return parent[id]
		}
		for _, f := range doc.Facts {
			for _, c := range f.Context {
				parent[root(c.ID)] = root(f.Owner.ID)
			}
		}
		groups := map[string][]profiledocument.Fact{}
		order := []string{}
		for _, f := range doc.Facts {
			key := root(f.Owner.ID)
			if f.Owner.ID == doc.ID && len(f.Context) == 0 {
				key = f.ID
			}
			if _, ok := groups[key]; !ok {
				order = append(order, key)
			}
			groups[key] = append(groups[key], f)
		}
		for _, key := range order {
			group := groups[key]
			refs := []string{}
			score := 0
			sub := doc
			sub.Facts = group
			// Reuse the approved evidence projection to resolve accepted excerpts.
			candidates := qualificationmatching.Select(sub, job, 0, 1<<30)
			for _, f := range group {
				refs = append(refs, fmt.Sprintf("%s/%s@%d", doc.ID, f.ID, f.Revision))
				score += relevance(string(f.Value), job)
				// Skills and formal qualifications are useful structured candidates even
				// when terminology differs. Their inclusion does not establish a match.
				if f.Field == "skills" || f.Field == "competencies" || f.Field == "tools" {
					score++
				}
				for _, e := range doc.Entities {
					if e.ID == f.Owner.ID && (e.Kind == "education" || e.Kind == "certifications" || e.Kind == "languages") {
						score++
					}
				}

				if f.Assertion == "negated" || f.Intent == "aspiration" || f.Certainty == "uncertain" || f.Support == "invalidated" {
					score += 10000
				}
			}
			units = append(units, contextUnit{refs, candidates.Facts, score})
		}
	} else {
		// Compatibility callers supply legacy section values. Only explicit
		// professional fields enter these units; contact/private fields never do.
		add := func(ref string, value any) {
			raw, _ := json.Marshal(value)
			if string(raw) == `""` {
				return
			}
			units = append(units, contextUnit{[]string{ref}, map[string]any{strings.Split(ref, "/")[0]: value}, relevance(string(raw), job)})
		}
		add("skills", in.Profile.Skills)
		add("competencies", in.Profile.Competencies)
		add("tools", in.Profile.Tools)
		for _, e := range in.Profile.Experience {
			add("experience/"+e.ID, map[string]any{"company": e.Company, "title": e.Title, "startDate": e.StartDate, "endDate": e.EndDate, "current": e.Current, "description": e.Description, "responsibilities": e.Responsibilities, "achievements": e.Achievements})
		}
		for _, p := range in.Profile.Projects {
			add("projects/"+p.ID, map[string]string{"name": p.Name, "description": p.Description, "technologies": p.Technologies, "highlights": p.Highlights})
		}
		if in.Qualifications != nil {
			for i, e := range in.Qualifications.Education {
				add(fmt.Sprintf("education/%d", i), map[string]string{"degree": e.Degree, "institution": e.Institution, "graduationDate": e.GraduationDate, "details": e.Details})
			}
			for i, c := range in.Qualifications.Certifications {
				add(fmt.Sprintf("certifications/%d", i), map[string]string{"name": c.Name, "issuer": c.Issuer, "date": c.Date})
			}
			for i, l := range in.Qualifications.Languages {
				add(fmt.Sprintf("languages/%d", i), l)
			}
		}
		// Legacy blocks have no typed assertion metadata: keep all conservatively
		// so that negation, limitations or ambiguous relationships are not lost.
		for i := range units {
			units[i].score += 1
		}
	}
	sort.SliceStable(units, func(i, j int) bool { return units[i].score > units[j].score })
	selected := []any{}
	for _, unit := range units {
		if unit.score == 0 {
			report.RelevanceExcluded += len(unit.refs)
			continue
		}
		candidate := map[string]any{"sources": unit.refs, "evidence": unit.value}
		raw, _ := json.Marshal(append(append([]any{}, selected...), candidate))
		if len(raw) > draftContextBudget {
			report.BudgetExcluded += len(unit.refs)
			continue
		}
		report.Bytes = len(raw)
		report.Sources = append(report.Sources, unit.refs...)
		selected = append(selected, candidate)
	}
	report.Complete = report.BudgetExcluded == 0
	return selected, report
}

// Package semanticeval scores curated synthetic outputs. Regex matches are
// bounded labels, not a general semantic judge or proof of factual safety.
package semanticeval

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Atom struct {
	ID       string            `json:"id"`
	Group    string            `json:"group"`
	Match    map[string]string `json:"match"`
	Optional bool              `json:"optional,omitempty"`
	Source   string            `json:"source,omitempty"`
}
type Case struct {
	ID               string            `json:"id"`
	Task             string            `json:"task"`
	Language         string            `json:"language"`
	Split            string            `json:"split"`
	Tags             []string          `json:"tags"`
	SourceID         string            `json:"sourceId,omitempty"`
	Request          json.RawMessage   `json:"request"`
	Provider         []json.RawMessage `json:"provider"`
	Gold             []Atom            `json:"gold"`
	Forbidden        []string          `json:"forbidden"`
	Notes            string            `json:"notes"`
	Entities         []string          `json:"entities,omitempty"`
	ExpectedFailures []string          `json:"expectedFailures,omitempty"`
	Role             string            `json:"role,omitempty"`
}
type Result struct {
	Expected              int                 `json:"expected"`
	Observed              int                 `json:"observed"`
	Matched               int                 `json:"matched"`
	Covered               int                 `json:"covered"`
	UnsupportedCandidates int                 `json:"unsupportedClaimCandidates"`
	Precision             float64             `json:"labelPrecision"`
	Completeness          float64             `json:"completeness"`
	Duplicates            int                 `json:"duplicates"`
	RelationshipErrors    int                 `json:"relationshipErrors"`
	ForbiddenAdditions    int                 `json:"forbiddenAdditions"`
	Missing               []string            `json:"missing"`
	Unmatched             []string            `json:"unmatched"`
	UnmatchedUnits        []map[string]string `json:"unmatchedUnits"`
	Failures              []string            `json:"failures"`
	HumanReview           string              `json:"humanReview"`
}

// Score uses one-to-one curated atoms per output group. Unmatched units count
// against label precision and require human adjudication; they are not silently
// called hallucinations. Every missing label remains individually visible.
func Score(c Case, raw json.RawMessage) (Result, error) {
	r := Result{Missing: []string{}, Unmatched: []string{}, UnmatchedUnits: []map[string]string{}, Failures: []string{}, HumanReview: "pending: unmatched units need source-grounded adjudication"}
	var output map[string]any
	if err := json.Unmarshal(raw, &output); err != nil {
		return r, err
	}
	groups := map[string]bool{}
	for _, a := range c.Gold {
		groups[a.Group] = true
		if !a.Optional {
			r.Expected++
		}
	}
	// Always observe all factual output groups, even when gold expects no units.
	for _, g := range []string{"claims", "operations", "gaps", "summary", "selected", "wording", "draft"} {
		if _, ok := output[g]; ok {
			groups[g] = true
		}
	}
	for _, field := range []string{"resume", "coverLetter", "applicationAnswers", "jobSummary", "jobTitle", "company"} {
		if _, ok := output[field]; ok {
			groups["draft"] = true
		}
	}
	names := make([]string, 0, len(groups))
	for g := range groups {
		names = append(names, g)
	}
	sort.Strings(names)
	used := map[string]bool{}
	for _, g := range names {
		units := outputUnits(g, output)
		for i, unit := range units {
			if g == "draft" {
				annotateEntities(unit, c.Entities)
			}
			r.Observed++
			found, duplicate, relationship := false, false, false
			for _, a := range c.Gold {
				if a.Group != g {
					continue
				}
				full, content := true, false
				for key, pattern := range a.Match {
					re, err := regexp.Compile("(?is)" + pattern)
					if err != nil {
						return r, fmt.Errorf("%s/%s: %w", c.ID, a.ID, err)
					}
					match := re.MatchString(unit[key])
					if !match {
						full = false
					}
					if (key == "text" || key == "value" || key == "requirement" || key == "content") && match {
						content = true
					}
				}
				if full && (!used[a.ID] || a.Optional) {
					used[a.ID] = true
					r.Matched++
					if !a.Optional {
						r.Covered++
					}
					found = true
					break
				}
				if full {
					duplicate = true
				}
				if content && !full {
					relationship = true
				}
			}
			if !found {
				id := fmt.Sprintf("%s[%d]", g, i)
				r.Unmatched = append(r.Unmatched, id)
				unit["location"] = id
				r.UnmatchedUnits = append(r.UnmatchedUnits, unit)
				if duplicate {
					r.Duplicates++
				} else if relationship {
					r.RelationshipErrors++
				} else {
					r.UnsupportedCandidates++
				}
			}
		}
	}
	for _, a := range c.Gold {
		if !used[a.ID] && !a.Optional {
			r.Missing = append(r.Missing, a.ID)
		}
	}
	for _, pattern := range c.Forbidden {
		re, err := regexp.Compile("(?is)" + pattern)
		if err != nil {
			return r, err
		}
		if re.Match(raw) {
			r.ForbiddenAdditions++
			r.Failures = append(r.Failures, "forbidden: "+pattern)
		}
	}
	r.Precision, r.Completeness = 1, 1
	if r.Observed > 0 {
		r.Precision = float64(r.Matched) / float64(r.Observed)
	}
	if r.Expected > 0 {
		r.Completeness = float64(r.Covered) / float64(r.Expected)
	}
	for _, id := range r.Missing {
		r.Failures = append(r.Failures, "missing: "+id)
	}
	for _, id := range r.Unmatched {
		r.Failures = append(r.Failures, "unmatched: "+id)
	}
	return r, nil
}

func outputUnits(group string, output map[string]any) []map[string]string {
	convert := func(v any) map[string]string {
		m := map[string]string{}
		if obj, ok := v.(map[string]any); ok {
			for k, value := range obj {
				if s, ok := value.(string); ok {
					m[k] = s
				} else {
					b, _ := json.Marshal(value)
					m[k] = string(b)
				}
			}
		} else {
			m["text"] = fmt.Sprint(v)
		}
		return m
	}
	if group == "draft" {
		return draftUnits(output)
	}
	if group == "wording" {
		var units []map[string]string
		if m, ok := output[group].(map[string]any); ok {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				units = append(units, map[string]string{"sourceId": k, "text": fmt.Sprint(m[k])})
			}
		}
		return units
	}
	var units []map[string]string
	if list, ok := output[group].([]any); ok {
		for _, v := range list {
			units = append(units, convert(v))
		}
	}
	return units
}

var proseBoundary = regexp.MustCompile(`[.!?](?:\s+|$)|;\s*|,\s*(?:with|com)\s+|(?:,\s*|\s+)(?:and|e|but|mas)\s+`)
var listPrefix = regexp.MustCompile(`^(?:[-*+]\s+|[0-9]+[.)]\s+)`)
var sectionHeading = regexp.MustCompile(`(?i)^(?:professional summary|technical skills|professional experience|education|certifications|languages|resumo profissional|resumo|competências técnicas|habilidades técnicas|experiência profissional|experiências profissionais|formação(?: acadêmica)?|educação|certificações|idiomas)$`)

// Draft prose is observed field by field, clause by clause. Full-clause labels
// cannot hide a new sentence behind a supported keyword elsewhere in a material.
// This deterministic segmenter is intentionally conservative, not a model judge.
func draftUnits(output map[string]any) []map[string]string {
	units := []map[string]string{}
	if cover, ok := output["coverLetter"].(map[string]any); ok {
		if greeting, ok := cover["greeting"].(string); ok {
			units = append(units, map[string]string{"field": "coverLetter.greeting", "text": greeting})
		}
	}
	for _, field := range []string{"jobTitle", "company"} {
		if value, ok := output[field]; ok {
			text := "null"
			if value != nil {
				text = fmt.Sprint(value)
			}
			units = append(units, map[string]string{"field": field, "text": text})
		}
	}
	for _, field := range []string{"jobSummary", "resume", "coverLetter", "applicationAnswers"} {
		value := output[field]
		if field == "coverLetter" {
			if obj, ok := value.(map[string]any); ok {
				value = obj["body"]
			}
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			heading := strings.HasPrefix(line, "#")
			line = strings.TrimSpace(strings.TrimLeft(line, "#"))
			line = listPrefix.ReplaceAllString(line, "")
			line = strings.ReplaceAll(strings.ReplaceAll(line, "**", ""), "`", "")
			if heading && sectionHeading.MatchString(line) {
				continue
			}
			for _, clause := range proseBoundary.Split(line, -1) {
				clause = strings.TrimSpace(strings.TrimRight(clause, ".!?"))
				if clause == "" {
					continue
				}
				units = append(units, map[string]string{"field": field, "text": clause})
			}
		}
	}
	return units
}

func annotateEntities(unit map[string]string, entities []string) {
	content := unit["text"]
	owners := []string{}
	for _, entity := range entities {
		pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(entity) + `\b`)
		if pattern.MatchString(content) {
			owners = append(owners, entity)
			content = pattern.ReplaceAllString(content, "{entity}")
		}
	}
	sort.Strings(owners)
	unit["owner"] = strings.Join(owners, "|")
	unit["content"] = content
}

// Validate rejects accidental split leakage, invalid labels and unlabeled cases.
func Validate(cases []Case) error {
	ids := map[string]bool{}
	requests := map[string]string{}
	for _, c := range cases {
		if c.ID == "" || ids[c.ID] || (c.Split != "development" && c.Split != "heldout") || (c.Language != "en" && c.Language != "pt-BR") || len(c.Gold) == 0 || strings.TrimSpace(c.Notes) == "" {
			return fmt.Errorf("invalid fixture %q", c.ID)
		}
		ids[c.ID] = true
		var request map[string]any
		if json.Unmarshal(c.Request, &request) != nil || request == nil {
			return fmt.Errorf("invalid request in %s", c.ID)
		}
		canonical, _ := json.Marshal(request)
		key := c.Task + string(canonical)
		if split, ok := requests[key]; ok && split != c.Split {
			return fmt.Errorf("request leaks across splits in %s", c.ID)
		}
		requests[key] = c.Split
		if c.Role != "" && c.Role != "acceptance" && c.Role != "negative_control" {
			return fmt.Errorf("unknown fixture role in %s", c.ID)
		}
		if c.Role == "negative_control" && len(c.ExpectedFailures) == 0 {
			return fmt.Errorf("unlabeled negative control %s", c.ID)
		}
		atoms := map[string]bool{}
		for _, a := range c.Gold {
			if a.ID == "" || atoms[a.ID] || len(a.Match) == 0 {
				return fmt.Errorf("invalid atom in %s", c.ID)
			}
			atoms[a.ID] = true
			if a.Group == "draft" && c.Role != "" {
				pattern := a.Match["text"]
				if pattern == "" {
					pattern = a.Match["content"]
				}
				if a.Source == "" || !strings.HasPrefix(pattern, "^") || !strings.HasSuffix(pattern, "$") {
					return fmt.Errorf("draft atom needs a source and full-clause label: %s/%s", c.ID, a.ID)
				}
			}
		}
		if _, err := Score(c, json.RawMessage(`{}`)); err != nil {
			return err
		}
	}
	return nil
}

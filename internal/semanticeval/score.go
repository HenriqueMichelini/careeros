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
	ID    string            `json:"id"`
	Group string            `json:"group"`
	Match map[string]string `json:"match"`
}
type Case struct {
	ID        string            `json:"id"`
	Task      string            `json:"task"`
	Language  string            `json:"language"`
	Split     string            `json:"split"`
	Tags      []string          `json:"tags"`
	SourceID  string            `json:"sourceId,omitempty"`
	Request   json.RawMessage   `json:"request"`
	Provider  []json.RawMessage `json:"provider"`
	Gold      []Atom            `json:"gold"`
	Forbidden []string          `json:"forbidden"`
	Notes     string            `json:"notes"`
}
type Result struct {
	Expected           int      `json:"expected"`
	Observed           int      `json:"observed"`
	Matched            int      `json:"matched"`
	Precision          float64  `json:"labelPrecision"`
	Completeness       float64  `json:"completeness"`
	Duplicates         int      `json:"duplicates"`
	RelationshipErrors int      `json:"relationshipErrors"`
	ForbiddenAdditions int      `json:"forbiddenAdditions"`
	Missing            []string `json:"missing"`
	Unmatched          []string `json:"unmatched"`
	Failures           []string `json:"failures"`
	HumanReview        string   `json:"humanReview"`
}

// Score uses one-to-one curated atoms per output group. Unmatched units count
// against label precision and require human adjudication; they are not silently
// called hallucinations. Every missing label remains individually visible.
func Score(c Case, raw json.RawMessage) (Result, error) {
	r := Result{Expected: len(c.Gold), Missing: []string{}, Unmatched: []string{}, Failures: []string{}, HumanReview: "pending: prose support and unmatched units"}
	var output map[string]any
	if err := json.Unmarshal(raw, &output); err != nil {
		return r, err
	}
	groups := map[string]bool{}
	for _, a := range c.Gold {
		groups[a.Group] = true
	}
	// Always observe all factual output groups, even when gold expects no units.
	for _, g := range []string{"claims", "operations", "gaps", "summary", "selected", "wording", "draft"} {
		if _, ok := output[g]; ok {
			groups[g] = true
		}
	}
	if _, ok := output["resume"]; ok {
		groups["draft"] = true
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
					if (key == "text" || key == "value" || key == "requirement") && match {
						content = true
					}
				}
				if full && !used[a.ID] {
					used[a.ID] = true
					r.Matched++
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
				if duplicate {
					r.Duplicates++
				} else if relationship {
					r.RelationshipErrors++
				}
			}
		}
	}
	for _, a := range c.Gold {
		if !used[a.ID] {
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
		r.Completeness = float64(r.Matched) / float64(r.Expected)
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
		return []map[string]string{convert(output)}
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

// Validate rejects accidental split leakage, invalid labels and unlabeled cases.
func Validate(cases []Case) error {
	ids := map[string]bool{}
	for _, c := range cases {
		if c.ID == "" || ids[c.ID] || (c.Split != "development" && c.Split != "heldout") || (c.Language != "en" && c.Language != "pt-BR") || len(c.Gold) == 0 || strings.TrimSpace(c.Notes) == "" {
			return fmt.Errorf("invalid fixture %q", c.ID)
		}
		ids[c.ID] = true
		atoms := map[string]bool{}
		for _, a := range c.Gold {
			if a.ID == "" || atoms[a.ID] || len(a.Match) == 0 {
				return fmt.Errorf("invalid atom in %s", c.ID)
			}
			atoms[a.ID] = true
		}
		if _, err := Score(c, json.RawMessage(`{}`)); err != nil {
			return err
		}
	}
	return nil
}

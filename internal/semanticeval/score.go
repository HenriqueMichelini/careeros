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

// Score matches curated output atoms one to one. Unmatched units require
// adjudication; they are not silently called hallucinations.
func Score(c Case, raw json.RawMessage) (Result, error) {
	result := Result{Missing: []string{}, Unmatched: []string{}, UnmatchedUnits: []map[string]string{}, Failures: []string{}, HumanReview: "pending: unmatched units need source-grounded adjudication"}
	var output map[string]any
	if err := json.Unmarshal(raw, &output); err != nil {
		return result, err
	}
	used := map[string]bool{}
	for _, group := range outputGroups(c, output) {
		for index, unit := range outputUnits(group, output) {
			if group == "draft" {
				annotateEntities(unit, c.Entities)
			}
			match, err := matchUnit(c.Gold, group, unit, used)
			if err != nil {
				return result, err
			}
			result.recordUnit(group, index, unit, match, used)
		}
	}
	return finishScore(c, raw, result, used)
}

func outputGroups(c Case, output map[string]any) []string {
	groups := map[string]bool{}
	for _, atom := range c.Gold {
		groups[atom.Group] = true
	}
	for _, group := range []string{"claims", "operations", "gaps", "summary", "selected", "wording", "draft"} {
		if _, ok := output[group]; ok {
			groups[group] = true
		}
	}
	for _, field := range []string{"resume", "coverLetter", "applicationAnswers", "jobSummary", "jobTitle", "company"} {
		if _, ok := output[field]; ok {
			groups["draft"] = true
		}
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type unitMatch struct {
	atomID                            string
	required, duplicate, relationship bool
}

func matchUnit(atoms []Atom, group string, unit map[string]string, used map[string]bool) (unitMatch, error) {
	result := unitMatch{}
	for _, atom := range atoms {
		if atom.Group != group {
			continue
		}
		full, content, err := matchFields(atom, unit)
		if err != nil {
			return result, err
		}
		if full && (!used[atom.ID] || atom.Optional) {
			return unitMatch{atomID: atom.ID, required: !atom.Optional}, nil
		}
		result.duplicate = result.duplicate || full
		result.relationship = result.relationship || (content && !full)
	}
	return result, nil
}
func matchFields(atom Atom, unit map[string]string) (bool, bool, error) {
	full, content := true, false
	for key, pattern := range atom.Match {
		expression, err := regexp.Compile("(?is)" + pattern)
		if err != nil {
			return false, false, fmt.Errorf("%s: %w", atom.ID, err)
		}
		matched := expression.MatchString(unit[key])
		full = full && matched
		content = content || (contentField(key) && matched)
	}
	return full, content, nil
}
func contentField(key string) bool {
	switch key {
	case "text", "value", "requirement", "content":
		return true
	default:
		return false
	}
}
func (r *Result) recordUnit(group string, index int, unit map[string]string, match unitMatch, used map[string]bool) {
	r.Observed++
	if match.atomID != "" {
		used[match.atomID] = true
		r.Matched++
		if match.required {
			r.Covered++
		}
		return
	}
	location := fmt.Sprintf("%s[%d]", group, index)
	r.Unmatched = append(r.Unmatched, location)
	unit["location"] = location
	r.UnmatchedUnits = append(r.UnmatchedUnits, unit)
	switch {
	case match.duplicate:
		r.Duplicates++
	case match.relationship:
		r.RelationshipErrors++
	default:
		r.UnsupportedCandidates++
	}
}
func finishScore(c Case, raw json.RawMessage, result Result, used map[string]bool) (Result, error) {
	for _, atom := range c.Gold {
		if atom.Optional {
			continue
		}
		result.Expected++
		if !used[atom.ID] {
			result.Missing = append(result.Missing, atom.ID)
		}
	}
	for _, pattern := range c.Forbidden {
		expression, err := regexp.Compile("(?is)" + pattern)
		if err != nil {
			return result, err
		}
		if expression.Match(raw) {
			result.ForbiddenAdditions++
			result.Failures = append(result.Failures, "forbidden: "+pattern)
		}
	}
	result.Precision = ratio(result.Matched, result.Observed)
	result.Completeness = ratio(result.Covered, result.Expected)
	for _, id := range result.Missing {
		result.Failures = append(result.Failures, "missing: "+id)
	}
	for _, id := range result.Unmatched {
		result.Failures = append(result.Failures, "unmatched: "+id)
	}
	return result, nil
}
func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 1
	}
	return float64(numerator) / float64(denominator)
}

func outputUnits(group string, output map[string]any) []map[string]string {
	switch group {
	case "draft":
		return draftUnits(output)
	case "wording":
		return wordingUnits(output[group])
	}
	units := []map[string]string{}
	if list, ok := output[group].([]any); ok {
		for _, value := range list {
			units = append(units, valueFields(value))
		}
	}
	return units
}
func valueFields(value any) map[string]string {
	fields := map[string]string{}
	object, ok := value.(map[string]any)
	if !ok {
		fields["text"] = fmt.Sprint(value)
		return fields
	}
	for key, value := range object {
		if text, ok := value.(string); ok {
			fields[key] = text
			continue
		}
		encoded, _ := json.Marshal(value)
		fields[key] = string(encoded)
	}
	return fields
}
func wordingUnits(value any) []map[string]string {
	units := []map[string]string{}
	wording, ok := value.(map[string]any)
	if !ok {
		return units
	}
	keys := make([]string, 0, len(wording))
	for key := range wording {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		units = append(units, map[string]string{"sourceId": key, "text": fmt.Sprint(wording[key])})
	}
	return units
}

var proseBoundary = regexp.MustCompile(`[.!?](?:\s+|$)|;\s*|,\s*(?:with|com)\s+|(?:,\s*|\s+)(?:and|e|but|mas)\s+`)
var listPrefix = regexp.MustCompile(`^(?:[-*+]\s+|[0-9]+[.)]\s+)`)
var sectionHeading = regexp.MustCompile(`(?i)^(?:professional summary|technical skills|professional experience|education|certifications|languages|resumo profissional|resumo|competências técnicas|habilidades técnicas|experiência profissional|experiências profissionais|formação(?: acadêmica)?|educação|certificações|idiomas)$`)

// Draft prose is observed field by field, clause by clause. Full-clause labels
// cannot hide a new sentence behind a supported keyword elsewhere in a material.
func draftUnits(output map[string]any) []map[string]string {
	units := draftMetadata(output)
	for _, field := range []string{"jobSummary", "resume", "coverLetter", "applicationAnswers"} {
		for _, clause := range proseClauses(draftText(field, output)) {
			units = append(units, map[string]string{"field": field, "text": clause})
		}
	}
	return units
}
func draftMetadata(output map[string]any) []map[string]string {
	units := []map[string]string{}
	if cover, ok := output["coverLetter"].(map[string]any); ok {
		if greeting, ok := cover["greeting"].(string); ok {
			units = append(units, map[string]string{"field": "coverLetter.greeting", "text": greeting})
		}
	}
	for _, field := range []string{"jobTitle", "company"} {
		value, ok := output[field]
		if !ok {
			continue
		}
		text := "null"
		if value != nil {
			text = fmt.Sprint(value)
		}
		units = append(units, map[string]string{"field": field, "text": text})
	}
	return units
}
func draftText(field string, output map[string]any) string {
	value := output[field]
	if field == "coverLetter" {
		if object, ok := value.(map[string]any); ok {
			value = object["body"]
		}
	}
	text, _ := value.(string)
	return text
}
func proseClauses(text string) []string {
	clauses := []string{}
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
			if clause != "" {
				clauses = append(clauses, clause)
			}
		}
	}
	return clauses
}
func annotateEntities(unit map[string]string, entities []string) {
	content := unit["text"]
	owners := []string{}
	for _, entity := range entities {
		expression := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(entity) + `\b`)
		if expression.MatchString(content) {
			owners = append(owners, entity)
			content = expression.ReplaceAllString(content, "{entity}")
		}
	}
	sort.Strings(owners)
	unit["owner"] = strings.Join(owners, "|")
	unit["content"] = content
}

// Validate rejects invalid labels, unlabeled controls and identical requests
// appearing on both sides of the development/held-out split before any inference.
func Validate(cases []Case) error {
	ids := map[string]bool{}
	requests := map[string]string{}
	for _, c := range cases {
		if ids[c.ID] {
			return fmt.Errorf("duplicate fixture %q", c.ID)
		}
		ids[c.ID] = true
		if err := validateCase(c); err != nil {
			return err
		}
		key, err := requestKey(c)
		if err != nil {
			return err
		}
		if split, exists := requests[key]; exists && split != c.Split {
			return fmt.Errorf("request leaks across splits in %s", c.ID)
		}
		requests[key] = c.Split
	}
	return nil
}
func validateCase(c Case) error {
	if c.ID == "" || len(c.Gold) == 0 || strings.TrimSpace(c.Notes) == "" {
		return fmt.Errorf("invalid fixture %q", c.ID)
	}
	if !oneOf(c.Split, "development", "heldout") || !oneOf(c.Language, "en", "pt-BR") {
		return fmt.Errorf("invalid split or language in %s", c.ID)
	}
	if !oneOf(c.Role, "", "acceptance", "negative_control") {
		return fmt.Errorf("unknown fixture role in %s", c.ID)
	}
	if c.Role == "negative_control" && len(c.ExpectedFailures) == 0 {
		return fmt.Errorf("unlabeled negative control %s", c.ID)
	}
	atoms := map[string]bool{}
	for _, atom := range c.Gold {
		if atom.ID == "" || atoms[atom.ID] || len(atom.Match) == 0 {
			return fmt.Errorf("invalid atom in %s", c.ID)
		}
		atoms[atom.ID] = true
		if err := validateAtom(c, atom); err != nil {
			return err
		}
	}
	return validatePatterns(c.Forbidden)
}
func validateAtom(c Case, atom Atom) error {
	patterns := []string{}
	for _, pattern := range atom.Match {
		patterns = append(patterns, pattern)
	}
	if err := validatePatterns(patterns); err != nil {
		return fmt.Errorf("%s/%s: %w", c.ID, atom.ID, err)
	}
	if atom.Group != "draft" || c.Role == "" {
		return nil
	}
	pattern := atom.Match["text"]
	if pattern == "" {
		pattern = atom.Match["content"]
	}
	if atom.Source == "" || !strings.HasPrefix(pattern, "^") || !strings.HasSuffix(pattern, "$") {
		return fmt.Errorf("draft atom needs a source and full-clause label: %s/%s", c.ID, atom.ID)
	}
	return nil
}
func validatePatterns(patterns []string) error {
	for _, pattern := range patterns {
		if _, err := regexp.Compile("(?is)" + pattern); err != nil {
			return err
		}
	}
	return nil
}
func requestKey(c Case) (string, error) {
	var request map[string]any
	if json.Unmarshal(c.Request, &request) != nil || request == nil {
		return "", fmt.Errorf("invalid request in %s", c.ID)
	}
	encoded, _ := json.Marshal(request)
	return c.Task + string(encoded), nil
}
func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

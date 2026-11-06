// Package cvsource projects the exact approved, indivisible CV source view.
// It intentionally excludes evidence bodies, private fields and compatibility
// splitting. The browser retains evidence locally; it is not a provider input.
package cvsource

import (
	"bytes"
	"encoding/json"
	"io"
	"professional-information-repo/internal/profiledocument"
	"reflect"
	"strconv"
)

// Normalize removes legacy spelling/default aliases before assigning an Action
// root. They cannot mint a fresh binding for the same unresolved candidate.
func Normalize(raw []byte) ([]byte, bool) {
	var in struct {
		Locale   string          `json:"locale,omitempty"`
		Language string          `json:"cvLanguage"`
		Density  string          `json:"density"`
		Assessed bool            `json:"statementSupport"`
		Facts    json.RawMessage `json:"facts"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || !in.Assessed {
		return nil, false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, false
	}
	if in.Language == "" {
		in.Language = in.Locale
	}
	if in.Locale != "" && in.Language != in.Locale {
		return nil, false
	}
	if in.Language != "en" && in.Language != "pt-BR" {
		return nil, false
	}
	if in.Density == "" {
		in.Density = "balanced"
	}
	if in.Density != "compact" && in.Density != "balanced" && in.Density != "detailed" {
		return nil, false
	}
	in.Locale = ""
	out, e := json.Marshal(in)
	return out, e == nil
}

type Fact struct {
	ID        string                      `json:"id"`
	Section   string                      `json:"section"`
	EntryID   string                      `json:"entryId"`
	Field     string                      `json:"field"`
	Text      string                      `json:"text"`
	Reference profiledocument.Reference   `json:"reference"`
	Owner     profiledocument.Reference   `json:"owner"`
	Context   []profiledocument.Reference `json:"context"`
	Kind      string                      `json:"kind"`
	Assertion string                      `json:"assertion"`
	Intent    string                      `json:"intent"`
	Certainty string                      `json:"certainty"`
	Support   string                      `json:"support"`
}

var allowed = map[string]map[string]bool{
	"skills": {"skills": true, "competencies": true}, "tools": {"tools": true},
	"experience":     {"title": true, "company": true, "startDate": true, "endDate": true, "current": true, "description": true, "responsibilities": true, "achievements": true},
	"projects":       {"name": true, "description": true, "technologies": true, "highlights": true},
	"education":      {"degree": true, "institution": true, "graduationDate": true, "details": true},
	"certifications": {"name": true, "issuer": true, "date": true}, "languages": {"name": true, "proficiency": true},
}

func Facts(doc profiledocument.Document) []Fact {
	out := []Fact{}
	groups := map[string]string{}
	for _, f := range doc.Facts {
		section, entry := f.Field, ""
		if f.Field == "competencies" {
			section = "skills"
		}
		for _, e := range doc.Entities {
			if e.ID == f.Owner.ID {
				section, entry = e.Kind, e.LegacyID
				break
			}
		}
		var value any
		_ = json.Unmarshal(f.Value, &value)
		text, _ := value.(string)
		if value == true {
			text = "true"
		}
		if !allowed[section][f.Field] || !profiledocument.HasText(text) {
			continue
		}
		if entry != "" {
			group := section + "/" + entry
			if _, ok := groups[group]; !ok {
				groups[group] = "entry" + strconv.Itoa(len(groups))
			}
			entry = groups[group]
		}
		context := append([]profiledocument.Reference{}, f.Context...)
		out = append(out, Fact{f.ID, section, entry, f.Field, text, profiledocument.Reference{ProfileID: doc.ID, ID: f.ID, Revision: f.Revision}, f.Owner, context, f.Kind, f.Assertion, f.Intent, f.Certainty, f.Support})
	}
	return out
}

// Matches requires the complete selected stable source view. A partial slice,
// reordered reference, invented evidence or a stale qualifier is not authority.
func Matches(root, saved []byte) bool {
	doc, err := profiledocument.Decode(saved)
	if err != nil {
		return false
	}
	var input struct {
		Facts json.RawMessage `json:"facts"`
	}
	if json.Unmarshal(root, &input) != nil {
		return false
	}
	var actual, expected any
	raw, _ := json.Marshal(Facts(doc))
	return json.Unmarshal(input.Facts, &actual) == nil && json.Unmarshal(raw, &expected) == nil && reflect.DeepEqual(actual, expected)
}

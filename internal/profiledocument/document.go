// Package profiledocument defines the browser-local Profile v2 contract. It is
// intentionally not wired to storage or providers in the stateless backend.
package profiledocument

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
)

//go:embed contract.json
var contractJSON []byte

const NormalizationPolicy = "exact-alias-v1"

type Reference struct {
	ProfileID string `json:"profileId"`
	ID        string `json:"id"`
	Revision  int64  `json:"revision"`
}
type Origin struct {
	Kind     string `json:"kind"`
	Original string `json:"original"`
}
type Entity struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	Kind     string `json:"kind"`
	LegacyID string `json:"legacyId"`
	Order    int64  `json:"order"`
}
type Temporal struct {
	Wording   string `json:"wording"`
	Precision string `json:"precision"`
}
type Normalization struct {
	Observed  string  `json:"observed"`
	Canonical *string `json:"canonical"`
	Policy    string  `json:"policy"`
}
type Fact struct {
	ID            string          `json:"id"`
	Revision      int64           `json:"revision"`
	Owner         Reference       `json:"owner"`
	Context       []Reference     `json:"context"`
	Field         string          `json:"field"`
	Order         int64           `json:"order"`
	Kind          string          `json:"kind"`
	Value         json.RawMessage `json:"value"`
	Assertion     string          `json:"assertion"`
	Intent        string          `json:"intent"`
	Certainty     string          `json:"certainty"`
	Temporal      Temporal        `json:"temporal"`
	Normalization Normalization   `json:"normalization"`
	Origin        Origin          `json:"origin"`
	Approval      string          `json:"approval"`
	Support       string          `json:"support"`
}
type Evidence struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	Excerpt  string `json:"excerpt"`
	Origin   string `json:"origin"`
	Approval string `json:"approval"`
}
type Link struct {
	ID    string    `json:"id"`
	Kind  string    `json:"kind"`
	From  Reference `json:"from"`
	To    Reference `json:"to"`
	State string    `json:"state"`
}
type Document struct {
	Version             int        `json:"version"`
	ID                  string     `json:"id"`
	Revision            int64      `json:"revision"`
	NormalizationPolicy string     `json:"normalizationPolicy"`
	Entities            []Entity   `json:"entities"`
	Facts               []Fact     `json:"facts"`
	Evidence            []Evidence `json:"evidence"`
	Links               []Link     `json:"links"`
}

var entityFields = map[string][]string{
	"experience":     {"company", "title", "startDate", "endDate", "current", "location", "description", "responsibilities", "achievements"},
	"projects":       {"name", "description", "technologies", "url", "highlights"},
	"education":      {"degree", "institution", "location", "graduationDate", "details"},
	"certifications": {"name", "issuer", "date", "credentialId", "url"},
	"languages":      {"name", "proficiency"},
}
var profileFields = []string{"fullName", "email", "phone", "location", "professionalLinks", "careerGoals", "skills", "competencies", "tools", "employmentStatus", "currentSalary", "desiredSalary", "additionalInfo"}

// Decode validates wire shape and same-Profile, revision-scoped domain links.
// No provider calls, persistence, inferred dates or terminology expansion occur.
func Decode(raw []byte) (Document, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return Document{}, err
	}
	var schema map[string]any
	if err := json.Unmarshal(contractJSON, &schema); err != nil {
		return Document{}, err
	}
	if !validShape(value, schema) {
		return Document{}, fmt.Errorf("invalid Profile v2 shape")
	}
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Document{}, err
	}
	if !doc.validReferences() {
		return Document{}, fmt.Errorf("invalid Profile v2 references or semantics")
	}
	return doc, nil
}

func NormalizeTerm(observed string) Normalization {
	aliases := map[string]string{"JavaScript": "JavaScript", "Javascript": "JavaScript", "TypeScript": "TypeScript", "Typescript": "TypeScript"}
	n := Normalization{Observed: observed, Policy: NormalizationPolicy}
	if canonical, ok := aliases[observed]; ok {
		n.Canonical = &canonical
	}
	return n
}

type node struct {
	revision int64
	kind     string
}

func (d Document) validReferences() bool {
	nodes := map[string]node{d.ID: {d.Revision, "profile"}}
	add := func(id string, revision int64, kind string) bool {
		if _, exists := nodes[id]; exists {
			return false
		}
		nodes[id] = node{revision, kind}
		return true
	}
	positions := map[string]map[int64]bool{}
	identities := map[string]map[string]bool{}
	for _, e := range d.Entities {
		if !add(e.ID, e.Revision, e.Kind) {
			return false
		}
		if positions[e.Kind] == nil {
			positions[e.Kind] = map[int64]bool{}
			identities[e.Kind] = map[string]bool{}
		}
		if positions[e.Kind][e.Order] || identities[e.Kind][e.LegacyID] {
			return false
		}
		positions[e.Kind][e.Order] = true
		identities[e.Kind][e.LegacyID] = true
	}
	for _, f := range d.Facts {
		if !add(f.ID, f.Revision, "fact") {
			return false
		}
	}
	for _, e := range d.Evidence {
		if !add(e.ID, e.Revision, "evidence") {
			return false
		}
	}
	ref := func(r Reference, kinds ...string) bool {
		n, ok := nodes[r.ID]
		if !ok || r.ProfileID != d.ID || r.Revision != n.revision {
			return false
		}
		for _, k := range kinds {
			if n.kind == k {
				return true
			}
		}
		return false
	}
	entityKinds := []string{"experience", "projects", "education", "certifications", "languages"}
	slots := map[string]map[string]bool{}
	for _, f := range d.Facts {
		if !ref(f.Owner, append([]string{"profile"}, entityKinds...)...) || f.Field == "id" {
			return false
		}
		if slots[f.Owner.ID] == nil {
			slots[f.Owner.ID] = map[string]bool{}
		}
		if slots[f.Owner.ID][f.Field] {
			return false
		}
		slots[f.Owner.ID][f.Field] = true
		for _, c := range f.Context {
			if !ref(c, entityKinds...) {
				return false
			}
		}
		if f.Normalization.Canonical != nil {
			normalized := NormalizeTerm(f.Normalization.Observed)
			if f.Kind != "statement" || normalized.Canonical == nil || *normalized.Canonical != *f.Normalization.Canonical {
				return false
			}
		}
		if !(f.Origin.Kind == "existing_profile" && f.Origin.Original == "unknown" || f.Origin.Kind == "manual_edit" && f.Origin.Original == "user" || (f.Origin.Kind == "accepted_proposal" || f.Origin.Kind == "ai_review") && f.Origin.Original == "unknown") {
			return false
		}
		kind := nodes[f.Owner.ID].kind
		fields := entityFields[kind]
		if kind == "profile" {
			fields = profileFields
			if _, group := entityFields[f.Field]; group {
				return false
			}
		}
		for _, field := range fields {
			if f.Field != field {
				continue
			}
			var v any
			if json.Unmarshal(f.Value, &v) != nil {
				return false
			}
			if field == "current" {
				if _, ok := v.(bool); !ok {
					return false
				}
			} else if _, ok := v.(string); !ok {
				return false
			}
		}
	}
	linkIDs := map[string]bool{}
	for _, l := range d.Links {
		if _, collision := nodes[l.ID]; collision || linkIDs[l.ID] {
			return false
		}
		linkIDs[l.ID] = true
		if l.State == "invalidated" {
			for _, r := range []Reference{l.From, l.To} {
				if r.ProfileID != d.ID {
					return false
				}
			}
			continue
		}
		kinds := entityKinds
		switch l.Kind {
		case "supports":
			kinds = []string{"evidence"}
		case "role_context":
			kinds = []string{"experience"}
		case "project_context":
			kinds = []string{"projects"}
		}
		if !ref(l.From, "fact") || !ref(l.To, kinds...) {
			return false
		}
	}
	for _, f := range d.Facts {
		if f.Support != "supported" {
			continue
		}
		if f.Approval != "approved" {
			return false
		}
		supported := false
		for _, l := range d.Links {
			if l.Kind == "supports" && l.State == "active" && l.From.ID == f.ID && l.From.Revision == f.Revision {
				supported = true
			}
		}
		if !supported {
			return false
		}
	}
	return true
}

func validShape(value any, rule map[string]any) bool {
	matches := func(kind string) bool {
		switch kind {
		case "null":
			return value == nil
		case "object":
			_, ok := value.(map[string]any)
			return ok
		case "array":
			_, ok := value.([]any)
			return ok
		case "string":
			_, ok := value.(string)
			return ok
		case "integer":
			n, ok := value.(float64)
			return ok && math.Trunc(n) == n && math.Abs(n) <= 9007199254740991
		}
		return false
	}
	if kind, exists := rule["type"]; exists {
		ok := false
		if name, single := kind.(string); single {
			ok = matches(name)
		} else {
			for _, name := range kind.([]any) {
				ok = ok || matches(name.(string))
			}
		}
		if !ok {
			return false
		}
	}
	if choices, ok := rule["enum"].([]any); ok {
		found := false
		for _, choice := range choices {
			a, _ := json.Marshal(value)
			b, _ := json.Marshal(choice)
			if bytes.Equal(a, b) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if minimum, ok := rule["minimum"].(float64); ok && value.(float64) < minimum {
		return false
	}
	if minimum, ok := rule["minLength"].(float64); ok && len(value.(string)) < int(minimum) {
		return false
	}
	if properties, ok := rule["properties"].(map[string]any); ok {
		object := value.(map[string]any)
		for _, key := range rule["required"].([]any) {
			if _, exists := object[key.(string)]; !exists {
				return false
			}
		}
		if rule["additionalProperties"] == false {
			for key := range object {
				if _, exists := properties[key]; !exists {
					return false
				}
			}
		}
		for key, child := range properties {
			if !validShape(object[key], child.(map[string]any)) {
				return false
			}
		}
	}
	if items, ok := rule["items"].(map[string]any); ok {
		for _, item := range value.([]any) {
			if !validShape(item, items) {
				return false
			}
		}
	}
	return true
}

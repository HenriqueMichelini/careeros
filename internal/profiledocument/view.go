package profiledocument

import (
	"encoding/json"
	"sort"
)

// View materializes the complete compatibility Profile without filtering facts,
// shortening text or discarding custom fields. Defaults mirror profileView in
// the client; authority still belongs to the canonical saved document.
func View(doc Document) map[string]any {
	view := map[string]any{}
	for _, name := range []string{"fullName", "email", "phone", "location", "professionalLinks", "careerGoals", "skills", "competencies", "tools", "employmentStatus", "currentSalary", "desiredSalary", "additionalInfo"} {
		view[name] = ""
	}
	fields := func(id string, target map[string]any) {
		facts := append([]Fact(nil), doc.Facts...)
		sort.SliceStable(facts, func(i, j int) bool { return facts[i].Order < facts[j].Order })
		for _, f := range facts {
			if f.Owner.ID == id {
				var value any
				_ = json.Unmarshal(f.Value, &value)
				target[f.Field] = value
			}
		}
	}
	fields(doc.ID, view)
	groups := map[string][]string{
		"experience":     {"company", "title", "startDate", "endDate", "current", "location", "description", "responsibilities", "achievements"},
		"projects":       {"name", "description", "technologies", "url", "highlights"},
		"education":      {"degree", "institution", "location", "graduationDate", "details"},
		"certifications": {"name", "issuer", "date", "credentialId", "url"},
		"languages":      {"name", "proficiency"},
	}
	entities := append([]Entity(nil), doc.Entities...)
	sort.SliceStable(entities, func(i, j int) bool { return entities[i].Order < entities[j].Order })
	for group, names := range groups {
		items := []any{}
		for _, e := range entities {
			if e.Kind == group {
				item := map[string]any{}
				for _, name := range names {
					item[name] = ""
					if name == "current" {
						item[name] = false
					}
				}
				fields(e.ID, item)
				item["id"] = e.LegacyID
				items = append(items, item)
			}
		}
		view[group] = items
	}
	return view
}

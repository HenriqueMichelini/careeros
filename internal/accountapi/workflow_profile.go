package accountapi

import (
	"encoding/json"
	"sort"

	"professional-information-repo/internal/profiledocument"
)

// Mirrors the established Profile view and careerProfile/cvQualifications wire
// contracts. Contact fields remain application-owned and outside AI payloads.
func workflowProfile(doc profiledocument.Document) (map[string]any, map[string]any) {
	fields := func(id string) map[string]any {
		result := map[string]any{}
		for _, f := range doc.Facts {
			if f.Owner.ID == id {
				var value any
				_ = json.Unmarshal(f.Value, &value)
				result[f.Field] = value
			}
		}
		return result
	}
	repo := map[string]any{}
	root := fields(doc.ID)
	for _, field := range []string{"careerGoals", "skills", "competencies", "tools", "employmentStatus", "currentSalary", "desiredSalary", "additionalInfo"} {
		repo[field] = ""
		if v, ok := root[field]; ok {
			repo[field] = v
		}
	}
	groups := map[string][]string{
		"experience":     {"company", "title", "startDate", "endDate", "current", "location", "description", "responsibilities", "achievements"},
		"projects":       {"name", "description", "technologies", "url", "highlights"},
		"education":      {"degree", "institution", "location", "graduationDate", "details"},
		"certifications": {"name", "issuer", "date", "credentialId", "url"},
		"languages":      {"name", "proficiency"},
	}
	qualifications := map[string]any{}
	entities := append([]profiledocument.Entity(nil), doc.Entities...)
	sort.SliceStable(entities, func(i, j int) bool { return entities[i].Order < entities[j].Order })
	for group, names := range groups {
		items := []map[string]any{}
		for _, e := range entities {
			if e.Kind == group {
				item := map[string]any{}
				values := fields(e.ID)
				for _, name := range names {
					item[name] = ""
					if name == "current" {
						item[name] = false
					}
				}
				// profileView keeps opaque/custom fields in the legacy
				// view. Evidence allowlisting and workflow input bounds are separate.
				for name, value := range values {
					item[name] = value
				}
				item["id"] = e.LegacyID
				text := func(key string) bool { v, _ := item[key].(string); return profiledocument.HasText(v) }
				if group == "education" && !text("degree") && !text("institution") {
					continue
				}
				if (group == "certifications" || group == "languages") && !text("name") {
					continue
				}
				items = append(items, item)
			}
		}
		if group == "experience" || group == "projects" {
			repo[group] = items
		} else {
			qualifications[group] = items
		}
	}
	return repo, qualifications
}

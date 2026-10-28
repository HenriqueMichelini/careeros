package accountapi

import (
	"encoding/json"
	"professional-information-repo/internal/profiledocument"
	"sort"
	"strings"
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
				item := map[string]any{"id": e.LegacyID}
				values := fields(e.ID)
				for _, name := range names {
					item[name] = ""
					if name == "current" {
						item[name] = false
					}
					if v, ok := values[name]; ok {
						item[name] = v
					}
				}
				text := func(key string) string { v, _ := item[key].(string); return strings.TrimSpace(v) }
				if group == "education" && text("degree") == "" && text("institution") == "" {
					continue
				}
				if (group == "certifications" || group == "languages") && text("name") == "" {
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

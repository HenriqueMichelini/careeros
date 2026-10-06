package profilevalidation

import "encoding/json"

type Education struct {
	ID             string `json:"id"`
	Degree         string `json:"degree"`
	Institution    string `json:"institution"`
	Location       string `json:"location"`
	GraduationDate string `json:"graduationDate"`
	Details        string `json:"details"`
}
type Certification struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Issuer       string `json:"issuer"`
	Date         string `json:"date"`
	CredentialID string `json:"credentialId"`
	URL          string `json:"url"`
}
type Language struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Proficiency string `json:"proficiency"`
}
type Qualifications struct {
	Education      []Education     `json:"education"`
	Certifications []Certification `json:"certifications"`
	Languages      []Language      `json:"languages"`
}

func CompleteQualifications(raw json.RawMessage) bool {
	var groups map[string]json.RawMessage
	if json.Unmarshal(raw, &groups) != nil || len(groups) != 3 {
		return false
	}
	for _, group := range []struct {
		name   string
		fields []string
	}{
		{"education", []string{"id", "degree", "institution", "location", "graduationDate", "details"}},
		{"certifications", []string{"id", "name", "issuer", "date", "credentialId", "url"}},
		{"languages", []string{"id", "name", "proficiency"}},
	} {
		if !jsonType(groups[group.name], '[') {
			return false
		}
		var items []map[string]json.RawMessage
		if json.Unmarshal(groups[group.name], &items) != nil {
			return false
		}
		for _, item := range items {
			if len(item) != len(group.fields) {
				return false
			}
			for _, field := range group.fields {
				if !jsonType(item[field], '"') {
					return false
				}
			}
		}
	}
	return true
}

func ValidQualifications(q Qualifications) bool {
	if len(q.Education) > 40 || len(q.Certifications) > 40 || len(q.Languages) > 40 {
		return false
	}
	ids := make(map[string]bool)
	validID := func(id string) bool {
		if id == "" || len(id) > 100 || ids[id] {
			return false
		}
		ids[id] = true
		return true
	}
	for _, e := range q.Education {
		if !validID(e.ID) || !bounded(2000, e.Degree, e.Institution, e.Location, e.GraduationDate, e.Details) {
			return false
		}
	}
	for _, c := range q.Certifications {
		if !validID(c.ID) || !bounded(2000, c.Name, c.Issuer, c.Date, c.CredentialID, c.URL) {
			return false
		}
	}
	for _, l := range q.Languages {
		if !validID(l.ID) || !bounded(2000, l.Name, l.Proficiency) {
			return false
		}
	}
	return true
}

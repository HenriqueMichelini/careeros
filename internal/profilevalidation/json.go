package profilevalidation

import (
	"encoding/json"
	"strings"
)

// CompleteJSON checks the exact persisted Profile shape before decoding.
// Unmarshal into a struct alone accepts missing fields and null strings.
func CompleteJSON(raw json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 10 {
		return false
	}
	for _, name := range []string{"careerGoals", "skills", "competencies", "tools", "employmentStatus", "currentSalary", "desiredSalary", "additionalInfo"} {
		if !jsonType(fields[name], '"') {
			return false
		}
	}
	for _, collection := range []struct {
		name    string
		fields  []string
		boolean string
	}{
		{"experience", []string{"id", "company", "title", "startDate", "endDate", "current", "location", "description", "responsibilities", "achievements"}, "current"},
		{"projects", []string{"id", "name", "description", "technologies", "url", "highlights"}, ""},
	} {
		if !jsonType(fields[collection.name], '[') {
			return false
		}
		var items []map[string]json.RawMessage
		if json.Unmarshal(fields[collection.name], &items) != nil {
			return false
		}
		for _, item := range items {
			if len(item) != len(collection.fields) {
				return false
			}
			for _, name := range collection.fields {
				if name == collection.boolean {
					value := strings.TrimSpace(string(item[name]))
					if value != "true" && value != "false" {
						return false
					}
				} else if !jsonType(item[name], '"') {
					return false
				}
			}
		}
	}
	return true
}
func jsonType(raw json.RawMessage, first byte) bool {
	value := strings.TrimSpace(string(raw))
	return len(value) > 0 && value[0] == first
}

package releasecapture

import (
	"encoding/json"
	"net/http"
	"slices"
)

// These contracts inspect the actual serialized bytes before each dispatch.
// Source admission comes from the exact approved browser request and the existing
// production projections/validators; neither hashes nor these checks prove meaning.
func (c *collector) allowedEnvelope(r *http.Request, stage string, raw []byte) bool {
	if r.Method != http.MethodPost || r.URL.Scheme != "https" || r.URL.User != nil || r.URL.RawQuery != "" || r.URL.Fragment != "" {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return false
	}
	if stage == "field_validation" {
		if r.URL.Host != "api.typesafe.ai" || r.URL.Path != "/v1/systemone" || !exactFields(object, "model", "state", "questions") || string(object["model"]) != `"jev-1.13.0"` {
			return false
		}
		var state map[string]json.RawMessage
		if json.Unmarshal(object["state"], &state) != nil || !exactFields(state, "field", "submission") || string(state["field"]) != `"job_posting"` {
			return false
		}
		var submission string
		return json.Unmarshal(state["submission"], &submission) == nil && submission == c.posting
	}
	if r.URL.Host != "api.openai.com" || r.URL.Path != "/v1/chat/completions" || !exactFields(object, "model", "reasoning_effort", "max_completion_tokens", "response_format", "messages") || string(object["model"]) != `"gpt-6-luna"` || string(object["reasoning_effort"]) != `"none"` {
		return false
	}
	return c.allowedOpenAI(stage, object)
}

func (c *collector) allowedOpenAI(stage string, object map[string]json.RawMessage) bool {
	ceiling, schema, roles := 8000, "", []string{"system", "user"}
	switch stage {
	case "generation":
		schema = "application_draft"
	case "resume_support":
		ceiling, schema = 4000, "resume_support"
	case "artifact_support":
		schema = "artifact_support"
	case "qualification_matching":
		roles = []string{"user"}
		if c.understandJob {
			schema = "qualification_job_context"
		} else {
			ceiling = 1200
		}
	case "requirement_evidence":
		roles, schema = []string{"user"}, "requirement_evidence"
	default:
		return false
	}
	var actualCeiling int
	if json.Unmarshal(object["max_completion_tokens"], &actualCeiling) != nil || actualCeiling != ceiling || !allowedFormat(object["response_format"], schema) {
		return false
	}
	var messages []map[string]json.RawMessage
	if json.Unmarshal(object["messages"], &messages) != nil || len(messages) != len(roles) {
		return false
	}
	for i, message := range messages {
		var role, content string
		if !exactFields(message, "role", "content") || json.Unmarshal(message["role"], &role) != nil || role != roles[i] || json.Unmarshal(message["content"], &content) != nil || content == "" {
			return false
		}
	}
	if len(roles) == 1 {
		return true
	}
	var content string
	_ = json.Unmarshal(messages[1]["content"], &content)
	var data map[string]json.RawMessage
	if json.Unmarshal([]byte(content), &data) != nil {
		return false
	}
	switch stage {
	case "generation":
		delete(data, "jobContext") // Optional, already source-validated by the handler.
		return exactFields(data, "careerEvidence", "jobPosting", "confirmedQualifications", "qualificationAnswers", "selection", "resumeApplicationEvidence")
	case "resume_support":
		return exactFields(data, "claims", "facts", "qualificationAnswers", "cvLanguage")
	case "artifact_support":
		return exactFields(data, "careerCitations", "blocks", "facts", "jobPosting", "jobCitations", "applicationAnswers", "qualificationAnswers", "cvLanguage")
	}
	return false
}

func allowedFormat(raw json.RawMessage, schema string) bool {
	var format map[string]json.RawMessage
	if json.Unmarshal(raw, &format) != nil {
		return false
	}
	if schema == "" {
		return exactFields(format, "type") && string(format["type"]) == `"json_object"`
	}
	if !exactFields(format, "type", "json_schema") || string(format["type"]) != `"json_schema"` {
		return false
	}
	var definition map[string]json.RawMessage
	if json.Unmarshal(format["json_schema"], &definition) != nil || !exactFields(definition, "name", "strict", "schema") || string(definition["strict"]) != "true" {
		return false
	}
	var name string
	return json.Unmarshal(definition["name"], &name) == nil && name == schema
}

func exactFields(object map[string]json.RawMessage, fields ...string) bool {
	if len(object) != len(fields) {
		return false
	}
	for field := range object {
		if !slices.Contains(fields, field) {
			return false
		}
	}
	return true
}

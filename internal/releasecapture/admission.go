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

type stageContract struct {
	ceiling                       int
	schema                        string
	roles, fields, optionalFields []string
}

var stageContracts = map[string]stageContract{
	"generation": {
		ceiling: 8000, schema: "application_draft", roles: []string{"system", "user"},
		fields: []string{"careerEvidence", "jobPosting", "confirmedQualifications", "qualificationAnswers", "selection", "resumeApplicationEvidence"}, optionalFields: []string{"jobContext"},
	},
	"resume_support": {
		ceiling: 4000, schema: "resume_support", roles: []string{"system", "user"}, fields: []string{"claims", "facts", "qualificationAnswers", "cvLanguage"},
	},
	"artifact_support": {
		ceiling: 8000, schema: "artifact_support", roles: []string{"system", "user"}, fields: []string{"careerCitations", "blocks", "facts", "jobPosting", "jobCitations", "applicationAnswers", "qualificationAnswers", "cvLanguage"},
	},
	"qualification_matching": {ceiling: 8000, schema: "qualification_job_context", roles: []string{"user"}},
	"requirement_evidence":   {ceiling: 8000, schema: "requirement_evidence", roles: []string{"user"}},
}

func (c *collector) allowedOpenAI(stage string, object map[string]json.RawMessage) bool {
	contract, ok := stageContracts[stage]
	if !ok {
		return false
	}
	if stage == "qualification_matching" && !c.understandJob {
		contract.ceiling, contract.schema = 1200, ""
	}
	var actualCeiling int
	if json.Unmarshal(object["max_completion_tokens"], &actualCeiling) != nil || actualCeiling != contract.ceiling || !allowedFormat(object["response_format"], contract.schema) {
		return false
	}
	var messages []map[string]json.RawMessage
	if json.Unmarshal(object["messages"], &messages) != nil || len(messages) != len(contract.roles) {
		return false
	}
	for i, message := range messages {
		var role, content string
		if !exactFields(message, "role", "content") || json.Unmarshal(message["role"], &role) != nil || role != contract.roles[i] || json.Unmarshal(message["content"], &content) != nil || content == "" {
			return false
		}
	}
	if len(contract.fields) == 0 {
		return true
	}
	var content string
	_ = json.Unmarshal(messages[1]["content"], &content)
	var data map[string]json.RawMessage
	if json.Unmarshal([]byte(content), &data) != nil {
		return false
	}
	for _, field := range contract.optionalFields {
		delete(data, field)
	}
	return exactFields(data, contract.fields...)
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

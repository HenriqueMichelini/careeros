package applicationdraft

import (
	"encoding/json"
	"testing"
)

func TestCompleteAccountDraftInputIncludesTemporaryAnswers(t *testing.T) {
	// The Apply client sends all ten fields after qualification review. Temporary
	// answers must not make this otherwise valid request fail the envelope bound.
	raw := []byte(`{
  "repository":{"careerGoals":"","skills":"TypeScript","competencies":"","experience":[],"tools":"","projects":[],"employmentStatus":"","currentSalary":"","desiredSalary":"","additionalInfo":""},
  "qualifications":{"education":[],"certifications":[],"languages":[]},
  "jobPosting":"TypeScript developer.","confirmedQualifications":[],
  "cvLanguage":"en","reviewResume":true,"reviewArtifacts":true,
  "selectedFactIds":[],"jobContext":{},
  "qualificationAnswers":[{"requirement":"TypeScript experience","userContext":"I use TypeScript."}]
 }`)
	var input map[string]json.RawMessage
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	if !completeInputShape(input) {
		t.Fatal("valid full Apply envelope rejected")
	}
	input["accountId"] = json.RawMessage(`"not-authority"`)
	if completeInputShape(input) {
		t.Fatal("ownership field accepted")
	}
}

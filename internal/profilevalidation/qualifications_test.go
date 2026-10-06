package profilevalidation

import (
	"encoding/json"
	"testing"
)

func TestQualificationsShapeAndBounds(t *testing.T) {
	raw := json.RawMessage(`{"education":[{"id":"e1","degree":"BSc","institution":"Example","location":"","graduationDate":"2018","details":""}],"certifications":[],"languages":[{"id":"l1","name":"English","proficiency":"Fluent"}]}`)
	if !CompleteQualifications(raw) {
		t.Fatal("valid qualifications rejected")
	}
	var q Qualifications
	if json.Unmarshal(raw, &q) != nil || !ValidQualifications(q) {
		t.Fatal("valid qualifications failed bounds")
	}
	q.Languages[0].ID = "e1"
	if ValidQualifications(q) {
		t.Fatal("duplicate ID accepted")
	}
	if CompleteQualifications(json.RawMessage(`{"education":[{"id":"e1","degree":"BSc"}],"certifications":[],"languages":[]}`)) {
		t.Fatal("incomplete education accepted")
	}
	if CompleteQualifications(json.RawMessage(`{"education":[],"certifications":[],"languages":[],"private":"x"}`)) {
		t.Fatal("extra group accepted")
	}
}

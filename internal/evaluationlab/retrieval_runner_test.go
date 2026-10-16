package evaluationlab_test

import (
	"context"
	"encoding/json"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func TestQualificationModulePreservesRetainedReportShape(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/qualification-evidence/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.RunQualificationEvidence(context.Background(), raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Retrieval) != 39 || len(r.Final) != 13 {
		t.Fatalf("shape drift: retrieval=%d final=%d", len(r.Retrieval), len(r.Final))
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	if len(object) != 5 || object["fixtureHash"] == nil || object["controlledFinalMatching"] == nil {
		t.Fatal("serializer contract changed")
	}
}

func TestApplicationModulePreservesRetrievalAndFieldReportShapes(t *testing.T) {
	read := func(path string) []byte {
		raw, err := os.ReadFile("../../docs/evaluations/" + path)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	r, err := lab.RunApplicationContext(context.Background(), read("qualification-evidence/cases.json"), read("semantic-quality/cases.v2.json"), read("semantic-quality/controlled.v2.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Retrieval) != 13 || len(r.Fields) != 40 {
		t.Fatalf("shape drift: retrieval=%d fields=%d", len(r.Retrieval), len(r.Fields))
	}
}

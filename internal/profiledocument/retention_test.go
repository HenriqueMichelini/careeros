package profiledocument

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestCleanupPreservesOpaqueValuesAndRejectsHistoricalOnlySupport(t *testing.T) {
	raw, err := os.ReadFile("../accountapi/testdata/profile-export-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	oldCount := len(doc.Evidence)
	orphan := `{"id":"historical-orphan","revision":1,"excerpt":"DELETED HISTORY","origin":"synthetic","approval":"approved"}`
	l, _ := json.Marshal(Link{ID: "historical-only", Kind: "supports", From: Reference{doc.ID, doc.Facts[0].ID, 1}, To: Reference{doc.ID, "historical-orphan", 1}, State: "invalidated"})
	input := bytes.Replace(raw, []byte(`"evidence": [`), []byte(`"evidence": [`+orphan+`,`), 1)
	input = bytes.Replace(input, []byte(`"links": [`), append([]byte(`"links": [`), append(l, ',')...), 1)
	next, changed, err := CleanupRetainedEvidence(input)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	after, err := Decode(next)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != doc.Revision+1 || len(after.Evidence) != oldCount || bytes.Contains(next, []byte("DELETED HISTORY")) {
		t.Fatal("cleanup scope", string(next))
	}
	for i, f := range doc.Facts {
		if !bytes.Equal(f.Value, after.Facts[i].Value) {
			t.Fatalf("opaque value bytes changed for %s: %s => %s", f.ID, f.Value, after.Facts[i].Value)
		}
	}
	for _, lexeme := range []string{"9007199254740993", "1e+308", "5e-324"} {
		if !bytes.Contains(next, []byte(lexeme)) {
			t.Fatal("numeric lexeme lost", lexeme)
		}
	}
	duplicate, changed, err := CleanupRetainedEvidence(next)
	if err != nil || changed || !bytes.Equal(next, duplicate) {
		t.Fatal("cleanup replay changed canonical bytes", changed, err)
	}
}

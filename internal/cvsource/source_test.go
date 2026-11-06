package cvsource

import (
	"encoding/json"
	"os"
	"professional-information-repo/internal/profiledocument"
	"testing"
)

func TestCVSourceProjectionCorpus(t *testing.T) {
	raw, e := os.ReadFile("../accountapi/testdata/profile-projections.json")
	if e != nil {
		t.Fatal(e)
	}
	var fixtures []struct {
		Name     string          `json:"name"`
		Document json.RawMessage `json:"document"`
	}
	if e = json.Unmarshal(raw, &fixtures); e != nil {
		t.Fatal(e)
	}
	results := []any{}
	for _, f := range fixtures {
		doc, e := profiledocument.Decode(f.Document)
		if e != nil {
			t.Fatal(f.Name, e)
		}
		results = append(results, map[string]any{"name": f.Name, "facts": Facts(doc)})
	}
	if path := os.Getenv("CAREEROS_CV_PROJECTION_OUTPUT"); path != "" {
		out, e := json.Marshal(results)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, out, 0600); e != nil {
			t.Fatal(e)
		}
	}
}

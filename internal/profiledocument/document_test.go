package profiledocument

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSharedContractCorpus(t *testing.T) {
	raw, err := os.ReadFile("fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string          `json:"name"`
		Valid    bool            `json:"valid"`
		Document json.RawMessage `json:"document"`
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range cases {
		t.Run(fixture.Name, func(t *testing.T) {
			doc, err := Decode(fixture.Document)
			if (err == nil) != fixture.Valid {
				t.Fatalf("valid=%v, error=%v", fixture.Valid, err)
			}
			if err == nil {
				encoded, err := json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = Decode(encoded); err != nil {
					t.Fatalf("Go roundtrip violates shared contract: %v", err)
				}
			}
		})
	}
}
func TestConservativeAliases(t *testing.T) {
	for _, term := range []string{"JS", "TS", "AWS", "Spring", "Spring Boot", " JavaScript "} {
		if NormalizeTerm(term).Canonical != nil {
			t.Fatalf("inferred alias for %q", term)
		}
	}
	for observed, expected := range map[string]string{"Javascript": "JavaScript", "Typescript": "TypeScript"} {
		n := NormalizeTerm(observed)
		if n.Observed != observed || n.Canonical == nil || *n.Canonical != expected || n.Policy != NormalizationPolicy {
			t.Fatalf("bad normalization: %+v", n)
		}
	}
}

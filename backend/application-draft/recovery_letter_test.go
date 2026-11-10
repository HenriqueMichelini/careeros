package applicationdraft

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRecoveryCompletedLetterCanonicalWordBoundary(t *testing.T) {
	raw, err := os.ReadFile("recovery_letter_fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Separator string
		Words, Count    int
		Valid           bool
	}
	if json.Unmarshal(raw, &cases) != nil {
		t.Fatal("fixture")
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			words := make([]string, c.Words)
			for i := range words {
				words[i] = "word"
			}
			body := strings.Join(words, c.Separator) + "."
			completed := "Dear hiring team,\n\n" + body + "\n\nSincerely,\nOriginal Person"
			if got := ValidCompletedCoverLetter(completed, "Original Person"); got != c.Valid {
				t.Fatalf("canonical split count=%d want valid=%v got=%v GoFields=%d", c.Count, c.Valid, got, len(strings.Fields(completed)))
			}
		})
	}
}

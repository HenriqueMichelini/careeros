package applicationdraft

import (
	"strings"
	"testing"
)

func TestManagedCompletedLetterKeepsLegacyRawCountingSeparate(t *testing.T) {
	letter := coverLetter{
		Greeting: "Dear hiring team,",
		Body:     strings.TrimSuffix(strings.Repeat("java\u0085", 400), "\u0085") + ".",
		Closing:  "Sincerely,",
	}
	if validCoverLetter(letter) {
		t.Fatal("raw legacy handler changed its existing word limit")
	}
	if !ValidCompletedCoverLetter(completeManagedLetter(letter, "Original Person"), "Original Person") {
		t.Fatal("managed completion diverged from the frozen recovery contract")
	}
}

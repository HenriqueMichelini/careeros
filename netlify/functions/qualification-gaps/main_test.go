package main

import (
	"testing"

	"professional-information-repo/internal/testsupport"
)

func TestDirectAndAliasFailClosed(t *testing.T) {
	testsupport.AssertWorkflowFailClosed(t, handler, "/api/qualification-gaps", "/.netlify/functions/qualification-gaps")
}

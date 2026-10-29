package main

import (
	"testing"

	"professional-information-repo/internal/testsupport"
)

func TestDirectAndAliasFailClosed(t *testing.T) {
	testsupport.AssertWorkflowFailClosed(t, handler, "/api/cv/generate", "/.netlify/functions/cv-generation")
}

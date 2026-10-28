package main

import (
	"testing"

	"professional-information-repo/internal/testsupport"
)

func TestDirectAndAliasFailClosed(t *testing.T) {
	testsupport.AssertWorkflowFailClosed(t, handler, "/api/profile/ingest", "/.netlify/functions/profile-ingestion")
}

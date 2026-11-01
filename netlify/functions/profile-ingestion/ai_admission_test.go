package main

import (
	"testing"

	profileingestion "professional-information-repo/backend/profile-ingestion"
	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestA29RealFunctionAuthenticatedPositiveAndManagedDenials(t *testing.T) {
	// Exercise the real package handler/global constructor. Reset its lazy runtime
	// after the fixture so independent tests cannot inherit the disposable store.
	t.Cleanup(func() {
		ingestionHandler = accountapi.WorkflowFromEnv("/api/profile/ingest", profileingestion.NewHandler())
	})
	aiadmission.AssertFunction(t, handler, "/api/profile/ingest", "/.netlify/functions/profile-ingestion")
}

package main

import (
	"testing"

	qualificationgaps "professional-information-repo/backend/qualification-gaps"
	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestA29RealFunctionAuthenticatedPositiveAndManagedDenials(t *testing.T) {
	// Exercise the real package handler/global constructor. Reset its lazy runtime
	// after the fixture so independent tests cannot inherit the disposable store.
	t.Cleanup(func() {
		gapHandler = accountapi.WorkflowFromEnv("/api/qualification-gaps", qualificationgaps.NewHandler())
	})
	aiadmission.AssertFunction(t, handler, "/api/qualification-gaps", "/.netlify/functions/qualification-gaps")
}

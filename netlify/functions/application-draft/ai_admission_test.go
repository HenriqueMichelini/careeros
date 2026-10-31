package main

import (
	"testing"

	applicationdraft "professional-information-repo/backend/application-draft"
	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestA29RealFunctionAuthenticatedPositiveAndManagedDenials(t *testing.T) {
	// Exercise the real package handler/global constructor. Reset its lazy runtime
	// after the fixture so independent tests cannot inherit the disposable store.
	t.Cleanup(func() {
		draftHandler = accountapi.WorkflowFromEnv("/api/application-draft", applicationdraft.NewHandler())
	})
	aiadmission.AssertFunction(t, handler, "/api/application-draft", "/.netlify/functions/application-draft")
}

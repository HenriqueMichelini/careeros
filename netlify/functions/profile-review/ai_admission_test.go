package main

import (
	"testing"

	profilereview "professional-information-repo/backend/profile-review"
	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestA29RealFunctionAuthenticatedPositiveAndManagedDenials(t *testing.T) {
	// Exercise the real package handler/global constructor. Reset its lazy runtime
	// after the fixture so independent tests cannot inherit the disposable store.
	t.Cleanup(func() { reviewHandler = accountapi.WorkflowFromEnv("/api/profile/review", profilereview.NewHandler()) })
	aiadmission.AssertFunction(t, handler, "/api/profile/review", "/.netlify/functions/profile-review")
}

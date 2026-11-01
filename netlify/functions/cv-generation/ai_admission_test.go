package main

import (
	"testing"

	cvgeneration "professional-information-repo/backend/cv-generation"
	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestA29RealFunctionAuthenticatedPositiveAndManagedDenials(t *testing.T) {
	// Exercise the real package handler/global constructor. Reset its lazy runtime
	// after the fixture so independent tests cannot inherit the disposable store.
	t.Cleanup(func() { draftHandler = accountapi.WorkflowFromEnv("/api/cv/generate", cvgeneration.NewHandler()) })
	aiadmission.AssertFunction(t, handler, "/api/cv/generate", "/.netlify/functions/cv-generation")
}

package main

import (
	"context"
	"github.com/aws/aws-lambda-go/events"
	"strings"
	"testing"
)

func TestDirectAndAliasFailClosed(t *testing.T) {
	t.Setenv("CAREEROS_PUBLIC_ORIGIN", "")
	t.Setenv("CAREEROS_REQUIRE_AUTH", "")
	for _, path := range []string{"/api/profile/ingest", "/.netlify/functions/profile-ingestion"} {
		response, err := handler(context.Background(), events.APIGatewayProxyRequest{HTTPMethod: "POST", Path: path, Body: "{}", Headers: map[string]string{"X-OpenAI-Api-Key": "sk-synthetic", "X-CareerOS-Account": "claimed-account"}})
		if err != nil || response.StatusCode != 503 || !strings.Contains(response.Body, "auth_unavailable") || response.Headers["Cache-Control"] != "no-store" {
			t.Fatalf("%s: %v %+v", path, err, response)
		}
	}
}

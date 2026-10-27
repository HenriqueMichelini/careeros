package main

import (
	"context"
	"github.com/aws/aws-lambda-go/events"
	"testing"
)

func TestNewAccountPathsFailClosedThroughDirectAndRedirectedFunction(t *testing.T) {
	t.Setenv("CAREEROS_PUBLIC_ORIGIN", "")
	for _, path := range []string{"/api/auth/session", "/api/profile/current", "/api/profile/operations/foreign", "/.netlify/functions/account/api/profile/current", "/.netlify/functions/account/api/profile/operations/foreign", "/.netlify/functions/account/api/auth/session"} {
		response, err := handler(context.Background(), events.APIGatewayProxyRequest{Path: path, HTTPMethod: "GET"})
		if err != nil || response.StatusCode != 503 {
			t.Fatalf("%s: %v %v", path, response, err)
		}
	}
}

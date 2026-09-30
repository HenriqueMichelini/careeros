package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestHandlerUsesProfileReviewContractAndNoStoreHeader(t *testing.T) {
	response, err := handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod: "POST",
		Path:       "/.netlify/functions/profile-review",
		Body:       `{}`,
	})
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	if response.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", response.StatusCode)
	}
	if response.Headers["Cache-Control"] != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", response.Headers["Cache-Control"])
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "key" {
		t.Fatalf("error = %q, want key", body["error"])
	}
}

func TestHandlerDecodesBase64RequestBody(t *testing.T) {
	response, err := handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:        "POST",
		Path:              "/api/profile/review",
		Body:              base64.StdEncoding.EncodeToString([]byte(`{}`)),
		IsBase64Encoded:   true,
		Headers:           map[string]string{"X-OpenAI-Api-Key": "sk-user-key"},
		MultiValueHeaders: map[string][]string{},
	})
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	if response.StatusCode != 400 {
		t.Fatalf("status = %d, want 400 for decoded incomplete JSON request", response.StatusCode)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "input" {
		t.Fatalf("error = %q, want input", body["error"])
	}
}

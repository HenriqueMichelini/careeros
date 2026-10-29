package testsupport

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

// AssertWorkflowFailClosed exercises the actual Function adapter through both
// URLs without configured auth, even when caller-supplied account/AI headers exist.
func AssertWorkflowFailClosed(t *testing.T, handler func(context.Context, events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error), paths ...string) {
	t.Helper()
	t.Setenv("CAREEROS_PUBLIC_ORIGIN", "")
	t.Setenv("CAREEROS_REQUIRE_AUTH", "")
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			response, err := handler(context.Background(), events.APIGatewayProxyRequest{
				HTTPMethod: "POST", Path: path, Body: "{}",
				Headers: map[string]string{"X-OpenAI-Api-Key": "sk-synthetic", "X-CareerOS-Account": "claimed-account"},
			})
			if err != nil || response == nil || response.StatusCode != http.StatusServiceUnavailable || response.Headers["Cache-Control"] != "no-store" {
				t.Fatalf("%s: %v %+v", path, err, response)
			}
			var body map[string]string
			if err := json.Unmarshal([]byte(response.Body), &body); err != nil || body["error"] != "auth_unavailable" {
				t.Fatalf("%s: expected auth_unavailable JSON: %v %s", path, err, response.Body)
			}
		})
	}
}

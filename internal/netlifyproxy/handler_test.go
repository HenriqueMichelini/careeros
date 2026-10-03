package netlifyproxy

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestHandlePreservesGatewayRequestAndHTTPResponse(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request-context")
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/qualification-gaps" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got, err := io.ReadAll(r.Body); err != nil || string(got) != `{"jobPosting":"role"}` {
			t.Errorf("body = %q, err=%v", got, err)
		}
		if got := r.Header.Values("X-Multi"); len(got) != 2 || got[0] != "first" || got[1] != "second" {
			t.Errorf("multi-value header = %v", got)
		}
		if got := r.Context().Value(contextKey{}); got != "request-context" {
			t.Errorf("context value = %v", got)
		}
		w.Header().Add("X-Result", "first")
		w.Header().Add("X-Result", "second")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "accepted")
	})
	response, err := Handle(handler, "/api/qualification-gaps", ctx, events.APIGatewayProxyRequest{
		HTTPMethod:        http.MethodPost,
		Path:              "/wrong-path",
		Body:              base64.StdEncoding.EncodeToString([]byte(`{"jobPosting":"role"}`)),
		IsBase64Encoded:   true,
		Headers:           map[string]string{"X-Multi": "single"},
		MultiValueHeaders: map[string][]string{"X-Multi": {"first", "second"}},
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if response.StatusCode != http.StatusAccepted || response.Body != "accepted" || response.Headers["X-Result"] != "first" {
		t.Fatalf("response = %+v", response)
	}
}

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
	if got := response.MultiValueHeaders["X-Result"]; len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("multi-value response header = %v", got)
	}
}

func TestHandlePreservesQueryParameters(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/google/callback" {
			t.Errorf("path = %q", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("state") != "state-1" || query.Get("code") != "code-1" {
			t.Errorf("query = %v", query)
		}
		if got := query["scope"]; len(got) != 2 || got[0] != "openid" || got[1] != "email" {
			t.Errorf("multi-value query = %v", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	response, err := Handle(handler, "/api/auth/google/callback", context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:                      http.MethodGet,
		QueryStringParameters:           map[string]string{"state": "state-1", "code": "code-1", "scope": "openid"},
		MultiValueQueryStringParameters: map[string][]string{"scope": {"openid", "email"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestHandleUsesEventPathWhenRouteIsEmpty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/session" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})
	response, err := Handle(handler, "", context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/api/auth/session",
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestHandlePreservesMultipleSetCookieHeaders(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "__Host-careeros_session", Value: "session", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.SetCookie(w, &http.Cookie{Name: "__Host-careeros_csrf", Value: "csrf", Path: "/", Secure: true, SameSite: http.SameSiteLaxMode})
		w.WriteHeader(http.StatusNoContent)
	})
	response, err := Handle(handler, "/api/auth/google/callback", context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
	})
	if err != nil {
		t.Fatal(err)
	}
	cookies := response.MultiValueHeaders["Set-Cookie"]
	if len(cookies) != 2 {
		t.Fatalf("Set-Cookie headers = %v, want 2", cookies)
	}
	if response.Headers["Set-Cookie"] != cookies[0] {
		t.Fatalf("single-value Set-Cookie = %q, first multi-value = %q", response.Headers["Set-Cookie"], cookies[0])
	}
}

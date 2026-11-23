package pilotruntime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestClosedSiteSeparatesAPIFromSPA(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":    {Data: []byte("<html>closed pilot shell</html>")},
		"assets/app.js": {Data: []byte("/* bundled */")},
	}
	h, err := newSite("127.0.0.1:8787", assets, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path, method, body string
		status             int
	}{
		{"/", http.MethodGet, "closed pilot shell", 200},
		{"/application/draft", http.MethodGet, "closed pilot shell", 200},
		{"/assets/app.js", http.MethodGet, "bundled", 200},
		{"/assets/missing.js", http.MethodGet, "not_found", 404},
		{"/api/auth/session", http.MethodGet, "auth_unavailable", 503},
		{"/.netlify/functions/account/api/auth/session", http.MethodGet, "auth_unavailable", 503},
		{"/.netlify/functions/profile-review", http.MethodPost, "auth_unavailable", 503},
		{"/api/no-such-route", http.MethodGet, "not_found", 404},
		{"/.netlify/functions/no-such-function", http.MethodGet, "not_found", 404},
		{"/%61pi/auth/session", http.MethodGet, "invalid_path", 400},
		{"/healthz", http.MethodGet, "live", 200},
		{"/readyz", http.MethodGet, "unavailable", 503},
		{"/application/draft", http.MethodPost, "method_not_allowed", 405},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, "http://127.0.0.1:8787"+tc.path, nil)
			if tc.method == http.MethodPost {
				request.Header.Set("Origin", "http://127.0.0.1:8787")
			}
			got := httptest.NewRecorder()
			h.ServeHTTP(got, request)
			if got.Code != tc.status || !strings.Contains(got.Body.String(), tc.body) {
				t.Fatalf("got %d %q, want %d containing %q", got.Code, got.Body.String(), tc.status, tc.body)
			}
			if tc.status >= 400 && (!strings.HasPrefix(got.Header().Get("Content-Type"), "application/json") ||
				got.Header().Get("Cache-Control") != "no-store") {
				t.Fatalf("error response headers: %v", got.Header())
			}
		})
	}
}

func TestClosedSiteBoundsDependencyRequests(t *testing.T) {
	const origin = "http://127.0.0.1:8787"
	inFlight := make(chan struct{}, 8)
	release := make(chan struct{})
	finished := make(chan struct{}, 8)
	h, err := newSite("127.0.0.1:8787", fstest.MapFS{"index.html": {Data: []byte("closed")}},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			inFlight <- struct{}{}
			<-release
		}), func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for range 8 {
		go func() {
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, origin+"/api/auth/session", nil))
			finished <- struct{}{}
		}()
	}
	defer func() {
		close(release)
		for range 8 {
			<-finished
		}
	}()
	deadline := time.After(3 * time.Second)
	for range 8 {
		select {
		case <-inFlight:
		case <-deadline:
			t.Fatal("eight requests did not reach the fixture handler")
		}
	}
	bounds := map[string]int{"/readyz": 503, "/healthz": 200, "/api/auth/session": 503}
	for path, expected := range bounds {
		got := httptest.NewRecorder()
		h.ServeHTTP(got, httptest.NewRequest(http.MethodGet, origin+path, nil))
		if got.Code != expected {
			t.Fatalf("%s: %d, want %d", path, got.Code, expected)
		}
	}
}

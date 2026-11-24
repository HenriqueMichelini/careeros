package pilotruntime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHostedAddress(t *testing.T) {
	const origin = "https://careeros-294-closed-123456.southamerica-east1.run.app"
	host, address, err := hostedAddress(origin, "")
	if err != nil || host != strings.TrimPrefix(origin, "https://") || address != "0.0.0.0:8080" {
		t.Fatalf("hosted address = %q, %q, %v", host, address, err)
	}
	_, address, err = hostedAddress(origin, "8443")
	if err != nil || address != "0.0.0.0:8443" {
		t.Fatalf("platform port = %q, %v", address, err)
	}
	for _, invalid := range []string{
		"http://careeros.example", "https://careeros.example/", "https://careeros.example?",
		"https://careeros.example:443", "https://user@careeros.example",
		"https://Careeros.Example", "https://127.0.0.1", "https://careeros.example.",
	} {
		if _, _, err := hostedAddress(invalid, "8080"); err == nil {
			t.Errorf("accepted origin %q", invalid)
		}
	}
	for _, invalid := range []string{"0", "65536", "08080", "+8080", " 8080"} {
		if _, _, err := hostedAddress(origin, invalid); err == nil {
			t.Errorf("accepted port %q", invalid)
		}
	}
}

func TestHostedSitePinsOriginAndClosedRoutes(t *testing.T) {
	const authority = "careeros-294-closed.example"
	const origin = "https://" + authority
	files := fstest.MapFS{"index.html": {Data: []byte("<main>hosted shell</main>")}}
	pilot := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.URL.Path)
	})
	closed := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reply(w, http.StatusServiceUnavailable, "error", "closed")
	})
	handler, err := newSiteAtOrigin(authority, origin, files, closedHostedRoutes(pilot, closed), func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, method, path, host string
		origins                  []string
		status                   int
		body                     string
	}{
		{"workspace", "GET", "/workspace", authority, nil, 200, "hosted shell"},
		{"wrong host despite forwarding", "GET", "/healthz", "alternate.example", nil, 421, "wrong_origin"},
		{"HTTP origin", "POST", "/api/auth/logout", authority, []string{"http://" + authority}, 403, "forbidden"},
		{"duplicate HTTPS origin", "POST", "/api/auth/logout", authority, []string{origin, origin}, 403, "forbidden"},
		{"absent write origin", "POST", "/api/auth/logout", authority, nil, 403, "forbidden"},
		{"exact HTTPS origin", "POST", "/api/auth/logout", authority, []string{origin}, 200, "/api/auth/logout"},
		{"callback navigation", "GET", "/api/auth/google/callback", authority, nil, 200, "/api/auth/google/callback"},
		{"same workflow alias", "POST", "/.netlify/functions/profile-review", authority, []string{origin}, 200, "/api/profile/review"},
		{"closed activation alias", "POST", "/.netlify/functions/account/api/pilot/activate", authority, []string{origin}, 503, "closed"},
		{"closed draft retrieval", "GET", "/api/drafts/dummy/versions/1", authority, nil, 503, "closed"},
		{"unknown API", "GET", "/api/made-up-route", authority, nil, 404, "not_found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, origin+test.path, nil)
			req.Host = test.host
			req.Header.Set("X-Forwarded-Host", authority)
			for _, value := range test.origins {
				req.Header.Add("Origin", value)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.body) {
				t.Fatalf("got status %d and body %q", response.Code, response.Body.String())
			}
		})
	}
}

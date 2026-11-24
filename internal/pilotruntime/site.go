// Package pilotruntime assembles closed pilot runtimes.
package pilotruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"professional-information-repo/internal/auth"
)

const (
	maxRequestBytes = 5 << 20
	maxAPIRequests  = 8
	readinessLimit  = 2 * time.Second
	functionPrefix  = "/.netlify/functions/"
)

var workflowAliases = map[string]string{
	"application-draft":  "/api/application-draft",
	"qualification-gaps": "/api/qualification-gaps",
	"cv-generation":      "/api/cv/generate",
	"profile-ingestion":  "/api/profile/ingest",
	"profile-review":     "/api/profile/review",
}

// newSite reserves API and Function routes before the static fallback. The
// caller supplies only the closed Account/pilot composition in the executable.
func newSite(authority string, assets fs.FS, account http.Handler, ready func(context.Context) error) (http.Handler, error) {
	return newSiteAtOrigin(authority, "http://"+authority, assets, account, ready)
}

func newSiteAtOrigin(authority, origin string, assets fs.FS, account http.Handler, ready func(context.Context) error) (http.Handler, error) {
	info, err := fs.Stat(assets, "index.html")
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("pilot frontend entrypoint unavailable")
	}
	index, err := fs.ReadFile(assets, "index.html")
	if err != nil || len(index) == 0 {
		return nil, errors.New("pilot frontend entrypoint unavailable")
	}

	available := func(ctx context.Context) bool {
		if account == nil || ready == nil {
			return false
		}
		ctx, cancel := context.WithTimeout(ctx, readinessLimit)
		defer cancel()
		return ready(ctx) == nil
	}
	slots := make(chan struct{}, maxAPIRequests)
	static := http.FileServerFS(assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if r.Host != authority {
			reply(w, http.StatusMisdirectedRequest, "error", "wrong_origin")
			return
		}
		p := r.URL.Path
		if r.URL.EscapedPath() != p || path.Clean(p) != p || strings.Contains(p, "\\") {
			reply(w, http.StatusBadRequest, "error", "invalid_path")
			return
		}
		if p == "/healthz" {
			serveHealth(w, r, false, available)
			return
		}
		if p == "/readyz" || inNamespace(p, "/api") || inNamespace(p, "/.netlify") {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				reply(w, http.StatusServiceUnavailable, "error", "runtime_busy")
				return
			}
			if p == "/readyz" {
				serveHealth(w, r, true, available)
				return
			}
			canonical := canonicalAPIPath(p)
			if !knownAPIPath(canonical) {
				reply(w, http.StatusNotFound, "error", "not_found")
				return
			}
			origins := r.Header.Values("Origin")
			if len(origins) != 0 || !safeMethod(r.Method) {
				if len(origins) != 1 || origins[0] != origin {
					reply(w, http.StatusForbidden, "error", "forbidden")
					return
				}
			}
			if r.ContentLength > maxRequestBytes {
				reply(w, http.StatusRequestEntityTooLarge, "error", "request_too_large")
				return
			}
			if !available(r.Context()) {
				auth.UnavailableHandler().ServeHTTP(w, r)
				return
			}
			request := r.Clone(r.Context())
			request.URL.Path, request.URL.RawPath = canonical, ""
			request.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
			account.ServeHTTP(w, request)
			return
		}
		if !safeMethod(r.Method) {
			w.Header().Set("Allow", "GET, HEAD")
			reply(w, http.StatusMethodNotAllowed, "error", "method_not_allowed")
			return
		}
		name := strings.TrimPrefix(p, "/")
		if name != "" && name != "index.html" {
			file, err := fs.Stat(assets, name)
			if err == nil && file.Mode().IsRegular() {
				static.ServeHTTP(w, r)
				return
			}
			if inNamespace(p, "/assets") || path.Ext(name) != "" {
				reply(w, http.StatusNotFound, "error", "not_found")
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
	}), nil
}

func inNamespace(p, prefix string) bool {
	return p == prefix || strings.HasPrefix(p, prefix+"/")
}

func canonicalAPIPath(p string) string {
	if !strings.HasPrefix(p, functionPrefix) {
		return p
	}
	alias := strings.TrimPrefix(p, functionPrefix)
	if strings.HasPrefix(alias, "account/api/") {
		return strings.TrimPrefix(alias, "account")
	}
	return workflowAliases[alias]
}

func knownAPIPath(p string) bool {
	switch p {
	case "/api/auth/session", "/api/auth/logout", "/api/auth/logout-all",
		"/api/auth/google/start", "/api/auth/google/callback",
		"/api/profile/current", "/api/profile/export", "/api/lifecycle/status",
		"/api/pilot/status", "/api/pilot/activate", "/api/draft-allowance",
		"/api/cv/generate", "/api/profile/ingest", "/api/profile/review",
		"/api/qualification-gaps", "/api/application-draft":
		return true
	}
	for _, prefix := range []string{"/api/profile/operations", "/api/deletion", "/api/rights", "/api/drafts", "/api/ai"} {
		if inNamespace(p, prefix) {
			return true
		}
	}
	return false
}

func safeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

func serveHealth(w http.ResponseWriter, r *http.Request, readiness bool, available func(context.Context) bool) {
	switch {
	case !safeMethod(r.Method):
		w.Header().Set("Allow", "GET, HEAD")
		reply(w, http.StatusMethodNotAllowed, "error", "method_not_allowed")
	case !readiness:
		reply(w, http.StatusOK, "status", "live")
	case !available(r.Context()):
		reply(w, http.StatusServiceUnavailable, "status", "unavailable")
	default:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, "{\"status\":\"ready\",\"invitations\":\"closed\",\"inference\":\"closed\"}\n")
	}
}

func reply(w http.ResponseWriter, status int, key, value string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{key: value})
}

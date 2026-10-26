package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func handlerStart(t *testing.T, service *Service) (*http.Response, string, *http.Cookie) {
	t.Helper()
	handler := NewHandler(service)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, testOrigin+"/api/auth/google/start", nil))
	response := recorder.Result()
	t.Cleanup(func() { _ = response.Body.Close() })
	if response.StatusCode != http.StatusFound {
		t.Fatalf("start status = %d, want 302", response.StatusCode)
	}
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := location.Query().Get("state")
	if state == "" {
		t.Fatalf("location has no state: %s", location)
	}
	binding, ok := findCookie(t, response, oidcCookieName)
	if !ok {
		t.Fatal("binding cookie missing")
	}
	return response, state, binding
}

func handlerCallback(t *testing.T, service *Service, state string, binding *http.Cookie) (*http.Response, uint64) {
	t.Helper()
	handler := NewHandler(service)
	query := url.Values{}
	query.Set("state", state)
	query.Set("code", "auth-code")
	request := httptest.NewRequest(http.MethodGet, testOrigin+"/api/auth/google/callback?"+query.Encode(), nil)
	if binding != nil {
		request.AddCookie(binding)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	response := recorder.Result()
	t.Cleanup(func() { _ = response.Body.Close() })
	return response, uint64(response.StatusCode)
}

func handlerLogin(t *testing.T, service *Service) (*http.Response, *http.Cookie, *http.Cookie) {
	t.Helper()
	_, state, binding := handlerStart(t, service)
	response, status := handlerCallback(t, service, state, binding)
	if status != http.StatusSeeOther {
		t.Fatalf("callback status = %d, want 303", status)
	}
	session, csrf := sessionCookies(t, response)
	return response, session, csrf
}

func runRequest(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestHandlerStartSetsBindingCookieAndSecurityHeaders(t *testing.T) {
	env := newTestEnv(t)
	response, _, binding := handlerStart(t, env.service)
	assertCookieSecurity(t, binding, true)
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header.Get("Cache-Control"))
	}
	if response.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q", response.Header.Get("Referrer-Policy"))
	}
	location := response.Header.Get("Location")
	if !strings.Contains(location, "code_challenge_method=S256") || !strings.Contains(location, "nonce=") {
		t.Fatalf("location = %s", location)
	}
}

func TestHandlerDeniesUnlistedProvider(t *testing.T) {
	env := newTestEnv(t)
	handler := NewHandler(env.service)
	recorder := runRequest(handler, httptest.NewRequest(http.MethodGet, testOrigin+"/api/auth/github/start", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body["error"] != "provider_not_allowed" {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if env.stores.Transactions.(*MemoryTransactionStore).Len() != 0 {
		t.Fatal("denied provider created a transaction")
	}
}

func TestHandlerCallbackSetsSessionAndRedirects(t *testing.T) {
	env := newTestEnv(t)
	response, session, csrf := handlerLogin(t, env.service)
	assertCookieSecurity(t, session, true)
	assertCookieSecurity(t, csrf, false)
	if response.Header.Get("Location") != "/" {
		t.Fatalf("redirect = %q, want /", response.Header.Get("Location"))
	}
	if _, ok := findCookie(t, response, oidcCookieName); !ok {
		t.Fatal("binding cookie was not cleared")
	}
	var view SessionView
	handler := NewHandler(env.service)
	request := requestWithCookie(http.MethodGet, testOrigin+"/api/auth/session", session)
	recorder := runRequest(handler, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("session status = %d", recorder.Code)
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Authenticated || view.Account == nil || view.Account.Email != "one@example.com" {
		t.Fatalf("view = %+v", view)
	}
	if strings.Contains(recorder.Body.String(), session.Value) || strings.Contains(recorder.Body.String(), csrf.Value) {
		t.Fatal("session secret leaked in the session response")
	}
}

func TestHandlerCallbackRejectsTamperingAndReplay(t *testing.T) {
	env := newTestEnv(t)
	_, state, binding := handlerStart(t, env.service)
	wrongBinding := &http.Cookie{Name: oidcCookieName, Value: "wrong"}
	response, status := handlerCallback(t, env.service, state, wrongBinding)
	if status != http.StatusBadRequest {
		t.Fatalf("wrong binding status = %d, want 400", status)
	}
	if _, ok := findCookie(t, response, sessionCookieName); ok {
		t.Fatal("session issued after a binding mismatch")
	}
	if env.provider.callCount() != 0 {
		t.Fatal("provider exchange attempted after a binding mismatch")
	}
	// The failed callback consumed the transaction; the real binding cannot replay it.
	_, status = handlerCallback(t, env.service, state, binding)
	if status != http.StatusBadRequest {
		t.Fatalf("replay status = %d, want 400", status)
	}
}

func TestHandlerSessionProbeIsUnauthenticatedWithoutCookie(t *testing.T) {
	env := newTestEnv(t)
	handler := NewHandler(env.service)
	recorder := runRequest(handler, httptest.NewRequest(http.MethodGet, testOrigin+"/api/auth/session", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var view SessionView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Authenticated {
		t.Fatal("unauthenticated probe returned an account")
	}
}

func TestHandlerSessionProbeClearsRevokedCookie(t *testing.T) {
	env := newTestEnv(t)
	_, session, _ := handlerLogin(t, env.service)
	if err := env.service.Logout(context.Background(), session.Value); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(env.service)
	recorder := runRequest(handler, requestWithCookie(http.MethodGet, testOrigin+"/api/auth/session", session))
	var view SessionView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Authenticated {
		t.Fatal("revoked session reported as authenticated")
	}
	if _, ok := findCookie(t, recorder.Result(), sessionCookieName); !ok {
		t.Fatal("revoked session cookie was not cleared")
	}
}

func TestHandlerLogoutRequiresSameOriginAndCSRF(t *testing.T) {
	env := newTestEnv(t)
	handler := NewHandler(env.service)
	_, session, csrf := handlerLogin(t, env.service)

	request := requestWithCookie(http.MethodPost, testOrigin+"/api/auth/logout", session)
	request.AddCookie(csrf)
	if recorder := runRequest(handler, request); recorder.Code != http.StatusForbidden {
		t.Fatalf("missing origin status = %d, want 403", recorder.Code)
	}

	request = requestWithCookie(http.MethodPost, testOrigin+"/api/auth/logout", session)
	request.Header.Set("Origin", testOrigin)
	if recorder := runRequest(handler, request); recorder.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status = %d, want 403", recorder.Code)
	}

	request = requestWithCookie(http.MethodPost, testOrigin+"/api/auth/logout", session)
	request.AddCookie(csrf)
	request.Header.Set("Origin", testOrigin)
	request.Header.Set("X-CSRF-Token", csrf.Value)
	recorder := runRequest(handler, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204; body=%s", recorder.Code, recorder.Body.String())
	}
	if _, err := env.service.Authenticate(context.Background(), session.Value); err != ErrAuthenticationRequired {
		t.Fatalf("session after logout = %v", err)
	}
}

func TestHandlerLogoutAllRevokesEverySession(t *testing.T) {
	env := newTestEnv(t)
	handler := NewHandler(env.service)
	_, session, csrf := handlerLogin(t, env.service)
	// A second session for the same account, without replacing the first one.
	otherStart, otherState := env.start(t, nil)
	other, err := env.service.CompleteLogin(context.Background(), "google", otherState, "code", otherStart.BindingToken)
	if err != nil {
		t.Fatal(err)
	}
	request := requestWithCookie(http.MethodPost, testOrigin+"/api/auth/logout-all", session)
	request.AddCookie(csrf)
	request.Header.Set("Origin", testOrigin)
	request.Header.Set("X-CSRF-Token", csrf.Value)
	recorder := runRequest(handler, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	for _, token := range []string{session.Value, other.SessionToken} {
		if _, err := env.service.Authenticate(context.Background(), token); err != ErrAuthenticationRequired {
			t.Fatalf("session after logout-all = %v", err)
		}
	}
}

func TestHandlerNilServiceFailsClosed(t *testing.T) {
	handler := NewHandler(nil)
	for _, target := range []string{"/api/auth/google/start", "/api/auth/session"} {
		recorder := runRequest(handler, httptest.NewRequest(http.MethodGet, testOrigin+target, nil))
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want 503", target, recorder.Code)
		}
	}
}

func TestGuardRequiresSessionOriginAndCSRF(t *testing.T) {
	env := newTestEnv(t)
	principal := env.login(t, nil)
	protected := env.service.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current, ok := PrincipalFromRequest(r)
		if !ok || current.AccountID == "" {
			t.Error("principal missing from context")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, current.AccountID)
	}))

	if recorder := runRequest(protected, httptest.NewRequest(http.MethodGet, testOrigin+"/api/account", nil)); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status = %d, want 401", recorder.Code)
	}
	request := requestWithCookie(http.MethodGet, testOrigin+"/api/account", &http.Cookie{Name: sessionCookieName, Value: principal.SessionToken})
	if recorder := runRequest(protected, request); recorder.Code != http.StatusOK {
		t.Fatalf("valid session status = %d, want 200", recorder.Code)
	}
	request = requestWithCookie(http.MethodPost, testOrigin+"/api/account", &http.Cookie{Name: sessionCookieName, Value: principal.SessionToken})
	if recorder := runRequest(protected, request); recorder.Code != http.StatusForbidden {
		t.Fatalf("missing origin status = %d, want 403", recorder.Code)
	}
	request = requestWithCookie(http.MethodPost, testOrigin+"/api/account", &http.Cookie{Name: sessionCookieName, Value: principal.SessionToken})
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: principal.CSRFToken})
	request.Header.Set("Origin", testOrigin)
	if recorder := runRequest(protected, request); recorder.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF header status = %d, want 403", recorder.Code)
	}
	request.Header.Set("X-CSRF-Token", principal.CSRFToken)
	if recorder := runRequest(protected, request); recorder.Code != http.StatusOK {
		t.Fatalf("valid CSRF status = %d, want 200", recorder.Code)
	}
}

func TestGuardRejectsCrossAccountAndRevokedCommit(t *testing.T) {
	env := newTestEnv(t)
	owner := env.loginAs(t, "owner", "owner@example.com")
	other := env.loginAs(t, "other", "owner@example.com")
	protected := env.service.ProtectFunc(func(w http.ResponseWriter, r *http.Request, principal Principal) {
		if err := env.service.AuthorizeOwner(principal, owner.Account.ID); err != nil {
			writeAuthError(w, err)
			return
		}
		if _, err := env.service.Revalidate(r.Context(), principal); err != nil {
			writeAuthError(w, err)
			return
		}
		_, _ = io.WriteString(w, "committed")
	})
	request := requestWithCookie(http.MethodGet, testOrigin+"/api/account", &http.Cookie{Name: sessionCookieName, Value: other.SessionToken})
	if recorder := runRequest(protected, request); recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-account status = %d, want 403", recorder.Code)
	}
	if _, err := env.service.LogoutAll(context.Background(), owner.Principal); err != nil {
		t.Fatal(err)
	}
	request = requestWithCookie(http.MethodGet, testOrigin+"/api/account", &http.Cookie{Name: sessionCookieName, Value: owner.SessionToken})
	if recorder := runRequest(protected, request); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("revoked status = %d, want 401", recorder.Code)
	}
}

func TestGuardRevalidatesImmediatelyBeforeCommit(t *testing.T) {
	env := newTestEnv(t)
	result := env.login(t, nil)
	started := make(chan struct{})
	release := make(chan struct{})
	protected := env.service.ProtectFunc(func(w http.ResponseWriter, r *http.Request, principal Principal) {
		close(started)
		<-release
		if _, err := env.service.Revalidate(r.Context(), principal); err != nil {
			writeAuthError(w, err)
			return
		}
		_, _ = io.WriteString(w, "committed")
	})
	request := requestWithCookie(http.MethodGet, testOrigin+"/api/account", &http.Cookie{Name: sessionCookieName, Value: result.SessionToken})
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		protected.ServeHTTP(recorder, request)
		close(done)
	}()
	<-started
	if _, err := env.service.LogoutAll(context.Background(), result.Principal); err != nil {
		t.Fatal(err)
	}
	close(release)
	<-done
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "committed") {
		t.Fatal("write committed after revocation")
	}
}

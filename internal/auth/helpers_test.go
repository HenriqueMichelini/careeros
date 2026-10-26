package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

const testOrigin = "http://127.0.0.1:8787"

type fakeProvider struct {
	id       string
	identity Identity
	err      error

	mu             sync.Mutex
	calls          int
	lastCode       string
	lastVerifier   string
	lastRedirect   string
	lastNonce      string
	beforeExchange func()
}

func (p *fakeProvider) ID() string { return p.id }

func (p *fakeProvider) AuthorizationURL(state, nonce, codeChallenge, redirectURI string) string {
	query := url.Values{}
	query.Set("state", state)
	query.Set("nonce", nonce)
	query.Set("code_challenge", codeChallenge)
	query.Set("code_challenge_method", "S256")
	query.Set("redirect_uri", redirectURI)
	return "https://provider.test/auth?" + query.Encode()
}

func (p *fakeProvider) Exchange(_ context.Context, code, verifier, redirectURI, nonce string) (Identity, error) {
	p.mu.Lock()
	p.calls++
	p.lastCode = code
	p.lastVerifier = verifier
	p.lastRedirect = redirectURI
	p.lastNonce = nonce
	hook := p.beforeExchange
	p.mu.Unlock()
	if hook != nil {
		hook()
	}
	if p.err != nil {
		return Identity{}, p.err
	}
	return p.identity, nil
}

func (p *fakeProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

type testEnv struct {
	service  *Service
	config   Config
	stores   Stores
	provider *fakeProvider
	now      time.Time
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	env := &testEnv{
		now: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC),
		provider: &fakeProvider{
			id: "google",
			identity: Identity{
				Provider:      "google",
				Issuer:        "https://accounts.google.com",
				Subject:       "subject-one",
				Email:         "one@example.com",
				EmailVerified: true,
				Name:          "One",
			},
		},
	}
	env.config = Config{
		PublicOrigin:   testOrigin,
		SessionTTL:     time.Hour,
		TransactionTTL: 10 * time.Minute,
		Providers: []ProviderConfig{{
			ID:                    "google",
			ClientID:              "client-id",
			ClientSecret:          "client-secret",
			RedirectURI:           testOrigin + "/api/auth/google/callback",
			Issuers:               []string{"https://accounts.google.com"},
			AuthorizationEndpoint: "https://provider.test/auth",
			TokenEndpoint:         "https://provider.test/token",
			JWKSURL:               "https://provider.test/jwks",
			Scopes:                []string{"openid", "email", "profile"},
		}},
		Now: func() time.Time { return env.now },
	}
	registry, err := NewRegistry(env.provider)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	env.stores = NewMemoryStores()
	env.service, err = NewService(env.config, registry, env.stores)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return env
}

func (e *testEnv) start(t *testing.T, current *Principal) (LoginStart, string) {
	t.Helper()
	start, err := e.service.BeginLogin(context.Background(), "google", "/", current)
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	parsed, err := url.Parse(start.URL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatalf("authorization URL has no state: %s", start.URL)
	}
	return start, state
}

func (e *testEnv) login(t *testing.T, current *Principal) LoginResult {
	t.Helper()
	start, state := e.start(t, current)
	result, err := e.service.CompleteLogin(context.Background(), "google", state, "auth-code", start.BindingToken)
	if err != nil {
		t.Fatalf("CompleteLogin: %v", err)
	}
	return result
}

func (e *testEnv) loginAs(t *testing.T, subject, email string) LoginResult {
	t.Helper()
	e.provider.mu.Lock()
	e.provider.identity.Subject = subject
	e.provider.identity.Email = email
	e.provider.mu.Unlock()
	return e.login(t, nil)
}

func requestWithCookie(method, target string, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func responseCookie(t *testing.T, response *http.Response, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response has no %s cookie; headers: %v", name, response.Header.Values("Set-Cookie"))
	return nil
}

func findCookie(t *testing.T, response *http.Response, name string) (*http.Cookie, bool) {
	t.Helper()
	for _, cookie := range response.Cookies() {
		if cookie.Name == name {
			return cookie, true
		}
	}
	return nil, false
}

func sessionCookies(t *testing.T, response *http.Response) (*http.Cookie, *http.Cookie) {
	t.Helper()
	session := responseCookie(t, response, sessionCookieName)
	csrf := responseCookie(t, response, csrfCookieName)
	return session, csrf
}

func assertCookieSecurity(t *testing.T, cookie *http.Cookie, httpOnly bool) {
	t.Helper()
	if !cookie.Secure {
		t.Errorf("%s cookie is not Secure", cookie.Name)
	}
	if cookie.HttpOnly != httpOnly {
		t.Errorf("%s HttpOnly = %v, want %v", cookie.Name, cookie.HttpOnly, httpOnly)
	}
	if cookie.Path != "/" {
		t.Errorf("%s Path = %q, want /", cookie.Name, cookie.Path)
	}
	if cookie.Domain != "" {
		t.Errorf("%s Domain = %q, want empty for __Host-", cookie.Name, cookie.Domain)
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("%s SameSite = %v, want Lax", cookie.Name, cookie.SameSite)
	}
}

func fmtSubject(index int) string {
	return fmt.Sprintf("subject-%d", index)
}

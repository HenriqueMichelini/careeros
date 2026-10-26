package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGoogleAuthorizationURLUsesExactRedirectAndPKCE(t *testing.T) {
	key := testRSAKey(t)
	jwks, _ := jwksServer(t, jwksFor(&key.PublicKey, "key-1"))
	provider, err := NewGoogleProvider(ProviderConfig{
		ID:                    "google",
		ClientID:              "client-id",
		ClientSecret:          "client-secret",
		RedirectURI:           testOrigin + "/api/auth/google/callback",
		Issuers:               []string{"https://accounts.google.com"},
		AuthorizationEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenEndpoint:         "https://oauth2.googleapis.com/token",
		JWKSURL:               jwks.URL,
		Scopes:                []string{"openid", "email", "profile"},
	}, jwks.Client(), time.Now, 0)
	if err != nil {
		t.Fatal(err)
	}
	raw := provider.AuthorizationURL("state-1", "nonce-1", "challenge-1", testOrigin+"/api/auth/google/callback")
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("state") != "state-1" || query.Get("nonce") != "nonce-1" || query.Get("code_challenge") != "challenge-1" {
		t.Fatalf("authorization parameters = %v", query)
	}
	if query.Get("code_challenge_method") != "S256" {
		t.Fatalf("challenge method = %q", query.Get("code_challenge_method"))
	}
	if query.Get("redirect_uri") != testOrigin+"/api/auth/google/callback" {
		t.Fatalf("redirect_uri = %q", query.Get("redirect_uri"))
	}
	if query.Get("response_type") != "code" || query.Get("scope") != "openid email profile" {
		t.Fatalf("response type or scope = %v", query)
	}
}

func TestGoogleExchangeValidatesIDToken(t *testing.T) {
	key := testRSAKey(t)
	jwks, _ := jwksServer(t, jwksFor(&key.PublicKey, "key-1"))
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	token := signToken(t, key, "key-1", nil, validClaims(now, "nonce-1"))
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if r.Form.Get("code_verifier") != "verifier-1" || r.Form.Get("code") != "code-1" {
			t.Errorf("token form = %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": token, "token_type": "Bearer"})
	}))
	t.Cleanup(tokenServer.Close)
	provider, err := NewGoogleProvider(ProviderConfig{
		ID:                    "google",
		ClientID:              "client-id",
		ClientSecret:          "client-secret",
		RedirectURI:           testOrigin + "/api/auth/google/callback",
		Issuers:               []string{"https://accounts.google.com"},
		AuthorizationEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenEndpoint:         tokenServer.URL,
		JWKSURL:               jwks.URL,
		Scopes:                []string{"openid", "email", "profile"},
	}, tokenServer.Client(), func() time.Time { return now }, 0)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := provider.Exchange(context.Background(), "code-1", "verifier-1", testOrigin+"/api/auth/google/callback", "nonce-1")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if identity.Provider != "google" || identity.Subject != "subject-one" || identity.Email != "one@example.com" {
		t.Fatalf("identity = %+v", identity)
	}
}

func TestGoogleExchangeRejectsBadProviderResponses(t *testing.T) {
	key := testRSAKey(t)
	jwks, _ := jwksServer(t, jwksFor(&key.PublicKey, "key-1"))
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	valid := signToken(t, key, "key-1", nil, validClaims(now, "nonce-1"))
	cases := []struct {
		name    string
		status  int
		body    string
		nonce   string
		wantErr bool
	}{
		{"provider-error", http.StatusBadRequest, `{"error":"invalid_grant"}`, "nonce-1", true},
		{"missing-id-token", http.StatusOK, `{"access_token":"opaque"}`, "nonce-1", true},
		{"wrong-nonce", http.StatusOK, `{"id_token":"` + valid + `"}`, "other-nonce", true},
		{"malformed", http.StatusOK, `{`, "nonce-1", true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(testCase.status)
				_, _ = w.Write([]byte(testCase.body))
			}))
			defer server.Close()
			provider, err := NewGoogleProvider(ProviderConfig{
				ID:                    "google",
				ClientID:              "client-id",
				ClientSecret:          "client-secret",
				RedirectURI:           testOrigin + "/api/auth/google/callback",
				Issuers:               []string{"https://accounts.google.com"},
				AuthorizationEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
				TokenEndpoint:         server.URL,
				JWKSURL:               jwks.URL,
				Scopes:                []string{"openid", "email", "profile"},
			}, server.Client(), func() time.Time { return now }, 0)
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.Exchange(context.Background(), "code", "verifier", testOrigin+"/api/auth/google/callback", testCase.nonce)
			if testCase.wantErr && err == nil {
				t.Fatal("expected rejection")
			}
			if !testCase.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPKCEChallengeMatchesVerifier(t *testing.T) {
	env := newTestEnv(t)
	start, state := env.start(t, nil)
	parsed, _ := url.Parse(start.URL)
	transaction, ok := env.stores.Transactions.(*MemoryTransactionStore).lookup(hashSecret(state))
	if !ok {
		t.Fatal("transaction missing")
	}
	challenge := sha256.Sum256([]byte(transaction.CodeVerifier))
	if base64.RawURLEncoding.EncodeToString(challenge[:]) != parsed.Query().Get("code_challenge") {
		t.Fatal("PKCE challenge mismatch")
	}
}

type offlineJWKSTransport func(*http.Request) (*http.Response, error)

func (f offlineJWKSTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProductionGoogleProviderJWKSCacheLifetime(t *testing.T) {
	key := testRSAKey(t)
	raw, err := json.Marshal(jwksFor(&key.PublicKey, "key-1"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, configured, cacheControl string
		lifetime                       time.Duration
	}{
		{"configured_fallback", "5m", "", 5 * time.Minute},
		{"default_fallback", "", "", time.Hour},
		{"invalid_header_uses_config", "5m", "max-age=invalid", 5 * time.Minute},
		{"header_precedes_shorter_config", "5m", "public, max-age=600", 10 * time.Minute},
		{"header_precedes_longer_config", "2h", "max-age=600", 10 * time.Minute},
		{"fallback_lower_bound", "1m", "", 5 * time.Minute},
		{"fallback_upper_bound", "48h", "", 24 * time.Hour},
		{"header_lower_bound", "1h", "max-age=60", 5 * time.Minute},
		{"header_upper_bound", "5m", "max-age=172800", 24 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CAREEROS_PUBLIC_ORIGIN", "https://careeros.example")
			t.Setenv("CAREEROS_GOOGLE_CLIENT_ID", "client-id")
			t.Setenv("CAREEROS_GOOGLE_CLIENT_SECRET", "synthetic-secret")
			t.Setenv("CAREEROS_GOOGLE_REDIRECT_URI", "")
			t.Setenv("CAREEROS_SESSION_TTL", "1h")
			t.Setenv("CAREEROS_TRANSACTION_TTL", "10m")
			t.Setenv("CAREEROS_JWKS_CACHE_TTL", tc.configured)
			cfg, err := ConfigFromEnv()
			if err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
			cfg.Now = func() time.Time { return now }
			calls := 0
			cfg.HTTPClient = &http.Client{Transport: offlineJWKSTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != googleJWKSURL {
					t.Fatalf("unexpected outbound request: %s", r.URL)
				}
				calls++
				header := make(http.Header)
				if tc.cacheControl != "" {
					header.Set("Cache-Control", tc.cacheControl)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: header,
					Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			})}
			provider, err := NewGoogleProvider(cfg.Providers[0], cfg.HTTPClient, cfg.Now, cfg.JWKSCacheTTL)
			if err != nil {
				t.Fatal(err)
			}
			verify := func() {
				t.Helper()
				token := signToken(t, key, "key-1", nil, validClaims(now, "nonce"))
				if _, err := provider.verifier.Verify(context.Background(), token, "nonce"); err != nil {
					t.Fatal(err)
				}
			}
			verify()
			now = now.Add(tc.lifetime - time.Second)
			verify()
			if calls != 1 {
				t.Fatalf("JWKS fetches before expiry = %d, want 1", calls)
			}
			now = now.Add(time.Second)
			verify()
			if calls != 2 {
				t.Fatalf("JWKS fetches at %v expiry = %d, want 2", tc.lifetime, calls)
			}
		})
	}
}

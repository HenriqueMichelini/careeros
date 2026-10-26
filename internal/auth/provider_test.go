package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	}, jwks.Client(), time.Now)
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
	}, tokenServer.Client(), func() time.Time { return now })
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
			}, server.Client(), func() time.Time { return now })
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

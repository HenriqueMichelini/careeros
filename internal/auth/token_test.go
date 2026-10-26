package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

func jwksFor(key *rsa.PublicKey, kid string) map[string]any {
	return map[string]any{
		"keys": []any{map[string]any{
			"kty": "RSA",
			"kid": kid,
			"use": "sig",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}},
	}
}

func jwksServer(t *testing.T, body any) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=600")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func signToken(t *testing.T, key *rsa.PrivateKey, kid string, header, claims map[string]any) string {
	t.Helper()
	if header == nil {
		header = map[string]any{"alg": "RS256", "kid": kid, "typ": "JWT"}
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func validClaims(now time.Time, nonce string) map[string]any {
	return map[string]any{
		"iss":            "https://accounts.google.com",
		"sub":            "subject-one",
		"aud":            "client-id",
		"azp":            "client-id",
		"exp":            now.Add(time.Hour).Unix(),
		"iat":            now.Unix(),
		"nonce":          nonce,
		"email":          "one@example.com",
		"email_verified": true,
		"name":           "One",
	}
}

func newTestVerifier(t *testing.T, serverURL string, now time.Time) *TokenVerifier {
	t.Helper()
	verifier, err := NewTokenVerifier(TokenVerifierConfig{
		JWKSURL:  serverURL,
		Issuers:  []string{"https://accounts.google.com", "accounts.google.com"},
		Audience: "client-id",
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}

func TestVerifierAcceptsValidRS256Token(t *testing.T) {
	key := testRSAKey(t)
	server, _ := jwksServer(t, jwksFor(&key.PublicKey, "key-1"))
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	verifier := newTestVerifier(t, server.URL, now)
	token := signToken(t, key, "key-1", nil, validClaims(now, "nonce-1"))
	claims, err := verifier.Verify(context.Background(), token, "nonce-1")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "subject-one" || claims.Email != "one@example.com" || !claims.EmailVerified {
		t.Fatalf("claims = %+v", claims)
	}
}

func TestVerifierRejectsUnsupportedAlgorithms(t *testing.T) {
	key := testRSAKey(t)
	server, _ := jwksServer(t, jwksFor(&key.PublicKey, "key-1"))
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	verifier := newTestVerifier(t, server.URL, now)

	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"https://accounts.google.com","sub":"s","aud":"client-id","exp":1}`)) + "."
	if _, err := verifier.Verify(context.Background(), unsigned, ""); err == nil {
		t.Fatal("unsigned token accepted")
	}

	header := map[string]any{"alg": "HS256", "kid": "key-1", "typ": "JWT"}
	symmetric := signToken(t, key, "key-1", header, validClaims(now, "nonce-1"))
	if _, err := verifier.Verify(context.Background(), symmetric, "nonce-1"); err == nil {
		t.Fatal("HS256 token accepted")
	}
}

func TestVerifierRejectsTamperedClaimsAndSignature(t *testing.T) {
	key := testRSAKey(t)
	server, _ := jwksServer(t, jwksFor(&key.PublicKey, "key-1"))
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	verifier := newTestVerifier(t, server.URL, now)
	token := signToken(t, key, "key-1", nil, validClaims(now, "nonce-1"))
	parts := splitToken(t, token)

	tamperedClaims := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"https://accounts.google.com","sub":"attacker","aud":"client-id","exp":9999999999,"nonce":"nonce-1"}`)) + "." + parts[2]
	if _, err := verifier.Verify(context.Background(), tamperedClaims, "nonce-1"); err == nil {
		t.Fatal("tampered claims accepted")
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	signature[len(signature)-1] ^= 0xff
	tamperedSignature := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(signature)
	if _, err := verifier.Verify(context.Background(), tamperedSignature, "nonce-1"); err == nil {
		t.Fatal("tampered signature accepted")
	}
}

func TestVerifierRejectsIssuerAudienceExpiryAndNonceMismatches(t *testing.T) {
	key := testRSAKey(t)
	server, _ := jwksServer(t, jwksFor(&key.PublicKey, "key-1"))
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	verifier := newTestVerifier(t, server.URL, now)

	mutations := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"issuer", func(claims map[string]any) { claims["iss"] = "https://evil.example" }},
		{"audience", func(claims map[string]any) { claims["aud"] = "another-client" }},
		{"expired", func(claims map[string]any) { claims["exp"] = now.Add(-2 * time.Hour).Unix() }},
		{"nonce", func(claims map[string]any) { claims["nonce"] = "another-nonce" }},
		{"missing-subject", func(claims map[string]any) { delete(claims, "sub") }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			claims := validClaims(now, "nonce-1")
			mutation.mutate(claims)
			token := signToken(t, key, "key-1", nil, claims)
			if _, err := verifier.Verify(context.Background(), token, "nonce-1"); err == nil {
				t.Fatalf("mutated token accepted: %s", mutation.name)
			}
		})
	}
}

func TestVerifierRefetchesJWKSForRotatedKey(t *testing.T) {
	first := testRSAKey(t)
	second := testRSAKey(t)
	var bodies atomic.Value
	bodies.Store(jwksFor(&first.PublicKey, "key-1"))
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(bodies.Load())
	}))
	t.Cleanup(server.Close)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	verifier := newTestVerifier(t, server.URL, now)

	oldToken := signToken(t, first, "key-1", nil, validClaims(now, "nonce-1"))
	if _, err := verifier.Verify(context.Background(), oldToken, "nonce-1"); err != nil {
		t.Fatalf("initial verify: %v", err)
	}
	bodies.Store(jwksFor(&second.PublicKey, "key-2"))
	newToken := signToken(t, second, "key-2", nil, validClaims(now, "nonce-2"))
	if _, err := verifier.Verify(context.Background(), newToken, "nonce-2"); err != nil {
		t.Fatalf("rotated key verify: %v", err)
	}
	if got := calls.Load(); got < 2 {
		t.Fatalf("JWKS calls = %d, want at least 2", got)
	}
}

func TestVerifierCachesJWKS(t *testing.T) {
	key := testRSAKey(t)
	server, calls := jwksServer(t, jwksFor(&key.PublicKey, "key-1"))
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	verifier := newTestVerifier(t, server.URL, now)
	for index := 0; index < 3; index++ {
		token := signToken(t, key, "key-1", nil, validClaims(now, "nonce-1"))
		if _, err := verifier.Verify(context.Background(), token, "nonce-1"); err != nil {
			t.Fatalf("verify %d: %v", index, err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("JWKS calls = %d, want 1", got)
	}
}

func TestVerifierRejectsUnknownKey(t *testing.T) {
	key := testRSAKey(t)
	server, _ := jwksServer(t, jwksFor(&key.PublicKey, "key-1"))
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	verifier := newTestVerifier(t, server.URL, now)
	token := signToken(t, key, "unknown", nil, validClaims(now, "nonce-1"))
	if _, err := verifier.Verify(context.Background(), token, "nonce-1"); err == nil {
		t.Fatal("token with unknown kid accepted")
	}
}

func TestVerifierRejectsSmallRSAKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	server, _ := jwksServer(t, jwksFor(&key.PublicKey, "key-1"))
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	verifier := newTestVerifier(t, server.URL, now)
	token := signToken(t, key, "key-1", nil, validClaims(now, "nonce-1"))
	if _, err := verifier.Verify(context.Background(), token, "nonce-1"); err == nil {
		t.Fatal("1024-bit key accepted")
	}
}

func splitToken(t *testing.T, token string) [3]string {
	t.Helper()
	var parts [3]string
	index := 0
	start := 0
	for position, char := range token {
		if char != '.' {
			continue
		}
		if index >= 3 {
			t.Fatal("too many token parts")
		}
		parts[index] = token[start:position]
		index++
		start = position + 1
	}
	if index != 2 {
		t.Fatalf("token parts = %d", index+1)
	}
	parts[2] = token[start:]
	return parts
}

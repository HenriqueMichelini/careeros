package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	jwksMinCacheTTL = 5 * time.Minute
	jwksMaxCacheTTL = 24 * time.Hour
	clockSkew       = 60 * time.Second
	maxTokenBytes   = 64 << 10
	maxSubjectBytes = 255
	maxEmailBytes   = 320
	maxNameRunes    = 200
)

// Claims are the validated claims CareerOS consumes. Display metadata never
// becomes an ownership key.
type Claims struct {
	Issuer        string
	Subject       string
	Audience      []string
	ExpiresAt     time.Time
	IssuedAt      time.Time
	Nonce         string
	Email         string
	EmailVerified bool
	Name          string
}

type TokenVerifierConfig struct {
	JWKSURL  string
	Issuers  []string
	Audience string
	Client   *http.Client
	Now      func() time.Time
	CacheTTL time.Duration
}

// TokenVerifier validates RS256 ID tokens against a closed issuer set, the
// configured audience, a nonce and a cached JWKS. It rejects unsigned,
// symmetrically signed or unknown-key tokens.
type TokenVerifier struct {
	jwksURL  string
	issuers  map[string]bool
	audience string
	client   *http.Client
	now      func() time.Time
	cacheTTL time.Duration

	mu         sync.Mutex
	keys       map[string]*rsa.PublicKey
	cacheUntil time.Time
}

func NewTokenVerifier(config TokenVerifierConfig) (*TokenVerifier, error) {
	if config.JWKSURL == "" || config.Audience == "" || len(config.Issuers) == 0 {
		return nil, fmt.Errorf("%w: incomplete token verifier configuration", ErrUnavailable)
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = defaultJWKSCacheTTL
	}
	issuers := make(map[string]bool, len(config.Issuers))
	for _, issuer := range config.Issuers {
		if issuer == "" {
			return nil, fmt.Errorf("%w: empty issuer", ErrUnavailable)
		}
		issuers[issuer] = true
	}
	return &TokenVerifier{
		jwksURL:  config.JWKSURL,
		issuers:  issuers,
		audience: config.Audience,
		client:   config.Client,
		now:      config.Now,
		cacheTTL: config.CacheTTL,
	}, nil
}

// Verify validates the compact JWT and returns a typed view of the claims.
// expectedNonce must come from the one-use server-side transaction.
func (v *TokenVerifier) Verify(ctx context.Context, raw, expectedNonce string) (Claims, error) {
	if len(raw) == 0 || len(raw) > maxTokenBytes {
		return Claims{}, fmt.Errorf("%w: token size", ErrInvalidCallback)
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Claims{}, fmt.Errorf("%w: malformed token", ErrInvalidCallback)
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: malformed header", ErrInvalidCallback)
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
		Type      string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return Claims{}, fmt.Errorf("%w: malformed header", ErrInvalidCallback)
	}
	if header.Algorithm != "RS256" {
		return Claims{}, fmt.Errorf("%w: unsupported algorithm", ErrInvalidCallback)
	}
	if header.Type != "" && header.Type != "JWT" {
		return Claims{}, fmt.Errorf("%w: unsupported token type", ErrInvalidCallback)
	}
	if header.KeyID == "" || len(header.KeyID) > 256 {
		return Claims{}, fmt.Errorf("%w: missing key id", ErrInvalidCallback)
	}
	key, err := v.publicKey(ctx, header.KeyID, false)
	if err != nil && errors.Is(err, ErrInvalidCallback) {
		// A key rotation may have introduced a kid after the last cache fill.
		// Refetch once; an unknown kid after refresh remains a rejection.
		key, err = v.publicKey(ctx, header.KeyID, true)
	}
	if err != nil {
		return Claims{}, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: malformed signature", ErrInvalidCallback)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return Claims{}, fmt.Errorf("%w: invalid signature", ErrInvalidCallback)
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: malformed payload", ErrInvalidCallback)
	}
	var rawClaims struct {
		Issuer        string          `json:"iss"`
		Subject       string          `json:"sub"`
		Audience      json.RawMessage `json:"aud"`
		AuthorizedFor string          `json:"azp"`
		ExpiresAt     json.Number     `json:"exp"`
		IssuedAt      json.Number     `json:"iat"`
		NotBefore     json.Number     `json:"nbf"`
		Nonce         string          `json:"nonce"`
		Email         string          `json:"email"`
		EmailVerified bool            `json:"email_verified"`
		Name          string          `json:"name"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.UseNumber()
	if err := decoder.Decode(&rawClaims); err != nil {
		return Claims{}, fmt.Errorf("%w: malformed claims", ErrInvalidCallback)
	}
	if !v.issuers[rawClaims.Issuer] {
		return Claims{}, fmt.Errorf("%w: issuer not allowed", ErrInvalidCallback)
	}
	audience, err := parseAudience(rawClaims.Audience)
	if err != nil || !contains(audience, v.audience) {
		return Claims{}, fmt.Errorf("%w: audience mismatch", ErrInvalidCallback)
	}
	if len(audience) > 1 && rawClaims.AuthorizedFor != v.audience {
		return Claims{}, fmt.Errorf("%w: authorized party mismatch", ErrInvalidCallback)
	}
	if rawClaims.AuthorizedFor != "" && rawClaims.AuthorizedFor != v.audience {
		return Claims{}, fmt.Errorf("%w: authorized party mismatch", ErrInvalidCallback)
	}
	expiresAt, err := requiredUnixTime(rawClaims.ExpiresAt)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: invalid expiry", ErrInvalidCallback)
	}
	now := v.now()
	if !now.Before(expiresAt.Add(clockSkew)) {
		return Claims{}, fmt.Errorf("%w: expired token", ErrInvalidCallback)
	}
	if rawClaims.IssuedAt != "" {
		issuedAt, err := unixTime(rawClaims.IssuedAt)
		if err != nil || issuedAt.After(now.Add(2*clockSkew)) {
			return Claims{}, fmt.Errorf("%w: invalid issued-at", ErrInvalidCallback)
		}
	}
	if rawClaims.NotBefore != "" {
		notBefore, err := unixTime(rawClaims.NotBefore)
		if err != nil || notBefore.After(now.Add(clockSkew)) {
			return Claims{}, fmt.Errorf("%w: token not active", ErrInvalidCallback)
		}
	}
	if subtle.ConstantTimeCompare([]byte(rawClaims.Nonce), []byte(expectedNonce)) != 1 {
		return Claims{}, fmt.Errorf("%w: nonce mismatch", ErrInvalidCallback)
	}
	if rawClaims.Subject == "" || len(rawClaims.Subject) > maxSubjectBytes || hasControl(rawClaims.Subject) {
		return Claims{}, fmt.Errorf("%w: invalid subject", ErrInvalidCallback)
	}
	if len(rawClaims.Email) > maxEmailBytes || hasControl(rawClaims.Email) {
		return Claims{}, fmt.Errorf("%w: invalid email claim", ErrInvalidCallback)
	}
	name := strings.TrimSpace(rawClaims.Name)
	if len([]rune(name)) > maxNameRunes {
		name = string([]rune(name)[:maxNameRunes])
	}
	return Claims{
		Issuer:        rawClaims.Issuer,
		Subject:       rawClaims.Subject,
		Audience:      audience,
		ExpiresAt:     expiresAt,
		IssuedAt:      parseOptionalTime(rawClaims.IssuedAt),
		Nonce:         rawClaims.Nonce,
		Email:         strings.TrimSpace(rawClaims.Email),
		EmailVerified: rawClaims.EmailVerified,
		Name:          name,
	}, nil
}

func (v *TokenVerifier) publicKey(ctx context.Context, keyID string, force bool) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !force && v.keys != nil && v.now().Before(v.cacheUntil) {
		if key, ok := v.keys[keyID]; ok {
			return key, nil
		}
	}
	if err := v.refreshLocked(ctx); err != nil {
		return nil, err
	}
	if key, ok := v.keys[keyID]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("%w: unknown signing key", ErrInvalidCallback)
}

func (v *TokenVerifier) refreshLocked(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("%w: jwks request", ErrUnavailable)
	}
	request.Header.Set("Accept", "application/json")
	response, err := v.client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: jwks request failed", ErrUnavailable)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxProviderResponseBytes))
	if err != nil {
		return fmt.Errorf("%w: jwks response", ErrUnavailable)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%w: jwks endpoint returned %d", ErrUnavailable, response.StatusCode)
	}
	var set struct {
		Keys []struct {
			KeyType   string `json:"kty"`
			KeyID     string `json:"kid"`
			Use       string `json:"use"`
			Algorithm string `json:"alg"`
			Modulus   string `json:"n"`
			Exponent  string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		return fmt.Errorf("%w: malformed jwks", ErrUnavailable)
	}
	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, jwk := range set.Keys {
		if jwk.KeyType != "RSA" || jwk.KeyID == "" {
			continue
		}
		if jwk.Use != "" && jwk.Use != "sig" {
			continue
		}
		if jwk.Algorithm != "" && jwk.Algorithm != "RS256" {
			continue
		}
		key, err := parseRSAKey(jwk.Modulus, jwk.Exponent)
		if err != nil {
			return fmt.Errorf("%w: invalid jwks key", ErrUnavailable)
		}
		keys[jwk.KeyID] = key
	}
	if len(keys) == 0 {
		return fmt.Errorf("%w: jwks has no usable keys", ErrUnavailable)
	}
	v.keys = keys
	ttl := v.cacheTTL
	if ttl == 0 {
		ttl = defaultJWKSCacheTTL
	}
	if maxAge, ok := cacheMaxAge(response.Header.Get("Cache-Control")); ok {
		ttl = maxAge
	}
	if ttl < jwksMinCacheTTL {
		ttl = jwksMinCacheTTL
	}
	if ttl > jwksMaxCacheTTL {
		ttl = jwksMaxCacheTTL
	}
	v.cacheUntil = v.now().Add(ttl)
	return nil
}

func parseRSAKey(modulus, exponent string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil || len(nBytes) == 0 {
		return nil, fmt.Errorf("invalid modulus")
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(exponent)
	if err != nil || len(eBytes) == 0 || len(eBytes) > 4 {
		return nil, fmt.Errorf("invalid exponent")
	}
	e := 0
	for _, value := range eBytes {
		e = e<<8 | int(value)
	}
	if e < 3 {
		return nil, fmt.Errorf("invalid exponent")
	}
	key := &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
	if key.N.BitLen() < 2048 {
		return nil, fmt.Errorf("key too small")
	}
	return key, nil
}

func cacheMaxAge(header string) (time.Duration, bool) {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(strings.ToLower(part), "max-age=") {
			continue
		}
		seconds, err := strconv.Atoi(strings.TrimSpace(part[len("max-age="):]))
		if err != nil || seconds <= 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	return 0, false
}

func parseAudience(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("missing audience")
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if single == "" {
			return nil, fmt.Errorf("empty audience")
		}
		return []string{single}, nil
	}
	var multiple []string
	if err := json.Unmarshal(raw, &multiple); err != nil || len(multiple) == 0 {
		return nil, fmt.Errorf("invalid audience")
	}
	for _, value := range multiple {
		if value == "" {
			return nil, fmt.Errorf("empty audience")
		}
	}
	return multiple, nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func requiredUnixTime(raw json.Number) (time.Time, error) {
	if raw == "" {
		return time.Time{}, fmt.Errorf("missing time")
	}
	return unixTime(raw)
}

func unixTime(raw json.Number) (time.Time, error) {
	seconds, err := raw.Int64()
	if err != nil || seconds <= 0 {
		return time.Time{}, fmt.Errorf("invalid time")
	}
	return time.Unix(seconds, 0).UTC(), nil
}

func parseOptionalTime(raw json.Number) time.Time {
	if raw == "" {
		return time.Time{}
	}
	value, err := unixTime(raw)
	if err != nil {
		return time.Time{}
	}
	return value
}

func hasControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

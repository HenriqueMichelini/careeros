package auth

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	defaultSessionTTL     = 12 * time.Hour
	defaultTransactionTTL = 10 * time.Minute
	defaultHTTPTimeout    = 10 * time.Second
	defaultJWKSCacheTTL   = time.Hour

	maxSessionTTL     = 30 * 24 * time.Hour
	maxTransactionTTL = time.Hour
)

// ProviderConfig is one explicitly allowlisted identity provider. Endpoints
// are configuration rather than discovery so tests can run offline and the
// production set is reviewable.
type ProviderConfig struct {
	ID                    string
	ClientID              string
	ClientSecret          string
	RedirectURI           string
	Issuers               []string
	AuthorizationEndpoint string
	TokenEndpoint         string
	JWKSURL               string
	Scopes                []string
}

// Config is the complete auth configuration. It contains no implicit
// provider: every entry in Providers must be selected explicitly.
type Config struct {
	PublicOrigin   string
	SessionTTL     time.Duration
	TransactionTTL time.Duration
	JWKSCacheTTL   time.Duration
	HTTPClient     *http.Client
	Now            func() time.Time
	Providers      []ProviderConfig
}

var providerIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ConfigFromEnv builds the production configuration. A missing or invalid
// configuration is an error; callers must not silently fall back to a guest or
// password mode.
func ConfigFromEnv() (Config, error) {
	origin := strings.TrimSpace(os.Getenv("CAREEROS_PUBLIC_ORIGIN"))
	clientID := strings.TrimSpace(os.Getenv("CAREEROS_GOOGLE_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("CAREEROS_GOOGLE_CLIENT_SECRET"))
	redirectURI := strings.TrimSpace(os.Getenv("CAREEROS_GOOGLE_REDIRECT_URI"))

	cfg := Config{
		PublicOrigin:   origin,
		SessionTTL:     durationFromEnv("CAREEROS_SESSION_TTL", defaultSessionTTL),
		TransactionTTL: durationFromEnv("CAREEROS_TRANSACTION_TTL", defaultTransactionTTL),
		JWKSCacheTTL:   durationFromEnv("CAREEROS_JWKS_CACHE_TTL", defaultJWKSCacheTTL),
	}
	if clientID != "" || clientSecret != "" {
		provider := DefaultGoogleProviderConfig(origin, clientID, clientSecret)
		if redirectURI != "" {
			provider.RedirectURI = redirectURI
		}
		cfg.Providers = append(cfg.Providers, provider)
	}
	if len(cfg.Providers) == 0 {
		return Config{}, fmt.Errorf("%w: no provider is configured", ErrUnavailable)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func durationFromEnv(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

// Validate rejects configurations that could widen the approved surface:
// arbitrary redirect targets, non-loopback plain HTTP, weak lifetimes or a
// provider set without an explicit OIDC issuer.
func (c Config) Validate() error {
	if c.PublicOrigin == "" {
		return fmt.Errorf("%w: CAREEROS_PUBLIC_ORIGIN is required", ErrUnavailable)
	}
	origin, err := url.Parse(c.PublicOrigin)
	if err != nil || origin.Scheme == "" || origin.Host == "" || origin.User != nil ||
		origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return fmt.Errorf("%w: CAREEROS_PUBLIC_ORIGIN must be an absolute origin without a path", ErrUnavailable)
	}
	origin.Scheme = strings.ToLower(origin.Scheme)
	origin.Host = strings.ToLower(origin.Host)
	if origin.Scheme != "https" && !(origin.Scheme == "http" && isLoopbackHost(origin.Hostname())) {
		return fmt.Errorf("%w: CAREEROS_PUBLIC_ORIGIN must use HTTPS except on loopback", ErrUnavailable)
	}
	if c.SessionTTL < 5*time.Minute || c.SessionTTL > maxSessionTTL {
		return fmt.Errorf("%w: session lifetime must be between 5 minutes and 30 days", ErrUnavailable)
	}
	if c.TransactionTTL < time.Minute || c.TransactionTTL > maxTransactionTTL {
		return fmt.Errorf("%w: transaction lifetime must be between 1 minute and 1 hour", ErrUnavailable)
	}
	if len(c.Providers) == 0 {
		return fmt.Errorf("%w: no provider is configured", ErrUnavailable)
	}
	seen := map[string]bool{}
	for _, provider := range c.Providers {
		if err := provider.validate(strings.TrimRight(c.PublicOrigin, "/")); err != nil {
			return err
		}
		if seen[provider.ID] {
			return fmt.Errorf("%w: duplicate provider %q", ErrUnavailable, provider.ID)
		}
		seen[provider.ID] = true
	}
	return nil
}

func (p ProviderConfig) validate(publicOrigin string) error {
	if !providerIDPattern.MatchString(p.ID) {
		return fmt.Errorf("%w: invalid provider id", ErrUnavailable)
	}
	if p.ClientID == "" || len(p.ClientID) > 512 || p.ClientSecret == "" || len(p.ClientSecret) > 1024 {
		return fmt.Errorf("%w: provider %q credentials are incomplete", ErrUnavailable, p.ID)
	}
	if len(p.Issuers) == 0 {
		return fmt.Errorf("%w: provider %q has no issuer", ErrUnavailable, p.ID)
	}
	for _, issuer := range p.Issuers {
		if issuer == "" || len(issuer) > 255 || hasControl(issuer) {
			return fmt.Errorf("%w: provider %q has an invalid issuer", ErrUnavailable, p.ID)
		}
		parsed, err := url.Parse(issuer)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			if parsed.User != nil || parsed.Fragment != "" {
				return fmt.Errorf("%w: provider %q has an invalid issuer", ErrUnavailable, p.ID)
			}
			if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
				return fmt.Errorf("%w: provider %q issuer must use HTTPS except on loopback", ErrUnavailable, p.ID)
			}
			continue
		}
		// Google documents both https://accounts.google.com and the bare
		// accounts.google.com issuer form. No other bare issuer is accepted.
		if p.ID == "google" && issuer == "accounts.google.com" {
			continue
		}
		return fmt.Errorf("%w: provider %q has an invalid issuer", ErrUnavailable, p.ID)
	}
	expectedRedirect := publicOrigin + "/api/auth/" + p.ID + "/callback"
	if p.RedirectURI != expectedRedirect {
		return fmt.Errorf("%w: provider %q redirect URI must be %q", ErrUnavailable, p.ID, expectedRedirect)
	}
	if err := validateEndpoint(p.AuthorizationEndpoint); err != nil {
		return fmt.Errorf("%w: provider %q authorization endpoint: %v", ErrUnavailable, p.ID, err)
	}
	if err := validateEndpoint(p.TokenEndpoint); err != nil {
		return fmt.Errorf("%w: provider %q token endpoint: %v", ErrUnavailable, p.ID, err)
	}
	if err := validateEndpoint(p.JWKSURL); err != nil {
		return fmt.Errorf("%w: provider %q JWKS endpoint: %v", ErrUnavailable, p.ID, err)
	}
	hasOpenID := false
	for _, scope := range p.Scopes {
		hasOpenID = hasOpenID || scope == "openid"
	}
	if !hasOpenID {
		return fmt.Errorf("%w: provider %q must request the openid scope", ErrUnavailable, p.ID)
	}
	return nil
}

func validateEndpoint(raw string) error {
	if raw == "" {
		return fmt.Errorf("missing endpoint")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("invalid absolute URL")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return fmt.Errorf("must use HTTPS except on loopback")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// DefaultGoogleProviderConfig is the production provider definition. It is
// still inert until the owner authorizes credentials and configuration.
func DefaultGoogleProviderConfig(publicOrigin, clientID, clientSecret string) ProviderConfig {
	return ProviderConfig{
		ID:                    "google",
		ClientID:              clientID,
		ClientSecret:          clientSecret,
		RedirectURI:           strings.TrimRight(publicOrigin, "/") + "/api/auth/google/callback",
		Issuers:               []string{"https://accounts.google.com", "accounts.google.com"},
		AuthorizationEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenEndpoint:         "https://oauth2.googleapis.com/token",
		JWKSURL:               "https://www.googleapis.com/oauth2/v3/certs",
		Scopes:                []string{"openid", "email", "profile"},
	}
}

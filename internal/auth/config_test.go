package auth

import (
	"strings"
	"testing"
	"time"
)

func baseConfig() Config {
	return Config{
		PublicOrigin:   "https://careeros.example",
		SessionTTL:     time.Hour,
		TransactionTTL: 10 * time.Minute,
		Providers: []ProviderConfig{{
			ID:                    "google",
			ClientID:              "client-id",
			ClientSecret:          "client-secret",
			RedirectURI:           "https://careeros.example/api/auth/google/callback",
			Issuers:               []string{"https://accounts.google.com"},
			AuthorizationEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
			TokenEndpoint:         "https://oauth2.googleapis.com/token",
			JWKSURL:               "https://www.googleapis.com/oauth2/v3/certs",
			Scopes:                []string{"openid", "email", "profile"},
		}},
	}
}

func TestConfigAcceptsExactGoogleOnlySetup(t *testing.T) {
	if err := baseConfig().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestConfigRejectsArbitraryRedirect(t *testing.T) {
	config := baseConfig()
	config.Providers[0].RedirectURI = "https://evil.example/callback"
	err := config.Validate()
	if err == nil || !strings.Contains(err.Error(), "redirect URI") {
		t.Fatalf("error = %v, want redirect URI rejection", err)
	}
}

func TestConfigRejectsPlainHTTPOutsideLoopback(t *testing.T) {
	config := baseConfig()
	config.PublicOrigin = "http://careeros.example"
	if err := config.Validate(); err == nil {
		t.Fatal("plain HTTP public origin accepted")
	}
	config = baseConfig()
	config.Providers[0].RedirectURI = "http://careeros.example/api/auth/google/callback"
	if err := config.Validate(); err == nil {
		t.Fatal("plain HTTP redirect URI accepted")
	}
}

func TestConfigRejectsDuplicateProviders(t *testing.T) {
	config := baseConfig()
	config.Providers = append(config.Providers, config.Providers[0])
	if err := config.Validate(); err == nil {
		t.Fatal("duplicate provider accepted")
	}
}

func TestConfigFromEnvRequiresOrigin(t *testing.T) {
	t.Setenv("CAREEROS_PUBLIC_ORIGIN", "")
	t.Setenv("CAREEROS_GOOGLE_CLIENT_ID", "client")
	t.Setenv("CAREEROS_GOOGLE_CLIENT_SECRET", "secret")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("missing public origin accepted")
	}
}

func TestConfigFromEnvSelectsOnlyGoogle(t *testing.T) {
	t.Setenv("CAREEROS_PUBLIC_ORIGIN", "https://careeros.example")
	t.Setenv("CAREEROS_GOOGLE_CLIENT_ID", "client")
	t.Setenv("CAREEROS_GOOGLE_CLIENT_SECRET", "secret")
	t.Setenv("CAREEROS_GOOGLE_REDIRECT_URI", "")
	t.Setenv("CAREEROS_SESSION_TTL", "2h")
	t.Setenv("CAREEROS_TRANSACTION_TTL", "5m")
	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if len(config.Providers) != 1 || config.Providers[0].ID != "google" {
		t.Fatalf("providers = %+v", config.Providers)
	}
	if config.Providers[0].RedirectURI != "https://careeros.example/api/auth/google/callback" {
		t.Fatalf("redirect URI = %q", config.Providers[0].RedirectURI)
	}
	if config.SessionTTL != 2*time.Hour || config.TransactionTTL != 5*time.Minute {
		t.Fatalf("lifetimes = %v / %v", config.SessionTTL, config.TransactionTTL)
	}
}

func TestRegistryDeniesUnlistedProvider(t *testing.T) {
	env := newTestEnv(t)
	if _, ok := env.service.providers.Get("github"); ok {
		t.Fatal("unlisted provider resolved")
	}
	if _, err := env.service.BeginLogin(t.Context(), "github", "/", nil); err != ErrProviderNotAllowed {
		t.Fatalf("error = %v, want ErrProviderNotAllowed", err)
	}
}

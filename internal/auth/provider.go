package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	googleAuthorizationEndpoint = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenEndpoint         = "https://oauth2.googleapis.com/token"
	googleJWKSURL               = "https://www.googleapis.com/oauth2/v3/certs"
	maxProviderResponseBytes    = 1 << 20
)

// Identity is a provider assertion after signature and claim validation. The
// email is display metadata only; Subject is the only provider identity key.
type Identity struct {
	Provider      string
	Issuer        string
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// Provider is one allowlisted OIDC provider. Implementations must validate the
// ID token before returning an Identity.
type Provider interface {
	ID() string
	AuthorizationURL(state, nonce, codeChallenge, redirectURI string) string
	Exchange(ctx context.Context, code, codeVerifier, redirectURI, nonce string) (Identity, error)
}

// Registry is a closed provider set. Lookup failure is a denial, not a
// fallback.
type Registry struct {
	providers map[string]Provider
}

func NewRegistry(providers ...Provider) (*Registry, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("%w: empty provider registry", ErrUnavailable)
	}
	registry := &Registry{providers: make(map[string]Provider, len(providers))}
	for _, provider := range providers {
		if provider == nil || provider.ID() == "" {
			return nil, fmt.Errorf("%w: invalid provider", ErrUnavailable)
		}
		if _, exists := registry.providers[provider.ID()]; exists {
			return nil, fmt.Errorf("%w: duplicate provider %q", ErrUnavailable, provider.ID())
		}
		registry.providers[provider.ID()] = provider
	}
	return registry, nil
}

func (r *Registry) Get(id string) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	provider, ok := r.providers[id]
	return provider, ok
}

func (r *Registry) IDs() []string {
	if r == nil {
		return nil
	}
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	return ids
}

// GoogleProvider performs the authorization-code flow directly against
// Google's OIDC endpoints. No broker, password or implicit linking is involved.
type GoogleProvider struct {
	config   ProviderConfig
	client   *http.Client
	verifier *TokenVerifier
}

func NewGoogleProvider(config ProviderConfig, client *http.Client, now func() time.Time) (*GoogleProvider, error) {
	if config.ID == "" {
		config.ID = "google"
	}
	if config.AuthorizationEndpoint == "" {
		config.AuthorizationEndpoint = googleAuthorizationEndpoint
	}
	if config.TokenEndpoint == "" {
		config.TokenEndpoint = googleTokenEndpoint
	}
	if config.JWKSURL == "" {
		config.JWKSURL = googleJWKSURL
	}
	if len(config.Scopes) == 0 {
		config.Scopes = []string{"openid", "email", "profile"}
	}
	if len(config.Issuers) == 0 {
		config.Issuers = []string{"https://accounts.google.com", "accounts.google.com"}
	}
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	if now == nil {
		now = time.Now
	}
	verifier, err := NewTokenVerifier(TokenVerifierConfig{
		JWKSURL:  config.JWKSURL,
		Issuers:  config.Issuers,
		Audience: config.ClientID,
		Client:   client,
		Now:      now,
	})
	if err != nil {
		return nil, err
	}
	return &GoogleProvider{config: config, client: client, verifier: verifier}, nil
}

func (p *GoogleProvider) ID() string { return p.config.ID }

func (p *GoogleProvider) AuthorizationURL(state, nonce, codeChallenge, redirectURI string) string {
	query := url.Values{}
	query.Set("client_id", p.config.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(p.config.Scopes, " "))
	query.Set("state", state)
	query.Set("nonce", nonce)
	query.Set("code_challenge", codeChallenge)
	query.Set("code_challenge_method", "S256")
	query.Set("access_type", "online")
	query.Set("prompt", "select_account")
	endpoint := p.config.AuthorizationEndpoint
	if strings.Contains(endpoint, "?") {
		return endpoint + "&" + query.Encode()
	}
	return endpoint + "?" + query.Encode()
}

func (p *GoogleProvider) Exchange(ctx context.Context, code, codeVerifier, redirectURI, nonce string) (Identity, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", p.config.ClientID)
	form.Set("client_secret", p.config.ClientSecret)
	form.Set("code_verifier", codeVerifier)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Identity{}, fmt.Errorf("%w: token request", ErrUnavailable)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: token request failed", ErrUnavailable)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxProviderResponseBytes))
	if err != nil {
		return Identity{}, fmt.Errorf("%w: token response", ErrUnavailable)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Identity{}, fmt.Errorf("%w: token endpoint returned %d", ErrInvalidCallback, response.StatusCode)
	}
	var tokenResponse struct {
		IDToken          string `json:"id_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return Identity{}, fmt.Errorf("%w: malformed token response", ErrInvalidCallback)
	}
	if tokenResponse.Error != "" || tokenResponse.IDToken == "" {
		return Identity{}, fmt.Errorf("%w: no usable ID token", ErrInvalidCallback)
	}
	claims, err := p.verifier.Verify(ctx, tokenResponse.IDToken, nonce)
	if err != nil {
		return Identity{}, err
	}
	if claims.Subject == "" {
		return Identity{}, fmt.Errorf("%w: missing subject", ErrInvalidCallback)
	}
	return Identity{
		Provider:      p.config.ID,
		Issuer:        claims.Issuer,
		Subject:       claims.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
		Name:          claims.Name,
	}, nil
}

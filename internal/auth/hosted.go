package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type googleSubject struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}

// NewHostedGoogleService creates the closed hosted candidate's Google-only
// authority. Exactly two independently approved subjects are required for the
// isolation observation. The allowlist acts on verified claims before mapping
// Accounts or creating Sessions. Email is never used for access.
func NewHostedGoogleService(config Config, stores Stores, allowlistJSON string) (*Service, error) {
	if len(config.Providers) != 1 || !strings.HasPrefix(config.PublicOrigin, "https://") {
		return nil, fmt.Errorf("%w: hosted Google configuration required", ErrUnavailable)
	}
	providerConfig := config.Providers[0]
	if providerConfig.ID != "google" || providerConfig.AuthorizationEndpoint != googleAuthorizationEndpoint ||
		providerConfig.TokenEndpoint != googleTokenEndpoint || providerConfig.JWKSURL != googleJWKSURL {
		return nil, fmt.Errorf("%w: hosted Google endpoints required", ErrUnavailable)
	}
	subjects, err := hostedGoogleSubjects(allowlistJSON, providerConfig.Issuers)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout: defaultHTTPTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if config.HTTPClient == nil || config.HTTPClient.Transport == nil {
		defaultTransport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, fmt.Errorf("%w: standard hosted HTTP transport required", ErrUnavailable)
		}
		transport := defaultTransport.Clone()
		transport.Proxy = nil
		client.Transport = transport
	} else {
		client.Transport = config.HTTPClient.Transport
	}
	provider, err := NewGoogleProvider(providerConfig, client, config.Now, config.JWKSCacheTTL)
	if err != nil {
		return nil, err
	}
	registry, err := NewRegistry(provider)
	if err != nil {
		return nil, err
	}
	service, err := NewService(config, registry, stores)
	if err != nil {
		return nil, err
	}
	service.hostedSubjects = subjects
	return service, nil
}

func hostedGoogleSubjects(value string, issuers []string) (map[googleSubject]struct{}, error) {
	invalid := fmt.Errorf("%w: two distinct approved Google identities required", ErrUnavailable)
	if len(value) == 0 || len(value) > 4096 {
		return nil, invalid
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	var entries []googleSubject
	if err := decoder.Decode(&entries); err != nil || len(entries) != 2 {
		return nil, invalid
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, invalid
	}
	approved := make(map[googleSubject]struct{}, len(entries))
	for _, entry := range entries {
		if (entry.Issuer != "https://accounts.google.com" && entry.Issuer != "accounts.google.com") ||
			!hasScope(issuers, entry.Issuer) || entry.Subject == "" || len(entry.Subject) > maxSubjectBytes ||
			entry.Subject != strings.TrimSpace(entry.Subject) || hasControl(entry.Subject) {
			return nil, invalid
		}
		approved[entry] = struct{}{}
	}
	// Google documents both issuer spellings for the same identity system. A
	// single subject in both forms cannot be the two-account isolation fixture.
	if entries[0].Subject == entries[1].Subject {
		return nil, invalid
	}
	return approved, nil
}

func (s *Service) permitsSubject(provider, issuer, subject string) bool {
	if s.hostedSubjects == nil {
		return true
	}
	_, approved := s.hostedSubjects[googleSubject{Issuer: issuer, Subject: subject}]
	return provider == "google" && approved
}

func (s *Service) permitsAccount(account Account) bool {
	if s.hostedSubjects == nil {
		return true
	}
	// Accounts in this candidate originate through one Google subject only.
	// Refuse restored/imported accounts with a different or ambiguous binding.
	identities := account.Identities
	return len(identities) == 1 && s.permitsSubject(identities[0].Provider, identities[0].Issuer, identities[0].Subject)
}

func (s *Service) verifyHostedAccountBinding(ctx context.Context, account Account) error {
	if s.hostedSubjects == nil {
		return nil
	}
	// The subject lookup may return just the matched identity. Read the complete
	// binding before issuing an opaque Session to an existing account.
	current, found, err := s.accounts.Get(ctx, account.ID)
	if err != nil {
		return err
	}
	if !found || current.Status != account.Status || current.Generation != account.Generation ||
		!s.permitsAccount(current) || len(account.Identities) != 1 {
		return ErrForbidden
	}
	bound, resolved := current.Identities[0], account.Identities[0]
	if bound.Provider != resolved.Provider || bound.Issuer != resolved.Issuer || bound.Subject != resolved.Subject {
		return ErrForbidden
	}
	return nil
}

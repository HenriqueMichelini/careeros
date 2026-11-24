package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

const approvedHostedSubjects = `[
 {"issuer":"https://accounts.google.com","subject":"subject-one"},
 {"issuer":"https://accounts.google.com","subject":"subject-two"}
]`

func TestHostedGoogleSubjectsAreClosedAndUnambiguous(t *testing.T) {
	invalid := []string{
		"",
		`null`,
		`[]`,
		`[{"issuer":"https://accounts.google.com","subject":"subject-one"}]`,
		approvedHostedSubjects + ` null`,
		`[{"issuer":"https://accounts.google.com","email":"one@example.com"},{"issuer":"https://accounts.google.com","subject":"subject-two"}]`,
		`[{"issuer":"https://accounts.google.com","subject":"subject-one"},{"issuer":"https://example.invalid","subject":"subject-two"}]`,
		`[{"issuer":"https://accounts.google.com","subject":"subject-one"},{"issuer":"accounts.google.com","subject":"subject-one"}]`,
		`[{"issuer":"https://accounts.google.com","subject":" subject-one"},{"issuer":"https://accounts.google.com","subject":"subject-two"}]`,
	}
	config := newTestEnv(t).config
	config.PublicOrigin = "https://candidate.example"
	config.Providers = []ProviderConfig{DefaultGoogleProviderConfig(config.PublicOrigin, "synthetic-id", "synthetic-secret")}
	for i, value := range invalid {
		_, err := NewHostedGoogleService(config, NewMemoryStores(), value)
		if !errors.Is(err, ErrUnavailable) {
			t.Errorf("case %d: hosted configuration returned %v", i, err)
		}
	}
	if _, err := NewHostedGoogleService(config, NewMemoryStores(), approvedHostedSubjects); err != nil {
		t.Fatal(err)
	}
}

func TestHostedGoogleSubjectDenialPrecedesAccountAndSession(t *testing.T) {
	env := newTestEnv(t)
	issuedBeforeSelection := env.loginAs(t, "unapproved-subject", "one@example.com")
	var err error
	env.service.hostedSubjects, err = hostedGoogleSubjects(approvedHostedSubjects, env.config.Providers[0].Issuers)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := env.service.Authenticate(ctx, issuedBeforeSelection.SessionToken); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("old unapproved session: %v", err)
	}
	if _, err := env.service.Revalidate(ctx, issuedBeforeSelection.Principal); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("old unapproved principal: %v", err)
	}
	accounts := env.stores.Accounts.(*MemoryAccountStore)
	deny := func(wrong Identity) {
		t.Helper()
		env.provider.identity = wrong
		before := len(accounts.accounts)
		start, state := env.start(t, nil)
		result, err := env.service.CompleteLogin(ctx, "google", state, "code", start.BindingToken)
		if !errors.Is(err, ErrForbidden) || result.Account.ID != "" || result.SessionToken != "" || len(accounts.accounts) != before {
			t.Fatalf("unapproved login reached account or session: %v", err)
		}
		if _, err := env.service.CompleteLogin(ctx, "google", state, "code", start.BindingToken); !errors.Is(err, ErrInvalidTransaction) {
			t.Fatalf("used transaction replay: %v", err)
		}
	}
	deny(Identity{Provider: "google", Issuer: "https://accounts.google.com", Subject: "new-subject", Email: "one@example.com"})
	deny(Identity{Provider: "google", Issuer: "accounts.google.com", Subject: "subject-one", Email: "one@example.com"})
	env.provider.identity.Issuer = "https://accounts.google.com"
	first := env.loginAs(t, "subject-one", "one@example.com")
	second := env.loginAs(t, "subject-two", "one@example.com")
	if first.Account.ID == second.Account.ID {
		t.Fatal("distinct allowed subjects shared an account")
	}
	if _, err := env.service.Authenticate(ctx, first.SessionToken); err != nil {
		t.Fatalf("allowed session: %v", err)
	}
	stored := accounts.accounts[first.Account.ID]
	stored.Identities = append(stored.Identities, IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "unexpected-alias"})
	accounts.accounts[first.Account.ID] = stored
	env.provider.identity.Subject = "subject-one"
	start, state := env.start(t, nil)
	if result, err := env.service.CompleteLogin(ctx, "google", state, "code", start.BindingToken); !errors.Is(err, ErrForbidden) || result.SessionToken != "" {
		t.Fatalf("ambiguous existing binding obtained a session: %v", err)
	}
}

type hostedRoundTripFunc func(*http.Request) (*http.Response, error)

func (f hostedRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHostedGoogleExchangeDoesNotFollowRedirect(t *testing.T) {
	env := newTestEnv(t)
	config := env.config
	config.PublicOrigin = "https://candidate.example"
	config.Providers = []ProviderConfig{DefaultGoogleProviderConfig(config.PublicOrigin, "synthetic-id", "synthetic-secret")}
	calls := 0
	config.HTTPClient = &http.Client{Transport: hostedRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != googleTokenEndpoint {
			t.Fatalf("request was redirected beyond the pinned Google endpoint: %s", r.URL.Host)
		}
		return &http.Response{
			StatusCode: http.StatusTemporaryRedirect,
			Header:     http.Header{"Location": {"https://external.example.invalid/receive"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})}
	var err error
	env.service, err = NewHostedGoogleService(config, env.stores, approvedHostedSubjects)
	if err != nil {
		t.Fatal(err)
	}
	start, state := env.start(t, nil)
	_, err = env.service.CompleteLogin(context.Background(), "google", state, "synthetic-code", start.BindingToken)
	if !errors.Is(err, ErrInvalidCallback) || calls != 1 {
		t.Fatalf("redirect response reached an unreviewed target, calls=%d error=%v", calls, err)
	}
}

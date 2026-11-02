// pilot-local is a synthetic-only operator surface. It never contacts Google,
// an AI provider, email or a participant. Production composition is unchanged.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

type fakeProvider struct{}

func (fakeProvider) ID() string { return "google" }
func (fakeProvider) AuthorizationURL(state, nonce, challenge, redirect string) string {
	return "https://synthetic.invalid/?state=" + url.QueryEscape(state)
}
func (fakeProvider) Exchange(_ context.Context, code, verifier, redirect, nonce string) (auth.Identity, error) {
	if !syntheticSubject(code) {
		return auth.Identity{}, auth.ErrForbidden
	}
	return auth.Identity{Provider: "google", Issuer: "https://accounts.google.com", Subject: code}, nil
}
func syntheticSubject(subject string) bool {
	return strings.HasPrefix(subject, "synthetic-") && len(subject) > 10 && len(subject) < 128
}
func identity(subject string) auth.IdentityRef {
	return auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: subject}
}
func signin(ctx context.Context, service *auth.Service, subject string) (auth.LoginResult, error) {
	start, err := service.BeginLogin(ctx, "google", "/", nil)
	if err != nil {
		return auth.LoginResult{}, err
	}
	u, err := url.Parse(start.URL)
	if err != nil {
		return auth.LoginResult{}, err
	}
	return service.CompleteLogin(ctx, "google", u.Query().Get("state"), subject, start.BindingToken)
}
func run() error {
	synthetic := flag.Bool("synthetic-only", false, "required: disposable loopback database and fake identities only")
	action := flag.String("action", "", "identity, bootstrap, invite, clear, suspend, revoke")
	subject := flag.String("subject", "", "synthetic target subject")
	operation := flag.String("operation-id", "", "stable idempotency key for operator command")
	flag.Parse()
	dsn := os.Getenv("CAREEROS_PILOT_LOCAL_DATABASE_URL")
	u, err := url.Parse(dsn)
	if !*synthetic || err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || !strings.HasSuffix(u.Path, "_test") || !syntheticSubject(*subject) {
		return fmt.Errorf("requires --synthetic-only, synthetic subject and disposable loopback *_test PostgreSQL")
	}
	// Reject alternate routing through connection-string options or multi-host DSNs.
	for key := range u.Query() {
		if key != "sslmode" && key != "search_path" {
			return fmt.Errorf("unsupported local DSN option")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := accountstore.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		return err
	}
	cfg := auth.Config{PublicOrigin: "http://127.0.0.1:8787", SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig("http://127.0.0.1:8787", "synthetic-client", "synthetic-secret")}}
	registry, err := auth.NewRegistry(fakeProvider{})
	if err != nil {
		return err
	}
	service, err := auth.NewService(cfg, registry, store.AuthStores())
	if err != nil {
		return err
	}
	if *action == "identity" {
		who, e := signin(ctx, service, *subject)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"accountId": who.Account.ID})
	}
	if *action == "bootstrap" {
		return store.BootstrapLocalPilotOperator(ctx, identity(*subject))
	}
	who, err := service.Authenticate(ctx, os.Getenv("CAREEROS_PILOT_LOCAL_SESSION_TOKEN"))
	if err != nil {
		return err
	}
	status, err := store.PilotCommand(ctx, who, *operation, accountstore.PilotCommand{Kind: *action, Identity: identity(*subject)})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(status)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// control-local is a synthetic-only operator adapter. It installs no provider,
// recovery verifier, email transport or participant configuration.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"strings"
	"time"
)

func run() error {
	synthetic := flag.Bool("synthetic-only", false, "required disposable local execution")
	action := flag.String("action", "status", "status, bootstrap, initialize, command")
	subject := flag.String("subject", "", "existing synthetic operator subject; bootstrap only")
	target := flag.String("account", "", "exact compromise account identity")
	control := flag.String("control", "", "invitations, inference, compromise")
	stop := flag.Bool("stop", false, "stop; omission requests reopening and requires verified recovery")
	version := flag.Int64("version", 0, "expected immutable control version")
	operation := flag.String("operation-id", "", "stable command identity")
	recovery := flag.String("recovery-reference", "", "previous exact recorded recovery reference; a label grants no authority")
	flag.Parse()
	dsn := os.Getenv("CAREEROS_CONTROL_LOCAL_DATABASE_URL")
	u, err := url.Parse(dsn)
	if !*synthetic || err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || !strings.HasSuffix(u.Path, "_test") {
		return fmt.Errorf("requires --synthetic-only and disposable loopback *_test PostgreSQL")
	}
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
	if *action == "bootstrap" {
		if !strings.HasPrefix(*subject, "synthetic-") || len(*subject) > 127 {
			return fmt.Errorf("requires an existing synthetic operator identity")
		}
		return store.BootstrapLocalControlOperator(ctx, auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: *subject})
	}
	cfg := auth.Config{PublicOrigin: "http://127.0.0.1:8787", SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig("http://127.0.0.1:8787", "synthetic-client", "synthetic-secret")}}
	// Authentication uses only persisted session hashes. No OIDC exchange or
	// automatic sign-in is installed by this command.
	registry, err := auth.NewRegistry(sessionOnlyProvider{})
	if err != nil {
		return err
	}
	service, err := auth.NewService(cfg, registry, store.AuthStores())
	if err != nil {
		return err
	}
	principal, err := service.Authenticate(ctx, os.Getenv("CAREEROS_CONTROL_LOCAL_SESSION_TOKEN"))
	if err != nil {
		principal, err = service.AuthenticateDeletionStatus(ctx, os.Getenv("CAREEROS_CONTROL_LOCAL_SESSION_TOKEN"))
	}
	if err != nil {
		return err
	}
	controls := store.LocalControls(nil)
	var status accountstore.ControlStatus
	switch *action {
	case "status":
		status, err = controls.Status(ctx, principal, *target)
	case "initialize":
		status, err = controls.InitializeSynthetic(ctx, principal, *operation)
	case "command":
		status, err = controls.Apply(ctx, principal, *operation, accountstore.ControlCommand{Control: *control, Stop: *stop, ExpectedVersion: *version, AccountID: *target, RecoveryReference: *recovery})
	default:
		return fmt.Errorf("unsupported local control action")
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(status)
}

type sessionOnlyProvider struct{}

func (sessionOnlyProvider) ID() string                                             { return "google" }
func (sessionOnlyProvider) AuthorizationURL(string, string, string, string) string { return "" }
func (sessionOnlyProvider) Exchange(context.Context, string, string, string, string) (auth.Identity, error) {
	return auth.Identity{}, auth.ErrForbidden
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

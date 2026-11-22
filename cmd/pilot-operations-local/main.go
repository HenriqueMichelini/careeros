// pilot-operations-local runs the synthetic pilot procedures and the
// independent status reader. It installs neither OIDC exchange nor real mail.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
)

type options struct {
	action, kind, impact, state, at, listen, subject, role, id string
	revision                                                   int
	synthetic                                                  bool
}

func main() {
	var o options
	flag.BoolVar(&o.synthetic, "synthetic-only", false, "required local exercise")
	flag.StringVar(&o.action, "action", "status", "local operations command")
	flag.StringVar(&o.kind, "kind", "", "typed fault, case command, or recovery limitation")
	flag.StringVar(&o.impact, "impact", "", "confirmed public impact")
	flag.StringVar(&o.state, "state", "", "unresolved public status")
	flag.StringVar(&o.at, "next-update", "", "next update as RFC3339")
	flag.StringVar(&o.listen, "listen", "", "private loopback address for independent status")
	flag.StringVar(&o.subject, "subject", "", "existing synthetic identity for bootstrap")
	flag.StringVar(&o.role, "role", "", "owner or support for bootstrap")
	flag.StringVar(&o.id, "id", "", "incident, case or verified recovery reference")
	flag.IntVar(&o.revision, "revision", 0, "incident revision")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(o options) error {
	if !o.synthetic {
		return fmt.Errorf("--synthetic-only is required")
	}
	raw, err := hex.DecodeString(os.Getenv("CAREEROS_OPERATIONS_JOURNAL_KEY"))
	if err != nil || len(raw) != 32 {
		return fmt.Errorf("configure the private local operations journal")
	}
	var key [32]byte
	copy(key[:], raw)
	config := accountstore.OperationsJournalConfig{
		Directory: os.Getenv("CAREEROS_OPERATIONS_JOURNAL_DIRECTORY"), BindingKey: key,
		Notify: func(_ context.Context, n accountstore.OperationsNotification) accountstore.OperationsDelivery {
			if json.NewEncoder(os.Stderr).Encode(n) != nil {
				return accountstore.OperationsFailed
			}
			return accountstore.OperationsDelivered
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	switch o.action {
	case "serve-status":
		status, err := accountstore.OpenLocalOperationsStatus(config)
		if err != nil {
			return err
		}
		host, _, e := net.SplitHostPort(o.listen)
		if e != nil || !net.ParseIP(host).IsLoopback() {
			return fmt.Errorf("use an explicit loopback status address")
		}
		server := http.Server{Addr: o.listen, Handler: accountapi.NewPilotPublicStatus(status), ReadHeaderTimeout: 3 * time.Second,
			WriteTimeout: 5 * time.Second, IdleTimeout: 10 * time.Second}
		return server.ListenAndServe()
	case "publish":
		status, err := accountstore.OpenLocalOperationsStatus(config)
		if err != nil {
			return err
		}
		next, e := time.Parse(time.RFC3339, o.at)
		if e != nil {
			return accountstore.ErrValidation
		}
		return status.PublishOutage(ctx, o.impact, o.state, next)
	}
	journal, err := accountstore.OpenLocalOperationsJournal(config)
	if err != nil {
		return err
	}
	if o.action == "fault" {
		return journal.SignalOfflineFault(ctx, accountstore.OperationsFault(o.kind))
	}
	return runDatabase(ctx, o, journal)
}

func runDatabase(ctx context.Context, o options, journal *accountstore.OperationsJournal) error {
	dsn := os.Getenv("CAREEROS_OPERATIONS_DATABASE_URL")
	if !aicontrol.LocalReconciliationDSN(dsn) {
		return fmt.Errorf("requires disposable loopback *_test PostgreSQL")
	}
	store, err := accountstore.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Close()
	if o.action == "bootstrap" {
		if !strings.HasPrefix(o.subject, "synthetic-") || len(o.subject) > 127 {
			return accountstore.ErrValidation
		}
		return store.BootstrapLocalOperationsRole(ctx, auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: o.subject}, o.role)
	}
	registry, err := auth.NewRegistry(sessionOnlyProvider{})
	if err != nil {
		return err
	}
	origin := "http://127.0.0.1:8787"
	service, err := auth.NewService(auth.Config{PublicOrigin: origin, SessionTTL: time.Hour, TransactionTTL: time.Minute,
		Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(origin, "synthetic-client", "synthetic-secret")}}, registry, store.AuthStores())
	if err != nil {
		return err
	}
	token := os.Getenv("CAREEROS_OPERATIONS_SESSION_TOKEN")
	p, err := service.Authenticate(ctx, token)
	if err != nil {
		p, err = service.AuthenticateDeletionStatus(ctx, token)
	}
	if err != nil {
		return err
	}
	recon, err := aicontrol.OpenLocalReconciliation(ctx, os.Getenv("CAREEROS_OPERATIONS_READER_URL"), os.Getenv("CAREEROS_OPERATIONS_NAMESPACE"))
	if err != nil {
		return err
	}
	defer recon.Close()
	source, err := recon.LocalOperationsSource(ctx)
	if err != nil {
		return err
	}
	ops, err := store.LocalOperations(accountstore.OperationsConfig{Build: "synthetic-local", Owner: "Henrique"}, source, journal)
	if err != nil {
		return err
	}
	out := json.NewEncoder(os.Stdout)
	switch o.action {
	case "status", "inspect":
		if o.action == "inspect" {
			if _, err = ops.OwnerStatus(ctx, p); err != nil {
				return err
			}
			if err = ops.Inspect(ctx); err != nil {
				return err
			}
		}
		return ops.WithOwnerStatus(ctx, p, func(s accountstore.OperationsStatus) error { return out.Encode(s) })
	case "ack", "resolve":
		if err = ops.ReviewIncident(ctx, p, o.id, o.revision, o.action == "resolve"); err != nil {
			return err
		}
		return ops.Deliver(ctx)
	case "case":
		return ops.WithCase(ctx, p, o.id, false, func(c accountstore.OperationsCase) error { return out.Encode(c) })
	case "intake":
		var c accountstore.OperationsCaseInput
		if err = decodeInput(&c); err != nil {
			return err
		}
		result, err := ops.CreateCase(ctx, p, c)
		if err != nil {
			return err
		}
		return out.Encode(map[string]string{"id": result.ID})
	case "case-update":
		var input struct{ Response string }
		if err = decodeInput(&input); err != nil {
			return err
		}
		return ops.UpdateCase(ctx, p, o.id, o.kind, input.Response)
	case "case-hold", "case-hold-retire":
		var next time.Time
		if o.action == "case-hold" {
			next, err = time.Parse(time.RFC3339, o.at)
			if err != nil {
				return accountstore.ErrValidation
			}
		}
		return ops.ReviewCaseHold(ctx, p, o.id, o.kind, next, o.action == "case-hold-retire")
	case "prune":
		return ops.PruneOrdinaryRecords(ctx, p)
	case "absence":
		var plan accountstore.OperationsAbsence
		if err = decodeInput(&plan); err != nil {
			return err
		}
		result, err := ops.PlanAbsence(ctx, p, plan)
		if err != nil {
			return err
		}
		return out.Encode(result)
	case "publish-recovery":
		return ops.PublishVerifiedRecovery(ctx, p, o.id, o.kind)
	default:
		return accountstore.ErrValidation
	}
}

func decodeInput(out any) error {
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 12<<10+1))
	if err != nil || len(raw) > 12<<10 || !accountstore.StrictJSON(raw) {
		return accountstore.ErrValidation
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return accountstore.ErrValidation
	}
	return nil
}

type sessionOnlyProvider struct{}

func (sessionOnlyProvider) ID() string                                             { return "google" }
func (sessionOnlyProvider) AuthorizationURL(string, string, string, string) string { return "" }
func (sessionOnlyProvider) Exchange(context.Context, string, string, string, string) (auth.Identity, error) {
	return auth.Identity{}, auth.ErrForbidden
}

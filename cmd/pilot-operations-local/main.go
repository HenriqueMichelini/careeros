// pilot-operations-local runs the synthetic pilot procedures and the
// independent status reader. It installs neither OIDC exchange nor real mail.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"professional-information-repo/internal/operationslocal"
)

type options struct {
	action, kind, impact, state, at, listen, subject, role, id string
	revision                                                   int
	synthetic, newEpisode                                      bool
	reference                                                  string
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
	flag.StringVar(&o.reference, "reference", "", "independently verified recovery reference")
	flag.BoolVar(&o.newEpisode, "new-episode", false, "start a distinct public outage episode")
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
	config, configured, err := operationslocal.JournalConfig(os.Stderr)
	if err != nil || !configured {
		return fmt.Errorf("configure the private local operations journal")
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
		if o.newEpisode {
			return status.BeginOutage(ctx, o.impact, o.state, next)
		}
		return status.PublishOutage(ctx, o.impact, o.state, next)
	case "outage-episode":
		status, err := accountstore.OpenLocalOperationsStatus(config)
		if err != nil {
			return err
		}
		id, err := status.Episode(ctx)
		if err != nil {
			return err
		}
		if id == "" {
			return accountstore.ErrNotFound
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"episode": id})
	}
	journal, err := accountstore.OpenLocalOperationsJournal(config)
	if o.action == "fault" {
		kind := accountstore.OperationsFault(o.kind)
		if err == nil {
			err = journal.SignalOfflineFault(ctx, kind)
		}
		if err == nil || errors.Is(err, accountstore.ErrValidation) && journal != nil {
			return err
		}
		if dsn := os.Getenv("CAREEROS_OPERATIONS_DATABASE_URL"); dsn != "" {
			return errors.Join(err, signalFallback(ctx, dsn, config, kind))
		}
		return err
	}
	if o.action == "status" && err != nil {
		return runDatabase(ctx, o, nil)
	}
	if err != nil {
		return err
	}
	if o.action == "offline-status" {
		key, err := offlineOwnerKey()
		if err != nil {
			return err
		}
		return journal.WithOfflineOwnerStatus(ctx, key, func(s accountstore.OperationsStatus) error { return json.NewEncoder(os.Stdout).Encode(s) })
	}
	return runDatabase(ctx, o, journal)
}

func signalFallback(ctx context.Context, dsn string, config accountstore.OperationsJournalConfig, kind accountstore.OperationsFault) error {
	if !aicontrol.LocalReconciliationDSN(dsn) {
		return auth.ErrForbidden
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	store, err := accountstore.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Close()
	return store.SignalLocalOperationsFallback(ctx, config, kind)
}

func offlineOwnerKey() ([32]byte, error) {
	var key [32]byte
	raw, err := hex.DecodeString(os.Getenv("CAREEROS_OPERATIONS_OFFLINE_OWNER_KEY"))
	if err != nil || len(raw) != len(key) {
		return key, auth.ErrForbidden
	}
	copy(key[:], raw)
	return key, nil
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
		return err
	}
	if journal == nil {
		return store.WithOperationsEmergencyStatus(ctx, p, func(status accountstore.OperationsStatus) error {
			return json.NewEncoder(os.Stdout).Encode(status)
		})
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
	cfgOps := accountstore.OperationsConfig{Build: "synthetic-local", Owner: "Henrique"}
	if o.action == "resolve" || o.action == "publish-recovery" || o.action == "replay-containment" && o.reference != "" {
		verifier, err := operationslocal.Recovery()
		if err != nil {
			return err
		}
		cfgOps.VerifyBackupRecovery = verifier.VerifyBackup
		cfgOps.VerifyProviderRecovery = verifier.VerifyProvider
		cfgOps.VerifyServiceRecovery = verifier.VerifyService
		cfgOps.VerifyEmergencyReview = verifier.VerifyEmergency
	}
	ops, err := store.LocalOperations(cfgOps, source, journal)
	if err != nil {
		return err
	}
	out := json.NewEncoder(os.Stdout)
	switch o.action {
	case "replay-containment":
		return ops.ReplayEmergency(ctx, p, o.reference)
	case "offline-setup":
		key, err := offlineOwnerKey()
		if err != nil {
			return err
		}
		return ops.ProvisionOfflineOwner(ctx, p, key)
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
		if o.action == "resolve" {
			err = ops.ResolveIncident(ctx, p, o.id, o.revision, o.reference)
		} else {
			err = ops.ReviewIncident(ctx, p, o.id, o.revision, false)
		}
		if err != nil {
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
		if o.kind == "respond" {
			if err = decodeInput(&input); err != nil {
				return err
			}
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
		return ops.PublishVerifiedRecoveryWithServiceProof(ctx, p, o.id, o.kind, o.reference)
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

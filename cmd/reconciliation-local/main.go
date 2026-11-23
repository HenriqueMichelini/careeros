// reconciliation-local is closed unless explicitly selected against a named,
// disposable loopback database. It never contacts providers or reads bill files.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/operationslocal"
)

type accountReader struct{ db *sql.DB }

func (a accountReader) Get(ctx context.Context, id string) (auth.Account, bool, error) {
	var out auth.Account
	err := a.db.QueryRowContext(ctx, `SELECT id,status,generation FROM careeros_accounts WHERE id=$1`, id).Scan(&out.ID, &out.Status, &out.Generation)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	return out, err == nil, err
}
func (accountReader) FindOrCreateBySubject(context.Context, auth.IdentityRef, string, string, time.Time) (auth.Account, bool, error) {
	return auth.Account{}, false, auth.ErrForbidden
}
func (accountReader) BumpGeneration(context.Context, string, time.Time) (auth.Account, error) {
	return auth.Account{}, auth.ErrForbidden
}

type disabledProvider struct{}

func (disabledProvider) ID() string                                             { return "google" }
func (disabledProvider) AuthorizationURL(string, string, string, string) string { return "" }
func (disabledProvider) Exchange(context.Context, string, string, string, string) (auth.Identity, error) {
	return auth.Identity{}, auth.ErrForbidden
}

func run() error {
	synthetic := flag.Bool("synthetic-only", false, "required disposable local fixture acknowledgement")
	mode := flag.String("mode", "", "preview, apply, status, events or rates; install is migrator-only")
	namespace := flag.String("namespace", "", "existing ledger namespace")
	dispatch := flag.String("dispatch", "", "status target dispatch")
	flag.Parse()
	dsn := os.Getenv("CAREEROS_RECONCILIATION_LOCAL_DATABASE_URL")
	if !*synthetic || !aicontrol.LocalReconciliationDSN(dsn) {
		return errors.New("requires --synthetic-only and a disposable loopback *_test database/schema")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// Only session reads are used from the existing store. The restricted account
	// projection excludes email, identity subjects and all career content.
	store, err := accountstore.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Close()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	registry, err := auth.NewRegistry(disabledProvider{})
	if err != nil {
		return err
	}
	cfg := auth.Config{PublicOrigin: "http://127.0.0.1:8787", SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig("http://127.0.0.1:8787", "synthetic-client", "synthetic-secret")}}
	stores := store.AuthStores()
	stores.Accounts = accountReader{db: db}
	stores.Transactions = auth.NewMemoryTransactionStore()
	service, err := auth.NewService(cfg, registry, stores)
	if err != nil {
		return err
	}
	sessionToken := os.Getenv("CAREEROS_RECONCILIATION_LOCAL_SESSION_TOKEN")
	principal, err := service.Authenticate(ctx, sessionToken)
	if err != nil {
		return err
	}
	if *mode == "install" {
		// This separate migrator operation requires the authenticated owner session
		// and a DDL/admin connection; ordinary reconciliation roles cannot elevate.
		return aicontrol.InstallLocalReconciliation(ctx, db, *namespace, principal.AccountID, time.Now().Add(time.Hour))
	}
	command, err := aicontrol.OpenLocalReconciliation(ctx, dsn, *namespace)
	if err != nil {
		return err
	}
	defer command.Close()
	if *mode == "apply" {
		config, configured, err := operationslocal.JournalConfig(os.Stderr)
		if err != nil {
			return err
		}
		if configured {
			journal, err := accountstore.OpenLocalOperationsJournal(config)
			if err != nil {
				return err
			}
			source, err := command.LocalOperationsSource(ctx)
			if err != nil {
				return err
			}
			if _, err = store.AttachLocalOperationsObserver(ctx, accountstore.OperationsConfig{Build: "synthetic-local", Owner: "Henrique"}, source, journal); err != nil {
				return err
			}
		}
	}
	var result any
	if *mode == "status" {
		result, err = command.Status(ctx, principal, *dispatch)
	} else if *mode == "events" {
		result, err = command.Events(ctx, principal)
	} else if *mode == "rates" {
		result, err = command.RateRevisions(ctx, principal, *dispatch)
	} else {
		raw, e := io.ReadAll(io.LimitReader(os.Stdin, 64*1024+1))
		if e != nil || len(raw) > 64*1024 || !accountstore.StrictJSON(raw) {
			return errors.New("invalid bounded command")
		}
		var input aicontrol.ReconciliationCommand
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if e = decoder.Decode(&input); e != nil {
			return errors.New("invalid command fields")
		}
		switch *mode {
		case "preview":
			result, err = command.Preview(ctx, principal, input)
		case "apply":
			result, err = command.ApplyLocal(ctx, principal, input, sessionToken)
		default:
			return errors.New("unsupported mode")
		}
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

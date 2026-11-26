package accountstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"professional-information-repo/internal/auth"
)

const (
	hostedDraftSchema   = "careeros_294_draft"
	hostedDraftLedger   = "careeros_294_draft_ledger"
	hostedDraftAttestor = "careeros_294_draft_attestor"
)

// OpenHostedDraftLedger uses the Account candidate's exact database endpoint
// and a distinct login. Each canonical component owns its own two-slot pool.
func OpenHostedDraftLedger(ctx context.Context, accountDSN, ledgerDSN string) (*sql.DB, error) {
	account, err := hostedPilotConfig(accountDSN, hostedPilotRuntime)
	if err != nil {
		return nil, err
	}
	ledger, err := hostedPilotConfig(ledgerDSN, hostedDraftLedger)
	if err != nil || ledger.Host != account.Host || ledger.Port != account.Port || ledger.Database != account.Database {
		return nil, auth.ErrUnavailable
	}
	ledger.RuntimeParams["application_name"] = "careeros-294-draft-ledger"
	ledger.RuntimeParams["search_path"] = hostedDraftSchema + "," + hostedPilotSchema + ",pg_catalog"
	return openHostedDraftPool(ctx, sql.OpenDB(stdlib.GetConnector(*ledger)))
}

// OpenHostedDraftAttestor has no Account or canonical serving credentials.
func OpenHostedDraftAttestor(ctx context.Context, dsn string) (*sql.DB, error) {
	config, err := hostedPilotConfig(dsn, hostedDraftAttestor)
	if err != nil {
		return nil, err
	}
	config.RuntimeParams["application_name"] = "careeros-294-draft-attestor"
	config.RuntimeParams["search_path"] = "pg_catalog"
	return openHostedDraftPool(ctx, sql.OpenDB(stdlib.GetConnector(*config)))
}

func openHostedDraftPool(ctx context.Context, db *sql.DB) (*sql.DB, error) {
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, auth.ErrUnavailable
	}
	return db, nil
}

// RunHostedDraftMigration uses the closed Account migration role and the
// canonical stop fence. Every additive Draft migration must commit together.
func RunHostedDraftMigration(ctx context.Context, dsn string, migrate func(context.Context, *sql.Tx) error) error {
	if migrate == nil {
		return auth.ErrUnavailable
	}
	store, err := openHostedPilotDatabase(ctx, dsn, hostedPilotMigrator)
	if err != nil {
		return err
	}
	defer store.Close()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return auth.ErrUnavailable
	}
	defer tx.Rollback()
	if err := authorizeHostedPilotMigration(ctx, tx); err != nil {
		return err
	}
	var allowed bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_294.careeros_hosted_account_identity
WHERE singleton AND schema_id=$1) AND EXISTS (SELECT 1 FROM pg_namespace n JOIN pg_roles r ON n.nspowner=r.oid
WHERE n.nspname='careeros_294_draft' AND r.rolname=current_user)`, HostedPilotSchemaID()).Scan(&allowed)
	if err != nil || !allowed {
		return auth.ErrUnavailable
	}
	if _, err := tx.ExecContext(ctx, "SET LOCAL search_path=careeros_294_draft,careeros_294,pg_catalog"); err != nil {
		return auth.ErrUnavailable
	}
	if err := migrate(ctx, tx); err != nil {
		return err
	}
	return unavailable(tx.Commit())
}

// HostedPilotInvitationClosed retains the full Account readiness checks and
// reports the observed inference stop for the explicitly scoped Draft mode.
func (s *Store) HostedPilotInvitationClosed(ctx context.Context) (inferenceStopped, admissionAvailable bool, err error) {
	if err := s.HostedPilotReady(ctx); err != nil {
		return false, false, err
	}
	var invitationStopped bool
	err = s.db.QueryRowContext(ctx, `SELECT invitation_stopped,inference_stopped,
initialized AND NOT inference_stopped AND NOT operations_fault AND operations_binding=''
FROM careeros_294.careeros_control_state WHERE singleton`).Scan(&invitationStopped, &inferenceStopped, &admissionAvailable)
	if err != nil || !invitationStopped {
		return false, false, auth.ErrUnavailable
	}
	return inferenceStopped, admissionAvailable, nil
}

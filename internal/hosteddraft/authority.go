// Package hosteddraft contains the separate fake Draft authority migration.
// The closed hosted server neither installs nor connects to this authority.
package hosteddraft

import (
	"context"
	"database/sql"
	_ "embed"
)

//go:embed bootstrap.sql
var bootstrapSQL string

//go:embed migrate.sql
var migrationSQL string

//go:embed attestor.sql
var attestationSQL string

// BootstrapSQL is the isolated administrator's transaction after the Account
// bootstrap and creation of the independently authenticated Draft login roles.
func BootstrapSQL() string { return bootstrapSQL }

// MigrationSQL is the schema owner's transaction after the Account migration.
// It installs no test scope, and therefore has no positive path by default.
func MigrationSQL() string {
	return "BEGIN;\nSET LOCAL search_path=pg_catalog,pg_temp;\n" + migrationSQL + "COMMIT;\n"
}

// MigrateScope installs the closed singleton scope and scoped role functions
// before the canonical ledger. The caller owns the selected migration transaction.
func MigrateScope(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, migrationSQL)
	return err
}

// MigrateAttestation installs independent proof after the canonical Dispatch
// tables exist, under the same selected migration transaction.
func MigrateAttestation(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, attestationSQL)
	return err
}

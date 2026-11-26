// Package hosteddraft contains the separate fake Draft authority migration.
// The closed hosted server neither installs nor connects to this authority.
package hosteddraft

import _ "embed"

//go:embed bootstrap.sql
var bootstrapSQL string

//go:embed migrate.sql
var migrationSQL string

// BootstrapSQL is the isolated administrator's transaction after the Account
// bootstrap and creation of the independently authenticated Draft login roles.
func BootstrapSQL() string { return bootstrapSQL }

// MigrationSQL is the schema owner's transaction after the Account migration.
// It installs no test scope, and therefore has no positive path by default.
func MigrationSQL() string { return migrationSQL }

package accountstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"professional-information-repo/internal/auth"
)

const (
	pilotPackageDatabase = "careeros_pilot_runtime_fixture"
	pilotPackageSchema   = "careeros_pilot_runtime"
	pilotPackageRole     = "careeros_pilot_runtime"
)

// PilotPackageSchemaID identifies the exact account migration embedded in both
// the local executable and the account migrator built from the same source.
func PilotPackageSchemaID() string { return fmt.Sprintf("%x", sha256.Sum256([]byte(schema))) }

// OpenClosedPilotPackage uses the existing account store with a bounded,
// explicitly disposable PostgreSQL connection. Local test role credentials
// come from the fixture's private enrollment, never a URL or ambient pgpass.
func OpenClosedPilotPackage(ctx context.Context, dsn string) (*Store, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != "postgresql" || u.User == nil || u.User.String() != pilotPackageRole ||
		u.Path != "/"+pilotPackageDatabase || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery ||
		u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || os.Getenv("PGSERVICE") != "" {
		return nil, auth.ErrUnavailable
	}
	address, err := netip.ParseAddrPort(u.Host)
	if err != nil || !address.Addr().IsLoopback() || address.Addr().Zone() != "" || address.Port() == 0 {
		return nil, auth.ErrUnavailable
	}
	// Resolve no hostname and allow no alternate target, credentials, TLS
	// material, service mapping or caller-supplied startup options.
	u.Host = address.String()
	u.RawQuery = "sslmode=disable&passfile=/dev/null&password=&target_session_attrs=any&connect_timeout=2"
	config, err := pgx.ParseConfig(u.String())
	if err != nil {
		return nil, auth.ErrUnavailable
	}
	config.Password = ""
	config.Fallbacks = nil
	config.KerberosSrvName, config.KerberosSpn = "", ""
	config.RuntimeParams = map[string]string{
		"application_name":                    "careeros-pilot-local",
		"search_path":                         pilotPackageSchema + ",pg_catalog",
		"statement_timeout":                   "5000",
		"lock_timeout":                        "2000",
		"idle_in_transaction_session_timeout": "5000",
	}
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	config.DialFunc = func(ctx context.Context, network, target string) (net.Conn, error) {
		peer, err := netip.ParseAddrPort(target)
		if network != "tcp" || err != nil || peer != address {
			return nil, auth.ErrUnavailable
		}
		return dialer.DialContext(ctx, network, target)
	}
	return openStore(ctx, sql.OpenDB(stdlib.GetConnector(*config)))
}

// ClosedPilotPackageReady checks that the exact schema was explicitly applied
// by the separate migrator and that the leased authority is still closed.
// Startup does not create a schema, a role, a grant, or an operator.
func (s *Store) ClosedPilotPackageReady(ctx context.Context) error {
	var ready bool
	err := s.db.QueryRowContext(ctx, `
SELECT current_database()=$1 AND current_schema()=$2 AND current_user=$3 AND session_user=$3
 AND NOT has_schema_privilege(current_user,current_schema(),'CREATE')
 AND NOT has_database_privilege(current_user,current_database(),'TEMP')
 AND NOT has_any_column_privilege(current_user,'careeros_pilot_runtime.careeros_control_state','UPDATE')
 AND NOT has_any_column_privilege(current_user,'careeros_pilot_runtime.careeros_pilot_access','UPDATE')
 AND NOT EXISTS (
   SELECT 1 FROM pg_roles WHERE rolname=current_user AND
    (rolsuper OR rolcreaterole OR rolcreatedb OR rolinherit OR rolreplication OR rolbypassrls))
 AND NOT EXISTS (SELECT 1 FROM pg_auth_members WHERE member=(SELECT oid FROM pg_roles WHERE rolname=current_user))
 AND m.schema_id=$4 AND c.invitation_stopped AND c.inference_stopped
FROM careeros_pilot_runtime.careeros_local_package_identity m
CROSS JOIN careeros_pilot_runtime.careeros_control_state c
WHERE m.singleton AND c.singleton`, pilotPackageDatabase, pilotPackageSchema, pilotPackageRole, PilotPackageSchemaID()).Scan(&ready)
	if err != nil || !ready {
		return auth.ErrUnavailable
	}
	return nil
}

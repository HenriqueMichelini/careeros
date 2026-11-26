package accountstore

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	_ "embed"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"professional-information-repo/internal/auth"
)

const (
	hostedPilotDatabase = "careeros_294"
	hostedPilotSchema   = "careeros_294"
	hostedPilotRuntime  = "careeros_294_runtime"
	hostedPilotMigrator = "careeros_294_migrator"
)

//go:embed hosted_bootstrap.sql
var hostedBootstrap string

//go:embed hosted_grants.sql
var hostedGrants string

var hostedHost = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$`)

// HostedPilotBootstrapSQL is the separately run administrative schema bootstrap.
// It accepts no credentials or administrative operations from serving code.
func HostedPilotBootstrapSQL() string { return hostedBootstrap }

// HostedPilotSchemaID binds the hosted Account schema and exact serving grants.
func HostedPilotSchemaID() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(schema+"\x00"+hostedGrants)))
}

// OpenHostedPilot opens a verified direct TLS connection pool for the narrow
// runtime role and checks the migrated authority without running any DDL.
func OpenHostedPilot(ctx context.Context, dsn string) (*Store, error) {
	store, err := openHostedPilotDatabase(ctx, dsn, hostedPilotRuntime)
	if err != nil {
		return nil, err
	}
	if err := store.HostedPilotReady(ctx); err != nil {
		store.Close()
		return nil, err
	}
	store.hostedProfilePruning = true
	return store, nil
}

// MigrateHostedPilot runs only with the separate, isolated schema owner. It
// applies schema, serving grants and the identity marker in one transaction.
func MigrateHostedPilot(ctx context.Context, dsn string) error {
	store, err := openHostedPilotDatabase(ctx, dsn, hostedPilotMigrator)
	if err != nil {
		return err
	}
	defer store.Close()
	return store.migrate(ctx, authorizeHostedPilotMigration, finalizeHostedPilotMigration)
}

func openHostedPilotDatabase(ctx context.Context, dsn, role string) (*Store, error) {
	config, err := hostedPilotConfig(dsn, role)
	if err != nil {
		return nil, err
	}
	return openStore(ctx, sql.OpenDB(stdlib.GetConnector(*config)))
}

func hostedPilotConfig(dsn, role string) (*pgx.ConnConfig, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != "postgresql" || u.User == nil || u.User.Username() != role ||
		u.Path != "/"+hostedPilotDatabase || u.RawPath != "" || u.Opaque != "" ||
		u.Fragment != "" || u.RawFragment != "" || u.RawQuery != "sslmode=verify-full" || os.Getenv("PGSERVICE") != "" {
		return nil, auth.ErrUnavailable
	}
	password, present := u.User.Password()
	host, port := u.Hostname(), u.Port()
	_, ipErr := netip.ParseAddr(host)
	if !present || password == "" || len(host) > 253 || !hostedHost.MatchString(host) || ipErr == nil ||
		strings.HasSuffix(strings.SplitN(host, ".", 2)[0], "-pooler") {
		return nil, auth.ErrUnavailable
	}
	if port == "" {
		port = "5432"
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 || (u.Host != host && u.Host != host+":"+port) {
		return nil, auth.ErrUnavailable
	}
	// Bind TLS policy, the single socket target, and provider-neutral timeouts.
	// No service file, fallback, ambient password, client cert or startup option
	// can rewrite the reviewed connection. Database errors may contain secrets.
	u.Host = host + ":" + port
	u.RawQuery = "sslmode=verify-full&sslrootcert=system&passfile=/dev/null&sslcert=&sslkey=&sslsni=1&sslnegotiation=postgres&target_session_attrs=any&connect_timeout=2"
	config, err := pgx.ParseConfig(u.String())
	if err != nil || config.TLSConfig == nil || len(config.Fallbacks) != 0 ||
		config.Host != host || config.Port != uint16(portNumber) || config.Database != hostedPilotDatabase || config.User != role {
		return nil, auth.ErrUnavailable
	}
	config.TLSConfig.MinVersion = tls.VersionTLS12
	config.KerberosSrvName, config.KerberosSpn = "", ""
	config.RuntimeParams = map[string]string{
		"application_name":                    "careeros-294-runtime",
		"search_path":                         hostedPilotSchema + ",pg_catalog",
		"statement_timeout":                   "5000",
		"lock_timeout":                        "2000",
		"idle_in_transaction_session_timeout": "5000",
	}
	if role == hostedPilotMigrator {
		config.RuntimeParams["application_name"] = "careeros-294-migrate"
		config.RuntimeParams["statement_timeout"] = "25000"
	}
	return config, nil
}

const hostedRoleCheck = `
current_database()='careeros_294' AND current_schema()='careeros_294' AND current_user=$1 AND session_user=$1
AND r.rolcanlogin AND r.rolconnlimit BETWEEN 1 AND $2
AND NOT (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolinherit OR r.rolreplication OR r.rolbypassrls)
AND NOT EXISTS (SELECT 1 FROM pg_auth_members membership WHERE membership.member=r.oid)
AND NOT has_database_privilege(current_user,current_database(),'CREATE,TEMP')
AND NOT EXISTS (SELECT 1 FROM pg_namespace n WHERE ($1<>'careeros_294_migrator' OR
                (n.nspname<>'careeros_294' AND NOT (n.nspname='careeros_294_draft' AND n.nspowner=r.oid)))
                AND has_schema_privilege(current_user,n.oid,'CREATE'))`

func authorizeHostedPilotMigration(ctx context.Context, tx *sql.Tx) error {
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT `+hostedRoleCheck+`
AND EXISTS (SELECT 1 FROM pg_namespace WHERE nspname='careeros_294' AND nspowner=r.oid)
FROM pg_roles r WHERE r.rolname=current_user`, hostedPilotMigrator, 2).Scan(&allowed)
	if err != nil || !allowed {
		return auth.ErrUnavailable
	}
	if err := LockControlStopTransaction(ctx, tx); err != nil {
		return err
	}
	exists, err := installed(ctx, tx, "careeros_294.careeros_control_state")
	if err != nil {
		return err
	}
	if exists {
		err = tx.QueryRowContext(ctx, `SELECT invitation_stopped AND inference_stopped
FROM careeros_294.careeros_control_state WHERE singleton`).Scan(&allowed)
		if err != nil || !allowed {
			return auth.ErrUnavailable
		}
	}
	return nil
}

func finalizeHostedPilotMigration(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, hostedGrants); err != nil {
		return unavailable(err)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO careeros_hosted_account_identity(singleton,schema_id) VALUES (true,$1)
ON CONFLICT (singleton) DO UPDATE SET schema_id=EXCLUDED.schema_id`, HostedPilotSchemaID())
	return unavailable(err)
}

// HostedPilotReady verifies the Account schema and serving role. The closed
// startup gate must additionally verify both authoritative stops.
func (s *Store) HostedPilotReady(ctx context.Context) error {
	return s.hostedPilotReady(ctx, false)
}

// HostedPilotClosed checks readiness and both durable stop flags together.
func (s *Store) HostedPilotClosed(ctx context.Context) error {
	return s.hostedPilotReady(ctx, true)
}

func (s *Store) hostedPilotReady(ctx context.Context, requireClosed bool) error {
	if s == nil || s.db == nil {
		return auth.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var ready bool
	err := s.db.QueryRowContext(ctx, `SELECT `+hostedRoleCheck+`
AND NOT EXISTS (
 SELECT 1 FROM (VALUES
  ('careeros_accounts', ARRAY['SELECT','INSERT']),
  ('careeros_identities', ARRAY['SELECT','INSERT']),
  ('careeros_sessions', ARRAY['SELECT','INSERT','DELETE']),
  ('careeros_deletion_status_sessions', ARRAY['SELECT','INSERT','DELETE']),
  ('careeros_oidc_transactions', ARRAY['SELECT','INSERT','DELETE']),
  ('careeros_deletion_operations', ARRAY['SELECT'])
 ) required(relation, privileges)
 CROSS JOIN LATERAL unnest(required.privileges) AS grant_kind(name)
 WHERE NOT has_table_privilege(current_user,'careeros_294.'||required.relation,grant_kind.name)
)
AND has_column_privilege(current_user,'careeros_294.careeros_accounts','generation','UPDATE')
AND has_column_privilege(current_user,'careeros_294.careeros_sessions','created_at','UPDATE')
AND has_column_privilege(current_user,'careeros_294.careeros_deletion_status_sessions','created_at','UPDATE')
AND has_function_privilege(current_user,'careeros_294.careeros_control_account_denied(text)','EXECUTE')
AND NOT EXISTS (
 SELECT 1 FROM unnest(ARRAY['account_id','operation_id','request_hash','profile_id','committed_revision']) required(column_name)
 WHERE NOT has_column_privilege(current_user,'careeros_294.careeros_profile_operations',required.column_name,'SELECT')
    OR NOT has_column_privilege(current_user,'careeros_294.careeros_profile_operations',required.column_name,'INSERT')
)
AND NOT has_column_privilege(current_user,'careeros_294.careeros_profile_operations','hosted_save_transaction','SELECT,INSERT')
AND EXISTS (
 SELECT 1 FROM pg_attribute attr JOIN pg_attrdef default_value ON default_value.adrelid=attr.attrelid
   AND default_value.adnum=attr.attnum
 WHERE attr.attrelid='careeros_294.careeros_profile_operations'::regclass AND attr.attname='hosted_save_transaction'
   AND attr.atttypid='xid8'::regtype AND pg_get_expr(default_value.adbin,default_value.adrelid)='pg_current_xact_id()'
)
AND EXISTS (
 SELECT 1 FROM pg_trigger trigger_spec JOIN pg_proc routine ON routine.oid=trigger_spec.tgfoid
 WHERE trigger_spec.tgrelid='careeros_294.careeros_profiles'::regclass
   AND trigger_spec.tgname='careeros_hosted_profile_pruning'
   AND trigger_spec.tgenabled='O' AND trigger_spec.tgtype=21
   AND trigger_spec.tgdeferrable AND trigger_spec.tginitdeferred AND trigger_spec.tgqual IS NULL
   AND trigger_spec.tgconstraint<>0 AND trigger_spec.tgnargs=0
   AND routine.oid='careeros_294.careeros_hosted_profile_pruning()'::regprocedure
   AND routine.prosecdef AND pg_get_userbyid(routine.proowner)='careeros_294_migrator'
   AND routine.proconfig=ARRAY['search_path=pg_catalog, pg_temp']
   AND NOT has_function_privilege(current_user,routine.oid,'EXECUTE')
   AND NOT EXISTS (
     SELECT 1 FROM aclexplode(COALESCE(routine.proacl,acldefault('f',routine.proowner))) acl
     WHERE acl.grantee=0 AND acl.privilege_type='EXECUTE'
   )
)
AND NOT EXISTS (
 SELECT 1 FROM pg_class relation JOIN pg_namespace n ON n.oid=relation.relnamespace
 WHERE n.nspname='careeros_294' AND relation.relkind IN ('r','p','v','m','f') AND (
  (relation.relname NOT IN ('careeros_accounts','careeros_identities','careeros_sessions',
     'careeros_deletion_status_sessions','careeros_oidc_transactions','careeros_profiles','careeros_profile_operations')
   AND has_any_column_privilege(current_user,relation.oid,'INSERT'))
  OR (relation.relname NOT IN ('careeros_sessions','careeros_deletion_status_sessions','careeros_oidc_transactions')
      AND has_table_privilege(current_user,relation.oid,'DELETE'))
  OR has_table_privilege(current_user,relation.oid,'TRUNCATE,TRIGGER')
  OR (has_any_column_privilege(current_user,relation.oid,'UPDATE') AND EXISTS (
    SELECT 1 FROM pg_attribute column_def
    WHERE column_def.attrelid=relation.oid AND column_def.attnum>0 AND NOT column_def.attisdropped
      AND has_column_privilege(current_user,relation.oid,column_def.attnum,'UPDATE')
      AND NOT ((relation.relname='careeros_accounts' AND column_def.attname='generation')
        OR (relation.relname IN ('careeros_sessions','careeros_deletion_status_sessions') AND column_def.attname='created_at')
        OR (relation.relname='careeros_profiles' AND column_def.attname IN ('profile_id','revision','document')))
  ))
 )
)
AND NOT EXISTS (
 SELECT 1 FROM pg_proc routine JOIN pg_namespace n ON n.oid=routine.pronamespace
 WHERE n.nspname='careeros_294' AND routine.prosecdef
   AND routine.oid NOT IN ('careeros_294.careeros_account_closed(text)'::regprocedure,
                           'careeros_294.careeros_deleted_profile_restored(text)'::regprocedure,
                           'careeros_294.careeros_control_account_denied(text)'::regprocedure)
   AND has_function_privilege(current_user,routine.oid,'EXECUTE')
)
AND m.schema_id=$3 AND (NOT $4 OR (c.invitation_stopped AND c.inference_stopped))
FROM pg_roles r CROSS JOIN careeros_294.careeros_hosted_account_identity m
CROSS JOIN careeros_294.careeros_control_state c
WHERE r.rolname=current_user AND m.singleton AND c.singleton`, hostedPilotRuntime, 8, HostedPilotSchemaID(), requireClosed).Scan(&ready)
	if err != nil || !ready {
		return auth.ErrUnavailable
	}
	return nil
}

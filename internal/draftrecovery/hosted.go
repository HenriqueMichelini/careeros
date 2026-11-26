package draftrecovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"strings"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
)

//go:embed hosted.sql
var hostedGrants string

// Hosted roles resolve Account reads through projections. Foreign keys still
// bind the physical Account tables, so the hosted migration qualifies them.
func hostedDraftSchema() string {
	return strings.NewReplacer(
		"REFERENCES careeros_accounts(", "REFERENCES careeros_294.careeros_accounts(",
		"REFERENCES careeros_pilot_access(", "REFERENCES careeros_294.careeros_pilot_access(",
	).Replace(schema)
}

func HostedDraftSchemaID() string {
	hash := sha256.Sum256([]byte(hostedDraftSchema() + "\x00" + hostedGrants))
	return hex.EncodeToString(hash[:])
}

// MigrateHostedDraft applies the canonical Draft store tables and narrow grants
// inside the caller's authorized transaction. Account, fake scope and canonical
// Action tables must already exist. The caller owns commit and rollback.
func MigrateHostedDraft(ctx context.Context, tx *sql.Tx, artifactKeyDigest [32]byte) error {
	if tx == nil || artifactKeyDigest == ([32]byte{}) {
		return ErrDenied
	}
	if err := accountstore.LockControlStopTransaction(ctx, tx); err != nil {
		return ErrDenied
	}
	var authorized bool
	if err := tx.QueryRowContext(ctx, `SELECT current_database()='careeros_294'
AND current_user='careeros_294_migrator' AND current_user=session_user
AND EXISTS (SELECT 1 FROM pg_namespace WHERE nspname='careeros_294_draft' AND pg_get_userbyid(nspowner)=current_user)
AND EXISTS (SELECT 1 FROM careeros_294.careeros_control_state WHERE singleton AND invitation_stopped AND inference_stopped)`).Scan(&authorized); err != nil || !authorized {
		return ErrDenied
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL search_path=careeros_294_draft,careeros_294,pg_catalog,pg_temp`); err != nil {
		return ErrDenied
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(72481308)`); err != nil {
		return ErrDenied
	}
	var installed bool
	if err := tx.QueryRowContext(ctx, `SELECT to_regclass('careeros_294_draft.hosted_draft_store_identity') IS NOT NULL`).Scan(&installed); err != nil {
		return ErrDenied
	}
	if installed {
		var unchanged bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
SELECT 1 FROM careeros_294_draft.hosted_draft_store_identity WHERE singleton AND schema_id=$1 AND artifact_key_digest=$2
)`, HostedDraftSchemaID(), artifactKeyDigest[:]).Scan(&unchanged); err != nil || !unchanged {
			return ErrConflict
		}
		return nil
	}
	if _, err := tx.ExecContext(ctx, hostedDraftSchema()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, hostedGrants); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO careeros_294_draft.hosted_draft_store_identity(singleton,schema_id,artifact_key_digest)
VALUES(true,$1,$2)`, HostedDraftSchemaID(), artifactKeyDigest[:])
	return err
}

// OpenHostedDraft binds the existing protocol to an explicitly trusted hosted
// harness and verified writer pool. It takes ownership of the pool on success.
func OpenHostedDraft(ctx context.Context, writer *sql.DB, actions *aicontrol.Harness, bindingKey []byte) (*Store, error) {
	if writer == nil || actions == nil || !actions.HostedDraftEnabled() || len(bindingKey) < 32 || len(bytes.Trim(bindingKey, "\x00")) == 0 {
		return nil, ErrDenied
	}
	store := &Store{
		db: writer,
		cfg: Config{
			Actions:    actions,
			BindingKey: append([]byte(nil), bindingKey...),
			Decisions:  DecisionFunc(managedPilotDecision),
		},
		managed: true, opening: true, terminal: true, hosted: true,
	}
	if err := store.HostedDraftReady(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) HostedDraftEnabled() bool {
	return s != nil && s.hosted && s.TerminalEnabled() && s.cfg.Actions.HostedDraftEnabled()
}

// HostedDraftReady rejects schema, role or capability drift without DDL.
func (s *Store) HostedDraftReady(ctx context.Context) error {
	if !s.HostedDraftEnabled() {
		return ErrDenied
	}
	artifactKeyDigest := sha256.Sum256(s.cfg.BindingKey)
	// Compare the catalog definition with the exact body migrated from source.
	const scopeDelimiter = "$draft_store_scope$"
	_, definition, _ := strings.Cut(hostedGrants, scopeDelimiter)
	scopeFunctionBody, _, _ := strings.Cut(definition, scopeDelimiter)
	var ready bool
	if err := s.db.QueryRowContext(ctx, `SELECT
current_database()='careeros_294' AND current_user='careeros_294_draft_ledger' AND session_user=current_user
AND current_schemas(false)=ARRAY['careeros_294_draft','careeros_294','pg_catalog']::name[]
AND EXISTS (SELECT 1 FROM careeros_294_draft.hosted_draft_store_identity WHERE singleton AND schema_id=$1 AND artifact_key_digest=$2)
AND EXISTS (
 SELECT 1 FROM pg_proc fn WHERE fn.oid='careeros_294_draft.draft_store_scope(text,text,text)'::regprocedure
 AND fn.prosecdef AND fn.provolatile='s' AND fn.prosrc=$3 AND pg_get_userbyid(fn.proowner)='careeros_294_migrator'
 AND fn.proconfig=ARRAY['search_path=pg_catalog, pg_temp']
 AND has_function_privilege(current_user,fn.oid,'EXECUTE')
 AND NOT EXISTS (SELECT 1 FROM aclexplode(COALESCE(fn.proacl,acldefault('f',fn.proowner))) acl
  WHERE acl.privilege_type='EXECUTE' AND acl.grantee NOT IN (fn.proowner,(SELECT oid FROM pg_roles WHERE rolname=current_user)))
)
AND EXISTS (SELECT 1 FROM pg_roles r WHERE r.rolname=current_user AND r.rolcanlogin AND r.rolconnlimit BETWEEN 1 AND 4
 AND NOT (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolinherit OR r.rolreplication OR r.rolbypassrls)
 AND NOT EXISTS (SELECT 1 FROM pg_auth_members m WHERE m.member=r.oid OR m.roleid=r.oid))
AND NOT EXISTS (
 SELECT 1 FROM (VALUES
  ('careeros_draft_admissions',ARRAY['state'],'NULL::text, owner, creation_key'),
  ('careeros_draft_actions',ARRAY['state','artifact_version','generation_attempt','published_digest','cleanup_queued'],'action_id, owner, creation_key'),
  ('careeros_draft_content',ARRAY['bundle'],'action_id, NULL::text, NULL::text'),
  ('careeros_draft_reviews',ARRAY['revision','decisions','accepted_at'],'action_id, NULL::text, NULL::text'),
  ('careeros_draft_review_commands',ARRAY[]::text[],'action_id, NULL::text, NULL::text'),
  ('careeros_draft_allowance',ARRAY['state','confirmed_at','released_at'],'action_id, owner, NULL::text'),
  ('careeros_draft_opening_receipts',ARRAY[]::text[],'action_id, NULL::text, NULL::text')
 ) required(relation,writable,scope_arguments)
 LEFT JOIN pg_class c ON c.oid=to_regclass('careeros_294_draft.'||required.relation)
 WHERE c.oid IS NULL OR NOT c.relrowsecurity OR pg_get_userbyid(c.relowner)<>'careeros_294_migrator'
    OR NOT has_table_privilege(current_user,c.oid,'SELECT') OR NOT has_table_privilege(current_user,c.oid,'INSERT')
    OR has_table_privilege(current_user,c.oid,'UPDATE,DELETE,TRUNCATE,TRIGGER')
    OR EXISTS (SELECT 1 FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped
        AND has_column_privilege(current_user,c.oid,a.attnum,'UPDATE')<>(a.attname=ANY(required.writable)))
    OR (SELECT count(*) FROM pg_policy p WHERE p.polrelid=c.oid)<>1
    OR NOT EXISTS (SELECT 1 FROM pg_policy p WHERE p.polrelid=c.oid AND p.polname='hosted_draft_scope'
        AND p.polroles=ARRAY[(SELECT oid FROM pg_roles WHERE rolname=current_user)] AND p.polcmd='*'
        AND p.polpermissive AND p.polwithcheck IS NULL
        AND pg_get_expr(p.polqual,c.oid)=format('((namespace = %L::text) AND draft_store_scope(%s))', 'careeros_294_draft', required.scope_arguments))
)`, HostedDraftSchemaID(), artifactKeyDigest[:], scopeFunctionBody).Scan(&ready); err != nil || !ready {
		return ErrDenied
	}
	return nil
}

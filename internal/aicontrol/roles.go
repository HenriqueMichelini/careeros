package aicontrol

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/url"
	"regexp"
)

// syntheticRolePools installs privileges only in the explicitly disposable
// fixture schema. The application migration/runtime never calls this function.
// Runtime and reconciliation use independent role-bound connection pools.
func syntheticRolePools(ctx context.Context, admin *sql.DB, dsn string) (*sql.DB, *sql.DB, string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, nil, "", ErrDenied
	}
	schema := u.Query().Get("search_path")
	if !regexp.MustCompile(`^careeros_ai_test_[A-Za-z0-9_]+$`).MatchString(schema) {
		return nil, nil, "", ErrDenied
	}
	var database string
	if admin.QueryRowContext(ctx, "SELECT current_database()").Scan(&database) != nil {
		return nil, nil, "", ErrUnavailable
	}
	digest := sha256.Sum256([]byte(database + "/" + schema))
	suffix := fmt.Sprintf("%x", digest[:8])
	runtimeRole, reviewerRole, supportRole := "ai_fixture_rt_"+suffix, "ai_fixture_review_"+suffix, "ai_fixture_support_"+suffix
	tx, err := admin.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, "", ErrUnavailable
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(72481309)"); err != nil {
		return nil, nil, "", ErrUnavailable
	}
	for _, role := range []string{runtimeRole, reviewerRole, supportRole} {
		var exists bool
		if tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)", role).Scan(&exists) != nil {
			return nil, nil, "", ErrUnavailable
		}
		if !exists {
			if _, err = tx.ExecContext(ctx, "CREATE ROLE "+role+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD NULL"); err != nil {
				return nil, nil, "", ErrUnavailable
			}
		}
	}
	// All interpolated identifiers above/below are validated/generated tokens.
	// No request, account ID, source text or provider material enters DDL.
	ddl := fmt.Sprintf(`
GRANT USAGE ON SCHEMA %[1]s TO %[2]s, %[3]s;
GRANT SELECT ON ai_fixture_namespace,ai_fixture_policy,ai_fixture_grant,ai_fixture_scope,ai_fixture_action,ai_fixture_stage,ai_fixture_child,ai_fixture_dispatch,ai_fixture_allocation,ai_fixture_evidence,ai_fixture_observation TO %[2]s, %[3]s;
GRANT SELECT(id,status,generation) ON careeros_accounts TO %[2]s, %[3]s;
GRANT SELECT ON careeros_sessions TO %[2]s, %[3]s;
-- Only runtime's current Profile binding needs transient career content.
-- Reconciliation receives no Profile/provenance or Profile-operation view.
GRANT SELECT(account_id,document) ON careeros_profiles TO %[2]s;
GRANT INSERT(namespace,id,owner,creation_key,workflow,binding,profile_binding,grant_version,execution_binding,execution_version) ON ai_fixture_action TO %[2]s;
GRANT INSERT(namespace,scope,limit_amount,capacity) ON ai_fixture_scope TO %[2]s;
GRANT INSERT(namespace,id,action,coordinate,stage,leg,attempt,provider,binding,maximum,floor_amount,counted,input_bytes,output_tokens) ON ai_fixture_dispatch TO %[2]s;
GRANT INSERT ON ai_fixture_stage,ai_fixture_child,ai_fixture_allocation,ai_fixture_observation TO %[2]s;
GRANT UPDATE(state,customer_reserved,attempt) ON ai_fixture_action TO %[2]s;
GRANT UPDATE(state,category,remote_state,intent_at,durable_bound,local_finished) ON ai_fixture_dispatch TO %[2]s;
GRANT UPDATE(counted,occupied) ON ai_fixture_scope TO %[2]s;
GRANT UPDATE(expires_at) ON ai_fixture_policy TO %[2]s;
GRANT UPDATE(version) ON ai_fixture_grant TO %[2]s;
-- FOR UPDATE needs a write privilege. A fixture trigger denies actual auth
-- mutation by either ledger role; existing auth stores retain their own role.
GRANT UPDATE(created_at) ON careeros_accounts,careeros_sessions TO %[2]s, %[3]s;
GRANT INSERT,UPDATE ON ai_fixture_namespace,ai_fixture_policy,ai_fixture_grant,ai_fixture_scope,ai_fixture_action,ai_fixture_stage,ai_fixture_dispatch,ai_fixture_allocation,ai_fixture_evidence TO %[3]s;
GRANT DELETE ON ai_fixture_evidence,ai_fixture_stage,ai_fixture_child TO %[3]s;

CREATE OR REPLACE FUNCTION ai_fixture_runtime_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=%[1]s,pg_catalog AS $guard$
BEGIN
 IF current_user IN ('%[2]s','%[3]s') AND TG_TABLE_NAME IN ('careeros_accounts','careeros_sessions') THEN
   RAISE EXCEPTION 'ledger role cannot mutate authentication';
 END IF;
 IF current_user <> '%[2]s' THEN RETURN NEW; END IF;
 IF TG_TABLE_NAME='ai_fixture_policy' THEN RAISE EXCEPTION 'runtime policy is immutable'; END IF;
 IF TG_TABLE_NAME='ai_fixture_grant' THEN RAISE EXCEPTION 'runtime grant is immutable'; END IF;
 IF TG_TABLE_NAME='ai_fixture_scope' THEN
   IF NEW.counted<OLD.counted OR NEW.occupied<OLD.occupied OR NEW.limit_amount<>OLD.limit_amount OR NEW.capacity<>OLD.capacity THEN
     RAISE EXCEPTION 'runtime cannot release exposure or capacity';
   END IF;
   RETURN NEW;
 END IF;
 IF TG_TABLE_NAME='ai_fixture_dispatch' THEN
   IF NEW.state='aborted' OR NEW.remote_state='terminal_proven' OR
      NEW.category NOT IN ('reserved_maximum','unknown_maximum') OR
      (OLD.state<>'reserved' AND NEW.state='reserved') OR
      (OLD.intent_at IS NOT NULL AND NEW.intent_at IS DISTINCT FROM OLD.intent_at) OR
      (OLD.durable_bound IS NOT NULL AND NEW.durable_bound IS DISTINCT FROM OLD.durable_bound) THEN
     RAISE EXCEPTION 'runtime cannot reconcile or reopen dispatch';
   END IF;
 END IF;
 IF TG_TABLE_NAME='ai_fixture_action' THEN
   IF NEW.customer_effect IS DISTINCT FROM OLD.customer_effect OR
      (OLD.customer_reserved AND NOT NEW.customer_reserved) OR
      ((NEW.state IS DISTINCT FROM OLD.state OR NEW.attempt IS DISTINCT FROM OLD.attempt) AND
       NOT (OLD.state='failed' AND NEW.state='recoverable' AND NEW.attempt=OLD.attempt+1)) THEN
     RAISE EXCEPTION 'runtime cannot debit or revive terminal action';
   END IF;
 END IF;
 RETURN NEW;
END $guard$;
DROP TRIGGER IF EXISTS ai_fixture_runtime_guard ON ai_fixture_scope;
CREATE TRIGGER ai_fixture_runtime_guard BEFORE UPDATE ON ai_fixture_scope FOR EACH ROW EXECUTE FUNCTION ai_fixture_runtime_guard();
DROP TRIGGER IF EXISTS ai_fixture_runtime_guard ON ai_fixture_dispatch;
CREATE TRIGGER ai_fixture_runtime_guard BEFORE UPDATE ON ai_fixture_dispatch FOR EACH ROW EXECUTE FUNCTION ai_fixture_runtime_guard();
DROP TRIGGER IF EXISTS ai_fixture_runtime_guard ON ai_fixture_action;
CREATE TRIGGER ai_fixture_runtime_guard BEFORE UPDATE ON ai_fixture_action FOR EACH ROW EXECUTE FUNCTION ai_fixture_runtime_guard();
DROP TRIGGER IF EXISTS ai_fixture_runtime_guard ON ai_fixture_policy;
CREATE TRIGGER ai_fixture_runtime_guard BEFORE UPDATE ON ai_fixture_policy FOR EACH ROW EXECUTE FUNCTION ai_fixture_runtime_guard();
DROP TRIGGER IF EXISTS ai_fixture_runtime_guard ON ai_fixture_grant;
CREATE TRIGGER ai_fixture_runtime_guard BEFORE UPDATE ON ai_fixture_grant FOR EACH ROW EXECUTE FUNCTION ai_fixture_runtime_guard();
DROP TRIGGER IF EXISTS ai_fixture_runtime_guard ON careeros_accounts;
CREATE TRIGGER ai_fixture_runtime_guard BEFORE UPDATE ON careeros_accounts FOR EACH ROW EXECUTE FUNCTION ai_fixture_runtime_guard();
DROP TRIGGER IF EXISTS ai_fixture_runtime_guard ON careeros_sessions;
CREATE TRIGGER ai_fixture_runtime_guard BEFORE UPDATE ON careeros_sessions FOR EACH ROW EXECUTE FUNCTION ai_fixture_runtime_guard();

-- The runtime's only downward release command. Durable intent makes this
-- command permanently ineligible; no lease, timeout or category override.
CREATE OR REPLACE FUNCTION ai_fixture_abandon(ns text, dispatch_id text) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=%[1]s,pg_catalog AS $abandon$
DECLARE d ai_fixture_dispatch%%ROWTYPE; action_id text; allocation record;
BEGIN
 SELECT action INTO action_id FROM ai_fixture_dispatch WHERE namespace=ns AND id=dispatch_id;
 IF action_id IS NULL THEN RAISE EXCEPTION 'dispatch unavailable'; END IF;
 PERFORM 1 FROM ai_fixture_action WHERE namespace=ns AND id=action_id FOR UPDATE;
 SELECT * INTO d FROM ai_fixture_dispatch WHERE namespace=ns AND id=dispatch_id FOR UPDATE;
 IF d.state='aborted' THEN RETURN; END IF;
 IF d.state<>'reserved' OR d.intent_at IS NOT NULL THEN RAISE EXCEPTION 'dispatch not provably unsent'; END IF;
 FOR allocation IN SELECT s.scope FROM ai_fixture_scope s JOIN ai_fixture_allocation a ON a.namespace=s.namespace AND a.scope=s.scope
    WHERE a.namespace=ns AND a.dispatch=dispatch_id ORDER BY s.scope FOR UPDATE OF s LOOP
   UPDATE ai_fixture_scope SET counted=counted-d.counted,occupied=occupied-1 WHERE namespace=ns AND scope=allocation.scope;
 END LOOP;
 UPDATE ai_fixture_dispatch SET state='aborted',category='aborted_unsent',floor_amount=0,counted=0,local_finished=true
    WHERE namespace=ns AND id=dispatch_id;
END $abandon$;
REVOKE ALL ON FUNCTION ai_fixture_abandon(text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ai_fixture_abandon(text,text) TO %[2]s;
`, schema, runtimeRole, reviewerRole)
	if _, err = tx.ExecContext(ctx, ddl); err != nil {
		return nil, nil, "", ErrUnavailable
	}
	if tx.Commit() != nil {
		return nil, nil, "", ErrUnavailable
	}
	open := func(role string) (*sql.DB, error) {
		clone := *u
		q := clone.Query()
		// Authenticate as this restricted fixture role. SET ROLE on an admin
		// connection would be escapable with RESET ROLE and is insufficient.
		clone.User = url.User(role)
		for _, key := range []string{"role", "user", "password", "options"} {
			q.Del(key)
		}
		clone.RawQuery = q.Encode()
		db, e := sql.Open("pgx", clone.String())
		if e != nil {
			return nil, ErrUnavailable
		}
		db.SetMaxOpenConns(8)
		var current string
		if db.QueryRowContext(ctx, "SELECT current_user").Scan(&current) != nil || current != role {
			db.Close()
			return nil, ErrUnavailable
		}
		return db, nil
	}
	runtimeDB, err := open(runtimeRole)
	if err != nil {
		return nil, nil, "", err
	}
	reviewerDB, err := open(reviewerRole)
	if err != nil {
		runtimeDB.Close()
		return nil, nil, "", err
	}
	return runtimeDB, reviewerDB, supportRole, nil
}

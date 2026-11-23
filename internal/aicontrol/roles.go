package aicontrol

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const ingestionResumeRule = `(OLD.state='failed' AND NEW.state='recoverable' AND NEW.attempt=OLD.attempt+1 AND NOT NEW.execution_finished AND (OLD.retry_creation_key='' OR OLD.retry_creation_key=NEW.retry_creation_key)) OR
           (OLD.workflow='profile_ingestion' AND OLD.state='recoverable' AND NEW.state=OLD.state AND NEW.attempt=OLD.attempt AND OLD.execution_finished AND NOT NEW.execution_finished AND NEW.retry_creation_key=OLD.retry_creation_key)`

// syntheticRolePools installs privileges only in the explicitly disposable
// fixture schema. The application migration/runtime never calls this function.
// Runtime and reconciliation use independent role-bound connection pools.
func syntheticRolePools(ctx context.Context, admin *sql.DB, dsn string, ingestion bool) (*sql.DB, *sql.DB, string, error) {
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
	var installed bool
	if tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_fixture_installation WHERE version=1)`).Scan(&installed) != nil {
		return nil, nil, "", ErrUnavailable
	}
	if ingestion {
		var present bool
		if tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid='ai_fixture_dispatch'::regclass AND attname='input_tokens' AND NOT attisdropped)`).Scan(&present) != nil {
			return nil, nil, "", ErrUnavailable
		}
		if !present {
			if _, err = tx.ExecContext(ctx, `ALTER TABLE ai_fixture_dispatch ADD COLUMN input_tokens bigint NOT NULL DEFAULT 0 CHECK(input_tokens>=0)`); err != nil {
				return nil, nil, "", ErrUnavailable
			}
		}
	}
	// Old Actions stay conservatively untracked; only new rows get the
	// database-owned admission default. Runtime has no INSERT/UPDATE grant on
	// this column. Returned local leases also require the code-owned reviewer
	// capability, not a raw stage INSERT using runtime's ordinary stage grant.
	var localInstalled bool
	if tx.QueryRowContext(ctx, `SELECT to_regprocedure('ai_fixture_local_execution_guard()') IS NOT NULL`).Scan(&localInstalled) != nil {
		return nil, nil, "", ErrUnavailable
	}
	if !localInstalled {
		if _, err = tx.ExecContext(ctx, fmt.Sprintf(`
ALTER TABLE ai_fixture_action ADD COLUMN IF NOT EXISTS local_execution_tracking boolean NOT NULL DEFAULT false;
ALTER TABLE ai_fixture_action ALTER COLUMN local_execution_tracking SET DEFAULT true;
CREATE FUNCTION ai_fixture_local_execution_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $guard$
BEGIN
 IF current_user='%[1]s' AND NEW.stage='local_execution' AND NEW.receipt_id<>'' THEN
   RAISE EXCEPTION 'runtime cannot attest local callback return';
 END IF;
 RETURN NEW;
END $guard$;
REVOKE ALL ON FUNCTION ai_fixture_local_execution_guard() FROM PUBLIC;
CREATE TRIGGER ai_fixture_local_execution_guard BEFORE INSERT ON ai_fixture_stage
 FOR EACH ROW EXECUTE FUNCTION ai_fixture_local_execution_guard();
`, runtimeRole)); err != nil {
			return nil, nil, "", ErrUnavailable
		}
	}
	var controlEpochInstalled bool
	if tx.QueryRowContext(ctx, `SELECT to_regprocedure('ai_fixture_control_epoch_guard()') IS NOT NULL`).Scan(&controlEpochInstalled) != nil {
		return nil, nil, "", ErrUnavailable
	}
	if !controlEpochInstalled {
		if _, err = tx.ExecContext(ctx, fmt.Sprintf(`
ALTER TABLE ai_fixture_action ADD COLUMN IF NOT EXISTS control_epoch bigint NOT NULL DEFAULT 1 CHECK(control_epoch>0);
ALTER TABLE ai_fixture_action ALTER COLUMN control_epoch SET DEFAULT careeros_current_inference_epoch();
CREATE FUNCTION ai_fixture_control_epoch_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=%[1]s,pg_catalog,pg_temp AS $guard$
BEGIN
 IF current_user IN ('%[2]s','%[3]s','%[4]s') THEN
  IF TG_OP='INSERT' THEN
   NEW.control_epoch:=careeros_current_inference_epoch();
  ELSIF current_user='%[2]s' AND OLD.state='failed' AND NEW.state='recoverable' AND NEW.attempt=OLD.attempt+1 AND NOT OLD.compacted THEN
   IF NOT careeros_inference_allowed() THEN RAISE EXCEPTION 'inference stopped'; END IF;
   IF OLD.execution_version<>'synthetic-execution-v1' AND EXISTS(SELECT 1 FROM ai_fixture_dispatch d WHERE d.namespace=OLD.namespace AND d.action=OLD.id AND (d.state<>'observed' OR d.remote_state<>'terminal_proven')) THEN RAISE EXCEPTION 'unknown intent cannot create fresh attempt'; END IF;
   NEW.control_epoch:=careeros_current_inference_epoch();
  ELSIF NEW.control_epoch IS DISTINCT FROM OLD.control_epoch THEN
   RAISE EXCEPTION 'restricted role cannot forge inference epoch';
  END IF;
 END IF;
 RETURN NEW;
END $guard$;
REVOKE ALL ON FUNCTION ai_fixture_control_epoch_guard() FROM PUBLIC;
CREATE TRIGGER ai_fixture_control_epoch_guard BEFORE INSERT OR UPDATE ON ai_fixture_action FOR EACH ROW EXECUTE FUNCTION ai_fixture_control_epoch_guard();
`, schema, runtimeRole, reviewerRole, supportRole)); err != nil {
			return nil, nil, "", ErrUnavailable
		}
	}
	// The schema-local marker and all privileges commit atomically. Reopening
	// a fixture authenticates existing restricted roles without reacquiring DDL
	// locks against admissions already using those tables. Missing/stale fixture
	// installation fails closed; this is not a production migration mechanism.
	if !installed {
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
GRANT SELECT ON careeros_pilot_access,careeros_identities TO %[2]s;
-- Only runtime's current Profile binding needs transient career content.
-- Reconciliation receives no Profile/provenance or Profile-operation view.
GRANT SELECT(account_id,document) ON careeros_profiles TO %[2]s;
GRANT INSERT(namespace,id,owner,creation_key,workflow,binding,profile_binding,grant_version,execution_binding,execution_version) ON ai_fixture_action TO %[2]s;
GRANT INSERT(namespace,scope,limit_amount,capacity) ON ai_fixture_scope TO %[2]s;
GRANT INSERT(namespace,id,action,coordinate,stage,leg,attempt,provider,binding,maximum,floor_amount,counted,input_bytes,output_tokens,input_tokens) ON ai_fixture_dispatch TO %[2]s;
GRANT INSERT ON ai_fixture_stage,ai_fixture_child,ai_fixture_allocation,ai_fixture_observation TO %[2]s;
GRANT UPDATE(state,customer_reserved,attempt,execution_finished,retry_creation_key) ON ai_fixture_action TO %[2]s;
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
   IF (NEW.execution_finished IS DISTINCT FROM OLD.execution_finished OR NEW.retry_creation_key IS DISTINCT FROM OLD.retry_creation_key) AND
      NOT (%[5]s) THEN
     RAISE EXCEPTION 'runtime cannot complete or rebind action';
   END IF;
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
`, schema, runtimeRole, reviewerRole, supportRole, ingestionResumeRule)
		if _, err = tx.ExecContext(ctx, ddl); err != nil {
			return nil, nil, "", ErrUnavailable
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_installation(version) VALUES(1)`); err != nil {
			return nil, nil, "", ErrUnavailable
		}
	}
	if ingestion {
		// Upgrade only the known inherited fixture guard. This permits runtime
		// to CLAIM (never release) an idle ingestion Action slot. Existing
		// qualification identities, money and unknown occupancy are untouched.
		if installed {
			var definition string
			if tx.QueryRowContext(ctx, `SELECT pg_get_functiondef('ai_fixture_runtime_guard()'::regprocedure)`).Scan(&definition) != nil {
				return nil, nil, "", ErrUnavailable
			}
			if !strings.Contains(definition, ingestionResumeRule) {
				old := "NOT (OLD.state='failed' AND NEW.state='recoverable' AND NEW.attempt=OLD.attempt+1 AND NOT NEW.execution_finished AND (OLD.retry_creation_key='' OR OLD.retry_creation_key=NEW.retry_creation_key))"
				if !strings.Contains(definition, old) {
					return nil, nil, "", ErrUnavailable
				}
				definition = strings.Replace(definition, old, "NOT ("+ingestionResumeRule+")", 1)
				if _, err = tx.ExecContext(ctx, definition); err != nil {
					return nil, nil, "", ErrUnavailable
				}
			}
		}
		var allowed bool
		if tx.QueryRowContext(ctx, `SELECT has_column_privilege($1,'ai_fixture_dispatch','input_tokens','INSERT')`, runtimeRole).Scan(&allowed) != nil {
			return nil, nil, "", ErrUnavailable
		}
		if !allowed {
			if _, err = tx.ExecContext(ctx, "GRANT INSERT(input_tokens) ON ai_fixture_dispatch TO "+runtimeRole); err != nil {
				return nil, nil, "", ErrUnavailable
			}
		}
	}
	// The separate reconciliation authority is installed only by this disposable
	// migrator. Neither runtime nor elevated commands can install/elevate actors.
	var reconGranted bool
	if tx.QueryRowContext(ctx, `SELECT has_table_privilege($1,'ai_recon_operator','SELECT')`, reviewerRole).Scan(&reconGranted) != nil {
		return nil, nil, "", ErrUnavailable
	}
	if !reconGranted {
		ddl := fmt.Sprintf(`
REVOKE ALL ON ai_recon_operator,ai_recon_command,ai_recon_state,ai_recon_stop,ai_recon_event,ai_recon_key,ai_recon_contract,ai_recon_action_key,ai_recon_terminal_proof,ai_recon_bounds_review,ai_recon_legacy_marker,ai_recon_rate_revision FROM PUBLIC, %[1]s, %[2]s, %[3]s;
GRANT SELECT ON ai_recon_stop,ai_recon_contract,ai_recon_key,ai_recon_action_key TO %[1]s;
GRANT INSERT ON ai_recon_action_key TO %[1]s;
GRANT INSERT ON ai_recon_contract TO %[1]s;
GRANT UPDATE(epoch) ON ai_fixture_namespace TO %[1]s;
CREATE FUNCTION ai_recon_namespace_guard() RETURNS trigger LANGUAGE plpgsql AS $ns$
BEGIN
 IF current_user<>pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid=TG_RELID)) AND current_user<>'%[2]s' THEN RAISE EXCEPTION 'runtime cannot mutate namespace'; END IF;
 RETURN NEW;
END $ns$;
CREATE TRIGGER ai_recon_namespace_guard BEFORE UPDATE ON ai_fixture_namespace FOR EACH ROW EXECUTE FUNCTION ai_recon_namespace_guard();
GRANT SELECT ON ai_recon_operator,ai_recon_command,ai_recon_state,ai_recon_stop,ai_recon_event,ai_recon_key,ai_recon_contract,ai_recon_action_key,ai_recon_terminal_proof,ai_recon_bounds_review,ai_recon_legacy_marker,ai_recon_rate_revision TO %[2]s;
GRANT INSERT ON ai_recon_legacy_marker,ai_recon_rate_revision TO %[2]s;
GRANT UPDATE(version) ON ai_recon_operator TO %[2]s;
GRANT UPDATE(data) ON ai_fixture_observation TO %[2]s;
GRANT INSERT,UPDATE ON ai_recon_command,ai_recon_state,ai_recon_stop,ai_recon_event,ai_recon_key TO %[2]s;`, runtimeRole, reviewerRole, supportRole)
		if _, err = tx.ExecContext(ctx, ddl); err != nil {
			return nil, nil, "", ErrUnavailable
		}
	}
	// Additive migration for inherited fixture identities too. These predicates
	// return only protective booleans; never grant deletion-table/status secrets.
	if _, err = tx.ExecContext(ctx, fmt.Sprintf(`GRANT EXECUTE ON FUNCTION careeros_account_closed(text),careeros_deleted_profile_restored(text),careeros_action_predates_deletion(text,timestamptz),careeros_invitation_denied(text),careeros_inference_allowed(),careeros_current_inference_epoch(),careeros_control_account_denied(text),careeros_operations_required() TO %s,%s`, runtimeRole, reviewerRole)); err != nil {
		return nil, nil, "", ErrUnavailable
	}
	if _, err = tx.ExecContext(ctx, fmt.Sprintf(operationsCaptureSQL, fmt.Sprintf("%q", schema))); err != nil {
		return nil, nil, "", ErrUnavailable
	}
	if _, err = tx.ExecContext(ctx, fmt.Sprintf(`REVOKE INSERT(namespace,sample),INSERT ON careeros_operations_pending FROM %[1]s,%[2]s;
	  REVOKE USAGE ON SEQUENCE careeros_operations_pending_id_seq FROM %[1]s,%[2]s;
	  GRANT SELECT,DELETE ON careeros_operations_pending TO %[2]s;
	  REVOKE EXECUTE ON FUNCTION careeros_capture_operations(text,text) FROM %[2]s;
	  GRANT EXECUTE ON FUNCTION careeros_capture_operations(text,text) TO %[1]s;
	  GRANT EXECUTE ON FUNCTION careeros_capture_reconciliation_operations(text,text,boolean) TO %[2]s;
	  GRANT EXECUTE ON FUNCTION careeros_operations_snapshot(text,text) TO %[2]s;
	  GRANT EXECUTE ON FUNCTION careeros_operations_attach_matches(text),careeros_operations_observer_matches(text),
	    careeros_operations_add_hold(text,text,bigint,boolean),careeros_operations_emergency(text,text) TO %[2]s`, runtimeRole, reviewerRole)); err != nil {
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

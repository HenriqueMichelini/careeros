-- Run only after the explicit account migration, as the disposable migrator.
-- Supply schema_id from the same package's pilot-runtime-local -schema-id.
BEGIN;
DO $fixture$
BEGIN
 IF current_database() <> 'careeros_pilot_runtime_fixture'
   OR current_schema() <> 'careeros_pilot_runtime'
   OR current_user <> 'careeros_pilot_migrator' OR session_user <> current_user THEN
  RAISE EXCEPTION 'dedicated pilot fixture migrator required';
 END IF;
 IF NOT EXISTS (SELECT 1 FROM careeros_control_state
                 WHERE singleton AND invitation_stopped AND inference_stopped) THEN
  RAISE EXCEPTION 'closed invitation and inference controls required';
 END IF;
END $fixture$;

-- Exact current closed surface. Grant no ability to install roles, publish
-- grants/invitations, mutate Profile data, or read/write the managed ledger.
GRANT SELECT ON careeros_accounts, careeros_identities, careeros_sessions, careeros_deletion_status_sessions,
 careeros_profiles, careeros_profile_operations, careeros_deletion_operations, careeros_deleted_profiles,
 careeros_pilot_access, careeros_invitation_cleanup, careeros_control_state,
 careeros_lifecycle_notices, careeros_lifecycle_cleanup, careeros_lifecycle_purpose_cleanup,
 careeros_lifecycle_residuals, careeros_profile_pruning TO careeros_pilot_runtime;
-- Store account/session row locks need an UPDATE privilege. These grants
-- also cover existing logout/logout-all generation and revocation updates.
GRANT UPDATE (generation) ON careeros_accounts TO careeros_pilot_runtime;
GRANT UPDATE (created_at), DELETE ON careeros_sessions, careeros_deletion_status_sessions TO careeros_pilot_runtime;
GRANT EXECUTE ON FUNCTION careeros_account_closed(text), careeros_deleted_profile_restored(text),
 careeros_control_account_denied(text) TO careeros_pilot_runtime;

CREATE OR REPLACE FUNCTION careeros_local_package_lock_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $lock_guard$
BEGIN
 IF current_user <> 'careeros_pilot_runtime' THEN RETURN NEW; END IF;
 IF TG_TABLE_NAME = 'careeros_accounts' THEN
  IF NEW.generation = OLD.generation+1 THEN RETURN NEW; END IF;
 END IF;
 RAISE EXCEPTION 'closed runtime cannot update this authority';
END $lock_guard$;
REVOKE ALL ON FUNCTION careeros_local_package_lock_guard() FROM PUBLIC;
DROP TRIGGER IF EXISTS careeros_local_package_lock_guard ON careeros_accounts;
CREATE TRIGGER careeros_local_package_lock_guard BEFORE UPDATE ON careeros_accounts
 FOR EACH ROW EXECUTE FUNCTION careeros_local_package_lock_guard();
DROP TRIGGER IF EXISTS careeros_local_package_lock_guard ON careeros_sessions;
CREATE TRIGGER careeros_local_package_lock_guard BEFORE UPDATE ON careeros_sessions
 FOR EACH ROW EXECUTE FUNCTION careeros_local_package_lock_guard();
DROP TRIGGER IF EXISTS careeros_local_package_lock_guard ON careeros_deletion_status_sessions;
CREATE TRIGGER careeros_local_package_lock_guard BEFORE UPDATE ON careeros_deletion_status_sessions
 FOR EACH ROW EXECUTE FUNCTION careeros_local_package_lock_guard();

CREATE TABLE IF NOT EXISTS careeros_local_package_identity (
 singleton boolean PRIMARY KEY CHECK(singleton),
 schema_id text NOT NULL CHECK (schema_id ~ '^[0-9a-f]{64}$')
);
REVOKE ALL ON careeros_local_package_identity FROM PUBLIC;
INSERT INTO careeros_local_package_identity(singleton,schema_id) VALUES (true, :'schema_id')
 ON CONFLICT (singleton) DO UPDATE SET schema_id=EXCLUDED.schema_id;
GRANT SELECT ON careeros_local_package_identity TO careeros_pilot_runtime;
COMMIT;

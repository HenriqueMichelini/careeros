-- Executed by the dedicated hosted migrator in the migration transaction.
GRANT SELECT ON careeros_accounts,careeros_identities,careeros_sessions,careeros_deletion_status_sessions,
 careeros_oidc_transactions,careeros_profiles,careeros_profile_operations,careeros_deletion_operations,
 careeros_deleted_profiles,careeros_pilot_access,careeros_invitation_cleanup,careeros_lifecycle_notices,
 careeros_lifecycle_cleanup,careeros_lifecycle_purpose_cleanup,careeros_lifecycle_residuals,careeros_profile_pruning
 TO careeros_294_runtime;
GRANT SELECT (singleton,version,initialized,invitation_stopped,inference_stopped,inference_epoch,system_generation,
 operations_binding,operations_fault) ON careeros_control_state TO careeros_294_runtime;
GRANT INSERT ON careeros_accounts,careeros_identities,careeros_sessions,careeros_deletion_status_sessions,
 careeros_oidc_transactions,careeros_profiles,careeros_profile_operations TO careeros_294_runtime;
GRANT DELETE ON careeros_sessions,careeros_deletion_status_sessions,careeros_oidc_transactions TO careeros_294_runtime;

-- Account/session row locks used by the repositories require UPDATE on at
-- least one column. The guarded columns cannot be changed to renew authority.
GRANT UPDATE (generation) ON careeros_accounts TO careeros_294_runtime;
GRANT UPDATE (created_at) ON careeros_sessions,careeros_deletion_status_sessions TO careeros_294_runtime;
GRANT UPDATE (profile_id,revision,document) ON careeros_profiles TO careeros_294_runtime;
GRANT EXECUTE ON FUNCTION careeros_account_closed(text),careeros_deleted_profile_restored(text),
 careeros_control_account_denied(text) TO careeros_294_runtime;

CREATE OR REPLACE FUNCTION careeros_hosted_account_lock_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $lock_guard$
BEGIN
 IF current_user <> 'careeros_294_runtime' THEN RETURN NEW; END IF;
 IF TG_TABLE_NAME = 'careeros_accounts' THEN
  IF NEW.generation = OLD.generation+1 THEN RETURN NEW; END IF;
 END IF;
 RAISE EXCEPTION 'hosted runtime cannot update this authority';
END $lock_guard$;
REVOKE ALL ON FUNCTION careeros_hosted_account_lock_guard() FROM PUBLIC;
DROP TRIGGER IF EXISTS careeros_hosted_account_lock_guard ON careeros_accounts;
CREATE TRIGGER careeros_hosted_account_lock_guard BEFORE UPDATE ON careeros_accounts
 FOR EACH ROW EXECUTE FUNCTION careeros_hosted_account_lock_guard();
DROP TRIGGER IF EXISTS careeros_hosted_account_lock_guard ON careeros_sessions;
CREATE TRIGGER careeros_hosted_account_lock_guard BEFORE UPDATE ON careeros_sessions
 FOR EACH ROW EXECUTE FUNCTION careeros_hosted_account_lock_guard();
DROP TRIGGER IF EXISTS careeros_hosted_account_lock_guard ON careeros_deletion_status_sessions;
CREATE TRIGGER careeros_hosted_account_lock_guard BEFORE UPDATE ON careeros_deletion_status_sessions
 FOR EACH ROW EXECUTE FUNCTION careeros_hosted_account_lock_guard();

CREATE TABLE IF NOT EXISTS careeros_hosted_account_identity (
 singleton boolean PRIMARY KEY CHECK (singleton),
 schema_id text NOT NULL CHECK (schema_id ~ '^[0-9a-f]{64}$')
);
REVOKE ALL ON careeros_hosted_account_identity FROM PUBLIC;
GRANT SELECT ON careeros_hosted_account_identity TO careeros_294_runtime;

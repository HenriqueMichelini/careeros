-- Executed by the dedicated hosted migrator in the migration transaction.
GRANT SELECT ON careeros_accounts,careeros_identities,careeros_sessions,careeros_deletion_status_sessions,
 careeros_oidc_transactions,careeros_profiles,careeros_deletion_operations,
 careeros_deleted_profiles,careeros_pilot_access,careeros_invitation_cleanup,careeros_lifecycle_notices,
 careeros_lifecycle_cleanup,careeros_lifecycle_purpose_cleanup,careeros_lifecycle_residuals,careeros_profile_pruning
 TO careeros_294_runtime;
GRANT SELECT (singleton,version,initialized,invitation_stopped,inference_stopped,inference_epoch,system_generation,
 operations_binding,operations_fault) ON careeros_control_state TO careeros_294_runtime;
GRANT INSERT ON careeros_accounts,careeros_identities,careeros_sessions,careeros_deletion_status_sessions,
 careeros_oidc_transactions,careeros_profiles TO careeros_294_runtime;
GRANT DELETE ON careeros_sessions,careeros_deletion_status_sessions,careeros_oidc_transactions TO careeros_294_runtime;

-- Set the default after adding the column so existing operation receipts
-- remain readable and gain no claim to a hosted Save transaction.
ALTER TABLE careeros_profile_operations ADD COLUMN IF NOT EXISTS hosted_save_transaction xid8;
ALTER TABLE careeros_profile_operations ALTER COLUMN hosted_save_transaction SET DEFAULT pg_current_xact_id();
REVOKE SELECT, INSERT ON careeros_profile_operations FROM careeros_294_runtime;
GRANT SELECT (account_id,operation_id,request_hash,profile_id,committed_revision),
 INSERT (account_id,operation_id,request_hash,profile_id,committed_revision)
 ON careeros_profile_operations TO careeros_294_runtime;

-- Account/session row locks used by the repositories require UPDATE on at
-- least one column. The guarded columns cannot be changed to renew authority.
GRANT UPDATE (generation) ON careeros_accounts TO careeros_294_runtime;
GRANT UPDATE (created_at) ON careeros_sessions,careeros_deletion_status_sessions TO careeros_294_runtime;
GRANT UPDATE (profile_id,revision,document) ON careeros_profiles TO careeros_294_runtime;
GRANT EXECUTE ON FUNCTION careeros_account_closed(text),careeros_deleted_profile_restored(text),
 careeros_control_account_denied(text) TO careeros_294_runtime;

-- PostgreSQL JSON extraction converts escapes to text and cannot represent
-- U+0000. Substitute only actual JSON NUL escapes in a temporary projection;
-- preserve escaped backslashes and leave the stored Profile bytes untouched.
-- Comparing two substitutions rejects NUL in an evidence ID, whose fence and
-- existing Save query already use PostgreSQL text.
CREATE OR REPLACE FUNCTION careeros_294.careeros_hosted_profile_evidence_ids(serialized bytea) RETURNS text[]
LANGUAGE plpgsql IMMUTABLE SET search_path=pg_catalog,pg_temp AS $evidence_ids$
DECLARE
 zero_escape constant text := $pattern$(?<!\\)((?:\\\\)*)\\u0000$pattern$;
 raw text := convert_from(serialized,'UTF8');
 projected text := regexp_replace(raw COLLATE "C",zero_escape,$replacement$\1A$replacement$,'g');
 evidence json := projected::json->'evidence';
 ids text[];
 alternate_ids text[];
BEGIN
 IF json_typeof(evidence) IS DISTINCT FROM 'array' THEN
  RAISE EXCEPTION 'invalid hosted Profile evidence array';
 END IF;
 SELECT ARRAY(SELECT item->>'id' FROM json_array_elements(evidence) WITH ORDINALITY e(item,position) ORDER BY position) INTO ids;
 IF projected<>raw THEN
  evidence := regexp_replace(raw COLLATE "C",zero_escape,$replacement$\1B$replacement$,'g')::json->'evidence';
  SELECT ARRAY(SELECT item->>'id' FROM json_array_elements(evidence) WITH ORDINALITY e(item,position) ORDER BY position) INTO alternate_ids;
  IF ids IS DISTINCT FROM alternate_ids THEN
   RAISE EXCEPTION 'invalid hosted Profile evidence identifier';
  END IF;
 END IF;
 IF array_position(ids,NULL) IS NOT NULL OR array_position(ids,'') IS NOT NULL THEN
  RAISE EXCEPTION 'invalid hosted Profile evidence identifier';
 END IF;
 RETURN ids;
END $evidence_ids$;
REVOKE ALL ON FUNCTION careeros_294.careeros_hosted_profile_evidence_ids(bytea) FROM PUBLIC;

-- A private deferred trigger binds the retired IDs to the actual row change.
-- A matching receipt and its pruning fences commit or roll back together.
CREATE OR REPLACE FUNCTION careeros_294.careeros_hosted_profile_pruning() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $profile_pruning$
DECLARE
 current_ids text[];
 removed_ids text[];
BEGIN
 IF session_user <> 'careeros_294_runtime' THEN RETURN NEW; END IF;
 IF TG_OP='INSERT' THEN
  IF EXISTS (
   SELECT 1 FROM careeros_294.careeros_profile_pruning f
   WHERE f.account_id=NEW.account_id AND f.profile_id=NEW.profile_id
  ) THEN
   RAISE EXCEPTION 'hosted Profile cannot be reinserted behind a pruning fence';
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.account_id<>OLD.account_id OR NEW.profile_id<>OLD.profile_id OR NEW.revision<>OLD.revision+1
  OR NEW.revision>9007199254740991 THEN
  RAISE EXCEPTION 'invalid hosted Profile revision transition';
 END IF;
 current_ids := careeros_294.careeros_hosted_profile_evidence_ids(NEW.document);
 IF EXISTS (
  SELECT 1 FROM careeros_294.careeros_profile_pruning f
  WHERE f.account_id=NEW.account_id AND f.profile_id=NEW.profile_id AND f.evidence_id=ANY(current_ids)
 ) THEN
  RAISE EXCEPTION 'hosted Profile contains a pruned evidence identifier';
 END IF;
 SELECT ARRAY(
  SELECT unnest(careeros_294.careeros_hosted_profile_evidence_ids(OLD.document))
  EXCEPT
  SELECT unnest(current_ids)
 ) INTO removed_ids;
 IF cardinality(removed_ids)=0 THEN RETURN NEW; END IF;
 IF NOT EXISTS (
  SELECT 1 FROM careeros_294.careeros_profiles p
  JOIN careeros_294.careeros_profile_operations o ON o.account_id=p.account_id
   AND o.profile_id=p.profile_id AND o.committed_revision=p.revision
  WHERE p.account_id=NEW.account_id AND p.profile_id=NEW.profile_id AND p.revision=NEW.revision
   AND p.document=NEW.document AND o.hosted_save_transaction=pg_current_xact_id()
 ) THEN
  RAISE EXCEPTION 'hosted Profile pruning requires the final Save and receipt in the transaction';
 END IF;
 WITH stamp AS MATERIALIZED (SELECT clock_timestamp() AS removed_at)
 INSERT INTO careeros_294.careeros_profile_pruning
  (account_id,profile_id,evidence_id,previous_revision,new_revision,removed_at,recovery_copy_deadline)
 SELECT NEW.account_id,NEW.profile_id,removed.id,OLD.revision,NEW.revision,
  stamp.removed_at,stamp.removed_at+interval '720 hours'
 FROM unnest(removed_ids) AS removed(id) CROSS JOIN stamp
 ON CONFLICT DO NOTHING;
 RETURN NEW;
END $profile_pruning$;
REVOKE ALL ON FUNCTION careeros_294.careeros_hosted_profile_pruning() FROM PUBLIC;
DROP TRIGGER IF EXISTS careeros_hosted_profile_pruning ON careeros_profiles;
CREATE CONSTRAINT TRIGGER careeros_hosted_profile_pruning AFTER INSERT OR UPDATE ON careeros_profiles
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION careeros_294.careeros_hosted_profile_pruning();

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

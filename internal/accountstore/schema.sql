-- Version 1. Application-owned tables; no browser/database access path.
CREATE TABLE IF NOT EXISTS careeros_accounts (
  id text PRIMARY KEY,
  status text NOT NULL CHECK (status IN ('active','suspended','deleted')),
  generation numeric(20,0) NOT NULL CHECK (generation BETWEEN 1 AND 18446744073709551615),
  created_at timestamptz NOT NULL,
  email bytea NOT NULL,
  display_name bytea NOT NULL
);
CREATE TABLE IF NOT EXISTS careeros_identities (
  provider bytea NOT NULL, issuer bytea NOT NULL, subject bytea NOT NULL,
  account_id text NOT NULL REFERENCES careeros_accounts(id),
  created_at timestamptz NOT NULL,
  PRIMARY KEY (provider,issuer,subject)
);
CREATE TABLE IF NOT EXISTS careeros_sessions (
  id text PRIMARY KEY, account_id text NOT NULL REFERENCES careeros_accounts(id),
  token_hash bytea UNIQUE NOT NULL CHECK (octet_length(token_hash)=32),
  csrf_hash bytea NOT NULL CHECK (octet_length(csrf_hash)=32),
  generation numeric(20,0) NOT NULL,
  created_at timestamptz NOT NULL, expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS careeros_sessions_owner ON careeros_sessions(account_id);
CREATE TABLE IF NOT EXISTS careeros_oidc_transactions (
  state_hash bytea PRIMARY KEY CHECK (octet_length(state_hash)=32),
  binding_hash bytea NOT NULL CHECK (octet_length(binding_hash)=32),
  provider text NOT NULL, nonce text NOT NULL, verifier text NOT NULL,
  redirect_uri text NOT NULL, return_to text NOT NULL,
  account_id text NOT NULL, generation numeric(20,0) NOT NULL,
  previous_session_id text NOT NULL, expires_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS careeros_profiles (
  account_id text PRIMARY KEY REFERENCES careeros_accounts(id),
  profile_id bytea NOT NULL,
  revision bigint NOT NULL CHECK (revision BETWEEN 1 AND 9007199254740991),
  document bytea NOT NULL CHECK (octet_length(document) <= 4194304)
);
-- Receipts contain no historical career content. An account-scoped key binds
-- exact request bytes/base to its historical revision, independently of current.
CREATE TABLE IF NOT EXISTS careeros_profile_operations (
  account_id text NOT NULL REFERENCES careeros_accounts(id),
  operation_id text NOT NULL,
  request_hash bytea NOT NULL CHECK (octet_length(request_hash)=32),
  profile_id bytea NOT NULL, committed_revision bigint NOT NULL,
  PRIMARY KEY (account_id,operation_id)
);

-- Content-free, monotonic deletion authority. No timer expires these markers.
CREATE TABLE IF NOT EXISTS careeros_deletion_operations (
 account_id text NOT NULL REFERENCES careeros_accounts(id), operation_id text NOT NULL,
 request_hash bytea NOT NULL CHECK(octet_length(request_hash)=32),
 kind text NOT NULL CHECK(kind IN ('profile','account')),
 profile_id bytea NOT NULL, profile_revision bigint NOT NULL,
 state text NOT NULL CHECK(state IN ('fenced_pending','failed','active_content_removed')),
 fenced_at timestamptz NOT NULL, removed_at timestamptz,
 failure_code text NOT NULL DEFAULT '',
 account_generation numeric(20,0) NOT NULL CHECK(account_generation BETWEEN 1 AND 18446744073709551615),
 PRIMARY KEY(account_id,operation_id)
);
ALTER TABLE careeros_deletion_operations ADD COLUMN IF NOT EXISTS account_generation numeric(20,0) NOT NULL DEFAULT 1;
CREATE TABLE IF NOT EXISTS careeros_deleted_profiles (
 account_id text NOT NULL REFERENCES careeros_accounts(id), profile_id bytea NOT NULL,
 operation_id text NOT NULL, fenced_at timestamptz NOT NULL,
 PRIMARY KEY(account_id,profile_id),
 FOREIGN KEY(account_id,operation_id) REFERENCES careeros_deletion_operations(account_id,operation_id)
);
REVOKE ALL ON careeros_deletion_operations,careeros_deleted_profiles FROM PUBLIC;
CREATE TABLE IF NOT EXISTS careeros_deletion_status_sessions (
 id text PRIMARY KEY, account_id text NOT NULL REFERENCES careeros_accounts(id),
 token_hash bytea UNIQUE NOT NULL CHECK(octet_length(token_hash)=32),
 csrf_hash bytea NOT NULL CHECK(octet_length(csrf_hash)=32), generation numeric(20,0) NOT NULL,
 created_at timestamptz NOT NULL, expires_at timestamptz NOT NULL
);
REVOKE ALL ON careeros_deletion_status_sessions FROM PUBLIC;

-- Pilot authority is opt-in through the explicit local composition. No role,
-- invitation, clearance or grant is created by migration.
CREATE TABLE IF NOT EXISTS careeros_pilot_operators (
  account_id text PRIMARY KEY REFERENCES careeros_accounts(id)
);
CREATE TABLE IF NOT EXISTS careeros_pilot_access (
  account_id text PRIMARY KEY REFERENCES careeros_accounts(id),
  provider bytea NOT NULL, issuer bytea NOT NULL, subject bytea NOT NULL,
  invitation_id text UNIQUE,
  invited_at timestamptz, invitation_expires_at timestamptz,
  invitation_revoked boolean NOT NULL DEFAULT false,
  age_eligible boolean CHECK (age_eligible IS NULL OR age_eligible),
  eligibility_checked_at timestamptz,
  eligibility_checked_by text REFERENCES careeros_accounts(id),
  activated_at timestamptz, expires_at timestamptz,
  participant_slot smallint UNIQUE CHECK (participant_slot BETWEEN 1 AND 5),
  draft_allowance integer NOT NULL DEFAULT 5 CHECK (draft_allowance=5),
  state text NOT NULL DEFAULT 'invited' CHECK (state IN ('invited','active','suspended','revoked')),
  FOREIGN KEY(provider,issuer,subject) REFERENCES careeros_identities(provider,issuer,subject),
  CHECK ((activated_at IS NULL) = (expires_at IS NULL)),
  CHECK ((activated_at IS NULL) = (participant_slot IS NULL)),
  CHECK ((age_eligible IS NULL) = (eligibility_checked_at IS NULL)),
  CHECK ((age_eligible IS NULL) = (eligibility_checked_by IS NULL))
);
CREATE TABLE IF NOT EXISTS careeros_pilot_commands (
  actor_id text NOT NULL REFERENCES careeros_accounts(id),
  operation_id text NOT NULL,
  request_hash bytea NOT NULL CHECK (octet_length(request_hash)=32),
  invitation_id text NOT NULL,
  PRIMARY KEY(actor_id,operation_id)
);
-- Known early access end is monotonic. NULL on inherited revocations means no
-- trustworthy event time, not permission to invent a retrieval window.
ALTER TABLE careeros_pilot_access ADD COLUMN IF NOT EXISTS access_ended_at timestamptz;
ALTER TABLE careeros_pilot_access ADD COLUMN IF NOT EXISTS retrieval_held boolean NOT NULL DEFAULT false;
ALTER TABLE careeros_pilot_access ADD COLUMN IF NOT EXISTS invitation_cleaned_at timestamptz;
ALTER TABLE careeros_pilot_access ALTER COLUMN provider DROP NOT NULL;
ALTER TABLE careeros_pilot_access ALTER COLUMN issuer DROP NOT NULL;
ALTER TABLE careeros_pilot_access ALTER COLUMN subject DROP NOT NULL;
CREATE TABLE IF NOT EXISTS careeros_invitation_cleanup (
 account_id text NOT NULL REFERENCES careeros_accounts(id), invitation_id text NOT NULL,
 expires_at timestamptz NOT NULL, fenced_at timestamptz NOT NULL, cleaned_at timestamptz,
 PRIMARY KEY(account_id,invitation_id)
);
REVOKE ALL ON careeros_invitation_cleanup FROM PUBLIC;
-- Defaults/inherited parent grants must not expose deletion/status credentials
-- to a pre-existing AI ledger runtime/support/reconciliation role.
DO $deletion_acl$
DECLARE denied record;
BEGIN
 FOR denied IN
  SELECT DISTINCT role.rolname FROM pg_class relation
  CROSS JOIN LATERAL aclexplode(COALESCE(relation.relacl,acldefault('r',relation.relowner))) permission
  JOIN pg_roles role ON role.oid=permission.grantee
  WHERE relation.oid IN ('careeros_deletion_operations'::regclass,'careeros_deleted_profiles'::regclass,'careeros_deletion_status_sessions'::regclass)
   AND permission.grantee<>relation.relowner
 LOOP
  EXECUTE format('REVOKE ALL ON careeros_deletion_operations,careeros_deleted_profiles,careeros_deletion_status_sessions FROM %I',denied.rolname);
 END LOOP;
END $deletion_acl$;
-- Fully qualified owning-schema relations and pg_temp explicitly LAST. Never
-- capture a caller's $user/public/temporary search path in a definer function.
CREATE TABLE IF NOT EXISTS careeros_profile_pruning (
 account_id text NOT NULL REFERENCES careeros_accounts(id), profile_id bytea NOT NULL,
 evidence_id text NOT NULL, previous_revision bigint NOT NULL, new_revision bigint NOT NULL,
 removed_at timestamptz NOT NULL, recovery_copy_deadline timestamptz NOT NULL,
 PRIMARY KEY(account_id,profile_id,evidence_id),
 CHECK(previous_revision>=1 AND new_revision>previous_revision)
);
REVOKE ALL ON careeros_profile_pruning FROM PUBLIC;
DO $deny_functions$
DECLARE owning_schema text;
BEGIN
 SELECT n.nspname INTO owning_schema FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid='careeros_deletion_operations'::regclass;
 IF owning_schema !~ '^[A-Za-z_][A-Za-z0-9_]*$' OR owning_schema ~ '^pg_' THEN RAISE EXCEPTION 'invalid application schema'; END IF;
 IF EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid IN ('careeros_profiles'::regclass,'careeros_deleted_profiles'::regclass,'careeros_profile_pruning'::regclass,'careeros_pilot_access'::regclass,'careeros_invitation_cleanup'::regclass) AND n.nspname<>owning_schema) THEN RAISE EXCEPTION 'mixed deletion schema'; END IF;
 EXECUTE format($definition$
 CREATE OR REPLACE FUNCTION %1$I.careeros_account_closed(owner_id text) RETURNS boolean
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $closed$
 SELECT EXISTS(SELECT 1 FROM %1$I.careeros_deletion_operations WHERE account_id=owner_id AND kind='account')
 $closed$;
 CREATE OR REPLACE FUNCTION %1$I.careeros_deleted_profile_restored(owner_id text) RETURNS boolean
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $deleted$
 SELECT EXISTS(SELECT 1 FROM %1$I.careeros_profiles p JOIN %1$I.careeros_deleted_profiles d ON p.account_id=d.account_id AND p.profile_id=d.profile_id WHERE p.account_id=owner_id)
 OR EXISTS(SELECT 1 FROM %1$I.careeros_profiles p JOIN %1$I.careeros_profile_pruning f ON f.account_id=p.account_id AND f.profile_id=p.profile_id WHERE p.account_id=owner_id AND p.revision<f.new_revision)
 $deleted$;
 CREATE OR REPLACE FUNCTION %1$I.careeros_action_predates_deletion(owner_id text,original_admission timestamptz) RETURNS boolean
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $cutoff$
 SELECT EXISTS(SELECT 1 FROM %1$I.careeros_deletion_operations WHERE account_id=owner_id AND original_admission<=fenced_at)
 $cutoff$;
 CREATE OR REPLACE FUNCTION %1$I.careeros_invitation_denied(owner_id text) RETURNS boolean
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $invitation$
 SELECT EXISTS(SELECT 1 FROM %1$I.careeros_pilot_access p JOIN %1$I.careeros_invitation_cleanup f ON p.account_id=f.account_id AND p.invitation_id=f.invitation_id WHERE p.account_id=owner_id)
 $invitation$;
 $definition$,owning_schema);
END $deny_functions$;
REVOKE ALL ON FUNCTION careeros_account_closed(text),careeros_deleted_profile_restored(text),careeros_action_predates_deletion(text,timestamptz),careeros_invitation_denied(text) FROM PUBLIC;

-- Purpose-bound lifecycle records contain no career payload or email address.
CREATE TABLE IF NOT EXISTS careeros_lifecycle_notices (
 account_id text NOT NULL REFERENCES careeros_accounts(id), notice_id text NOT NULL,
 kind text NOT NULL CHECK(kind IN ('access_ended','removal_soon')),
 due_at timestamptz NOT NULL, access_ended_at timestamptz NOT NULL, removal_at timestamptz NOT NULL,
 email_state text NOT NULL CHECK(email_state IN ('pending','sending','sent','failed','unknown','unavailable')),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 3),
 next_attempt_at timestamptz NOT NULL, operator_signal boolean NOT NULL DEFAULT false,
 PRIMARY KEY(account_id,notice_id), UNIQUE(account_id,access_ended_at,kind)
);
CREATE TABLE IF NOT EXISTS careeros_lifecycle_cleanup (
 account_id text NOT NULL, access_ended_at timestamptz NOT NULL, profile_id bytea NOT NULL,
 operation_id text NOT NULL,
 PRIMARY KEY(account_id,access_ended_at,profile_id),
 FOREIGN KEY(account_id,operation_id) REFERENCES careeros_deletion_operations(account_id,operation_id)
);
CREATE TABLE IF NOT EXISTS careeros_lifecycle_scan (
 account_id text PRIMARY KEY REFERENCES careeros_accounts(id), checked_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS careeros_lifecycle_purpose_cleanup (
 account_id text NOT NULL REFERENCES careeros_accounts(id), namespace text NOT NULL,
 action_id text NOT NULL, closed_at timestamptz NOT NULL,
 PRIMARY KEY(account_id,namespace,action_id)
);
CREATE TABLE IF NOT EXISTS careeros_lifecycle_residuals (
 account_id text NOT NULL, operation_id text NOT NULL, class text NOT NULL CHECK(class IN ('recovery_copy','recipient')),
 original_fenced_at timestamptz NOT NULL, deadline timestamptz,
 state text NOT NULL DEFAULT 'not_verified' CHECK(state IN ('not_verified','pending','failed','verified_unrecoverable')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>=1), evidence_id text NOT NULL DEFAULT '',
 PRIMARY KEY(account_id,operation_id,class),
 FOREIGN KEY(account_id,operation_id) REFERENCES careeros_deletion_operations(account_id,operation_id)
);
CREATE TABLE IF NOT EXISTS careeros_lifecycle_residual_evidence (
 account_id text NOT NULL, operation_id text NOT NULL, class text NOT NULL,
 evidence_id text NOT NULL, revision bigint NOT NULL CHECK(revision>=1), state text NOT NULL,
 request_hash bytea NOT NULL CHECK(octet_length(request_hash)=32),
 PRIMARY KEY(account_id,operation_id,class,evidence_id),
 FOREIGN KEY(account_id,operation_id,class) REFERENCES careeros_lifecycle_residuals(account_id,operation_id,class)
);
REVOKE ALL ON careeros_lifecycle_notices,careeros_lifecycle_cleanup,careeros_lifecycle_scan,careeros_lifecycle_purpose_cleanup,careeros_lifecycle_residuals,careeros_lifecycle_residual_evidence,careeros_profile_pruning,careeros_invitation_cleanup FROM PUBLIC;
DO $lifecycle_acl$
DECLARE denied record;
BEGIN
 FOR denied IN
 SELECT DISTINCT role.rolname FROM pg_class relation
 CROSS JOIN LATERAL aclexplode(COALESCE(relation.relacl,acldefault('r',relation.relowner))) permission
 JOIN pg_roles role ON role.oid=permission.grantee
 WHERE relation.oid IN ('careeros_lifecycle_notices'::regclass,'careeros_lifecycle_cleanup'::regclass,'careeros_lifecycle_scan'::regclass,'careeros_lifecycle_purpose_cleanup'::regclass,'careeros_lifecycle_residuals'::regclass,'careeros_lifecycle_residual_evidence'::regclass,'careeros_profile_pruning'::regclass,'careeros_invitation_cleanup'::regclass)
 AND permission.grantee<>relation.relowner
 LOOP
 EXECUTE format('REVOKE ALL ON careeros_lifecycle_notices,careeros_lifecycle_cleanup,careeros_lifecycle_scan,careeros_lifecycle_purpose_cleanup,careeros_lifecycle_residuals,careeros_lifecycle_residual_evidence,careeros_profile_pruning,careeros_invitation_cleanup FROM %I',denied.rolname);
 END LOOP;
END $lifecycle_acl$;

-- Separate invitation/inference authority. New installations are closed until
-- an explicit local synthetic initialization or verified operator recovery.
CREATE TABLE IF NOT EXISTS careeros_control_state (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 initialized boolean NOT NULL DEFAULT false,
 invitation_stopped boolean NOT NULL DEFAULT true,
 inference_stopped boolean NOT NULL DEFAULT true,
 changed_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
ALTER TABLE careeros_control_state ADD COLUMN IF NOT EXISTS inference_epoch bigint NOT NULL DEFAULT 1 CHECK(inference_epoch>0);
ALTER TABLE careeros_control_state ADD COLUMN IF NOT EXISTS system_generation bigint NOT NULL DEFAULT 1 CHECK(system_generation>0);
ALTER TABLE careeros_control_state ADD COLUMN IF NOT EXISTS operations_binding text NOT NULL DEFAULT '';
ALTER TABLE careeros_control_state ADD COLUMN IF NOT EXISTS operations_fault boolean NOT NULL DEFAULT false;
ALTER TABLE careeros_control_state ADD COLUMN IF NOT EXISTS operations_observer text NOT NULL DEFAULT '';
ALTER TABLE careeros_control_state ADD COLUMN IF NOT EXISTS operations_fault_id text NOT NULL DEFAULT '';
ALTER TABLE careeros_control_state ADD COLUMN IF NOT EXISTS operations_fault_at timestamptz;
ALTER TABLE careeros_control_state ADD COLUMN IF NOT EXISTS operations_fault_since timestamptz;
ALTER TABLE careeros_control_state ADD COLUMN IF NOT EXISTS operations_fault_kinds text[] NOT NULL DEFAULT '{}';
INSERT INTO careeros_control_state(singleton) VALUES(true) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS careeros_control_operators (
 account_id text PRIMARY KEY REFERENCES careeros_accounts(id),
 installed_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE IF NOT EXISTS careeros_control_commands (
 actor_id text NOT NULL, operation_id text NOT NULL,
 request_hash bytea NOT NULL CHECK(octet_length(request_hash)=32),
 result bytea NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(actor_id,operation_id)
);
CREATE TABLE IF NOT EXISTS careeros_control_accounts (
 account_id text PRIMARY KEY REFERENCES careeros_accounts(id),
 compromised boolean NOT NULL DEFAULT true,
 minimum_generation numeric(20,0) NOT NULL,
 stopped_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 prior_grant_state text, prior_retrieval_held boolean,
 held_grant_version bigint
);
CREATE TABLE IF NOT EXISTS careeros_control_recovery (
 reference text PRIMARY KEY, actor_id text NOT NULL, operation_id text NOT NULL,
 evidence bytea NOT NULL, recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(actor_id,operation_id) REFERENCES careeros_control_commands(actor_id,operation_id)
 DEFERRABLE INITIALLY DEFERRED
);
ALTER TABLE careeros_pilot_access ADD COLUMN IF NOT EXISTS authority_version bigint NOT NULL DEFAULT 1 CHECK(authority_version>0);
ALTER TABLE careeros_pilot_access ADD COLUMN IF NOT EXISTS control_compromise_held boolean NOT NULL DEFAULT false;
REVOKE ALL ON careeros_control_state,careeros_control_operators,careeros_control_commands,careeros_control_accounts,careeros_control_recovery FROM PUBLIC;
DO $control_functions$
DECLARE owning_schema text; denied record;
BEGIN
 SELECT n.nspname INTO owning_schema FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid='careeros_control_state'::regclass;
 IF owning_schema !~ '^[A-Za-z_][A-Za-z0-9_]*$' OR owning_schema ~ '^pg_' THEN RAISE EXCEPTION 'invalid control schema'; END IF;
 IF EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid IN ('careeros_control_operators'::regclass,'careeros_control_commands'::regclass,'careeros_control_accounts'::regclass,'careeros_control_recovery'::regclass,'careeros_accounts'::regclass) AND n.nspname<>owning_schema) THEN RAISE EXCEPTION 'mixed control schema'; END IF;
 EXECUTE format($definition$
 CREATE OR REPLACE FUNCTION %1$I.careeros_inference_allowed() RETURNS boolean
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $allowed$
 SELECT COALESCE((SELECT initialized AND NOT inference_stopped AND NOT operations_fault FROM %1$I.careeros_control_state WHERE singleton),false)
 $allowed$;
 CREATE OR REPLACE FUNCTION %1$I.careeros_current_inference_epoch() RETURNS bigint
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $epoch$
 SELECT COALESCE((SELECT inference_epoch FROM %1$I.careeros_control_state WHERE singleton AND initialized),0)
 $epoch$;
 CREATE OR REPLACE FUNCTION %1$I.careeros_operations_required() RETURNS boolean
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $required$
 SELECT COALESCE((SELECT operations_binding<>'' FROM %1$I.careeros_control_state WHERE singleton),true)
 $required$;
 DROP FUNCTION IF EXISTS %1$I.careeros_operations_fault_stop();
 CREATE OR REPLACE FUNCTION %1$I.careeros_control_account_denied(owner_id text) RETURNS boolean
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $denied$
 SELECT NOT EXISTS(SELECT 1 FROM %1$I.careeros_control_state WHERE singleton)
 OR EXISTS(SELECT 1 FROM %1$I.careeros_control_accounts c LEFT JOIN %1$I.careeros_accounts a ON a.id=c.account_id
 WHERE c.account_id=owner_id AND (c.compromised OR a.id IS NULL OR a.generation<c.minimum_generation))
 $denied$;
 REVOKE ALL ON FUNCTION %1$I.careeros_inference_allowed() FROM PUBLIC;
 REVOKE ALL ON FUNCTION %1$I.careeros_current_inference_epoch() FROM PUBLIC;
 REVOKE ALL ON FUNCTION %1$I.careeros_operations_required() FROM PUBLIC;
 REVOKE ALL ON FUNCTION %1$I.careeros_control_account_denied(text) FROM PUBLIC;
 $definition$,owning_schema);
 FOR denied IN
 SELECT DISTINCT role.rolname FROM pg_class relation
 CROSS JOIN LATERAL aclexplode(COALESCE(relation.relacl,acldefault('r',relation.relowner))) permission
 JOIN pg_roles role ON role.oid=permission.grantee
 WHERE relation.oid IN ('careeros_control_state'::regclass,'careeros_control_operators'::regclass,'careeros_control_commands'::regclass,'careeros_control_accounts'::regclass,'careeros_control_recovery'::regclass)
 AND permission.grantee<>relation.relowner
 LOOP
 EXECUTE format('REVOKE ALL ON careeros_control_state,careeros_control_operators,careeros_control_commands,careeros_control_accounts,careeros_control_recovery FROM %I',denied.rolname);
 END LOOP;
END $control_functions$;

-- Ordinary support and operational observation are distinct capabilities from
-- the manual financial and authoritative stop/reopen operators.
CREATE TABLE IF NOT EXISTS careeros_operations_roles (
 account_id text NOT NULL REFERENCES careeros_accounts(id),
 role text NOT NULL CHECK(role IN ('owner','support')),
 installed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id,role)
);
REVOKE ALL ON careeros_operations_roles FROM PUBLIC;
CREATE TABLE IF NOT EXISTS careeros_operations_containment (
 incident_id text NOT NULL, incident_revision bigint NOT NULL CHECK(incident_revision>0),
 invitation_stopped boolean NOT NULL, resolved boolean NOT NULL DEFAULT false,
 PRIMARY KEY(incident_id,incident_revision)
);
REVOKE ALL ON careeros_operations_containment FROM PUBLIC;
ALTER TABLE careeros_operations_containment ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT clock_timestamp();
CREATE TABLE IF NOT EXISTS careeros_operations_pending (
 id bigserial PRIMARY KEY, namespace text NOT NULL, sample bytea NOT NULL
);
ALTER TABLE careeros_operations_pending ADD COLUMN IF NOT EXISTS source_version smallint NOT NULL DEFAULT 0;
ALTER TABLE careeros_operations_pending ADD COLUMN IF NOT EXISTS critical_event boolean NOT NULL DEFAULT false;
REVOKE ALL ON careeros_operations_pending FROM PUBLIC;
CREATE TABLE IF NOT EXISTS careeros_operations_deferred (
 namespace text NOT NULL, event_id text NOT NULL, evidence jsonb NOT NULL,
 PRIMARY KEY(namespace,event_id)
);
REVOKE ALL ON careeros_operations_deferred FROM PUBLIC;

DO $operations_functions$
DECLARE owning_schema text;
BEGIN
 SELECT n.nspname INTO owning_schema FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE c.oid='careeros_operations_containment'::regclass;
 EXECUTE format($definitions$
 CREATE OR REPLACE FUNCTION %1$I.careeros_operations_attach_matches(candidate text) RETURNS boolean
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $match$
 SELECT COALESCE((SELECT operations_binding<>'' AND operations_binding=candidate
  FROM %1$I.careeros_control_state WHERE singleton),false)
 $match$;
 DROP FUNCTION IF EXISTS %1$I.careeros_operations_add_hold(text,bigint,boolean);
 CREATE OR REPLACE FUNCTION %1$I.careeros_operations_observer_matches(candidate text) RETURNS boolean
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $observer$
 SELECT COALESCE((SELECT operations_binding<>'' AND operations_observer<>'' AND operations_observer=candidate
  FROM %1$I.careeros_control_state WHERE singleton),false)
 $observer$;
 CREATE OR REPLACE FUNCTION %1$I.careeros_operations_record_emergency(reason text) RETURNS boolean
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $record$
 DECLARE observed timestamptz;
 BEGIN
  IF reason NOT IN ('database_fault','authority_fault','control_fault','auth_fault','provider_fault','backup_fault','persistence_fault','journal_cause_unconfirmed') THEN
   RAISE EXCEPTION 'invalid operations emergency';
  END IF;
  PERFORM pg_advisory_xact_lock(72481312);
  observed := clock_timestamp();
  UPDATE %1$I.careeros_control_state SET operations_fault=true,invitation_stopped=true,inference_stopped=true,
   operations_fault_id=replace(gen_random_uuid()::text||gen_random_uuid()::text,'-',''),
   operations_fault_at=observed,operations_fault_since=COALESCE(operations_fault_since,observed),
   operations_fault_kinds=CASE WHEN reason=ANY(operations_fault_kinds) THEN operations_fault_kinds
    ELSE array_append(operations_fault_kinds,reason) END,
   version=LEAST(version::numeric+1,9223372036854775807)::bigint,
   inference_epoch=LEAST(inference_epoch::numeric+1,9223372036854775807)::bigint,changed_at=observed
  WHERE singleton AND operations_binding<>'';
  RETURN FOUND;
 END $record$;
 CREATE OR REPLACE FUNCTION %1$I.careeros_operations_emergency(capability text,reason text) RETURNS boolean
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $emergency$
 BEGIN
  PERFORM pg_advisory_xact_lock(72481312);
  IF NOT %1$I.careeros_operations_observer_matches(capability) THEN RETURN false; END IF;
  RETURN %1$I.careeros_operations_record_emergency(reason);
 END $emergency$;
 CREATE OR REPLACE FUNCTION %1$I.careeros_operations_add_hold(capability text,incident text,revision bigint,invitations boolean)
 RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $hold$
 BEGIN
  IF incident !~ '^[0-9a-f]{64}$' OR revision<1 THEN RAISE EXCEPTION 'invalid operation hold'; END IF;
  PERFORM pg_advisory_xact_lock(72481312);
  IF NOT %1$I.careeros_operations_observer_matches(capability) THEN RAISE EXCEPTION 'operations observer unavailable'; END IF;
  INSERT INTO %1$I.careeros_operations_containment(incident_id,incident_revision,invitation_stopped)
  VALUES (incident,revision,invitations) ON CONFLICT DO NOTHING;
  IF FOUND THEN
   UPDATE %1$I.careeros_control_state SET inference_stopped=true,
    invitation_stopped=invitation_stopped OR invitations,
    version=version+1,inference_epoch=inference_epoch+1,changed_at=clock_timestamp()
   WHERE singleton AND operations_binding<>'' AND version<9223372036854775807 AND inference_epoch<9223372036854775807;
   IF NOT FOUND THEN RAISE EXCEPTION 'operations control authority unavailable'; END IF;
  END IF;
 END $hold$;
 REVOKE ALL ON FUNCTION %1$I.careeros_operations_attach_matches(text),%1$I.careeros_operations_observer_matches(text),
  %1$I.careeros_operations_record_emergency(text),%1$I.careeros_operations_emergency(text,text),
  %1$I.careeros_operations_add_hold(text,text,bigint,boolean) FROM PUBLIC;
 $definitions$,owning_schema);
END $operations_functions$;
CREATE TABLE IF NOT EXISTS careeros_operations_cases (
 id text PRIMARY KEY, purpose text NOT NULL, locale text NOT NULL,
 message text NOT NULL, example text NOT NULL, example_kind text NOT NULL,
 response text NOT NULL DEFAULT '', received_at timestamptz NOT NULL,
 review_at timestamptz NOT NULL, responded_at timestamptz, closed_at timestamptz
);
CREATE TABLE IF NOT EXISTS careeros_operations_case_holds (
 case_id text PRIMARY KEY REFERENCES careeros_operations_cases(id) ON DELETE CASCADE,
 purpose text NOT NULL, review_at timestamptz NOT NULL, retired_at timestamptz
);
ALTER TABLE careeros_operations_cases ADD COLUMN IF NOT EXISTS intake_at timestamptz;
CREATE TABLE IF NOT EXISTS careeros_operations_diagnostics (
 id text PRIMARY KEY, kind text NOT NULL, build text NOT NULL, event_at timestamptz NOT NULL
);
REVOKE ALL ON careeros_operations_cases,careeros_operations_case_holds,careeros_operations_diagnostics FROM PUBLIC;

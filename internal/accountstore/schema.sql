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

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

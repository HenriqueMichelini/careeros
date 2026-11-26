DO $draft_setup$
BEGIN
 IF current_database() <> 'careeros_294' OR current_user <> 'careeros_294_migrator' OR session_user <> current_user THEN
  RAISE EXCEPTION 'isolated hosted Draft migrator required';
 END IF;
 -- Same exclusive stop fence as accountstore.LockControlStopTransaction.
 PERFORM pg_advisory_xact_lock(72481312);
 IF NOT EXISTS (SELECT 1 FROM careeros_294.careeros_control_state WHERE singleton AND invitation_stopped AND inference_stopped) THEN
  RAISE EXCEPTION 'closed Account controls required for Draft migration';
 END IF;
END $draft_setup$;

-- A separately approved authority must pin this row. The migration and both
-- service logins have no way to insert one through the exposed API.
CREATE TABLE IF NOT EXISTS careeros_294_draft.fake_scope (
 singleton boolean PRIMARY KEY CHECK (singleton),
 action_id text NOT NULL UNIQUE CHECK (action_id ~ '^[A-Za-z0-9_-]{1,128}$'),
 account_id text NOT NULL REFERENCES careeros_294.careeros_accounts(id),
 grant_activated_at timestamptz NOT NULL,
 grant_version bigint NOT NULL CHECK (grant_version>0),
 profile_id bytea NOT NULL,
 profile_revision bigint NOT NULL CHECK (profile_revision>0),
 inference_epoch bigint NOT NULL CHECK (inference_epoch>0),
 expires_at timestamptz NOT NULL
);

-- Intent and attestation are insert-only metadata. Artifacts, prompts and
-- financial/customer effects do not cross this privilege boundary.
CREATE TABLE IF NOT EXISTS careeros_294_draft.fake_intents (
 action_id text NOT NULL REFERENCES careeros_294_draft.fake_scope(action_id),
 stage smallint NOT NULL CHECK (stage BETWEEN 1 AND 4),
 session_id text NOT NULL,
 account_generation numeric(20,0) NOT NULL,
 request_digest bytea NOT NULL CHECK (octet_length(request_digest)=32),
 intended_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY (action_id,stage),
 UNIQUE (action_id,stage,request_digest)
);
CREATE TABLE IF NOT EXISTS careeros_294_draft.fake_attestations (
 action_id text NOT NULL,
 stage smallint NOT NULL,
 request_digest bytea NOT NULL,
 result_digest bytea NOT NULL CHECK (octet_length(result_digest)=32),
 attested_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY (action_id,stage),
 FOREIGN KEY (action_id,stage,request_digest) REFERENCES careeros_294_draft.fake_intents(action_id,stage,request_digest)
);
REVOKE ALL ON ALL TABLES IN SCHEMA careeros_294_draft FROM PUBLIC,careeros_294_runtime,careeros_294_draft_writer,careeros_294_draft_attestor;

CREATE OR REPLACE FUNCTION careeros_294_draft.record_fake_intent(
 draft_id text, owner_id text, active_session text, session_generation numeric,
 session_token_hash bytea, stage_name text, request_binding bytea
) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $fake_intent$
DECLARE
 stage_number smallint;
 binding careeros_294_draft.fake_scope%ROWTYPE;
 session_end timestamptz;
 grant_end timestamptz;
 checked_at timestamptz;
BEGIN
 -- Matches the no-retry, four-leg managed Draft fixture. Additional graph
 -- choices require a separate policy and cannot be supplied by callers.
 stage_number := CASE stage_name WHEN 'field_validation' THEN 1 WHEN 'generation' THEN 2
                               WHEN 'resume_support' THEN 3 WHEN 'artifact_support' THEN 4 END;
 IF session_user <> 'careeros_294_draft_writer' OR stage_number IS NULL
   OR octet_length(session_token_hash) IS DISTINCT FROM 32 OR octet_length(request_binding) IS DISTINCT FROM 32 THEN
  RETURN false;
 END IF;

 -- Match Account session and Pilot lock ordering. This establishes intent
 -- ordering only. A future dispatcher must also hold the handoff lease.
 PERFORM pg_advisory_xact_lock_shared(72481312);
 PERFORM 1 FROM careeros_294.careeros_accounts a
 WHERE a.id=owner_id AND a.status='active' AND a.generation=session_generation FOR UPDATE;
 IF NOT FOUND THEN RETURN false; END IF;
 SELECT s.expires_at INTO session_end FROM careeros_294.careeros_sessions s
 WHERE s.id=active_session AND s.account_id=owner_id AND s.generation=session_generation
   AND s.token_hash=session_token_hash FOR UPDATE;
 IF NOT FOUND OR careeros_294.careeros_account_closed(owner_id)
   OR careeros_294.careeros_control_account_denied(owner_id)
   OR careeros_294.careeros_deleted_profile_restored(owner_id) THEN RETURN false; END IF;

 SELECT f.* INTO binding FROM careeros_294_draft.fake_scope f
 WHERE f.singleton AND f.action_id=draft_id AND f.account_id=owner_id FOR SHARE;
 IF NOT FOUND THEN RETURN false; END IF;
 SELECT a.expires_at INTO grant_end FROM careeros_294.careeros_pilot_access a
 JOIN careeros_294.careeros_identities i USING(provider,issuer,subject)
 WHERE a.account_id=owner_id AND i.account_id=owner_id AND a.state='active' AND a.age_eligible
   AND NOT a.invitation_revoked AND NOT a.retrieval_held AND NOT a.control_compromise_held
   AND a.activated_at=binding.grant_activated_at AND a.authority_version=binding.grant_version
   AND a.draft_allowance=5 AND a.participant_slot BETWEEN 1 AND 5 FOR SHARE OF a;
 IF NOT FOUND OR careeros_294.careeros_invitation_denied(owner_id) THEN RETURN false; END IF;
 PERFORM 1 FROM careeros_294.careeros_profiles p
 WHERE p.account_id=owner_id AND p.profile_id=binding.profile_id AND p.revision=binding.profile_revision FOR SHARE;
 IF NOT FOUND THEN RETURN false; END IF;
 PERFORM 1 FROM careeros_294.careeros_control_state c WHERE c.singleton AND c.invitation_stopped
   AND c.inference_epoch=binding.inference_epoch FOR SHARE;
 IF NOT FOUND OR NOT careeros_294.careeros_inference_allowed() OR careeros_294.careeros_operations_required() THEN RETURN false; END IF;
 checked_at := clock_timestamp();
 IF grant_end IS NULL OR NOT isfinite(grant_end) OR NOT isfinite(session_end) OR NOT isfinite(binding.expires_at)
   OR checked_at >= LEAST(session_end,grant_end,binding.expires_at) THEN RETURN false; END IF;

 IF stage_number > 1 AND NOT EXISTS (
  SELECT 1 FROM careeros_294_draft.fake_intents i JOIN careeros_294_draft.fake_attestations a USING(action_id,stage,request_digest)
  WHERE i.action_id=draft_id AND i.stage=stage_number-1 AND i.session_id=active_session AND i.account_generation=session_generation
 ) THEN RETURN false; END IF;
 INSERT INTO careeros_294_draft.fake_intents (action_id,stage,session_id,account_generation,request_digest)
 VALUES (draft_id,stage_number,active_session,session_generation,request_binding)
 ON CONFLICT (action_id,stage) DO NOTHING;
 RETURN FOUND;
END $fake_intent$;

CREATE OR REPLACE FUNCTION careeros_294_draft.record_fake_attestation(
 draft_id text, stage_number smallint, request_binding bytea, result_binding bytea
) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $fake_attestation$
BEGIN
 IF session_user <> 'careeros_294_draft_attestor' OR stage_number IS NULL OR stage_number NOT BETWEEN 1 AND 4
   OR octet_length(request_binding) IS DISTINCT FROM 32 OR octet_length(result_binding) IS DISTINCT FROM 32 THEN
  RETURN false;
 END IF;
 -- Recording already-issued work may complete after an inference stop. The
 -- receipt cannot admit, publish, settle or reopen anything.
 INSERT INTO careeros_294_draft.fake_attestations (action_id,stage,request_digest,result_digest)
 SELECT i.action_id,i.stage,i.request_digest,result_binding FROM careeros_294_draft.fake_intents i
 WHERE i.action_id=draft_id AND i.stage=stage_number AND i.request_digest=request_binding
 ON CONFLICT (action_id,stage) DO NOTHING;
 RETURN FOUND;
END $fake_attestation$;

REVOKE ALL ON ALL FUNCTIONS IN SCHEMA careeros_294_draft FROM PUBLIC,careeros_294_runtime,careeros_294_draft_writer,careeros_294_draft_attestor;
GRANT EXECUTE ON FUNCTION careeros_294_draft.record_fake_intent(text,text,text,numeric,bytea,text,bytea) TO careeros_294_draft_writer;
GRANT EXECUTE ON FUNCTION careeros_294_draft.record_fake_attestation(text,smallint,bytea,bytea) TO careeros_294_draft_attestor;

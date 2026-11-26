-- Canonical Draft tables were created in the isolated Draft schema by the
-- migrator. The existing Account and fake-evidence roles get no ledger grants.
DO $ledger_role$
BEGIN
 IF NOT EXISTS (
  SELECT 1 FROM pg_roles r WHERE r.rolname='careeros_294_draft_ledger'
  AND r.rolcanlogin AND r.rolconnlimit BETWEEN 2 AND 4
  AND NOT (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolinherit OR r.rolreplication OR r.rolbypassrls)
  AND NOT EXISTS(SELECT 1 FROM pg_auth_members m WHERE m.member=r.oid OR m.roleid=r.oid)
  AND NOT has_database_privilege(r.oid,current_database(),'CREATE,TEMP')
  AND has_database_privilege(r.oid,current_database(),'CONNECT')
  AND NOT EXISTS(SELECT 1 FROM pg_namespace n WHERE has_schema_privilege(r.oid,n.oid,'CREATE'))
 ) THEN RAISE EXCEPTION 'isolated hosted Draft ledger role required'; END IF;
END $ledger_role$;

GRANT USAGE ON SCHEMA careeros_294_draft,careeros_294 TO careeros_294_draft_ledger;

CREATE TABLE hosted_draft_action_scope (
 singleton boolean PRIMARY KEY CHECK (singleton),
 action_id text NOT NULL UNIQUE REFERENCES fake_scope(action_id),
 creation_key text NOT NULL CHECK (creation_key ~ '^[A-Za-z0-9_-]{1,128}$')
);

CREATE TABLE hosted_draft_ledger_identity (
 singleton boolean PRIMARY KEY CHECK (singleton),
 schema_id bytea NOT NULL CHECK (octet_length(schema_id)=32)
);
REVOKE ALL ON hosted_draft_ledger_identity FROM PUBLIC,careeros_294_runtime,careeros_294_draft_writer,careeros_294_draft_attestor,careeros_294_draft_ledger;
GRANT SELECT ON hosted_draft_ledger_identity TO careeros_294_draft_ledger;

-- Only the exact owner selected by the independent scope is visible to the
-- canonical Go lock and Session matching code. UPDATE is needed for row locks.
CREATE VIEW careeros_294_draft.careeros_accounts WITH (security_barrier=true) AS
 SELECT id,status,generation FROM careeros_294.careeros_accounts
 WHERE id=(SELECT account_id FROM careeros_294_draft.fake_scope WHERE singleton);
CREATE VIEW careeros_294_draft.careeros_sessions WITH (security_barrier=true) AS
 SELECT id,account_id,token_hash,csrf_hash,generation,created_at,expires_at FROM careeros_294.careeros_sessions
 WHERE account_id=(SELECT account_id FROM careeros_294_draft.fake_scope WHERE singleton);
CREATE VIEW careeros_294_draft.careeros_profiles WITH (security_barrier=true) AS
 SELECT account_id,profile_id,revision,document FROM careeros_294.careeros_profiles
 WHERE account_id=(SELECT account_id FROM careeros_294_draft.fake_scope WHERE singleton);
CREATE VIEW careeros_294_draft.careeros_pilot_access WITH (security_barrier=true) AS
 SELECT account_id,provider,issuer,subject,activated_at,expires_at,state,age_eligible,invitation_revoked,
        access_ended_at,retrieval_held,control_compromise_held,authority_version,draft_allowance,participant_slot
 FROM careeros_294.careeros_pilot_access
 WHERE account_id=(SELECT account_id FROM careeros_294_draft.fake_scope WHERE singleton);
CREATE VIEW careeros_294_draft.careeros_identities WITH (security_barrier=true) AS
 SELECT account_id,provider,issuer,subject FROM careeros_294.careeros_identities
 WHERE account_id=(SELECT account_id FROM careeros_294_draft.fake_scope WHERE singleton);
CREATE VIEW careeros_294_draft.careeros_deleted_profiles WITH (security_barrier=true) AS
 SELECT account_id,profile_id FROM careeros_294.careeros_deleted_profiles
 WHERE account_id=(SELECT account_id FROM careeros_294_draft.fake_scope WHERE singleton);

CREATE FUNCTION hosted_canonical_draft_account_readonly() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $guard$
BEGIN
 RAISE EXCEPTION 'Draft ledger cannot update Account authority' USING ERRCODE='42501';
END $guard$;

CREATE TRIGGER hosted_draft_account_readonly INSTEAD OF UPDATE ON careeros_294_draft.careeros_accounts
 FOR EACH ROW EXECUTE FUNCTION hosted_canonical_draft_account_readonly();
CREATE TRIGGER hosted_draft_account_readonly INSTEAD OF UPDATE ON careeros_294_draft.careeros_sessions
 FOR EACH ROW EXECUTE FUNCTION hosted_canonical_draft_account_readonly();
CREATE TRIGGER hosted_draft_account_readonly INSTEAD OF UPDATE ON careeros_294_draft.careeros_profiles
 FOR EACH ROW EXECUTE FUNCTION hosted_canonical_draft_account_readonly();
CREATE TRIGGER hosted_draft_account_readonly INSTEAD OF UPDATE ON careeros_294_draft.careeros_pilot_access
 FOR EACH ROW EXECUTE FUNCTION hosted_canonical_draft_account_readonly();

CREATE FUNCTION hosted_draft_owner_matches(owner_id text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,pg_temp AS $match$
 SELECT EXISTS(SELECT 1 FROM careeros_294_draft.fake_scope s WHERE s.singleton AND s.account_id=owner_id)
$match$;

CREATE FUNCTION careeros_294_draft.careeros_account_closed(owner_id text) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $scope$
 SELECT NOT careeros_294_draft.hosted_draft_owner_matches(owner_id) OR careeros_294.careeros_account_closed(owner_id)
$scope$;
CREATE FUNCTION careeros_294_draft.careeros_deleted_profile_restored(owner_id text) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $scope$
 SELECT NOT careeros_294_draft.hosted_draft_owner_matches(owner_id) OR careeros_294.careeros_deleted_profile_restored(owner_id)
$scope$;
CREATE FUNCTION careeros_294_draft.careeros_action_predates_deletion(owner_id text,original_admission timestamptz) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $scope$
 SELECT NOT careeros_294_draft.hosted_draft_owner_matches(owner_id) OR careeros_294.careeros_action_predates_deletion(owner_id,original_admission)
$scope$;
CREATE FUNCTION careeros_294_draft.careeros_invitation_denied(owner_id text) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $scope$
 SELECT NOT careeros_294_draft.hosted_draft_owner_matches(owner_id) OR careeros_294.careeros_invitation_denied(owner_id)
$scope$;
CREATE FUNCTION careeros_294_draft.careeros_control_account_denied(owner_id text) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $scope$
 SELECT NOT careeros_294_draft.hosted_draft_owner_matches(owner_id) OR careeros_294.careeros_control_account_denied(owner_id)
$scope$;

CREATE FUNCTION hosted_draft_scope_matches(owner_id text,draft_id text,key text) RETURNS boolean
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $scope$
 SELECT EXISTS (
  SELECT 1 FROM careeros_294_draft.fake_scope s
  JOIN careeros_294_draft.hosted_draft_action_scope a USING(singleton,action_id)
  JOIN careeros_294_draft.ai_fixture_policy p ON p.namespace='careeros_294_draft'
  JOIN careeros_294.careeros_control_state c ON c.singleton
  JOIN careeros_294.careeros_profiles profile ON profile.account_id=s.account_id
  JOIN careeros_294.careeros_pilot_access grant_row ON grant_row.account_id=s.account_id
  WHERE s.singleton AND s.account_id=owner_id AND s.action_id=draft_id AND a.creation_key=key
   AND c.invitation_stopped AND s.inference_epoch=c.inference_epoch
   AND s.profile_id=profile.profile_id AND s.profile_revision=profile.revision
   AND s.grant_activated_at=grant_row.activated_at AND s.grant_version=grant_row.authority_version
   AND grant_row.state='active' AND grant_row.age_eligible AND NOT grant_row.invitation_revoked
   AND NOT grant_row.retrieval_held AND NOT grant_row.control_compromise_held
   AND grant_row.draft_allowance=5 AND grant_row.participant_slot BETWEEN 1 AND 5
   AND isfinite(s.expires_at) AND isfinite(grant_row.expires_at) AND s.expires_at<=p.expires_at
   AND clock_timestamp()<LEAST(s.expires_at,grant_row.expires_at,p.expires_at)
 )
$scope$;

CREATE FUNCTION hosted_draft_identity(bind_digest bytea)
RETURNS TABLE(owner_id text,action_id text,creation_key text)
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $identity$
 SELECT s.account_id,s.action_id,a.creation_key FROM careeros_294_draft.fake_scope s
 JOIN careeros_294_draft.hosted_draft_action_scope a USING(singleton,action_id)
 JOIN careeros_294_draft.ai_recon_key k ON k.namespace='careeros_294_draft'
 WHERE session_user='careeros_294_draft_ledger' AND k.version='input_v1' AND k.purpose='input_binding'
 AND NOT k.retired AND k.verifier_digest=bind_digest
 AND careeros_294_draft.hosted_draft_scope_matches(s.account_id,s.action_id,a.creation_key)
$identity$;

CREATE TABLE hosted_draft_handoffs (
 namespace text NOT NULL CHECK (namespace='careeros_294_draft'),
 dispatch_id text NOT NULL,
 action_id text NOT NULL REFERENCES fake_scope(action_id),
 stage text NOT NULL,
 provider text NOT NULL,
 request_digest bytea NOT NULL CHECK (octet_length(request_digest)=32),
 nonce bytea NOT NULL UNIQUE CHECK (octet_length(nonce)=32),
 backend_pid integer NOT NULL,
 expires_at timestamptz NOT NULL,
 ended_at timestamptz,
 PRIMARY KEY(namespace,dispatch_id),
 FOREIGN KEY(namespace,dispatch_id) REFERENCES ai_fixture_dispatch(namespace,id)
);
REVOKE ALL ON hosted_draft_handoffs FROM PUBLIC,careeros_294_runtime,careeros_294_draft_writer,careeros_294_draft_attestor,careeros_294_draft_ledger;

-- The canonical transaction calls this function on the backend that owns the
-- session lease. A transaction-only lock cannot satisfy the attestor after
-- this write commits. The nonce identifies exactly one handoff on that lease.
CREATE FUNCTION record_draft_handoff(dispatch_namespace text,dispatch_identity text,action_identity text,
 stage_name text,provider_name text,request_binding bytea,handoff_nonce bytea) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $handoff$
DECLARE checked_at timestamptz;
BEGIN
 IF session_user<>'careeros_294_draft_ledger' OR dispatch_namespace IS DISTINCT FROM 'careeros_294_draft'
   OR octet_length(request_binding) IS DISTINCT FROM 32 OR octet_length(handoff_nonce) IS DISTINCT FROM 32
   OR NOT careeros_294.careeros_inference_allowed() OR careeros_294.careeros_operations_required()
   OR NOT EXISTS (SELECT 1 FROM pg_locks held WHERE held.locktype='advisory' AND held.pid=pg_backend_pid()
    AND held.database=(SELECT oid FROM pg_database WHERE datname=current_database()) AND held.classid=0 AND held.objid=72481312
    AND held.objsubid=1 AND held.mode='ShareLock' AND held.granted) THEN
  RETURN false;
 END IF;
 checked_at:=clock_timestamp();
 INSERT INTO careeros_294_draft.hosted_draft_handoffs
 (namespace,dispatch_id,action_id,stage,provider,request_digest,nonce,backend_pid,expires_at)
 SELECT d.namespace,d.id,d.action,d.stage,d.provider,d.binding,handoff_nonce,pg_backend_pid(),
   LEAST(f.expires_at,p.expires_at,a.created_at+interval '25 seconds',checked_at+interval '10 seconds')
 FROM careeros_294_draft.ai_fixture_dispatch d
 JOIN careeros_294_draft.ai_fixture_action a ON a.namespace=d.namespace AND a.id=d.action
 JOIN careeros_294_draft.fake_scope f ON f.action_id=a.id AND f.account_id=a.owner
 JOIN careeros_294_draft.ai_fixture_policy p ON p.namespace=d.namespace
 WHERE d.namespace=dispatch_namespace AND d.id=dispatch_identity AND d.action=action_identity
  AND d.stage=stage_name AND d.provider=provider_name AND d.binding=request_binding
  AND d.attempt=1 AND d.leg=1 AND a.attempt=1 AND a.state='recoverable' AND a.workflow='application_draft'
  AND d.state='intent' AND d.remote_state='possibly_active' AND NOT d.local_finished AND NOT d.compacted
  AND a.control_epoch=careeros_294.careeros_current_inference_epoch()
  AND d.intent_at IS NOT NULL AND d.durable_bound IS NOT NULL AND d.intent_at<=d.durable_bound
  AND careeros_294_draft.hosted_draft_scope_matches(a.owner,a.id,a.creation_key)
  AND checked_at<LEAST(f.expires_at,p.expires_at,a.created_at+interval '25 seconds')
 ON CONFLICT DO NOTHING;
 RETURN FOUND;
END $handoff$;

CREATE FUNCTION end_draft_handoff(dispatch_identity text,handoff_nonce bytea) RETURNS boolean
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $handoff$
 WITH ended AS (
  UPDATE careeros_294_draft.hosted_draft_handoffs SET ended_at=clock_timestamp()
  WHERE namespace='careeros_294_draft' AND dispatch_id=dispatch_identity AND nonce=handoff_nonce
   AND backend_pid=pg_backend_pid() AND session_user='careeros_294_draft_ledger' AND ended_at IS NULL
  RETURNING 1
 ) SELECT EXISTS(SELECT 1 FROM ended)
$handoff$;

REVOKE ALL ON FUNCTION record_draft_handoff(text,text,text,text,text,bytea,bytea),end_draft_handoff(text,bytea)
 FROM PUBLIC,careeros_294_runtime,careeros_294_draft_writer,careeros_294_draft_attestor,careeros_294_draft_ledger;
GRANT EXECUTE ON FUNCTION record_draft_handoff(text,text,text,text,text,bytea,bytea),end_draft_handoff(text,bytea)
 TO careeros_294_draft_ledger;

CREATE FUNCTION hosted_draft_ledger_guard() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $guard$
BEGIN
 IF TG_RELID='careeros_294_draft.ai_fixture_namespace'::regclass OR
    TG_RELID='careeros_294_draft.ai_fixture_policy'::regclass THEN
  RAISE EXCEPTION 'Draft ledger policy is immutable' USING ERRCODE='42501';
 END IF;
 IF TG_RELID='careeros_294_draft.ai_fixture_scope'::regclass THEN
  IF TG_OP='INSERT' THEN
   IF NEW.namespace<>'careeros_294_draft' OR NEW.scope IS DISTINCT FROM
      (SELECT 'account:'||account_id FROM careeros_294_draft.fake_scope WHERE singleton)
      OR NEW.limit_amount<>2000000 OR NEW.capacity<>1 THEN
    RAISE EXCEPTION 'Draft ledger scope unavailable' USING ERRCODE='42501';
   END IF;
  ELSIF NEW.counted<OLD.counted OR NEW.occupied<OLD.occupied THEN
   RAISE EXCEPTION 'Draft writer cannot release exposure' USING ERRCODE='42501';
  END IF;
  RETURN NEW;
 END IF;
 IF TG_RELID='careeros_294_draft.ai_fixture_action'::regclass THEN
  IF TG_OP='INSERT' THEN
   IF NEW.namespace<>'careeros_294_draft' OR NEW.workflow<>'application_draft' OR
      NEW.execution_version<>'application-draft-local-20261104-v1' OR NOT careeros_294.careeros_inference_allowed() OR
      NOT careeros_294_draft.hosted_draft_scope_matches(NEW.owner,NEW.id,NEW.creation_key) THEN
    RAISE EXCEPTION 'Draft action outside scope' USING ERRCODE='42501';
   END IF;
   NEW.control_epoch:=careeros_294.careeros_current_inference_epoch();
  ELSIF (OLD.customer_effect AND NEW IS DISTINCT FROM OLD) OR
        (NEW.state<>OLD.state AND NOT (
         (OLD.state='recoverable' AND NEW.state IN ('failed','candidate','canceled','closed','deleted')) OR
         (OLD.state='candidate' AND NEW.state IN ('delivered','canceled','closed','deleted')) OR
         (OLD.state='failed' AND NEW.state IN ('canceled','closed','deleted')))) OR
        (NEW.result_ref IS DISTINCT FROM OLD.result_ref AND NOT (OLD.state='recoverable' AND NEW.state='candidate')) OR
        (NEW.customer_reserved AND NOT OLD.customer_reserved AND NOT EXISTS (
         SELECT 1 FROM careeros_294_draft.careeros_draft_allowance allowance WHERE allowance.namespace=NEW.namespace
         AND allowance.action_id=NEW.id AND allowance.owner=NEW.owner AND allowance.state='reserved')) OR
        (NEW.state='candidate' AND OLD.state<>'candidate' AND
         (OLD.state<>'recoverable' OR NOT OLD.customer_reserved OR
          NOT careeros_294.careeros_inference_allowed() OR
          NOT careeros_294_draft.hosted_draft_scope_matches(NEW.owner,NEW.id,NEW.creation_key) OR
          NOT EXISTS (SELECT 1 FROM careeros_294_draft.careeros_draft_actions draft
           JOIN careeros_294_draft.careeros_draft_content content USING(namespace,action_id)
           WHERE draft.namespace=NEW.namespace AND draft.action_id=NEW.id AND draft.owner=NEW.owner
           AND draft.state='ready' AND draft.artifact_version=1 AND draft.published_digest ~ '^[0-9a-f]{64}$'
           AND content.bundle IS NOT NULL) OR
          (SELECT count(*) FROM careeros_294_draft.ai_fixture_dispatch d WHERE d.namespace=NEW.namespace
           AND d.action=NEW.id AND d.attempt=1 AND d.state='observed' AND d.remote_state='terminal_proven')<>4)) OR
        ((NEW.customer_effect OR NEW.state='delivered') AND
         (OLD.state<>'candidate' OR NOT OLD.customer_reserved OR NEW.customer_reserved OR
          NOT NEW.customer_effect OR NEW.state<>'delivered' OR
          NOT EXISTS (SELECT 1 FROM careeros_294_draft.careeros_draft_allowance allowance
           JOIN careeros_294_draft.careeros_draft_opening_receipts receipt USING(namespace,action_id)
           WHERE allowance.namespace=NEW.namespace AND allowance.action_id=NEW.id AND allowance.owner=NEW.owner
           AND allowance.state='delivered' AND receipt.issued_at<=allowance.confirmed_at) OR
          NOT careeros_294_draft.hosted_draft_scope_matches(NEW.owner,NEW.id,NEW.creation_key))) THEN
   RAISE EXCEPTION 'Draft ledger action transition denied' USING ERRCODE='42501';
  END IF;
  RETURN NEW;
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.namespace<>'careeros_294_draft' OR NEW.coordinate<>'request' OR NEW.attempt<>1 OR NEW.leg<>1 OR
     NOT EXISTS (SELECT 1 FROM careeros_294_draft.ai_fixture_action a WHERE a.namespace=NEW.namespace AND a.id=NEW.action
       AND a.state='recoverable' AND careeros_294_draft.hosted_draft_scope_matches(a.owner,a.id,a.creation_key)) OR
     NOT careeros_294.careeros_inference_allowed() OR
     NEW.stage NOT IN ('field_validation','generation','resume_support','artifact_support') OR
     ((NEW.stage='field_validation')<>(NEW.provider='typesafe')) OR
     (NEW.stage<>'field_validation' AND NOT EXISTS (
       SELECT 1 FROM careeros_294_draft.ai_fixture_dispatch p WHERE p.namespace=NEW.namespace AND p.action=NEW.action
       AND p.state='observed' AND p.remote_state='terminal_proven'
       AND p.stage=CASE NEW.stage WHEN 'generation' THEN 'field_validation' WHEN 'resume_support' THEN 'generation'
                    WHEN 'artifact_support' THEN 'resume_support' END)) THEN
   RAISE EXCEPTION 'Draft dispatch outside scope or order' USING ERRCODE='42501';
  END IF;
 ELSIF OLD.state='reserved' AND NEW.state='intent' AND NEW.remote_state='possibly_active' AND
       NEW.category='unknown_maximum' AND NEW.durable_bound IS NULL AND careeros_294.careeros_inference_allowed() AND
       EXISTS (SELECT 1 FROM careeros_294_draft.ai_fixture_action a WHERE a.namespace=NEW.namespace AND a.id=NEW.action
       AND a.state='recoverable' AND careeros_294_draft.hosted_draft_scope_matches(a.owner,a.id,a.creation_key)) THEN
  NEW.intent_at:=clock_timestamp();
 ELSIF OLD.state='intent' AND NEW.state=OLD.state AND NEW.remote_state=OLD.remote_state AND
       NEW.category=OLD.category AND NEW.intent_at=OLD.intent_at AND OLD.durable_bound IS NULL AND NEW.durable_bound IS NOT NULL THEN
  NEW.durable_bound:=clock_timestamp();
 ELSE
  RAISE EXCEPTION 'Draft writer cannot attest or replay dispatch' USING ERRCODE='42501';
 END IF;
 RETURN NEW;
END $guard$;

-- Use current_user in WHEN so narrowly exposed independent security-definer
-- commands can make their own attested transition under the schema owner.
CREATE TRIGGER hosted_draft_ledger_guard BEFORE UPDATE ON ai_fixture_namespace
 FOR EACH ROW WHEN (current_user='careeros_294_draft_ledger') EXECUTE FUNCTION hosted_draft_ledger_guard();
CREATE TRIGGER hosted_draft_ledger_guard BEFORE UPDATE ON ai_fixture_policy
 FOR EACH ROW WHEN (current_user='careeros_294_draft_ledger') EXECUTE FUNCTION hosted_draft_ledger_guard();
CREATE TRIGGER hosted_draft_ledger_guard BEFORE INSERT OR UPDATE ON ai_fixture_scope
 FOR EACH ROW WHEN (current_user='careeros_294_draft_ledger') EXECUTE FUNCTION hosted_draft_ledger_guard();
CREATE TRIGGER hosted_draft_ledger_guard BEFORE INSERT OR UPDATE ON ai_fixture_action
 FOR EACH ROW WHEN (current_user='careeros_294_draft_ledger') EXECUTE FUNCTION hosted_draft_ledger_guard();
CREATE TRIGGER hosted_draft_ledger_guard BEFORE INSERT OR UPDATE ON ai_fixture_dispatch
 FOR EACH ROW WHEN (current_user='careeros_294_draft_ledger') EXECUTE FUNCTION hosted_draft_ledger_guard();

REVOKE ALL ON ai_fixture_installation,ai_fixture_namespace,ai_fixture_policy,ai_fixture_grant,
 ai_fixture_scope,ai_fixture_action,ai_fixture_stage,ai_fixture_child,ai_fixture_dispatch,ai_fixture_allocation,
 ai_fixture_evidence,ai_fixture_observation,ai_recon_key,ai_recon_action_key,ai_recon_contract,
 ai_recon_stop,hosted_draft_action_scope FROM PUBLIC,careeros_294_runtime,careeros_294_draft_writer,careeros_294_draft_attestor,careeros_294_draft_ledger;
REVOKE ALL ON careeros_294_draft.careeros_accounts,careeros_294_draft.careeros_sessions,
 careeros_294_draft.careeros_profiles,careeros_294_draft.careeros_pilot_access,
 careeros_294_draft.careeros_identities,careeros_294_draft.careeros_deleted_profiles
 FROM PUBLIC,careeros_294_runtime,careeros_294_draft_writer,careeros_294_draft_attestor,careeros_294_draft_ledger;
REVOKE ALL ON FUNCTION hosted_canonical_draft_account_readonly(),hosted_draft_owner_matches(text),
 hosted_draft_scope_matches(text,text,text),hosted_draft_identity(bytea),hosted_draft_ledger_guard(),
 careeros_294_draft.careeros_account_closed(text),careeros_294_draft.careeros_deleted_profile_restored(text),
 careeros_294_draft.careeros_action_predates_deletion(text,timestamptz),careeros_294_draft.careeros_invitation_denied(text),
 careeros_294_draft.careeros_control_account_denied(text) FROM PUBLIC,careeros_294_runtime,careeros_294_draft_writer,careeros_294_draft_attestor;

GRANT SELECT ON careeros_294_draft.careeros_accounts,careeros_294_draft.careeros_sessions,
 careeros_294_draft.careeros_profiles,careeros_294_draft.careeros_pilot_access,
 careeros_294_draft.careeros_identities,careeros_294_draft.careeros_deleted_profiles TO careeros_294_draft_ledger;
GRANT UPDATE(status) ON careeros_294_draft.careeros_accounts TO careeros_294_draft_ledger;
GRANT UPDATE(created_at) ON careeros_294_draft.careeros_sessions TO careeros_294_draft_ledger;
GRANT UPDATE(revision) ON careeros_294_draft.careeros_profiles TO careeros_294_draft_ledger;
GRANT UPDATE(state) ON careeros_294_draft.careeros_pilot_access TO careeros_294_draft_ledger;
GRANT EXECUTE ON FUNCTION hosted_draft_scope_matches(text,text,text),hosted_draft_identity(bytea),
 careeros_294_draft.careeros_account_closed(text),careeros_294_draft.careeros_deleted_profile_restored(text),
 careeros_294_draft.careeros_action_predates_deletion(text,timestamptz),careeros_294_draft.careeros_invitation_denied(text),
 careeros_294_draft.careeros_control_account_denied(text),careeros_294.careeros_inference_allowed(),
 careeros_294.careeros_current_inference_epoch(),careeros_294.careeros_operations_required() TO careeros_294_draft_ledger;

GRANT SELECT ON ai_fixture_namespace,ai_fixture_policy,ai_fixture_scope,ai_fixture_action,
 ai_fixture_stage,ai_fixture_child,ai_fixture_dispatch,ai_fixture_allocation,ai_fixture_observation,
 ai_recon_key,ai_recon_action_key,ai_recon_stop TO careeros_294_draft_ledger;
GRANT UPDATE(retired) ON ai_fixture_namespace TO careeros_294_draft_ledger;
GRANT UPDATE(expires_at) ON ai_fixture_policy TO careeros_294_draft_ledger;
GRANT INSERT(namespace,scope,limit_amount,capacity),UPDATE(counted,occupied) ON ai_fixture_scope TO careeros_294_draft_ledger;
GRANT INSERT(namespace,id,owner,creation_key,workflow,binding,profile_binding,grant_version,execution_binding,execution_version),
 UPDATE(state,result_ref,execution_finished,customer_reserved,customer_effect) ON ai_fixture_action TO careeros_294_draft_ledger;
GRANT INSERT(namespace,id,action,coordinate,stage,leg,attempt,provider,binding,maximum,floor_amount,counted,input_bytes,output_tokens,input_tokens),
 UPDATE(state,category,remote_state,intent_at,durable_bound) ON ai_fixture_dispatch TO careeros_294_draft_ledger;
GRANT INSERT ON ai_fixture_stage,ai_fixture_allocation,ai_fixture_observation,ai_recon_action_key,ai_recon_contract TO careeros_294_draft_ledger;

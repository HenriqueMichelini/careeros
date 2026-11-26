-- Install after the canonical Draft Dispatch ledger in the selected migration transaction.
CREATE TABLE IF NOT EXISTS careeros_294_draft.canonical_fake_attestations (
 namespace text NOT NULL CHECK(namespace='careeros_294_draft'),
 dispatch_id text NOT NULL,
 action_id text NOT NULL REFERENCES careeros_294_draft.fake_scope(action_id),
 stage text NOT NULL CHECK(stage IN ('field_validation','generation','resume_support','artifact_support')),
 provider text NOT NULL CHECK(provider IN ('typesafe','openai')),
 fixture_version text NOT NULL CHECK(fixture_version='closed-draft-java-v1'),
 request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),
 result_digest bytea NOT NULL CHECK(octet_length(result_digest)=32),
 attested_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(namespace,dispatch_id),
 UNIQUE(namespace,action_id,stage),
 FOREIGN KEY(namespace,dispatch_id) REFERENCES careeros_294_draft.ai_fixture_dispatch(namespace,id)
);

-- Existing local receipts lack this proof and cannot satisfy verification.
ALTER TABLE careeros_294_draft.canonical_fake_attestations
 ADD COLUMN IF NOT EXISTS handoff_nonce bytea CHECK(octet_length(handoff_nonce)=32);

REVOKE ALL ON careeros_294_draft.canonical_fake_attestations
 FROM PUBLIC,careeros_294_runtime,careeros_294_draft_writer,careeros_294_draft_ledger,careeros_294_draft_attestor;

DROP FUNCTION IF EXISTS careeros_294_draft.record_canonical_fake_attestation(text,text,text,text,text,bytea,bytea);
CREATE OR REPLACE FUNCTION careeros_294_draft.record_canonical_fake_attestation(
 dispatch_namespace text, dispatch_identity text, action_identity text, stage_name text, provider_name text,
 request_binding bytea, result_binding bytea, expected_handoff_nonce bytea
) RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $canonical_attestor$
DECLARE
 expected_result bytea;
 expected_provider text;
 scoped_owner text;
 handoff_backend integer;
 handoff_expires timestamptz;
 scope_count integer;
BEGIN
 IF current_database()<>'careeros_294' OR session_user<>'careeros_294_draft_attestor' OR dispatch_namespace IS DISTINCT FROM 'careeros_294_draft'
   OR octet_length(request_binding) IS DISTINCT FROM 32 OR octet_length(result_binding) IS DISTINCT FROM 32
   OR octet_length(expected_handoff_nonce) IS DISTINCT FROM 32 THEN
  RETURN false;
 END IF;
 -- Exact response bytes from closed-draft-java-v1, independently pinned in
 -- the database so the attestor login cannot authorize arbitrary result bytes.
 expected_result := CASE stage_name
  WHEN 'field_validation' THEN decode('62d032a2dae6e83c285e26e19d5e7eb53b727db89561c6d9aa83f9265e1abb1a','hex')
  WHEN 'generation' THEN decode('dd74d1e72c7accdaf2805c73b2bc29afff5eb406c6c2406080305148c6815894','hex')
  WHEN 'resume_support' THEN decode('8f0553a6a8d84a828b7cacd8c1a76c27eaa0f3f98951f2178d5086401d867fa9','hex')
  WHEN 'artifact_support' THEN decode('6053778b433fdb51d9648462070c6f589d5d89cc20898761bb442363517fb87a','hex')
 END;
 expected_provider := CASE stage_name WHEN 'field_validation' THEN 'typesafe' ELSE 'openai' END;
 IF result_binding IS DISTINCT FROM expected_result OR provider_name IS DISTINCT FROM expected_provider THEN RETURN false; END IF;

 -- Intent durability does not grant fake entry. Require the exact final
 -- handoff committed by the serving backend that owns the stop lease.
 SELECT f.account_id,h.backend_pid,h.expires_at INTO scoped_owner,handoff_backend,handoff_expires
 FROM careeros_294_draft.ai_fixture_dispatch d
 JOIN careeros_294_draft.ai_fixture_action a ON a.namespace=d.namespace AND a.id=d.action
 JOIN careeros_294_draft.fake_scope f ON f.action_id=a.id AND f.account_id=a.owner
 JOIN careeros_294_draft.hosted_draft_handoffs h ON h.namespace=d.namespace AND h.dispatch_id=d.id
   AND h.action_id=d.action AND h.stage=d.stage AND h.provider=d.provider AND h.request_digest=d.binding
 WHERE d.namespace=dispatch_namespace AND d.id=dispatch_identity AND d.action=action_identity
   AND d.stage=stage_name AND d.provider=provider_name AND d.binding=request_binding
   AND h.nonce=expected_handoff_nonce AND h.expires_at>clock_timestamp() AND h.ended_at IS NULL
   AND d.attempt=1 AND d.leg=1 AND a.attempt=1 AND a.workflow='application_draft' AND NOT d.compacted
   AND d.state='intent' AND NOT d.local_finished AND d.remote_state='possibly_active'
   AND d.intent_at IS NOT NULL AND d.durable_bound IS NOT NULL AND d.intent_at<=d.durable_bound
 FOR UPDATE OF d,h;
 IF NOT FOUND THEN RETURN false; END IF;

 PERFORM s.scope FROM careeros_294_draft.ai_fixture_scope s
 JOIN careeros_294_draft.ai_fixture_allocation allocation ON allocation.namespace=s.namespace AND allocation.scope=s.scope
 WHERE allocation.namespace=dispatch_namespace AND allocation.dispatch=dispatch_identity
   AND s.scope IN ('global','account:'||scoped_owner,'provider:'||provider_name) AND s.occupied>0
 ORDER BY s.scope FOR UPDATE OF s;
 GET DIAGNOSTICS scope_count = ROW_COUNT;
 IF scope_count<>3 OR (SELECT count(*) FROM careeros_294_draft.ai_fixture_allocation
   WHERE namespace=dispatch_namespace AND dispatch=dispatch_identity)<>3 THEN RETURN false; END IF;

 -- A second blocking lease could queue behind the stop waiting on serving.
 -- Holding the handoff row serializes its revocation. Inspect the issuing
 -- connection's still-granted lock after row lock waits.
 IF handoff_expires<=clock_timestamp() OR NOT EXISTS (
  SELECT 1 FROM pg_locks held JOIN pg_database db ON db.oid=held.database
  WHERE db.datname=current_database() AND held.locktype='advisory' AND held.classid=0 AND held.objid=72481312
    AND held.objsubid=1 AND held.mode='ShareLock' AND held.granted AND held.pid=handoff_backend
 ) THEN RETURN false; END IF;

 INSERT INTO careeros_294_draft.canonical_fake_attestations
  (namespace,dispatch_id,action_id,stage,provider,fixture_version,request_digest,result_digest,handoff_nonce)
 VALUES (dispatch_namespace,dispatch_identity,action_identity,stage_name,provider_name,'closed-draft-java-v1',request_binding,result_binding,expected_handoff_nonce)
 ON CONFLICT DO NOTHING;
 IF NOT FOUND THEN RETURN false; END IF;

 UPDATE careeros_294_draft.ai_fixture_dispatch SET state='observed',local_finished=true,remote_state='terminal_proven'
 WHERE namespace=dispatch_namespace AND id=dispatch_identity;
 IF NOT FOUND THEN RAISE EXCEPTION 'hosted Draft Dispatch changed during attestation'; END IF;
 UPDATE careeros_294_draft.ai_fixture_scope s SET occupied=s.occupied-1
 FROM careeros_294_draft.ai_fixture_allocation allocation
 WHERE allocation.namespace=dispatch_namespace AND allocation.dispatch=dispatch_identity
   AND s.namespace=allocation.namespace AND s.scope=allocation.scope AND s.occupied>0;
 GET DIAGNOSTICS scope_count = ROW_COUNT;
 IF scope_count<>3 THEN RAISE EXCEPTION 'hosted Draft allocation changed during attestation'; END IF;
 RETURN true;
END $canonical_attestor$;

REVOKE ALL ON FUNCTION careeros_294_draft.record_canonical_fake_attestation(text,text,text,text,text,bytea,bytea,bytea)
 FROM PUBLIC,careeros_294_runtime,careeros_294_draft_writer,careeros_294_draft_ledger,careeros_294_draft_attestor;
GRANT EXECUTE ON FUNCTION careeros_294_draft.record_canonical_fake_attestation(text,text,text,text,text,bytea,bytea,bytea)
 TO careeros_294_draft_attestor;

CREATE OR REPLACE FUNCTION careeros_294_draft.canonical_fake_attestation_matches(
 dispatch_namespace text, dispatch_identity text, action_identity text, stage_name text, provider_name text,
 request_binding bytea, result_binding bytea, expected_handoff_nonce bytea
) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $verify_attestation$
 SELECT current_database()='careeros_294' AND session_user='careeros_294_draft_ledger'
  AND EXISTS (
   SELECT 1 FROM careeros_294_draft.canonical_fake_attestations p
   JOIN careeros_294_draft.ai_fixture_dispatch d ON d.namespace=p.namespace AND d.id=p.dispatch_id
    AND d.action=p.action_id AND d.stage=p.stage AND d.provider=p.provider AND d.binding=p.request_digest
   WHERE p.namespace=dispatch_namespace AND p.dispatch_id=dispatch_identity AND p.action_id=action_identity
     AND p.stage=stage_name AND p.provider=provider_name AND p.fixture_version='closed-draft-java-v1'
     AND p.request_digest=request_binding AND p.result_digest=result_binding AND p.handoff_nonce=expected_handoff_nonce
     AND d.state='observed' AND d.local_finished AND d.remote_state='terminal_proven' AND NOT d.compacted
  )
$verify_attestation$;

REVOKE ALL ON FUNCTION careeros_294_draft.canonical_fake_attestation_matches(text,text,text,text,text,bytea,bytea,bytea)
 FROM PUBLIC,careeros_294_runtime,careeros_294_draft_writer,careeros_294_draft_ledger,careeros_294_draft_attestor;
GRANT EXECUTE ON FUNCTION careeros_294_draft.canonical_fake_attestation_matches(text,text,text,text,text,bytea,bytea,bytea)
 TO careeros_294_draft_ledger;

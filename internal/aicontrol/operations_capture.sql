-- Installed only for the explicitly isolated synthetic schema. The runtime
-- supplies a namespace and optional existing reconciliation event reference.
CREATE OR REPLACE FUNCTION %[1]s.careeros_operations_snapshot(ns text, event_ref text) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $snapshot$
DECLARE
 sampled_at timestamptz := clock_timestamp();
 scopes jsonb;
 entries jsonb;
 event jsonb;
 active bigint;
 exhausted bigint := 0;
BEGIN
 IF NOT EXISTS (SELECT 1 FROM %[1]s.ai_fixture_namespace WHERE id=ns AND NOT retired)
   OR (SELECT count(*) FROM %[1]s.ai_fixture_namespace)<>1
   OR NOT EXISTS (SELECT 1 FROM %[1]s.ai_fixture_scope WHERE namespace=ns AND scope='global') THEN
  RETURN NULL;
 END IF;
 SELECT jsonb_agg(jsonb_build_object('Name',scope,'Limit',limit_amount,'Counted',counted) ORDER BY scope)
 INTO scopes FROM (
  SELECT scope,limit_amount,counted FROM %[1]s.ai_fixture_scope WHERE namespace=ns ORDER BY scope LIMIT 65537
 ) s;
 SELECT jsonb_agg(jsonb_build_object(
   'Action',x.action_id,'Owner',x.owner,'Policy',x.execution_version,'Dispatch',x.dispatch_id,
   'Attempt',x.attempt,'Amount',x.counted,'At',x.intent,'Remote',x.remote_state,'Expense',x.expense,
   'Maximum',x.maximum,'Category',x.category,'Observed',x.estimate) ORDER BY x.action_id,x.attempt,x.dispatch_id)
 INTO entries FROM (
  SELECT a.id AS action_id,a.owner,a.execution_version,d.id AS dispatch_id,d.attempt,d.counted,
   COALESCE(d.intent_at,a.created_at) AS intent,d.remote_state,d.expense,d.maximum,d.category,
   CASE WHEN jsonb_typeof(o.data->'estimate')='number' THEN
    CASE WHEN (o.data->>'estimate')::numeric BETWEEN 0 AND 9223372036854775807
      AND (o.data->>'estimate')::numeric=trunc((o.data->>'estimate')::numeric)
     THEN (o.data->>'estimate')::numeric::bigint ELSE 0 END
    ELSE 0 END AS estimate
  FROM %[1]s.ai_fixture_dispatch d JOIN %[1]s.ai_fixture_action a ON a.namespace=d.namespace AND a.id=d.action
  LEFT JOIN %[1]s.ai_fixture_observation o ON o.namespace=d.namespace AND o.dispatch=d.id AND o.sequence=1
  WHERE d.namespace=ns ORDER BY a.id,d.attempt,d.id LIMIT 65537
 ) x;
 IF COALESCE(jsonb_array_length(scopes),0)>65536 OR COALESCE(jsonb_array_length(entries),0)>65536 THEN
  RETURN NULL;
 END IF;
 SELECT count(DISTINCT a.id) INTO active FROM %[1]s.ai_fixture_action a JOIN %[1]s.ai_fixture_stage s
 ON s.namespace=a.namespace AND s.action=a.id AND s.attempt=a.attempt
 WHERE a.namespace=ns AND a.state='recoverable' AND s.stage='local_execution' AND s.receipt_id='';
 IF to_regclass('%[1]s.careeros_draft_allowance') IS NOT NULL THEN
  EXECUTE 'SELECT count(*) FROM %[1]s.careeros_pilot_access p WHERE p.state=''active''
   AND p.expires_at>$1 AND p.age_eligible AND NOT p.invitation_revoked AND NOT p.retrieval_held
   AND (SELECT count(*) FROM %[1]s.careeros_draft_allowance a WHERE a.owner=p.account_id
    AND (a.state=''delivered'' OR a.state=''reserved'' AND a.deadline>$1))>=p.draft_allowance'
  INTO exhausted USING sampled_at;
 END IF;
 IF event_ref IS NOT NULL THEN
  SELECT evidence INTO event FROM %[1]s.careeros_operations_deferred
   WHERE namespace=ns AND event_id=event_ref;
  IF event IS NULL THEN RETURN NULL; END IF;
 END IF;
 RETURN jsonb_build_object('At',sampled_at,'Scopes',COALESCE(scopes,'[]'::jsonb),'Entries',
  COALESCE(entries,'[]'::jsonb),'Active',active,'Exhausted',exhausted,'Event',event);
END $snapshot$;

CREATE OR REPLACE FUNCTION %[1]s.careeros_capture_operations(ns text, event_ref text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $capture$
DECLARE captured bytea;
BEGIN
 IF NOT %[1]s.careeros_operations_required() THEN RETURN false; END IF;
 -- Allocate IDs in committed observation order, independently of readers.
 PERFORM pg_advisory_xact_lock(72481324);
 captured := convert_to(%[1]s.careeros_operations_snapshot(ns,event_ref)::text,'UTF8');
 IF captured IS NULL OR octet_length(captured)>16777216 THEN
  RAISE EXCEPTION 'operations observation unavailable';
 END IF;
 INSERT INTO %[1]s.careeros_operations_pending(namespace,sample,source_version,critical_event)
 VALUES(ns,captured,2,event_ref IS NOT NULL);
 RETURN true;
END $capture$;
REVOKE ALL ON FUNCTION %[1]s.careeros_operations_snapshot(text,text),%[1]s.careeros_capture_operations(text,text) FROM PUBLIC;

CREATE OR REPLACE FUNCTION %[1]s.careeros_capture_runtime_operations(ns text) RETURNS boolean
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $runtime$
 SELECT %[1]s.careeros_capture_operations(ns,NULL)
$runtime$;

-- A current enrolled observer and an exact transition in this transaction
-- jointly authorize the capture. The private receipt survives a failed
-- capture independently of later financial edits or corrections.
DROP FUNCTION IF EXISTS %[1]s.careeros_capture_reconciliation_operations(text,text,boolean);
CREATE OR REPLACE FUNCTION %[1]s.careeros_capture_reconciliation_operations(capability text, ns text, command_ref text, include_event boolean)
RETURNS integer LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $reconciliation$
DECLARE original jsonb;
BEGIN
 IF NOT %[1]s.careeros_operations_required() THEN RETURN 0; END IF;
 IF NOT %[1]s.careeros_operations_observer_matches(capability) THEN
  RAISE EXCEPTION 'enrolled operations observer required';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM %[1]s.ai_recon_command c
   WHERE c.namespace=ns AND c.id=command_ref AND c.xmin=pg_current_xact_id()::xid) THEN
  RAISE EXCEPTION 'current reconciliation command required';
 END IF;
 SELECT jsonb_build_object('ID',e.id,'Rate',contract.rate_id,'Amount',e.obligation,'Maximum',e.reserved,
   'Kind',CASE WHEN c.kind='unpriced' THEN 'invariant_breach' ELSE 'maximum_breach' END,'At',e.created_at)
 INTO original FROM %[1]s.ai_recon_event e
   JOIN %[1]s.ai_recon_command c ON c.namespace=e.namespace AND c.id=e.id AND c.dispatch=e.dispatch
   JOIN %[1]s.ai_recon_contract contract ON contract.namespace=e.namespace AND contract.dispatch=e.dispatch
   JOIN %[1]s.ai_fixture_dispatch d ON d.namespace=e.namespace AND d.id=e.dispatch
   JOIN %[1]s.ai_recon_stop stop ON stop.namespace=e.namespace AND stop.event_id=e.id AND stop.stopped
   WHERE e.namespace=ns AND e.id=command_ref AND e.xmin=pg_current_xact_id()::xid
     AND e.obligation=c.after_amount AND e.obligation=d.counted AND e.reserved=d.maximum
     AND (c.kind='unpriced' OR e.obligation>e.reserved);
 IF include_event AND original IS NULL THEN
  RAISE EXCEPTION 'current reconciliation event required';
 END IF;
 IF original IS NOT NULL THEN
  INSERT INTO %[1]s.careeros_operations_deferred(namespace,event_id,evidence) VALUES(ns,command_ref,original);
 END IF;
 BEGIN
  PERFORM %[1]s.careeros_capture_operations(ns,CASE WHEN include_event THEN command_ref ELSE NULL END);
 EXCEPTION WHEN OTHERS THEN
  IF NOT %[1]s.careeros_operations_record_emergency('persistence_fault') THEN RAISE; END IF;
  RETURN 2;
 END;
 DELETE FROM %[1]s.careeros_operations_deferred WHERE namespace=ns AND event_id=command_ref;
 RETURN 1;
END $reconciliation$;

CREATE OR REPLACE FUNCTION %[1]s.careeros_replay_operations_captures(ns text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $replay$
DECLARE original_ref text;
BEGIN
 IF NOT %[1]s.careeros_capture_operations(ns,NULL) THEN RETURN false; END IF;
 FOR original_ref IN SELECT event_id FROM %[1]s.careeros_operations_deferred WHERE namespace=ns
  ORDER BY (evidence->>'At')::timestamptz,event_id
 LOOP
  PERFORM %[1]s.careeros_capture_operations(ns,original_ref);
  DELETE FROM %[1]s.careeros_operations_deferred WHERE namespace=ns AND event_id=original_ref;
 END LOOP;
 RETURN true;
END $replay$;

CREATE OR REPLACE FUNCTION %[1]s.careeros_ack_operations_capture(capability text, ns text, receipt bigint, fingerprint bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $ack$
BEGIN
 IF NOT %[1]s.careeros_operations_observer_matches(capability) OR receipt<1 OR octet_length(fingerprint)<>32 THEN
  RETURN false;
 END IF;
 DELETE FROM %[1]s.careeros_operations_pending WHERE namespace=ns AND id=receipt AND sha256(sample)=fingerprint;
 RETURN NOT EXISTS(SELECT 1 FROM %[1]s.careeros_operations_pending WHERE namespace=ns AND id=receipt);
END $ack$;

REVOKE ALL ON FUNCTION %[1]s.careeros_capture_runtime_operations(text),
 %[1]s.careeros_capture_reconciliation_operations(text,text,text,boolean),
 %[1]s.careeros_replay_operations_captures(text),%[1]s.careeros_ack_operations_capture(text,text,bigint,bytea) FROM PUBLIC;

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
  SELECT jsonb_build_object('ID',e.id,'Rate',c.rate_id,'Amount',e.obligation,'Maximum',e.reserved,
    'Kind',CASE WHEN cmd.kind='unpriced' THEN 'invariant_breach' ELSE 'maximum_breach' END,
    'At',e.created_at) INTO event
  FROM %[1]s.ai_recon_event e
  JOIN %[1]s.ai_recon_command cmd ON cmd.namespace=e.namespace AND cmd.id=e.id
  JOIN %[1]s.ai_recon_contract c ON c.namespace=e.namespace AND c.dispatch=e.dispatch
  WHERE e.namespace=ns AND e.id=event_ref;
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
 INSERT INTO %[1]s.careeros_operations_pending(namespace,sample,source_version) VALUES(ns,captured,2);
 RETURN true;
END $capture$;
REVOKE ALL ON FUNCTION %[1]s.careeros_operations_snapshot(text,text),%[1]s.careeros_capture_operations(text,text) FROM PUBLIC;

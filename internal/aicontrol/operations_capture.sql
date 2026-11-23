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
   'Maximum',x.maximum,'Category',x.category,'Observed',x.estimate,'PendingPricedBreach',x.pending_breach)
   ORDER BY x.action_id,x.attempt,x.dispatch_id)
 INTO entries FROM (
  SELECT a.id AS action_id,a.owner,a.execution_version,d.id AS dispatch_id,d.attempt,d.counted,
   COALESCE(d.intent_at,a.created_at) AS intent,d.remote_state,d.expense,d.maximum,d.category,
   d.counted>d.maximum AND EXISTS(SELECT 1 FROM %[1]s.careeros_operations_deferred deferred
    WHERE deferred.namespace=d.namespace AND deferred.evidence->>'IncidentDispatch'=d.id) AS pending_breach,
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

CREATE OR REPLACE FUNCTION %[1]s.careeros_read_operations_snapshot(ns text) RETURNS jsonb
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $read$
 SELECT %[1]s.careeros_operations_snapshot(ns,NULL)
$read$;

-- Bind only the authoritative facts underlying the receipt. The current
-- counted amount is checked separately against the receipt after accounting.
CREATE OR REPLACE FUNCTION %[1]s.careeros_finance_source_binding(ns text, ref text, expected_breach boolean) RETURNS bytea
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $source_binding$
 SELECT sha256(convert_to(jsonb_build_array(
   c.dispatch,c.actor,c.kind,c.source,c.source_revision,c.supersedes,c.reason,c.before_amount,c.after_amount,c.version,
   extract(epoch FROM c.receipt_at),extract(epoch FROM c.event_at),c.binding,c.evidence_binding,c.detail,c.result,
   e.contract,e.obligation,e.reserved,extract(epoch FROM e.created_at),e.owner_review_required,
   stop.version,stop.contract,stop.obligation,stop.reserved,
   d.action,d.provider,d.state,extract(epoch FROM d.intent_at),d.maximum,COALESCE(expected_breach,d.maximum_breached),
   contract.provider,contract.model,contract.tier,contract.billing_namespace,contract.rate_id,contract.currency,
   state.version,CASE WHEN c.kind='unpriced' THEN
     jsonb_build_array(state.unpriced,state.unpriced_command,state.unpriced_source,state.unpriced_revision)
     ELSE jsonb_build_array(state.command_id,state.source,state.source_revision) END
 )::text,'UTF8'))
 FROM %[1]s.ai_recon_event e
 JOIN %[1]s.ai_recon_command c ON c.namespace=e.namespace AND c.id=e.id AND c.dispatch=e.dispatch
 JOIN %[1]s.ai_recon_state state ON state.namespace=e.namespace AND state.dispatch=e.dispatch
 JOIN %[1]s.ai_recon_contract contract ON contract.namespace=e.namespace AND contract.dispatch=e.dispatch
 JOIN %[1]s.ai_fixture_dispatch d ON d.namespace=e.namespace AND d.id=e.dispatch
 JOIN %[1]s.ai_recon_stop stop ON stop.namespace=e.namespace AND stop.event_id=e.id AND stop.stopped
 WHERE e.namespace=ns AND e.id=ref
  AND octet_length(c.detail::text)<=65536 AND octet_length(c.result::text)<=65536
  AND octet_length(e.contract)<=1024 AND stop.contract=e.contract
$source_binding$;

-- A finance-only command can attest one exact critical transition with its
-- current ordinary reviewer session. The finance database role alone cannot
-- attest it. Only an enrolled observer can later promote the private receipt.
CREATE OR REPLACE FUNCTION %[1]s.careeros_defer_finance_operations(ns text, command_ref text, actor_id text, session_id text, session_token text)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $finance$
DECLARE dispatch_ref text; action_ref text;
BEGIN
 IF NOT %[1]s.careeros_operations_required() THEN RETURN; END IF;
 IF ns IS NULL OR ns !~ '^[A-Za-z0-9_-]{1,128}$' OR command_ref IS NULL OR command_ref !~ '^[A-Za-z0-9_-]{1,128}$'
  OR actor_id IS NULL OR octet_length(actor_id) NOT BETWEEN 1 AND 128
  OR session_id IS NULL OR octet_length(session_id) NOT BETWEEN 1 AND 128
  OR session_token IS NULL OR octet_length(session_token) NOT BETWEEN 1 AND 128 THEN
  RAISE EXCEPTION 'bounded reconciliation authority required';
 END IF;
 -- Match the command's control, account, session, namespace, reviewer,
 -- action, dispatch and state order. Recheck current authority after waiting.
 PERFORM pg_advisory_xact_lock(72481312);
 PERFORM 1 FROM %[1]s.careeros_accounts WHERE id=actor_id FOR UPDATE;
 PERFORM 1 FROM %[1]s.careeros_sessions WHERE id=session_id AND account_id=actor_id FOR UPDATE;
 PERFORM 1 FROM %[1]s.ai_fixture_namespace WHERE id=ns FOR UPDATE;
 PERFORM 1 FROM %[1]s.ai_recon_operator WHERE namespace=ns AND actor=actor_id FOR SHARE;
 SELECT c.dispatch INTO dispatch_ref FROM %[1]s.ai_recon_command c
  WHERE c.namespace=ns AND c.id=command_ref AND c.xmin=pg_current_xact_id()::xid;
 IF dispatch_ref IS NULL OR dispatch_ref !~ '^[A-Za-z0-9_-]{1,128}$' THEN
  RAISE EXCEPTION 'current reconciliation dispatch required';
 END IF;
 SELECT d.action INTO action_ref FROM %[1]s.ai_fixture_dispatch d WHERE d.namespace=ns AND d.id=dispatch_ref;
 PERFORM 1 FROM %[1]s.ai_fixture_action WHERE namespace=ns AND id=action_ref FOR UPDATE;
 PERFORM 1 FROM %[1]s.ai_fixture_dispatch WHERE namespace=ns AND id=dispatch_ref FOR UPDATE;
 PERFORM 1 FROM %[1]s.ai_recon_state WHERE namespace=ns AND dispatch=dispatch_ref FOR UPDATE;
 IF %[1]s.careeros_account_closed(actor_id) OR %[1]s.careeros_deleted_profile_restored(actor_id)
  OR %[1]s.careeros_control_account_denied(actor_id) OR NOT EXISTS (
  SELECT 1 FROM %[1]s.careeros_sessions s JOIN %[1]s.careeros_accounts a ON a.id=s.account_id
    JOIN %[1]s.ai_recon_operator r ON r.namespace=ns AND r.actor=a.id
   WHERE s.id=session_id AND a.id=actor_id AND s.token_hash=sha256(convert_to(session_token,'UTF8'))
    AND s.expires_at>clock_timestamp() AND a.status='active' AND s.generation=a.generation
    AND NOT r.revoked AND r.expires_at>clock_timestamp()
 ) THEN RAISE EXCEPTION 'current reconciliation reviewer required'; END IF;
 INSERT INTO %[1]s.careeros_operations_deferred(namespace,event_id,evidence,awaiting_observer,source_dispatch,source_transaction,source_binding)
 SELECT ns,e.id,jsonb_build_object('ID',e.id,'Rate',contract.rate_id,'Amount',e.obligation,'Maximum',e.reserved,
   'IncidentDispatch',CASE WHEN c.kind<>'unpriced' AND NOT d.maximum_breached THEN d.id ELSE '' END,
   'Kind',CASE WHEN c.kind='unpriced' THEN 'invariant_breach' ELSE 'maximum_breach' END,'At',e.created_at),true,d.id,
   pg_current_xact_id(),%[1]s.careeros_finance_source_binding(ns,e.id,
     d.maximum_breached OR (c.kind<>'unpriced' AND (c.kind<>'observation' OR e.obligation>d.counted)))
 FROM %[1]s.ai_recon_event e
 JOIN %[1]s.ai_recon_command c ON c.namespace=e.namespace AND c.id=e.id AND c.dispatch=e.dispatch
 JOIN %[1]s.ai_recon_state state ON state.namespace=e.namespace AND state.dispatch=e.dispatch
 JOIN %[1]s.ai_recon_contract contract ON contract.namespace=e.namespace AND contract.dispatch=e.dispatch
 JOIN %[1]s.ai_fixture_dispatch d ON d.namespace=e.namespace AND d.id=e.dispatch
 JOIN %[1]s.ai_recon_stop stop ON stop.namespace=e.namespace AND stop.event_id=e.id AND stop.stopped
 WHERE e.namespace=ns AND e.id=command_ref AND e.dispatch=dispatch_ref AND c.actor=actor_id AND state.version=c.version
  AND d.action=action_ref AND d.intent_at IS NOT NULL AND d.state NOT IN ('reserved','aborted') AND d.provider=contract.provider
  AND contract.rate_id ~ '^[A-Za-z0-9_-]{1,128}$' AND jsonb_typeof(c.detail)='object'
  AND c.source ~ '^[A-Za-z0-9_-]{1,128}$' AND c.reason ~ '^[A-Za-z0-9_-]{1,128}$' AND c.source_revision>0
  AND c.detail->>'provider'=contract.provider AND c.detail->>'model'=contract.model AND c.detail->>'tier'=contract.tier
  AND c.detail->>'billingNamespace'=contract.billing_namespace AND c.detail->>'rate' ~ '^[A-Za-z0-9_-]{1,128}$'
  AND c.detail->>'currency' ~ '^[A-Z]{3}$' AND e.created_at BETWEEN transaction_timestamp() AND clock_timestamp()
  AND e.owner_review_required AND octet_length(e.contract)<=1024
  AND e.contract=concat_ws('/',contract.provider,contract.model,contract.tier,c.detail->>'rate',c.detail->>'currency')
  AND d.counted=c.before_amount AND d.counted>=0 AND d.maximum>0 AND e.reserved=d.maximum AND e.obligation=c.after_amount
  AND (
   (c.kind='unpriced' AND state.unpriced AND state.unpriced_command=c.id AND state.unpriced_source=c.source
    AND state.unpriced_revision=c.source_revision AND e.obligation=greatest(d.counted,d.maximum))
   OR (e.obligation>d.maximum AND (NOT d.maximum_breached OR e.obligation>d.counted)
    AND c.detail->>'rate'=contract.rate_id AND c.detail->>'currency'=contract.currency
    AND (c.kind='observation' AND e.obligation>=d.counted
     OR c.kind IN ('usage','bill','corrected_bill','maximum') AND state.command_id=c.id
      AND state.source=c.source AND state.source_revision=c.source_revision))
  )
  AND stop.obligation=e.obligation AND stop.reserved=e.reserved AND stop.contract=e.contract
  AND c.xmin=pg_current_xact_id()::xid AND e.xmin=c.xmin AND state.xmin=c.xmin AND stop.xmin=c.xmin AND d.xmin<>c.xmin;
 IF NOT FOUND THEN RAISE EXCEPTION 'current critical reconciliation transition required'; END IF;
END $finance$;

CREATE OR REPLACE FUNCTION %[1]s.careeros_assert_finance_receipt(receipt %[1]s.careeros_operations_deferred) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $assert_finance$
BEGIN
 IF receipt.source_binding IS NULL OR receipt.source_binding IS DISTINCT FROM
  %[1]s.careeros_finance_source_binding(receipt.namespace,receipt.event_id,NULL)
  OR NOT EXISTS (SELECT 1 FROM %[1]s.ai_fixture_dispatch d
    WHERE d.namespace=receipt.namespace AND d.id=receipt.source_dispatch
      AND d.counted=(receipt.evidence->>'Amount')::bigint AND d.maximum=(receipt.evidence->>'Maximum')::bigint) THEN
  RAISE EXCEPTION 'reconciliation receipt does not match authoritative accounting';
 END IF;
END $assert_finance$;

-- Validate initial accounting even if no bound row was updated. Subsequent
-- source writes check immediately, independent of the constraint's timing.
CREATE OR REPLACE FUNCTION %[1]s.careeros_verify_finance_receipt() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $verify_finance$
DECLARE receipt %[1]s.careeros_operations_deferred;
BEGIN
 IF TG_TABLE_NAME='careeros_operations_deferred' THEN
  PERFORM %[1]s.careeros_assert_finance_receipt(NEW);
 ELSE
  FOR receipt IN SELECT * FROM %[1]s.careeros_operations_deferred
   WHERE source_transaction=pg_current_xact_id_if_assigned() AND namespace=OLD.namespace
  LOOP
   PERFORM %[1]s.careeros_assert_finance_receipt(receipt);
  END LOOP;
 END IF;
 RETURN NULL;
END $verify_finance$;
DO $install_finance_receipt$
DECLARE relation text;
BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid='%[1]s.careeros_operations_deferred'::regclass
    AND tgname='careeros_verify_finance_receipt') THEN
  CREATE CONSTRAINT TRIGGER careeros_verify_finance_receipt AFTER INSERT ON %[1]s.careeros_operations_deferred
   DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (NEW.awaiting_observer)
   EXECUTE FUNCTION %[1]s.careeros_verify_finance_receipt();
 END IF;
 FOREACH relation IN ARRAY ARRAY['ai_fixture_dispatch','ai_recon_command','ai_recon_event','ai_recon_stop','ai_recon_state','ai_recon_contract']
 LOOP
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid=to_regclass('%[1]s.'||relation)
    AND tgname='careeros_guard_finance_receipt') THEN
   EXECUTE 'CREATE TRIGGER careeros_guard_finance_receipt AFTER UPDATE OR DELETE ON %[1]s.'||quote_ident(relation)
    ||' FOR EACH ROW EXECUTE FUNCTION %[1]s.careeros_verify_finance_receipt()';
  END IF;
 END LOOP;
END $install_finance_receipt$;
REVOKE ALL ON FUNCTION %[1]s.careeros_finance_source_binding(text,text,boolean),
 %[1]s.careeros_assert_finance_receipt(%[1]s.careeros_operations_deferred),%[1]s.careeros_verify_finance_receipt() FROM PUBLIC;

CREATE OR REPLACE FUNCTION %[1]s.careeros_promote_finance_operations(capability text, ns text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $promote$
DECLARE original_ref text;
BEGIN
 IF NOT %[1]s.careeros_operations_observer_matches(capability) THEN
  RAISE EXCEPTION 'enrolled operations observer required';
 END IF;
 IF NOT EXISTS (SELECT 1 FROM %[1]s.careeros_operations_deferred WHERE namespace=ns AND awaiting_observer
   AND (source_transaction IS NULL OR source_transaction IS DISTINCT FROM pg_current_xact_id_if_assigned())) THEN
  RETURN false;
 END IF;
 PERFORM pg_advisory_xact_lock(72481324);
 SELECT event_id INTO original_ref FROM %[1]s.careeros_operations_deferred
  WHERE namespace=ns AND awaiting_observer
   AND (source_transaction IS NULL OR source_transaction IS DISTINCT FROM pg_current_xact_id_if_assigned())
  ORDER BY (evidence->>'At')::timestamptz,event_id LIMIT 1 FOR UPDATE;
 IF NOT FOUND THEN RETURN false; END IF;
 IF NOT %[1]s.careeros_capture_operations(ns,original_ref) THEN RETURN false; END IF;
 DELETE FROM %[1]s.careeros_operations_deferred WHERE namespace=ns AND event_id=original_ref;
 RETURN true;
END $promote$;

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
 -- A first priced breach uses the existing dispatch identity. Unpriced
 -- evidence and subsequent material revisions have their own command IDs.
 SELECT jsonb_build_object('ID',e.id,'Rate',contract.rate_id,'Amount',e.obligation,'Maximum',e.reserved,
   'IncidentDispatch',CASE WHEN c.kind<>'unpriced' AND NOT include_event THEN e.dispatch ELSE '' END,
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
  PERFORM %[1]s.careeros_capture_operations(ns,CASE WHEN original IS NOT NULL THEN command_ref ELSE NULL END);
 EXCEPTION WHEN OTHERS THEN
  IF NOT %[1]s.careeros_operations_record_emergency('persistence_fault') THEN RAISE; END IF;
  RETURN 2;
 END;
 DELETE FROM %[1]s.careeros_operations_deferred WHERE namespace=ns AND event_id=command_ref AND NOT awaiting_observer;
 RETURN 1;
END $reconciliation$;

CREATE OR REPLACE FUNCTION %[1]s.careeros_replay_operations_captures(ns text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $replay$
DECLARE original_ref text;
BEGIN
 FOR original_ref IN SELECT event_id FROM %[1]s.careeros_operations_deferred WHERE namespace=ns
  AND (source_transaction IS NULL OR source_transaction IS DISTINCT FROM pg_current_xact_id_if_assigned())
  ORDER BY (evidence->>'At')::timestamptz,event_id
 LOOP
  PERFORM %[1]s.careeros_capture_operations(ns,original_ref);
  DELETE FROM %[1]s.careeros_operations_deferred WHERE namespace=ns AND event_id=original_ref;
 END LOOP;
 RETURN %[1]s.careeros_capture_operations(ns,NULL);
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
 %[1]s.careeros_read_operations_snapshot(text),%[1]s.careeros_defer_finance_operations(text,text,text,text,text),
 %[1]s.careeros_promote_finance_operations(text,text),
 %[1]s.careeros_capture_reconciliation_operations(text,text,text,boolean),
 %[1]s.careeros_replay_operations_captures(text),%[1]s.careeros_ack_operations_capture(text,text,bigint,bytea) FROM PUBLIC;

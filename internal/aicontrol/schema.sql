-- Synthetic-only schema. Never embedded by account-migrate or production startup.
CREATE TABLE ai_fixture_namespace (
 id text PRIMARY KEY, epoch bigint NOT NULL CHECK(epoch>0), retired boolean NOT NULL DEFAULT false
);
CREATE TABLE ai_fixture_policy (
 namespace text PRIMARY KEY REFERENCES ai_fixture_namespace(id), expires_at timestamptz NOT NULL,
 max_dispatches integer NOT NULL CHECK(max_dispatches>0), max_attempts integer NOT NULL CHECK(max_attempts>0),
 max_stages integer NOT NULL CHECK(max_stages>0), units bigint NOT NULL CHECK(units>0),
 max_input_bytes bigint NOT NULL CHECK(max_input_bytes>0), max_output_tokens bigint NOT NULL CHECK(max_output_tokens>0)
);
CREATE TABLE ai_fixture_grant (
 namespace text NOT NULL, owner text NOT NULL, expires_at timestamptz NOT NULL,
 revoked boolean NOT NULL DEFAULT false, version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 units bigint NOT NULL CHECK(units>0), PRIMARY KEY(namespace,owner)
);
CREATE TABLE ai_fixture_scope (
 namespace text NOT NULL, scope text NOT NULL, limit_amount bigint NOT NULL CHECK(limit_amount>0),
 counted numeric(30,0) NOT NULL DEFAULT 0 CHECK(counted>=0), capacity integer NOT NULL CHECK(capacity>0),
 occupied integer NOT NULL DEFAULT 0 CHECK(occupied>=0), PRIMARY KEY(namespace,scope)
);
CREATE TABLE ai_fixture_action (
 namespace text NOT NULL, id text NOT NULL, owner text NOT NULL, creation_key text NOT NULL,
 workflow text NOT NULL, binding bytea NOT NULL CHECK(octet_length(binding)=32), profile_binding bytea CHECK(octet_length(profile_binding)=32),
 execution_binding bytea NOT NULL CHECK(octet_length(execution_binding)=32), execution_version text NOT NULL,
 state text NOT NULL DEFAULT 'recoverable' CHECK(state IN ('recoverable','failed','candidate','delivered','canceled','closed','deleted')),
 customer_reserved boolean NOT NULL DEFAULT false, customer_effect boolean NOT NULL DEFAULT false, attempt integer NOT NULL DEFAULT 1 CHECK(attempt>0),
 result_ref text NOT NULL DEFAULT '', compacted boolean NOT NULL DEFAULT false,
 grant_version bigint NOT NULL CHECK(grant_version>0),
 PRIMARY KEY(namespace,id), UNIQUE(namespace,owner,creation_key),
 CHECK((NOT customer_reserved AND NOT customer_effect) OR workflow='application_draft')
);
CREATE TABLE ai_fixture_stage (
 namespace text NOT NULL, action text NOT NULL, coordinate text NOT NULL, stage text NOT NULL,
 attempt integer NOT NULL CHECK(attempt>0), binding bytea NOT NULL CHECK(octet_length(binding)=32),
 receipt_id text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(namespace,action,coordinate,stage,attempt),
 FOREIGN KEY(namespace,action) REFERENCES ai_fixture_action(namespace,id)
);
CREATE TABLE ai_fixture_child (
 namespace text NOT NULL, action text NOT NULL, id text NOT NULL,
 parent_coordinate text NOT NULL, parent_revision text NOT NULL, attempt integer NOT NULL CHECK(attempt>0),
 binding bytea NOT NULL CHECK(octet_length(binding)=32),
 PRIMARY KEY(namespace,action,id),UNIQUE(namespace,action,parent_coordinate,parent_revision,attempt),
 FOREIGN KEY(namespace,action) REFERENCES ai_fixture_action(namespace,id)
);
CREATE TABLE ai_fixture_dispatch (
 namespace text NOT NULL, id text NOT NULL, action text NOT NULL, coordinate text NOT NULL,
 stage text NOT NULL, leg integer NOT NULL CHECK(leg>0), attempt integer NOT NULL CHECK(attempt>0),
 provider text NOT NULL CHECK(provider IN ('openai','typesafe')), binding bytea NOT NULL CHECK(octet_length(binding)=32),
 maximum bigint NOT NULL CHECK(maximum>0), floor_amount bigint NOT NULL CHECK(floor_amount>=0),
 input_bytes bigint NOT NULL CHECK(input_bytes>=0), output_tokens bigint NOT NULL CHECK(output_tokens>0),
 expense bigint CHECK(expense>=0), counted bigint NOT NULL CHECK(counted>=0), evidence_version bigint NOT NULL DEFAULT 0 CHECK(evidence_version>=0),
 maximum_breached boolean NOT NULL DEFAULT false,
 category text NOT NULL DEFAULT 'reserved_maximum' CHECK(category IN ('reserved_maximum','unknown_maximum','supported_estimate','confirmed_bill','reviewed_conservative_maximum','aborted_unsent')),
 state text NOT NULL DEFAULT 'reserved' CHECK(state IN ('reserved','intent','observed','aborted')),
 remote_state text NOT NULL DEFAULT 'not_started' CHECK(remote_state IN ('not_started','possibly_active','terminal_proven')),
 local_finished boolean NOT NULL DEFAULT false, intent_at timestamptz, durable_bound timestamptz,
 compacted boolean NOT NULL DEFAULT false, PRIMARY KEY(namespace,id),
 UNIQUE(namespace,action,coordinate,stage,attempt,leg),
 FOREIGN KEY(namespace,action) REFERENCES ai_fixture_action(namespace,id)
);
CREATE TABLE ai_fixture_allocation (
 namespace text NOT NULL, dispatch text NOT NULL, scope text NOT NULL,
 PRIMARY KEY(namespace,dispatch,scope),
 FOREIGN KEY(namespace,dispatch) REFERENCES ai_fixture_dispatch(namespace,id),
 FOREIGN KEY(namespace,scope) REFERENCES ai_fixture_scope(namespace,scope)
);
CREATE TABLE ai_fixture_evidence (
 namespace text NOT NULL, id text NOT NULL, dispatch text NOT NULL,
 binding bytea NOT NULL CHECK(octet_length(binding)=32), kind text NOT NULL, data jsonb NOT NULL,
 actor text NOT NULL CHECK(actor='fixture_reviewer'),
 PRIMARY KEY(namespace,id), FOREIGN KEY(namespace,dispatch) REFERENCES ai_fixture_dispatch(namespace,id)
);
CREATE TABLE ai_fixture_observation (
 namespace text NOT NULL, dispatch text NOT NULL, sequence integer NOT NULL CHECK(sequence IN (0,1)),
 data jsonb NOT NULL, observed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(namespace,dispatch,sequence),
 FOREIGN KEY(namespace,dispatch) REFERENCES ai_fixture_dispatch(namespace,id)
);
CREATE FUNCTION ai_fixture_keep_closure_audit() RETURNS trigger LANGUAGE plpgsql AS $audit$
BEGIN
 IF OLD.kind='conservative_closure' THEN RAISE EXCEPTION 'closure audit is immutable'; END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $audit$;
CREATE TRIGGER ai_fixture_keep_closure_audit BEFORE UPDATE OR DELETE ON ai_fixture_evidence
 FOR EACH ROW EXECUTE FUNCTION ai_fixture_keep_closure_audit();

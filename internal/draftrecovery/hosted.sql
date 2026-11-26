-- Policies use only the exact scope bound by the independent authority. Its
-- clocks and remaining access are still evaluated by the canonical protocol.
CREATE OR REPLACE FUNCTION careeros_294_draft.draft_store_scope(draft_id text,owner_id text,request_key text) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $draft_store_scope$
 SELECT session_user='careeros_294_draft_ledger' AND EXISTS (
  SELECT 1 FROM careeros_294_draft.fake_scope f
  JOIN careeros_294_draft.hosted_draft_action_scope command USING(singleton,action_id)
  WHERE f.singleton
  AND (draft_id IS NULL OR f.action_id=draft_id) AND (owner_id IS NULL OR f.account_id=owner_id)
  AND (request_key IS NULL OR command.creation_key=request_key)
 )
$draft_store_scope$;
REVOKE ALL ON FUNCTION careeros_294_draft.draft_store_scope(text,text,text) FROM PUBLIC,
 careeros_294_runtime,careeros_294_draft_writer,careeros_294_draft_attestor;
GRANT EXECUTE ON FUNCTION careeros_294_draft.draft_store_scope(text,text,text) TO careeros_294_draft_ledger;

ALTER TABLE careeros_draft_admissions ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS hosted_draft_scope ON careeros_draft_admissions;
CREATE POLICY hosted_draft_scope ON careeros_draft_admissions TO careeros_294_draft_ledger
 USING (namespace='careeros_294_draft' AND careeros_294_draft.draft_store_scope(NULL,owner,creation_key));

ALTER TABLE careeros_draft_actions ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS hosted_draft_scope ON careeros_draft_actions;
CREATE POLICY hosted_draft_scope ON careeros_draft_actions TO careeros_294_draft_ledger
 USING (namespace='careeros_294_draft' AND careeros_294_draft.draft_store_scope(action_id,owner,creation_key));

ALTER TABLE careeros_draft_allowance ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS hosted_draft_scope ON careeros_draft_allowance;
CREATE POLICY hosted_draft_scope ON careeros_draft_allowance TO careeros_294_draft_ledger
 USING (namespace='careeros_294_draft' AND careeros_294_draft.draft_store_scope(action_id,owner,NULL));

-- These relations inherit their owner through foreign keys to the Action or
-- allowance. The exact bound Action and namespace suffice at the row boundary.
DO $hosted_draft_content_policies$
DECLARE relation text;
BEGIN
 FOREACH relation IN ARRAY ARRAY['careeros_draft_content','careeros_draft_reviews',
  'careeros_draft_review_commands','careeros_draft_opening_receipts'] LOOP
  EXECUTE format('ALTER TABLE careeros_294_draft.%I ENABLE ROW LEVEL SECURITY',relation);
  EXECUTE format('DROP POLICY IF EXISTS hosted_draft_scope ON careeros_294_draft.%I',relation);
  EXECUTE format('CREATE POLICY hosted_draft_scope ON careeros_294_draft.%I TO careeros_294_draft_ledger
   USING (namespace=''careeros_294_draft'' AND careeros_294_draft.draft_store_scope(action_id,NULL,NULL))',relation);
 END LOOP;
END $hosted_draft_content_policies$;

GRANT SELECT,INSERT ON careeros_draft_admissions,careeros_draft_actions,careeros_draft_content,careeros_draft_reviews,
 careeros_draft_review_commands,careeros_draft_allowance,careeros_draft_opening_receipts TO careeros_294_draft_ledger;
GRANT UPDATE (state) ON careeros_draft_admissions TO careeros_294_draft_ledger;
GRANT UPDATE (state,artifact_version,generation_attempt,published_digest,cleanup_queued) ON careeros_draft_actions TO careeros_294_draft_ledger;
GRANT UPDATE (bundle) ON careeros_draft_content TO careeros_294_draft_ledger;
GRANT UPDATE (revision,decisions,accepted_at) ON careeros_draft_reviews TO careeros_294_draft_ledger;
GRANT UPDATE (state,confirmed_at,released_at) ON careeros_draft_allowance TO careeros_294_draft_ledger;

CREATE TABLE IF NOT EXISTS careeros_294_draft.hosted_draft_store_identity (
 singleton boolean PRIMARY KEY CHECK(singleton), schema_id text NOT NULL CHECK(schema_id ~ '^[0-9a-f]{64}$'),
 artifact_key_digest bytea NOT NULL CHECK(octet_length(artifact_key_digest)=32)
);
REVOKE ALL ON careeros_294_draft.hosted_draft_store_identity FROM PUBLIC;
GRANT SELECT ON careeros_294_draft.hosted_draft_store_identity TO careeros_294_draft_ledger;

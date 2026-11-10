-- A content-free pre-admission guard freezes the original authority across the
-- separate restricted-ledger commit. No prompt, Profile or job bytes live here.
CREATE TABLE IF NOT EXISTS careeros_draft_admissions (
 namespace text NOT NULL, owner text NOT NULL REFERENCES careeros_accounts(id),
 creation_key text NOT NULL, input_digest text NOT NULL, source_digest text NOT NULL,
 profile_id text NOT NULL, profile_revision bigint NOT NULL, language text NOT NULL,
 authority_epoch numeric(20,0) NOT NULL, grant_expires_at timestamptz NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','closed','deleted')),
 PRIMARY KEY(owner,creation_key)
);
REVOKE ALL ON careeros_draft_admissions FROM PUBLIC;
-- Explicit controlled-provider migration; never installed by production startup.
-- Content is outside the restricted non-content AI ledger.
CREATE TABLE IF NOT EXISTS careeros_draft_actions (
 namespace text NOT NULL, action_id text NOT NULL, owner text NOT NULL REFERENCES careeros_accounts(id),
 creation_key text NOT NULL, input_digest text NOT NULL, source_digest text NOT NULL,
 profile_id text NOT NULL, profile_revision bigint NOT NULL,
 language text NOT NULL, generation_version text NOT NULL, review_version text NOT NULL,
 authority_epoch numeric(20,0) NOT NULL,
 admitted_at timestamptz NOT NULL, deadline timestamptz NOT NULL, grant_expires_at timestamptz NOT NULL,
 state text NOT NULL CHECK(state IN ('pending','incomplete','ready','closed','deleted')),
 artifact_version integer NOT NULL DEFAULT 0 CHECK(artifact_version IN (0,1)),
 published_digest text NOT NULL DEFAULT '', cleanup_queued boolean NOT NULL DEFAULT false,
 PRIMARY KEY(namespace,action_id), UNIQUE(owner,creation_key),
 FOREIGN KEY(namespace,action_id) REFERENCES ai_fixture_action(namespace,id)
);
CREATE TABLE IF NOT EXISTS careeros_draft_content (
 namespace text NOT NULL, action_id text NOT NULL, input bytea NOT NULL,
 bundle bytea CHECK(bundle IS NULL OR octet_length(bundle)<=4194304),
 PRIMARY KEY(namespace,action_id),
 FOREIGN KEY(namespace,action_id) REFERENCES careeros_draft_actions(namespace,action_id)
);
REVOKE ALL ON careeros_draft_content FROM PUBLIC;
REVOKE ALL ON careeros_draft_actions FROM PUBLIC;
-- Defaults may grant a parent role SELECT before these tables are created.
-- Begin with only their owner, including when ledger roles inherit that parent.
DO $draft_content_acl$
DECLARE denied record;
BEGIN
 FOR denied IN
   SELECT DISTINCT role.rolname FROM pg_class relation
   CROSS JOIN LATERAL aclexplode(COALESCE(relation.relacl,acldefault('r',relation.relowner))) permission
   JOIN pg_roles role ON role.oid=permission.grantee
   WHERE relation.oid IN ('careeros_draft_actions'::regclass,'careeros_draft_content'::regclass,'careeros_draft_admissions'::regclass)
     AND permission.grantee<>relation.relowner
 LOOP
   EXECUTE format('REVOKE ALL ON TABLE careeros_draft_actions,careeros_draft_content,careeros_draft_admissions FROM %I',denied.rolname);
 END LOOP;
END $draft_content_acl$;

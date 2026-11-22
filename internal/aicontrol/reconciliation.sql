-- Restricted non-content reconciliation. Installed only by the disposable migrator.
CREATE TABLE ai_recon_operator (
 namespace text NOT NULL REFERENCES ai_fixture_namespace(id), actor text NOT NULL,
 version bigint NOT NULL DEFAULT 1, expires_at timestamptz NOT NULL, revoked boolean NOT NULL DEFAULT false,
 PRIMARY KEY(namespace,actor)
);
CREATE TABLE ai_recon_command (
 namespace text NOT NULL, id text NOT NULL, dispatch text NOT NULL,
 binding bytea NOT NULL CHECK(octet_length(binding)=32), evidence_binding bytea NOT NULL CHECK(octet_length(evidence_binding)=32), actor text NOT NULL,
 kind text NOT NULL, source text NOT NULL, source_revision bigint NOT NULL,
 supersedes text NOT NULL, reason text NOT NULL, receipt_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 event_at timestamptz, before_amount bigint NOT NULL, after_amount bigint NOT NULL,
 version bigint NOT NULL, category text NOT NULL,
 detail jsonb, result jsonb NOT NULL, PRIMARY KEY(namespace,id), UNIQUE(namespace,dispatch,source,source_revision),
 FOREIGN KEY(namespace,dispatch) REFERENCES ai_fixture_dispatch(namespace,id)
);
CREATE TABLE ai_recon_state (
 namespace text NOT NULL, dispatch text NOT NULL, version bigint NOT NULL DEFAULT 0,
 source text NOT NULL DEFAULT '', source_revision bigint NOT NULL DEFAULT 0,
 command_id text NOT NULL DEFAULT '', disputed_floor bigint NOT NULL DEFAULT 0,
 unpriced boolean NOT NULL DEFAULT false, unpriced_command text NOT NULL DEFAULT '', unpriced_source text NOT NULL DEFAULT '', unpriced_revision bigint NOT NULL DEFAULT 0,
 next_review timestamptz, retained_purpose text NOT NULL DEFAULT '', retention_reference text NOT NULL DEFAULT '',
 PRIMARY KEY(namespace,dispatch), FOREIGN KEY(namespace,dispatch) REFERENCES ai_fixture_dispatch(namespace,id)
);
CREATE TABLE ai_recon_stop (
 namespace text PRIMARY KEY REFERENCES ai_fixture_namespace(id), stopped boolean NOT NULL DEFAULT false,
 version bigint NOT NULL DEFAULT 0, event_id text NOT NULL DEFAULT '', contract text NOT NULL DEFAULT '',
 obligation bigint NOT NULL DEFAULT 0, reserved bigint NOT NULL DEFAULT 0
);
CREATE TABLE ai_recon_event (
 namespace text NOT NULL, id text NOT NULL, dispatch text NOT NULL, contract text NOT NULL,
 obligation bigint NOT NULL, reserved bigint NOT NULL, owner_review_required boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(namespace,id)
);
CREATE TABLE ai_recon_key (
 namespace text NOT NULL, version text NOT NULL, purpose text NOT NULL,
 verifier_digest bytea NOT NULL, retired boolean NOT NULL DEFAULT false,
 checkpoint_reference text NOT NULL DEFAULT '', retirement_actor text NOT NULL DEFAULT '', reviewed_at timestamptz,
 PRIMARY KEY(namespace,version,purpose)
);

CREATE TABLE ai_recon_contract (
 namespace text NOT NULL, dispatch text NOT NULL,
 provider text NOT NULL, model text NOT NULL, tier text NOT NULL,
 rate_id text NOT NULL, currency text NOT NULL, billing_namespace text NOT NULL,
 PRIMARY KEY(namespace,dispatch), FOREIGN KEY(namespace,dispatch) REFERENCES ai_fixture_dispatch(namespace,id)
);

CREATE TABLE ai_recon_action_key (
 namespace text NOT NULL, action text NOT NULL, key_version text NOT NULL,
 PRIMARY KEY(namespace,action), FOREIGN KEY(namespace,action) REFERENCES ai_fixture_action(namespace,id),
 CHECK(length(key_version)>0)
);

CREATE TABLE ai_recon_terminal_proof (
 namespace text NOT NULL, dispatch text NOT NULL, reference text NOT NULL,
 provider text NOT NULL, version text NOT NULL, request_binding bytea NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(namespace,reference), FOREIGN KEY(namespace,dispatch) REFERENCES ai_fixture_dispatch(namespace,id)
);

CREATE TABLE ai_recon_bounds_review (
 namespace text NOT NULL, reference text NOT NULL, contract text NOT NULL,
 original_bound bigint NOT NULL, version text NOT NULL,
 PRIMARY KEY(namespace,reference)
);

CREATE TABLE ai_recon_legacy_marker (
 namespace text NOT NULL, id text NOT NULL, dispatch text NOT NULL,
 owner text NOT NULL, kind text NOT NULL, key_version text NOT NULL,
 original_binding bytea NOT NULL, payload_binding bytea NOT NULL, lineage_binding bytea NOT NULL,
 PRIMARY KEY(namespace,id)
);

CREATE TABLE ai_recon_rate_revision (
 namespace text NOT NULL, id text NOT NULL, dispatch text NOT NULL,
 binding bytea NOT NULL, source text NOT NULL, revision bigint NOT NULL,
 data jsonb NOT NULL, recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(namespace,id)
);

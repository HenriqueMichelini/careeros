package aicontrol

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"errors"
	"io"
	"net/http"
	"time"

	"professional-information-repo/internal/accountstore"
)

const hostedDraftNamespace = "careeros_294_draft"
const hostedDraftMaxResponseBytes = 1 << 20

//go:embed hosted_draft.sql
var hostedDraftSchema string

// HostedDraftCall identifies one committed, leased Draft dispatch. Request
// carries only the canonical request body and allowlisted content-type header.
type HostedDraftCall struct {
	Namespace, DispatchID, ActionID, Stage, Provider string
	RequestDigest                                    [32]byte
	HandoffNonce                                     [32]byte
	Request                                          *http.Request
}

type HostedDraftConfig struct {
	BindingKey [32]byte
	Respond    func(context.Context, HostedDraftCall) (*http.Response, error)
}

type HostedDraftMigrationConfig struct {
	BindingKeyDigest [32]byte
	ExpiresAt        time.Time
}

type hostedDraftAuthority struct {
	owner, action, creationKey string
	keyDigest                  [32]byte
	respond                    func(context.Context, HostedDraftCall) (*http.Response, error)
}

func hostedDraftSchemaID() [32]byte {
	return sha256.Sum256([]byte(fixtureSchema + "\x00" + reconciliationSchema + "\x00" + hostedDraftSchema))
}

// MigrateHostedDraft installs canonical ledger state in the separately
// bootstrapped hosted test schema. It does not create a test admission scope.
// Migration must run with both admission stops in effect.
func MigrateHostedDraft(ctx context.Context, tx *sql.Tx, cfg HostedDraftMigrationConfig) error {
	if tx == nil || cfg.BindingKeyDigest == ([32]byte{}) || cfg.ExpiresAt.IsZero() {
		return ErrDenied
	}
	if err := accountstore.LockControlStopTransaction(ctx, tx); err != nil {
		return err
	}
	var authorized bool
	if tx.QueryRowContext(ctx, `SELECT current_database()='careeros_294'
 AND current_user='careeros_294_migrator' AND session_user=current_user
 AND EXISTS (SELECT 1 FROM pg_namespace n WHERE n.nspname='careeros_294_draft' AND n.nspowner=(current_user::regrole)::oid)
 AND EXISTS (SELECT 1 FROM careeros_294.careeros_control_state WHERE singleton AND invitation_stopped AND inference_stopped)
 AND isfinite($1::timestamptz) AND $1>clock_timestamp() AND $1<=clock_timestamp()+interval '4 hours'`,
		cfg.ExpiresAt).Scan(&authorized) != nil || !authorized {
		return ErrDenied
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL search_path=careeros_294_draft,careeros_294,pg_catalog`); err != nil {
		return ErrUnavailable
	}
	var installed bool
	if tx.QueryRowContext(ctx, `SELECT to_regclass('careeros_294_draft.hosted_draft_action_scope') IS NOT NULL`).Scan(&installed) != nil {
		return ErrUnavailable
	}
	if !installed {
		for _, migration := range []string{fixtureSchema, reconciliationSchema, hostedDraftSchema} {
			if _, err := tx.ExecContext(ctx, migration); err != nil {
				return err
			}
		}
		if err := ensureAttributionFloor(ctx, tx); err != nil {
			return err
		}
		if err := installHostedDraftPolicy(ctx, tx, cfg); err != nil {
			return err
		}
		identity := hostedDraftSchemaID()
		if _, err := tx.ExecContext(ctx, `INSERT INTO hosted_draft_ledger_identity VALUES(true,$1)`, identity[:]); err != nil {
			return err
		}
	}
	identity := hostedDraftSchemaID()
	if tx.QueryRowContext(ctx, `SELECT EXISTS (
 SELECT 1 FROM ai_fixture_policy p JOIN ai_recon_key k USING(namespace)
 WHERE p.namespace=$1 AND p.expires_at=$2 AND p.max_attempts=1 AND k.version='input_v1'
 AND k.purpose='input_binding' AND NOT k.retired AND k.verifier_digest=$3)
 AND EXISTS(SELECT 1 FROM hosted_draft_ledger_identity WHERE singleton AND schema_id=$4)`,
		hostedDraftNamespace, cfg.ExpiresAt, cfg.BindingKeyDigest[:], identity[:]).Scan(&authorized) != nil || !authorized {
		return ErrConflict
	}
	return nil
}

func installHostedDraftPolicy(ctx context.Context, tx *sql.Tx, cfg HostedDraftMigrationConfig) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO ai_fixture_namespace(id,epoch) VALUES($1,1)`, hostedDraftNamespace); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO ai_fixture_policy VALUES($1,$2,4,1,4,1,$3,$4)`, hostedDraftNamespace,
		cfg.ExpiresAt, 4*(512<<10), 65536+managedDraftPolicy.OutputTotal); err != nil {
		return err
	}
	for _, name := range []string{"global", "provider:openai", "provider:typesafe"} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO ai_fixture_scope(namespace,scope,limit_amount,capacity) VALUES($1,$2,$3,1)`,
			hostedDraftNamespace, name, selectedQualificationEnvelope().GlobalTotal); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO ai_recon_key(namespace,version,purpose,verifier_digest) VALUES($1,'input_v1','input_binding',$2)`,
		hostedDraftNamespace, cfg.BindingKeyDigest[:])
	return err
}

// OpenHostedDraft opens a code-owned fake responder on a dedicated writer pool
// against the installed authority. Its only admitted work is one approved Draft.
func OpenHostedDraft(ctx context.Context, writerDB *sql.DB, cfg HostedDraftConfig) (*Harness, error) {
	if writerDB == nil || cfg.BindingKey == ([32]byte{}) || cfg.Respond == nil {
		return nil, ErrDenied
	}
	gate := NewSyntheticGate()
	policy := QualificationSyntheticConfig(hostedDraftNamespace, gate, func(_ string, request []byte) (int64, bool) {
		return int64(len(request)), len(request) > 0
	})
	policy.BindingKey, policy.ManagedDraft = cfg.BindingKey, true
	policy.MaxAttempts, policy.MaxDispatches, policy.MaxStages, policy.CustomerUnits = 1, 4, 4, 1
	proof := &hostedDraftAuthority{keyDigest: sha256.Sum256(cfg.BindingKey[:]), respond: cfg.Respond}
	if err := proof.check(ctx, writerDB); err != nil {
		return nil, err
	}
	gate.Reconcile(hostedDraftNamespace, 1)
	return &Harness{db: writerDB, cfg: policy, hosted: proof}, nil
}

func (h *Harness) HostedDraftEnabled() bool { return h != nil && h.hosted != nil }

func (h *Harness) HostedDraftOwner() string {
	if !h.HostedDraftEnabled() {
		return ""
	}
	return h.hosted.owner
}

func (h *Harness) HostedDraftReady(ctx context.Context) error {
	if !h.HostedDraftEnabled() {
		return ErrDenied
	}
	configured := *h.hosted
	return configured.check(ctx, h.db)
}

func (h *hostedDraftAuthority) check(ctx context.Context, db *sql.DB) error {
	var owner, action, creation string
	identity := hostedDraftSchemaID()
	err := db.QueryRowContext(ctx, `SELECT i.owner_id,i.action_id,i.creation_key
 FROM careeros_294_draft.hosted_draft_identity($1) i JOIN pg_roles r ON r.rolname=current_user
 WHERE current_database()='careeros_294' AND current_user='careeros_294_draft_ledger' AND session_user=current_user
 AND current_schemas(false)=ARRAY['careeros_294_draft','careeros_294','pg_catalog']::name[]
 AND r.rolcanlogin AND r.rolconnlimit BETWEEN 2 AND 4
 AND NOT (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolinherit OR r.rolreplication OR r.rolbypassrls)
 AND NOT EXISTS(SELECT 1 FROM pg_auth_members m WHERE m.member=r.oid OR m.roleid=r.oid)
 AND NOT has_database_privilege(current_user,current_database(),'CREATE,TEMP')
 AND NOT EXISTS (SELECT 1 FROM pg_namespace n WHERE has_schema_privilege(current_user,n.oid,'CREATE'))
 AND EXISTS (SELECT 1 FROM pg_stat_ssl WHERE pid=pg_backend_pid() AND ssl)
 AND EXISTS (SELECT 1 FROM careeros_294_draft.hosted_draft_ledger_identity WHERE singleton AND schema_id=$2)
 AND (SELECT count(*) FROM pg_trigger t JOIN pg_proc routine ON routine.oid=t.tgfoid
  WHERE t.tgrelid IN ('careeros_294_draft.ai_fixture_namespace'::regclass,'careeros_294_draft.ai_fixture_policy'::regclass,
   'careeros_294_draft.ai_fixture_scope'::regclass,'careeros_294_draft.ai_fixture_action'::regclass,
   'careeros_294_draft.ai_fixture_dispatch'::regclass) AND t.tgenabled='O' AND t.tgqual IS NOT NULL
  AND t.tgname='hosted_draft_ledger_guard' AND routine.oid='careeros_294_draft.hosted_draft_ledger_guard()'::regprocedure
  AND routine.prosecdef AND pg_get_userbyid(routine.proowner)='careeros_294_migrator')=5
 AND NOT EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
  WHERE n.nspname='careeros_294' AND c.relkind IN ('r','p','v','m','f')
  AND (has_any_column_privilege(current_user,c.oid,'SELECT,INSERT,UPDATE,REFERENCES') OR
       has_table_privilege(current_user,c.oid,'DELETE,TRUNCATE,TRIGGER')))
 `, h.keyDigest[:], identity[:]).Scan(&owner, &action, &creation)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDenied
	}
	if err != nil {
		return ErrUnavailable
	}
	if !tokenLabel.MatchString(owner) || !tokenLabel.MatchString(action) || !tokenLabel.MatchString(creation) ||
		(h.owner != "" && (h.owner != owner || h.action != action || h.creationKey != creation)) {
		return ErrDenied
	}
	h.owner, h.action, h.creationKey = owner, action, creation
	return nil
}

func (h *Harness) checkHostedDraftScope(ctx context.Context, tx *sql.Tx) error {
	if h.hosted == nil {
		return nil
	}
	var current bool
	if tx.QueryRowContext(ctx, `SELECT careeros_294_draft.hosted_draft_scope_matches($1,$2,$3)`,
		h.hosted.owner, h.hosted.action, h.hosted.creationKey).Scan(&current) != nil || !current {
		return ErrDenied
	}
	return nil
}

func (s *Session) recordHostedDraftHandoff(ctx context.Context, tx *sql.Tx, d Dispatch, binding []byte) ([32]byte, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nonce, ErrUnavailable
	}
	var recorded bool
	if err := tx.QueryRowContext(ctx, `SELECT careeros_294_draft.record_draft_handoff($1,$2,$3,$4,$5,$6,$7)`,
		s.h.cfg.Namespace, d.ID, d.Action, d.Stage, d.Provider, binding, nonce[:]).Scan(&recorded); err != nil || !recorded {
		return nonce, ErrDenied
	}
	return nonce, nil
}

func endHostedDraftHandoff(ctx context.Context, lease *accountstore.ControlDispatchLease, dispatchID string, nonce [32]byte) bool {
	finalize, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	tx, err := lease.Begin(finalize)
	if err != nil {
		return false
	}
	defer tx.Rollback()
	var ended bool
	err = tx.QueryRowContext(finalize, `SELECT careeros_294_draft.end_draft_handoff($1,$2)`, dispatchID, nonce[:]).Scan(&ended)
	return err == nil && ended && tx.Commit() == nil
}

func (h *Harness) checkHostedDraftResponse(ctx context.Context, d Dispatch, binding []byte, nonce [32]byte, response *http.Response) error {
	if response == nil || response.Body == nil {
		return ErrOutputUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, hostedDraftMaxResponseBytes+1))
	closeErr := response.Body.Close()
	response.Body = http.NoBody
	if err != nil || closeErr != nil || len(body) > hostedDraftMaxResponseBytes || response.StatusCode != http.StatusOK {
		return ErrOutputUnavailable
	}
	result := sha256.Sum256(body)
	var attested bool
	if err := h.db.QueryRowContext(ctx, `SELECT careeros_294_draft.canonical_fake_attestation_matches($1,$2,$3,$4,$5,$6,$7,$8)`,
		h.cfg.Namespace, d.ID, d.Action, d.Stage, d.Provider, binding, result[:], nonce[:]).Scan(&attested); err != nil || !attested {
		return ErrOutputUnavailable
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return nil
}

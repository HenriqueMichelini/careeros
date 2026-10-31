package aicontrol

import (
	"bytes"
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/auth"
)

type Session struct {
	h                    *Harness
	p                    auth.Principal
	id, workflow         string
	root, profile        []byte
	execution            []byte
	rootInput            []byte // transient request only; never persisted or logged
	attempt              int
	grantVersion         int64
	gate                 gateLease
	childID, childParent string
}

func (s *Session) ID() string { return s.id }

// Open implements the provider-free root create/recover handshake. Keys are
// account scoped; only the verified principal supplies ownership.
func (h *Harness) Open(ctx context.Context, p auth.Principal, key, workflow string, rootJSON []byte) (*Session, error) {
	if !tokenLabel.MatchString(key) || !workflows[workflow] {
		return nil, ErrDenied
	}
	binding, err := h.fingerprint(p.AccountID, "root/"+workflow, rootJSON)
	if err != nil {
		return nil, err
	}
	lease, err := h.cfg.Gate.capture(h.cfg.Namespace)
	if err != nil {
		return nil, err
	}
	tx, err := h.begin(ctx, p)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	grantVersion, err := h.checkGrant(ctx, tx, p, 0)
	if err != nil {
		return nil, err
	}
	var id, state string
	var old, profile, execution []byte
	var compacted bool
	attempt := 1
	var frozenGrant int64
	var executionVersion string
	err = tx.QueryRowContext(ctx, `SELECT id,binding,state,compacted,profile_binding,attempt,grant_version,execution_binding,execution_version FROM ai_fixture_action WHERE namespace=$1 AND owner=$2 AND creation_key=$3 FOR UPDATE`, h.cfg.Namespace, p.AccountID, key).Scan(&id, &old, &state, &compacted, &profile, &attempt, &frozenGrant, &execution, &executionVersion)
	if err == nil {
		if !hmac.Equal(old, binding) {
			return nil, ErrConflict
		}
		if compacted || state == "canceled" || state == "closed" || state == "deleted" {
			return nil, ErrDenied
		}
		if frozenGrant != grantVersion {
			return nil, ErrDenied
		}
		current, e := h.executionBinding(ctx, tx, p.AccountID, workflow)
		if e != nil {
			return nil, e
		}
		if executionVersion != h.cfg.ExecutionPolicyVersion || !hmac.Equal(execution, current) {
			return nil, ErrConflict
		}
	} else if errors.Is(err, sql.ErrNoRows) {
		id = opaque()
		if id == "" {
			return nil, ErrUnavailable
		}
		var valid bool
		err = tx.QueryRowContext(ctx, `SELECT expires_at>clock_timestamp() FROM ai_fixture_policy WHERE namespace=$1 FOR UPDATE`, h.cfg.Namespace).Scan(&valid)
		if err != nil {
			return nil, ErrUnavailable
		}
		if !valid {
			return nil, ErrDenied
		}
		if workflow == "application_draft" || workflow == "qualification_gaps" {
			profile, err = h.currentProfileBinding(ctx, tx, p.AccountID)
			if err != nil {
				return nil, err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_scope(namespace,scope,limit_amount,capacity) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, h.cfg.Namespace, "account:"+p.AccountID, h.cfg.Limit, h.cfg.Capacity)
		if err != nil {
			return nil, ErrUnavailable
		}
		execution, err = h.executionBinding(ctx, tx, p.AccountID, workflow)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_action(namespace,id,owner,creation_key,workflow,binding,profile_binding,grant_version,execution_binding,execution_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, h.cfg.Namespace, id, p.AccountID, key, workflow, binding, profile, grantVersion, execution, h.cfg.ExecutionPolicyVersion)
		if err != nil {
			return nil, ErrUnavailable
		}
	} else {
		return nil, ErrUnavailable
	}
	if err = accountstore.AuthorizeTransaction(ctx, tx, p); err != nil {
		return nil, err
	}
	if _, err = h.checkGrant(ctx, tx, p, grantVersion); err != nil {
		return nil, err
	}
	var policyValid bool
	if tx.QueryRowContext(ctx, `SELECT expires_at>clock_timestamp() FROM ai_fixture_policy WHERE namespace=$1`, h.cfg.Namespace).Scan(&policyValid) != nil {
		return nil, ErrUnavailable
	}
	if !policyValid {
		return nil, ErrDenied
	}
	if err = lease.allowed(); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, ErrUnavailable
	}
	if err = h.fault("after_create_commit"); err != nil {
		return nil, ErrUnavailable
	}
	if err = lease.allowed(); err != nil {
		return nil, err
	}
	return &Session{h: h, p: p, id: id, workflow: workflow, root: binding, rootInput: append([]byte(nil), rootJSON...), profile: profile, execution: execution, attempt: attempt, grantVersion: grantVersion, gate: lease}, nil
}

func (h *Harness) currentProfileBinding(ctx context.Context, tx *sql.Tx, owner string) ([]byte, error) {
	var raw []byte
	if tx.QueryRowContext(ctx, `SELECT document FROM careeros_profiles WHERE account_id=$1`, owner).Scan(&raw) != nil || len(raw) == 0 {
		return nil, ErrDenied
	}
	return h.fingerprint(owner, "saved-profile", raw)
}

type Dispatch struct {
	ID, Action, Stage, Coordinate, Provider, State, Category, Remote string
	Maximum, Floor, Counted                                          int64
	Expense                                                          *int64
	IntentAt, DurableBound                                           *time.Time
	LocalFinished, Compacted                                         bool
	EvidenceVersion                                                  int64
	MaximumBreached                                                  bool
}
type ActionStatus struct {
	ID, State                        string
	CustomerReserved, CustomerEffect bool
	Dispatches                       []Dispatch
}
type Leg struct {
	Provider, Coordinate, Stage string
	Number                      int
	Body, StageInput            []byte
}

func (s *Session) Send(ctx context.Context, provider string, req *http.Request) (*http.Response, error) {
	workflow, root := aidispatch.WorkflowRoot(ctx)
	if workflow != s.workflow {
		return nil, ErrDenied
	}
	rootBinding, e := s.h.fingerprint(s.p.AccountID, "root/"+workflow, root)
	if e != nil {
		return nil, e
	}
	if !hmac.Equal(rootBinding, s.root) {
		return nil, ErrConflict
	}
	raw, err := requestBody(req)
	if err != nil {
		return nil, err
	}
	coordinate := aidispatch.Coordinate(ctx)
	if coordinate == "" {
		coordinate = "request"
	}
	stage := aidiagnostics.StageName(ctx)
	if err = s.validateExecutionStage(ctx, provider, stage, raw); err != nil {
		return nil, err
	}
	// A handler stage has one logical provider leg. Reusing the same Session
	// pointer must not turn a duplicate HTTP command into another ordinal.
	leg := Leg{Provider: provider, Coordinate: coordinate, Stage: stage, Number: 1, Body: raw, StageInput: aidispatch.StageInput(ctx)}
	d, err := s.Reserve(ctx, leg)
	if err != nil {
		return nil, err
	}
	return s.invoke(ctx, d, req, raw)
}

func requestBody(req *http.Request) ([]byte, error) {
	if req == nil || req.Method != http.MethodPost || req.URL == nil || req.URL.RawQuery != "" || req.URL.Fragment != "" {
		return nil, ErrDenied
	}
	var body io.ReadCloser
	var err error
	if req.GetBody != nil {
		body, err = req.GetBody()
	} else {
		body = req.Body
	}
	if err != nil || body == nil {
		return nil, ErrDenied
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, (512<<10)+1))
	if err != nil || len(raw) > 512<<10 {
		return nil, ErrDenied
	}
	req.Body = io.NopCloser(bytes.NewReader(raw))
	return raw, nil
}

func maximum(provider string, raw []byte) (int64, error) {
	// Fixed synthetic rates, never actual provider rates. The complete bounded
	// envelope is an intentionally conservative byte-based input upper bound.
	if provider == "typesafe" {
		return 60, nil
	}
	if provider != "openai" {
		return 0, ErrDenied
	}
	var v struct {
		Model   string `json:"model"`
		Ceiling int64  `json:"max_completion_tokens"`
	}
	if json.Unmarshal(raw, &v) != nil || v.Model != "gpt-6-luna" || v.Ceiling <= 0 || v.Ceiling > 8000 {
		return 0, ErrDenied
	}
	return 60, nil // private SYN fixture maximum; not an inference-price claim
}

func (s *Session) legBinding(provider string, body, input []byte) ([]byte, error) {
	if len(input) == 0 {
		input = []byte(`{}`)
	}
	if !accountstore.StrictJSON(input) {
		return nil, ErrDenied
	}
	// Fingerprint each envelope separately so reserved credential fields cannot
	// become indirectly hashed by nesting it under another JSON object.
	bodyBinding, err := s.h.fingerprint(s.p.AccountID, "provider-body/"+provider, body)
	if err != nil {
		return nil, err
	}
	inputBinding, err := s.h.fingerprint(s.p.AccountID, "stage-input", input)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(struct{ Body, StageInput []byte }{bodyBinding, inputBinding})
	if err != nil {
		return nil, ErrDenied
	}
	return s.h.fingerprint(s.p.AccountID, "dispatch/"+provider, raw)
}

func (s *Session) Reserve(ctx context.Context, leg Leg) (Dispatch, error) {
	h := s.h
	var zero Dispatch
	if !coordinateLabel.MatchString(leg.Coordinate) || !stages[leg.Stage] || leg.Number < 1 || leg.Number > 2 {
		return zero, ErrDenied
	}
	binding, err := s.legBinding(leg.Provider, leg.Body, leg.StageInput)
	if err != nil {
		return zero, err
	}
	m, err := maximum(leg.Provider, leg.Body)
	if err != nil {
		return zero, err
	}
	tx, err := h.begin(ctx, s.p)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()
	if _, err = h.checkGrant(ctx, tx, s.p, s.grantVersion); err != nil {
		return zero, err
	}
	if err = s.checkCurrent(ctx, tx); err != nil {
		return zero, err
	}
	var state string
	var root []byte
	var compacted bool
	var currentAttempt int
	err = tx.QueryRowContext(ctx, `SELECT state,binding,compacted,attempt FROM ai_fixture_action WHERE namespace=$1 AND id=$2 AND owner=$3 FOR UPDATE`, h.cfg.Namespace, s.id, s.p.AccountID).Scan(&state, &root, &compacted, &currentAttempt)
	if err != nil {
		return zero, ErrDenied
	}
	if !hmac.Equal(root, s.root) {
		return zero, ErrConflict
	}
	if compacted || currentAttempt != s.attempt {
		return zero, ErrDenied
	}
	if err = s.validateChild(ctx, tx, leg); err != nil {
		return zero, err
	}
	if err = s.checkExecution(ctx, tx); err != nil {
		return zero, err
	}
	identity, _ := executionIdentity(s.workflow, h.cfg.ExecutionPolicyVersion, h.cfg.RatePolicyVersion)
	stageIdentity, ok := identity.Stages[leg.Stage]
	if !ok || stageIdentity.Provider != leg.Provider {
		return zero, ErrDenied
	}
	if err = validateRequestIdentity(stageIdentity, leg.Body); err != nil {
		return zero, err
	}
	var id string
	var existingBinding []byte
	err = tx.QueryRowContext(ctx, `SELECT id,binding FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND coordinate=$3 AND stage=$4 AND attempt=$5 AND leg=$6`, h.cfg.Namespace, s.id, leg.Coordinate, leg.Stage, s.attempt, leg.Number).Scan(&id, &existingBinding)
	if err == nil {
		if !hmac.Equal(binding, existingBinding) {
			return zero, ErrConflict
		}
		d, e := loadDispatch(ctx, tx, h.cfg.Namespace, id)
		return d, e
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return zero, ErrUnavailable
	}
	if state != "recoverable" {
		return zero, ErrDenied
	}
	var maxDispatches, maxAttempts, maxStages int
	var maxInput, maxOutput int64
	var expiry time.Time
	err = tx.QueryRowContext(ctx, `SELECT max_dispatches,max_attempts,max_stages,expires_at,max_input_bytes,max_output_tokens FROM ai_fixture_policy WHERE namespace=$1 FOR UPDATE`, h.cfg.Namespace).Scan(&maxDispatches, &maxAttempts, &maxStages, &expiry, &maxInput, &maxOutput)
	if err != nil {
		return zero, ErrUnavailable
	}
	if s.attempt > maxAttempts {
		return zero, ErrCapacity
	}
	var dispatchCount int
	if tx.QueryRowContext(ctx, `SELECT count(*) FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2`, h.cfg.Namespace, s.id).Scan(&dispatchCount) != nil {
		return zero, ErrUnavailable
	}
	if dispatchCount >= maxDispatches {
		return zero, ErrCapacity
	}
	var usedInput, usedOutput int64
	if tx.QueryRowContext(ctx, `SELECT COALESCE(sum(input_bytes),0),COALESCE(sum(output_tokens),0) FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2`, h.cfg.Namespace, s.id).Scan(&usedInput, &usedOutput) != nil {
		return zero, ErrUnavailable
	}
	output := int64(65536)
	if leg.Provider == "openai" {
		var v struct {
			Ceiling int64 `json:"max_completion_tokens"`
		}
		if json.Unmarshal(leg.Body, &v) != nil {
			return zero, ErrDenied
		}
		output = v.Ceiling
	}
	inputSum, e := checkedSum(usedInput, int64(len(leg.Body)))
	if e != nil || inputSum > maxInput {
		return zero, ErrCapacity
	}
	outputSum, e := checkedSum(usedOutput, output)
	if e != nil || outputSum > maxOutput {
		return zero, ErrCapacity
	}
	if err = s.checkGraph(ctx, tx, leg); err != nil {
		return zero, err
	}
	var oldStage []byte
	err = tx.QueryRowContext(ctx, `SELECT binding FROM ai_fixture_stage WHERE namespace=$1 AND action=$2 AND coordinate=$3 AND stage=$4 AND attempt=$5`, h.cfg.Namespace, s.id, leg.Coordinate, leg.Stage, s.attempt).Scan(&oldStage)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err = s.stageLimit(ctx, tx); err != nil {
			return zero, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_stage(namespace,action,coordinate,stage,attempt,binding) VALUES($1,$2,$3,$4,$5,$6)`, h.cfg.Namespace, s.id, leg.Coordinate, leg.Stage, s.attempt, binding)
		if err != nil {
			return zero, ErrUnavailable
		}
	} else if err != nil {
		return zero, ErrUnavailable
	} else if !hmac.Equal(oldStage, binding) {
		return zero, ErrConflict
	}
	scopes := []string{"global", "account:" + s.p.AccountID, "provider:" + leg.Provider}
	sort.Strings(scopes)
	for _, scope := range scopes {
		var limit, count int64
		var capacity, occupied int
		if tx.QueryRowContext(ctx, `SELECT limit_amount,counted,capacity,occupied FROM ai_fixture_scope WHERE namespace=$1 AND scope=$2 FOR UPDATE`, h.cfg.Namespace, scope).Scan(&limit, &count, &capacity, &occupied) != nil {
			return zero, ErrUnavailable
		}
		sum, e := checkedSum(count, m)
		if e != nil || sum > limit || occupied >= capacity {
			return zero, ErrCapacity
		}
	}
	if err = h.fault("after_scope_locks"); err != nil {
		return zero, err
	}
	if err = s.checkCurrent(ctx, tx); err != nil {
		return zero, err
	}
	var valid bool
	if tx.QueryRowContext(ctx, `SELECT expires_at>clock_timestamp() FROM ai_fixture_policy WHERE namespace=$1`, h.cfg.Namespace).Scan(&valid) != nil {
		return zero, ErrUnavailable
	}
	if !valid {
		return zero, ErrDenied
	}
	id = opaque()
	if id == "" {
		return zero, ErrUnavailable
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_dispatch(namespace,id,action,coordinate,stage,leg,attempt,provider,binding,maximum,floor_amount,counted,input_bytes,output_tokens) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10,$10,$11,$12)`, h.cfg.Namespace, id, s.id, leg.Coordinate, leg.Stage, leg.Number, s.attempt, leg.Provider, binding, m, len(leg.Body), output)
	if err != nil {
		return zero, ErrUnavailable
	}
	for _, scope := range scopes {
		if _, err = tx.ExecContext(ctx, `UPDATE ai_fixture_scope SET counted=counted+$3,occupied=occupied+1 WHERE namespace=$1 AND scope=$2`, h.cfg.Namespace, scope, m); err != nil {
			return zero, ErrUnavailable
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_allocation VALUES($1,$2,$3)`, h.cfg.Namespace, id, scope); err != nil {
			return zero, ErrUnavailable
		}
	}
	if err = h.fault("before_reserve_commit"); err != nil {
		return zero, err
	}
	if err = s.checkCurrent(ctx, tx); err != nil {
		return zero, err
	}
	if tx.Commit() != nil {
		return zero, ErrUnavailable
	}
	return Dispatch{ID: id, Action: s.id, Stage: leg.Stage, Coordinate: leg.Coordinate, Provider: leg.Provider, State: "reserved", Category: "reserved_maximum", Remote: "not_started", Maximum: m, Floor: m, Counted: m}, nil
}

func (s *Session) invoke(ctx context.Context, d Dispatch, req *http.Request, raw []byte) (*http.Response, error) {
	h := s.h
	if req == nil || req.URL == nil || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Port() != "" || req.URL.RawQuery != "" || req.URL.Fragment != "" || req.Method != http.MethodPost {
		return nil, ErrDenied
	}
	if !(d.Provider == "openai" && req.URL.Host == "api.openai.com" && req.URL.Path == "/v1/chat/completions" || d.Provider == "typesafe" && req.URL.Host == "api.typesafe.ai" && req.URL.Path == "/v1/systemone") {
		return nil, ErrDenied
	}
	h.mu.Lock()
	respond := h.responder
	h.mu.Unlock()
	if respond == nil {
		return nil, ErrDenied
	}
	binding, err := s.legBinding(d.Provider, raw, aidispatch.StageInput(ctx))
	if err != nil {
		return nil, err
	}
	tx, err := h.begin(ctx, s.p)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = h.checkGrant(ctx, tx, s.p, s.grantVersion); err != nil {
		return nil, err
	}
	var state string
	var root []byte
	var compacted bool
	var currentAttempt int
	if tx.QueryRowContext(ctx, `SELECT state,binding,compacted,attempt FROM ai_fixture_action WHERE namespace=$1 AND id=$2 AND owner=$3 FOR UPDATE`, h.cfg.Namespace, s.id, s.p.AccountID).Scan(&state, &root, &compacted, &currentAttempt) != nil {
		return nil, ErrDenied
	}
	if compacted || state != "recoverable" || currentAttempt != s.attempt || !hmac.Equal(root, s.root) {
		return nil, ErrDenied
	}
	if err = s.validateChild(ctx, tx, Leg{Coordinate: d.Coordinate}); err != nil {
		return nil, err
	}
	if err = s.checkExecution(ctx, tx); err != nil {
		return nil, err
	}
	var frozen []byte
	var dispatchState string
	if tx.QueryRowContext(ctx, `SELECT binding,state FROM ai_fixture_dispatch WHERE namespace=$1 AND id=$2 AND action=$3 FOR UPDATE`, h.cfg.Namespace, d.ID, s.id).Scan(&frozen, &dispatchState) != nil {
		return nil, ErrDenied
	}
	if !hmac.Equal(frozen, binding) {
		return nil, ErrConflict
	}
	if dispatchState != "reserved" {
		return nil, ErrOutputUnavailable
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.scope FROM ai_fixture_scope s JOIN ai_fixture_allocation a ON a.namespace=s.namespace AND a.scope=s.scope WHERE a.namespace=$1 AND a.dispatch=$2 ORDER BY s.scope FOR UPDATE OF s`, h.cfg.Namespace, d.ID)
	if err != nil {
		return nil, ErrUnavailable
	}
	for rows.Next() {
		var scope string
		if rows.Scan(&scope) != nil {
			rows.Close()
			return nil, ErrUnavailable
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, ErrUnavailable
	}
	if err = h.fault("before_claim"); err != nil {
		return nil, err
	}
	if err = s.checkCurrent(ctx, tx); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE ai_fixture_dispatch SET state='intent',category='unknown_maximum',remote_state='possibly_active',intent_at=clock_timestamp() WHERE namespace=$1 AND id=$2 AND state='reserved' AND EXISTS(SELECT 1 FROM ai_fixture_policy WHERE namespace=$1 AND expires_at>clock_timestamp()) AND EXISTS(SELECT 1 FROM careeros_sessions WHERE id=$3 AND expires_at>clock_timestamp()) AND EXISTS(SELECT 1 FROM ai_fixture_grant WHERE namespace=$1 AND owner=$4 AND version=$5 AND NOT revoked AND expires_at>clock_timestamp())`, h.cfg.Namespace, d.ID, s.p.SessionID, s.p.AccountID, s.grantVersion)
	if err != nil {
		return nil, ErrUnavailable
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return nil, ErrDenied
	}
	if err = h.fault("before_intent_commit"); err != nil {
		return nil, err
	}
	if tx.Commit() != nil {
		return nil, ErrUnavailable
	}
	if err = h.fault("after_intent_commit"); err != nil {
		return nil, ErrUnavailable
	} // never recreate a lost permit
	_, healthy := h.cfg.Clock.Sample()
	if !healthy {
		return nil, ErrUnavailable
	}
	var bound time.Time
	if h.db.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&bound) != nil {
		return nil, ErrUnavailable
	}
	// The witness is after known successful durability. A missing witness is
	// never reconstructed as an earlier time by a later lookup/restart.
	if _, err = h.db.ExecContext(ctx, `UPDATE ai_fixture_dispatch SET durable_bound=$3 WHERE namespace=$1 AND id=$2 AND durable_bound IS NULL`, h.cfg.Namespace, d.ID, bound); err != nil {
		return nil, ErrUnavailable
	}
	if err = h.fault("before_invoke"); err != nil {
		return nil, err
	}
	// A claimed intent is non-resendable, not a reusable authorization token.
	// A delayed local handoff must still have current account/session, grant,
	// policy, Profile and external epoch authority. Denial retains the intent.
	handoff, err := h.begin(ctx, s.p)
	if err != nil {
		return nil, err
	}
	defer handoff.Rollback()
	if err = s.checkCurrent(ctx, handoff); err != nil {
		return nil, err
	}
	if err = s.stageAction(ctx, handoff); err != nil {
		return nil, err
	}
	if err = handoff.Commit(); err != nil {
		return nil, ErrUnavailable
	}
	cloned := req.Clone(ctx)
	cloned.Header = http.Header{"Content-Type": []string{"application/json"}}
	cloned.Body = io.NopCloser(bytes.NewReader(raw))
	cloned.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
	resp, callErr := s.gate.invoke(ctx, func(callCtx context.Context) (*http.Response, error) {
		return respond(callCtx, d.Provider, cloned.WithContext(callCtx))
	}) // one fake gateway handoff; no network client
	_, _ = h.db.ExecContext(context.WithoutCancel(ctx), `UPDATE ai_fixture_dispatch SET state='observed',local_finished=true WHERE namespace=$1 AND id=$2 AND state='intent'`, h.cfg.Namespace, d.ID)
	return h.observeResponse(ctx, d, resp, callErr)
}

const dispatchColumns = `id,action,coordinate,stage,provider,state,category,remote_state,maximum,floor_amount,counted,expense,intent_at,durable_bound,local_finished,compacted,evidence_version,maximum_breached`

func scanDispatch(row interface{ Scan(...any) error }) (Dispatch, error) {
	var d Dispatch
	var e sql.NullInt64
	var at, bound sql.NullTime
	err := row.Scan(&d.ID, &d.Action, &d.Coordinate, &d.Stage, &d.Provider, &d.State, &d.Category, &d.Remote, &d.Maximum, &d.Floor, &d.Counted, &e, &at, &bound, &d.LocalFinished, &d.Compacted, &d.EvidenceVersion, &d.MaximumBreached)
	if err != nil {
		return d, ErrUnavailable
	}
	if e.Valid {
		d.Expense = &e.Int64
	}
	if at.Valid {
		d.IntentAt = &at.Time
	}
	if bound.Valid {
		d.DurableBound = &bound.Time
	}
	return d, nil
}
func loadDispatch(ctx context.Context, tx *sql.Tx, ns, id string) (Dispatch, error) {
	return scanDispatch(tx.QueryRowContext(ctx, `SELECT `+dispatchColumns+` FROM ai_fixture_dispatch WHERE namespace=$1 AND id=$2`, ns, id))
}
func (h *Harness) Status(ctx context.Context, p auth.Principal, id string) (ActionStatus, error) {
	var out ActionStatus
	tx, err := h.begin(ctx, p)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if tx.QueryRowContext(ctx, `SELECT id,state,customer_reserved,customer_effect FROM ai_fixture_action WHERE namespace=$1 AND id=$2 AND owner=$3`, h.cfg.Namespace, id, p.AccountID).Scan(&out.ID, &out.State, &out.CustomerReserved, &out.CustomerEffect) != nil {
		return out, ErrDenied
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+dispatchColumns+` FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 ORDER BY id`, h.cfg.Namespace, id)
	if err != nil {
		return out, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		d, e := scanDispatch(rows)
		if e != nil {
			return out, e
		}
		out.Dispatches = append(out.Dispatches, d)
	}
	if rows.Err() != nil {
		return out, ErrUnavailable
	}
	return out, safe(tx.Commit())
}

// Retry preserves the root binding and creates a fresh bounded attempt, only
// after authoritative known failure. It does not release any supplier hold.
func (s *Session) Retry(ctx context.Context) (*Session, error) {
	h := s.h
	tx, err := h.begin(ctx, s.p)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = s.checkCurrent(ctx, tx); err != nil {
		return nil, err
	}
	if _, err = h.checkGrant(ctx, tx, s.p, s.grantVersion); err != nil {
		return nil, err
	}
	var state string
	var current int
	if tx.QueryRowContext(ctx, `SELECT state,attempt FROM ai_fixture_action WHERE namespace=$1 AND id=$2 AND owner=$3 FOR UPDATE`, h.cfg.Namespace, s.id, s.p.AccountID).Scan(&state, &current) != nil {
		return nil, ErrDenied
	}
	if err = s.checkExecution(ctx, tx); err != nil {
		return nil, err
	}
	next := s.attempt + 1
	if current == s.attempt && state == "failed" {
		var maxAttempts int
		if tx.QueryRowContext(ctx, `SELECT max_attempts FROM ai_fixture_policy WHERE namespace=$1`, h.cfg.Namespace).Scan(&maxAttempts) != nil {
			return nil, ErrUnavailable
		}
		if next > maxAttempts {
			return nil, ErrCapacity
		}
		if _, err = tx.ExecContext(ctx, `UPDATE ai_fixture_action SET state='recoverable',attempt=$3 WHERE namespace=$1 AND id=$2 AND attempt=$4 AND state='failed'`, h.cfg.Namespace, s.id, next, s.attempt); err != nil {
			return nil, ErrUnavailable
		}
	} else if current != next || state != "recoverable" {
		return nil, ErrDenied
	}
	if err = accountstore.AuthorizeTransaction(ctx, tx, s.p); err != nil {
		return nil, err
	}
	if _, err = h.checkGrant(ctx, tx, s.p, s.grantVersion); err != nil {
		return nil, err
	}
	if err = s.gate.allowed(); err != nil {
		return nil, err
	}
	if err = s.checkCurrent(ctx, tx); err != nil {
		return nil, err
	}
	if tx.Commit() != nil {
		return nil, ErrUnavailable
	}
	cloned := s.childSession(s.childID, s.childParent)
	cloned.attempt = next
	return cloned, nil
}

func (s *Session) childSession(id, parent string) *Session {
	return &Session{h: s.h, p: s.p, id: s.id, workflow: s.workflow, root: append([]byte(nil), s.root...), rootInput: append([]byte(nil), s.rootInput...), profile: append([]byte(nil), s.profile...), execution: append([]byte(nil), s.execution...), attempt: s.attempt, grantVersion: s.grantVersion, gate: s.gate, childID: id, childParent: parent}
}

func (s *Session) checkCurrent(ctx context.Context, tx *sql.Tx) error {
	if e := accountstore.AuthorizeTransaction(ctx, tx, s.p); e != nil {
		return e
	}
	if e := s.h.ready(ctx, tx); e != nil {
		return e
	}
	if _, e := s.h.checkGrant(ctx, tx, s.p, s.grantVersion); e != nil {
		return e
	}
	if e := s.gate.allowed(); e != nil {
		return e
	}
	var valid bool
	if tx.QueryRowContext(ctx, `SELECT expires_at>clock_timestamp() FROM ai_fixture_policy WHERE namespace=$1`, s.h.cfg.Namespace).Scan(&valid) != nil {
		return ErrUnavailable
	}
	if !valid {
		return ErrDenied
	}
	if s.profile != nil {
		b, e := s.h.currentProfileBinding(ctx, tx, s.p.AccountID)
		if e != nil {
			return e
		}
		if !hmac.Equal(b, s.profile) {
			return ErrConflict
		}
	}
	return nil
}

// Abandon serializes against the only possible intent claimant. It can release
// only a reservation that remains unclaimed, and makes later invocation impossible.
func (s *Session) Abandon(ctx context.Context, id string) error {
	return s.h.mutateDispatch(ctx, s.p, id, func(tx *sql.Tx, d Dispatch) error {
		if d.Action != s.id {
			return ErrDenied
		}
		if d.State == "aborted" {
			return nil
		}
		if d.State != "reserved" {
			return ErrDenied
		}
		_, e := tx.ExecContext(ctx, `SELECT ai_fixture_abandon($1,$2)`, s.h.cfg.Namespace, id)
		return safe(e)
	})
}

func (h *Harness) mutateDispatch(ctx context.Context, p auth.Principal, id string, fn func(*sql.Tx, Dispatch) error) error {
	tx, err := h.begin(ctx, p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var action string
	if tx.QueryRowContext(ctx, `SELECT action FROM ai_fixture_dispatch WHERE namespace=$1 AND id=$2`, h.cfg.Namespace, id).Scan(&action) != nil {
		return ErrDenied
	}
	var owner string
	if tx.QueryRowContext(ctx, `SELECT owner FROM ai_fixture_action WHERE namespace=$1 AND id=$2 FOR UPDATE`, h.cfg.Namespace, action).Scan(&owner) != nil || owner != p.AccountID {
		return ErrDenied
	}
	if _, err = tx.ExecContext(ctx, `SELECT id FROM ai_fixture_dispatch WHERE namespace=$1 AND id=$2 FOR UPDATE`, h.cfg.Namespace, id); err != nil {
		return ErrUnavailable
	}
	d, err := loadDispatch(ctx, tx, h.cfg.Namespace, id)
	if err != nil {
		return err
	}
	if err = fn(tx, d); err != nil {
		return err
	}
	if err = accountstore.AuthorizeTransaction(ctx, tx, p); err != nil {
		return err
	}
	return safe(tx.Commit())
}
func (h *Harness) adjustScopes(ctx context.Context, tx *sql.Tx, id string, delta int64, slots int) error {
	rows, e := tx.QueryContext(ctx, `SELECT s.scope FROM ai_fixture_scope s JOIN ai_fixture_allocation a ON a.namespace=s.namespace AND a.scope=s.scope WHERE a.namespace=$1 AND a.dispatch=$2 ORDER BY s.scope FOR UPDATE OF s`, h.cfg.Namespace, id)
	if e != nil {
		return ErrUnavailable
	}
	var scopes []string
	for rows.Next() {
		var scope string
		if rows.Scan(&scope) != nil {
			rows.Close()
			return ErrUnavailable
		}
		scopes = append(scopes, scope)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return ErrUnavailable
	}
	for _, scope := range scopes {
		if _, e = tx.ExecContext(ctx, `UPDATE ai_fixture_scope SET counted=counted+$3,occupied=occupied+$4 WHERE namespace=$1 AND scope=$2`, h.cfg.Namespace, scope, delta, slots); e != nil {
			return ErrUnavailable
		}
	}
	return nil
}

// ScopeStatus is restricted synthetic fixture inspection, not ordinary diagnostics.
type ScopeStatus struct {
	Limit, Counted     int64
	Capacity, Occupied int
}

func (h *Harness) ScopeStatus(ctx context.Context, p auth.Principal, scope string) (ScopeStatus, error) {
	var out ScopeStatus
	if scope != "global" && scope != "provider:openai" && scope != "provider:typesafe" && scope != "account:"+p.AccountID {
		return out, ErrDenied
	}
	tx, e := h.begin(ctx, p)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	e = tx.QueryRowContext(ctx, `SELECT limit_amount,counted,capacity,occupied FROM ai_fixture_scope WHERE namespace=$1 AND scope=$2`, h.cfg.Namespace, scope).Scan(&out.Limit, &out.Counted, &out.Capacity, &out.Occupied)
	if e != nil {
		return out, ErrUnavailable
	}
	return out, safe(tx.Commit())
}

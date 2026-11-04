package aicontrol

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"reflect"

	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/profiledocument"
)

// IngestionSyntheticConfig enables both approved local workflows in ONE
// namespace. Existing qualification rows, scope counters and execution bindings
// are reused unchanged. No production composition selects this capability.
func IngestionSyntheticConfig(namespace string, gate *SyntheticGate, bound func(string, []byte) (int64, bool)) SyntheticConfig {
	c := QualificationSyntheticConfig(namespace, gate, bound)
	c.Ingestion = true
	return c
}
func (h *Harness) IngestionEnabled() bool { return h != nil && h.cfg.Qualification && h.cfg.Ingestion }
func (s *Session) ingestion() bool {
	return s.h.IngestionEnabled() && s.workflow == "profile_ingestion"
}

// The account lock from begin covers this comparison and the saved binding
// frozen by Open. A preflight HTTP read alone could authorize a stale supplied
// Profile against a newer saved snapshot. Later stage/send checks retain that
// exact frozen saved-document binding.
func (h *Harness) ingestionSavedSnapshot(ctx context.Context, tx *sql.Tx, owner string, root []byte) error {
	var saved []byte
	if tx.QueryRowContext(ctx, `SELECT document FROM careeros_profiles WHERE account_id=$1`, owner).Scan(&saved) != nil {
		return ErrUnavailable
	}
	doc, e := profiledocument.Decode(saved)
	if e != nil {
		return ErrDenied
	}
	var request struct {
		Profile  map[string]any  `json:"profile"`
		Document json.RawMessage `json:"document"`
	}
	if json.Unmarshal(root, &request) != nil || request.Profile == nil {
		return ErrDenied
	}
	// The legacy compatibility shape omits only these optional empty defaults.
	for _, name := range []string{"fullName", "email", "phone", "location", "professionalLinks"} {
		if _, ok := request.Profile[name]; !ok {
			request.Profile[name] = ""
		}
	}
	for _, name := range []string{"education", "certifications", "languages"} {
		if _, ok := request.Profile[name]; !ok {
			request.Profile[name] = []any{}
		}
	}
	if !reflect.DeepEqual(request.Profile, profiledocument.View(doc)) {
		return ErrConflict
	}
	if len(request.Document) > 0 && string(request.Document) != "null" {
		var a, b any
		if json.Unmarshal(saved, &a) != nil || json.Unmarshal(request.Document, &b) != nil || !reflect.DeepEqual(a, b) {
			return ErrConflict
		}
	}
	return nil
}

func (h *Harness) ingestionMaximum(provider string, raw []byte) (int64, int64, error) {
	if h.cfg.TokenBound == nil {
		return 0, 0, ErrDenied
	}
	n, ok := h.cfg.TokenBound(provider, append([]byte(nil), raw...))
	if !ok {
		return 0, 0, ErrDenied
	}
	output := int64(0)
	if provider == "openai" {
		var v struct {
			Model     string `json:"model"`
			Tier      string `json:"service_tier"`
			Reasoning string `json:"reasoning_effort"`
			Output    int64  `json:"max_completion_tokens"`
		}
		if json.Unmarshal(raw, &v) != nil || v.Model != "gpt-6-luna" || v.Tier != "standard" || v.Reasoning != "none" || v.Output != selectedIngestionEnvelope().OpenAIOutput {
			return 0, 0, ErrDenied
		}
		output = v.Output
	}
	m, e := IngestionReservation(provider, n, output)
	if m <= 0 && e == nil {
		m = 1
	}
	return m, n, e
}

func (s *Session) ingestionLimits(ctx context.Context, tx *sql.Tx, leg Leg, input, output int64) error {
	var ts, oa int
	var ti, oi, out int64
	err := tx.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE provider='typesafe'),count(*) FILTER(WHERE provider='openai'),COALESCE(sum(input_tokens) FILTER(WHERE provider='typesafe'),0),COALESCE(sum(input_tokens) FILTER(WHERE provider='openai'),0),COALESCE(sum(output_tokens) FILTER(WHERE provider='openai'),0) FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND attempt=$3`, s.h.cfg.Namespace, s.id, s.attempt).Scan(&ts, &oa, &ti, &oi, &out)
	if err != nil {
		return ErrUnavailable
	}
	p := selectedIngestionEnvelope()
	if leg.Provider == "typesafe" {
		if ts >= p.TypeSafeCalls || ti+input > p.TypeSafeTotal {
			return ErrCapacity
		}
	} else {
		if oa >= p.OpenAICalls || oi+input > p.OpenAITotal || out+output > p.OutputTotal {
			return ErrCapacity
		}
	}
	return nil
}

type IngestionStage struct {
	Session      *Session
	State        string
	RetryAllowed bool
}

// BeginIngestionStage binds one explicit HTTP protocol operation before any
// provider handoff. Career content is fingerprinted only, never retained here.
// Changed answers receive new bounded clarification identities; changed regular
// stages conflict. Unknown/completed dispatches return status, never resends.
func (s *Session) BeginIngestionStage(ctx context.Context, coordinate, kind string, input []byte) (IngestionStage, error) {
	out := IngestionStage{Session: s, State: "ready"}
	if !s.ingestion() || !coordinateLabel.MatchString(coordinate) {
		return out, ErrDenied
	}
	if kind != "plan" && kind != "extract" && kind != "assemble" && kind != "reconcile" && kind != "clarify" {
		return out, ErrDenied
	}
	current := *s
	current.foregroundIndependent = kind == "clarify"
	binding, err := s.h.fingerprint(s.p.AccountID, "ingestion-request/"+coordinate, input)
	if err != nil {
		return out, err
	}
	h := s.h
	tx, err := h.begin(ctx, s.p)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = current.checkCurrent(ctx, tx); err != nil {
		return out, err
	}
	var state string
	if tx.QueryRowContext(ctx, `SELECT state FROM ai_fixture_action WHERE namespace=$1 AND id=$2 FOR UPDATE`, h.cfg.Namespace, s.id).Scan(&state) != nil {
		return out, ErrUnavailable
	}
	if state == "failed" {
		out.State = "known_failure"
		out.RetryAllowed = s.attempt < 2
		return out, safe(tx.Commit())
	}
	if state != "recoverable" {
		return out, ErrDenied
	}
	if err = s.checkExecution(ctx, tx); err != nil {
		return out, err
	}
	if kind == "clarify" {
		// The digest is keyed; neither selected claim nor answer content becomes
		// an ordinary label or a free-form ledger field.
		revision := s.ingestionRevision(binding)
		var count int
		if tx.QueryRowContext(ctx, `SELECT count(*) FROM ai_fixture_child WHERE namespace=$1 AND action=$2 AND attempt=$3`, h.cfg.Namespace, s.id, s.attempt).Scan(&count) != nil {
			return out, ErrUnavailable
		}
		var old []byte
		e := tx.QueryRowContext(ctx, `SELECT binding FROM ai_fixture_child WHERE namespace=$1 AND action=$2 AND id=$3 AND attempt=$4`, h.cfg.Namespace, s.id, revision, s.attempt).Scan(&old)
		if e == sql.ErrNoRows {
			if count >= selectedIngestionEnvelope().Clarify {
				return out, ErrCapacity
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_child(namespace,action,id,parent_coordinate,parent_revision,attempt,binding) VALUES($1,$2,$3,$4,$5,$6,$7)`, h.cfg.Namespace, s.id, revision, coordinate, revision, s.attempt, binding); err != nil {
				return out, ErrUnavailable
			}
		} else if e != nil || !hmac.Equal(old, binding) {
			return out, ErrConflict
		}
		out.Session = s.childSession(revision, coordinate)
		coordinate += "/revision/" + revision
	}
	var old []byte
	err = tx.QueryRowContext(ctx, `SELECT binding FROM ai_fixture_stage WHERE namespace=$1 AND action=$2 AND coordinate=$3 AND stage=$4 AND attempt=$5`, h.cfg.Namespace, s.id, coordinate, "managed_request_"+kind, s.attempt).Scan(&old)
	if err == nil && !hmac.Equal(old, binding) {
		return out, ErrConflict
	}
	if err == sql.ErrNoRows {
		var count int
		if tx.QueryRowContext(ctx, `SELECT count(*) FROM ai_fixture_stage WHERE namespace=$1 AND action=$2 AND attempt=$3 AND stage=$4`, h.cfg.Namespace, s.id, s.attempt, "managed_request_"+kind).Scan(&count) != nil {
			return out, ErrUnavailable
		}
		limit := 1
		switch kind {
		case "extract":
			limit = 20
		case "reconcile":
			limit = 20
		case "clarify":
			limit = 4
		}
		if count >= limit {
			return out, ErrCapacity
		}
		if kind != "clarify" {
			var valid bool
			if tx.QueryRowContext(ctx, `SELECT COALESCE(min(created_at),clock_timestamp())+interval '300 seconds'>clock_timestamp() FROM ai_fixture_stage WHERE namespace=$1 AND action=$2 AND attempt=$3 AND stage LIKE 'managed_request_%' AND stage<>'managed_request_clarify'`, h.cfg.Namespace, s.id, s.attempt).Scan(&valid) != nil {
				return out, ErrUnavailable
			}
			if !valid {
				return out, ErrCapacity
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_stage(namespace,action,coordinate,stage,attempt,binding) VALUES($1,$2,$3,$4,$5,$6)`, h.cfg.Namespace, s.id, coordinate, "managed_request_"+kind, s.attempt, binding); err != nil {
			return out, ErrUnavailable
		}
	} else if err != nil {
		return out, ErrUnavailable
	}
	var reached, unknown int
	if tx.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE remote_state<>'terminal_proven' OR state<>'observed') FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND attempt=$3 AND (coordinate=$4 OR coordinate=$4||'/original' OR coordinate=$4||'/answer')`, h.cfg.Namespace, s.id, s.attempt, coordinate).Scan(&reached, &unknown) != nil {
		return out, ErrUnavailable
	}
	if reached > 0 {
		out.State = "result_unavailable"
		if unknown > 0 {
			out.State = "unknown"
		} else if kind == "clarify" {
			var failed bool
			if tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_fixture_stage WHERE namespace=$1 AND action=$2 AND coordinate=$3 AND attempt=$4 AND stage='known_failed_clarification')`, h.cfg.Namespace, s.id, coordinate, s.attempt).Scan(&failed) != nil {
				return out, ErrUnavailable
			}
			if failed {
				out.State = "known_failure"
				out.RetryAllowed = s.attempt < 2
			}
		}
		return out, safe(tx.Commit())
	}
	if err = h.managedOpen(ctx, tx, s.p.AccountID, s.workflow, s.id, false); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE ai_fixture_action SET execution_finished=false WHERE namespace=$1 AND id=$2`, h.cfg.Namespace, s.id); err != nil {
		return out, ErrUnavailable
	}
	if err = current.checkCurrent(ctx, tx); err != nil {
		return out, err
	}
	return out, safe(tx.Commit())
}

// The first persisted protocol request fixes this attempt's absolute foreground
// deadline. Neither stage replay, idle time nor status extends it. Explicit
// clarification is separately bounded by its existing handler/client deadline;
// only a deliberately authorized new attempt has a new first-request witness.
func (s *Session) ingestionDeadline(ctx context.Context, tx *sql.Tx) error {
	var valid bool
	if tx.QueryRowContext(ctx, `SELECT COALESCE(min(created_at),clock_timestamp())+interval '300 seconds'>clock_timestamp() FROM ai_fixture_stage WHERE namespace=$1 AND action=$2 AND attempt=$3 AND stage LIKE 'managed_request_%' AND stage<>'managed_request_clarify'`, s.h.cfg.Namespace, s.id, s.attempt).Scan(&valid) != nil {
		return ErrUnavailable
	}
	if !valid {
		return ErrCapacity
	}
	return nil
}

func (s *Session) FinishIngestionStage(ctx context.Context, failed bool) error {
	if !s.ingestion() {
		return ErrDenied
	}
	if failed && s.childID != "" {
		return s.finishManaged(ctx, false, true)
	}
	return s.FinishQualification(ctx, failed)
}

// RetryIngestionStage promotes only an unchanged, authoritatively known-failed
// clarification to the Action's deliberate new financial attempt. A changed
// answer is a separate bounded child in the current attempt, never this command.
func (s *Session) RetryIngestionStage(ctx context.Context, key, coordinate, kind string, input []byte) (*Session, error) {
	if !s.ingestion() || !tokenLabel.MatchString(key) {
		return nil, ErrDenied
	}
	if kind != "clarify" {
		return s.RetryQualification(ctx, key)
	}
	binding, err := s.h.fingerprint(s.p.AccountID, "ingestion-request/"+coordinate, input)
	if err != nil {
		return nil, err
	}
	revision := s.ingestionRevision(binding)
	h := s.h.SyntheticReviewer().h
	tx, err := h.begin(ctx, s.p)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var state, retry string
	var attempt int
	if tx.QueryRowContext(ctx, `SELECT state,attempt,retry_creation_key FROM ai_fixture_action WHERE namespace=$1 AND id=$2 AND owner=$3 FOR UPDATE`, h.cfg.Namespace, s.id, s.p.AccountID).Scan(&state, &attempt, &retry) != nil {
		return nil, ErrDenied
	}
	if retry != "" {
		if retry != key || attempt != 2 {
			return nil, ErrConflict
		}
		if tx.Commit() != nil {
			return nil, ErrUnavailable
		}
		return s.RetryQualification(ctx, key)
	}
	var failed bool
	var unknown int
	if tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_fixture_stage WHERE namespace=$1 AND action=$2 AND coordinate=$3 AND stage='known_failed_clarification' AND attempt=$4 AND binding=$5)`, h.cfg.Namespace, s.id, coordinate+"/revision/"+revision, attempt, binding).Scan(&failed) != nil {
		return nil, ErrUnavailable
	}
	if tx.QueryRowContext(ctx, `SELECT count(*) FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND attempt=$3 AND (state<>'observed' OR remote_state<>'terminal_proven')`, h.cfg.Namespace, s.id, attempt).Scan(&unknown) != nil {
		return nil, ErrUnavailable
	}
	if !failed || unknown > 0 || attempt != 1 || state != "recoverable" {
		return nil, ErrDenied
	}
	if _, err = tx.ExecContext(ctx, `UPDATE ai_fixture_action SET state='failed' WHERE namespace=$1 AND id=$2`, h.cfg.Namespace, s.id); err != nil {
		return nil, ErrUnavailable
	}
	if tx.Commit() != nil {
		return nil, ErrUnavailable
	}
	return s.RetryQualification(ctx, key)
}

func (s *Session) ingestionRevision(binding []byte) string {
	bytes := append(append([]byte(nil), binding...), byte(s.attempt))
	digest := sha256.Sum256(bytes)
	return hex.EncodeToString(digest[:16])
}

func (h *Harness) IngestionStatus(ctx context.Context, p auth.Principal, key string) (QualificationStatus, error) {
	if !h.IngestionEnabled() {
		return QualificationStatus{}, ErrDenied
	}
	out, e := h.QualificationStatus(ctx, p, key)
	if e != nil {
		return out, e
	}
	var workflow string
	if h.db.QueryRowContext(ctx, `SELECT workflow FROM ai_fixture_action WHERE namespace=$1 AND owner=$2 AND creation_key=$3`, h.cfg.Namespace, p.AccountID, key).Scan(&workflow) != nil || workflow != "profile_ingestion" {
		return QualificationStatus{}, ErrDenied
	}
	var failed bool
	if h.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_fixture_stage f WHERE f.namespace=$1 AND f.action=$2 AND f.attempt=$3 AND f.stage='known_failed_clarification' AND f.coordinate=(SELECT coordinate FROM ai_fixture_stage r WHERE r.namespace=f.namespace AND r.action=f.action AND r.attempt=f.attempt AND r.stage LIKE 'managed_request_%' ORDER BY created_at DESC LIMIT 1))`, h.cfg.Namespace, out.ActionID, out.Attempt).Scan(&failed) != nil {
		return QualificationStatus{}, ErrUnavailable
	}
	if failed && out.State != "unknown" {
		out.State = "known_failure"
		out.RetryAllowed = out.Attempt < 2
	}
	return out, nil
}

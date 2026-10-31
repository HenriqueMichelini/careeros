package aicontrol

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"professional-information-repo/internal/aidispatch"
)

func (s *Session) stageAction(ctx context.Context, tx *sql.Tx) error {
	var state string
	var binding []byte
	var compacted bool
	var attempt int
	err := tx.QueryRowContext(ctx, `SELECT state,binding,compacted,attempt FROM ai_fixture_action WHERE namespace=$1 AND id=$2 AND owner=$3 FOR UPDATE`, s.h.cfg.Namespace, s.id, s.p.AccountID).Scan(&state, &binding, &compacted, &attempt)
	if err != nil || state != "recoverable" || compacted || attempt != s.attempt {
		return ErrDenied
	}
	if !hmac.Equal(binding, s.root) {
		return ErrConflict
	}
	return s.checkExecution(ctx, tx)
}

func (s *Session) stageLimit(ctx context.Context, tx *sql.Tx) (int, error) {
	var maximum int
	if tx.QueryRowContext(ctx, `SELECT max_stages FROM ai_fixture_policy WHERE namespace=$1 FOR UPDATE`, s.h.cfg.Namespace).Scan(&maximum) != nil {
		return 0, ErrUnavailable
	}
	var count int
	if tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM ai_fixture_stage WHERE namespace=$1 AND action=$2)+(SELECT count(*) FROM ai_fixture_child WHERE namespace=$1 AND action=$2)`, s.h.cfg.Namespace, s.id).Scan(&count) != nil {
		return 0, ErrUnavailable
	}
	if count >= maximum {
		return 0, ErrCapacity
	}
	return maximum, nil
}

func (s *Session) stageCurrent(ctx context.Context, tx *sql.Tx) error {
	return s.checkCurrent(ctx, tx)
}

// BindStage allocates and fingerprints a provider-free validated protocol stage.
// It stores only replay metadata. Stable receipt identity/time lets the handler
// reconstruct the same signed handoff without storing career content.
func (s *Session) BindStage(ctx context.Context, coordinate string, input []byte) (aidispatch.StageReceipt, error) {
	var out aidispatch.StageReceipt
	if s.workflow != "profile_ingestion" || (coordinate != "ingest/v2/plan" && coordinate != "ingest/v2/assemble") {
		return out, ErrDenied
	}
	workflow, root := aidispatch.WorkflowRoot(ctx)
	rootBinding, err := s.h.fingerprint(s.p.AccountID, "root/"+workflow, root)
	if err != nil || workflow != s.workflow || !hmac.Equal(rootBinding, s.root) {
		return out, ErrDenied
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(input, &envelope) != nil {
		return out, ErrDenied
	}
	var stage struct {
		Version int    `json:"version"`
		Kind    string `json:"kind"`
	}
	if json.Unmarshal(envelope["stage"], &stage) != nil || stage.Version != 2 || coordinate != "ingest/v2/"+stage.Kind {
		return out, ErrDenied
	}
	inputRoot := map[string]json.RawMessage{"input": envelope["input"], "profile": envelope["profile"]}
	if document := envelope["document"]; len(document) > 0 && string(document) != "null" {
		inputRoot["document"] = document
	}
	encodedRoot, err := json.Marshal(inputRoot)
	if err != nil {
		return out, ErrDenied
	}
	boundInputRoot, err := s.h.fingerprint(s.p.AccountID, "root/"+workflow, encodedRoot)
	if err != nil || !hmac.Equal(boundInputRoot, s.root) {
		return out, errors.Join(ErrConflict, aidispatch.ErrBindingConflict)
	}
	binding, err := s.h.fingerprint(s.p.AccountID, "provider-free-stage/"+coordinate, input)
	if err != nil {
		return out, err
	}
	tx, err := s.h.begin(ctx, s.p)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err = s.h.checkGrant(ctx, tx, s.p, s.grantVersion); err != nil {
		return out, err
	}
	if err = s.stageAction(ctx, tx); err != nil {
		return out, err
	}
	var old []byte
	err = tx.QueryRowContext(ctx, `SELECT binding,receipt_id,created_at FROM ai_fixture_stage WHERE namespace=$1 AND action=$2 AND coordinate=$3 AND stage=$4 AND attempt=$5`, s.h.cfg.Namespace, s.id, coordinate, "provider_free_"+stage.Kind, s.attempt).Scan(&old, &out.ID, &out.CreatedAt)
	if err == nil {
		if !hmac.Equal(binding, old) {
			return out, errors.Join(ErrConflict, aidispatch.ErrBindingConflict)
		}
		if err = s.stageCurrent(ctx, tx); err != nil {
			return out, err
		}
		return out, safe(tx.Commit())
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, ErrUnavailable
	}
	if _, err = s.stageLimit(ctx, tx); err != nil {
		return out, err
	}
	if err = s.stageCurrent(ctx, tx); err != nil {
		return out, err
	}
	out.ID = opaque()
	if out.ID == "" {
		return out, ErrUnavailable
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO ai_fixture_stage(namespace,action,coordinate,stage,attempt,binding,receipt_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,clock_timestamp()) RETURNING created_at`, s.h.cfg.Namespace, s.id, coordinate, "provider_free_"+stage.Kind, s.attempt, binding, out.ID).Scan(&out.CreatedAt)
	if err != nil {
		return out, ErrUnavailable
	}
	return out, safe(tx.Commit())
}

// NewClarification is an explicit no-send synthetic command, not an HTTP option
// or a new customer right. A server child follows an observed parent
// reconciliation dispatch; observed does not establish semantic acceptance.
// The handler still validates signed handoffs and source scope on execution.
// Replaying creation returns that child rather than allocating another revision.
func (s *Session) NewClarification(ctx context.Context, parentCoordinate, parentRevision string) (*Session, error) {
	if s.workflow != "profile_ingestion" || !coordinateLabel.MatchString(parentCoordinate) || strings.Contains(parentCoordinate, "/revision/") || !(strings.HasPrefix(parentCoordinate, "ingest/v2/clarify/claim/") || strings.Contains(parentCoordinate, "/clarification/")) || (parentRevision != "" && !tokenLabel.MatchString(parentRevision)) {
		return nil, ErrDenied
	}
	tx, err := s.h.begin(ctx, s.p)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = s.h.checkGrant(ctx, tx, s.p, s.grantVersion); err != nil {
		return nil, err
	}
	if err = s.stageAction(ctx, tx); err != nil {
		return nil, err
	}
	parent := parentCoordinate
	if parentRevision != "" {
		var registered string
		if tx.QueryRowContext(ctx, `SELECT parent_coordinate FROM ai_fixture_child WHERE namespace=$1 AND action=$2 AND id=$3 AND attempt=$4`, s.h.cfg.Namespace, s.id, parentRevision, s.attempt).Scan(&registered) != nil || registered != parentCoordinate {
			return nil, ErrDenied
		}
		parent += "/revision/" + parentRevision
	}
	var completed bool
	if tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND attempt=$3 AND coordinate=$4 AND stage='reconciliation' AND state='observed')`, s.h.cfg.Namespace, s.id, s.attempt, parent).Scan(&completed) != nil || !completed {
		return nil, ErrDenied
	}
	encoded, _ := json.Marshal(struct{ Parent, Revision string }{parentCoordinate, parentRevision})
	binding, err := s.h.fingerprint(s.p.AccountID, "clarification-child/"+s.id, encoded)
	if err != nil {
		return nil, err
	}
	var id string
	var old []byte
	err = tx.QueryRowContext(ctx, `SELECT id,binding FROM ai_fixture_child WHERE namespace=$1 AND action=$2 AND parent_coordinate=$3 AND parent_revision=$4 AND attempt=$5`, s.h.cfg.Namespace, s.id, parentCoordinate, parentRevision, s.attempt).Scan(&id, &old)
	if err == nil {
		if !hmac.Equal(binding, old) {
			return nil, ErrConflict
		}
	} else if errors.Is(err, sql.ErrNoRows) {
		if _, err = s.stageLimit(ctx, tx); err != nil {
			return nil, err
		}
		id = opaque()
		if id == "" {
			return nil, ErrUnavailable
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_child(namespace,action,id,parent_coordinate,parent_revision,attempt,binding) VALUES($1,$2,$3,$4,$5,$6,$7)`, s.h.cfg.Namespace, s.id, id, parentCoordinate, parentRevision, s.attempt, binding)
		if err != nil {
			return nil, ErrUnavailable
		}
	} else {
		return nil, ErrUnavailable
	}
	if err = s.stageCurrent(ctx, tx); err != nil {
		return nil, err
	}
	if tx.Commit() != nil {
		return nil, ErrUnavailable
	}
	return s.childSession(id, parentCoordinate), nil
}

func (s *Session) ClarificationRevision() string { return s.childID }

func (s *Session) validateChild(ctx context.Context, tx *sql.Tx, leg Leg) error {
	if s.childID == "" {
		if strings.Contains(leg.Coordinate, "/revision/") {
			return ErrDenied
		}
		return nil
	}
	base := s.childParent + "/revision/" + s.childID
	if leg.Coordinate != base && leg.Coordinate != base+"/original" && leg.Coordinate != base+"/answer" {
		return ErrDenied
	}
	var parent string
	if tx.QueryRowContext(ctx, `SELECT parent_coordinate FROM ai_fixture_child WHERE namespace=$1 AND action=$2 AND id=$3 AND attempt=$4`, s.h.cfg.Namespace, s.id, s.childID, s.attempt).Scan(&parent) != nil || parent != s.childParent {
		return ErrDenied
	}
	return nil
}

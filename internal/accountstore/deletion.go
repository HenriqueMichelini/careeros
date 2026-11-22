package accountstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"professional-information-repo/internal/auth"
)

type DeletionRequest struct {
	OperationID string `json:"operationId"`
	Kind        string `json:"kind"`
	ProfileID   string `json:"profileId"`
	Revision    int64  `json:"revision"`
	Confirm     bool   `json:"confirm"`
}

// DeletionStatus deliberately contains no career content, subject or credential.
type DeletionStatus struct {
	OperationID          string     `json:"operationId"`
	Kind                 string     `json:"kind"`
	State                string     `json:"state"`
	FencedAt             time.Time  `json:"fencedAt"`
	RemovedAt            *time.Time `json:"removedAt,omitempty"`
	RecoveryCopies       string     `json:"recoveryCopies"`
	RecoveryCopyDeadline *time.Time `json:"recoveryCopyDeadline,omitempty"`
	Recipients           string     `json:"recipients"`
	FailureCode          string     `json:"failureCode,omitempty"`
}

func deletionStatus(ctx context.Context, tx *sql.Tx, owner, operation string) (DeletionStatus, []byte, error) {
	var v DeletionStatus
	var hash []byte
	var removed sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT operation_id,kind,state,fenced_at,removed_at,failure_code,request_hash FROM careeros_deletion_operations WHERE account_id=$1 AND operation_id=$2`, owner, operation).Scan(&v.OperationID, &v.Kind, &v.State, &v.FencedAt, &removed, &v.FailureCode, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil, ErrNotFound
	}
	if err != nil {
		return v, nil, unavailable(err)
	}
	v.RecoveryCopies, v.Recipients = "not_verified", "not_verified"
	until := v.FencedAt.Add(30 * 24 * time.Hour)
	v.RecoveryCopyDeadline = &until
	if removed.Valid {
		v.RemovedAt = &removed.Time
	}
	return v, hash, nil
}

func (s *Store) deletionTransaction(ctx context.Context, p auth.Principal) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, unavailable(err)
	}
	// Same order as PilotCommand/Save, also when ordinary non-pilot mode is used.
	if err = lockPilot(ctx, tx); err == nil {
		err = authorized(ctx, tx, p)
	}
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func (s *Store) StartDeletion(ctx context.Context, p auth.Principal, in DeletionRequest) (DeletionStatus, error) {
	if !validOperation(in.OperationID) || !in.Confirm || (in.Kind != "profile" && in.Kind != "account") || len(in.ProfileID) > 1024 || in.Revision < 0 {
		return DeletionStatus{}, ErrValidation
	}
	if (in.Kind == "profile" && (in.ProfileID == "" || in.Revision == 0)) || (in.Kind == "account" && (in.ProfileID != "" || in.Revision != 0)) {
		return DeletionStatus{}, ErrValidation
	}
	raw, _ := json.Marshal(in)
	hash := sha256.Sum256(raw)
	tx, err := s.deletionTransaction(ctx, p)
	if err != nil {
		return DeletionStatus{}, err
	}
	defer tx.Rollback()
	prior, binding, err := deletionStatus(ctx, tx, p.AccountID, in.OperationID)
	if err == nil {
		if !bytes.Equal(binding, hash[:]) {
			return prior, ErrConflict
		}
		return prior, unavailable(tx.Commit())
	}
	if !errors.Is(err, ErrNotFound) {
		return prior, err
	}
	var id []byte
	var rev int64
	err = tx.QueryRowContext(ctx, `SELECT profile_id,revision FROM careeros_profiles WHERE account_id=$1 FOR UPDATE`, p.AccountID).Scan(&id, &rev)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return prior, unavailable(err)
	}
	if in.Kind == "profile" && (err != nil || string(id) != in.ProfileID || rev != in.Revision) {
		return prior, ErrConflict
	}
	if err = authorized(ctx, tx, p); err != nil {
		return prior, err
	}
	generation := p.Generation
	if in.Kind == "account" {
		if generation == ^uint64(0) {
			return prior, ErrConflict
		}
		generation++
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO careeros_deletion_operations(account_id,operation_id,request_hash,kind,profile_id,profile_revision,state,fenced_at,removed_at,failure_code,account_generation) VALUES($1,$2,$3,$4,$5,$6,'fenced_pending',clock_timestamp(),NULL,'',$7)`, p.AccountID, in.OperationID, hash[:], in.Kind, []byte(in.ProfileID), in.Revision, generation)
	if err != nil {
		return prior, unavailable(err)
	}
	if len(id) > 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO careeros_deleted_profiles VALUES($1,$2,$3,clock_timestamp()) ON CONFLICT DO NOTHING`, p.AccountID, id, in.OperationID)
		if err != nil {
			return prior, unavailable(err)
		}
	}
	var cutoff time.Time
	if err = tx.QueryRowContext(ctx, `SELECT fenced_at FROM careeros_deletion_operations WHERE account_id=$1 AND operation_id=$2`, p.AccountID, in.OperationID).Scan(&cutoff); err != nil {
		return prior, unavailable(err)
	}
	if err = fenceCareerContent(ctx, tx, p.AccountID, in.Kind, in.ProfileID, cutoff); err != nil {
		return prior, err
	}
	// Purpose/shared/allowance acquisition can block beyond session expiry.
	// Roll the entire tentative fence back unless current authority still wins.
	// Initial authorization already excluded older markers under this account
	// lock. Our own tentative marker must not reject its creating transaction.
	if err = authorizedSession(ctx, tx, p); err != nil {
		return prior, err
	}
	// The primary Profile row is small and removed in the fence transaction.
	// Dependent purpose content remains inaccessible until bounded cleanup.
	if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_profiles WHERE account_id=$1`, p.AccountID); err != nil {
		return prior, unavailable(err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE careeros_deletion_operations SET fenced_at=clock_timestamp() WHERE account_id=$1 AND operation_id=$2`, p.AccountID, in.OperationID); err != nil {
		return prior, unavailable(err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE careeros_deleted_profiles p SET fenced_at=o.fenced_at FROM careeros_deletion_operations o WHERE p.account_id=o.account_id AND p.operation_id=o.operation_id AND o.account_id=$1 AND o.operation_id=$2`, p.AccountID, in.OperationID); err != nil {
		return prior, unavailable(err)
	}
	if in.Kind == "account" {
		if _, err = tx.ExecContext(ctx, `UPDATE careeros_accounts SET status='deleted',generation=generation+1 WHERE id=$1;`, p.AccountID); err != nil {
			return prior, unavailable(err)
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_sessions WHERE account_id=$1`, p.AccountID); err != nil {
			return prior, unavailable(err)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE careeros_pilot_access SET state='revoked',invitation_revoked=true WHERE account_id=$1`, p.AccountID); err != nil {
			return prior, unavailable(err)
		}
	}
	out, _, err := deletionStatus(ctx, tx, p.AccountID, in.OperationID)
	if err != nil {
		return out, err
	}
	return out, unavailable(tx.Commit())
}

func installed(ctx context.Context, tx *sql.Tx, table string) (bool, error) {
	var ok bool
	err := tx.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&ok)
	return ok, unavailable(err)
}

func fenceCareerContent(ctx context.Context, tx *sql.Tx, owner, kind, profile string, cutoff time.Time) error {
	present, err := installed(ctx, tx, "careeros_draft_actions")
	if err != nil {
		return err
	}
	if present {
		// Lock purpose then shared Action, matching load/publication/opening order.
		rows, err := tx.QueryContext(ctx, `SELECT namespace,action_id FROM careeros_draft_actions WHERE owner=$1 AND ($2='account' OR profile_id=$3) ORDER BY namespace,action_id FOR UPDATE`, owner, kind, profile)
		if err != nil {
			return unavailable(err)
		}
		type ref struct{ namespace, id string }
		var refs []ref
		for rows.Next() {
			var r ref
			if err = rows.Scan(&r.namespace, &r.id); err != nil {
				break
			}
			refs = append(refs, r)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return unavailable(err)
		}
		for _, r := range refs {
			if _, err = tx.ExecContext(ctx, `SELECT id FROM ai_fixture_action WHERE namespace=$1 AND id=$2 FOR UPDATE`, r.namespace, r.id); err != nil {
				return unavailable(err)
			}
			if _, err = tx.ExecContext(ctx, `UPDATE careeros_draft_actions SET state='deleted',cleanup_queued=true WHERE namespace=$1 AND action_id=$2`, r.namespace, r.id); err != nil {
				return unavailable(err)
			}
			if _, err = tx.ExecContext(ctx, `UPDATE careeros_draft_allowance SET state='released',released_at=clock_timestamp() WHERE namespace=$1 AND action_id=$2 AND state='reserved'`, r.namespace, r.id); err != nil {
				return unavailable(err)
			}
			if _, err = tx.ExecContext(ctx, `UPDATE ai_fixture_action SET state='deleted',customer_reserved=false WHERE namespace=$1 AND id=$2`, r.namespace, r.id); err != nil {
				return unavailable(err)
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE careeros_draft_admissions SET state='deleted' WHERE owner=$1 AND ($2='account' OR profile_id=$3)`, owner, kind, profile); err != nil {
			return unavailable(err)
		}
	}
	present, err = installed(ctx, tx, "ai_fixture_action")
	if err != nil {
		return err
	}
	if present {
		// Transient workflows have no retained content. Their old Action identity is
		// terminal, while every dispatch, observation, allocation and hold survives.
		if _, err = tx.ExecContext(ctx, `UPDATE ai_fixture_action SET state='deleted' WHERE owner=$1 AND workflow<>'application_draft' AND created_at<=$2`, owner, cutoff); err != nil {
			return unavailable(err)
		}
	}
	return nil
}

func (s *Store) WithDeletionStatus(ctx context.Context, p auth.Principal, operation string, write func(DeletionStatus) error) error {
	if operation != "" && !validOperation(operation) {
		return ErrValidation
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = authorizeDeletionStatus(ctx, tx, p); err != nil {
		return err
	}
	if operation == "" {
		err = tx.QueryRowContext(ctx, `SELECT operation_id FROM careeros_deletion_operations WHERE account_id=$1 ORDER BY fenced_at DESC,operation_id DESC LIMIT 1`, p.AccountID).Scan(&operation)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return unavailable(err)
		}
	}
	out, _, err := deletionStatus(ctx, tx, p.AccountID, operation)
	if err != nil {
		return err
	}
	if err = authorizeDeletionStatus(ctx, tx, p); err != nil {
		return err
	}
	if err = write(out); err != nil {
		return err
	}
	return unavailable(tx.Commit())
}

// PurgeDeletion is the code-owned queue worker, not a public customer command.
// Its database capability is kept separate from restricted AI runtime roles.
// The operation must already have won its durable fence. No session/grant is
// restored to let cleanup run after closure.
func (s *Store) PurgeDeletion(ctx context.Context, owner, operation string) (DeletionStatus, error) {
	if !validOperation(operation) || owner == "" {
		return DeletionStatus{}, ErrValidation
	}
	out, err := s.purgeDeletion(ctx, owner, operation)
	if err != nil {
		// Persist only a bounded error code, never a driver message/content. The
		// original fence survives the failed removal transaction.
		_, _ = s.db.ExecContext(ctx, `UPDATE careeros_deletion_operations SET state='failed',failure_code='active_removal_failed' WHERE account_id=$1 AND operation_id=$2`, owner, operation)
	}
	return out, err
}

func (s *Store) purgeDeletion(ctx context.Context, owner, operation string) (DeletionStatus, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DeletionStatus{}, unavailable(err)
	}
	defer tx.Rollback()
	if err = lockPilot(ctx, tx); err != nil {
		return DeletionStatus{}, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE`, owner); err != nil {
		return DeletionStatus{}, unavailable(err)
	}
	out, _, err := deletionStatus(ctx, tx, owner, operation)
	if err != nil {
		return out, err
	}
	var profile []byte
	if err = tx.QueryRowContext(ctx, `SELECT profile_id FROM careeros_deletion_operations WHERE account_id=$1 AND operation_id=$2 FOR UPDATE`, owner, operation).Scan(&profile); err != nil {
		return out, unavailable(err)
	}
	present, err := installed(ctx, tx, "careeros_draft_content")
	if err != nil {
		return out, err
	}
	if present {
		// Review/decision rows cascade only from the affected purpose content.
		if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_draft_content c USING careeros_draft_actions a WHERE c.namespace=a.namespace AND c.action_id=a.action_id AND a.owner=$1 AND ($2='account' OR a.profile_id=$3) AND a.state='deleted'`, owner, out.Kind, string(profile)); err != nil {
			return out, unavailable(err)
		}
		var remaining bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_draft_content c JOIN careeros_draft_actions a ON c.namespace=a.namespace AND c.action_id=a.action_id WHERE a.owner=$1 AND ($2='account' OR a.profile_id=$3))`, owner, out.Kind, string(profile)).Scan(&remaining); err != nil {
			return out, unavailable(err)
		}
		if remaining {
			return out, auth.ErrUnavailable
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_profiles WHERE account_id=$1 AND ($2='account' OR profile_id=$3)`, owner, out.Kind, profile); err != nil {
		return out, unavailable(err)
	}
	if out.Kind == "account" {
		if _, err = tx.ExecContext(ctx, `UPDATE careeros_accounts SET status='deleted',email=''::bytea,display_name=''::bytea WHERE id=$1`, owner); err != nil {
			return out, unavailable(err)
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_oidc_transactions WHERE account_id=$1`, owner); err != nil {
			return out, unavailable(err)
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_sessions WHERE account_id=$1`, owner); err != nil {
			return out, unavailable(err)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE careeros_pilot_access SET state='revoked',invitation_revoked=true WHERE account_id=$1`, owner); err != nil {
			return out, unavailable(err)
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE careeros_deletion_operations SET state='active_content_removed',removed_at=COALESCE(removed_at,clock_timestamp()),failure_code='' WHERE account_id=$1 AND operation_id=$2`, owner, operation); err != nil {
		return out, unavailable(err)
	}
	out, _, err = deletionStatus(ctx, tx, owner, operation)
	if err != nil {
		return out, err
	}
	return out, unavailable(tx.Commit())
}

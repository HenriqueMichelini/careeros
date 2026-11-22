package accountstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"professional-information-repo/internal/auth"
	"time"
)

const lifecycleSnapshotVersion = "careeros-lifecycle-metadata-v1"

type LifecycleNoticeFence struct {
	AccountID      string    `json:"accountId"`
	NoticeID       string    `json:"noticeId"`
	Kind           string    `json:"kind"`
	DueAt          time.Time `json:"dueAt"`
	AccessEndedAt  time.Time `json:"accessEndedAt"`
	RemovalAt      time.Time `json:"removalAt"`
	EmailState     string    `json:"emailState"`
	Attempts       int       `json:"attempts"`
	NextAttemptAt  time.Time `json:"nextAttemptAt"`
	OperatorSignal bool      `json:"operatorSignal"`
}
type LifecycleCleanupFence struct {
	AccountID     string    `json:"accountId"`
	AccessEndedAt time.Time `json:"accessEndedAt"`
	ProfileID     string    `json:"profileId"`
	OperationID   string    `json:"operationId"`
}
type InvitationCleanupFence struct {
	AccountID            string     `json:"accountId"`
	InvitationID         string     `json:"invitationId"`
	ExpiresAt            time.Time  `json:"expiresAt"`
	FencedAt             time.Time  `json:"fencedAt"`
	CleanedAt            *time.Time `json:"cleanedAt,omitempty"`
	RecoveryCopyDeadline time.Time  `json:"recoveryCopyDeadline"`
}
type LifecycleSnapshot struct {
	Version     string                        `json:"version"`
	Notices     []LifecycleNoticeFence        `json:"notices"`
	Residuals   []LifecycleResidualObligation `json:"residuals"`
	Evidence    []ResidualEvidenceFence       `json:"evidence"`
	Cleanup     []LifecycleCleanupFence       `json:"cleanup"`
	Invitations []InvitationCleanupFence      `json:"invitations"`
	Seal        string                        `json:"seal"`
}

func (r *DeletionRecovery) lifecycleSeal(v LifecycleSnapshot) string {
	v.Seal = ""
	raw, _ := json.Marshal(v)
	m := hmac.New(sha256.New, r.key)
	m.Write([]byte(lifecycleSnapshotVersion + "\n"))
	m.Write(raw)
	return hex.EncodeToString(m.Sum(nil))
}

// ExportLifecycle is metadata-only. Restore callers must first reapply the
// independently surviving deletion authority, and keep service opening closed.
func (r *DeletionRecovery) ExportLifecycle(ctx context.Context) (LifecycleSnapshot, error) {
	v := LifecycleSnapshot{Version: lifecycleSnapshotVersion, Notices: []LifecycleNoticeFence{}, Residuals: []LifecycleResidualObligation{}, Evidence: []ResidualEvidenceFence{}, Cleanup: []LifecycleCleanupFence{}, Invitations: []InvitationCleanupFence{}}
	tx, err := r.store.db.BeginTx(ctx, nil)
	if err != nil {
		return v, unavailable(err)
	}
	defer tx.Rollback()
	if err = lockPilot(ctx, tx); err != nil {
		return v, err
	}
	read := func(query string, scan func(*sql.Rows) error) error {
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return unavailable(err)
		}
		defer rows.Close()
		for rows.Next() {
			if err = scan(rows); err != nil {
				return unavailable(err)
			}
		}
		return unavailable(rows.Err())
	}
	if err = read(`SELECT id FROM careeros_accounts WHERE EXISTS(SELECT 1 FROM careeros_lifecycle_notices WHERE account_id=id) OR EXISTS(SELECT 1 FROM careeros_lifecycle_residuals WHERE account_id=id) OR EXISTS(SELECT 1 FROM careeros_invitation_cleanup WHERE account_id=id) ORDER BY id FOR UPDATE`, func(rows *sql.Rows) error { var id string; return rows.Scan(&id) }); err != nil {
		return v, err
	}
	if err = read(`SELECT account_id,notice_id,kind,due_at,access_ended_at,removal_at,CASE WHEN email_state='sending' THEN 'unknown' ELSE email_state END,attempts,next_attempt_at,operator_signal OR email_state='sending' FROM careeros_lifecycle_notices ORDER BY account_id,notice_id LIMIT 10001`, func(rows *sql.Rows) error {
		var n LifecycleNoticeFence
		if err := rows.Scan(&n.AccountID, &n.NoticeID, &n.Kind, &n.DueAt, &n.AccessEndedAt, &n.RemovalAt, &n.EmailState, &n.Attempts, &n.NextAttemptAt, &n.OperatorSignal); err != nil {
			return err
		}
		v.Notices = append(v.Notices, n)
		return nil
	}); err != nil {
		return v, err
	}
	if err = read(`SELECT account_id,operation_id,class,original_fenced_at,deadline,state,revision,evidence_id FROM careeros_lifecycle_residuals ORDER BY account_id,operation_id,class LIMIT 10001`, func(rows *sql.Rows) error {
		var o LifecycleResidualObligation
		var deadline sql.NullTime
		if err := rows.Scan(&o.AccountID, &o.OperationID, &o.Class, &o.OriginalFencedAt, &deadline, &o.State, &o.Revision, &o.EvidenceID); err != nil {
			return err
		}
		if deadline.Valid {
			o.Deadline = &deadline.Time
		}
		v.Residuals = append(v.Residuals, o)
		return nil
	}); err != nil {
		return v, err
	}
	if err = read(`SELECT account_id,operation_id,class,evidence_id,revision,state,request_hash FROM careeros_lifecycle_residual_evidence ORDER BY account_id,operation_id,class,evidence_id LIMIT 10001`, func(rows *sql.Rows) error {
		var e ResidualEvidenceFence
		if err := rows.Scan(&e.AccountID, &e.OperationID, &e.Class, &e.Evidence.ID, &e.Evidence.Revision, &e.Evidence.State, &e.Binding); err != nil {
			return err
		}
		v.Evidence = append(v.Evidence, e)
		return nil
	}); err != nil {
		return v, err
	}
	if err = read(`SELECT account_id,access_ended_at,profile_id,operation_id FROM careeros_lifecycle_cleanup ORDER BY account_id,access_ended_at,profile_id LIMIT 10001`, func(rows *sql.Rows) error {
		var f LifecycleCleanupFence
		var id []byte
		if err := rows.Scan(&f.AccountID, &f.AccessEndedAt, &id, &f.OperationID); err != nil {
			return err
		}
		f.ProfileID = string(id)
		v.Cleanup = append(v.Cleanup, f)
		return nil
	}); err != nil {
		return v, err
	}
	if err = read(`SELECT account_id,invitation_id,expires_at,fenced_at,cleaned_at FROM careeros_invitation_cleanup ORDER BY account_id,invitation_id LIMIT 10001`, func(rows *sql.Rows) error {
		var f InvitationCleanupFence
		var cleaned sql.NullTime
		if err := rows.Scan(&f.AccountID, &f.InvitationID, &f.ExpiresAt, &f.FencedAt, &cleaned); err != nil {
			return err
		}
		if cleaned.Valid {
			f.CleanedAt = &cleaned.Time
		}
		f.RecoveryCopyDeadline = f.FencedAt.Add(30 * 24 * time.Hour)
		v.Invitations = append(v.Invitations, f)
		return nil
	}); err != nil {
		return v, err
	}
	if len(v.Notices)+len(v.Residuals)+len(v.Evidence)+len(v.Cleanup)+len(v.Invitations) > 10000 {
		return v, auth.ErrCapacity
	}
	v.Seal = r.lifecycleSeal(v)
	return v, unavailable(tx.Commit())
}

func validLifecycleSnapshot(v LifecycleSnapshot) bool {
	if v.Version != lifecycleSnapshotVersion || len(v.Notices)+len(v.Residuals)+len(v.Evidence)+len(v.Cleanup)+len(v.Invitations) > 10000 {
		return false
	}
	seen := map[string]bool{}
	unique := func(key string) bool {
		if seen[key] {
			return false
		}
		seen[key] = true
		return true
	}
	for _, n := range v.Notices {
		if !validOperation(n.AccountID) || n.AccessEndedAt.IsZero() || !n.RemovalAt.Equal(n.AccessEndedAt.Add(30*24*time.Hour)) || (n.Kind != "access_ended" && n.Kind != "removal_soon") || n.NoticeID != lifecycleKey(n.AccountID, n.Kind, n.AccessEndedAt) || !unique("notice:"+n.AccountID+":"+n.NoticeID) || n.Attempts < 0 || n.Attempts > 3 {
			return false
		}
		due := n.AccessEndedAt
		if n.Kind == "removal_soon" {
			due = n.RemovalAt.Add(-7 * 24 * time.Hour)
		}
		if !n.DueAt.Equal(due) || n.NextAttemptAt.Before(due) || n.NextAttemptAt.After(due.Add(24*time.Hour)) {
			return false
		}
		switch n.EmailState {
		case "pending":
			if n.Attempts != 0 {
				return false
			}
		case "sent", "failed", "unknown", "unavailable":
			if n.Attempts < 1 {
				return false
			}
		default:
			return false
		}
	}
	residuals := map[string]LifecycleResidualObligation{}
	for _, o := range v.Residuals {
		key := o.AccountID + ":" + o.OperationID + ":" + o.Class
		if !validOperation(o.AccountID) || !validOperation(o.OperationID) || !unique("residual:"+key) || o.OriginalFencedAt.IsZero() || o.Revision < 1 || o.Revision > 9007199254740991 {
			return false
		}
		if o.Class == "recovery_copy" {
			if o.Deadline == nil || !o.Deadline.Equal(o.OriginalFencedAt.Add(30*24*time.Hour)) {
				return false
			}
		} else if o.Class != "recipient" || o.Deadline != nil {
			return false
		}
		if o.State == "not_verified" {
			if o.EvidenceID != "" {
				return false
			}
		} else if !validResidualEvidence(LifecycleResidualEvidence{o.EvidenceID, o.Revision, o.State}) {
			return false
		}
		residuals[key] = o
	}
	latest := map[string]bool{}
	for _, e := range v.Evidence {
		key := e.AccountID + ":" + e.OperationID + ":" + e.Class
		o, exists := residuals[key]
		if !exists || !validResidualEvidence(e.Evidence) || !unique("evidence:"+key+":"+e.Evidence.ID) || !bytes.Equal(e.Binding, residualEvidenceBinding(e.AccountID, e.OperationID, e.Class, e.Evidence)) {
			return false
		}
		if o.EvidenceID == e.Evidence.ID && o.Revision == e.Evidence.Revision && o.State == e.Evidence.State {
			latest[key] = true
		}
	}
	for key, o := range residuals {
		if o.EvidenceID != "" && !latest[key] {
			return false
		}
	}
	for _, f := range v.Cleanup {
		if !validOperation(f.AccountID) || !validOperation(f.OperationID) || f.ProfileID == "" || len(f.ProfileID) > 1024 || f.AccessEndedAt.IsZero() || !unique("cleanup:"+f.AccountID+":"+f.OperationID) {
			return false
		}
	}
	for _, f := range v.Invitations {
		if !validOperation(f.AccountID) || !validOperation(f.InvitationID) || f.ExpiresAt.IsZero() || f.FencedAt.IsZero() || f.FencedAt.After(f.ExpiresAt) || (f.CleanedAt != nil && f.CleanedAt.Before(f.FencedAt)) || !f.RecoveryCopyDeadline.Equal(f.FencedAt.Add(30*24*time.Hour)) || !unique("invitation:"+f.AccountID+":"+f.InvitationID) {
			return false
		}
	}
	return true
}

// ApplyLifecycle preserves original clocks and one-use notice intent. It has
// no recipient callback, financial operation, content reader or service-open step.
func (r *DeletionRecovery) ApplyLifecycle(ctx context.Context, v LifecycleSnapshot) error {
	if !hmac.Equal([]byte(v.Seal), []byte(r.lifecycleSeal(v))) || !validLifecycleSnapshot(v) {
		return ErrValidation
	}
	tx, err := r.store.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if err = lockPilot(ctx, tx); err != nil {
		return err
	}
	owners := map[string]bool{}
	for _, n := range v.Notices {
		owners[n.AccountID] = true
	}
	for _, o := range v.Residuals {
		owners[o.AccountID] = true
	}
	for _, f := range v.Cleanup {
		owners[f.AccountID] = true
	}
	for _, f := range v.Invitations {
		if err = recordInvitationCleanup(ctx, tx, f.AccountID, f.InvitationID, f.ExpiresAt, f.FencedAt, f.CleanedAt); err != nil {
			return err
		}
		owners[f.AccountID] = true
	}
	var ids []string
	for id := range owners {
		ids = append(ids, id)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM careeros_accounts WHERE id=ANY($1) ORDER BY id FOR UPDATE`, ids)
	if err != nil {
		return unavailable(err)
	}
	count := 0
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		count++
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return unavailable(err)
	}
	if count != len(ids) {
		return ErrConflict
	}
	for _, n := range v.Notices {
		result, err := tx.ExecContext(ctx, `INSERT INTO careeros_lifecycle_notices VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(account_id,notice_id) DO UPDATE SET email_state=CASE WHEN careeros_lifecycle_notices.email_state='sent' OR EXCLUDED.email_state='sent' THEN 'sent' WHEN careeros_lifecycle_notices.email_state IN ('sending','unknown') OR EXCLUDED.email_state='unknown' THEN 'unknown' WHEN careeros_lifecycle_notices.email_state='unavailable' OR EXCLUDED.email_state='unavailable' THEN 'unavailable' WHEN careeros_lifecycle_notices.email_state='failed' OR EXCLUDED.email_state='failed' THEN 'failed' ELSE 'pending' END,attempts=GREATEST(careeros_lifecycle_notices.attempts,EXCLUDED.attempts),next_attempt_at=GREATEST(careeros_lifecycle_notices.next_attempt_at,EXCLUDED.next_attempt_at),operator_signal=careeros_lifecycle_notices.operator_signal OR EXCLUDED.operator_signal WHERE careeros_lifecycle_notices.kind=EXCLUDED.kind AND careeros_lifecycle_notices.due_at=EXCLUDED.due_at AND careeros_lifecycle_notices.access_ended_at=EXCLUDED.access_ended_at AND careeros_lifecycle_notices.removal_at=EXCLUDED.removal_at`, n.AccountID, n.NoticeID, n.Kind, n.DueAt, n.AccessEndedAt, n.RemovalAt, n.EmailState, n.Attempts, n.NextAttemptAt, n.OperatorSignal)
		if err != nil {
			return unavailable(err)
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return ErrConflict
		}
	}
	for _, o := range v.Residuals {
		var same bool
		if err = tx.QueryRowContext(ctx, `SELECT fenced_at=$3 FROM careeros_deletion_operations WHERE account_id=$1 AND operation_id=$2 FOR UPDATE`, o.AccountID, o.OperationID, o.OriginalFencedAt).Scan(&same); err != nil {
			return unavailable(err)
		}
		if !same {
			return ErrConflict
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO careeros_lifecycle_residuals VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(account_id,operation_id,class) DO UPDATE SET state=CASE WHEN EXCLUDED.revision>careeros_lifecycle_residuals.revision OR careeros_lifecycle_residuals.evidence_id='' THEN EXCLUDED.state ELSE careeros_lifecycle_residuals.state END,revision=GREATEST(careeros_lifecycle_residuals.revision,EXCLUDED.revision),evidence_id=CASE WHEN EXCLUDED.revision>careeros_lifecycle_residuals.revision OR careeros_lifecycle_residuals.evidence_id='' THEN EXCLUDED.evidence_id ELSE careeros_lifecycle_residuals.evidence_id END WHERE careeros_lifecycle_residuals.original_fenced_at=EXCLUDED.original_fenced_at AND careeros_lifecycle_residuals.deadline IS NOT DISTINCT FROM EXCLUDED.deadline AND (careeros_lifecycle_residuals.revision<>EXCLUDED.revision OR careeros_lifecycle_residuals.evidence_id='' OR EXCLUDED.evidence_id='' OR (careeros_lifecycle_residuals.state=EXCLUDED.state AND careeros_lifecycle_residuals.evidence_id=EXCLUDED.evidence_id))`, o.AccountID, o.OperationID, o.Class, o.OriginalFencedAt, o.Deadline, o.State, o.Revision, o.EvidenceID)
		if err != nil {
			return unavailable(err)
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return ErrConflict
		}
	}
	for _, e := range v.Evidence {
		if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_lifecycle_residual_evidence VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, e.AccountID, e.OperationID, e.Class, e.Evidence.ID, e.Evidence.Revision, e.Evidence.State, e.Binding); err != nil {
			return unavailable(err)
		}
		var same bool
		if err = tx.QueryRowContext(ctx, `SELECT request_hash=$5 FROM careeros_lifecycle_residual_evidence WHERE account_id=$1 AND operation_id=$2 AND class=$3 AND evidence_id=$4`, e.AccountID, e.OperationID, e.Class, e.Evidence.ID, e.Binding).Scan(&same); err != nil {
			return unavailable(err)
		}
		if !same {
			return ErrConflict
		}
	}
	for _, f := range v.Cleanup {
		var same bool
		if err = tx.QueryRowContext(ctx, `SELECT kind='profile' AND profile_id=$3 FROM careeros_deletion_operations WHERE account_id=$1 AND operation_id=$2`, f.AccountID, f.OperationID, []byte(f.ProfileID)).Scan(&same); err != nil {
			return unavailable(err)
		}
		if !same {
			return ErrConflict
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO careeros_lifecycle_cleanup VALUES($1,$2,$3,$4) ON CONFLICT(account_id,access_ended_at,profile_id) DO UPDATE SET operation_id=EXCLUDED.operation_id WHERE careeros_lifecycle_cleanup.operation_id=EXCLUDED.operation_id`, f.AccountID, f.AccessEndedAt, []byte(f.ProfileID), f.OperationID)
		if err != nil {
			return unavailable(err)
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return ErrConflict
		}
	}
	for _, f := range v.Invitations {
		if _, err = tx.ExecContext(ctx, `UPDATE careeros_pilot_access SET invitation_revoked=true,invitation_expires_at=LEAST(invitation_expires_at,$3),invitation_cleaned_at=CASE WHEN invitation_cleaned_at IS NULL THEN $4 ELSE LEAST(invitation_cleaned_at,$4) END,provider=NULL,issuer=NULL,subject=NULL,age_eligible=NULL,eligibility_checked_at=NULL,eligibility_checked_by=NULL,state=CASE WHEN activated_at IS NULL THEN state ELSE 'revoked' END,retrieval_held=retrieval_held OR activated_at IS NOT NULL WHERE account_id=$1 AND invitation_id=$2`, f.AccountID, f.InvitationID, f.ExpiresAt, f.CleanedAt); err != nil {
			return unavailable(err)
		}
	}
	return unavailable(tx.Commit())
}

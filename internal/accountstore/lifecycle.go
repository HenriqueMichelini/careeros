package accountstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/profiledocument"
)

type LifecycleNotice struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	DueAt          time.Time `json:"dueAt"`
	RemovalAt      time.Time `json:"removalAt"`
	EmailState     string    `json:"emailState"`
	Attempts       int       `json:"attempts"`
	OperatorSignal bool      `json:"operatorSignal"`
}
type LifecycleStatus struct {
	Notices           []LifecycleNotice           `json:"notices"`
	Cleanup           []DeletionStatus            `json:"cleanup"`
	Residuals         []LifecycleResidual         `json:"residuals"`
	PurposeCleanup    int                         `json:"purposeCleanup"`
	NextCheckAt       *time.Time                  `json:"nextCheckAt,omitempty"`
	RemovalPending    bool                        `json:"removalPending"`
	ContentExpired    bool                        `json:"contentExpired"`
	CareerReadAllowed bool                        `json:"careerReadAllowed"`
	InvitationCleanup []LifecycleInvitationStatus `json:"invitationCleanup"`
}
type LifecycleInvitationStatus struct {
	InvitationID         string     `json:"invitationId"`
	State                string     `json:"state"`
	FencedAt             time.Time  `json:"fencedAt"`
	CleanedAt            *time.Time `json:"cleanedAt,omitempty"`
	RecoveryCopyDeadline time.Time  `json:"recoveryCopyDeadline"`
	RecoveryCopies       string     `json:"recoveryCopies"`
}
type LifecycleResidual struct {
	OperationID string     `json:"operationId"`
	Class       string     `json:"class"`
	State       string     `json:"state"`
	Deadline    *time.Time `json:"deadline,omitempty"`
	Overdue     bool       `json:"overdue"`
}

// LifecycleEmailNotice is passed only to an explicitly injected recipient
// adapter. The recipient is read at handoff, never copied into a job record.
type LifecycleEmailNotice struct {
	Key       string
	Recipient string
	Kind      string
	RemovalAt time.Time
}
type LifecycleEmailResult struct{ State string }
type LifecycleEmail func(context.Context, LifecycleEmailNotice) (LifecycleEmailResult, error)

// LifecycleWorker is a code-owned capability, not an HTTP mutation or scheduler.
// No environment loader constructs a recipient transport.
type LifecycleWorker struct {
	store    *Store
	email    LifecycleEmail
	residual SyntheticResidualObserver
}
type LifecycleBatch struct {
	Processed int `json:"processed"`
}

func NewLifecycleWorker(store *Store, email LifecycleEmail, observers ...SyntheticResidualObserver) (*LifecycleWorker, error) {
	if store == nil || len(observers) > 1 {
		return nil, ErrValidation
	}
	w := &LifecycleWorker{store: store, email: email}
	if len(observers) == 1 && observers[0] != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var synthetic bool
		if err := store.db.QueryRowContext(ctx, `SELECT current_database() LIKE '%\_test' ESCAPE '\'`).Scan(&synthetic); err != nil {
			return nil, unavailable(err)
		}
		if !synthetic {
			return nil, auth.ErrForbidden
		}
		w.residual = observers[0]
	}
	return w, nil
}

func (w *LifecycleWorker) Run(ctx context.Context, limit int) (LifecycleBatch, error) {
	var out LifecycleBatch
	if limit < 1 || limit > 100 {
		return out, ErrValidation
	}
	rows, err := w.store.db.QueryContext(ctx, `SELECT p.account_id FROM careeros_pilot_access p LEFT JOIN careeros_lifecycle_scan s USING(account_id) ORDER BY s.checked_at NULLS FIRST,p.account_id LIMIT $1`, limit)
	if err != nil {
		return out, unavailable(err)
	}
	var owners []string
	for rows.Next() {
		var owner string
		if err = rows.Scan(&owner); err != nil {
			break
		}
		owners = append(owners, owner)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, unavailable(err)
	}
	for _, owner := range owners {
		operations, err := w.scan(ctx, owner)
		if err != nil {
			return out, err
		}
		for _, operation := range operations {
			if _, err = w.store.PurgeDeletion(ctx, owner, operation); err != nil {
				return out, err
			}
		}
		if err = w.deliver(ctx, owner); err != nil {
			return out, err
		}
		if err = w.observeResiduals(ctx, owner); err != nil {
			return out, err
		}
		out.Processed++
	}
	return out, nil
}

func lifecycleKey(owner, kind string, end time.Time) string {
	h := sha256.Sum256([]byte("lifecycle-v1\n" + owner + "\n" + kind + "\n" + end.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(h[:])
}
func lifecycleWindow(a pilotAccess) (time.Time, time.Time, bool) {
	if !a.activated.Valid || !a.expires.Valid || a.state == "revoked" && !a.accessEnded.Valid {
		return time.Time{}, time.Time{}, false
	}
	end := a.expires.Time
	if a.accessEnded.Valid && a.accessEnded.Time.Before(end) {
		end = a.accessEnded.Time
	}
	return end, end.Add(30 * 24 * time.Hour), true
}

func (w *LifecycleWorker) scan(ctx context.Context, owner string) ([]string, error) {
	tx, err := w.store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, unavailable(err)
	}
	defer tx.Rollback()
	if err = lockPilot(ctx, tx); err != nil {
		return nil, err
	}
	var generation uint64
	if err = tx.QueryRowContext(ctx, `SELECT generation FROM careeros_accounts WHERE id=$1 FOR UPDATE`, owner).Scan(&generation); err != nil {
		return nil, unavailable(err)
	}
	a, err := readPilot(ctx, tx, owner)
	if err != nil {
		return nil, unavailable(err)
	}
	now, err := pilotNow(ctx, tx)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_lifecycle_scan VALUES($1,$2) ON CONFLICT(account_id) DO UPDATE SET checked_at=EXCLUDED.checked_at`, owner, now); err != nil {
		return nil, unavailable(err)
	}
	if err = cleanupUndelivered(ctx, tx, owner, a); err != nil {
		return nil, err
	}
	now, err = pilotNow(ctx, tx)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_lifecycle_residuals(account_id,operation_id,class,original_fenced_at,deadline) SELECT account_id,operation_id,'recovery_copy',fenced_at,fenced_at+interval '720 hours' FROM careeros_deletion_operations WHERE account_id=$1 UNION ALL SELECT account_id,operation_id,'recipient',fenced_at,NULL FROM careeros_deletion_operations WHERE account_id=$1 ON CONFLICT DO NOTHING`, owner); err != nil {
		return nil, unavailable(err)
	}
	if !a.activated.Valid {
		if a.invitation.Valid && (a.revoked || a.invitationExpiry.Valid && !now.Before(a.invitationExpiry.Time)) {
			if a.invitationExpiry.Valid {
				if err = recordInvitationCleanup(ctx, tx, owner, a.invitation.String, a.invitationExpiry.Time, now, &now); err != nil {
					return nil, err
				}
			}
			_, err = tx.ExecContext(ctx, `UPDATE careeros_pilot_access SET invitation_revoked=true,provider=NULL,issuer=NULL,subject=NULL,age_eligible=NULL,eligibility_checked_at=NULL,eligibility_checked_by=NULL,invitation_cleaned_at=COALESCE(invitation_cleaned_at,$2) WHERE account_id=$1 AND activated_at IS NULL`, owner, now)
			if err != nil {
				return nil, unavailable(err)
			}
		}
		if err = cleanupOrphanEvidence(ctx, tx, owner); err != nil {
			return nil, err
		}
		return nil, unavailable(tx.Commit())
	}
	if a.state == "revoked" && !a.accessEnded.Valid {
		return nil, unavailable(tx.Commit())
	}
	end, removal, _ := lifecycleWindow(a)
	for _, notice := range []struct {
		kind string
		due  time.Time
	}{{"access_ended", end}, {"removal_soon", removal.Add(-7 * 24 * time.Hour)}} {
		if !now.Before(notice.due) {
			_, err = tx.ExecContext(ctx, `INSERT INTO careeros_lifecycle_notices(account_id,notice_id,kind,due_at,access_ended_at,removal_at,email_state,next_attempt_at) VALUES($1,$2,$3,$4,$5,$6,'pending',$4) ON CONFLICT DO NOTHING`, owner, lifecycleKey(owner, notice.kind, end), notice.kind, notice.due, end, removal)
			if err != nil {
				return nil, unavailable(err)
			}
		}
	}
	var operations []string
	if !now.Before(removal) {
		operations, err = w.expireProfiles(ctx, tx, owner, generation, end)
		if err != nil {
			return nil, err
		}
	} else if err = cleanupOrphanEvidence(ctx, tx, owner); err != nil {
		return nil, err
	}
	return operations, unavailable(tx.Commit())
}

func cleanupOrphanEvidence(ctx context.Context, tx *sql.Tx, owner string) error {
	var raw []byte
	var revision int64
	err := tx.QueryRowContext(ctx, `SELECT document,revision FROM careeros_profiles WHERE account_id=$1 AND NOT careeros_deleted_profile_restored($1) FOR UPDATE`, owner).Scan(&raw, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return unavailable(err)
	}
	next, changed, err := profiledocument.CleanupRetainedEvidence(raw)
	if err != nil {
		return auth.ErrUnavailable
	}
	if !changed {
		return nil
	}
	if err = recordProfilePruning(ctx, tx, owner, raw, next); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE careeros_profiles SET document=$2,revision=revision+1 WHERE account_id=$1 AND revision=$3`, owner, next, revision)
	if err != nil {
		return unavailable(err)
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	return nil
}

func (w *LifecycleWorker) expireProfiles(ctx context.Context, tx *sql.Tx, owner string, generation uint64, end time.Time) ([]string, error) {
	// Both canonical and purpose-bound sources are inventoried. Missing current
	// Profile bytes cannot hide an older retained Draft from expiry cleanup.
	query := `SELECT profile_id,revision FROM careeros_profiles WHERE account_id=$1`
	present, err := installed(ctx, tx, "careeros_draft_actions")
	if err != nil {
		return nil, err
	}
	if present {
		query += ` UNION SELECT convert_to(a.profile_id,'UTF8'),a.profile_revision FROM careeros_draft_actions a JOIN careeros_draft_content c USING(namespace,action_id) WHERE a.owner=$1`
	}
	query = `SELECT profile_id,max(revision) FROM (` + query + `) sources GROUP BY profile_id ORDER BY profile_id LIMIT 16`
	rows, err := tx.QueryContext(ctx, query, owner)
	if err != nil {
		return nil, unavailable(err)
	}
	type source struct {
		id  []byte
		rev int64
	}
	var sources []source
	for rows.Next() {
		var v source
		if err = rows.Scan(&v.id, &v.rev); err != nil {
			break
		}
		sources = append(sources, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, unavailable(err)
	}
	var operations []string
	for _, src := range sources {
		var operation string
		err = tx.QueryRowContext(ctx, `SELECT operation_id FROM careeros_lifecycle_cleanup WHERE account_id=$1 AND access_ended_at=$2 AND profile_id=$3`, owner, end, src.id).Scan(&operation)
		if errors.Is(err, sql.ErrNoRows) {
			operation, err = newID()
			if err != nil {
				return nil, err
			}
			hash := sha256.Sum256([]byte("lifecycle-expiry-v1\n" + owner + "\n" + string(src.id) + "\n" + end.UTC().Format(time.RFC3339Nano)))
			_, err = tx.ExecContext(ctx, `INSERT INTO careeros_deletion_operations(account_id,operation_id,request_hash,kind,profile_id,profile_revision,state,fenced_at,account_generation) VALUES($1,$2,$3,'profile',$4,$5,'fenced_pending',clock_timestamp(),$6)`, owner, operation, hash[:], src.id, src.rev, generation)
			if err == nil {
				_, err = tx.ExecContext(ctx, `INSERT INTO careeros_lifecycle_cleanup VALUES($1,$2,$3,$4)`, owner, end, src.id, operation)
			}
			if err != nil {
				return nil, unavailable(err)
			}
		} else if err != nil {
			return nil, unavailable(err)
		}
		var cutoff time.Time
		if err = tx.QueryRowContext(ctx, `SELECT fenced_at FROM careeros_deletion_operations WHERE account_id=$1 AND operation_id=$2`, owner, operation).Scan(&cutoff); err != nil {
			return nil, unavailable(err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_deleted_profiles VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, owner, src.id, operation, cutoff); err != nil {
			return nil, unavailable(err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_lifecycle_residuals(account_id,operation_id,class,original_fenced_at,deadline) VALUES($1,$2,'recovery_copy',$3::timestamptz,$3::timestamptz+interval '720 hours'),($1,$2,'recipient',$3::timestamptz,NULL) ON CONFLICT DO NOTHING`, owner, operation, cutoff); err != nil {
			return nil, unavailable(err)
		}
		if err = fenceCareerContent(ctx, tx, owner, "profile", string(src.id), cutoff); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_profiles WHERE account_id=$1 AND profile_id=$2`, owner, src.id); err != nil {
			return nil, unavailable(err)
		}
		operations = append(operations, operation)
	}
	rows, err = tx.QueryContext(ctx, `SELECT c.operation_id FROM careeros_lifecycle_cleanup c JOIN careeros_deletion_operations o USING(account_id,operation_id) WHERE c.account_id=$1 AND o.state<>'active_content_removed' ORDER BY c.operation_id LIMIT 16`, owner)
	if err != nil {
		return nil, unavailable(err)
	}
	for rows.Next() {
		var operation string
		if err = rows.Scan(&operation); err != nil {
			break
		}
		found := false
		for _, old := range operations {
			if old == operation {
				found = true
			}
		}
		if !found {
			operations = append(operations, operation)
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, unavailable(err)
	}
	return operations, nil
}

func (w *LifecycleWorker) deliver(ctx context.Context, owner string) error {
	for i := 0; i < 2; i++ {
		tx, err := w.store.db.BeginTx(ctx, nil)
		if err != nil {
			return unavailable(err)
		}
		if err = lockPilot(ctx, tx); err == nil {
			_, err = tx.ExecContext(ctx, `SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE`, owner)
		}
		if err != nil {
			tx.Rollback()
			return unavailable(err)
		}
		var n LifecycleEmailNotice
		var recipient []byte
		err = tx.QueryRowContext(ctx, `SELECT n.notice_id,n.kind,n.removal_at,a.email FROM careeros_lifecycle_notices n JOIN careeros_accounts a ON a.id=n.account_id WHERE n.account_id=$1 AND n.email_state IN ('pending','failed') AND n.attempts<3 AND n.next_attempt_at<=clock_timestamp() ORDER BY n.due_at,n.notice_id LIMIT 1 FOR UPDATE OF n`, owner).Scan(&n.Key, &n.Kind, &n.RemovalAt, &recipient)
		if errors.Is(err, sql.ErrNoRows) {
			tx.Rollback()
			return nil
		}
		if err != nil {
			tx.Rollback()
			return unavailable(err)
		}
		n.Recipient = string(recipient)
		state := "sending"
		if w.email == nil || len(recipient) == 0 {
			state = "unavailable"
		}
		_, err = tx.ExecContext(ctx, `UPDATE careeros_lifecycle_notices SET email_state=$3,attempts=attempts+1,operator_signal=$4 WHERE account_id=$1 AND notice_id=$2`, owner, n.Key, state, state == "unavailable")
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
		if err != nil {
			return unavailable(err)
		}
		if state == "unavailable" {
			continue
		}
		result, callErr := w.email(ctx, n)
		state = result.State
		if callErr != nil || (state != "sent" && state != "failed") {
			state = "unknown"
		}
		_, err = w.store.db.ExecContext(context.WithoutCancel(ctx), `UPDATE careeros_lifecycle_notices SET email_state=$3,operator_signal=($3<>'sent'),next_attempt_at=due_at+CASE WHEN attempts=1 THEN interval '1 hour' ELSE interval '24 hours' END WHERE account_id=$1 AND notice_id=$2 AND email_state='sending'`, owner, n.Key, state)
		if err != nil {
			return unavailable(err)
		}
	}
	return nil
}

func (s *Store) WithLifecycleStatus(ctx context.Context, p auth.Principal, write func(LifecycleStatus) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockPilot(ctx, tx); err != nil {
		return err
	}
	if err = authorizedAccount(ctx, tx, p); err != nil {
		return err
	}
	out := LifecycleStatus{Notices: []LifecycleNotice{}, Cleanup: []DeletionStatus{}, Residuals: []LifecycleResidual{}, InvitationCleanup: []LifecycleInvitationStatus{}}
	rows, err := tx.QueryContext(ctx, `SELECT notice_id,kind,due_at,removal_at,CASE WHEN email_state='sending' THEN 'unknown' ELSE email_state END,attempts,operator_signal OR email_state='sending' FROM careeros_lifecycle_notices WHERE account_id=$1 ORDER BY due_at,notice_id LIMIT 32`, p.AccountID)
	if err != nil {
		return unavailable(err)
	}
	for rows.Next() {
		var n LifecycleNotice
		if err = rows.Scan(&n.ID, &n.Kind, &n.DueAt, &n.RemovalAt, &n.EmailState, &n.Attempts, &n.OperatorSignal); err != nil {
			break
		}
		out.Notices = append(out.Notices, n)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return unavailable(err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT operation_id FROM careeros_lifecycle_cleanup WHERE account_id=$1 ORDER BY access_ended_at,profile_id LIMIT 32`, p.AccountID)
	if err != nil {
		return unavailable(err)
	}
	var operations []string
	for rows.Next() {
		var operation string
		if err = rows.Scan(&operation); err != nil {
			break
		}
		operations = append(operations, operation)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return unavailable(err)
	}
	for _, operation := range operations {
		status, _, err := deletionStatus(ctx, tx, p.AccountID, operation)
		if err != nil {
			return err
		}
		out.Cleanup = append(out.Cleanup, status)
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM careeros_lifecycle_purpose_cleanup WHERE account_id=$1`, p.AccountID).Scan(&out.PurposeCleanup); err != nil {
		return unavailable(err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT operation_id,class,state,deadline,deadline IS NOT NULL AND clock_timestamp()>=deadline AND state<>'verified_unrecoverable' FROM careeros_lifecycle_residuals WHERE account_id=$1 ORDER BY operation_id,class LIMIT 64`, p.AccountID)
	if err != nil {
		return unavailable(err)
	}
	for rows.Next() {
		var r LifecycleResidual
		var deadline sql.NullTime
		if err = rows.Scan(&r.OperationID, &r.Class, &r.State, &deadline, &r.Overdue); err != nil {
			break
		}
		if deadline.Valid {
			r.Deadline = &deadline.Time
		}
		out.Residuals = append(out.Residuals, r)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return unavailable(err)
	}
	a, err := readPilot(ctx, tx, p.AccountID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return unavailable(err)
	}
	now, err := pilotNow(ctx, tx)
	if err != nil {
		return err
	}
	var restored bool
	if err = tx.QueryRowContext(ctx, `SELECT careeros_deleted_profile_restored($1)`, p.AccountID).Scan(&restored); err != nil {
		return unavailable(err)
	}
	out.CareerReadAllowed = pilotStatus(p.AccountID, a, now).CanRetrieve && !restored
	rows, err = tx.QueryContext(ctx, `SELECT invitation_id,fenced_at,cleaned_at FROM careeros_invitation_cleanup WHERE account_id=$1 ORDER BY fenced_at,invitation_id LIMIT 32`, p.AccountID)
	if err != nil {
		return unavailable(err)
	}
	for rows.Next() {
		var v LifecycleInvitationStatus
		var cleaned sql.NullTime
		if err = rows.Scan(&v.InvitationID, &v.FencedAt, &cleaned); err != nil {
			break
		}
		v.State = "pending"
		v.RecoveryCopies = "not_verified"
		v.RecoveryCopyDeadline = v.FencedAt.Add(30 * 24 * time.Hour)
		if cleaned.Valid {
			v.State = "active_payload_removed"
			v.CleanedAt = &cleaned.Time
		}
		out.InvitationCleanup = append(out.InvitationCleanup, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return unavailable(err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT profile_id,evidence_id,recovery_copy_deadline FROM careeros_profile_pruning WHERE account_id=$1 ORDER BY removed_at,profile_id,evidence_id LIMIT $2`, p.AccountID, 64-len(out.Residuals))
	if err != nil {
		return unavailable(err)
	}
	for rows.Next() {
		var profile []byte
		var evidence string
		var deadline time.Time
		if err = rows.Scan(&profile, &evidence, &deadline); err != nil {
			break
		}
		hash := sha256.Sum256([]byte(p.AccountID + "\n" + string(profile) + "\n" + evidence))
		out.Residuals = append(out.Residuals, LifecycleResidual{OperationID: "pruning-" + hex.EncodeToString(hash[:]), Class: "recovery_copy", State: "not_verified", Deadline: &deadline, Overdue: !now.Before(deadline)})
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return unavailable(err)
	}
	if end, removal, known := lifecycleWindow(a); known {
		out.ContentExpired = !now.Before(removal)
		seen := map[string]bool{}
		for _, n := range out.Notices {
			seen[n.ID] = true
		}
		for _, n := range []struct {
			kind string
			due  time.Time
		}{{"access_ended", end}, {"removal_soon", removal.Add(-7 * 24 * time.Hour)}} {
			if now.Before(n.due) {
				if out.NextCheckAt == nil || n.due.Before(*out.NextCheckAt) {
					due := n.due
					out.NextCheckAt = &due
				}
				continue
			}
			id := lifecycleKey(p.AccountID, n.kind, end)
			if !seen[id] {
				out.Notices = append(out.Notices, LifecycleNotice{ID: id, Kind: n.kind, DueAt: n.due, RemovalAt: removal, EmailState: "pending"})
			}
		}
		if now.Before(removal) {
			if out.NextCheckAt == nil || removal.Before(*out.NextCheckAt) {
				out.NextCheckAt = &removal
			}
		} else {
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_profiles WHERE account_id=$1)`, p.AccountID).Scan(&out.RemovalPending); err != nil {
				return unavailable(err)
			}
			present, e := installed(ctx, tx, "careeros_draft_content")
			if e != nil {
				return e
			}
			if present {
				var purpose bool
				if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_draft_content c JOIN careeros_draft_actions a USING(namespace,action_id) WHERE a.owner=$1)`, p.AccountID).Scan(&purpose); err != nil {
					return unavailable(err)
				}
				out.RemovalPending = out.RemovalPending || purpose
			}
		}
	}
	if err = authorizedAccount(ctx, tx, p); err != nil {
		return err
	}
	if err = write(out); err != nil {
		return err
	}
	return unavailable(tx.Commit())
}

func cleanupUndelivered(ctx context.Context, tx *sql.Tx, owner string, a pilotAccess) error {
	present, err := installed(ctx, tx, "careeros_draft_actions")
	if err != nil || !present {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT d.namespace,d.action_id FROM careeros_draft_actions d JOIN ai_fixture_action a ON a.namespace=d.namespace AND a.id=d.action_id WHERE d.owner=$1 AND NOT a.customer_effect AND EXISTS(SELECT 1 FROM careeros_draft_content c WHERE c.namespace=d.namespace AND c.action_id=d.action_id) ORDER BY d.namespace,d.action_id LIMIT 16 FOR UPDATE OF d`, owner)
	if err != nil {
		return unavailable(err)
	}
	type ref struct{ ns, id string }
	var refs []ref
	for rows.Next() {
		var r ref
		if err = rows.Scan(&r.ns, &r.id); err != nil {
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
		var state string
		var delivered bool
		if err = tx.QueryRowContext(ctx, `SELECT state,customer_effect FROM ai_fixture_action WHERE namespace=$1 AND id=$2 FOR UPDATE`, r.ns, r.id).Scan(&state, &delivered); err != nil {
			return unavailable(err)
		}
		if delivered {
			continue
		}
		if _, err = tx.ExecContext(ctx, `SELECT action_id FROM careeros_draft_allowance WHERE namespace=$1 AND action_id=$2 FOR UPDATE`, r.ns, r.id); err != nil {
			return unavailable(err)
		}
		var deadline, grant time.Time
		var purposeState string
		if err = tx.QueryRowContext(ctx, `SELECT deadline,grant_expires_at,state FROM careeros_draft_actions WHERE namespace=$1 AND action_id=$2`, r.ns, r.id).Scan(&deadline, &grant, &purposeState); err != nil {
			return unavailable(err)
		}
		if grant.Before(deadline) {
			deadline = grant
		}
		if a.expires.Valid && a.expires.Time.Before(deadline) {
			deadline = a.expires.Time
		}
		if a.accessEnded.Valid && a.accessEnded.Time.Before(deadline) {
			deadline = a.accessEnded.Time
		}
		now, e := pilotNow(ctx, tx)
		if e != nil {
			return e
		}
		terminal := state == "canceled" || state == "closed" || state == "deleted" || purposeState == "closed" || purposeState == "deleted" || !now.Before(deadline)
		if !terminal {
			continue
		}
		if _, err = tx.ExecContext(ctx, `UPDATE careeros_draft_actions SET state=CASE WHEN state='deleted' THEN state ELSE 'closed' END,cleanup_queued=true WHERE namespace=$1 AND action_id=$2`, r.ns, r.id); err != nil {
			return unavailable(err)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE ai_fixture_action SET state=CASE WHEN state='deleted' THEN state ELSE 'closed' END,customer_reserved=false WHERE namespace=$1 AND id=$2 AND NOT customer_effect`, r.ns, r.id); err != nil {
			return unavailable(err)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE careeros_draft_allowance SET state='released',released_at=clock_timestamp() WHERE namespace=$1 AND action_id=$2 AND state='reserved'`, r.ns, r.id); err != nil {
			return unavailable(err)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE careeros_draft_admissions SET state=CASE WHEN state='deleted' THEN state ELSE 'closed' END WHERE owner=$1 AND creation_key=(SELECT creation_key FROM careeros_draft_actions WHERE namespace=$2 AND action_id=$3)`, owner, r.ns, r.id); err != nil {
			return unavailable(err)
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_draft_content WHERE namespace=$1 AND action_id=$2`, r.ns, r.id); err != nil {
			return unavailable(err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_lifecycle_purpose_cleanup VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, owner, r.ns, r.id, now); err != nil {
			return unavailable(err)
		}
	}
	return nil
}

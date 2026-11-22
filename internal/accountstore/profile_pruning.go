package accountstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/profiledocument"
	"time"
)

func recordProfilePruning(ctx context.Context, tx *sql.Tx, owner string, before, after []byte) error {
	if string(before) == "null" {
		return nil
	}
	old, err := profiledocument.Decode(before)
	if err != nil {
		return ErrValidation
	}
	next, err := profiledocument.Decode(after)
	if err != nil {
		return ErrValidation
	}
	retained := map[string]bool{}
	for _, e := range next.Evidence {
		retained[e.ID] = true
	}
	stamp, err := pilotNow(ctx, tx)
	if err != nil {
		return err
	}
	for _, e := range old.Evidence {
		if retained[e.ID] {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_profile_pruning(account_id,profile_id,evidence_id,previous_revision,new_revision,removed_at,recovery_copy_deadline) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, owner, []byte(old.ID), e.ID, old.Revision, next.Revision, stamp, stamp.Add(30*24*time.Hour)); err != nil {
			return unavailable(err)
		}
	}
	return nil
}

type ProfilePruningFence struct {
	AccountID            string    `json:"accountId"`
	ProfileID            string    `json:"profileId"`
	EvidenceID           string    `json:"evidenceId"`
	PreviousRevision     int64     `json:"previousRevision"`
	NewRevision          int64     `json:"newRevision"`
	RemovedAt            time.Time `json:"removedAt"`
	RecoveryCopyDeadline time.Time `json:"recoveryCopyDeadline"`
}
type ProfilePruningSnapshot struct {
	Version string                `json:"version"`
	Fences  []ProfilePruningFence `json:"fences"`
	Seal    string                `json:"seal"`
}

const profilePruningVersion = "careeros-profile-pruning-v1"

func (r *DeletionRecovery) pruningSeal(v ProfilePruningSnapshot) string {
	v.Seal = ""
	raw, _ := json.Marshal(v)
	m := hmac.New(sha256.New, r.key)
	m.Write([]byte(profilePruningVersion + "\n"))
	m.Write(raw)
	return hex.EncodeToString(m.Sum(nil))
}

// ExportProfilePruning carries only original revision/identifier/clock fences.
// It neither exports a historical Profile nor proves hosted recovery readiness.
func (r *DeletionRecovery) ExportProfilePruning(ctx context.Context) (ProfilePruningSnapshot, error) {
	v := ProfilePruningSnapshot{Version: profilePruningVersion, Fences: []ProfilePruningFence{}}
	tx, err := r.store.db.BeginTx(ctx, nil)
	if err != nil {
		return v, unavailable(err)
	}
	defer tx.Rollback()
	if err = lockPilot(ctx, tx); err != nil {
		return v, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.id FROM careeros_accounts a WHERE EXISTS(SELECT 1 FROM careeros_profile_pruning p WHERE p.account_id=a.id) ORDER BY a.id FOR UPDATE`)
	if err != nil {
		return v, unavailable(err)
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return v, unavailable(err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT account_id,profile_id,evidence_id,previous_revision,new_revision,removed_at,recovery_copy_deadline FROM careeros_profile_pruning ORDER BY account_id,profile_id,evidence_id LIMIT 10001`)
	if err != nil {
		return v, unavailable(err)
	}
	for rows.Next() {
		var f ProfilePruningFence
		var id []byte
		if err = rows.Scan(&f.AccountID, &id, &f.EvidenceID, &f.PreviousRevision, &f.NewRevision, &f.RemovedAt, &f.RecoveryCopyDeadline); err != nil {
			break
		}
		f.ProfileID = string(id)
		v.Fences = append(v.Fences, f)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return v, unavailable(err)
	}
	if len(v.Fences) > 10000 {
		return v, auth.ErrCapacity
	}
	v.Seal = r.pruningSeal(v)
	return v, unavailable(tx.Commit())
}
func (r *DeletionRecovery) ApplyProfilePruning(ctx context.Context, v ProfilePruningSnapshot) error {
	if v.Version != profilePruningVersion || len(v.Fences) > 10000 || !hmac.Equal([]byte(v.Seal), []byte(r.pruningSeal(v))) {
		return ErrValidation
	}
	seen := map[string]bool{}
	for _, f := range v.Fences {
		key := f.AccountID + "\x00" + f.ProfileID + "\x00" + f.EvidenceID
		if !validOperation(f.AccountID) || f.ProfileID == "" || len(f.ProfileID) > 1024 || f.EvidenceID == "" || len(f.EvidenceID) > 1024 || f.PreviousRevision < 1 || f.NewRevision <= f.PreviousRevision || f.NewRevision > 9007199254740991 || f.RemovedAt.IsZero() || !f.RecoveryCopyDeadline.Equal(f.RemovedAt.Add(30*24*time.Hour)) || seen[key] {
			return ErrValidation
		}
		seen[key] = true
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
	for _, f := range v.Fences {
		owners[f.AccountID] = true
	}
	var ids []string
	for id := range owners {
		ids = append(ids, id)
	}
	// All writers hold the global pilot lock first; lock owner order consistently.
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
	for _, f := range v.Fences {
		if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_profile_pruning VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, f.AccountID, []byte(f.ProfileID), f.EvidenceID, f.PreviousRevision, f.NewRevision, f.RemovedAt, f.RecoveryCopyDeadline); err != nil {
			return unavailable(err)
		}
		var same bool
		if err = tx.QueryRowContext(ctx, `SELECT previous_revision=$4 AND new_revision=$5 AND removed_at=$6 AND recovery_copy_deadline=$7 FROM careeros_profile_pruning WHERE account_id=$1 AND profile_id=$2 AND evidence_id=$3`, f.AccountID, []byte(f.ProfileID), f.EvidenceID, f.PreviousRevision, f.NewRevision, f.RemovedAt, f.RecoveryCopyDeadline).Scan(&same); err != nil {
			return unavailable(err)
		}
		if !same {
			return ErrConflict
		}
	}
	return unavailable(tx.Commit())
}

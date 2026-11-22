package accountstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

const DeletionFenceVersion = "careeros-deletion-fences-v1"

type ClosedIdentity struct {
	Provider string `json:"provider"`
	Issuer   string `json:"issuer"`
	Subject  string `json:"subject"`
}

// DeletionFence retains only protective linkage, original clocks and replay
// binding. Closed identities are the minimum deny mapping, not sign-in tokens.
type DeletionFence struct {
	AccountID            string           `json:"accountId"`
	OperationID          string           `json:"operationId"`
	Kind                 string           `json:"kind"`
	ProfileID            string           `json:"profileId"`
	Revision             int64            `json:"revision"`
	RequestBinding       []byte           `json:"requestBinding"`
	AccountGeneration    uint64           `json:"accountGeneration"`
	FencedAt             time.Time        `json:"fencedAt"`
	RemovedAt            *time.Time       `json:"removedAt,omitempty"`
	RecoveryCopyDeadline time.Time        `json:"recoveryCopyDeadline"`
	DeletedProfileIDs    []string         `json:"deletedProfileIds"`
	ClosedIdentities     []ClosedIdentity `json:"closedIdentities,omitempty"`
}
type AccessEndFence struct {
	AccountID     string     `json:"accountId"`
	ActivatedAt   time.Time  `json:"activatedAt"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	AccessEndedAt *time.Time `json:"accessEndedAt,omitempty"`
	RetrievalHeld bool       `json:"retrievalHeld"`
	State         string     `json:"state"`
}
type DeletionSnapshot struct {
	Version    string           `json:"version"`
	Fences     []DeletionFence  `json:"fences"`
	AccessEnds []AccessEndFence `json:"accessEnds"`
	Seal       string           `json:"seal"`
}

// DeletionRecovery is an explicitly code-injected, independently keyed
// migrator/recovery capability. No HTTP route, environment loader or ordinary
// workspace role creates it. Applying markers does not open a restored system.
type DeletionRecovery struct {
	store *Store
	key   []byte
}

func NewDeletionRecovery(store *Store, key []byte) (*DeletionRecovery, error) {
	if store == nil || len(key) < 32 || len(key) > 128 {
		return nil, ErrValidation
	}
	return &DeletionRecovery{store: store, key: append([]byte(nil), key...)}, nil
}
func (r *DeletionRecovery) seal(v DeletionSnapshot) string {
	v.Seal = ""
	raw, _ := json.Marshal(v)
	m := hmac.New(sha256.New, r.key)
	m.Write([]byte(DeletionFenceVersion + "\n"))
	m.Write(raw)
	return hex.EncodeToString(m.Sum(nil))
}
func (r *DeletionRecovery) Export(ctx context.Context) (DeletionSnapshot, error) {
	v := DeletionSnapshot{Version: DeletionFenceVersion, Fences: []DeletionFence{}, AccessEnds: []AccessEndFence{}}
	tx, err := r.store.db.BeginTx(ctx, nil)
	if err != nil {
		return v, unavailable(err)
	}
	defer tx.Rollback()
	if err = lockPilot(ctx, tx); err != nil {
		return v, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM careeros_accounts WHERE EXISTS(SELECT 1 FROM careeros_deletion_operations WHERE account_id=id) OR EXISTS(SELECT 1 FROM careeros_pilot_access WHERE account_id=id AND (access_ended_at IS NOT NULL OR retrieval_held OR state<>'active')) ORDER BY id FOR UPDATE`)
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
	rows, err = tx.QueryContext(ctx, `SELECT account_id,operation_id,kind,profile_id,profile_revision,request_hash,account_generation,fenced_at,removed_at FROM careeros_deletion_operations ORDER BY account_id,operation_id`)
	if err != nil {
		return v, unavailable(err)
	}
	for rows.Next() {
		var f DeletionFence
		var id []byte
		var removed sql.NullTime
		if err = rows.Scan(&f.AccountID, &f.OperationID, &f.Kind, &id, &f.Revision, &f.RequestBinding, &f.AccountGeneration, &f.FencedAt, &removed); err != nil {
			break
		}
		f.ProfileID = string(id)
		f.DeletedProfileIDs = []string{}
		f.RecoveryCopyDeadline = f.FencedAt.Add(30 * 24 * time.Hour)
		if removed.Valid {
			f.RemovedAt = &removed.Time
		}
		v.Fences = append(v.Fences, f)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return v, unavailable(err)
	}
	for i := range v.Fences {
		f := &v.Fences[i]
		rows, err = tx.QueryContext(ctx, `SELECT profile_id FROM careeros_deleted_profiles WHERE account_id=$1 AND operation_id=$2 ORDER BY profile_id`, f.AccountID, f.OperationID)
		if err != nil {
			return v, unavailable(err)
		}
		for rows.Next() {
			var id []byte
			if err = rows.Scan(&id); err != nil {
				break
			}
			f.DeletedProfileIDs = append(f.DeletedProfileIDs, string(id))
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return v, unavailable(err)
		}
		if f.Kind == "account" {
			rows, err = tx.QueryContext(ctx, `SELECT provider,issuer,subject FROM careeros_identities WHERE account_id=$1 ORDER BY provider,issuer,subject`, f.AccountID)
			if err != nil {
				return v, unavailable(err)
			}
			for rows.Next() {
				var p, issuer, subject []byte
				if err = rows.Scan(&p, &issuer, &subject); err != nil {
					break
				}
				f.ClosedIdentities = append(f.ClosedIdentities, ClosedIdentity{string(p), string(issuer), string(subject)})
			}
			if err == nil {
				err = rows.Err()
			}
			rows.Close()
			if err != nil {
				return v, unavailable(err)
			}
		}
	}
	rows, err = tx.QueryContext(ctx, `SELECT account_id,activated_at,expires_at,access_ended_at,retrieval_held OR state='suspended',state FROM careeros_pilot_access WHERE activated_at IS NOT NULL AND (access_ended_at IS NOT NULL OR retrieval_held OR state<>'active') ORDER BY account_id`)
	if err != nil {
		return v, unavailable(err)
	}
	for rows.Next() {
		var f AccessEndFence
		var ended sql.NullTime
		if err = rows.Scan(&f.AccountID, &f.ActivatedAt, &f.ExpiresAt, &ended, &f.RetrievalHeld, &f.State); err != nil {
			break
		}
		if ended.Valid {
			f.AccessEndedAt = &ended.Time
		}
		v.AccessEnds = append(v.AccessEnds, f)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return v, unavailable(err)
	}
	if err = tx.Commit(); err != nil {
		return v, unavailable(err)
	}
	v.Seal = r.seal(v)
	return v, nil
}

func validDeletionSnapshot(v DeletionSnapshot) bool {
	if v.Version != DeletionFenceVersion || len(v.Fences) > 10000 || len(v.AccessEnds) > 10000 {
		return false
	}
	seen := map[string]bool{}
	for _, f := range v.Fences {
		key := f.AccountID + "/" + f.OperationID
		if !validOperation(f.AccountID) || !validOperation(f.OperationID) || seen[key] || len(f.RequestBinding) != 32 || f.AccountGeneration == 0 || f.FencedAt.IsZero() || !f.RecoveryCopyDeadline.Equal(f.FencedAt.Add(30*24*time.Hour)) || len(f.DeletedProfileIDs) > 10000 {
			return false
		}
		seen[key] = true
		if f.Kind == "profile" {
			if f.ProfileID == "" || len(f.ProfileID) > 1024 || f.Revision < 1 || len(f.ClosedIdentities) > 0 {
				return false
			}
		} else if f.Kind == "account" {
			if f.ProfileID != "" || f.Revision != 0 || len(f.ClosedIdentities) == 0 || len(f.ClosedIdentities) > 8 {
				return false
			}
		} else {
			return false
		}
		if f.RemovedAt != nil && f.RemovedAt.Before(f.FencedAt) {
			return false
		}
		for _, id := range f.DeletedProfileIDs {
			if id == "" || len(id) > 1024 {
				return false
			}
		}
		for _, id := range f.ClosedIdentities {
			if id.Provider == "" || len(id.Provider) > 128 || id.Issuer == "" || len(id.Issuer) > 2048 || id.Subject == "" || len(id.Subject) > 2048 {
				return false
			}
		}
	}
	for _, f := range v.AccessEnds {
		if !validOperation(f.AccountID) || f.ActivatedAt.IsZero() || !f.ExpiresAt.After(f.ActivatedAt) || (f.AccessEndedAt != nil && f.AccessEndedAt.IsZero()) || (f.State != "active" && f.State != "suspended" && f.State != "revoked") {
			return false
		}
	}
	return true
}

// Apply verifies the independently surviving snapshot and commits all deny
// markers BEFORE bounded primary cleanup. It never grants rights, releases
// provider holds, rewrites rates or opens inference/results. The downstream
// recovery controller must keep serving closed on any error.
func (r *DeletionRecovery) Apply(ctx context.Context, v DeletionSnapshot) error {
	if !validDeletionSnapshot(v) || !hmac.Equal([]byte(v.Seal), []byte(r.seal(v))) {
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
	// A closure can restore a missing DENIED account and its subject mapping,
	// never a workspace account. Profile-only records cannot create an account.
	for _, f := range v.Fences {
		if f.Kind != "account" {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_accounts VALUES($1,'deleted',$2,$3,''::bytea,''::bytea) ON CONFLICT(id) DO UPDATE SET status='deleted',generation=GREATEST(careeros_accounts.generation,EXCLUDED.generation)`, f.AccountID, f.AccountGeneration, f.FencedAt); err != nil {
			return unavailable(err)
		}
		for _, id := range f.ClosedIdentities {
			var target string
			err = tx.QueryRowContext(ctx, `SELECT account_id FROM careeros_identities WHERE provider=$1 AND issuer=$2 AND subject=$3`, []byte(id.Provider), []byte(id.Issuer), []byte(id.Subject)).Scan(&target)
			if err == nil && target != f.AccountID {
				return ErrConflict
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return unavailable(err)
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_identities VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, []byte(id.Provider), []byte(id.Issuer), []byte(id.Subject), f.AccountID, f.FencedAt); err != nil {
				return unavailable(err)
			}
		}
	}
	for _, f := range v.Fences {
		var account string
		if err = tx.QueryRowContext(ctx, `SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE`, f.AccountID).Scan(&account); err != nil {
			return unavailable(err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_deletion_operations(account_id,operation_id,kind,profile_id,profile_revision,request_hash,account_generation,state,fenced_at,removed_at,failure_code) VALUES($1,$2,$3,$4,$5,$6,$7,'fenced_pending',$8,$9,'') ON CONFLICT DO NOTHING`, f.AccountID, f.OperationID, f.Kind, []byte(f.ProfileID), f.Revision, f.RequestBinding, f.AccountGeneration, f.FencedAt, f.RemovedAt); err != nil {
			return unavailable(err)
		}
		var hash []byte
		var original time.Time
		if err = tx.QueryRowContext(ctx, `SELECT request_hash,fenced_at FROM careeros_deletion_operations WHERE account_id=$1 AND operation_id=$2 FOR UPDATE`, f.AccountID, f.OperationID).Scan(&hash, &original); err != nil {
			return unavailable(err)
		}
		if !bytes.Equal(hash, f.RequestBinding) || !original.Equal(f.FencedAt) {
			return ErrConflict
		}
		if f.RemovedAt != nil {
			if _, err = tx.ExecContext(ctx, `UPDATE careeros_deletion_operations SET removed_at=CASE WHEN removed_at IS NULL THEN $3 ELSE LEAST(removed_at,$3) END WHERE account_id=$1 AND operation_id=$2`, f.AccountID, f.OperationID, f.RemovedAt); err != nil {
				return unavailable(err)
			}
		}
		for _, id := range f.DeletedProfileIDs {
			if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_deleted_profiles VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, f.AccountID, []byte(id), f.OperationID, f.FencedAt); err != nil {
				return unavailable(err)
			}
		}
		if err = fenceCareerContent(ctx, tx, f.AccountID, f.Kind, f.ProfileID, f.FencedAt); err != nil {
			return err
		}
	}
	for _, f := range v.AccessEnds {
		result, e := tx.ExecContext(ctx, `UPDATE careeros_pilot_access SET state=CASE WHEN state='revoked' OR $6='revoked' OR $3::timestamptz IS NOT NULL THEN 'revoked' WHEN state='suspended' OR $6='suspended' OR $4 THEN 'suspended' ELSE state END,access_ended_at=CASE WHEN $3::timestamptz IS NULL THEN access_ended_at WHEN access_ended_at IS NULL THEN $3 ELSE LEAST(access_ended_at,$3) END,retrieval_held=retrieval_held OR $4,expires_at=LEAST(expires_at,$5) WHERE account_id=$1 AND activated_at=$2`, f.AccountID, f.ActivatedAt, f.AccessEndedAt, f.RetrievalHeld, f.ExpiresAt, f.State)
		if e != nil {
			return unavailable(e)
		}
		if n, e := result.RowsAffected(); e != nil || n != 1 {
			return ErrConflict
		}
	}
	if err = tx.Commit(); err != nil {
		return unavailable(err)
	}
	for _, f := range v.Fences {
		if _, err = r.store.PurgeDeletion(ctx, f.AccountID, f.OperationID); err != nil {
			return err
		}
	}
	return nil
}

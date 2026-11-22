package aicontrol

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"errors"

	"professional-information-repo/internal/auth"
)

// RecordedRateRevision records attributable reference evidence, not a dispatch
// pricing policy. Values are integer micro currency units per million tokens;
// missing categories stay missing. No record automatically reprices old rows,
// converts currency, changes a workflow rate or enables any provider access.
type RecordedRateRevision struct {
	ID         string `json:"id"`
	Replaces   string `json:"replaces"`
	Revision   int64  `json:"revision"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Tier       string `json:"tier"`
	Currency   string `json:"currency"`
	Input      *int64 `json:"input,omitempty"`
	Cached     *int64 `json:"cached,omitempty"`
	CacheWrite *int64 `json:"cacheWrite,omitempty"`
	Output     *int64 `json:"output,omitempty"`
}

func (r *Reconciliation) validateRateRevision(ctx context.Context, tx *sql.Tx, d Dispatch, c ReconciliationCommand) error {
	revision := c.RateRevision
	if revision == nil || !tokenLabel.MatchString(revision.ID) || !tokenLabel.MatchString(revision.Replaces) || revision.Revision <= 0 || revision.Provider != d.Provider || revision.Model != c.Evidence.Model || revision.Tier != "standard" || len(revision.Currency) != 3 || c.Evidence.Usage != nil || c.Evidence.Gross != nil || c.Evidence.Difference != nil {
		return ErrDenied
	}
	for _, letter := range revision.Currency {
		if letter < 'A' || letter > 'Z' {
			return ErrDenied
		}
	}
	for _, value := range []*int64{revision.Input, revision.Cached, revision.CacheWrite, revision.Output} {
		if value != nil && (*value < 0 || *value > 1_000_000_000_000) {
			return ErrDenied
		}
	}
	var old []byte
	err := tx.QueryRowContext(ctx, `SELECT binding FROM ai_recon_rate_revision WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, revision.ID).Scan(&old)
	if err == nil {
		if !hmac.Equal(old, decodeDigest(r.digest("rate-record", "immutable-rate-revision", revision))) {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ErrUnavailable
	}
	return nil
}
func (r *Reconciliation) recordRateRevision(ctx context.Context, tx *sql.Tx, c ReconciliationCommand) error {
	raw, _ := json.Marshal(c.RateRevision)
	_, err := tx.ExecContext(ctx, `INSERT INTO ai_recon_rate_revision(namespace,id,dispatch,binding,source,revision,data) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, r.h.cfg.Namespace, c.RateRevision.ID, c.Dispatch, decodeDigest(r.digest("rate-record", "immutable-rate-revision", c.RateRevision)), c.Evidence.Source, c.RateRevision.Revision, raw)
	return safe(err)
}
func (r *Reconciliation) RateRevisions(ctx context.Context, p auth.Principal, id string) ([]RecordedRateRevision, error) {
	tx, _, _, err := r.begin(ctx, p, id, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT data FROM ai_recon_rate_revision WHERE namespace=$1 AND dispatch=$2 ORDER BY recorded_at,id LIMIT 100`, r.h.cfg.Namespace, id)
	if err != nil {
		return nil, ErrUnavailable
	}
	out := []RecordedRateRevision{}
	for rows.Next() {
		var raw []byte
		var record RecordedRateRevision
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &record) != nil {
			rows.Close()
			return nil, ErrUnavailable
		}
		out = append(out, record)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, ErrUnavailable
	}
	return out, r.authorize(ctx, tx, p)
}

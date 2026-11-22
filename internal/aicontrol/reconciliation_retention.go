package aicontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type DetailPurpose struct {
	Field   string `json:"field"`
	Purpose string `json:"purpose"`
}
type DetailRetirementReview struct {
	Reference  string          `json:"reference"`
	Needed     []DetailPurpose `json:"needed"`
	NextReview *time.Time      `json:"nextReview,omitempty"`
}

func (r *Reconciliation) validateRetirement(d Dispatch, review *DetailRetirementReview) error {
	if review == nil || !tokenLabel.MatchString(review.Reference) || d.IntentAt == nil {
		return ErrDenied
	}
	now, healthy := r.h.cfg.Clock.Sample()
	if !healthy || now.Before(*d.IntentAt) || now.Sub(*d.IntentAt) < 30*24*time.Hour {
		return ErrDenied
	}
	seen := map[string]bool{}
	for _, need := range review.Needed {
		if need.Field != "usage" && need.Field != "request_id" && need.Field != "source_reference" {
			return ErrDenied
		}
		if need.Purpose != "dispute" && need.Purpose != "adjustment" && need.Purpose != "capacity" {
			return ErrDenied
		}
		key := need.Field + "/" + need.Purpose
		if seen[key] {
			return ErrDenied
		}
		seen[key] = true
	}
	if len(review.Needed) > 0 && (review.NextReview == nil || !review.NextReview.After(now) || review.NextReview.After(now.Add(7*24*time.Hour))) {
		return ErrDenied
	}
	if len(review.Needed) == 0 && review.NextReview != nil {
		return ErrDenied
	}
	return nil
}

func (r *Reconciliation) retireDetails(ctx context.Context, tx *sql.Tx, d Dispatch, review *DetailRetirementReview) error {
	if err := r.validateRetirement(d, review); err != nil {
		return err
	}
	if err := r.sealLegacyEvidence(ctx, tx); err != nil {
		return err
	}
	usage, request, source := false, false, false
	for _, need := range review.Needed {
		switch need.Field {
		case "usage":
			usage = true
		case "request_id":
			request = true
		case "source_reference":
			source = true
		}
	}
	// A purpose review retaining every field is a deferral, not retirement.
	// In particular it must not erase observation attribution metadata or
	// retroactively restore any previously retired data/floor.
	if usage && request && source {
		return r.recordRetentionReview(ctx, tx, d.ID, review)
	}
	var financialKind string
	err := tx.QueryRowContext(ctx, `SELECT c.kind FROM ai_recon_state s JOIN ai_recon_command c ON c.namespace=s.namespace AND c.id=s.command_id WHERE s.namespace=$1 AND s.dispatch=$2`, r.h.cfg.Namespace, d.ID).Scan(&financialKind)
	if err != nil && err != sql.ErrNoRows {
		return ErrUnavailable
	}
	requiredLost := !request || !source || (financialKind == "usage" && !usage) || financialKind == ""
	if requiredLost {
		if _, err = tx.ExecContext(ctx, `UPDATE ai_recon_state SET attribution_floor=GREATEST(attribution_floor,$3) WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, d.ID, d.Counted); err != nil {
			return ErrUnavailable
		}
	}
	// Keep command/source digests, correction identities and original results.
	// Retire only detailed numeric/request/reference fields that the explicit
	// review says have no current dispute, adjustment or capacity-proof purpose.
	if _, err := tx.ExecContext(ctx, `UPDATE ai_recon_command SET detail=
 CASE WHEN $3 THEN detail ELSE detail-'usage' END
 WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, d.ID, usage); err != nil {
		return ErrUnavailable
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ai_recon_command SET detail=
 CASE WHEN $3 THEN detail ELSE detail-'requestId' END
 WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, d.ID, request); err != nil {
		return ErrUnavailable
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ai_recon_command SET detail=
 CASE WHEN $3 THEN detail ELSE detail-'reason'-'source' END
 WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, d.ID, source); err != nil {
		return ErrUnavailable
	}
	// Observation identity rows survive, so delayed/duplicate append cannot
	// repopulate retired detail through ON CONFLICT after a restart.
	if _, err := tx.ExecContext(ctx, `UPDATE ai_fixture_observation SET data=jsonb_build_object('detail_retired',true)
 || CASE WHEN $3 THEN jsonb_build_object('input',data->'input','output',data->'output','cached',data->'cached','reasoning',data->'reasoning','cache_write',data->'cache_write') ELSE '{}'::jsonb END
 || CASE WHEN $4 THEN jsonb_build_object('request_id',data->'request_id') ELSE '{}'::jsonb END
 WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, d.ID, usage, request); err != nil {
		return ErrUnavailable
	}
	// The old fixture evidence IDs and keyed conflict bindings remain. A compact
	// non-content closure audit remains independently verifiable.
	if !usage && !request && !source {
		if _, err := tx.ExecContext(ctx, `UPDATE ai_fixture_evidence SET data='{}'::jsonb WHERE namespace=$1 AND dispatch=$2 AND kind<>'conservative_closure'`, r.h.cfg.Namespace, d.ID); err != nil {
			return ErrUnavailable
		}
	}
	if err := r.recordRetentionReview(ctx, tx, d.ID, review); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE ai_fixture_dispatch SET compacted=true WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, d.ID)
	return safe(err)
}

func (r *Reconciliation) recordRetentionReview(ctx context.Context, tx *sql.Tx, id string, review *DetailRetirementReview) error {
	raw, _ := json.Marshal(review.Needed)
	_, err := tx.ExecContext(ctx, `UPDATE ai_recon_state SET next_review=$3,retained_purpose=$4,retention_reference=$5 WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, id, review.NextReview, string(raw), review.Reference)
	return safe(err)
}

// retainedAttribution checks the accepted obligation's surviving evidence,
// rather than treating an unrelated retired field as loss of every attribution.
// Immutable replay markers alone cannot reconstruct removed detail.
func (r *Reconciliation) retainedAttribution(ctx context.Context, tx *sql.Tx, d Dispatch, id, source string, revision int64, incoming ReconciliationEvidence) (bool, error) {
	if id == "" {
		return false, nil
	}
	var kind string
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT kind,detail FROM ai_recon_command WHERE namespace=$1 AND dispatch=$2 AND id=$3`, r.h.cfg.Namespace, d.ID, id).Scan(&kind, &raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, ErrUnavailable
	}
	var prior ReconciliationEvidence
	if json.Unmarshal(raw, &prior) != nil {
		return false, nil
	}
	if prior.Source != source || prior.Revision != revision || !tokenLabel.MatchString(prior.Reason) || !metadataLabel.MatchString(prior.RequestID) || prior.RequestID == "unavailable" || prior.RequestID != incoming.RequestID {
		return false, nil
	}
	if prior.Provider != incoming.Provider || prior.Model != incoming.Model || prior.Tier != incoming.Tier || prior.Rate != incoming.Rate || prior.Currency != incoming.Currency || prior.BillingNamespace != incoming.BillingNamespace {
		return false, nil
	}
	switch kind {
	case "bill", "corrected_bill":
		return prior.Gross != nil && d.Expense != nil && *prior.Gross == *d.Expense, nil
	case "usage":
		if prior.Usage == nil || d.Expense == nil {
			return false, nil
		}
		var amount int64
		if prior.Rate == QualificationRateID {
			amount, err = QualificationEstimate(prior.Provider, prior.Model, prior.Tier, prior.Rate, *prior.Usage)
		} else {
			amount, err = Estimate(prior.Provider, prior.Model, prior.Tier, *prior.Usage)
		}
		return err == nil && amount == *d.Expense, nil
	case "maximum":
		return d.Expense == nil && d.Category == "reviewed_conservative_maximum", nil
	}
	return false, nil
}

// Existing reviewed schemas did not distinguish a review from actual loss.
// Keep fully surviving attribution usable; when necessary historical detail is
// already gone, preserve its current counted floor conservatively on upgrade.
// This never raises existing counted expense or releases any obligation.
func ensureAttributionFloor(ctx context.Context, tx *sql.Tx) error {
	var present bool
	if tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid='ai_recon_state'::regclass AND attname='attribution_floor' AND NOT attisdropped)`).Scan(&present) != nil {
		return ErrUnavailable
	}
	if present {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE ai_recon_state ADD COLUMN attribution_floor bigint NOT NULL DEFAULT 0 CHECK(attribution_floor>=0)`); err != nil {
		return ErrUnavailable
	}
	_, err := tx.ExecContext(ctx, `UPDATE ai_recon_state s SET attribution_floor=d.counted FROM ai_fixture_dispatch d WHERE d.namespace=s.namespace AND d.id=s.dispatch AND d.compacted AND EXISTS(
 SELECT 1 FROM ai_recon_command c LEFT JOIN LATERAL (
 SELECT p.kind,p.detail FROM ai_recon_command p WHERE p.namespace=c.namespace AND p.dispatch=c.dispatch AND p.version<c.version AND p.kind IN ('usage','bill','corrected_bill','maximum') ORDER BY p.version DESC LIMIT 1
 ) prior ON true WHERE c.namespace=s.namespace AND c.dispatch=s.dispatch AND c.kind='compact' AND
 (prior.kind IS NULL OR NOT (prior.detail ? 'source') OR NOT (prior.detail ? 'requestId') OR (prior.kind='usage' AND NOT (prior.detail ? 'usage'))))`)
	return safe(err)
}

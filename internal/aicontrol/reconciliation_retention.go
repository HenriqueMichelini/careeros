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
	raw, _ := json.Marshal(review.Needed)
	if _, err := tx.ExecContext(ctx, `UPDATE ai_recon_state SET next_review=$3,retained_purpose=$4,retention_reference=$5 WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, d.ID, review.NextReview, string(raw), review.Reference); err != nil {
		return ErrUnavailable
	}
	_, err := tx.ExecContext(ctx, `UPDATE ai_fixture_dispatch SET compacted=true WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, d.ID)
	return safe(err)
}

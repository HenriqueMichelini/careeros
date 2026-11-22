package aicontrol

import (
	"context"
	"database/sql"
)

const localBoundsReviewVersion = "code-owned-fake-bounds-review-v1"

// IssueCodeOwnedFakeBoundsReview verifies the unchanged local fake bound. It
// cannot reconfigure workflow limits or claim a reviewed live provider bound.
// Only a migrator-owned connection can install this independently checked proof.
func (h *Harness) IssueCodeOwnedFakeBoundsReview(ctx context.Context, migrator *sql.DB, id, reference string) error {
	if h == nil || migrator == nil || !tokenLabel.MatchString(id) || !tokenLabel.MatchString(reference) {
		return ErrDenied
	}
	var owner bool
	if migrator.QueryRowContext(ctx, `SELECT current_user=pg_get_userbyid(relowner) FROM pg_class WHERE oid='ai_recon_bounds_review'::regclass`).Scan(&owner) != nil || !owner {
		return ErrDenied
	}
	var contract, provider, rate, execution string
	var bound, input, output int64
	if migrator.QueryRowContext(ctx, `SELECT c.provider||'/'||c.model||'/'||c.tier||'/'||c.rate_id||'/'||c.currency,c.provider,c.rate_id,d.maximum,d.input_tokens,d.output_tokens,a.execution_version FROM ai_recon_contract c JOIN ai_fixture_dispatch d ON d.namespace=c.namespace AND d.id=c.dispatch JOIN ai_fixture_action a ON a.namespace=d.namespace AND a.id=d.action WHERE c.namespace=$1 AND c.dispatch=$2`, h.cfg.Namespace, id).Scan(&contract, &provider, &rate, &bound, &input, &output, &execution) != nil {
		return ErrDenied
	}
	expected := int64(60)
	if rate == QualificationRateID {
		if execution == "qualification-local-20261102-v1" {
			expected = 12000
			if provider == "typesafe" {
				expected = 1344
			}
		} else {
			if input < 0 || input > 1536000 || output < 0 || output > 240000 {
				return ErrDenied
			}
			if provider == "typesafe" {
				expected = (input*42 + 999) / 1000
			} else {
				expected = (input*125 + output*500 + 999) / 1000
			}
		}
	} else if rate != "synthetic-unit-v1" {
		return ErrDenied
	}
	if bound != expected {
		return ErrDenied
	}
	_, err := migrator.ExecContext(ctx, `INSERT INTO ai_recon_bounds_review(namespace,reference,contract,original_bound,version) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, h.cfg.Namespace, reference, contract, bound, localBoundsReviewVersion)
	if err != nil {
		return ErrUnavailable
	}
	var old string
	var oldBound int64
	if migrator.QueryRowContext(ctx, `SELECT contract,original_bound FROM ai_recon_bounds_review WHERE namespace=$1 AND reference=$2`, h.cfg.Namespace, reference).Scan(&old, &oldBound) != nil || old != contract || oldBound != bound {
		return ErrConflict
	}
	return nil
}

func (r *Reconciliation) validateReopen(ctx context.Context, tx *sql.Tx, d Dispatch, c ReconciliationCommand) error {
	if c.StopVersion <= 0 || !tokenLabel.MatchString(c.BoundsReference) || !tokenLabel.MatchString(c.ReconciliationReference) || c.Evidence.Usage != nil || c.Evidence.Gross != nil || c.Evidence.Difference != nil {
		return ErrDenied
	}
	var stopped, retired bool
	var version int64
	var contract string
	if tx.QueryRowContext(ctx, `SELECT s.stopped,s.version,s.contract,n.retired FROM ai_recon_stop s JOIN ai_fixture_namespace n ON n.id=s.namespace WHERE s.namespace=$1`, r.h.cfg.Namespace).Scan(&stopped, &version, &contract, &retired) != nil || !stopped || retired || version != c.StopVersion {
		return ErrDenied
	}
	var reviewContract, reviewVersion string
	var bound int64
	if tx.QueryRowContext(ctx, `SELECT contract,version,original_bound FROM ai_recon_bounds_review WHERE namespace=$1 AND reference=$2`, r.h.cfg.Namespace, c.BoundsReference).Scan(&reviewContract, &reviewVersion, &bound) != nil || reviewContract != contract || reviewVersion != localBoundsReviewVersion || bound != d.Maximum {
		return ErrDenied
	}
	var outstanding int
	if tx.QueryRowContext(ctx, `SELECT count(*) FROM ai_fixture_dispatch d LEFT JOIN ai_recon_state r ON r.namespace=d.namespace AND r.dispatch=d.id WHERE d.namespace=$1 AND (d.counted>d.maximum OR COALESCE(r.unpriced,false))`, r.h.cfg.Namespace).Scan(&outstanding) != nil || outstanding != 0 {
		return ErrDenied
	}
	// Bind the owner reopening to a committed compatible reviewed correction.
	var accepted bool
	if tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_recon_command WHERE namespace=$1 AND id=$2 AND dispatch=$3 AND kind='corrected_bill' AND after_amount<=$4)`, r.h.cfg.Namespace, c.ReconciliationReference, d.ID, d.Maximum).Scan(&accepted) != nil || !accepted {
		return ErrDenied
	}
	return nil
}

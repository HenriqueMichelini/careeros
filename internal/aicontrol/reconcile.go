package aicontrol

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

// Reviewer is an explicit synthetic reconciliation capability. Runtime Session
// has no adjustment/cleanup method. Production actor elevation remains absent.
type Reviewer struct {
	h          *Harness
	authorized bool
}

func (h *Harness) SyntheticReviewer() *Reviewer {
	return &Reviewer{h: &Harness{db: h.reviewerDB, cfg: h.cfg, hooks: map[string]func() error{}}, authorized: true}
}

// Evidence contains only bounded identities and numeric accounting metadata.
// No response/error/invoice/raw payload is accepted or retained.
type Evidence struct {
	ID, Dispatch, Provider, Kind, RequestID, Model, Tier string
	Amount                                               *int64
	ReleaseFloor                                         bool
	Usage                                                *Usage
	// Version orders reviewed fixture evidence independently of its idempotency
	// key. Zero denotes initial version 1 for the original fixture API.
	Version int64
}

// Usage categories deliberately retain missingness. Cached/reasoning categories
// are subsets, never added twice to input/output totals. Unknown category/tier
// denies an estimate rather than manufacturing a zero price.
type Usage struct {
	Input, Output, Cached, Reasoning, CacheWrite *int64
	Unsupported                                  bool
}

func Estimate(provider, model, tier string, u Usage) (int64, error) {
	parts, err := EstimateParts(provider, model, tier, "synthetic-unit-v1", u)
	return parts.Total, err
}

// UsageParts keeps subsets visible while charging each token exactly once.
// Rates are a named artificial fixture contract, never inferred vendor prices.
type UsageParts struct{ UncachedInput, CachedInput, Output, ReasoningSubset, Total int64 }

func EstimateParts(provider, model, tier, rateID string, u Usage) (UsageParts, error) {
	var parts UsageParts
	if rateID != "synthetic-unit-v1" {
		return parts, ErrDenied
	}
	if u.Unsupported || u.Input == nil || u.Output == nil || model == "" || tier != "standard" {
		return parts, ErrDenied
	}
	for _, n := range []*int64{u.Input, u.Output, u.Cached, u.Reasoning, u.CacheWrite} {
		if n != nil && *n < 0 {
			return parts, ErrDenied
		}
	}
	if u.Cached != nil && *u.Cached > *u.Input || u.Reasoning != nil && *u.Reasoning > *u.Output {
		return parts, ErrDenied
	}
	if provider == "openai" {
		if model != "gpt-6-luna" || u.CacheWrite != nil {
			return parts, ErrDenied
		}
	} else if provider == "typesafe" {
		if model != "jev-1.13.0" || u.Cached != nil || u.Reasoning != nil || u.CacheWrite != nil {
			return parts, ErrDenied
		}
	} else {
		return parts, ErrDenied
	}
	parts.UncachedInput = *u.Input
	parts.Output = *u.Output
	if u.Cached != nil {
		parts.CachedInput = *u.Cached
		parts.UncachedInput -= *u.Cached
	}
	if u.Reasoning != nil {
		parts.ReasoningSubset = *u.Reasoning
	}
	total, err := checkedSum(parts.UncachedInput, parts.CachedInput)
	if err != nil {
		return UsageParts{}, err
	}
	parts.Total, err = checkedSum(total, parts.Output)
	return parts, err
}

func (r *Reviewer) ApplyEvidence(ctx context.Context, p auth.Principal, e Evidence) error {
	if r == nil || !r.authorized || r.h == nil || !tokenLabel.MatchString(e.ID) || !tokenLabel.MatchString(e.Dispatch) || e.RequestID != "" && !tokenLabel.MatchString(e.RequestID) || e.Kind != "estimate" && e.Kind != "bill" && e.Kind != "terminal" {
		return ErrDenied
	}
	if e.Kind == "terminal" && e.Amount != nil {
		return ErrDenied
	}
	if e.Version < 0 {
		return ErrDenied
	}
	if e.Version == 0 {
		e.Version = 1
	}
	if e.Usage != nil {
		amount, err := Estimate(e.Provider, e.Model, e.Tier, *e.Usage)
		if err != nil {
			return err
		}
		if e.Amount != nil && *e.Amount != amount {
			return ErrConflict
		}
		e.Amount = &amount
	}
	if e.Kind != "terminal" && (e.Amount == nil || *e.Amount < 0) {
		return ErrDenied
	}
	if e.Model != "" && e.Model != "gpt-6-luna" && e.Model != "jev-1.13.0" || e.Tier != "" && e.Tier != "standard" {
		return ErrDenied
	}
	if e.Model != "" && (e.Provider == "openai" && e.Model != "gpt-6-luna" || e.Provider == "typesafe" && e.Model != "jev-1.13.0") {
		return ErrDenied
	}
	raw, _ := json.Marshal(e)
	binding, err := r.h.fingerprint(p.AccountID, "evidence", raw)
	if err != nil {
		return err
	}
	return r.h.mutateDispatch(ctx, p, e.Dispatch, func(tx *sql.Tx, d Dispatch) error {
		if d.Provider != e.Provider || d.State == "reserved" || d.State == "aborted" || d.Compacted {
			return ErrDenied
		}
		var old []byte
		var priorDispatch string
		err := tx.QueryRowContext(ctx, `SELECT binding,dispatch FROM ai_fixture_evidence WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, e.ID).Scan(&old, &priorDispatch)
		if err == nil {
			if priorDispatch == e.Dispatch && hmac.Equal(old, binding) {
				return nil
			}
			return ErrConflict
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return ErrUnavailable
		}
		if e.Kind == "terminal" {
			if d.Remote != "terminal_proven" {
				if err = r.h.adjustScopes(ctx, tx, d.ID, 0, -1); err != nil {
					return err
				}
				_, err = tx.ExecContext(ctx, `UPDATE ai_fixture_dispatch SET remote_state='terminal_proven' WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, d.ID)
				if err != nil {
					return ErrUnavailable
				}
			}
		} else {
			if d.Category == "confirmed_bill" && e.Kind != "bill" {
				return ErrDenied
			}
			// A new evidence ID cannot silently replay an older/lower report.
			// A correction needs a later explicit reviewed version. Estimates
			// may upgrade to bills, never overwrite a confirmed bill.
			if e.Version < d.EvidenceVersion {
				return ErrDenied
			}
			if e.Version == d.EvidenceVersion && d.Expense != nil && *e.Amount != *d.Expense && !(e.Kind == "bill" && d.Category == "supported_estimate") {
				return ErrConflict
			}
			floor := d.Floor
			if e.ReleaseFloor {
				floor = *e.Amount
			}
			contribution := countMax(floor, *e.Amount)
			if err = r.h.adjustScopes(ctx, tx, d.ID, contribution-d.Counted, 0); err != nil {
				return err
			}
			category := "supported_estimate"
			if e.Kind == "bill" {
				category = "confirmed_bill"
			}
			_, err = tx.ExecContext(ctx, `UPDATE ai_fixture_dispatch SET expense=$3,floor_amount=$4,counted=$5,category=$6,evidence_version=$7,maximum_breached=maximum_breached OR $3>maximum WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, d.ID, *e.Amount, floor, contribution, category, e.Version)
			if err != nil {
				return ErrUnavailable
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_evidence(namespace,id,dispatch,binding,kind,data,actor) VALUES($1,$2,$3,$4,$5,$6,'fixture_reviewer')`, r.h.cfg.Namespace, e.ID, e.Dispatch, binding, e.Kind, raw)
		return safe(err)
	})
}

func (r *Reviewer) ReviewConservativeClosure(ctx context.Context, p auth.Principal, id string) error {
	if r == nil || !r.authorized || r.h == nil {
		return ErrDenied
	}
	return r.h.mutateDispatch(ctx, p, id, func(tx *sql.Tx, d Dispatch) error {
		if d.Category == "reviewed_conservative_maximum" {
			return nil
		}
		if d.State == "reserved" || d.State == "aborted" || d.Expense != nil || d.DurableBound == nil || d.IntentAt == nil {
			return ErrDenied
		}
		now, healthy := r.h.cfg.Clock.Sample()
		if !healthy || now.Before(*d.DurableBound) || now.Sub(*d.DurableBound) < 168*time.Hour {
			return ErrDenied
		}
		_, err := tx.ExecContext(ctx, `UPDATE ai_fixture_dispatch SET category='reviewed_conservative_maximum' WHERE namespace=$1 AND id=$2 AND expense IS NULL`, r.h.cfg.Namespace, id)
		return safe(err)
	})
}

// Compact drops detailed attribution only after reviewed conservative closure.
// Expense, original clocks, terminal replay identity and capacity remain intact.
func (r *Reviewer) Compact(ctx context.Context, p auth.Principal, id string) error {
	if r == nil || !r.authorized || r.h == nil {
		return ErrDenied
	}
	return r.h.mutateDispatch(ctx, p, id, func(tx *sql.Tx, d Dispatch) error {
		if d.Compacted {
			return nil
		}
		if d.Category != "reviewed_conservative_maximum" {
			return ErrDenied
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM ai_fixture_evidence WHERE namespace=$1 AND dispatch=$2`, r.h.cfg.Namespace, id); err != nil {
			return ErrUnavailable
		}
		_, err := tx.ExecContext(ctx, `UPDATE ai_fixture_dispatch SET compacted=true WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, id)
		return safe(err)
	})
}

// RetireNamespace is monotonic denial, not deletion of financial/replay rows or
// proof of a hosted fence. Marker pruning needs separate explicit retirement proof.
func (r *Reviewer) RetireNamespace(ctx context.Context, p auth.Principal) error {
	if r == nil || !r.authorized || r.h == nil {
		return ErrDenied
	}
	// Retirement is a denial transition and remains possible after entrypoints
	// have closed. It needs current operator fixture authentication, not an open
	// admission gateway. Repeating it cannot re-enable any authority.
	tx, err := r.h.db.BeginTx(ctx, nil)
	if err != nil {
		return ErrUnavailable
	}
	defer tx.Rollback()
	if err = accountstore.AuthorizeTransaction(ctx, tx, p); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE ai_fixture_namespace SET retired=true WHERE id=$1`, r.h.cfg.Namespace)
	if err != nil {
		return ErrUnavailable
	}
	if err := tx.Commit(); err != nil {
		return ErrUnavailable
	}
	r.h.cfg.Gate.Retire(r.h.cfg.Namespace)
	return nil
}

func (r *Reviewer) MarkFailed(ctx context.Context, p auth.Principal, id string) error {
	if r == nil || !r.authorized || r.h == nil {
		return ErrDenied
	}
	tx, err := r.h.begin(ctx, p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var invalid int
	if tx.QueryRowContext(ctx, `SELECT count(*) FROM ai_fixture_dispatch WHERE namespace=$1 AND action=$2 AND (state<>'observed' OR remote_state<>'terminal_proven')`, r.h.cfg.Namespace, id).Scan(&invalid) != nil {
		return ErrUnavailable
	}
	if invalid > 0 {
		return ErrDenied
	}
	res, err := tx.ExecContext(ctx, `UPDATE ai_fixture_action SET state='failed' WHERE namespace=$1 AND id=$2 AND owner=$3 AND state='recoverable'`, r.h.cfg.Namespace, id, p.AccountID)
	if err != nil {
		return ErrUnavailable
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrDenied
	}
	return safe(tx.Commit())
}

// AdvanceSyntheticPeriod carries every existing obligation. It changes only the
// fixture limit; no amount, remote slot or customer effect is reset.
func (r *Reviewer) AdvanceSyntheticPeriod(ctx context.Context, p auth.Principal, limit int64) error {
	if r == nil || !r.authorized || r.h == nil || limit <= 0 || limit > 1<<50 {
		return ErrDenied
	}
	tx, err := r.h.begin(ctx, p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE ai_fixture_scope SET limit_amount=$2 WHERE namespace=$1`, r.h.cfg.Namespace, limit)
	if err != nil {
		return ErrUnavailable
	}
	return safe(tx.Commit())
}

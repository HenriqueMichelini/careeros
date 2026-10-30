package aicontrol

import (
	"context"
	"database/sql"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

// ReserveCustomer exercises SYN fixture rights only. No real entitlement or
// payment API is read or mutated. One account lock orders all its reservations.
func (s *Session) ReserveCustomer(ctx context.Context) error {
	tx, err := s.h.begin(ctx, s.p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.checkCurrent(ctx, tx); err != nil {
		return err
	}
	var state string
	var reserved, effect bool
	if tx.QueryRowContext(ctx, `SELECT state,customer_reserved,customer_effect FROM ai_fixture_action WHERE namespace=$1 AND id=$2 AND owner=$3 FOR UPDATE`, s.h.cfg.Namespace, s.id, s.p.AccountID).Scan(&state, &reserved, &effect) != nil {
		return ErrDenied
	}
	if state != "recoverable" {
		return ErrDenied
	}
	if reserved || effect {
		return nil
	}
	var used, units int64
	if tx.QueryRowContext(ctx, `SELECT count(*) FROM ai_fixture_action WHERE namespace=$1 AND owner=$2 AND (customer_reserved OR customer_effect)`, s.h.cfg.Namespace, s.p.AccountID).Scan(&used) != nil {
		return ErrUnavailable
	}
	if tx.QueryRowContext(ctx, `SELECT units FROM ai_fixture_grant WHERE namespace=$1 AND owner=$2`, s.h.cfg.Namespace, s.p.AccountID).Scan(&units) != nil {
		return ErrUnavailable
	}
	if used >= units {
		return ErrCapacity
	}
	if err = s.checkCurrent(ctx, tx); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE ai_fixture_action SET customer_reserved=true WHERE namespace=$1 AND id=$2`, s.h.cfg.Namespace, s.id)
	if err != nil {
		return ErrUnavailable
	}
	return safe(tx.Commit())
}

// CustomerEvent is a synthetic state-model command, never a live result receipt.
// Complete+Recoverable explicitly represent fake result authority. Accept/export
// do not create effects. Customer deadlines are supplied independently from W.
type CustomerEvent struct {
	Kind, ResultRef       string
	Complete, Recoverable bool
	Deadline              *time.Time
}

func (r *Reviewer) Customer(ctx context.Context, p auth.Principal, id string, e CustomerEvent) error {
	if r == nil || !r.authorized || r.h == nil {
		return ErrDenied
	}
	if e.ResultRef != "" && !tokenLabel.MatchString(e.ResultRef) {
		return ErrDenied
	}
	tx, err := r.h.begin(ctx, p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state, ref string
	var reserved, effect bool
	if tx.QueryRowContext(ctx, `SELECT state,result_ref,customer_reserved,customer_effect FROM ai_fixture_action WHERE namespace=$1 AND id=$2 AND owner=$3 FOR UPDATE`, r.h.cfg.Namespace, id, p.AccountID).Scan(&state, &ref, &reserved, &effect) != nil {
		return ErrDenied
	}
	if e.Kind == "accept" || e.Kind == "export" {
		return nil
	}
	if state == "canceled" || state == "closed" || state == "deleted" {
		if e.Kind == "cancel" && state == "canceled" || e.Kind == "close" && state == "closed" || e.Kind == "delete" && state == "deleted" {
			return nil
		}
		return ErrDenied
	}
	switch e.Kind {
	case "cancel":
		if effect {
			return nil
		}
		state = "canceled"
		reserved = false
	case "close":
		now, healthy := r.h.cfg.Clock.Sample()
		if !healthy || e.Deadline == nil || now.Before(*e.Deadline) {
			return ErrDenied
		}
		if effect {
			return nil
		}
		state = "closed"
		reserved = false
	case "candidate":
		if !e.Complete || !e.Recoverable || e.ResultRef == "" || effect {
			return ErrDenied
		}
		if ref != "" && ref != e.ResultRef {
			return ErrConflict
		}
		ref = e.ResultRef
		state = "candidate"
	case "open":
		if state == "delivered" && effect && ref == e.ResultRef {
			return nil
		}
		if state != "candidate" || !reserved || !e.Complete || !e.Recoverable || ref == "" || ref != e.ResultRef {
			return ErrDenied
		}
		state = "delivered"
		reserved = false
		effect = true
	case "delete":
		if effect {
			return ErrDenied
		}
		state = "deleted"
		reserved = false
		ref = ""
	default:
		return ErrDenied
	}
	if err = accountstore.AuthorizeTransaction(ctx, tx, p); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE ai_fixture_action SET state=$3,result_ref=$4,customer_reserved=$5,customer_effect=$6 WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, id, state, ref, reserved, effect)
	if err != nil {
		return ErrUnavailable
	}
	return safe(tx.Commit())
}

// DeleteAction preserves all financial and replay rows. It stores no deleted
// career payload and never cascades from Profile/account deletion.
func (r *Reviewer) DeleteAction(ctx context.Context, p auth.Principal, id string) error {
	return r.Customer(ctx, p, id, CustomerEvent{Kind: "delete"})
}

var _ *sql.Tx

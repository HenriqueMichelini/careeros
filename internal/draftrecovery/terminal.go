package draftrecovery

import (
	"context"

	"professional-information-repo/internal/auth"
)

const TerminalPolicyVersion = "draft-terminal-resolution-local-20261122-v1"

// OpenManagedTerminal deliberately composes the local managed graph with finite
// pilot opening and terminal resolution. Ordinary managed/controlled constructors
// remain unchanged; no production startup selects this disposable composition.
func OpenManagedTerminal(ctx context.Context, dsn string, cfg Config) (*Store, error) {
	if cfg.Review != nil {
		return nil, ErrDenied
	}
	s, err := OpenManaged(ctx, dsn, cfg.Actions, cfg.BindingKey)
	if err != nil {
		return nil, err
	}
	s.cfg.Decisions = restrictivePilotDecisions(cfg.Decisions)
	s.cfg.Fault = cfg.Fault
	s.opening, s.terminal = true, true
	return s, nil
}

func (s *Store) TerminalEnabled() bool { return s != nil && s.managed && s.opening && s.terminal }

// Cancel acts only on an authenticated exact existing Action. It settles the
// customer outcome without claiming provider termination or releasing exposure.
func (s *Store) Cancel(ctx context.Context, p auth.Principal, key string, expected Action) error {
	if !s.TerminalEnabled() || !validToken(key) || !validToken(expected.Namespace) || !validToken(expected.ID) || expected.Workflow != "application_draft" {
		return ErrInvalid
	}
	if expected.Owner != p.AccountID {
		return ErrDenied
	}
	tx, err := s.begin(ctx, p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = s.openingGrant(ctx, tx, p); err != nil {
		return err
	}
	r, d, _, err := s.load(ctx, tx, p, key)
	if err != nil {
		return err
	}
	if r.Action != expected {
		if r.State == "closed" || r.State == "deleted" {
			if err = tx.Commit(); err != nil {
				return ErrDenied
			}
		}
		return ErrConflict
	}
	if r.State == "closed" || r.State == "deleted" {
		return tx.Commit()
	}
	if !d.Read {
		return ErrDenied
	}
	if err = s.fault("before_cancel_commit"); err != nil {
		return err
	}
	r, d, _, err = s.load(ctx, tx, p, key)
	if err != nil {
		return err
	}
	if r.State == "closed" || r.State == "deleted" {
		return tx.Commit()
	}
	if !d.Read {
		return ErrDenied
	}
	if d.Delivered {
		return tx.Commit()
	}
	result, err := tx.ExecContext(ctx, `UPDATE ai_fixture_action SET state='canceled'
 WHERE namespace=$1 AND id=$2 AND owner=$3 AND workflow='application_draft'
 AND NOT customer_effect AND state NOT IN ('canceled','closed','deleted','delivered')`, r.Action.Namespace, r.Action.ID, p.AccountID)
	if err != nil {
		return ErrDenied
	}
	if n, e := result.RowsAffected(); e != nil || n != 1 {
		return ErrDenied
	}
	// The existing terminal loader closes purpose access and releases this
	// customer reservation once. Provider rows and quantities are untouched.
	if _, _, _, err = s.load(ctx, tx, p, key); err != nil {
		return err
	}
	return tx.Commit()
}

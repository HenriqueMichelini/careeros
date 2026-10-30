package aicontrol

import (
	"context"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

// SyntheticRetirementProof is explicit fixture evidence, not an assertion about
// a hosted system. The numerical capability/backup horizons remain unselected.
type SyntheticRetirementProof struct {
	Namespace                                                                         string
	Epoch                                                                             int64
	CapabilitiesExpired, RestoreHorizonExpired, SendersExcluded, AllEntrypointsClosed bool
}

// PruneNamespaceMarkers removes only redundant binding/detail markers after a
// whole synthetic namespace is irrevocably denied. Namespace denial, financial
// allocations, counted expense, remote holds and original clock anchors remain.
// This command creates no production retention period or operational fence.
func (r *Reviewer) PruneNamespaceMarkers(ctx context.Context, p auth.Principal, proof SyntheticRetirementProof) error {
	if r == nil || !r.authorized || r.h == nil || proof.Namespace != r.h.cfg.Namespace || proof.Epoch <= 0 || !proof.CapabilitiesExpired || !proof.RestoreHorizonExpired || !proof.SendersExcluded || !proof.AllEntrypointsClosed {
		return ErrDenied
	}
	g := r.h.cfg.Gate
	if g == nil {
		return ErrDenied
	}
	g.mu.Lock()
	n := g.namespaces[proof.Namespace]
	closed := n != nil && n.retired && !n.open && len(n.active) == 0 && n.epoch == proof.Epoch
	g.mu.Unlock()
	if !closed {
		return ErrDenied
	}
	tx, err := r.h.db.BeginTx(ctx, nil)
	if err != nil {
		return ErrUnavailable
	}
	defer tx.Rollback()
	if err = accountstore.AuthorizeTransaction(ctx, tx, p); err != nil {
		return err
	}
	var epoch int64
	var retired bool
	if tx.QueryRowContext(ctx, `SELECT epoch,retired FROM ai_fixture_namespace WHERE id=$1 FOR UPDATE`, proof.Namespace).Scan(&epoch, &retired) != nil {
		return ErrUnavailable
	}
	if !retired || epoch != proof.Epoch {
		return ErrDenied
	}
	for _, statement := range []string{
		`DELETE FROM ai_fixture_evidence WHERE namespace=$1`,
		`DELETE FROM ai_fixture_child WHERE namespace=$1`,
		`DELETE FROM ai_fixture_stage WHERE namespace=$1`,
		`UPDATE ai_fixture_dispatch SET binding=decode(repeat('00',32),'hex'),compacted=true WHERE namespace=$1`,
		`UPDATE ai_fixture_action SET creation_key='retired_'||id,binding=decode(repeat('00',32),'hex'),profile_binding=NULL,compacted=true,result_ref='' WHERE namespace=$1`,
	} {
		if _, err = tx.ExecContext(ctx, statement, proof.Namespace); err != nil {
			return ErrUnavailable
		}
	}
	return safe(tx.Commit())
}

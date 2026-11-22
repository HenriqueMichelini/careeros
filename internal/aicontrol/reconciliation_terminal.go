package aicontrol

import (
	"context"
	"crypto/hmac"
	"database/sql"
)

const localTerminalProofVersion = "code-owned-fake-terminal-v1"

// IssueCodeOwnedFakeTerminal is the explicit proof seam for controlled local
// provider code with an independent migrator connection. Neither runtime nor
// reconciliation credentials can mint these proofs. It is not a live provider
// lifecycle contract: HTTP EOF, bills, timers and operator assertions never call
// this seam. All descendants of the named in-memory fake request are covered by
// the test-owned provider contract, not inferred from a successful HTTP status.
func (h *Harness) IssueCodeOwnedFakeTerminal(ctx context.Context, migrator *sql.DB, id, reference, request string) error {
	if h == nil || migrator == nil || !tokenLabel.MatchString(id) || !tokenLabel.MatchString(reference) || !metadataLabel.MatchString(request) {
		return ErrDenied
	}
	var owner bool
	if migrator.QueryRowContext(ctx, `SELECT current_user=pg_get_userbyid(relowner) FROM pg_class WHERE oid='ai_recon_terminal_proof'::regclass`).Scan(&owner) != nil || !owner {
		return ErrDenied
	}
	var provider string
	if migrator.QueryRowContext(ctx, `SELECT provider FROM ai_fixture_dispatch WHERE namespace=$1 AND id=$2 AND intent_at IS NOT NULL`, h.cfg.Namespace, id).Scan(&provider) != nil {
		return ErrDenied
	}
	r := h.Reconciliation()
	binding := r.digest("terminal", "exact-fake-request", struct{ Dispatch, Provider, Request, Version string }{id, provider, request, localTerminalProofVersion})
	_, err := migrator.ExecContext(ctx, `INSERT INTO ai_recon_terminal_proof(namespace,dispatch,reference,provider,version,request_binding) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, h.cfg.Namespace, id, reference, provider, localTerminalProofVersion, decodeDigest(binding))
	if err != nil {
		return ErrUnavailable
	}
	var old []byte
	var oldDispatch string
	if migrator.QueryRowContext(ctx, `SELECT dispatch,request_binding FROM ai_recon_terminal_proof WHERE namespace=$1 AND reference=$2`, h.cfg.Namespace, reference).Scan(&oldDispatch, &old) != nil || oldDispatch != id || !hmac.Equal(old, decodeDigest(binding)) {
		return ErrConflict
	}
	return nil
}

func (r *Reconciliation) validateTerminal(ctx context.Context, tx *sql.Tx, d Dispatch, e ReconciliationEvidence) error {
	if e.Usage != nil || e.Gross != nil || e.Difference != nil {
		return ErrDenied
	}
	var provider, dispatch, version string
	var binding []byte
	if tx.QueryRowContext(ctx, `SELECT provider,dispatch,version,request_binding FROM ai_recon_terminal_proof WHERE namespace=$1 AND reference=$2`, r.h.cfg.Namespace, e.Source).Scan(&provider, &dispatch, &version, &binding) != nil {
		return ErrDenied
	}
	wanted := r.digest("terminal", "exact-fake-request", struct{ Dispatch, Provider, Request, Version string }{d.ID, d.Provider, e.RequestID, localTerminalProofVersion})
	if provider != d.Provider || dispatch != d.ID || version != localTerminalProofVersion || !hmac.Equal(binding, decodeDigest(wanted)) {
		return ErrDenied
	}
	return nil
}

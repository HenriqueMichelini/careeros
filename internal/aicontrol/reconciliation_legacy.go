package aicontrol

import (
	"bytes"
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"errors"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

func (r *Reconciliation) legacyPayload(owner, kind string, raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, ErrDenied
	}
	return decodeDigest(r.digest(owner, "inherited-evidence/"+kind, value)), nil
}

// sealLegacyEvidence verifies inherited input-key signatures before either
// detail retirement or input-key retirement. The independent audit seal keeps
// those old conflict/replay obligations verifiable without destroying them or
// silently depending on a discarded input key. Missing original detail/key
// prevents retirement; an epoch bump or restored row supplies no substitute.
func (r *Reconciliation) sealLegacyEvidence(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT e.id,e.dispatch,e.kind,e.binding,e.data,a.owner,k.key_version FROM ai_fixture_evidence e JOIN ai_fixture_dispatch d ON d.namespace=e.namespace AND d.id=e.dispatch JOIN ai_fixture_action a ON a.namespace=d.namespace AND a.id=d.action JOIN ai_recon_action_key k ON k.namespace=a.namespace AND k.action=a.id WHERE e.namespace=$1 AND NOT EXISTS(SELECT 1 FROM ai_recon_legacy_marker m WHERE m.namespace=e.namespace AND m.id=e.id)`, r.h.cfg.Namespace)
	if err != nil {
		return ErrUnavailable
	}
	type inherited struct {
		id, dispatch, kind, owner, version string
		binding, raw                       []byte
	}
	var items []inherited
	for rows.Next() {
		var e inherited
		if rows.Scan(&e.id, &e.dispatch, &e.kind, &e.binding, &e.raw, &e.owner, &e.version) != nil {
			rows.Close()
			return ErrUnavailable
		}
		items = append(items, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return ErrUnavailable
	}
	keys := bindingKeys(r.h.cfg)
	for _, e := range items {
		key, ok := keys[e.version]
		if !ok || key == ([32]byte{}) {
			return ErrDenied
		}
		cfg := r.h.cfg
		cfg.BindingKey = key
		verifier := &Harness{cfg: cfg}
		purpose := "evidence"
		if e.kind == "conservative_closure" {
			purpose = "conservative-closure-audit"
		}
		original, err := verifier.fingerprint(e.owner, purpose, e.raw)
		if err != nil || !hmac.Equal(original, e.binding) {
			return ErrDenied
		}
		payload, err := r.legacyPayload(e.owner, e.kind, e.raw)
		if err != nil {
			return err
		}
		lineage := decodeDigest(r.digest(e.owner, "inherited-lineage", struct {
			ID, Dispatch, Kind, Version string
			Original, Payload           []byte
		}{e.id, e.dispatch, e.kind, e.version, e.binding, payload}))
		if _, err = tx.ExecContext(ctx, `INSERT INTO ai_recon_legacy_marker(namespace,id,dispatch,owner,kind,key_version,original_binding,payload_binding,lineage_binding) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, r.h.cfg.Namespace, e.id, e.dispatch, e.owner, e.kind, e.version, e.binding, payload, lineage); err != nil {
			return ErrUnavailable
		}
	}
	return nil
}

func (r *Reviewer) legacyEvidenceReplay(ctx context.Context, p auth.Principal, e Evidence, raw []byte) (bool, error) {
	tx, err := r.h.db.BeginTx(ctx, nil)
	if err != nil {
		return false, ErrUnavailable
	}
	defer tx.Rollback()
	if err = accountstore.AuthorizeTransaction(ctx, tx, p); err != nil {
		return false, err
	}
	var owner, dispatch, kind, version string
	var original, payload, lineage []byte
	err = tx.QueryRowContext(ctx, `SELECT owner,dispatch,kind,key_version,original_binding,payload_binding,lineage_binding FROM ai_recon_legacy_marker WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, e.ID).Scan(&owner, &dispatch, &kind, &version, &original, &payload, &lineage)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, ErrUnavailable
	}
	if owner != p.AccountID {
		return true, ErrDenied
	}
	audit := &Reconciliation{h: r.h}
	expected := decodeDigest(audit.digest(owner, "inherited-lineage", struct {
		ID, Dispatch, Kind, Version string
		Original, Payload           []byte
	}{e.ID, dispatch, kind, version, original, payload}))
	candidate, err := audit.legacyPayload(owner, e.Kind, raw)
	if err != nil || dispatch != e.Dispatch || kind != e.Kind || !hmac.Equal(lineage, expected) || !hmac.Equal(payload, candidate) {
		return true, ErrConflict
	}
	return true, accountstore.AuthorizeTransaction(ctx, tx, p)
}

func (r *Reviewer) legacyInputFingerprint(ctx context.Context, p auth.Principal, dispatch, purpose string, raw []byte) ([]byte, error) {
	var version string
	if r.h.db.QueryRowContext(ctx, `SELECT k.key_version FROM ai_fixture_dispatch d JOIN ai_fixture_action a ON a.namespace=d.namespace AND a.id=d.action JOIN ai_recon_action_key k ON k.namespace=a.namespace AND k.action=a.id WHERE d.namespace=$1 AND d.id=$2 AND a.owner=$3`, r.h.cfg.Namespace, dispatch, p.AccountID).Scan(&version) != nil {
		return nil, ErrDenied
	}
	key, ok := bindingKeys(r.h.cfg)[version]
	if !ok || key == ([32]byte{}) {
		return nil, ErrDenied
	}
	cfg := r.h.cfg
	cfg.BindingKey = key
	verifier := &Harness{cfg: cfg}
	return verifier.fingerprint(p.AccountID, purpose, raw)
}

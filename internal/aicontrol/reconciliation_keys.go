package aicontrol

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"

	"professional-information-repo/internal/auth"
)

func bindingKeys(cfg SyntheticConfig) map[string][32]byte {
	keys := map[string][32]byte{}
	for v, k := range cfg.BindingVerificationKeys {
		keys[v] = k
	}
	keys[cfg.BindingKeyVersion] = cfg.BindingKey
	return keys
}

func installBindingVersions(ctx context.Context, tx *sql.Tx, cfg SyntheticConfig) error {
	keys := bindingKeys(cfg)
	for v, k := range keys {
		if !tokenLabel.MatchString(v) || k == ([32]byte{}) {
			return ErrDenied
		}
		digest := sha256.Sum256(k[:])
		if _, err := tx.ExecContext(ctx, `INSERT INTO ai_recon_key(namespace,version,purpose,verifier_digest) VALUES($1,$2,'input_binding',$3) ON CONFLICT DO NOTHING`, cfg.Namespace, v, digest[:]); err != nil {
			return ErrUnavailable
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT version,verifier_digest,retired FROM ai_recon_key WHERE namespace=$1 AND purpose='input_binding'`, cfg.Namespace)
	if err != nil {
		return ErrUnavailable
	}
	for rows.Next() {
		var v string
		var digest []byte
		var retired bool
		if rows.Scan(&v, &digest, &retired) != nil {
			rows.Close()
			return ErrUnavailable
		}
		key, present := keys[v]
		got := sha256.Sum256(key[:])
		if (!retired && (!present || !hmac.Equal(got[:], digest))) || (v == cfg.BindingKeyVersion && retired) {
			rows.Close()
			return ErrDenied
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return ErrUnavailable
	}
	// All inherited rows predate versioned input rotation and use input_v1.
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_recon_action_key(namespace,action,key_version) SELECT a.namespace,a.id,'input_v1' FROM ai_fixture_action a WHERE a.namespace=$1 AND NOT EXISTS(SELECT 1 FROM ai_recon_action_key k WHERE k.namespace=a.namespace AND k.action=a.id)`, cfg.Namespace)
	return safe(err)
}

func (h *Harness) actionBindingHarness(ctx context.Context, tx *sql.Tx, p auth.Principal, creation string) (*Harness, error) {
	version := h.cfg.BindingKeyVersion
	var old string
	err := tx.QueryRowContext(ctx, `SELECT k.key_version FROM ai_fixture_action a JOIN ai_recon_action_key k ON k.namespace=a.namespace AND k.action=a.id WHERE a.namespace=$1 AND a.owner=$2 AND a.creation_key=$3`, h.cfg.Namespace, p.AccountID, creation).Scan(&old)
	if err == nil {
		version = old
	} else if err != sql.ErrNoRows {
		return nil, ErrUnavailable
	}
	keys := bindingKeys(h.cfg)
	key, ok := keys[version]
	if !ok || key == ([32]byte{}) {
		return nil, ErrDenied
	}
	digest := sha256.Sum256(key[:])
	var expected []byte
	var retired bool
	if tx.QueryRowContext(ctx, `SELECT verifier_digest,retired FROM ai_recon_key WHERE namespace=$1 AND version=$2 AND purpose='input_binding'`, h.cfg.Namespace, version).Scan(&expected, &retired) != nil || retired || !hmac.Equal(expected, digest[:]) {
		return nil, ErrDenied
	}
	if version == h.cfg.BindingKeyVersion {
		return h, nil
	}
	cfg := h.cfg
	cfg.BindingKey = key
	cfg.BindingKeyVersion = version
	return &Harness{db: h.db, reviewerDB: h.reviewerDB, supportRole: h.supportRole, cfg: cfg, origin: h, hooks: map[string]func() error{}}, nil
}

// WithLocalCheckpointVerifier supplies independent code-owned evidence for all
// old entrypoints/senders, current authority, result capabilities and recovery
// copies. There is no default verifier, request boolean or DB epoch shortcut.
func (r *Reconciliation) WithLocalCheckpointVerifier(verify func(context.Context, string, string) error) *Reconciliation {
	return &Reconciliation{h: r.h, checkpointVerifier: verify}
}

type KeyRetirement struct{ Version, Checkpoint, Actor string }

func (r *Reconciliation) RetireInputKey(ctx context.Context, p auth.Principal, version, checkpoint string) (KeyRetirement, error) {
	out := KeyRetirement{Version: version, Checkpoint: checkpoint, Actor: p.AccountID}
	if r == nil || r.h == nil || r.runtime || r.checkpointVerifier == nil || !tokenLabel.MatchString(version) || !tokenLabel.MatchString(checkpoint) || r.h.cfg.Gate == nil {
		return out, ErrDenied
	}
	// Retire is independent and monotonic. Drain includes active callbacks,
	// response bodies and protected writes; a database generation bump is not it.
	g := r.h.cfg.Gate
	g.mu.Lock()
	n := g.namespaces[r.h.cfg.Namespace]
	closed := n != nil && n.retired && !n.open && len(n.active) == 0
	g.mu.Unlock()
	if !closed {
		return out, ErrDenied
	}
	if err := r.checkpointVerifier(ctx, r.h.cfg.Namespace, checkpoint); err != nil {
		return out, ErrDenied
	}
	tx, err := r.h.reviewerDB.BeginTx(ctx, nil)
	if err != nil {
		return out, ErrUnavailable
	}
	defer tx.Rollback()
	if err = r.authorize(ctx, tx, p); err != nil {
		return out, err
	}
	var retired bool
	if tx.QueryRowContext(ctx, `SELECT retired FROM ai_fixture_namespace WHERE id=$1 FOR UPDATE`, r.h.cfg.Namespace).Scan(&retired) != nil || !retired {
		return out, ErrDenied
	}
	if err = r.sealLegacyEvidence(ctx, tx); err != nil {
		return out, err
	}
	var old string
	if tx.QueryRowContext(ctx, `SELECT checkpoint_reference FROM ai_recon_key WHERE namespace=$1 AND version=$2 AND purpose='input_binding' FOR UPDATE`, r.h.cfg.Namespace, version).Scan(&old) != nil {
		return out, ErrDenied
	}
	if old != "" {
		if old != checkpoint {
			return out, ErrConflict
		}
		return out, r.authorize(ctx, tx, p)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE ai_recon_key SET retired=true,checkpoint_reference=$3,retirement_actor=$4,reviewed_at=clock_timestamp() WHERE namespace=$1 AND version=$2 AND purpose='input_binding'`, r.h.cfg.Namespace, version, checkpoint, p.AccountID); err != nil {
		return out, ErrUnavailable
	}
	if err = r.authorize(ctx, tx, p); err != nil {
		return out, err
	}
	return out, safe(tx.Commit())
}

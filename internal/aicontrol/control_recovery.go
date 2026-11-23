package aicontrol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

// SyntheticControlRecovery is a non-restored, code-owned local checkpoint
// registry. It is never constructed by ordinary startup, HTTP input or CLI
// labels. It observes the existing restricted ledger; it settles no obligation.
type SyntheticControlRecovery struct {
	mu          sync.Mutex
	store       *accountstore.Store
	harnesses   []*Harness
	checkpoints map[string]controlCheckpoint
}
type controlCheckpoint struct {
	evidence   accountstore.ControlRecoveryEvidence
	digests    []string
	epochs     []int64
	leases     []gateLease
	exclusions map[*SyntheticGate]syntheticExclusion
	activated  bool
}

func NewSyntheticControlRecovery(ctx context.Context, store *accountstore.Store, harnesses ...*Harness) (*SyntheticControlRecovery, error) {
	if store == nil || len(harnesses) == 0 {
		return nil, auth.ErrForbidden
	}
	seen := map[*Harness]bool{}
	for _, h := range harnesses {
		if h == nil || h.cfg.Gate == nil || h.db == nil || seen[h] {
			return nil, auth.ErrForbidden
		}
		seen[h] = true
		if err := store.CheckControlDatabase(ctx, h.db); err != nil {
			return nil, err
		}
	}
	return &SyntheticControlRecovery{store: store, harnesses: append([]*Harness(nil), harnesses...), checkpoints: map[string]controlCheckpoint{}}, nil
}

// Prepare records Q and a conservative D/R observation. If the independent
// gateway is closed/mismatched, actual local whole-gateway exclusion is also
// required; a restored database generation cannot create that capability.
func (r *SyntheticControlRecovery) Prepare(ctx context.Context, p auth.Principal, command accountstore.ControlCommand) (string, error) {
	if r == nil || command.Stop {
		return "", auth.ErrForbidden
	}
	status, err := r.store.LocalControls(nil).Status(ctx, p, command.AccountID)
	if err != nil {
		return "", err
	}
	if status.Version != command.ExpectedVersion || command.Control == "inference" && !status.InferenceStopped || command.Control == "invitations" && !status.InvitationStopped || command.Control == "compromise" && !status.Compromised {
		return "", auth.ErrForbidden
	}
	if err := r.namespaceInventory(ctx); err != nil {
		return "", err
	}
	checkpoint := controlCheckpoint{exclusions: map[*SyntheticGate]syntheticExclusion{}}
	whole := false
	for _, h := range r.harnesses {
		var epoch int64
		var retired bool
		if err = h.reviewerDB.QueryRowContext(ctx, `SELECT epoch,retired FROM ai_fixture_namespace WHERE id=$1`, h.cfg.Namespace).Scan(&epoch, &retired); err != nil {
			return "", auth.ErrUnavailable
		}
		if retired {
			return "", auth.ErrForbidden
		}
		checkpoint.epochs = append(checkpoint.epochs, epoch)
		lease, _ := h.cfg.Gate.capture(h.cfg.Namespace)
		checkpoint.leases = append(checkpoint.leases, lease)
		if !h.cfg.Gate.permits(h.cfg.Namespace, epoch) {
			whole = true
		}
		digest, e := h.controlLedgerDigest(ctx)
		if e != nil {
			return "", e
		}
		checkpoint.digests = append(checkpoint.digests, digest)
	}
	if whole {
		for index, h := range r.harnesses {
			proof, e := h.cfg.Gate.wholeSystemExclusion()
			if e != nil {
				return "", auth.ErrForbidden
			}
			if proof.epochs[h.cfg.Namespace] != checkpoint.epochs[index] {
				return "", auth.ErrForbidden
			}
			checkpoint.exclusions[h.cfg.Gate] = proof
		}
	}
	reference := opaque()
	if reference == "" {
		return "", auth.ErrUnavailable
	}
	checkpoint.evidence = accountstore.ControlRecoveryEvidence{Reference: reference, ActorID: p.AccountID, Control: command.Control, AccountID: command.AccountID, StopVersion: status.Version, Recovery: opaque(), Reconciliation: opaque(), Generation: status.SystemGeneration, WholeSystemExcluded: whole}
	if command.Control == "inference" || command.Control == "invitations" {
		checkpoint.evidence.OutageEpisode, err = r.store.ActiveOperationsOutage(ctx)
		if err != nil {
			return "", err
		}
	}
	observation, _ := json.Marshal(struct {
		Digests []string
		Epochs  []int64
	}{checkpoint.digests, checkpoint.epochs})
	digest := sha256.Sum256(observation)
	checkpoint.evidence.ObservationDigest = hex.EncodeToString(digest[:])
	if whole {
		if status.SystemGeneration >= 9223372036854775807 {
			return "", auth.ErrForbidden
		}
		checkpoint.evidence.Generation++
		checkpoint.evidence.Exclusion = opaque()
	}
	r.mu.Lock()
	r.checkpoints[reference] = checkpoint
	r.mu.Unlock()
	return reference, nil
}

func (r *SyntheticControlRecovery) checkpoint(command accountstore.ControlCommand) (controlCheckpoint, bool) {
	if r == nil {
		return controlCheckpoint{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.checkpoints[command.RecoveryReference]
	return v, ok
}

func (r *SyntheticControlRecovery) VerifyControlRecovery(ctx context.Context, p auth.Principal, command accountstore.ControlCommand) (accountstore.ControlRecoveryEvidence, error) {
	checkpoint, ok := r.checkpoint(command)
	if !ok {
		return accountstore.ControlRecoveryEvidence{}, auth.ErrForbidden
	}
	status, err := r.store.LocalControls(nil).Status(ctx, p, command.AccountID)
	if err != nil {
		return accountstore.ControlRecoveryEvidence{}, err
	}
	if status.Version != checkpoint.evidence.StopVersion {
		return accountstore.ControlRecoveryEvidence{}, auth.ErrForbidden
	}
	if err = r.RecheckControlRecovery(ctx, p, command, checkpoint.evidence); err != nil {
		return accountstore.ControlRecoveryEvidence{}, err
	}
	return checkpoint.evidence, nil
}

// Recheck makes only direct restricted reads. The caller holds the exclusive
// control and namespace fence, so reacquiring an admission lock would deadlock.
func (r *SyntheticControlRecovery) RecheckControlRecovery(ctx context.Context, p auth.Principal, command accountstore.ControlCommand, evidence accountstore.ControlRecoveryEvidence) error {
	checkpoint, ok := r.checkpoint(command)
	if !ok || checkpoint.evidence != evidence || evidence.ActorID != p.AccountID || evidence.Control != command.Control || evidence.AccountID != command.AccountID || evidence.StopVersion != command.ExpectedVersion {
		return auth.ErrForbidden
	}
	if command.Control == "inference" || command.Control == "invitations" {
		episode, err := r.store.ActiveOperationsOutage(ctx)
		if err != nil {
			return err
		}
		if episode != evidence.OutageEpisode {
			return auth.ErrForbidden
		}
	}
	if err := r.namespaceInventory(ctx); err != nil {
		return err
	}
	for index, h := range r.harnesses {
		digest, err := h.controlLedgerDigest(ctx)
		if err != nil {
			return err
		}
		if digest != checkpoint.digests[index] {
			return auth.ErrForbidden
		}
		var epoch int64
		var retired bool
		if err = h.reviewerDB.QueryRowContext(ctx, `SELECT epoch,retired FROM ai_fixture_namespace WHERE id=$1`, h.cfg.Namespace).Scan(&epoch, &retired); err != nil {
			return auth.ErrUnavailable
		}
		if retired || epoch != checkpoint.epochs[index] {
			return auth.ErrForbidden
		}
		if evidence.WholeSystemExcluded {
			if !checkpoint.exclusions[h.cfg.Gate].valid() {
				return auth.ErrForbidden
			}
		} else if checkpoint.leases[index].allowed() != nil {
			return auth.ErrForbidden
		}
	}
	return nil
}

// A whole-schema generation cannot omit an installed namespace. All concrete
// local gateway instances must be supplied by the code-owned composition.
func (r *SyntheticControlRecovery) namespaceInventory(ctx context.Context) error {
	expected := map[string]bool{}
	for _, h := range r.harnesses {
		expected[h.cfg.Namespace] = true
	}
	rows, err := r.harnesses[0].reviewerDB.QueryContext(ctx, `SELECT id FROM ai_fixture_namespace ORDER BY id`)
	if err != nil {
		return auth.ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			return auth.ErrUnavailable
		}
		if !expected[id] {
			return auth.ErrForbidden
		}
		delete(expected, id)
	}
	if rows.Err() != nil {
		return auth.ErrUnavailable
	}
	if len(expected) != 0 {
		return auth.ErrForbidden
	}
	return nil
}

func (h *Harness) controlLedgerDigest(ctx context.Context) (string, error) {
	var contained bool
	if err := h.reviewerDB.QueryRowContext(ctx, `SELECT COALESCE((SELECT stopped FROM ai_recon_stop WHERE namespace=$1),false)`, h.cfg.Namespace).Scan(&contained); err != nil {
		return "", auth.ErrUnavailable
	}
	if contained {
		return "", auth.ErrForbidden
	}
	var inconsistent bool
	if err := h.reviewerDB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_fixture_scope s WHERE s.namespace=$1 AND (s.counted<0 OR s.counted<COALESCE((SELECT sum(d.counted) FROM ai_fixture_allocation a JOIN ai_fixture_dispatch d ON d.namespace=a.namespace AND d.id=a.dispatch WHERE a.namespace=s.namespace AND a.scope=s.scope),0)))`, h.cfg.Namespace).Scan(&inconsistent); err != nil {
		return "", auth.ErrUnavailable
	}
	if inconsistent {
		return "", auth.ErrForbidden
	}
	// These existing restricted tables contain no career/credential payload.
	// Hash only, retaining no duplicate evidence/linkage dataset in this registry.
	hash := sha256.New()
	for _, table := range []string{"ai_fixture_action", "ai_fixture_dispatch", "ai_fixture_scope", "ai_fixture_allocation", "ai_recon_state"} {
		rows, err := h.reviewerDB.QueryContext(ctx, "SELECT row_to_json(t)::text FROM "+table+" t WHERE namespace=$1 ORDER BY row_to_json(t)::text", h.cfg.Namespace)
		if err != nil {
			return "", auth.ErrUnavailable
		}
		for rows.Next() {
			var row string
			if err = rows.Scan(&row); err != nil {
				break
			}
			raw, _ := json.Marshal([]string{table, row})
			hash.Write(raw)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return "", auth.ErrUnavailable
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Complete opens only the independently verified local gateway after the
// durable generation advance. It cannot be used before Apply or on stale proof.
func (r *SyntheticControlRecovery) Complete(ctx context.Context, p auth.Principal, reference string) error {
	r.mu.Lock()
	checkpoint, ok := r.checkpoints[reference]
	r.mu.Unlock()
	if !ok || checkpoint.evidence.ActorID != p.AccountID {
		return auth.ErrForbidden
	}
	return r.store.LocalControls(nil).WithStatus(ctx, p, checkpoint.evidence.AccountID, func(status accountstore.ControlStatus) error {
		if status.Version != checkpoint.evidence.StopVersion+1 || status.SystemGeneration != checkpoint.evidence.Generation {
			return auth.ErrForbidden
		}
		if status.InferenceStopped && checkpoint.evidence.Control == "inference" || status.InvitationStopped && checkpoint.evidence.Control == "invitations" || status.Compromised && checkpoint.evidence.Control == "compromise" {
			return auth.ErrForbidden
		}
		r.mu.Lock()
		current := r.checkpoints[reference]
		r.mu.Unlock()
		if current.activated {
			return nil
		}
		if checkpoint.evidence.WholeSystemExcluded {
			epochs := map[*SyntheticGate]map[string]int64{}
			for index, h := range r.harnesses {
				var epoch int64
				if err := h.reviewerDB.QueryRowContext(ctx, `SELECT epoch FROM ai_fixture_namespace WHERE id=$1`, h.cfg.Namespace).Scan(&epoch); err != nil {
					return auth.ErrUnavailable
				}
				if epoch != checkpoint.epochs[index]+1 {
					return auth.ErrForbidden
				}
				if epochs[h.cfg.Gate] == nil {
					epochs[h.cfg.Gate] = map[string]int64{}
				}
				epochs[h.cfg.Gate][h.cfg.Namespace] = epoch
				if err := h.fault("before_control_gateway_open"); err != nil {
					return err
				}
			}
			if err := openSyntheticExclusions(checkpoint.exclusions, epochs); err != nil {
				return auth.ErrForbidden
			}
		}
		r.mu.Lock()
		current = r.checkpoints[reference]
		current.activated = true
		r.checkpoints[reference] = current
		r.mu.Unlock()
		return nil
	})
}

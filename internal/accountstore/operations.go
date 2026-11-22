package accountstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"professional-information-repo/internal/auth"
)

// OperationsRiskSource is installed by source composition, never a customer
// request. Its figures retain the original ledger's units and contribution.
type OperationsRiskSource interface {
	OperationsSample(context.Context) (OperationsRiskSample, error)
}

// OperationsRiskBinding is implemented by the local managed source. It binds
// this decision journal to the same physical store as actual fake dispatch.
type OperationsRiskBinding interface {
	CheckOperationsStore(context.Context, *Store) error
	BindOperations(*Operations)
}

type OperationsCap struct {
	Key, Scope, Policy, Currency string
	Counted, Limit               int64
}
type OperationsRiskSample struct {
	At        time.Time
	Namespace string
	Complete  bool
	Caps      []OperationsCap
	Events    []OperationsRiskEvent
	Figures   OperationsFigures
}
type OperationsRiskEvent struct {
	Key, Kind, Policy string
	At                time.Time
	Exposure, Maximum int64
}
type OperationsFigures struct {
	At                   time.Time `json:"observedAt"`
	Currency             string    `json:"currency"`
	Counted              int64     `json:"countedExposure"`
	ObservedUsage        int64     `json:"observedCompatibleUsageEstimate"`
	SupportedEstimate    int64     `json:"supportedEstimate"`
	ConservativeMaximum  int64     `json:"reviewedConservativeMaximum"`
	AttributableBills    int64     `json:"attributableBills"`
	UnknownDifference    int64     `json:"unknownDifference"`
	PossibleRemote       int64     `json:"possibleRemote"`
	LocalActiveActions   int64     `json:"localActiveActions"`
	ExhaustedDraftGrants int64     `json:"exhaustedDraftGrants"`
}
type OperationsConfig struct{ Build, Owner string }

type OperationsIncident struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Scope     string    `json:"scope,omitempty"`
	Policy    string    `json:"policy,omitempty"`
	Currency  string    `json:"currency,omitempty"`
	Exposure  int64     `json:"exposure,omitempty"`
	Cap       int64     `json:"cap,omitempty"`
	Threshold int64     `json:"threshold,omitempty"`
	EventAt   time.Time `json:"eventAt"`
	Owner     string    `json:"owner"`
	Response  string    `json:"response"`
	State     string    `json:"state"`
	Severity  string    `json:"severity"`
	Revision  int       `json:"revision"`
	Delivery  string    `json:"delivery"`
}
type OperationsStatus struct {
	Incidents   []OperationsIncident `json:"incidents"`
	Figures     *OperationsFigures   `json:"figures,omitempty"`
	Persistence string               `json:"persistence"`
	Backup      string               `json:"backup"`
}

type Operations struct {
	store   *Store
	cfg     OperationsConfig
	source  OperationsRiskSource
	journal *OperationsJournal
}

// LocalOperationsObserver is a code-owned composition seam. It does not
// authenticate or expose operator records to the request.
func (s *Store) LocalOperationsObserver() *Operations {
	return s.operations.Load()
}

func (s *Store) LocalOperations(cfg OperationsConfig, source OperationsRiskSource, journal *OperationsJournal) (*Operations, error) {
	if s == nil || source == nil || journal == nil || cfg.Owner != "Henrique" || !validOperation(cfg.Build) {
		return nil, ErrValidation
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	binding, ok := source.(OperationsRiskBinding)
	if !ok {
		return nil, ErrValidation
	}
	if err := binding.CheckOperationsStore(ctx, s); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, unavailable(err)
	}
	defer tx.Rollback()
	if err = localControlDatabase(ctx, tx); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", controlFence); err != nil {
		return nil, unavailable(err)
	}
	handle := journal.binding([]byte("operation-authority\x00" + journal.id))
	var persisted string
	err = tx.QueryRowContext(ctx, `UPDATE careeros_control_state SET operations_binding=$1 WHERE singleton
	  AND (operations_binding='' OR operations_binding=$1) RETURNING operations_binding`, handle).Scan(&persisted)
	if err != nil || persisted != handle {
		return nil, auth.ErrUnavailable
	}
	if err = tx.Commit(); err != nil {
		return nil, unavailable(err)
	}
	ops := &Operations{store: s, cfg: cfg, source: source, journal: journal}
	s.operations.Store(ops)
	binding.BindOperations(ops)
	return ops, nil
}

func (o *Operations) Inspect(ctx context.Context) error {
	sample, err := o.source.OperationsSample(ctx)
	if err != nil {
		return errors.Join(err, o.ObserveFault(context.WithoutCancel(ctx), OperationsDatabaseFault))
	}
	if !sample.Complete || sample.At.IsZero() || !validOperation(sample.Namespace) {
		return errors.Join(auth.ErrUnavailable, o.ObserveFault(ctx, OperationsAuthorityFault))
	}
	if err = o.observeDraftGrants(ctx, &sample.Figures); err != nil {
		return errors.Join(err, o.ObserveFault(context.WithoutCancel(ctx), OperationsAuthorityFault))
	}
	if err = o.recordSample(ctx, sample); err != nil {
		return err
	}
	return errors.Join(o.synchronizeContainment(ctx), o.Deliver(ctx))
}

func (o *Operations) observeDraftGrants(ctx context.Context, figures *OperationsFigures) error {
	tx, err := o.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	hasDrafts, err := installed(ctx, tx, "careeros_draft_allowance")
	if err != nil || !hasDrafts {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM careeros_pilot_access p WHERE p.state='active'
	 AND p.expires_at>$1 AND p.age_eligible AND NOT p.invitation_revoked AND NOT p.retrieval_held
	 AND (SELECT count(*) FROM careeros_draft_allowance a WHERE a.owner=p.account_id
	 AND (a.state='delivered' OR a.state='reserved' AND a.deadline>$1))>=p.draft_allowance`, figures.At).Scan(&figures.ExhaustedDraftGrants)
	return unavailable(err)
}

func (o *Operations) recordSample(ctx context.Context, sample OperationsRiskSample) error {
	for _, cap := range sample.Caps {
		if cap.Limit <= 0 || cap.Counted < 0 || cap.Key == "" {
			return errors.Join(auth.ErrUnavailable, o.ObserveFault(ctx, OperationsAuthorityFault))
		}
	}
	return o.journal.transaction(ctx, true, func(state *operationsJournalState) error {
		if state.Figures == nil || !sample.At.Before(state.Figures.At) {
			o.recordCaps(state, sample)
			state.Figures = &sample.Figures
			state.Persistence = "available_at_observation"
		}
		for _, event := range sample.Events {
			incident := OperationsIncident{Kind: event.Kind, Policy: event.Policy, Currency: sample.Figures.Currency,
				Exposure: event.Exposure, Cap: event.Maximum, EventAt: event.At, Owner: o.cfg.Owner, State: "unacknowledged"}
			switch event.Kind {
			case "unknown_dispatch":
				incident.Response, incident.Severity = "retain_exposure_and_capacity_and_review", "warning"
			case "maximum_breach":
				incident.Response, incident.Severity = "contain_and_reconcile_original_dispatch_bound", "critical"
			default:
				return auth.ErrUnavailable
			}
			o.journal.upsert(state, sample.Namespace+"/"+event.Kind+"/"+event.Key, incident, false)
		}
		return nil
	})
}

func (o *Operations) recordCaps(state *operationsJournalState, sample OperationsRiskSample) {
	if state.Rolling == nil {
		state.Rolling = map[string]time.Time{}
	}
	for _, cap := range sample.Caps {
		// L-floor(L/5) = ceil(4L/5), with no multiplication or float.
		threshold := cap.Limit - cap.Limit/5
		key := sample.Namespace + "/" + cap.Policy + "/" + cap.Scope + "/" + cap.Key
		if strings.HasSuffix(cap.Scope, "rolling24h") {
			index := o.journal.binding([]byte("rolling\x00" + key))
			if cap.Counted < threshold {
				delete(state.Rolling, index)
				continue
			}
			if state.Rolling[index].IsZero() {
				state.Rolling[index] = sample.At
			}
			key += "/" + state.Rolling[index].UTC().Format(time.RFC3339Nano)
		}
		if cap.Counted < threshold {
			continue
		}
		incident := OperationsIncident{Kind: "spend_warning", Scope: cap.Scope, Policy: cap.Policy, Currency: cap.Currency,
			Exposure: cap.Counted, Cap: cap.Limit, Threshold: threshold, EventAt: sample.At, Owner: o.cfg.Owner,
			Response: "review_conservative_exposure", State: "unacknowledged", Severity: "warning"}
		o.journal.upsert(state, "spend_warning/"+key, incident, false)
		if cap.Counted > cap.Limit {
			incident.Kind, incident.Severity, incident.Response = "spend_breach", "critical", "contain_and_reconcile_spend_bound"
			o.journal.upsert(state, "spend_breach/"+key, incident, false)
		}
	}
}
func (o *Operations) Deliver(ctx context.Context) error { return o.journal.deliver(ctx) }

func (s *Store) BootstrapLocalOperationsRole(ctx context.Context, identity auth.IdentityRef, role string) error {
	if role != "owner" && role != "support" {
		return ErrValidation
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if err = localControlDatabase(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(72481331)"); err != nil {
		return unavailable(err)
	}
	var id string
	if err = tx.QueryRowContext(ctx, `SELECT account_id FROM careeros_identities WHERE provider=$1 AND issuer=$2 AND subject=$3`, []byte(identity.Provider), []byte(identity.Issuer), []byte(identity.Subject)).Scan(&id); err != nil {
		return auth.ErrForbidden
	}
	if role == "owner" {
		var allowed bool
		if err = tx.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM careeros_operations_roles WHERE role='owner' AND account_id<>$1)`, id).Scan(&allowed); err != nil {
			return unavailable(err)
		}
		if !allowed {
			return auth.ErrForbidden
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_operations_roles(account_id,role) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, role); err != nil {
		return unavailable(err)
	}
	return unavailable(tx.Commit())
}

func operationsActor(ctx context.Context, tx *sql.Tx, p auth.Principal, role string) error {
	if role == "owner" {
		if err := authorizedAccount(ctx, tx, p); err != nil {
			if err = authorizeRestrictedRights(ctx, tx, p); err != nil {
				return err
			}
		}
	} else if err := authorizedAccount(ctx, tx, p); err != nil {
		return err
	}
	var allowed bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_operations_roles WHERE account_id=$1 AND role=$2)`, p.AccountID, role).Scan(&allowed); err != nil {
		return unavailable(err)
	}
	if !allowed {
		return auth.ErrForbidden
	}
	return nil
}

func (o *Operations) WithOwnerStatus(ctx context.Context, p auth.Principal, write func(OperationsStatus) error) error {
	if write == nil {
		return ErrValidation
	}
	tx, err := o.store.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if err = LockControlTransaction(ctx, tx); err != nil {
		return err
	}
	if err = operationsActor(ctx, tx, p, "owner"); err != nil {
		return err
	}
	status, err := o.journal.status(ctx)
	if err != nil {
		return err
	}
	if err = operationsActor(ctx, tx, p, "owner"); err != nil {
		return err
	}
	return write(status)
}
func (o *Operations) OwnerStatus(ctx context.Context, p auth.Principal) (OperationsStatus, error) {
	var status OperationsStatus
	err := o.WithOwnerStatus(ctx, p, func(v OperationsStatus) error { status = v; return nil })
	return status, err
}

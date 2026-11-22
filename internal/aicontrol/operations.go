package aicontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

type operationsSource struct{ h *Harness }

// LocalOperationsSource exposes only the code-owned numeric observer, never a
// customer-selected amount or an ordinary-support view of ledger identities.
func (h *Harness) LocalOperationsSource() accountstore.OperationsRiskSource {
	return &operationsSource{h: h}
}

// LocalOperationsSource uses the existing restricted reconciliation reader
// on an installed selected qualification namespace without running a workflow.
func (r *Reconciliation) LocalOperationsSource(ctx context.Context) (accountstore.OperationsRiskSource, error) {
	var cap int64
	if r.h.reviewerDB.QueryRowContext(ctx, `SELECT limit_amount FROM ai_fixture_scope
	  WHERE namespace=$1 AND scope='global'`, r.h.cfg.Namespace).Scan(&cap) != nil ||
		cap != selectedQualificationEnvelope().GlobalTotal {
		return nil, ErrDenied
	}
	cfg := r.h.cfg
	cfg.Qualification = true
	return (&Harness{reviewerDB: r.h.reviewerDB, cfg: cfg}).LocalOperationsSource(), nil
}

func (s *operationsSource) CheckOperationsStore(ctx context.Context, store *accountstore.Store) error {
	if s == nil || s.h == nil || !s.h.cfg.Qualification {
		return ErrDenied
	}
	return store.CheckControlDatabase(ctx, s.h.reviewerDB)
}

func (s *operationsSource) BindOperations(ops *accountstore.Operations) {
	s.h.mu.Lock()
	defer s.h.mu.Unlock()
	s.h.operations = ops
}

func (h *Harness) localOperations() *accountstore.Operations {
	if h.origin != nil {
		h = h.origin
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.operations
}

func (h *Harness) holdOperations(ctx context.Context, tx *sql.Tx) (func(), error) {
	var required bool
	if tx.QueryRowContext(ctx, `SELECT careeros_operations_required()`).Scan(&required) != nil {
		return nil, ErrUnavailable
	}
	if !required {
		return func() {}, nil
	}
	ops := h.localOperations()
	if ops == nil {
		return nil, ErrUnavailable
	}
	release, err := ops.HoldProtected(ctx, "inference")
	if errors.Is(err, auth.ErrForbidden) {
		return nil, ErrDenied
	}
	return release, err
}

func (s *operationsSource) OperationsSample(ctx context.Context) (accountstore.OperationsRiskSample, error) {
	var out accountstore.OperationsRiskSample
	if s == nil || s.h == nil || !s.h.cfg.Qualification {
		return out, ErrDenied
	}
	h := s.h
	tx, err := h.reviewerDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return out, ErrUnavailable
	}
	defer tx.Rollback()
	out, err = h.sampleOperations(ctx, tx)
	if err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, ErrUnavailable
	}
	return out, nil
}

func (h *Harness) sampleOperations(ctx context.Context, tx *sql.Tx) (accountstore.OperationsRiskSample, error) {
	var out accountstore.OperationsRiskSample
	var err error
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&out.At); err != nil {
		return out, ErrUnavailable
	}
	out.Namespace = h.cfg.Namespace
	out.Figures.At, out.Figures.Currency = out.At, "USD"
	var namespaces int
	var selected bool
	if tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(bool_and(id=$1 AND NOT retired),false) FROM ai_fixture_namespace`, h.cfg.Namespace).Scan(&namespaces, &selected) != nil {
		return out, ErrUnavailable
	}
	if namespaces != 1 || !selected {
		return out, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT scope,limit_amount,counted FROM ai_fixture_scope WHERE namespace=$1 ORDER BY scope`, h.cfg.Namespace)
	if err != nil {
		return out, ErrUnavailable
	}
	foundGlobal := false
	for rows.Next() {
		var scope string
		var limit, counted int64
		if err = rows.Scan(&scope, &limit, &counted); err != nil {
			break
		}
		label := ""
		switch {
		case scope == "global":
			label = "pilot_total"
			foundGlobal = limit == selectedQualificationEnvelope().GlobalTotal
		case strings.HasPrefix(scope, "provider:"):
			label = "provider_total"
		case strings.HasPrefix(scope, "account:"):
			label = "account_grant"
		default:
			err = ErrUnavailable
		}
		if err != nil {
			break
		}
		out.Caps = append(out.Caps, accountstore.OperationsCap{Key: h.Reconciliation().digest("operations", "scope", scope),
			Scope: label, Policy: QualificationRateID, Currency: "USD", Counted: counted, Limit: limit})
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, ErrUnavailable
	}
	if !foundGlobal {
		return out, nil
	}
	rows, err = tx.QueryContext(ctx, `SELECT a.id,a.owner,a.execution_version,d.id,d.attempt,d.counted,
 COALESCE(d.intent_at,a.created_at),d.remote_state,d.expense,d.maximum,d.category,
 COALESCE((o.data->>'estimate')::bigint,0)
 FROM ai_fixture_dispatch d JOIN ai_fixture_action a ON a.namespace=d.namespace AND a.id=d.action
 LEFT JOIN ai_fixture_observation o ON o.namespace=d.namespace AND o.dispatch=d.id AND o.sequence=1
 WHERE d.namespace=$1 ORDER BY a.id,d.attempt,d.id`, h.cfg.Namespace)
	if err != nil {
		return out, ErrUnavailable
	}
	caps := map[string]accountstore.OperationsCap{}
	accountDay := map[string]int64{}
	var globalDay int64
	count := 0
	for rows.Next() {
		count++
		if count > 65536 {
			err = ErrUnavailable
			break
		}
		var action, owner, policy, dispatch, remote, category string
		var attempt, amount, maximum, observed int64
		var at time.Time
		var expense sql.NullInt64
		if err = rows.Scan(&action, &owner, &policy, &dispatch, &attempt, &amount, &at, &remote, &expense, &maximum, &category, &observed); err != nil {
			break
		}
		attemptLimit, actionLimit, ok := operationsMoneyLimits(policy)
		if !ok || amount < 0 || !addOperationsFigures(&out.Figures, amount, expense, observed, category) {
			err = ErrUnavailable
			break
		}
		if _, tracked := accountDay[owner]; !tracked {
			accountDay[owner] = 0
		}
		event := accountstore.OperationsRiskEvent{Key: h.Reconciliation().digest("operations", "dispatch", dispatch),
			At: at, Policy: policy, Exposure: amount, Maximum: maximum}
		if remote == "possibly_active" {
			out.Figures.PossibleRemote++
			event.Kind = "unknown_dispatch"
			event.Exposure = maximum
			out.Events = append(out.Events, event)
		}
		if amount > maximum {
			event.Kind = "maximum_breach"
			event.Exposure = amount
			out.Events = append(out.Events, event)
		}
		for _, cap := range []struct {
			key, scope string
			limit      int64
		}{{action + "/" + strconv.FormatInt(attempt, 10), "attempt", attemptLimit}, {action, "action", actionLimit}} {
			key := cap.scope + "/" + cap.key
			v := caps[key]
			if amount > math.MaxInt64-v.Counted {
				err = ErrUnavailable
				break
			}
			v.Key = h.Reconciliation().digest("operations", cap.scope, cap.key)
			v.Scope, v.Policy, v.Currency, v.Limit = cap.scope, policy, "USD", cap.limit
			v.Counted += amount
			caps[key] = v
		}
		if err != nil {
			break
		}
		// Exactly the original rolling admission predicate: unresolved amounts
		// and differences carry even after the original intent is 24h old.
		if at.After(out.At.Add(-24*time.Hour)) || remote != "terminal_proven" || !expense.Valid || amount > expense.Int64 {
			if amount > math.MaxInt64-globalDay || amount > math.MaxInt64-accountDay[owner] {
				err = ErrUnavailable
				break
			}
			globalDay += amount
			accountDay[owner] += amount
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, ErrUnavailable
	}
	err = tx.QueryRowContext(ctx, `SELECT
 (SELECT count(DISTINCT a.id) FROM ai_fixture_action a JOIN ai_fixture_stage s
 ON s.namespace=a.namespace AND s.action=a.id AND s.attempt=a.attempt
 WHERE a.namespace=$1 AND a.state='recoverable' AND s.stage='local_execution' AND s.receipt_id=''),
 (SELECT count(*) FROM ai_fixture_grant g WHERE g.namespace=$1 AND NOT g.revoked AND g.expires_at>$2
 AND (SELECT count(*) FROM ai_fixture_action a WHERE a.namespace=g.namespace AND a.owner=g.owner
 AND (a.customer_reserved OR a.customer_effect))>=g.units)`, h.cfg.Namespace, out.At).
		Scan(&out.Figures.LocalActiveActions, &out.Figures.ExhaustedDraftGrants)
	if err != nil {
		return out, ErrUnavailable
	}
	for _, cap := range caps {
		out.Caps = append(out.Caps, cap)
	}
	p := selectedQualificationEnvelope()
	for owner, amount := range accountDay {
		out.Caps = append(out.Caps, accountstore.OperationsCap{Key: h.Reconciliation().digest("operations", "account_rolling24h", owner),
			Scope: "account_rolling24h", Policy: QualificationRateID, Currency: "USD", Counted: amount, Limit: p.AccountDay})
	}
	out.Caps = append(out.Caps, accountstore.OperationsCap{Key: h.Reconciliation().digest("operations", "global_rolling24h", h.cfg.Namespace),
		Scope: "global_rolling24h", Policy: QualificationRateID, Currency: "USD", Counted: globalDay, Limit: p.GlobalDay})
	sort.Slice(out.Caps, func(a, b int) bool { return out.Caps[a].Key < out.Caps[b].Key })
	out.Complete = true
	return out, nil
}

// A staged sample is committed with the ledger change. Another enrolled
// process can replay it if the originating process ends before delivery.
func (h *Harness) stageOperations(ctx context.Context, tx *sql.Tx, extra *accountstore.OperationsRiskEvent) (bool, error) {
	var required bool
	if tx.QueryRowContext(ctx, `SELECT careeros_operations_required()`).Scan(&required) != nil {
		return false, ErrUnavailable
	}
	if !required {
		return false, nil
	}
	sample, err := h.sampleOperations(ctx, tx)
	if err != nil || !sample.Complete {
		return true, ErrUnavailable
	}
	if extra != nil {
		extra.At = sample.At
		sample.Events = append(sample.Events, *extra)
	}
	raw, err := json.Marshal(sample)
	if err != nil {
		return true, ErrUnavailable
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO careeros_operations_pending(namespace,sample) VALUES($1,$2)`, h.cfg.Namespace, raw)
	return true, safe(err)
}

func (s *operationsSource) DrainOperations(ctx context.Context, publish func(accountstore.OperationsRiskSample) error) error {
	db := s.h.reviewerDB
	for {
		var id int64
		var raw []byte
		err := db.QueryRowContext(ctx, `SELECT id,sample FROM careeros_operations_pending
		  WHERE namespace=$1 ORDER BY id LIMIT 1`, s.h.cfg.Namespace).Scan(&id, &raw)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return ErrUnavailable
		}
		var sample accountstore.OperationsRiskSample
		if json.Unmarshal(raw, &sample) != nil {
			return ErrUnavailable
		}
		if err = publish(sample); err != nil {
			return err
		}
		if _, err = db.ExecContext(ctx, `DELETE FROM careeros_operations_pending WHERE id=$1 AND namespace=$2`, id, s.h.cfg.Namespace); err != nil {
			return ErrUnavailable
		}
	}
}

func (h *Harness) deliverOperations(ctx context.Context) error {
	if ops := h.localOperations(); ops != nil {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return ops.DrainCommittedRisk(ctx)
	}
	return nil
}

func (h *Harness) observeAuthorityFault(ctx context.Context, kind accountstore.OperationsFault, err error) error {
	if !errors.Is(err, ErrUnavailable) && !errors.Is(err, auth.ErrUnavailable) {
		return err
	}
	if ctx.Err() != nil {
		return err
	}
	if ops := h.localOperations(); ops != nil {
		_ = ops.ObserveFault(ctx, kind)
	}
	return err
}

type managedControlFault struct{ error }

func (e managedControlFault) Unwrap() error { return e.error }

func (h *Harness) unavailableControl(err error) error {
	if h.localOperations() == nil {
		return err
	}
	return managedControlFault{err}
}

func (h *Harness) observeManagedControlFault(ctx context.Context, err error) {
	var fault managedControlFault
	if errors.As(err, &fault) {
		_ = h.observeAuthorityFault(ctx, accountstore.OperationsControlFault, fault.error)
	}
}

func addOperationsFigures(figures *accountstore.OperationsFigures, amount int64, expense sql.NullInt64, observed int64, category string) bool {
	add := func(total *int64, delta int64) bool {
		if delta < 0 || delta > math.MaxInt64-*total {
			return false
		}
		*total += delta
		return true
	}
	if !add(&figures.Counted, amount) || !add(&figures.ObservedUsage, observed) {
		return false
	}
	if !expense.Valid {
		return add(&figures.UnknownDifference, amount)
	}
	switch category {
	case "supported_estimate":
		if !add(&figures.SupportedEstimate, expense.Int64) {
			return false
		}
	case "confirmed_bill":
		if !add(&figures.AttributableBills, expense.Int64) {
			return false
		}
	case "reviewed_conservative_maximum":
		if !add(&figures.ConservativeMaximum, expense.Int64) {
			return false
		}
	}
	if amount <= expense.Int64 {
		return true
	}
	return add(&figures.UnknownDifference, amount-expense.Int64)
}

func operationsMoneyLimits(policy string) (int64, int64, bool) {
	switch policy {
	case "qualification-local-20261102-v1":
		p := selectedQualificationEnvelope()
		return p.AttemptMoney, p.ActionMoney, true
	case IngestionPolicyVersion:
		p := selectedIngestionEnvelope()
		return p.AttemptMoney, p.ActionMoney, true
	case ProfileReviewPolicyVersion:
		p := selectedProfileReviewEnvelope()
		return p.AttemptMoney, p.ActionMoney, true
	case StandaloneCVPolicyVersion:
		p := selectedStandaloneCVEnvelope()
		return p.AttemptMoney, p.ActionMoney, true
	case ManagedDraftPolicyVersion:
		return managedDraftPolicy.AttemptMoney, managedDraftPolicy.ActionMoney, true
	default:
		return 0, 0, false
	}
}

package aicontrol

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
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

//go:embed operations_capture.sql
var operationsCaptureSQL string

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
	return (&Harness{reviewerDB: r.h.reviewerDB, cfg: cfg, origin: r.h}).LocalOperationsSource(), nil
}

func (s *operationsSource) CheckOperationsStore(ctx context.Context, store *accountstore.Store) error {
	if s == nil || s.h == nil || !s.h.cfg.Qualification {
		return ErrDenied
	}
	return store.CheckControlDatabase(ctx, s.h.reviewerDB)
}

func (s *operationsSource) BindOperations(ops *accountstore.Operations) {
	h := s.h
	if h.origin != nil {
		h = h.origin
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.operations = ops
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

type capturedOperations struct {
	At                time.Time
	Active, Exhausted int64
	Scopes            []struct {
		Name           string
		Limit, Counted int64
	}
	Entries []struct {
		Action, Owner, Policy, Dispatch, Remote, Category string
		Attempt, Amount, Maximum, Observed                int64
		Expense                                           *int64
		At                                                time.Time
		PendingPricedBreach                               bool
	}
	Event *struct {
		ID, Rate, Kind, IncidentDispatch string
		Amount, Maximum                  int64
		At                               time.Time
	}
}

func (s *operationsSource) OperationsSample(ctx context.Context) (accountstore.OperationsRiskSample, error) {
	tx, err := s.h.reviewerDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return accountstore.OperationsRiskSample{}, ErrUnavailable
	}
	defer tx.Rollback()
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT careeros_operations_snapshot($1,NULL)`, s.h.cfg.Namespace).Scan(&raw); err != nil {
		return accountstore.OperationsRiskSample{}, ErrUnavailable
	}
	if tx.Commit() != nil {
		return accountstore.OperationsRiskSample{}, ErrUnavailable
	}
	return s.decodeOperations(raw)
}

func (s *operationsSource) decodeOperations(raw []byte) (accountstore.OperationsRiskSample, error) {
	var out accountstore.OperationsRiskSample
	var capture capturedOperations
	if len(raw) == 0 || len(raw) > 16<<20 || json.Unmarshal(raw, &capture) != nil ||
		capture.At.IsZero() || capture.Active < 0 || capture.Exhausted < 0 || len(capture.Scopes) > 65536 || len(capture.Entries) > 65536 {
		return out, ErrUnavailable
	}
	h := s.h
	out.At, out.Namespace = capture.At, h.cfg.Namespace
	out.Figures.At, out.Figures.Currency = capture.At, "USD"
	out.Figures.LocalActiveActions, out.Figures.ExhaustedDraftGrants = capture.Active, capture.Exhausted
	foundGlobal := false
	for _, scope := range capture.Scopes {
		label := ""
		switch {
		case scope.Name == "global":
			foundGlobal = scope.Limit == selectedQualificationEnvelope().GlobalTotal
			label = "pilot_total"
		case scope.Name == "provider:openai" || scope.Name == "provider:typesafe":
			label = "provider_total"
		case strings.HasPrefix(scope.Name, "account:") && tokenLabel.MatchString(strings.TrimPrefix(scope.Name, "account:")):
			label = "account_grant"
		default:
			return out, ErrUnavailable
		}
		if scope.Limit <= 0 || scope.Counted < 0 {
			return out, ErrUnavailable
		}
		out.Caps = append(out.Caps, accountstore.OperationsCap{Key: h.Reconciliation().digest("operations", "scope", scope.Name),
			Scope: label, Policy: QualificationRateID, Currency: "USD", Counted: scope.Counted, Limit: scope.Limit})
	}
	if !foundGlobal {
		return out, ErrUnavailable
	}
	caps := map[string]accountstore.OperationsCap{}
	accountDay := map[string]int64{}
	var globalDay int64
	for _, d := range capture.Entries {
		if !tokenLabel.MatchString(d.Action) || !tokenLabel.MatchString(d.Owner) || !tokenLabel.MatchString(d.Dispatch) ||
			d.At.IsZero() || d.At.After(capture.At) || d.Attempt <= 0 || d.Maximum <= 0 || d.Amount < 0 || d.Observed < 0 ||
			(d.Expense != nil && *d.Expense < 0) {
			return out, ErrUnavailable
		}
		switch d.Remote {
		case "not_started", "possibly_active", "terminal_proven":
		default:
			return out, ErrUnavailable
		}
		switch d.Category {
		case "reserved_maximum", "unknown_maximum", "supported_estimate", "reviewed_conservative_maximum", "confirmed_bill", "aborted_unsent":
		default:
			return out, ErrUnavailable
		}
		attemptLimit, actionLimit, ok := operationsMoneyLimits(d.Policy)
		expense := sql.NullInt64{}
		if d.Expense != nil {
			expense = sql.NullInt64{Int64: *d.Expense, Valid: true}
		}
		if !ok || !addOperationsFigures(&out.Figures, d.Amount, expense, d.Observed, d.Category) {
			return out, ErrUnavailable
		}
		event := accountstore.OperationsRiskEvent{Key: h.Reconciliation().digest("operations", "dispatch", d.Dispatch),
			At: d.At, Policy: d.Policy, Exposure: d.Maximum, Maximum: d.Maximum}
		if d.Remote == "possibly_active" {
			out.Figures.PossibleRemote++
			event.Kind = "unknown_dispatch"
			out.Events = append(out.Events, event)
		}
		if d.Amount > d.Maximum && !d.PendingPricedBreach {
			event.Kind, event.Exposure = "maximum_breach", d.Amount
			out.Events = append(out.Events, event)
		}
		for _, cap := range []struct {
			key, scope string
			limit      int64
		}{
			{d.Action + "/" + strconv.FormatInt(d.Attempt, 10), "attempt", attemptLimit},
			{d.Action, "action", actionLimit},
		} {
			key := cap.scope + "/" + cap.key
			value := caps[key]
			if d.Amount > math.MaxInt64-value.Counted {
				return out, ErrUnavailable
			}
			value.Key = h.Reconciliation().digest("operations", cap.scope, cap.key)
			value.Scope, value.Policy, value.Currency, value.Limit = cap.scope, d.Policy, "USD", cap.limit
			value.Counted += d.Amount
			caps[key] = value
		}
		if _, tracked := accountDay[d.Owner]; !tracked {
			accountDay[d.Owner] = 0
		}
		if d.At.After(out.At.Add(-24*time.Hour)) || d.Remote != "terminal_proven" || !expense.Valid || d.Amount > expense.Int64 {
			if d.Amount > math.MaxInt64-globalDay || d.Amount > math.MaxInt64-accountDay[d.Owner] {
				return out, ErrUnavailable
			}
			globalDay += d.Amount
			accountDay[d.Owner] += d.Amount
		}
	}
	if event := capture.Event; event != nil {
		if !tokenLabel.MatchString(event.ID) || !tokenLabel.MatchString(event.Rate) ||
			event.Amount < 0 || event.Maximum <= 0 || event.At.IsZero() || event.At.After(capture.At) ||
			(event.Kind != "maximum_breach" && event.Kind != "invariant_breach") {
			return out, ErrUnavailable
		}
		key := h.Reconciliation().digest("operations", "reconciliation", event.ID)
		if event.IncidentDispatch != "" {
			if event.Kind != "maximum_breach" || !tokenLabel.MatchString(event.IncidentDispatch) {
				return out, ErrUnavailable
			}
			key = h.Reconciliation().digest("operations", "dispatch", event.IncidentDispatch)
		}
		out.Events = append(out.Events, accountstore.OperationsRiskEvent{
			Key: key, Kind: event.Kind,
			At: event.At, Policy: event.Rate, Exposure: event.Amount, Maximum: event.Maximum,
		})
	}
	for _, cap := range caps {
		out.Caps = append(out.Caps, cap)
	}
	envelope := selectedQualificationEnvelope()
	for owner, amount := range accountDay {
		out.Caps = append(out.Caps, accountstore.OperationsCap{Key: h.Reconciliation().digest("operations", "account_rolling24h", owner),
			Scope: "account_rolling24h", Policy: QualificationRateID, Currency: "USD", Counted: amount, Limit: envelope.AccountDay})
	}
	out.Caps = append(out.Caps, accountstore.OperationsCap{Key: h.Reconciliation().digest("operations", "global_rolling24h", h.cfg.Namespace),
		Scope: "global_rolling24h", Policy: QualificationRateID, Currency: "USD", Counted: globalDay, Limit: envelope.GlobalDay})
	sort.Slice(out.Caps, func(a, b int) bool { return out.Caps[a].Key < out.Caps[b].Key })
	out.Complete = true
	return out, nil
}

// The database derives the observation from the locked ledger transition.
// Neither configured runtime nor the application supplies numeric evidence.
func (h *Harness) stageOperations(ctx context.Context, tx *sql.Tx) (bool, error) {
	var enabled bool
	if tx.QueryRowContext(ctx, `SELECT careeros_operations_required()`).Scan(&enabled) != nil {
		return false, ErrUnavailable
	}
	if !enabled {
		return false, nil
	}
	if tx.QueryRowContext(ctx, `SELECT careeros_capture_runtime_operations($1)`, h.cfg.Namespace).Scan(&enabled) != nil {
		return true, ErrUnavailable
	}
	return enabled, nil
}

func (s *operationsSource) DrainOperations(ctx context.Context, publish func(accountstore.OperationsRiskSample) error) error {
	db := s.h.reviewerDB
	for {
		var id int64
		var version int
		var raw []byte
		err := db.QueryRowContext(ctx, `SELECT id,source_version,sample FROM careeros_operations_pending
		  WHERE namespace=$1 ORDER BY id LIMIT 1`, s.h.cfg.Namespace).Scan(&id, &version, &raw)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil || version != 2 || id <= 0 {
			return ErrUnavailable
		}
		observer := s.h.localOperations()
		if observer == nil {
			return ErrUnavailable
		}
		sample, err := s.decodeOperations(raw)
		if err != nil {
			return err
		}
		sample.DurableSequence = id
		if err = publish(sample); err != nil {
			return err
		}
		if err = observer.AcknowledgeCapture(ctx, db, s.h.cfg.Namespace, id, sha256.Sum256(raw)); err != nil {
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

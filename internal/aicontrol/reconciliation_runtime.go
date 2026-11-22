package aicontrol

import (
	"context"
	"database/sql"
	"encoding/json"

	"professional-information-repo/internal/auth"
)

type BillingContract struct {
	Provider, Model, Tier, Rate, Currency, BillingNamespace string
}
type UnaccountedBound struct {
	Amount    int64
	Reference string
}
type UsageCoverageValidator func(BillingContract, ProviderObservation) (UnaccountedBound, error)

// SetSyntheticUsageCoverage is code-owned fake composition, never a request
// option. Without a trustworthy full-charge contract, normal observations keep
// their original conservative floor. It neither retrieves nor assumes bills.
func (h *Harness) SetSyntheticUsageCoverage(validate UsageCoverageValidator) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.usageCoverage = validate
}

// SettleSavedObservation accepts only the identity of an immutable observation.
// Runtime chooses neither amount, rate, category, difference nor floor release.
func (h *Harness) SettleSavedObservation(ctx context.Context, p auth.Principal, id string) error {
	return h.settleSavedObservation(ctx, p, id, false)
}
func (h *Harness) settleSavedObservation(ctx context.Context, p auth.Principal, id string, observer bool) error {
	source := h
	if h.origin != nil {
		source = h.origin
	}
	source.mu.Lock()
	validator := source.usageCoverage
	source.mu.Unlock()
	if validator == nil || !tokenLabel.MatchString(id) {
		return ErrDenied
	}
	var raw []byte
	var contract BillingContract
	if h.db.QueryRowContext(ctx, `SELECT o.data,c.provider,c.model,c.tier,c.rate_id,c.currency,c.billing_namespace FROM ai_fixture_observation o JOIN ai_recon_contract c ON c.namespace=o.namespace AND c.dispatch=o.dispatch WHERE o.namespace=$1 AND o.dispatch=$2 AND o.sequence=1`, h.cfg.Namespace, id).Scan(&raw, &contract.Provider, &contract.Model, &contract.Tier, &contract.Rate, &contract.Currency, &contract.BillingNamespace) != nil {
		return ErrDenied
	}
	var observation ProviderObservation
	if json.Unmarshal(raw, &observation) != nil || observation.BodyState != "complete" || observation.Unsupported || observation.RequestIDState != "valid" || observation.ModelState != "valid" || observation.TierState != "valid" || observation.RateState != "supported" || observation.Provider != contract.Provider || observation.Model != contract.Model || observation.Tier != contract.Tier || observation.RateID != contract.Rate {
		return ErrDenied
	}
	bound, err := validator(contract, observation)
	if err != nil || bound.Amount < 0 || !tokenLabel.MatchString(bound.Reference) {
		return ErrDenied
	}
	command := ReconciliationCommand{ID: "normal_" + id, Kind: "usage", Dispatch: id, Evidence: ReconciliationEvidence{
		Source: "normal_" + id, Revision: 1, Reason: bound.Reference, Provider: contract.Provider, Model: contract.Model, Tier: contract.Tier, Rate: contract.Rate, Currency: contract.Currency, BillingNamespace: contract.BillingNamespace,
		RequestID: observation.RequestID, Complete: true, Difference: &bound.Amount, Usage: &Usage{Input: observation.Input.Value, Output: observation.Output.Value, Cached: observation.Cached.Value, Reasoning: observation.Reasoning.Value, CacheWrite: observation.CacheWrite.Value},
	}}
	r := &Reconciliation{h: h, runtime: true, observer: observer}
	if err := r.runtimeVersion(ctx, &command); err != nil {
		return err
	}
	preview, err := r.Preview(ctx, p, command)
	if err != nil {
		return err
	}
	command.Preview = preview.Token
	_, err = r.Apply(ctx, p, command)
	return err
}

// runtimeVersion preserves the first admitted evidence command's expected
// version on replay, while permitting a later validated coverage command after
// a prior observation that deliberately retained the original floor.
func (r *Reconciliation) runtimeVersion(ctx context.Context, c *ReconciliationCommand) error {
	var expected int64
	err := r.h.reviewerDB.QueryRowContext(ctx, `SELECT version-1 FROM ai_recon_command WHERE namespace=$1 AND id=$2`, r.h.cfg.Namespace, c.ID).Scan(&expected)
	if err == sql.ErrNoRows {
		err = r.h.reviewerDB.QueryRowContext(ctx, `SELECT COALESCE((SELECT version FROM ai_recon_state WHERE namespace=$1 AND dispatch=$2),0)`, r.h.cfg.Namespace, c.Dispatch).Scan(&expected)
	}
	if err != nil {
		return ErrUnavailable
	}
	c.ExpectedVersion = expected
	return nil
}

// recordRuntimeObservation is called only from the code-owned response reader
// after its immutable observation append. Its original dispatch capability
// remains financial authority after the customer's session/grant ends. It can
// only preserve/raise exposure, never release a floor or remote slot.
func (h *Harness) recordRuntimeObservation(ctx context.Context, p auth.Principal, d Dispatch, o ProviderObservation) error {
	if o.Estimate == nil || o.ModelState != "valid" || o.TierState != "valid" || o.RateState != "supported" {
		return ErrDenied
	}
	request := o.RequestID
	if o.RequestIDState != "valid" {
		request = "unavailable"
	}
	currency := "SYN"
	if o.RateID == QualificationRateID {
		currency = "USD"
	}
	command := ReconciliationCommand{ID: "observation_" + d.ID, Kind: "observation", Dispatch: d.ID, Evidence: ReconciliationEvidence{Source: "observation_" + d.ID, Revision: 1, Reason: "runtime_provider_observation", Provider: d.Provider, Model: o.Model, Tier: o.Tier, Rate: o.RateID, Currency: currency, BillingNamespace: "synthetic", RequestID: request, Usage: &Usage{Input: o.Input.Value, Output: o.Output.Value, Cached: o.Cached.Value, Reasoning: o.Reasoning.Value, CacheWrite: o.CacheWrite.Value, Unsupported: o.Unsupported}}}
	r := &Reconciliation{h: h, runtime: true, observer: true}
	if err := r.runtimeVersion(ctx, &command); err != nil {
		return err
	}
	preview, err := r.Preview(ctx, p, command)
	if err != nil {
		return err
	}
	command.Preview = preview.Token
	_, err = r.Apply(ctx, p, command)
	return err
}

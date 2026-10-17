package evaluationlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"professional-information-repo/internal/semanticeval"
	"time"
)

type GuardApproval struct {
	PreflightID  string      `json:"preflightId"`
	Hash         string      `json:"hash"`
	Actor        ReviewActor `json:"actor"`
	Confirmation string      `json:"confirmation"`
	Rationale    string      `json:"rationale"`
}
type GuardApprovalReceipt struct {
	Hash      string      `json:"hash"`
	Actor     ReviewActor `json:"actor"`
	Origin    string      `json:"origin"`
	Rationale string      `json:"rationale"`
}
type GuardApprovalChallenge struct {
	Hash         string `json:"hash"`
	Reviewer     string `json:"reviewer"`
	Confirmation string `json:"confirmation"`
}
type GuardAuthorization struct {
	RunID string `json:"runId"`
	Token string `json:"-"`
}
type GuardCallObservation struct {
	Outcome string
	Call    semanticeval.Call
}
type GuardRetainedCall struct {
	UsageUnavailableReason string            `json:"usageUnavailableReason,omitempty"`
	Spec                   GuardDispatchSpec `json:"spec"`
	State                  string            `json:"state"`
	Call                   semanticeval.Call `json:"call"`
	StartedAt              string            `json:"startedAt"`
	FinishedAt             string            `json:"finishedAt,omitempty"`
}
type guardSecrets struct {
	token       string
	credentials map[string]string
	ctx         context.Context
	cancel      context.CancelFunc
	inFlight    bool
}
type ControlledGuardTransport func(context.Context, GuardDispatchSpec, string) (GuardCallObservation, error)

func (g *LiveGuard) Approval(id string) (GuardApprovalChallenge, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	record, ok := g.records[id]
	if !ok || g.closed || !g.options.Enabled || g.options.HumanReviewer == "" || record.State != "planned" {
		return GuardApprovalChallenge{}, fmt.Errorf("configured human and pending owned preflight required")
	}
	expires, err := time.Parse(time.RFC3339Nano, record.Preflight.ExpiresAt)
	if err != nil || !g.options.Now().Before(expires) {
		return GuardApprovalChallenge{}, fmt.Errorf("preflight expired")
	}
	return GuardApprovalChallenge{Hash: guardApprovalHash(record), Reviewer: g.options.HumanReviewer, Confirmation: "Approve " + guardApprovalHash(record) + " as " + g.options.HumanReviewer}, nil
}
func (g *LiveGuard) Authorize(input GuardApproval, credentials map[string]string) (GuardAuthorization, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	record, ok := g.records[input.PreflightID]
	if !ok || g.closed || !g.options.Enabled || g.active != "" || g.options.HumanReviewer == "" || record.State != "planned" || input.Actor.Type != "human" || input.Actor.ID != g.options.HumanReviewer || input.Hash != guardApprovalHash(record) || input.Confirmation != "Approve "+guardApprovalHash(record)+" as "+g.options.HumanReviewer || input.Rationale == "" || len(input.Rationale) > 4096 {
		return GuardAuthorization{}, fmt.Errorf("exact independent human authorization required")
	}
	if err := g.comparisonAdmission(record); err != nil {
		return GuardAuthorization{}, err
	}
	expires, err := time.Parse(time.RFC3339Nano, record.Preflight.ExpiresAt)
	if err != nil || !g.options.Now().Before(expires) {
		return GuardAuthorization{}, fmt.Errorf("preflight expired")
	}
	if record.Preflight.Instance != g.instance || record.Preflight.Hash != livePreflightHash(record.Preflight) {
		return GuardAuthorization{}, fmt.Errorf("changed or restarted authorization")
	}
	if err = g.planner.ValidatePlan(record.Preflight.Plan); err != nil {
		return GuardAuthorization{}, fmt.Errorf("owned plan unavailable")
	}
	if len(credentials) != len(record.Preflight.Recipients) {
		return GuardAuthorization{}, fmt.Errorf("exact recipient credentials required")
	}
	owned := map[string]string{}
	for _, recipient := range record.Preflight.Recipients {
		value := credentials[recipient]
		if value == "" || len(value) > 4096 {
			return GuardAuthorization{}, fmt.Errorf("exact recipient credentials required")
		}
		owned[recipient] = value
	}
	record.Approval = &GuardApprovalReceipt{Hash: input.Hash, Actor: input.Actor, Origin: g.options.Origin, Rationale: input.Rationale}
	publicEvidence, _ := json.Marshal(record)
	for _, value := range owned {
		encoded, _ := json.Marshal(value)
		if bytes.Contains(publicEvidence, encoded[1:len(encoded)-1]) {
			clear(owned)
			return GuardAuthorization{}, fmt.Errorf("credentials cannot appear in retained approval metadata")
		}
	}
	record.State = "authorized"
	if err = g.store.saveGuard(record); err != nil {
		clear(owned)
		return GuardAuthorization{}, fmt.Errorf("approval retention unavailable")
	}
	timeout := time.Duration(record.Preflight.Plan.Caps.DeadlineMS) * time.Millisecond
	if remaining := expires.Sub(g.options.Now()); remaining < timeout {
		timeout = remaining
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	token := newLabID()
	g.secrets[input.PreflightID] = &guardSecrets{token: token, credentials: owned, ctx: ctx, cancel: cancel}
	g.active = input.PreflightID
	g.records[input.PreflightID] = record
	go func() { <-ctx.Done(); g.expire(input.PreflightID, token) }()
	return GuardAuthorization{RunID: input.PreflightID, Token: token}, nil
}
func (g *LiveGuard) revoke(id string) {
	inFlight := false
	if secret := g.secrets[id]; secret != nil {
		inFlight = secret.inFlight
		secret.cancel()
		clear(secret.credentials)
		delete(g.secrets, id)
	}
	if g.active == id && !inFlight {
		g.active = ""
	}
}
func (g *LiveGuard) Dispatch(ctx context.Context, authorization GuardAuthorization, id string, transport ControlledGuardTransport) error {
	g.mu.Lock()
	record, ok := g.records[authorization.RunID]
	secret := g.secrets[authorization.RunID]
	if !ok || g.closed || !g.options.Enabled || secret == nil || !secretEqual(secret.token, authorization.Token) || secret.inFlight || transport == nil || (record.State != "authorized" && record.State != "running") || record.Next >= len(record.Preflight.Dispatches) || record.Preflight.Dispatches[record.Next].ID != id {
		g.mu.Unlock()
		return fmt.Errorf("unused next dispatch and current authorization required")
	}
	expires, _ := time.Parse(time.RFC3339Nano, record.Preflight.ExpiresAt)
	if ctx.Err() != nil || secret.ctx.Err() != nil || !g.options.Now().Before(expires) {
		record.State = "canceled"
		g.records[authorization.RunID] = record
		g.revoke(authorization.RunID)
		_ = g.store.saveGuard(record)
		g.mu.Unlock()
		return fmt.Errorf("run canceled or expired")
	}
	spec := record.Preflight.Dispatches[record.Next]
	cases := map[string]bool{}
	legs := map[string]bool{}
	for _, call := range record.Calls {
		cases[call.Spec.CaseID] = true
		legs[call.Spec.HTTPID] = true
	}
	cases[spec.CaseID] = true
	legs[spec.HTTPID] = true
	caps := record.Preflight.Plan.Caps
	if len(record.Calls) >= caps.MaxProviderCalls || len(cases) > caps.MaxCases || len(legs) > caps.MaxHTTPAttempts || record.ReservedTokenUnits+spec.TokenReservation > record.Preflight.MaxTokenUnits {
		record.State = "failed"
		g.records[authorization.RunID] = record
		g.revoke(authorization.RunID)
		_ = g.store.saveGuard(record)
		g.mu.Unlock()
		return fmt.Errorf("dispatch budget exhausted")
	}
	credential := secret.credentials[spec.Recipient]
	record.State = "running"
	record.ReservedTokenUnits += spec.TokenReservation
	record.Calls = append(record.Calls, GuardRetainedCall{Spec: spec, State: "unknown-final", StartedAt: g.options.Now().UTC().Format(time.RFC3339Nano)})
	if err := g.store.saveGuard(record); err != nil {
		record = g.records[authorization.RunID]
		record.State = "failed"
		g.records[authorization.RunID] = record
		g.revoke(authorization.RunID)
		g.mu.Unlock()
		return fmt.Errorf("dispatch intent retention unavailable")
	}
	g.records[authorization.RunID] = record
	secret.inFlight = true
	callCtx, cancel := context.WithCancel(secret.ctx)
	stop := context.AfterFunc(ctx, cancel)
	g.mu.Unlock()
	started := time.Now()
	observed, callErr := transport(callCtx, cloneJSON(spec), credential)
	elapsed := float64(time.Since(started)) / float64(time.Millisecond)
	canceled := callCtx.Err() != nil
	stop()
	cancel()
	credential = ""
	g.mu.Lock()
	defer g.mu.Unlock()
	record = g.records[authorization.RunID]
	call := &record.Calls[len(record.Calls)-1]
	call.FinishedAt = g.options.Now().UTC().Format(time.RFC3339Nano)
	outcome := observed.Outcome
	if canceled {
		outcome = "canceled"
	} else if errors.Is(callErr, context.DeadlineExceeded) {
		outcome = "timeout"
	} else if callErr != nil {
		outcome = "transport_failure"
	}
	if !contains([]string{"completed", "canceled", "timeout", "transport_failure", "refused", "truncated", "malformed", "rate_limited", "unavailable", "contract_failure"}, outcome) {
		outcome = "contract_failure"
	}
	// The transport supplies observations, never report metadata or arbitrary
	// request/header fields that could echo credentials.
	source := observed.Call
	observed.Call = semanticeval.Call{DispatchID: spec.ID, Provider: spec.Recipient, Model: spec.Model, Reasoning: spec.Reasoning, MaxCompletionTokens: spec.MaxOutputTokens, ReturnedModel: source.ReturnedModel, Status: source.Status, InputTokens: cloneJSON(source.InputTokens), OutputTokens: cloneJSON(source.OutputTokens), ReasoningTokens: cloneJSON(source.ReasoningTokens), CachedTokens: cloneJSON(source.CachedTokens), CacheWriteTokens: cloneJSON(source.CacheWriteTokens)}
	observed.Call.Provider = spec.Recipient
	observed.Call.DurationMS = elapsed
	if observed.Call.ReturnedModel != spec.Model && (outcome == "completed" || observed.Call.ReturnedModel != "") {
		observed.Call.ReturnedModel = safeGuardModel(observed.Call.ReturnedModel)
		outcome = "contract_failure"
	}
	if observed.Call.Status < 0 || observed.Call.Status > 599 {
		observed.Call.Status = 0
		outcome = "contract_failure"
	}
	if outcome == "completed" && (observed.Call.Status < 200 || observed.Call.Status >= 300) {
		outcome = "unavailable"
		if observed.Call.Status == 429 {
			outcome = "rate_limited"
		}
	}
	validation := validateUsage(observed.Call)
	if validation.Inconsistent {
		outcome = "contract_failure"
		call.UsageUnavailableReason = "Inconsistent controlled usage cannot be totaled or priced."
	}
	if observed.Call.OutputTokens != nil && *observed.Call.OutputTokens > record.Preflight.MaxOutputTokens {
		outcome = "contract_failure"
		call.UsageUnavailableReason = "Observed output exceeds the frozen ceiling."
	}
	if observed.Call.InputTokens != nil && (*observed.Call.InputTokens > spec.TokenReservation || observed.Call.OutputTokens != nil && *observed.Call.OutputTokens > spec.TokenReservation-*observed.Call.InputTokens) {
		outcome = "contract_failure"
		call.UsageUnavailableReason = "Observed usage exceeds the frozen reservation."
	}
	call.Call = observed.Call
	call.State = outcome
	if record.State == "canceled" || record.State == "expired" || g.closed {
		call.State = "canceled"
		if record.State != "expired" {
			record.State = "canceled"
		}
	} else if outcome != "completed" {
		record.State = "failed"
	} else {
		record.Next++
		if record.Next == len(record.Preflight.Dispatches) {
			record.State = "completed"
		}
	}
	if g.closed {
		defer g.store.releaseGuard(g.instance)
	}
	if err := g.store.saveGuard(record); err != nil {
		record.State = "unknown-final"
		g.active = ""
		g.records[authorization.RunID] = record
		g.revoke(authorization.RunID)
		return fmt.Errorf("dispatch result retention unavailable")
	}
	g.records[authorization.RunID] = record
	if secret = g.secrets[authorization.RunID]; secret != nil {
		secret.inFlight = false
	}
	if record.State == "completed" || record.State == "failed" || record.State == "canceled" || record.State == "expired" {
		g.active = ""
		g.revoke(authorization.RunID)
	}
	if call.State != "completed" {
		return fmt.Errorf("controlled dispatch did not complete")
	}
	return nil
}

type GuardReport struct {
	Metrics               []Metric              `json:"metrics"`
	AttemptedLogicalCases int                   `json:"admittedLogicalCases"`
	AttemptedHTTPAttempts int                   `json:"admittedWorkflowHTTPLegs"`
	Comparison            *GuardComparison      `json:"comparison,omitempty"`
	Version               string                `json:"version"`
	Preflight             LivePreflight         `json:"preflight"`
	Approval              *GuardApprovalReceipt `json:"approval,omitempty"`
	State                 string                `json:"state"`
	AttemptedCalls        int                   `json:"attemptedCalls"`
	ReservedTokenUnits    int64                 `json:"reservedTokenUnits"`
	UnreachedCalls        int                   `json:"unreachedCalls"`
	Calls                 []GuardRetainedCall   `json:"calls"`
	Usage                 UsageSummary          `json:"usage"`
	Limits                []string              `json:"limits"`
}

func (g *LiveGuard) Report(id string) (GuardReport, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	record, ok := g.records[id]
	if !ok {
		return GuardReport{}, fmt.Errorf("unknown guard evidence")
	}
	ledger := []Dispatch{}
	cases := map[string]bool{}
	legs := map[string]bool{}
	for _, call := range record.Calls {
		cases[call.Spec.CaseID] = true
		legs[call.Spec.HTTPID] = true
		priced := call.Call
		if call.UsageUnavailableReason != "" {
			priced.Provider = ""
			priced.InputTokens = nil
			priced.OutputTokens = nil
			priced.CachedTokens = nil
			priced.CacheWriteTokens = nil
			priced.ReasoningTokens = nil
		}
		ledger = append(ledger, Dispatch{Identity: call.Spec.ID, CaseIdentity: call.Spec.CaseID, Origin: "simulated", State: call.State, Call: priced})
	}
	usage, err := SummarizeDispatches(ledger, record.Preflight.Prices)
	if err != nil {
		if record.State == "unknown-final" {
			usage.TotalUSD = nil
		}
		for _, call := range record.Calls {
			if call.State == "unknown-final" {
				usage.TotalUSD = nil
			}
		}
		return GuardReport{}, err
	}
	return GuardReport{Metrics: guardMetrics(record, usage, len(cases), len(legs)), AttemptedLogicalCases: len(cases), AttemptedHTTPAttempts: len(legs), Comparison: cloneJSON(record.Comparison), Version: LiveGuardVersion, Preflight: cloneJSON(record.Preflight), Approval: cloneJSON(record.Approval), State: record.State, AttemptedCalls: len(record.Calls), ReservedTokenUnits: record.ReservedTokenUnits, UnreachedCalls: len(record.Preflight.Dispatches) - len(record.Calls), Calls: cloneJSON(record.Calls), Usage: usage, Limits: []string{"Controlled guard verification only; no actual provider dispatch, experiment approval, quality or population claim.", "Reserved token units are not provider-observed tokens or a dollar cap; unavailable prices/usage leave full cost unavailable."}}, nil
}
func (g *LiveGuard) ExportReport(id string) ([]byte, error) {
	report, err := g.Report(id)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(report)
	if len(raw) > 1<<20 {
		return nil, fmt.Errorf("guard export exceeds bounds")
	}
	return raw, err
}

func (g *LiveGuard) Cancel(authorization GuardAuthorization) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	secret := g.secrets[authorization.RunID]
	if secret == nil || !secretEqual(secret.token, authorization.Token) {
		return fmt.Errorf("current run authorization required")
	}
	record := g.records[authorization.RunID]
	record.State = "canceled"
	g.records[authorization.RunID] = record
	g.revoke(authorization.RunID)
	if err := g.store.saveGuard(record); err != nil {
		return fmt.Errorf("cancellation retention unavailable")
	}
	return nil
}

func (g *LiveGuard) expire(id, token string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	secret := g.secrets[id]
	if secret == nil || !secretEqual(secret.token, token) {
		return
	}
	record := g.records[id]
	record.State = "expired"
	g.records[id] = record
	g.revoke(id)
	_ = g.store.saveGuard(record)
}

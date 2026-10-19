package profileingestion

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"professional-information-repo/internal/preprocessing"
)

// newEntryAnchorPrefix marks a temporary entity anchor that names a brand new
// entry. Its claim-id component is namespaced at the assembly boundary exactly
// like a claim id.
const newEntryAnchorPrefix = "new:"

// submissionContextBatch is one batch's authenticated contribution to the
// assembly handoff. It carries the namespaced claims so reconciliation (#186)
// can read the complete submission, while the wire `context` object only
// reports membership.
type submissionContextBatch struct {
	Index         int                   `json:"index"`
	Primary       []preprocessing.Range `json:"primary"`
	Context       []preprocessing.Range `json:"context"`
	ClaimIDs      []string              `json:"claimIds"`
	Claims        []claim               `json:"claims"`
	SkippedClaims []skippedClaim        `json:"skippedClaims"`
	Coverage      SubmissionCoverage    `json:"coverage"`
}

// submissionContextReceiptPayload is the purpose-separated assembly handoff. It
// binds the exact run/source/Profile/document/plan/version and expires after ten
// minutes. It authenticates transient data; it is not authorization to bypass
// Jev, assert truth or save.
type submissionContextReceiptPayload struct {
	Version     string                   `json:"version"`
	Binding     string                   `json:"binding"`
	ExpiresAt   int64                    `json:"expiresAt"`
	SourceID    string                   `json:"sourceId"`
	ProfileID   string                   `json:"profileId"`
	DocumentID  string                   `json:"documentId,omitempty"`
	RunID       string                   `json:"runId"`
	PlanVersion string                   `json:"planVersion"`
	Batches     []submissionContextBatch `json:"batches"`
	Coverage    SubmissionCoverage       `json:"coverage"`
}

// SubmissionAssemblyResponse is the provider-free assembly result: a signed
// context receipt plus the authenticated aggregate membership ledger.
type SubmissionAssemblyResponse struct {
	Stage          SubmissionStage     `json:"stage"`
	ContextReceipt string              `json:"contextReceipt"`
	ExpiresAt      int64               `json:"expiresAt"`
	Context        SubmissionAggregate `json:"context"`
}

// submissionAssemble verifies the signed plan and the complete set of batch
// receipts before doing anything else. It is entirely provider-free: it makes
// zero Jev/OpenAI calls, stores no backend state and performs no semantic
// deduplication, identity guessing, conflict resolution, source deletion or save.
func (a app) submissionAssemble(ctx context.Context, w http.ResponseWriter, in request, raw []byte, stage SubmissionStage, fail func(int, string)) {
	if code := SubmissionFieldsError(raw, submissionAssemblyFields...); code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	if len(a.signingKey) < 32 {
		fail(503, "configuration")
		return
	}
	if _, ok := submissionDocument(in); !ok {
		fail(400, SubmissionErrInput)
		return
	}
	var planReceipt submissionPlanReceipt
	if code := a.openSubmission(submissionPlanDomain, in.PlanReceipt, &planReceipt); code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	if planReceipt.Version != submissionReceiptVersion || planReceipt.Binding != submissionRequestBinding(in) {
		fail(400, SubmissionErrReceipt)
		return
	}
	if planReceipt.ExpiresAt <= time.Now().UnixMilli() {
		fail(400, SubmissionErrReceiptExpired)
		return
	}
	plan := planReceipt.Plan
	if code := SubmissionManifestError(plan, in.Input); code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	// Assembly requires a complete plan: a gap would mean a silently selected
	// subset of the submission. `submissionManifest` reports a complete plan as
	// `ready` with empty `remaining`/`capacity`, so this completeness check
	// coincides with `status == ready`.
	if !plan.Complete || len(plan.Remaining) != 0 {
		fail(400, SubmissionErrManifest)
		return
	}
	opened, code := a.openSubmissionBatches(in, plan)
	if code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	aggregate, batches, code := submissionAssembly(plan, opened)
	if code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	if ctx.Err() != nil {
		fail(504, "timeout")
		return
	}
	expires := time.Now().Add(SubmissionReceiptTTL).UnixMilli()
	payload := mustJSON(submissionContextReceiptPayload{
		Version: submissionReceiptVersion, Binding: planReceipt.Binding, ExpiresAt: expires,
		SourceID: plan.SourceID, ProfileID: plan.ProfileID, DocumentID: plan.DocumentID,
		RunID: plan.RunID, PlanVersion: plan.PlanVersion,
		Batches: batches, Coverage: aggregate.Coverage,
	})
	receipt, code := a.sealSubmission(submissionContextDomain, payload)
	if code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(SubmissionAssemblyResponse{Stage: stage, ContextReceipt: receipt, ExpiresAt: expires, Context: aggregate})
}

// openSubmissionBatches opens every batch receipt and verifies its version,
// request binding, expiry and full identity/portion binding to the plan. It
// rejects missing, duplicated, extra, mixed, expired, tampered or incompatible
// batches before any further work.
func (a app) openSubmissionBatches(in request, plan SubmissionPlan) ([]submissionBatchReceiptPayload, string) {
	if len(in.BatchReceipts) != len(plan.Batches) {
		return nil, SubmissionErrManifest
	}
	binding := submissionRequestBinding(in)
	now := time.Now().UnixMilli()
	opened := make([]submissionBatchReceiptPayload, len(plan.Batches))
	seen := make([]bool, len(plan.Batches))
	for _, receipt := range in.BatchReceipts {
		var payload submissionBatchReceiptPayload
		if code := a.openSubmission(submissionBatchDomain, receipt, &payload); code != "" {
			return nil, code
		}
		if payload.Version != submissionReceiptVersion || payload.Binding != binding {
			return nil, SubmissionErrReceipt
		}
		if payload.ExpiresAt <= now {
			return nil, SubmissionErrReceiptExpired
		}
		if payload.SourceID != plan.SourceID || payload.ProfileID != plan.ProfileID ||
			payload.DocumentID != plan.DocumentID || payload.RunID != plan.RunID ||
			payload.PlanVersion != plan.PlanVersion {
			return nil, SubmissionErrBatch
		}
		index := payload.BatchIndex
		if index < 0 || index >= len(plan.Batches) {
			return nil, SubmissionErrManifest
		}
		if seen[index] {
			return nil, SubmissionErrManifest
		}
		batch := plan.Batches[index]
		// The receipt must reproduce the plan's exact primary coverage and its
		// distinct structural context; a mixed or incompatible batch is rejected.
		if !sameRanges(payload.Primary, batch.Primary) || !sameRanges(payload.Context, batch.Context) {
			return nil, SubmissionErrBatch
		}
		seen[index] = true
		opened[index] = payload
	}
	for _, present := range seen {
		if !present {
			return nil, SubmissionErrManifest
		}
	}
	return opened, ""
}

// submissionAssembly namespaces every batch-local claim id and builds the
// aggregate membership ledger and the signed-handoff batches. It preserves exact
// occurrences/excerpts, skipped metadata and distinct per-batch context overlaps,
// and it fails rather than returning an aggregate that violates the contract.
func submissionAssembly(plan SubmissionPlan, opened []submissionBatchReceiptPayload) (SubmissionAggregate, []submissionContextBatch, string) {
	aggregateBatches := make([]SubmissionAggregateBatch, 0, len(plan.Batches))
	contextBatches := make([]submissionContextBatch, 0, len(plan.Batches))
	capacity := SubmissionCapacityWithin
	validClaims, invalidClaims := 0, 0
	for index := range plan.Batches {
		payload := opened[index]
		if payload.Coverage.DiscoveryComplete ||
			(payload.Coverage.Capacity != SubmissionCapacityWithin && payload.Coverage.Capacity != SubmissionCapacityExhausted) {
			return SubmissionAggregate{}, nil, SubmissionErrCoverage
		}
		if payload.Coverage.Capacity == SubmissionCapacityExhausted {
			capacity = SubmissionCapacityExhausted
		}
		claims := make([]claim, 0, len(payload.Claims))
		ids := make([]string, 0, len(payload.Claims))
		for _, original := range payload.Claims {
			mapped := namespaceClaim(plan.RunID, index, original)
			claims = append(claims, mapped)
			ids = append(ids, mapped.ID)
		}
		skipped := append([]skippedClaim{}, payload.SkippedClaims...)
		aggregateBatches = append(aggregateBatches, SubmissionAggregateBatch{
			Index: index, Primary: plan.Batches[index].Primary,
			ClaimIDs: ids, ClaimCount: len(claims), SkippedCount: len(skipped),
		})
		contextBatches = append(contextBatches, submissionContextBatch{
			Index: index, Primary: plan.Batches[index].Primary, Context: plan.Batches[index].Context,
			ClaimIDs: ids, Claims: claims, SkippedClaims: skipped, Coverage: payload.Coverage,
		})
		validClaims += len(claims)
		invalidClaims += len(skipped)
	}
	aggregate := SubmissionAggregate{
		ProtocolVersion: SubmissionProtocolVersion,
		SourceID:        plan.SourceID, ProfileID: plan.ProfileID, DocumentID: plan.DocumentID,
		RunID: plan.RunID, PlanVersion: plan.PlanVersion,
		Batches: aggregateBatches,
		// Assembly never proves discovery; capacity degrades if any batch
		// reported exhaustion.
		Coverage: SubmissionCoverage{ValidClaims: validClaims, InvalidClaims: invalidClaims, DiscoveryComplete: false, Capacity: capacity},
	}
	if code := SubmissionAggregateError(aggregate, plan); code != "" {
		return SubmissionAggregate{}, nil, code
	}
	return aggregate, contextBatches, ""
}

// namespaceClaim returns a copy of a batch claim with its local id replaced by
// the deterministic global id. Occurrences, excerpts and metadata are untouched.
func namespaceClaim(runID string, batchIndex int, c claim) claim {
	c.ID = namespaceClaimID(runID, batchIndex, c.ID)
	return c
}

// namespaceClaimID namespaces one batch-local claim id at the assembly boundary.
func namespaceClaimID(runID string, batchIndex int, localID string) string {
	return GlobalClaimID(runID, batchIndex, localID)
}

// namespaceAnchor remaps a temporary entity anchor consistently. An anchor is a
// copied existing Profile entry id (unchanged) or `new:<anchor claim id>`, whose
// claim-id component is namespaced like every other boundary claim id.
func namespaceAnchor(runID string, batchIndex int, anchor string) string {
	if strings.HasPrefix(anchor, newEntryAnchorPrefix) {
		return newEntryAnchorPrefix + namespaceClaimID(runID, batchIndex, strings.TrimPrefix(anchor, newEntryAnchorPrefix))
	}
	return anchor
}

// namespaceClaimReferences remaps claim-id-bearing references (supporting or
// related claim ids) with the same boundary rule as claim ids so references stay
// consistent across the complete submission. Batch receipts carry no such
// references yet; reconciliation (#186) consumes this boundary rule.
func namespaceClaimReferences(runID string, batchIndex int, ids []string) []string {
	if len(ids) == 0 {
		return ids
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.HasPrefix(id, newEntryAnchorPrefix) {
			out = append(out, namespaceAnchor(runID, batchIndex, id))
			continue
		}
		out = append(out, namespaceClaimID(runID, batchIndex, id))
	}
	return out
}

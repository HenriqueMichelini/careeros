package profileingestion

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"professional-information-repo/internal/fieldvalidation"
	"professional-information-repo/internal/preprocessing"
	"professional-information-repo/internal/profiledocument"
)

// submissionReceiptVersion separates whole-submission handoffs from v1 staged
// receipts. It reuses receiptVersion so plan/batch/context handoffs stay pinned
// to the exact workflow versions already bound by the v1 contract.
const submissionReceiptVersion = "profile-ingestion-submission-v1/" + receiptVersion

// submissionAllowedFields names the exact transport keys a submission envelope
// accepts. Any other key is `input` (SubmissionFieldsError); planning/assembly
// never invent or silently drop fields.
var (
	submissionPlanFields     = []string{"stage", "input", "profile", "document"}
	submissionExtractFields  = []string{"stage", "input", "profile", "document", "planReceipt", "batchIndex"}
	submissionAssemblyFields = []string{"stage", "input", "profile", "document", "planReceipt", "batchReceipts"}
)

// submissionPlanReceipt is the authenticated plan handoff. It binds the exact
// request identity and carries the deterministic plan; it is not Jev acceptance,
// not discovery completeness and not permission to save.
type submissionPlanReceipt struct {
	Version   string         `json:"version"`
	Binding   string         `json:"binding"`
	ExpiresAt int64          `json:"expiresAt"`
	Plan      SubmissionPlan `json:"plan"`
}

// submissionBatchReceiptPayload authenticates one bounded extraction result, its
// stable batch identity and its exact source references. It stores no server
// state, so a batch receipt remains client-held and transient.
type submissionBatchReceiptPayload struct {
	Version         string                `json:"version"`
	Binding         string                `json:"binding"`
	ExpiresAt       int64                 `json:"expiresAt"`
	SourceID        string                `json:"sourceId"`
	ProfileID       string                `json:"profileId"`
	DocumentID      string                `json:"documentId,omitempty"`
	RunID           string                `json:"runId"`
	PlanVersion     string                `json:"planVersion"`
	BatchIndex      int                   `json:"batchIndex"`
	Primary         []preprocessing.Range `json:"primary"`
	Context         []preprocessing.Range `json:"context"`
	Claims          []claim               `json:"claims"`
	UnverifiedCount int                   `json:"unverifiedCount"`
	SkippedClaims   []skippedClaim        `json:"skippedClaims"`
	Coverage        SubmissionCoverage    `json:"coverage"`
}

// SubmissionPlanResponse is the provider-free planning result. It deliberately
// carries no `decision`: field acceptance requires the Jev classifier, and
// planning makes zero Jev/OpenAI calls. A signed plan is not Jev acceptance.
type SubmissionPlanResponse struct {
	Stage       SubmissionStage `json:"stage"`
	PlanReceipt string          `json:"planReceipt"`
	ExpiresAt   int64           `json:"expiresAt"`
	Plan        SubmissionPlan  `json:"plan"`
}

// submission dispatches version-2 submission stages. Plan, extract, assembly and
// reconciliation are implemented here; an unknown kind is rejected as `stage`
// without touching v1 behavior. Assembly is provider-free.
func (a app) submission(ctx context.Context, w http.ResponseWriter, r *http.Request, in request, raw []byte, key string, status *int, outcome *string) {
	fail := func(s int, code string) {
		*status = s
		*outcome = code
		writeStageError(w, s, code)
	}
	stage := SubmissionStage{Version: in.Stage.Version, Kind: in.Stage.Kind}
	if code := SubmissionStageError(stage); code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	switch stage.Kind {
	case SubmissionStagePlan:
		a.submissionPlan(ctx, w, in, raw, stage, fail)
	case SubmissionStageExtract:
		a.submissionExtract(ctx, w, r, in, raw, key, stage, status, outcome, fail)
	case SubmissionStageAssemble:
		a.submissionAssemble(ctx, w, in, raw, stage, fail)
	case SubmissionStageReconcile:
		a.submissionReconcile(ctx, w, r, in, raw, key, stage, status, outcome, fail)
	default:
		fail(SubmissionErrorStatus(SubmissionErrStage), SubmissionErrStage)
	}
}

// submissionPlan performs only bounded local work: preparation and the
// deterministic planner. It never contacts a provider.
func (a app) submissionPlan(ctx context.Context, w http.ResponseWriter, in request, raw []byte, stage SubmissionStage, fail func(int, string)) {
	if code := SubmissionFieldsError(raw, submissionPlanFields...); code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	if len(a.signingKey) < 32 {
		fail(503, "configuration")
		return
	}
	documentID, ok := submissionDocument(in)
	if !ok {
		fail(400, SubmissionErrInput)
		return
	}
	prepared, err := preprocessing.PrepareBoundedWithContext(ctx, in.Input, fieldvalidation.ProfessionalInformation)
	if err != nil || prepared.Status != preprocessing.Ready {
		fail(400, "preparation")
		return
	}
	binding := submissionRequestBinding(in)
	_, plan, code := submissionManifest(prepared.Source, canonicalBinding(in.Profile), documentID)
	if code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	// Fail rather than return a plan that violates a manifest invariant.
	if code := SubmissionManifestError(plan, in.Input); code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	if ctx.Err() != nil {
		fail(504, "timeout")
		return
	}
	expires := time.Now().Add(SubmissionReceiptTTL).UnixMilli()
	payload := mustJSON(submissionPlanReceipt{submissionReceiptVersion, binding, expires, plan})
	receipt, code := a.sealSubmission(submissionPlanDomain, payload)
	if code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(SubmissionPlanResponse{Stage: stage, PlanReceipt: receipt, ExpiresAt: expires, Plan: plan})
}

// submissionExtract verifies the plan/source/Profile/document/portion/version
// binding and the signing configuration before dispatch, classifies the exact
// complete original with Jev, then makes exactly one bounded extraction call.
func (a app) submissionExtract(ctx context.Context, w http.ResponseWriter, r *http.Request, in request, raw []byte, key string, stage SubmissionStage, status *int, outcome *string, fail func(int, string)) {
	if code := SubmissionFieldsError(raw, submissionExtractFields...); code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	if len(a.signingKey) < 32 {
		fail(503, "configuration")
		return
	}
	documentID, ok := submissionDocument(in)
	if !ok {
		fail(400, SubmissionErrInput)
		return
	}
	var receipt submissionPlanReceipt
	if code := a.openSubmission(submissionPlanDomain, in.PlanReceipt, &receipt); code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	if receipt.Version != submissionReceiptVersion || receipt.Binding != submissionRequestBinding(in) {
		fail(400, SubmissionErrReceipt)
		return
	}
	if receipt.ExpiresAt <= time.Now().UnixMilli() {
		fail(400, SubmissionErrReceiptExpired)
		return
	}
	plan := receipt.Plan
	if code := SubmissionManifestError(plan, in.Input); code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	if in.BatchIndex == nil || *in.BatchIndex < 0 {
		fail(400, SubmissionErrInput)
		return
	}
	batchIndex := *in.BatchIndex
	if batchIndex >= len(plan.Batches) {
		fail(400, SubmissionErrManifest)
		return
	}
	// Re-derive the exact view deterministically instead of trusting
	// client-supplied ranges; the receipt must match the planner byte-for-byte.
	prepared, err := preprocessing.PrepareBoundedWithContext(ctx, in.Input, fieldvalidation.ProfessionalInformation)
	if err != nil || prepared.Status != preprocessing.Ready {
		fail(400, "preparation")
		return
	}
	planned, derived, code := submissionManifest(prepared.Source, canonicalBinding(in.Profile), documentID)
	if code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	if derived.SourceID != plan.SourceID || derived.ProfileID != plan.ProfileID || derived.DocumentID != plan.DocumentID || derived.PlanVersion != plan.PlanVersion {
		fail(400, SubmissionErrBatch)
		return
	}
	if batchIndex >= len(derived.Batches) {
		fail(400, SubmissionErrManifest)
		return
	}
	if !sameRanges(plan.Batches[batchIndex].Primary, derived.Batches[batchIndex].Primary) ||
		!sameRanges(plan.Batches[batchIndex].Context, derived.Batches[batchIndex].Context) {
		fail(400, SubmissionErrBatch)
		return
	}
	if ctx.Err() != nil {
		fail(504, "timeout")
		return
	}
	decision := fieldvalidation.Classify(ctx, a.client, r.Header.Get("X-TypeSafe-Api-Key"), fieldvalidation.ProfessionalInformation, in.Input)
	if decision.Outcome.Kind != "accept" {
		*outcome = decision.Outcome.Kind
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if decision.Outcome.Kind == "service_failure" {
			*status = codeStatus(decision.Outcome.Reason)
			w.WriteHeader(*status)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"decision": decision})
		return
	}
	view := planned.Portions[batchIndex].View
	ledger := &reconciliationContext{view: &view, skipped: []skippedClaim{}}
	claims, unverified, code := a.extract(ctx, key, prepared.Source, ledger)
	if code != "" {
		fail(codeStatus(code), code)
		return
	}
	if ctx.Err() != nil {
		fail(504, "timeout")
		return
	}
	coverage := submissionCoverage(coverageFor(len(claims), unverified))
	expires := time.Now().Add(SubmissionReceiptTTL).UnixMilli()
	payload := mustJSON(submissionBatchReceiptPayload{
		Version: submissionReceiptVersion, Binding: receipt.Binding, ExpiresAt: expires,
		SourceID: plan.SourceID, ProfileID: plan.ProfileID, DocumentID: plan.DocumentID, RunID: plan.RunID,
		PlanVersion: plan.PlanVersion, BatchIndex: batchIndex,
		Primary: derived.Batches[batchIndex].Primary, Context: derived.Batches[batchIndex].Context,
		Claims: claims, UnverifiedCount: unverified, SkippedClaims: ledger.skipped, Coverage: coverage,
	})
	batchReceipt, code := a.sealSubmission(submissionBatchDomain, payload)
	if code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(SubmissionExtractionResponse{
		Stage: stage, BatchReceipt: batchReceipt, ExpiresAt: expires, BatchIndex: batchIndex,
		Claims: claims, UnverifiedCount: unverified, SkippedClaims: ledger.skipped, Coverage: coverage,
	})
}

// submissionManifest deterministically plans the source and maps it onto the
// signed submission manifest. It performs no provider call.
func submissionManifest(source preprocessing.Source, profileID, documentID string) (preprocessing.Plan, SubmissionPlan, string) {
	planned, err := source.Plan(submissionPlanBudget(), submissionPlanner(source))
	if err != nil {
		return preprocessing.Plan{}, SubmissionPlan{}, "preparation"
	}
	batches := make([]SubmissionBatch, 0, len(planned.Portions))
	for index, portion := range planned.Portions {
		batches = append(batches, SubmissionBatch{
			Index:         index,
			Primary:       append([]preprocessing.Range{}, portion.View.Original...),
			Context:       viewContextRanges(portion.View),
			PreparedBytes: portion.PreparedBytes,
			PayloadBytes:  portion.PayloadBytes,
		})
	}
	// The preprocessing planner reports `ready` only for a complete single view;
	// a complete multi-portion plan carries `insufficient_capacity` with a valid
	// reason. The submission contract treats `status != ready` as an explicitly
	// partial plan (manifest invariant 8), so derive status/capacity from
	// mechanical completeness instead of the single-view status.
	complete := planned.Complete && len(planned.Unprocessed) == 0
	status, capacity := string(preprocessing.InsufficientCapacity), string(planned.Reason)
	if complete {
		status, capacity = string(preprocessing.Ready), ""
	}
	manifest := SubmissionPlan{
		ProtocolVersion: SubmissionProtocolVersion,
		SourceID:        source.ID(),
		ProfileID:       profileID,
		DocumentID:      documentID,
		RunID:           submissionRunID(profileID),
		PlanVersion:     preprocessing.PlanningVersion,
		Status:          status,
		Complete:        complete,
		Capacity:        capacity,
		OriginalBytes:   len(source.Original()),
		PreparedBytes:   planned.PreparedBytes,
		Batches:         batches,
		Remaining:       append([]preprocessing.Range{}, planned.Unprocessed...),
	}
	return planned, manifest, ""
}

// submissionPlanBudget is the retained 200-2,000 prepared-byte portion budget at
// its whole-submission maximum, the full provider envelope and the 256-portion cap.
func submissionPlanBudget() preprocessing.Budget {
	return preprocessing.Budget{PreparedBytes: SubmissionMaxPreparedBytes, PayloadBytes: maxProviderPayload, MaxPortions: SubmissionPortionCap}
}

// submissionPlanner is a pure serializer of the final extraction request. The
// planner evaluates it for candidate views and it never contacts a provider.
func submissionPlanner(source preprocessing.Source) preprocessing.PayloadEncoder {
	return func(view preprocessing.View) ([]byte, error) {
		return providerPayload(extractionPrompt(source, &view), "profile_claims", extractionSchema()), nil
	}
}

// viewContextRanges flattens a view's repeated structural context into original
// source ranges. Context repeats earlier coverage and never adds new coverage.
func viewContextRanges(view preprocessing.View) []preprocessing.Range {
	ranges := []preprocessing.Range{}
	for _, reference := range view.Context {
		ranges = append(ranges, reference.Original...)
	}
	return ranges
}

// submissionRunID names one foreground submission. It is unique per call and
// falls back to the Profile identity only if the system entropy source fails.
func submissionRunID(profileID string) string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	if len(profileID) >= 32 {
		return "run-" + profileID[:32]
	}
	return "run-" + profileID
}

// submissionDocument decodes and matches the optional canonical document, which
// is part of the plan/source/Profile/document identity bound by every receipt.
func submissionDocument(in request) (string, bool) {
	if len(in.Document) == 0 {
		return "", true
	}
	document, err := profiledocument.Decode(in.Document)
	if err != nil || !documentMatchesProfile(document, in.Profile) {
		return "", false
	}
	return document.ID, true
}

// submissionRequestBinding binds the exact input/Profile/document identity and
// ignores every transport field, so plan and extract agree on one handoff.
func submissionRequestBinding(in request) string {
	in.Stage, in.Receipt = nil, ""
	in.ApplyAnswer, in.Clarification, in.Portion = nil, nil, nil
	in.PlanReceipt, in.BatchReceipts, in.ContextReceipt = "", nil, ""
	in.BatchIndex, in.Focus = nil, nil
	return canonicalBinding(in)
}

// submissionCoverage maps the retained claim accounting onto the submission
// coverage envelope. Discovery is never asserted.
func submissionCoverage(coverage processingCoverage) SubmissionCoverage {
	return SubmissionCoverage{ValidClaims: coverage.ValidClaims, InvalidClaims: coverage.InvalidClaims, DiscoveryComplete: coverage.DiscoveryComplete, Capacity: coverage.Capacity}
}

// submissionSignature purpose-separates plan and batch handoffs of the same key.
func submissionSignature(domain string, key, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(domain))
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

// sealSubmission frames a payload as base64(payload).base64(HMAC-SHA256). The
// payload is authenticated, not encrypted, and never exceeds 128 KiB.
func (a app) sealSubmission(domain string, payload []byte) (string, string) {
	if len(payload) > SubmissionReceiptBytes {
		return "", SubmissionErrCapacity
	}
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(submissionSignature(domain, a.signingKey, payload)), ""
}

// openSubmission verifies framing, signature and shape for one handoff domain.
func (a app) openSubmission(domain, receipt string, dst any) string {
	if len(receipt) > base64.RawURLEncoding.EncodedLen(SubmissionReceiptBytes)+1+43 {
		return SubmissionErrCapacity
	}
	parts := strings.Split(receipt, ".")
	if len(parts) != 2 {
		return SubmissionErrReceipt
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil || len(payload) > SubmissionReceiptBytes {
		return SubmissionErrReceipt
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, submissionSignature(domain, a.signingKey, payload)) {
		return SubmissionErrReceipt
	}
	if !strict(payload, dst) {
		return SubmissionErrReceipt
	}
	return ""
}

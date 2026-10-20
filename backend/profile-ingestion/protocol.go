package profileingestion

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	"professional-information-repo/internal/preprocessing"
)

// SubmissionProtocolVersion identifies the whole-submission contract layered on
// the existing /api/profile/ingest route. It is independent of the v1 staged
// protocol (stage.version == 1) and of the legacy one-response protocol.
const SubmissionProtocolVersion = "profile-submission-v1"

// SubmissionStageVersion is the stage.version used by submission stages.
const SubmissionStageVersion = 2

const (
	SubmissionStagePlan      = "plan"
	SubmissionStageExtract   = "extract"
	SubmissionStageAssemble  = "assemble"
	SubmissionStageReconcile = "reconcile"
	SubmissionStageClarify   = "clarify"
)

// Submission budgets. Existing #41/#178 bounds are referenced, not re-declared,
// where a constant already exists. The one new bound is the absolute foreground
// submission cap (see docs/submission-protocol.md).
const (
	SubmissionOriginalBytes      = maxInput
	SubmissionMinPreparedBytes   = 200
	SubmissionMaxPreparedBytes   = 2000
	SubmissionPortionCap         = 256
	SubmissionBatchClaimCap      = 30
	SubmissionBatchOpCap         = 60
	SubmissionReceiptBytes       = maxReceiptPayload
	SubmissionContextBytes       = maxReceiptPayload
	SubmissionExtractBodyBytes   = maxBody
	SubmissionAssemblyBodyBytes  = maxStageBody
	SubmissionReconcileBodyBytes = maxStageBody
	SubmissionClarifyBodyBytes   = maxStageBody
	SubmissionClarifyAnswerBytes = 1000
)

const (
	SubmissionHandlerTimeout = stageTimeout
	SubmissionRequestTimeout = 29 * time.Second
	SubmissionUnitBudget     = 65 * time.Second
	SubmissionForegroundCap  = 300 * time.Second
	SubmissionReceiptTTL     = receiptTTL
)

// Purpose-separated receipt domains. Staged v1 receipts keep their own domain;
// submission plan/batch/context handoffs must never be interchangeable.
const (
	submissionPlanDomain    = "careeros/profile-ingestion/plan/v1\x00"
	submissionBatchDomain   = "careeros/profile-ingestion/batch/v1\x00"
	submissionContextDomain = "careeros/profile-ingestion/context/v1\x00"
)

// Submission error vocabulary. Retained v1 codes (input, capacity, timeout,
// receipt_invalid, receipt_expired, configuration, ...) stay in force; these add
// the submission-specific rejections documented in docs/submission-protocol.md.
const (
	SubmissionErrInput          = "input"
	SubmissionErrVersion        = "version"
	SubmissionErrStage          = "stage"
	SubmissionErrState          = "state"
	SubmissionErrManifest       = "manifest_incomplete"
	SubmissionErrBatch          = "batch_mismatch"
	SubmissionErrContext        = "context_invalid"
	SubmissionErrCoverage       = "coverage_invalid"
	SubmissionErrFocus          = "focus_invalid"
	SubmissionErrUnicode        = "unicode_boundary"
	SubmissionErrCapacity       = "capacity"
	SubmissionErrStale          = "stale"
	SubmissionErrReceipt        = "receipt_invalid"
	SubmissionErrReceiptExpired = "receipt_expired"
)

// Submission coverage capacities.
const (
	SubmissionCapacityExhausted = "possibly_exhausted"
	SubmissionCapacityWithin    = "within_limit"
)

// Submission plan capacity reasons mirror preprocessing.CapacityReason.
var submissionCapacityReasons = []string{
	"original_size", "single_view_capacity", "indivisible_or_context_capacity", "portion_count",
}

// Selection states: pending and selected are transient browser memory; only
// saved work persists through the canonical save.
const (
	SubmissionSelectionPending  = "pending"
	SubmissionSelectionSelected = "selected"
	SubmissionSelectionSaved    = "saved"
)

// submissionSessionStatuses is the client-owned review lifecycle vocabulary.
var submissionSessionStatuses = []string{
	"idle", "planning", "extracting", "assembling", "reconciling", "reviewing",
	"saving", "saved", "paused", "stopped", "failed", "expired", "stale", "discarded",
}

// SubmissionStage is the plan/extract/assemble/reconcile stage envelope.
type SubmissionStage struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
}

// SubmissionExtractionRequest binds one bounded extraction batch to its plan.
type SubmissionExtractionRequest struct {
	PlanReceipt string `json:"planReceipt"`
	BatchIndex  int    `json:"batchIndex"`
}

// SubmissionAssemblyRequest supplies the complete set of extraction receipts.
type SubmissionAssemblyRequest struct {
	PlanReceipt   string   `json:"planReceipt"`
	BatchReceipts []string `json:"batchReceipts"`
}

// SubmissionReconciliationRequest binds one focus batch to the aggregate context.
type SubmissionReconciliationRequest struct {
	ContextReceipt string          `json:"contextReceipt"`
	Focus          SubmissionFocus `json:"focus"`
}

// SubmissionExtractionResponse is one authenticated bounded extraction result.
type SubmissionExtractionResponse struct {
	Stage           SubmissionStage    `json:"stage"`
	BatchReceipt    string             `json:"batchReceipt"`
	ExpiresAt       int64              `json:"expiresAt"`
	BatchIndex      int                `json:"batchIndex"`
	Claims          []claim            `json:"claims"`
	UnverifiedCount int                `json:"unverifiedCount"`
	SkippedClaims   []skippedClaim     `json:"skippedClaims"`
	Coverage        SubmissionCoverage `json:"coverage"`
}

// SubmissionBatchResult is one batch's validated extraction in a submission result.
type SubmissionBatchResult struct {
	Index         int            `json:"index"`
	Claims        []claim        `json:"claims"`
	SkippedClaims []skippedClaim `json:"skippedClaims"`
}

// SubmissionFocusResult is the reconciled output for one focus batch.
type SubmissionFocusResult struct {
	Focus      SubmissionFocus `json:"focus"`
	Operations []operation     `json:"operations"`
	Outcomes   []claimOutcome  `json:"outcomes"`
}

// SubmissionResult is the consolidated in-memory review of one submission.
type SubmissionResult struct {
	ProtocolVersion string                  `json:"protocolVersion"`
	SubmissionID    string                  `json:"submissionId"`
	SourceID        string                  `json:"sourceId"`
	ProfileID       string                  `json:"profileId"`
	DocumentID      string                  `json:"documentId,omitempty"`
	RunID           string                  `json:"runId"`
	Plan            SubmissionPlan          `json:"plan"`
	Batches         []SubmissionBatchResult `json:"batches"`
	Context         SubmissionAggregate     `json:"context"`
	Focus           []SubmissionFocusResult `json:"focus"`
	Coverage        SubmissionCoverage      `json:"coverage"`
}

// SubmissionSelection is one selectable destination unit and its dependencies.
// Status is pending, selected or saved.
type SubmissionSelection struct {
	UnitID       string   `json:"unitId"`
	Status       string   `json:"status"`
	Dependencies []string `json:"dependencies"`
}

// SubmissionSession is transient browser session state. Nothing here is stored.
type SubmissionSession struct {
	ProtocolVersion string                `json:"protocolVersion"`
	SubmissionID    string                `json:"submissionId"`
	Status          string                `json:"status"`
	TotalBatches    int                   `json:"totalBatches"`
	PlacedBatches   []int                 `json:"placedBatches"`
	Selections      []SubmissionSelection `json:"selections"`
}

// SubmissionBatch is one bounded extraction unit. Primary ranges are unique
// coverage; context ranges repeat earlier coverage and never add new regions.
type SubmissionBatch struct {
	Index         int                   `json:"index"`
	Primary       []preprocessing.Range `json:"primary"`
	Context       []preprocessing.Range `json:"context"`
	PreparedBytes int                   `json:"preparedBytes"`
	PayloadBytes  int                   `json:"payloadBytes"`
}

// SubmissionPlan is the deterministic, signed source plan.
type SubmissionPlan struct {
	ProtocolVersion string                `json:"protocolVersion"`
	SourceID        string                `json:"sourceId"`
	ProfileID       string                `json:"profileId"`
	DocumentID      string                `json:"documentId,omitempty"`
	RunID           string                `json:"runId"`
	PlanVersion     string                `json:"planVersion"`
	Status          string                `json:"status"`
	Complete        bool                  `json:"complete"`
	Capacity        string                `json:"capacity,omitempty"`
	OriginalBytes   int                   `json:"originalBytes"`
	PreparedBytes   int                   `json:"preparedBytes"`
	Batches         []SubmissionBatch     `json:"batches"`
	Remaining       []preprocessing.Range `json:"remaining"`
}

// SubmissionAggregateBatch is one batch's authenticated membership in the
// aggregate context. ClaimIDs are global ids assigned at assembly.
type SubmissionAggregateBatch struct {
	Index        int                   `json:"index"`
	Primary      []preprocessing.Range `json:"primary"`
	ClaimIDs     []string              `json:"claimIds"`
	ClaimCount   int                   `json:"claimCount"`
	SkippedCount int                   `json:"skippedCount"`
}

// SubmissionCoverage reports returned-claim accounting, never semantic discovery.
type SubmissionCoverage struct {
	ValidClaims       int    `json:"validClaims"`
	InvalidClaims     int    `json:"invalidClaims"`
	DiscoveryComplete bool   `json:"discoveryComplete"`
	Capacity          string `json:"capacity"`
}

// SubmissionAggregate is the authenticated whole-submission claim ledger.
type SubmissionAggregate struct {
	ProtocolVersion string                     `json:"protocolVersion"`
	SourceID        string                     `json:"sourceId"`
	ProfileID       string                     `json:"profileId"`
	DocumentID      string                     `json:"documentId,omitempty"`
	RunID           string                     `json:"runId"`
	PlanVersion     string                     `json:"planVersion"`
	Batches         []SubmissionAggregateBatch `json:"batches"`
	Coverage        SubmissionCoverage         `json:"coverage"`
}

// SubmissionFocus names the one-to-thirty global claims reconciled in one batch.
type SubmissionFocus struct {
	ClaimIDs []string `json:"claimIds"`
}

// canonicalBinding hashes a value with object keys sorted and array order and
// number literals preserved. It is the server-side request/handoff binding.
func canonicalBinding(v any) string {
	sum := sha256.Sum256(canonicalForm(v))
	return hex.EncodeToString(sum[:])
}

// canonicalForm renders the canonical serialization hashed by canonicalBinding.
func canonicalForm(v any) []byte {
	decoder := json.NewDecoder(bytes.NewReader(mustJSON(v)))
	decoder.UseNumber()
	var canonical any
	_ = decoder.Decode(&canonical)
	return mustJSON(canonical)
}

// SubmissionStageError validates a stage envelope version and kind.
func SubmissionStageError(s SubmissionStage) string {
	if s.Version != SubmissionStageVersion {
		return SubmissionErrVersion
	}
	switch s.Kind {
	case SubmissionStagePlan, SubmissionStageExtract, SubmissionStageAssemble, SubmissionStageReconcile, SubmissionStageClarify:
		return ""
	}
	return SubmissionErrStage
}

// SubmissionFieldsError rejects unknown fields. Callers pass the exact allowed
// field set for the envelope; any extra key is `input`.
func SubmissionFieldsError(raw []byte, allowed ...string) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return SubmissionErrInput
	}
	permitted := map[string]bool{}
	for _, name := range allowed {
		permitted[name] = true
	}
	for name := range fields {
		if !permitted[name] {
			return SubmissionErrInput
		}
	}
	return ""
}

// ValidPortionBudget reports whether a requested portion budget is within the
// retained 200-2,000 prepared-byte range.
func ValidPortionBudget(b int) bool {
	return b >= SubmissionMinPreparedBytes && b <= SubmissionMaxPreparedBytes
}

// PortionBudgetError names the rejection code for an out-of-range portion budget.
func PortionBudgetError(b int) string {
	if ValidPortionBudget(b) {
		return ""
	}
	return SubmissionErrCapacity
}

// ValidSubmissionStatus reports whether a client lifecycle status is known.
func ValidSubmissionStatus(s string) bool {
	for _, known := range submissionSessionStatuses {
		if known == s {
			return true
		}
	}
	return false
}

// SubmissionStatusError names the rejection code for an unknown lifecycle status.
func SubmissionStatusError(s string) string {
	if ValidSubmissionStatus(s) {
		return ""
	}
	return SubmissionErrState
}

// ValidSelectionStatus reports whether a selection state is pending/selected/saved.
func ValidSelectionStatus(s string) bool {
	return s == SubmissionSelectionPending || s == SubmissionSelectionSelected || s == SubmissionSelectionSaved
}

// SubmissionSelectionError names the rejection code for an unknown selection state.
func SubmissionSelectionError(s string) string {
	if ValidSelectionStatus(s) {
		return ""
	}
	return SubmissionErrState
}

// SubmissionCountsError checks the planner portion cap and the per-bounded-result
// claim and operation caps. Aggregate claim totals may exceed the per-batch cap.
func SubmissionCountsError(batches, claims, operations int) string {
	if batches < 0 || claims < 0 || operations < 0 {
		return SubmissionErrInput
	}
	if batches > SubmissionPortionCap || claims > SubmissionBatchClaimCap || operations > SubmissionBatchOpCap {
		return SubmissionErrCapacity
	}
	return ""
}

// GlobalClaimID namespaces a batch-local claim id at the assembly boundary.
func GlobalClaimID(runID string, batchIndex int, localID string) string {
	return runID + "/" + strconv.Itoa(batchIndex) + "/" + localID
}

// SubmissionManifestError verifies a plan against its exact original text.
func SubmissionManifestError(plan SubmissionPlan, original string) string {
	if plan.ProtocolVersion != SubmissionProtocolVersion {
		return SubmissionErrVersion
	}
	if plan.PlanVersion != preprocessing.PlanningVersion {
		return SubmissionErrVersion
	}
	if plan.SourceID == "" || plan.ProfileID == "" || plan.RunID == "" {
		return SubmissionErrManifest
	}
	if plan.Status != string(preprocessing.Ready) && plan.Status != string(preprocessing.InsufficientCapacity) {
		return SubmissionErrState
	}
	if plan.Status == string(preprocessing.InsufficientCapacity) {
		if !contains(submissionCapacityReasons, plan.Capacity) {
			return SubmissionErrState
		}
	} else if plan.Capacity != "" {
		return SubmissionErrState
	}
	if !utf8.ValidString(original) {
		return SubmissionErrUnicode
	}
	total := len(original)
	if plan.OriginalBytes != total {
		return SubmissionErrManifest
	}
	if plan.PreparedBytes < 0 || plan.PreparedBytes > maxPreparedInput {
		return SubmissionErrCapacity
	}
	if len(plan.Batches) > SubmissionPortionCap {
		return SubmissionErrCapacity
	}
	primary := []preprocessing.Range{}
	for i, batch := range plan.Batches {
		if batch.Index != i {
			return SubmissionErrManifest
		}
		if batch.PreparedBytes < 0 || batch.PreparedBytes > SubmissionMaxPreparedBytes || batch.PayloadBytes < 0 || batch.PayloadBytes > maxProviderPayload {
			return SubmissionErrCapacity
		}
		for _, r := range batch.Primary {
			if code := rangeError(r, original, total); code != "" {
				return code
			}
		}
		primary = append(primary, batch.Primary...)
	}
	// Context repeats earlier coverage: it must fall inside some batch's primary
	// range, never introduce new coverage.
	for _, batch := range plan.Batches {
		for _, r := range batch.Context {
			if code := rangeError(r, original, total); code != "" {
				return code
			}
			if !withinAny(r, primary) {
				return SubmissionErrContext
			}
		}
	}
	for _, r := range plan.Remaining {
		if code := rangeError(r, original, total); code != "" {
			return code
		}
	}
	combined := append(append([]preprocessing.Range{}, primary...), plan.Remaining...)
	if !tiles(combined, total) {
		return SubmissionErrManifest
	}
	if plan.Complete != (len(plan.Remaining) == 0) {
		return SubmissionErrManifest
	}
	return ""
}

// SubmissionAggregateError verifies assembly membership and identity binding.
func SubmissionAggregateError(agg SubmissionAggregate, plan SubmissionPlan) string {
	if agg.ProtocolVersion != SubmissionProtocolVersion {
		return SubmissionErrVersion
	}
	if agg.SourceID != plan.SourceID || agg.ProfileID != plan.ProfileID ||
		agg.DocumentID != plan.DocumentID || agg.RunID != plan.RunID ||
		agg.PlanVersion != plan.PlanVersion {
		return SubmissionErrBatch
	}
	if !plan.Complete {
		return SubmissionErrManifest
	}
	if len(agg.Batches) != len(plan.Batches) {
		return SubmissionErrManifest
	}
	ids := map[string]bool{}
	validClaims, invalidClaims := 0, 0
	for i, batch := range agg.Batches {
		if batch.Index != i || batch.Index != plan.Batches[i].Index {
			return SubmissionErrManifest
		}
		if batch.ClaimCount != len(batch.ClaimIDs) || batch.SkippedCount < 0 {
			return SubmissionErrManifest
		}
		if batch.ClaimCount > SubmissionBatchClaimCap {
			return SubmissionErrCapacity
		}
		if !sameRanges(batch.Primary, plan.Batches[i].Primary) {
			return SubmissionErrBatch
		}
		prefix := agg.RunID + "/" + strconv.Itoa(batch.Index) + "/"
		for _, id := range batch.ClaimIDs {
			if id == "" || ids[id] || len(id) <= len(prefix) || id[:len(prefix)] != prefix {
				return SubmissionErrManifest
			}
			ids[id] = true
		}
		validClaims += batch.ClaimCount
		invalidClaims += batch.SkippedCount
	}
	if agg.Coverage.ValidClaims != validClaims || agg.Coverage.InvalidClaims != invalidClaims {
		return SubmissionErrManifest
	}
	if agg.Coverage.DiscoveryComplete ||
		(agg.Coverage.Capacity != SubmissionCapacityWithin && agg.Coverage.Capacity != SubmissionCapacityExhausted) {
		return SubmissionErrCoverage
	}
	if len(mustJSON(agg)) > SubmissionContextBytes {
		return SubmissionErrCapacity
	}
	return ""
}

// SubmissionFocusError verifies that a focus batch names one-to-thirty distinct
// aggregate claims.
func SubmissionFocusError(focus SubmissionFocus, agg SubmissionAggregate) string {
	if len(focus.ClaimIDs) == 0 || len(focus.ClaimIDs) > SubmissionBatchClaimCap {
		return SubmissionErrFocus
	}
	known := map[string]bool{}
	for _, batch := range agg.Batches {
		for _, id := range batch.ClaimIDs {
			known[id] = true
		}
	}
	seen := map[string]bool{}
	for _, id := range focus.ClaimIDs {
		if id == "" || seen[id] || !known[id] {
			return SubmissionErrFocus
		}
		seen[id] = true
	}
	return ""
}

// SubmissionResultError verifies the consolidated review envelope.
func SubmissionResultError(result SubmissionResult) string {
	if result.ProtocolVersion != SubmissionProtocolVersion {
		return SubmissionErrVersion
	}
	if result.Coverage.DiscoveryComplete {
		return SubmissionErrCoverage
	}
	ids := map[string]bool{}
	validClaims, invalidClaims := 0, 0
	for _, batch := range result.Batches {
		for _, c := range batch.Claims {
			if c.ID == "" || ids[c.ID] {
				return SubmissionErrManifest
			}
			ids[c.ID] = true
			validClaims++
		}
		invalidClaims += len(batch.SkippedClaims)
	}
	for _, focus := range result.Focus {
		if code := SubmissionFocusError(focus.Focus, result.Context); code != "" {
			return code
		}
		if len(focus.Operations) > SubmissionBatchOpCap {
			return SubmissionErrCapacity
		}
	}
	if result.Coverage.ValidClaims != validClaims || result.Coverage.InvalidClaims != invalidClaims {
		return SubmissionErrManifest
	}
	return ""
}

// SubmissionSessionError verifies transient session state and its selections.
func SubmissionSessionError(session SubmissionSession) string {
	if session.ProtocolVersion != SubmissionProtocolVersion {
		return SubmissionErrVersion
	}
	if code := SubmissionStatusError(session.Status); code != "" {
		return code
	}
	units := map[string]bool{}
	for _, selection := range session.Selections {
		if selection.UnitID == "" || units[selection.UnitID] {
			return SubmissionErrManifest
		}
		units[selection.UnitID] = true
		if code := SubmissionSelectionError(selection.Status); code != "" {
			return code
		}
	}
	return ""
}

// SubmissionErrorStatus maps a submission error code to its HTTP status.
func SubmissionErrorStatus(code string) int {
	switch code {
	case SubmissionErrInput, SubmissionErrVersion, SubmissionErrStage, SubmissionErrState,
		SubmissionErrManifest, SubmissionErrBatch, SubmissionErrContext, SubmissionErrCoverage,
		SubmissionErrFocus, SubmissionErrUnicode, SubmissionErrCapacity,
		SubmissionErrReceipt, SubmissionErrReceiptExpired:
		return 400
	case "configuration":
		return 503
	case SubmissionErrStale:
		return 409
	case "timeout":
		return 504
	default:
		return codeStatus(code)
	}
}

// rangeError validates one byte range against the exact original text.
func rangeError(r preprocessing.Range, original string, total int) string {
	if r.Start < 0 || r.End > total || r.End <= r.Start {
		return SubmissionErrManifest
	}
	if !runeBoundary(original, r.Start) || !runeBoundary(original, r.End) {
		return SubmissionErrUnicode
	}
	return ""
}

// runeBoundary reports whether i is a UTF-8 rune boundary in s.
func runeBoundary(s string, i int) bool {
	if i < 0 || i > len(s) {
		return false
	}
	return i == len(s) || utf8.RuneStart(s[i])
}

func withinAny(r preprocessing.Range, ranges []preprocessing.Range) bool {
	for _, p := range ranges {
		if p.Start <= r.Start && r.End <= p.End {
			return true
		}
	}
	return false
}

// tiles reports whether ranges cover [0,total) exactly once.
func tiles(ranges []preprocessing.Range, total int) bool {
	sorted := append([]preprocessing.Range{}, ranges...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
	prev := 0
	for _, r := range sorted {
		if r.Start != prev || r.End <= r.Start {
			return false
		}
		prev = r.End
	}
	return prev == total
}

func sameRanges(a, b []preprocessing.Range) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

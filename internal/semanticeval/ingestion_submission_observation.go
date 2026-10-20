package semanticeval

import (
	"strconv"

	ingestion "professional-information-repo/backend/profile-ingestion"
	"professional-information-repo/internal/preprocessing"
)

// SubmissionIngestionEvidence is the returned stage ledger, not an assembled
// success asserted by an adapter. Missing stages remain missing. Receipt bytes
// are retained, but normalization does not authenticate signatures or expiry.
type SubmissionIngestionEvidence struct {
	Plan     IngestionResponseEvidence   `json:"plan"`
	Batches  []IngestionResponseEvidence `json:"batches"`
	Assembly IngestionResponseEvidence   `json:"assembly"`
	Focus    []IngestionResponseEvidence `json:"focus"`
}
type submissionExtractionWire struct {
	Stage           ingestion.SubmissionStage `json:"stage"`
	BatchReceipt    string                    `json:"batchReceipt"`
	ExpiresAt       int64                     `json:"expiresAt"`
	BatchIndex      int                       `json:"batchIndex"`
	Claims          []IngestionClaim          `json:"claims"`
	UnverifiedCount int                       `json:"unverifiedCount"`
	SkippedClaims   []IngestionSkippedClaim   `json:"skippedClaims"`
	Coverage        IngestionCoverage         `json:"coverage"`
}
type submissionFocusWire struct {
	Stage      ingestion.SubmissionStage `json:"stage"`
	Focus      ingestion.SubmissionFocus `json:"focus"`
	Operations []IngestionOperation      `json:"operations"`
	Outcomes   []IngestionDisposition    `json:"outcomes"`
}

// NormalizeSubmissionIngestion mechanically projects actual returned batch and
// focus evidence. It never receives labels and never fills an unreturned stage.
func NormalizeSubmissionIngestion(evidence SubmissionIngestionEvidence) IngestionProposalObservation {
	o := newIngestionObservation(ingestion.SubmissionProtocolVersion)
	o.retain(evidence.Plan)
	for _, r := range evidence.Batches {
		o.retain(r)
	}
	o.retain(evidence.Assembly)
	for _, r := range evidence.Focus {
		o.retain(r)
	}
	var plan ingestion.SubmissionPlanResponse
	var assembly ingestion.SubmissionAssemblyResponse
	planOK := o.decode(evidence.Plan, &plan)
	assemblyOK := o.decode(evidence.Assembly, &assembly)
	if !planOK || !assemblyOK {
		return o
	}
	o.stage(evidence.Plan, plan.Stage, ingestion.SubmissionStagePlan)
	o.stage(evidence.Assembly, assembly.Stage, ingestion.SubmissionStageAssemble)
	if plan.PlanReceipt == "" || assembly.ContextReceipt == "" {
		o.unavailable("missing_receipt", evidence.Assembly.ID, "")
	}
	if !plan.Plan.Complete || plan.Plan.Status != "ready" || len(plan.Plan.Remaining) != 0 || plan.Plan.ProtocolVersion != ingestion.SubmissionProtocolVersion || plan.Plan.PlanVersion != preprocessing.PlanningVersion || plan.Plan.RunID == "" || plan.Plan.SourceID == "" || plan.Plan.ProfileID == "" || len(plan.Plan.Batches) > ingestion.SubmissionPortionCap {
		o.unavailable("incomplete_or_unknown_plan", evidence.Plan.ID, "/plan")
	}
	if code := ingestion.SubmissionAggregateError(assembly.Context, plan.Plan); code != "" {
		o.unavailable("assembly_"+code, evidence.Assembly.ID, "/context")
	}
	o.requireFields(evidence.Plan, evidence.Plan.Body, "", "endpoint", "stage", "planReceipt", "expiresAt", "plan")
	o.requireFields(evidence.Assembly, evidence.Assembly.Body, "", "endpoint", "stage", "contextReceipt", "expiresAt", "context")
	planRaw := rawField(evidence.Plan.Body, "plan")
	o.requireFields(evidence.Plan, planRaw, "/plan", "endpoint", "protocolVersion", "sourceId", "profileId", "runId", "planVersion", "status", "complete", "originalBytes", "preparedBytes", "batches", "remaining")
	o.requireArray(evidence.Plan, planRaw, "/plan", "batches", "endpoint")
	o.requireArray(evidence.Plan, planRaw, "/plan", "remaining", "endpoint")
	capacity := "within_limit"
	parsed := map[int]submissionExtractionWire{}
	responses := map[int]IngestionResponseEvidence{}
	for _, r := range evidence.Batches {
		var batch submissionExtractionWire
		if !o.decode(r, &batch) {
			continue
		}
		o.stage(r, batch.Stage, ingestion.SubmissionStageExtract)
		if batch.BatchReceipt == "" {
			o.unavailable("missing_receipt", r.ID, "/batchReceipt")
		}
		if _, exists := parsed[batch.BatchIndex]; exists || batch.BatchIndex < 0 || batch.BatchIndex >= len(plan.Plan.Batches) {
			o.unavailable("duplicate_or_unknown_batch", r.ID, "/batchIndex")
			continue
		}
		parsed[batch.BatchIndex] = batch
		responses[batch.BatchIndex] = r
	}
	for i := range plan.Plan.Batches {
		batch, exists := parsed[i]
		if !exists {
			o.unavailable("missing_batch", evidence.Plan.ID, "/plan/batches/"+strconv.Itoa(i))
			continue
		}
		r := responses[i]
		o.requireFields(r, r.Body, "", "claims", "stage", "batchReceipt", "expiresAt", "batchIndex", "claims", "unverifiedCount", "skippedClaims", "coverage")
		o.requireArray(r, r.Body, "", "claims", "claims")
		o.requireArray(r, r.Body, "", "skippedClaims", "coverage")
		o.requireFields(r, rawField(r.Body, "coverage"), "/coverage", "coverage", "validClaims", "invalidClaims", "discoveryComplete", "capacity")
		if len(batch.Claims) > ingestion.SubmissionBatchClaimCap {
			o.unavailable("batch_capacity", r.ID, "/claims", "claims")
		}
		o.validateCoverage(r, batch.Coverage, len(batch.Claims), len(batch.SkippedClaims))
		if batch.Coverage.Capacity == "possibly_exhausted" {
			capacity = "possibly_exhausted"
		}
		if batch.UnverifiedCount != len(batch.SkippedClaims) {
			o.unavailable("coverage_mismatch", r.ID, "/unverifiedCount", "coverage")
		}
		o.Coverage = append(o.Coverage, IngestionCoverageObservation{r.ID, batch.Coverage})
		o.skippedMappings(r, batch.SkippedClaims, len(o.SkippedClaims))
		o.SkippedClaims = append(o.SkippedClaims, batch.SkippedClaims...)
		if i >= len(assembly.Context.Batches) {
			o.unavailable("assembly_membership", r.ID, "/claims", "claims")
			continue
		}
		membership := assembly.Context.Batches[i]
		if len(batch.Claims) != len(membership.ClaimIDs) || len(batch.SkippedClaims) != membership.SkippedCount {
			o.unavailable("assembly_membership", r.ID, "/claims", "claims", "coverage")
		}
		seen := map[string]bool{}
		rawClaims := rawIngestionArray(r.Body, "claims")
		o.validateClaimMetadata(r, rawClaims)
		for j, c := range batch.Claims {
			path := ingestionPath("claims", j)
			o.requireFields(r, rawClaims[j], path, "claims", "id", "source", "text", "targets", "question")
			o.requireArray(r, rawClaims[j], path, "targets", "claims")
			if c.ID == "" || seen[c.ID] {
				o.unavailable("duplicate_or_missing_claim_id", r.ID, path, "claims")
			}
			seen[c.ID] = true
			global := ingestion.GlobalClaimID(plan.Plan.RunID, i, c.ID)
			if j >= len(membership.ClaimIDs) || membership.ClaimIDs[j] != global {
				o.unavailable("assembly_membership", r.ID, path, "claims")
				continue
			}
			o.mapping(r.ID, path, "claim", c.ID, global)
			o.mapping(evidence.Assembly.ID, "/context/batches/"+strconv.Itoa(i)+"/claimIds/"+strconv.Itoa(j), "claim", global, global)
			c.ID = global
			o.Claims = append(o.Claims, c)
		}
	}
	coverageRaw := rawField(rawField(evidence.Assembly.Body, "context"), "coverage")
	o.requireFields(evidence.Assembly, coverageRaw, "/context/coverage", "coverage", "validClaims", "invalidClaims", "discoveryComplete", "capacity")
	if assembly.Context.Coverage.Capacity != capacity {
		o.unavailable("coverage_mismatch", evidence.Assembly.ID, "/context/coverage/capacity", "coverage")
	}
	c := assembly.Context.Coverage
	o.Coverage = append(o.Coverage, IngestionCoverageObservation{evidence.Assembly.ID, IngestionCoverage{c.ValidClaims, c.InvalidClaims, c.DiscoveryComplete, c.Capacity}})
	placed := map[string]bool{}
	for _, r := range evidence.Focus {
		var focus submissionFocusWire
		if !o.decode(r, &focus) {
			continue
		}
		o.stage(r, focus.Stage, ingestion.SubmissionStageReconcile)
		o.requireFields(r, r.Body, "", "dispositions", "stage", "focus", "operations", "outcomes")
		o.requireArray(r, r.Body, "", "operations", "operations")
		o.requireArray(r, r.Body, "", "outcomes", "dispositions")
		o.requireArray(r, rawField(r.Body, "focus"), "/focus", "claimIds", "dispositions")
		if code := ingestion.SubmissionFocusError(focus.Focus, assembly.Context); code != "" {
			o.unavailable("focus_membership", r.ID, "/focus", "operations", "dispositions")
		}
		focusClaims := []IngestionClaim{}
		for _, id := range focus.Focus.ClaimIDs {
			if placed[id] {
				o.unavailable("duplicate_focus_claim", r.ID, "/focus", "operations", "dispositions")
			}
			placed[id] = true
			for _, claim := range o.Claims {
				if claim.ID == id {
					focusClaims = append(focusClaims, claim)
				}
			}
			if !hasIngestionClaim(o.Claims, id) {
				o.unavailable("missing_extracted_claim", r.ID, "/focus", "claims", "operations", "dispositions")
			}
		}
		if len(focus.Operations) > ingestion.SubmissionBatchOpCap {
			o.unavailable("focus_capacity", r.ID, "/operations", "operations")
		}
		o.validateRecords(r, focusClaims, focus.Operations, focus.Outcomes, o.Claims)
		offset := len(o.Operations)
		for i, op := range focus.Operations {
			o.Operations = append(o.Operations, op)
			o.mapping(r.ID, ingestionPath("operations", i), "operation", strconv.Itoa(i), "operation:"+strconv.Itoa(offset+i))
		}
		for i, v := range focus.Outcomes {
			for j := range v.OperationIndexes {
				v.OperationIndexes[j] += offset
			}
			o.Outcomes = append(o.Outcomes, v)
			o.mapping(r.ID, ingestionPath("outcomes", i), "disposition", v.ClaimID, v.ClaimID)
			if ingestionUnresolvedKind(v.Kind) {
				o.UnresolvedClaimIDs = append(o.UnresolvedClaimIDs, v.ClaimID)
			}
		}
	}
	for _, claim := range o.Claims {
		if !placed[claim.ID] {
			o.unavailable("missing_focus", evidence.Assembly.ID, "/context/batches", "operations", "dispositions")
		}
	}
	return o
}
func (o *IngestionProposalObservation) stage(r IngestionResponseEvidence, s ingestion.SubmissionStage, kind string) {
	if s.Version != ingestion.SubmissionStageVersion || s.Kind != kind {
		o.unavailable("stage_contract", r.ID, "/stage")
	}
}
func ingestionUnresolvedKind(kind string) bool {
	return kind == "clarification" || kind == "unresolved" || kind == "contradiction" || kind == "correction" || kind == "unsupported"
}

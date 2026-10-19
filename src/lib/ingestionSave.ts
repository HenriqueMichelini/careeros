// Submission-level review validation and canonical save compiler (#189).
//
// One reviewed selection spanning many bounded results compiles into exactly one
// canonical `editProfileBatch`/`saveProfile` transaction. This module validates
// the complete submission and its actual selection against one pinned Profile
// snapshot, resolves the review model's owner/support edges, compiles only the
// explicitly selected operations into one bounded `IngestionResult`, and reuses
// the canonical `applyIngestionDocument`/`saveProfile` seams. There is no
// provider call, no Profile reconstruction from a presentation preview and no
// second writer: a retry re-runs the same pure compile.
//
// It is deliberate that this is the only place the process-wide single-result
// 30/60 cap relaxes: the aggregate assembled from many bounded responses may
// legitimately exceed it while every bounded response still respected it.

import {
  type IngestionClaim,
  type IngestionOperation,
  type IngestionResult,
} from "./ingestion"
import { applyIngestionDocument } from "./ingestionDocument"
import {
  submissionBudgets,
  submissionProtocolVersion,
} from "./ingestionProtocol"
import {
  reviewPlanError,
  reviewSelectionError,
  selectedReviewOperations,
  type ReviewPlan,
  type ReviewSubmission,
} from "./ingestionReview"
import type { ProfileDocument } from "./profileDocument"
import {
  ProfileStorageError,
  saveProfile,
  type ProfileLock,
  type ProfileStorage,
} from "./profileStorage"

// The single pinned review: the submitted aggregate, the canonical snapshot the
// review was built against, the plan that carries the selection, and the whole
// original paste the accepted excerpts are validated against.
export interface SubmissionSaveContext {
  submission: ReviewSubmission
  snapshot: ProfileDocument
  plan: ReviewPlan
  input: string
}

// Storage and the origin lock are injected so tests can supply a fake storage
// and a controlled lock, exactly like profileStorage.ts.
export interface SubmissionSaveStore {
  storage: ProfileStorage
  lock?: ProfileLock
}

export interface SubmissionSaveOutcome {
  status: "saved" | "empty" | "failed"
  // The specific rejection code on failure ("stale", "lock", "storage",
  // "selection_closure", "incompatible_evidence", ...); "" on success/empty.
  code: string
  // True only after the canonical write is durable. The caller may clear the
  // submitted draft only once this is true.
  acknowledged: boolean
  candidate: ProfileDocument | null
}

const fieldKey = (op: IngestionOperation) =>
  `${op.target}\u0000${op.entryId}\u0000${op.field}`

// validateSubmissionSave checks the complete submission and its actual selection
// against one pinned snapshot before any mutation. It preserves the per-response
// 30-claim/60-operation limits per bounded batch (the aggregate may exceed them)
// and rejects incomplete, dangling, contradictory or incompatible selections.
export function validateSubmissionSave(
  submission: ReviewSubmission,
  snapshot: ProfileDocument,
  plan: ReviewPlan,
): string {
  if (
    submission.protocolVersion !== submissionProtocolVersion ||
    plan.protocolVersion !== submissionProtocolVersion
  )
    return "version"
  if (
    plan.submissionId !== submission.submissionId ||
    plan.runId !== submission.runId ||
    plan.sourceId !== submission.sourceId ||
    plan.profileId !== submission.profileId ||
    plan.planVersion !== submission.planVersion
  )
    return "input"
  // The plan pins exactly one Profile snapshot; a different document is stale.
  if (plan.snapshot !== snapshot) return "stale"

  const structural = reviewPlanError(plan)
  if (structural) return structural

  const claims = new Map(submission.claims.map((claim) => [claim.id, claim]))
  const batchOf = new Map<string, number>()
  for (const batch of submission.batches) {
    if (batch.claimIds.length > submissionBudgets.batchClaimCap)
      return "capacity"
    for (const id of batch.claimIds) batchOf.set(id, batch.index)
  }
  const operationsByBatch = new Map<number, number>()
  for (const op of plan.operations) {
    if (!claims.has(op.claimId)) return "dangling_support"
    const index = batchOf.get(op.claimId)
    if (index === undefined) return "manifest_incomplete"
    operationsByBatch.set(index, (operationsByBatch.get(index) ?? 0) + 1)
  }
  for (const count of operationsByBatch.values())
    if (count > submissionBudgets.batchOperationCap) return "capacity"

  const selection = reviewSelectionError(plan)
  if (selection) return selection

  const entities = new Set(
    snapshot.entities.map((entity) => `${entity.kind}\u0000${entity.legacyId}`),
  )
  // A field cannot mix evidence-only support with a wording update: they retain
  // different wording and provenance and must never be folded into one edit.
  const actionByField = new Map<string, "evidence" | "wording">()
  for (const op of selectedReviewOperations(plan)) {
    for (const id of op.supportingClaimIds ?? []) {
      const supporting = claims.get(id)
      if (!supporting || supporting.question) return "dangling_support"
    }
    if (op.entryId && !op.entryId.startsWith("new:")) {
      if (!entities.has(`${op.target}\u0000${op.entryId}`)) return "stale"
    }
    const action = op.action === "evidence" ? "evidence" : "wording"
    const prior = actionByField.get(fieldKey(op))
    if (prior && prior !== action) return "incompatible_evidence"
    actionByField.set(fieldKey(op), action)
  }
  return ""
}

// compileSubmissionSave compiles only the explicitly selected operations into one
// IngestionResult suitable for the canonical `applyIngestionDocument` seam.
//
// Global claim ids are transient pointers (only the excerpt and its accepted
// origin persist) and can exceed the bounded result identity length, so the
// referenced claims are deterministically renumbered and every reference
// (claimId, supportingClaimIds and the `new:` anchor) is rewritten consistently.
// Edited wording keeps its machine proposal in `proposedValue`, so the canonical
// seam records manual provenance and never inherits machine support.
export function compileSubmissionSave(
  submission: ReviewSubmission,
  _snapshot: ProfileDocument,
  plan: ReviewPlan,
): IngestionResult {
  const selected = selectedReviewOperations(plan)
  const claims = new Map(submission.claims.map((claim) => [claim.id, claim]))
  const ids = new Map<string, string>()
  for (const op of selected) {
    for (const id of [op.claimId, ...(op.supportingClaimIds ?? [])])
      if (!ids.has(id)) ids.set(id, `c${ids.size}`)
  }
  const mapped = (id: string) => ids.get(id) ?? id
  const rename = (id: string) =>
    id.startsWith("new:") ? `new:${mapped(id.slice(4))}` : id
  const compiledClaims: IngestionClaim[] = []
  for (const claim of submission.claims) {
    const compact = ids.get(claim.id)
    if (!compact) continue
    compiledClaims.push({
      id: compact,
      source: claim.source,
      text: claim.text,
      targets: [...claim.targets],
      question: claim.question,
      ...(claim.meaning ? { meaning: structuredClone(claim.meaning) } : {}),
      ...(claim.sourceReference
        ? { sourceReference: structuredClone(claim.sourceReference) }
        : {}),
      ...(claim.supportingSources
        ? { supportingSources: structuredClone(claim.supportingSources) }
        : {}),
    })
  }
  const operations: IngestionOperation[] = selected.map((op) => ({
    claimId: mapped(op.claimId),
    target: op.target,
    entryId: rename(op.entryId),
    field: op.field,
    action: op.action,
    value: op.value,
    finding: op.finding,
    approved: true,
    ...(op.proposedValue !== undefined
      ? { proposedValue: op.proposedValue }
      : {}),
    ...(op.supportingClaimIds
      ? { supportingClaimIds: op.supportingClaimIds.map(mapped) }
      : {}),
  }))
  return {
    claims: compiledClaims,
    operations,
    unverifiedClaimCount: 0,
    unresolvedClaimIds: [],
    unplacedOperationCount: 0,
  }
}

function failureCode(error: unknown, fallback: string): string {
  if (error instanceof ProfileStorageError) return error.code
  if (
    error &&
    typeof error === "object" &&
    "code" in error &&
    typeof (error as { code: unknown }).code === "string"
  )
    return (error as { code: string }).code
  return fallback
}

// saveSubmissionSelect performs exactly one canonical transaction: compile the
// selected operations, apply them through `applyIngestionDocument`, then persist
// once through the Web-Lock-protected `saveProfile`. It never mutates the pinned
// snapshot or storage on failure and performs zero provider calls, so Retry is a
// pure re-run. `acknowledged` is true only after the write is durable, so the
// caller clears the submitted draft only then.
export async function saveSubmissionSelect(
  context: SubmissionSaveContext,
  store: SubmissionSaveStore,
): Promise<SubmissionSaveOutcome> {
  const invalid = validateSubmissionSave(
    context.submission,
    context.snapshot,
    context.plan,
  )
  if (invalid)
    return {
      status: "failed",
      code: invalid,
      acknowledged: false,
      candidate: null,
    }
  if (!selectedReviewOperations(context.plan).length)
    return { status: "empty", code: "", acknowledged: false, candidate: null }

  let candidate: ProfileDocument
  try {
    candidate = applyIngestionDocument(
      context.snapshot,
      context.snapshot,
      context.input,
      compileSubmissionSave(context.submission, context.snapshot, context.plan),
      { aggregate: true },
    )
  } catch (failure) {
    return {
      status: "failed",
      code: failureCode(failure, "validation"),
      acknowledged: false,
      candidate: null,
    }
  }
  try {
    await saveProfile(store.storage, context.snapshot, candidate, store.lock)
  } catch (failure) {
    return {
      status: "failed",
      code: failureCode(failure, "storage"),
      acknowledged: false,
      candidate: null,
    }
  }
  return { status: "saved", code: "", acknowledged: true, candidate }
}

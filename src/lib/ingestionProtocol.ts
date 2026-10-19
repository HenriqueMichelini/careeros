// Whole-submission ingestion contract (#182). This module is the TypeScript
// mirror of backend/profile-ingestion/protocol.go. It pins versions, budgets,
// identities, envelopes, invariants and the error vocabulary shared with the
// browser coordinator (#187/#188) and the review/save model (#183/#189).
//
// It performs no feature execution: no transport, no provider call, no storage.

export const submissionProtocolVersion = "profile-submission-v1"
export const submissionStageVersion = 2

export const submissionStages = ["plan", "extract", "assemble", "reconcile"] as const
export type SubmissionStageKind = (typeof submissionStages)[number]

export const submissionBudgets = {
  originalBytes: 30000,
  minPreparedBytes: 200,
  maxPreparedBytes: 2000,
  preparedWorkflowBytes: 60000,
  portionCap: 256,
  batchClaimCap: 30,
  batchOperationCap: 60,
  providerPayloadBytes: 256 << 10,
  handlerTimeoutMs: 27_000,
  requestTimeoutMs: 29_000,
  unitBudgetMs: 65_000,
  foregroundCapMs: 300_000,
  receiptTtlMs: 600_000,
  receiptBytes: 128 << 10,
  contextBytes: 128 << 10,
  extractBodyBytes: 160 << 10,
  assemblyBodyBytes: 320 << 10,
  reconcileBodyBytes: 320 << 10,
} as const

// Submission-specific rejection codes. Retained v1 codes (input, capacity,
// timeout, receipt_invalid, receipt_expired, configuration, ...) stay in force.
export const submissionErrors = {
  input: "input",
  version: "version",
  stage: "stage",
  state: "state",
  manifest: "manifest_incomplete",
  batch: "batch_mismatch",
  context: "context_invalid",
  coverage: "coverage_invalid",
  focus: "focus_invalid",
  unicode: "unicode_boundary",
  capacity: "capacity",
  stale: "stale",
} as const
export type SubmissionErrorCode = (typeof submissionErrors)[keyof typeof submissionErrors]

// Client-owned review lifecycle. pending and selected are transient; only saved
// work persists.
export const submissionStatuses = [
  "idle",
  "planning",
  "extracting",
  "assembling",
  "reconciling",
  "reviewing",
  "saving",
  "saved",
  "paused",
  "stopped",
  "failed",
  "expired",
  "stale",
  "discarded",
] as const
export type SubmissionStatus = (typeof submissionStatuses)[number]

export interface SubmissionRange {
  Start: number
  End: number
}

export interface SubmissionStage {
  version: number
  kind: string
}

export interface SubmissionBatch {
  index: number
  primary: SubmissionRange[]
  context: SubmissionRange[]
  preparedBytes: number
  payloadBytes: number
}

export interface SubmissionPlan {
  protocolVersion: string
  sourceId: string
  profileId: string
  documentId?: string
  runId: string
  planVersion: string
  status: string
  complete: boolean
  capacity?: string
  originalBytes: number
  preparedBytes: number
  batches: SubmissionBatch[]
  remaining: SubmissionRange[]
}

export interface SubmissionAggregateBatch {
  index: number
  primary: SubmissionRange[]
  claimIds: string[]
  claimCount: number
  skippedCount: number
}

export interface SubmissionCoverage {
  validClaims: number
  invalidClaims: number
  discoveryComplete: boolean
  capacity: string
}

export interface SubmissionAggregate {
  protocolVersion: string
  sourceId: string
  profileId: string
  documentId?: string
  runId: string
  planVersion: string
  batches: SubmissionAggregateBatch[]
  coverage: SubmissionCoverage
}

export interface SubmissionFocus {
  claimIds: string[]
}

// GlobalClaimId namespaces a batch-local claim id at the assembly boundary.
export function globalClaimId(runId: string, batchIndex: number, localId: string): string {
  return `${runId}/${batchIndex}/${localId}`
}

const encoder = new TextEncoder()

function utf8Length(value: string): number {
  return encoder.encode(value).length
}

function canonicalJson(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonicalJson).join(",")}]`
  if (value !== null && typeof value === "object")
    return `{${Object.entries(value as Record<string, unknown>)
      .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
      .map(([key, item]) => `${JSON.stringify(key)}:${canonicalJson(item)}`)
      .join(",")}}`
  const encoded = JSON.stringify(value)
  return encoded === undefined ? "null" : encoded
}

// canonicalBinding is the reference request/handoff binding: object key order is
// ignored; array order and number literals are preserved.
export function canonicalBinding(value: unknown): string {
  return canonicalJson(value)
}

export function submissionStageError(stage: SubmissionStage): string {
  if (stage.version !== submissionStageVersion) return submissionErrors.version
  return (submissionStages as readonly string[]).includes(stage.kind) ? "" : submissionErrors.stage
}

export function portionBudgetError(bytes: number): string {
  return bytes >= submissionBudgets.minPreparedBytes && bytes <= submissionBudgets.maxPreparedBytes
    ? ""
    : submissionErrors.capacity
}

export function submissionStatusError(status: string): string {
  return (submissionStatuses as readonly string[]).includes(status) ? "" : submissionErrors.state
}

export function submissionCountsError(counts: {
  batches: number
  claims: number
  operations: number
}): string {
  if (counts.batches < 0 || counts.claims < 0 || counts.operations < 0) return submissionErrors.manifest
  if (
    counts.batches > submissionBudgets.portionCap ||
    counts.claims > submissionBudgets.batchClaimCap ||
    counts.operations > submissionBudgets.batchOperationCap
  )
    return submissionErrors.capacity
  return ""
}

function runeBoundary(bytes: Uint8Array, index: number): boolean {
  if (index < 0 || index > bytes.length) return false
  return index === bytes.length || (bytes[index] & 0xc0) !== 0x80
}

function rangeError(range: SubmissionRange, bytes: Uint8Array): string {
  if (range.Start < 0 || range.End > bytes.length || range.End <= range.Start) return submissionErrors.manifest
  if (!runeBoundary(bytes, range.Start) || !runeBoundary(bytes, range.End)) return submissionErrors.unicode
  return ""
}

function withinAny(range: SubmissionRange, ranges: SubmissionRange[]): boolean {
  return ranges.some((candidate) => candidate.Start <= range.Start && range.End <= candidate.End)
}

function sameRanges(a: SubmissionRange[], b: SubmissionRange[]): boolean {
  return a.length === b.length && a.every((range, index) => range.Start === b[index].Start && range.End === b[index].End)
}

function tiles(ranges: SubmissionRange[], total: number): boolean {
  const sorted = [...ranges].sort((a, b) => a.Start - b.Start)
  let previous = 0
  for (const range of sorted) {
    if (range.Start !== previous || range.End <= range.Start) return false
    previous = range.End
  }
  return previous === total
}

export function submissionManifestError(plan: SubmissionPlan, original: string): string {
  if (plan.protocolVersion !== submissionProtocolVersion) return submissionErrors.version
  if (plan.status !== "ready" && plan.status !== "insufficient_capacity") return submissionErrors.state
  const bytes = encoder.encode(original)
  const total = bytes.length
  if (plan.originalBytes !== total) return submissionErrors.manifest
  if (plan.preparedBytes < 0 || plan.preparedBytes > submissionBudgets.preparedWorkflowBytes)
    return submissionErrors.capacity
  if (plan.batches.length > submissionBudgets.portionCap) return submissionErrors.capacity
  const primary: SubmissionRange[] = []
  for (let index = 0; index < plan.batches.length; index++) {
    const batch = plan.batches[index]
    if (batch.index !== index) return submissionErrors.manifest
    if (
      batch.preparedBytes < 0 ||
      batch.preparedBytes > submissionBudgets.maxPreparedBytes ||
      batch.payloadBytes < 0 ||
      batch.payloadBytes > submissionBudgets.providerPayloadBytes
    )
      return submissionErrors.capacity
    for (const range of batch.primary) {
      const code = rangeError(range, bytes)
      if (code) return code
    }
    primary.push(...batch.primary)
  }
  // Context repeats earlier coverage: it must fall inside some batch's primary
  // range, never introduce new coverage.
  for (const batch of plan.batches)
    for (const range of batch.context) {
      const code = rangeError(range, bytes)
      if (code) return code
      if (!withinAny(range, primary)) return submissionErrors.context
    }
  for (const range of plan.remaining) {
    const code = rangeError(range, bytes)
    if (code) return code
  }
  if (!tiles([...primary, ...plan.remaining], total)) return submissionErrors.manifest
  if (plan.complete !== (plan.remaining.length === 0)) return submissionErrors.manifest
  return ""
}

export function submissionAggregateError(aggregate: SubmissionAggregate, plan: SubmissionPlan): string {
  if (aggregate.protocolVersion !== submissionProtocolVersion) return submissionErrors.version
  if (
    aggregate.sourceId !== plan.sourceId ||
    aggregate.profileId !== plan.profileId ||
    (aggregate.documentId ?? "") !== (plan.documentId ?? "") ||
    aggregate.runId !== plan.runId ||
    aggregate.planVersion !== plan.planVersion
  )
    return submissionErrors.batch
  if (!plan.complete) return submissionErrors.manifest
  if (aggregate.batches.length !== plan.batches.length) return submissionErrors.manifest
  const ids = new Set<string>()
  let validClaims = 0
  let invalidClaims = 0
  for (let index = 0; index < aggregate.batches.length; index++) {
    const batch = aggregate.batches[index]
    if (batch.index !== index || batch.index !== plan.batches[index].index) return submissionErrors.manifest
    if (batch.claimCount !== batch.claimIds.length || batch.skippedCount < 0) return submissionErrors.manifest
    if (batch.claimCount > submissionBudgets.batchClaimCap) return submissionErrors.capacity
    if (!sameRanges(batch.primary, plan.batches[index].primary)) return submissionErrors.batch
    const prefix = `${aggregate.runId}/${batch.index}/`
    for (const id of batch.claimIds) {
      if (!id || ids.has(id) || id.length <= prefix.length || !id.startsWith(prefix)) return submissionErrors.manifest
      ids.add(id)
    }
    validClaims += batch.claimCount
    invalidClaims += batch.skippedCount
  }
  if (aggregate.coverage.validClaims !== validClaims || aggregate.coverage.invalidClaims !== invalidClaims)
    return submissionErrors.manifest
  if (
    aggregate.coverage.discoveryComplete ||
    (aggregate.coverage.capacity !== "within_limit" && aggregate.coverage.capacity !== "possibly_exhausted")
  )
    return submissionErrors.coverage
  if (utf8Length(JSON.stringify(aggregate)) > submissionBudgets.contextBytes) return submissionErrors.capacity
  return ""
}

export function submissionFocusError(focus: SubmissionFocus, aggregate: SubmissionAggregate): string {
  if (focus.claimIds.length === 0 || focus.claimIds.length > submissionBudgets.batchClaimCap)
    return submissionErrors.focus
  const known = new Set<string>()
  for (const batch of aggregate.batches) for (const id of batch.claimIds) known.add(id)
  const seen = new Set<string>()
  for (const id of focus.claimIds) {
    if (!id || seen.has(id) || !known.has(id)) return submissionErrors.focus
    seen.add(id)
  }
  return ""
}

// In-memory foreground submission controller and typed transport (#187/#188).
//
// One `start()` drives a single whole-submission through the bounded v2 stages:
// plan -> sequential extraction batches -> provider-free assembly -> sequential
// reconciliation focus batches -> one validated in-memory review. It reuses the
// existing request/deadline/error seams, keeps every source, receipt and
// unapproved proposal in browser memory and never writes the canonical Profile,
// `localStorage` or `sessionStorage` before explicit approval.
//
// #188 completes the in-memory lifecycle: explicit stop/pause/resume/retry and
// discard, stable per-request handoff identities with retained expiry, staleness
// invalidation that blocks stale saves, and a session registry so the owner
// survives an internal page unmount/remount. A retryable comparison failure
// reuses the still-valid extraction/context; an expired/invalid handoff stops
// with an explicit restart requirement and no provider fallback. Late responses,
// cross-tab updates, repeated clicks and delayed saves cannot overwrite newer
// work. Nothing is ever persisted.
//
// The transport is injectable so controlled tests dispatch the exact envelopes
// and return controlled responses.

import {
  ingestionInputBytes,
  ingestionMaxBytes,
  ingestionOutcomeKinds,
  validProfile,
  IngestionError,
  type IngestionClaim,
  type IngestionOperation,
  type IngestionOutcome,
} from "./ingestion"
import { parseFieldDecision } from "./fieldDecision"
import {
  canonicalForm,
  globalClaimId,
  submissionAggregateError,
  submissionBudgets,
  submissionCapacityExhausted,
  submissionCapacityWithin,
  submissionErrors,
  submissionFieldsError,
  submissionFocusError,
  submissionManifestError,
  submissionProtocolVersion,
  submissionResultError,
  submissionSessionError,
  submissionStageError,
  submissionStageVersion,
  type SubmissionAggregate,
  type SubmissionCoverage,
  type SubmissionFocus,
  type SubmissionFocusResult,
  type SubmissionPlan,
  type SubmissionResult,
  type SubmissionSelectionStatus,
  type SubmissionSession,
  type SubmissionStage,
  type SubmissionStageKind,
  type SubmissionStatus,
} from "./ingestionProtocol"
import { withContactFields } from "./profile"
import type { ProfileDocument } from "./profileDocument"
import type { ProfessionalRepository } from "./types"
import {
  buildReviewPlan,
  reviewSelectionError,
  selectReviewUnit,
  toSubmissionSelections,
  type ReviewPlan,
  type ReviewSubmission,
} from "./ingestionReview"

export const submissionPlanVersion = "portions-v1"

const submissionEndpoint = "/api/profile/ingest"
// Mirrors backend/profile-ingestion openSubmission: RawURLEncoding of a 128 KiB
// payload plus the "." separator and the 32-byte HMAC signature.
const submissionReceiptBound = Math.ceil(submissionBudgets.receiptBytes / 3) * 4 + 1 + 43
const submissionReceiptPattern = /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]{43}$/
const submissionExpirySlackMs = 60_000

// Retained v1 codes plus the submission codes, so a structured error body maps
// to its own code rather than a generic outage.
const submissionTransportCodes: readonly string[] = [
  "input",
  "key",
  "rate_limit",
  "outage",
  "timeout",
  "truncated",
  "invalid_output",
  "preparation",
  "capacity",
  "configuration",
  "receipt_invalid",
  "receipt_expired",
  "cancelled",
  "version",
  "stage",
  "state",
  "manifest_incomplete",
  "batch_mismatch",
  "context_invalid",
  "coverage_invalid",
  "focus_invalid",
  "unicode_boundary",
  "stale",
]

const claimFields = [
  "id",
  "source",
  "text",
  "targets",
  "question",
  "meaning",
  "supportingSources",
  "sourceReference",
  "segmentId",
]
const operationFields = [
  "supportingClaimIds",
  "claimId",
  "target",
  "entryId",
  "field",
  "action",
  "value",
  "finding",
]
const outcomeFields = [
  "claimId",
  "kind",
  "reason",
  "relatedFacts",
  "relatedClaimIds",
  "operationIndexes",
]
const operationActions: readonly string[] = ["add", "update", "remove", "evidence"]
const operationFindings: readonly string[] = ["addition", "overlap", "conflict", "in_place"]

const record = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === "object" && !Array.isArray(value)
const hasKeys = (value: Record<string, unknown>, expected: readonly string[]): boolean =>
  Object.keys(value).length === expected.length && expected.every((key) => key in value)
const strings = (value: unknown): value is string[] =>
  Array.isArray(value) && value.every((item) => typeof item === "string")

function submissionSignalError(signal: AbortSignal): IngestionError {
  if (!signal.aborted) return new IngestionError("outage")
  const reason = signal.reason as { name?: unknown } | undefined
  return new IngestionError(reason?.name === "TimeoutError" ? "timeout" : "cancelled")
}

function combineSignals(...signals: (AbortSignal | undefined)[]): AbortSignal {
  const present = signals.filter((signal): signal is AbortSignal => !!signal)
  if (present.length === 0) return new AbortController().signal
  if (present.length === 1) return present[0]
  return AbortSignal.any(present)
}

// --- Transport seam ---------------------------------------------------------

export interface SubmissionTransportKeys {
  apiKey: string
  typesafeKey: string
}
interface SubmissionBaseBody {
  input: string
  profile: ProfessionalRepository
  document?: ProfileDocument
}
export interface SubmissionPlanBody extends SubmissionBaseBody {
  stage: { version: typeof submissionStageVersion; kind: "plan" }
}
export interface SubmissionExtractBody extends SubmissionBaseBody {
  stage: { version: typeof submissionStageVersion; kind: "extract" }
  planReceipt: string
  batchIndex: number
}
export interface SubmissionAssembleBody extends SubmissionBaseBody {
  stage: { version: typeof submissionStageVersion; kind: "assemble" }
  planReceipt: string
  batchReceipts: string[]
}
export interface SubmissionReconcileBody extends SubmissionBaseBody {
  stage: { version: typeof submissionStageVersion; kind: "reconcile" }
  contextReceipt: string
  focus: SubmissionFocus
}

// Typed per-stage calls. Each POSTs one v2 envelope with the pinned user keys
// and the caller's AbortSignal, and returns the parsed JSON body. The controller
// owns deadline composition and response validation.
export interface SubmissionTransport {
  plan(
    body: SubmissionPlanBody,
    keys: SubmissionTransportKeys,
    signal: AbortSignal,
  ): Promise<unknown>
  extract(
    body: SubmissionExtractBody,
    keys: SubmissionTransportKeys,
    signal: AbortSignal,
  ): Promise<unknown>
  assemble(
    body: SubmissionAssembleBody,
    keys: SubmissionTransportKeys,
    signal: AbortSignal,
  ): Promise<unknown>
  reconcile(
    body: SubmissionReconcileBody,
    keys: SubmissionTransportKeys,
    signal: AbortSignal,
  ): Promise<unknown>
}

function submissionResponseCode(raw: unknown, response: Response): string {
  if (record(raw) && typeof raw.error === "string") {
    const code = raw.error
    if (submissionTransportCodes.includes(code)) return code
  }
  return response.status === 504 ? "timeout" : "outage"
}

// createSubmissionTransport is the production adapter over the retained
// `/api/profile/ingest` route. It performs no validation beyond transport
// framing; the controller validates every response.
export function createSubmissionTransport(
  fetchImpl: typeof fetch = globalThis.fetch,
  endpoint = submissionEndpoint,
): SubmissionTransport {
  const post = async (
    body: object,
    keys: SubmissionTransportKeys,
    signal: AbortSignal,
  ): Promise<unknown> => {
    if (signal.aborted) throw submissionSignalError(signal)
    let response: Response
    let raw: unknown
    try {
      response = await fetchImpl(endpoint, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-OpenAI-Api-Key": keys.apiKey,
          "X-TypeSafe-Api-Key": keys.typesafeKey,
        },
        body: JSON.stringify(body),
        cache: "no-store",
        signal,
      })
      try {
        raw = await response.json()
      } catch {
        if (signal.aborted) throw submissionSignalError(signal)
        raw = null
      }
      if (signal.aborted) throw submissionSignalError(signal)
    } catch (error) {
      throw error instanceof IngestionError ? error : submissionSignalError(signal)
    }
    if (!response.ok) throw new IngestionError(submissionResponseCode(raw, response))
    return raw
  }
  return {
    plan: (body, keys, signal) => post(body, keys, signal),
    extract: (body, keys, signal) => post(body, keys, signal),
    assemble: (body, keys, signal) => post(body, keys, signal),
    reconcile: (body, keys, signal) => post(body, keys, signal),
  }
}

// --- Runtime seam -----------------------------------------------------------

export interface SubmissionRuntime {
  now(): number
  id(): string
  timeout(ms: number): AbortSignal
}
export const submissionRuntime: SubmissionRuntime = {
  now: () => Date.now(),
  id: () => globalThis.crypto.randomUUID(),
  timeout: (ms) => AbortSignal.timeout(ms),
}

// --- Pin --------------------------------------------------------------------

export interface SubmissionSnapshot {
  input: string
  profile: ProfessionalRepository
  document?: ProfileDocument
}

// SubmissionPin is captured once, before any dispatch. Every request body is
// built from the frozen copy so a later change to the caller's input/Profile/
// document cannot leak into an in-flight submission.
export interface SubmissionPin {
  protocolVersion: string
  planVersion: string
  submissionId: string
  input: string
  profile: ProfessionalRepository
  document?: ProfileDocument
  keys: SubmissionTransportKeys
  pinId: string
}

export interface SubmissionPinCandidate {
  input: string
  profile: ProfessionalRepository
  document?: ProfileDocument
  protocolVersion?: string
  planVersion?: string
  keys?: SubmissionTransportKeys
}

// submissionPinError reports whether a candidate snapshot still matches the
// pinned identity. #188 reuses it to invalidate a stale run; the controller
// checks it before every dispatch so a mid-run change stops rather than mixes.
// `keys` is optional so callers that do not hold the credential (and the #182
// contract fixtures) keep comparing identity only; when supplied, a credential
// change is `stale` too.
export function submissionPinError(pin: SubmissionPin, candidate: SubmissionPinCandidate): string {
  if (candidate.protocolVersion !== undefined && candidate.protocolVersion !== pin.protocolVersion)
    return submissionErrors.stale
  if (candidate.planVersion !== undefined && candidate.planVersion !== pin.planVersion)
    return submissionErrors.stale
  if (
    candidate.keys !== undefined &&
    (candidate.keys.apiKey !== pin.keys.apiKey ||
      candidate.keys.typesafeKey !== pin.keys.typesafeKey)
  )
    return submissionErrors.stale
  const pinId = canonicalForm({
    input: candidate.input,
    profile: withContactFields(candidate.profile),
    document: candidate.document ?? null,
  })
  return pinId === pin.pinId ? "" : submissionErrors.stale
}

// --- State ------------------------------------------------------------------

export interface SubmissionProgress {
  status: string
  stage: SubmissionStageKind | ""
  // Primary source coverage: bounded batches planned and actually extracted.
  plannedBatches: number
  extractedBatches: number
  // Returned-claim accounting, never semantic fact discovery.
  claims: number
  skippedClaims: number
  plannedFocusBatches: number
  reconciledFocusBatches: number
  discoveryComplete: false
}

interface SubmissionBatchExtraction {
  index: number
  claims: IngestionClaim[]
  skippedClaims: unknown[]
}

export interface SubmissionState {
  // Protocol lifecycle status (submissionStatuses) plus the recovery reason that
  // distinguishes the otherwise-overlapping terminal states (paused, stopped,
  // timed_out, failed, expired, stale, discarded).
  status: SubmissionStatus
  reason: SubmissionRecoveryReason
  submissionId: string
  pinId: string
  // The retained draft text. Stop, pause, expiry and staleness never erase it.
  input: string
  plan: SubmissionPlan | null
  planReceipt: string
  context: SubmissionAggregate | null
  contextReceipt: string
  batches: SubmissionBatchExtraction[]
  batchReceipts: string[]
  focus: SubmissionFocusResult[]
  coverage: SubmissionCoverage | null
  progress: SubmissionProgress
  error: string
  // Stable identity of the request currently in flight (or the last one).
  requestId: string
  // Every signed handoff this session holds, keyed by the request that produced
  // it. `valid` is cleared when that request is cancelled/superseded or the
  // context changes, so a late response is never reused.
  handoffs: SubmissionHandoff[]
  // Requests whose in-flight handoff was invalidated (cancelled, superseded or
  // invalidated by a stop/pause/discard). Their responses are never recorded.
  cancelledRequestIds: string[]
  // An explicit new start is required: no retained handoff can be reused.
  restartRequired: boolean
  // Input/Profile/document/credential/protocol changed: receipts and results are
  // invalid and a save must be blocked, but the text and edits are retained.
  stale: boolean
  // Whether the retained result is a valid review for the current pin. Stale
  // results stay in memory but are never presented as applicable.
  applicable: boolean
  pendingBatches: number
  pendingFocusBatches: number
  // Monotonic revision of the reviewed selection, used to reject delayed saves.
  selectionRevision: number
}

export const submissionRecoveryReasons = [
  "paused",
  "stopped",
  "timed_out",
  "failed",
  "expired",
  "stale",
  "discarded",
] as const
export type SubmissionRecoveryReason = (typeof submissionRecoveryReasons)[number] | ""

// A signed handoff retained in browser memory with the request identity that
// produced it. Never persisted.
export interface SubmissionHandoff {
  requestId: string
  stage: SubmissionStageKind | ""
  receipt: string
  expiresAt: number
  valid: boolean
}

export interface SubmissionRunOptions {
  signal?: AbortSignal
  // onStage fires when a stage's work actually begins.
  onStage?: (stage: SubmissionStageKind) => void
  // onProgress fires after each bounded unit completes with updated accounting.
  onProgress?: (progress: SubmissionProgress) => void
}

// resume/retry accept the current caller snapshot so a changed input, Profile,
// document, credential or protocol stops the continuation before any dispatch.
export interface SubmissionResumeOptions extends SubmissionRunOptions {
  candidate?: SubmissionPinCandidate
}

// A capability captured before a canonical save. #189 resolves the save; #188
// only blocks a save whose identity or selection moved underneath it.
export interface SubmissionSaveGuard {
  submissionId: string
  pinId: string
  selectionRevision: number
}
export interface SubmissionSaveCheck {
  allowed: boolean
  code: string
}

export interface SubmissionControllerOptions {
  transport: SubmissionTransport
  runtime?: Partial<SubmissionRuntime>
}

function submissionStatusForError(code: string): SubmissionStatus {
  switch (code) {
    case submissionErrors.receiptExpired:
    case submissionErrors.receiptInvalid:
      return "expired"
    case submissionErrors.stale:
      return "stale"
    case "cancelled":
      return "stopped"
    default:
      return "failed"
  }
}

// submissionReasonForError splits the overloaded `failed` status into the
// distinct `timed_out` and `failed` recoveries while keeping stopped/expired/
// stale separate.
function submissionReasonForError(code: string): SubmissionRecoveryReason {
  switch (code) {
    case "cancelled":
      return "stopped"
    case "timeout":
      return "timed_out"
    case submissionErrors.receiptExpired:
    case submissionErrors.receiptInvalid:
      return "expired"
    case submissionErrors.stale:
      return "stale"
    default:
      return "failed"
  }
}

function focusBatches(claimIds: string[]): string[][] {
  const batches: string[][] = []
  for (let index = 0; index < claimIds.length; index += submissionBudgets.batchClaimCap)
    batches.push(claimIds.slice(index, index + submissionBudgets.batchClaimCap))
  return batches
}

// --- Validated response readers ---------------------------------------------

interface SubmissionPlanRead {
  plan: SubmissionPlan
  planReceipt: string
  expiresAt: number
}
interface SubmissionExtractionRead {
  batchReceipt: string
  expiresAt: number
  index: number
  claims: IngestionClaim[]
  skippedClaims: unknown[]
}

function readStage(value: unknown, kind: SubmissionStageKind): string {
  if (!record(value) || !hasKeys(value, ["version", "kind"]) || typeof value.kind !== "string")
    return submissionErrors.stage
  const code = submissionStageError({ version: Number(value.version), kind: value.kind })
  if (code) return code
  return value.kind === kind ? "" : submissionErrors.stage
}

function requireFields(raw: unknown, allowed: string[]): Record<string, unknown> {
  if (submissionFieldsError(raw, allowed) !== "") throw new IngestionError("invalid_output")
  return raw as Record<string, unknown>
}

function rejectDecision(raw: Record<string, unknown>): void {
  if (!("decision" in raw)) return
  const decision = parseFieldDecision(raw.decision, "professional_information")
  if (!decision || !hasKeys(raw, ["decision"])) throw new IngestionError("invalid_output")
  throw new IngestionError("field_decision", decision)
}

function readReceipt(value: unknown): string {
  if (
    typeof value !== "string" ||
    value.length > submissionReceiptBound ||
    !submissionReceiptPattern.test(value)
  )
    throw new IngestionError("invalid_output")
  return value
}

function readExpiry(value: unknown, now: number): number {
  if (
    !Number.isSafeInteger(value) ||
    (value as number) <= now ||
    (value as number) > now + submissionBudgets.receiptTtlMs + submissionExpirySlackMs
  )
    throw new IngestionError("invalid_output")
  return value as number
}

function readPlan(raw: unknown, pin: SubmissionPin, now: number): SubmissionPlanRead {
  const body = requireFields(raw, ["stage", "planReceipt", "expiresAt", "plan"])
  rejectDecision(body)
  const stageError = readStage(body.stage, "plan")
  if (stageError) throw new IngestionError(stageError)
  const planReceipt = readReceipt(body.planReceipt)
  const expiresAt = readExpiry(body.expiresAt, now)
  if (!record(body.plan)) throw new IngestionError("invalid_output")
  const plan = body.plan as unknown as SubmissionPlan
  const manifestError = submissionManifestError(plan, pin.input)
  if (manifestError) throw new IngestionError(manifestError)
  if (plan.protocolVersion !== pin.protocolVersion || plan.planVersion !== pin.planVersion)
    throw new IngestionError(submissionErrors.stale)
  // A partial plan declares a limit and cannot be assembled into a review.
  if (!plan.complete || plan.remaining.length > 0 || plan.status !== "ready")
    throw new IngestionError(submissionErrors.manifest)
  return { plan, planReceipt, expiresAt }
}

function readClaim(value: unknown): IngestionClaim {
  if (submissionFieldsError(value, claimFields) !== "")
    throw new IngestionError("invalid_output")
  const claim = value as Record<string, unknown>
  if (
    typeof claim.id !== "string" ||
    !claim.id ||
    typeof claim.source !== "string" ||
    typeof claim.text !== "string" ||
    typeof claim.question !== "string" ||
    !strings(claim.targets)
  )
    throw new IngestionError("invalid_output")
  return value as unknown as IngestionClaim
}

function readSkipped(value: unknown): unknown {
  if (
    !record(value) ||
    !hasKeys(value, ["index", "reason", "text", "source", "shortened"]) ||
    !Number.isInteger(value.index) ||
    typeof value.reason !== "string" ||
    typeof value.text !== "string" ||
    typeof value.source !== "string" ||
    typeof value.shortened !== "boolean"
  )
    throw new IngestionError("invalid_output")
  return value
}

function readCoverage(value: unknown, claims: number, skipped: number): SubmissionCoverage {
  if (
    !record(value) ||
    !hasKeys(value, ["validClaims", "invalidClaims", "discoveryComplete", "capacity"]) ||
    value.validClaims !== claims ||
    value.invalidClaims !== skipped ||
    value.discoveryComplete !== false ||
    (value.capacity !== submissionCapacityWithin && value.capacity !== submissionCapacityExhausted)
  )
    throw new IngestionError("invalid_output")
  return value as unknown as SubmissionCoverage
}

function readExtraction(raw: unknown, expectedIndex: number, now: number): SubmissionExtractionRead {
  const body = requireFields(raw, [
    "stage",
    "batchReceipt",
    "expiresAt",
    "batchIndex",
    "claims",
    "unverifiedCount",
    "skippedClaims",
    "coverage",
  ])
  rejectDecision(body)
  const stageError = readStage(body.stage, "extract")
  if (stageError) throw new IngestionError(stageError)
  const batchReceipt = readReceipt(body.batchReceipt)
  const expiresAt = readExpiry(body.expiresAt, now)
  if (body.batchIndex !== expectedIndex) throw new IngestionError(submissionErrors.manifest)
  if (
    !Array.isArray(body.claims) ||
    body.claims.length > submissionBudgets.batchClaimCap ||
    !Number.isSafeInteger(body.unverifiedCount) ||
    (body.unverifiedCount as number) < 0 ||
    !Array.isArray(body.skippedClaims) ||
    body.skippedClaims.length !== body.unverifiedCount ||
    body.claims.length + (body.unverifiedCount as number) > submissionBudgets.batchClaimCap
  )
    throw new IngestionError("invalid_output")
  const ids = new Set<string>()
  const claims = body.claims.map((item) => {
    const claim = readClaim(item)
    if (claim.id.length > 40 || ids.has(claim.id)) throw new IngestionError("invalid_output")
    ids.add(claim.id)
    return claim
  })
  const skippedClaims = body.skippedClaims.map(readSkipped)
  readCoverage(body.coverage, claims.length, skippedClaims.length)
  return { batchReceipt, expiresAt, index: expectedIndex, claims, skippedClaims }
}

interface SubmissionAssemblyRead {
  context: SubmissionAggregate
  contextReceipt: string
  expiresAt: number
}

function readAssembly(
  raw: unknown,
  plan: SubmissionPlan,
  pin: SubmissionPin,
  now: number,
): SubmissionAssemblyRead {
  const body = requireFields(raw, ["stage", "contextReceipt", "expiresAt", "context"])
  rejectDecision(body)
  const stageError = readStage(body.stage, "assemble")
  if (stageError) throw new IngestionError(stageError)
  const contextReceipt = readReceipt(body.contextReceipt)
  const expiresAt = readExpiry(body.expiresAt, now)
  if (!record(body.context)) throw new IngestionError("invalid_output")
  const context = body.context as unknown as SubmissionAggregate
  const aggregateError = submissionAggregateError(context, plan)
  if (aggregateError) throw new IngestionError(aggregateError)
  if (
    context.sourceId !== plan.sourceId ||
    context.profileId !== plan.profileId ||
    context.runId !== plan.runId ||
    (context.documentId ?? "") !== (plan.documentId ?? "") ||
    context.planVersion !== pin.planVersion
  )
    throw new IngestionError(submissionErrors.batch)
  return { context, contextReceipt, expiresAt }
}

function readOperation(
  value: unknown,
  focus: Set<string>,
  known: Set<string>,
): IngestionOperation {
  if (submissionFieldsError(value, operationFields) !== "")
    throw new IngestionError("invalid_output")
  const op = value as Record<string, unknown>
  if (
    typeof op.claimId !== "string" ||
    !focus.has(op.claimId) ||
    typeof op.target !== "string" ||
    typeof op.entryId !== "string" ||
    typeof op.field !== "string" ||
    typeof op.value !== "string" ||
    !operationActions.includes(String(op.action)) ||
    !operationFindings.includes(String(op.finding))
  )
    throw new IngestionError("invalid_output")
  if (op.supportingClaimIds !== undefined) {
    if (
      !strings(op.supportingClaimIds) ||
      new Set(op.supportingClaimIds).size !== op.supportingClaimIds.length ||
      op.supportingClaimIds.some((id) => !known.has(id))
    )
      throw new IngestionError("invalid_output")
  }
  return { ...op, proposedValue: op.value, approved: false } as unknown as IngestionOperation
}

function readOutcome(
  value: unknown,
  focus: Set<string>,
  known: Set<string>,
): IngestionOutcome {
  if (submissionFieldsError(value, outcomeFields) !== "")
    throw new IngestionError("invalid_output")
  const outcome = value as Record<string, unknown>
  if (
    typeof outcome.claimId !== "string" ||
    !focus.has(outcome.claimId) ||
    typeof outcome.kind !== "string" ||
    !ingestionOutcomeKinds.includes(outcome.kind as never) ||
    typeof outcome.reason !== "string" ||
    !Array.isArray(outcome.relatedFacts) ||
    !strings(outcome.relatedClaimIds) ||
    outcome.relatedClaimIds.some((id) => !known.has(id)) ||
    outcome.relatedClaimIds.some((id) => id === outcome.claimId)
  )
    throw new IngestionError("invalid_output")
  if (
    outcome.operationIndexes !== undefined &&
    (!Array.isArray(outcome.operationIndexes) ||
      !outcome.operationIndexes.every((index) => Number.isInteger(index)))
  )
    throw new IngestionError("invalid_output")
  return value as unknown as IngestionOutcome
}

function readFocus(
  raw: unknown,
  requested: string[],
  context: SubmissionAggregate,
): SubmissionFocusResult {
  const body = requireFields(raw, ["stage", "focus", "operations", "outcomes"])
  rejectDecision(body)
  const stageError = readStage(body.stage, "reconcile")
  if (stageError) throw new IngestionError(stageError)
  if (!record(body.focus) || !hasKeys(body.focus, ["claimIds"]))
    throw new IngestionError("invalid_output")
  const focus = body.focus as unknown as SubmissionFocus
  if (
    !strings(focus.claimIds) ||
    focus.claimIds.length !== requested.length ||
    focus.claimIds.some((id, index) => id !== requested[index])
  )
    throw new IngestionError(submissionErrors.focus)
  const focusError = submissionFocusError(focus, context)
  if (focusError) throw new IngestionError(focusError)
  if (!Array.isArray(body.operations) || !Array.isArray(body.outcomes))
    throw new IngestionError("invalid_output")
  if (body.operations.length > submissionBudgets.batchOperationCap)
    throw new IngestionError(submissionErrors.capacity)
  const known = new Set<string>()
  for (const batch of context.batches) for (const id of batch.claimIds) known.add(id)
  const focusSet = new Set(focus.claimIds)
  const operations = body.operations.map((item) => readOperation(item, focusSet, known))
  const outcomes = body.outcomes.map((item) => readOutcome(item, focusSet, known))
  return { focus, operations: operations as unknown[], outcomes: outcomes as unknown[] }
}

// --- Controller -------------------------------------------------------------

const emptyProgress = (): SubmissionProgress => ({
  status: "idle",
  stage: "",
  plannedBatches: 0,
  extractedBatches: 0,
  claims: 0,
  skippedClaims: 0,
  plannedFocusBatches: 0,
  reconciledFocusBatches: 0,
  discoveryComplete: false,
})

const emptyState = (): SubmissionState => ({
  status: "idle",
  reason: "",
  submissionId: "",
  pinId: "",
  input: "",
  plan: null,
  planReceipt: "",
  context: null,
  contextReceipt: "",
  batches: [],
  batchReceipts: [],
  focus: [],
  coverage: null,
  progress: emptyProgress(),
  error: "",
  requestId: "",
  handoffs: [],
  cancelledRequestIds: [],
  restartRequired: false,
  stale: false,
  applicable: false,
  pendingBatches: 0,
  pendingFocusBatches: 0,
  selectionRevision: 0,
})

export class SubmissionController {
  private readonly transport: SubmissionTransport
  private readonly runtime: SubmissionRuntime
  private pin: SubmissionPin | null = null
  private source: SubmissionSnapshot | null = null
  private state: SubmissionState = emptyState()
  private result: SubmissionResult | null = null
  private reviewPlan: ReviewPlan | null = null
  private attempt: { token: object; controller: AbortController } | null = null
  private running = false
  private requestSeq = 0

  constructor(options: SubmissionControllerOptions) {
    this.transport = options.transport
    this.runtime = { ...submissionRuntime, ...options.runtime }
  }

  getState(): SubmissionState {
    return this.state
  }
  getPin(): SubmissionPin | null {
    return this.pin
  }
  // getResult only exposes an applicable review: a stale or partial session
  // keeps its in-memory data in getState() but never presents it as savable.
  getResult(): SubmissionResult | null {
    return this.state.applicable ? this.result : null
  }
  // getReviewPlan retains the pinned review identity (stable #183 unit ids) so
  // selection and reviewed edits survive an internal page unmount/remount.
  getReviewPlan(): ReviewPlan | null {
    if (!this.state.applicable || !this.result || !this.pin?.document) return null
    if (!this.reviewPlan) {
      try {
        this.reviewPlan = buildReviewPlan({
          submission: toReviewSubmission(this.result),
          snapshot: this.pin.document,
        })
      } catch {
        return null
      }
    }
    return this.reviewPlan
  }

  // --- Lifecycle -----------------------------------------------------------

  // start pins a fresh session. It is refused while work is in flight or a
  // review is retained (a repeated click cannot replace newer state); after a
  // stop/failure/expiry/staleness/discard it is the explicit restart.
  async start(
    snapshot: SubmissionSnapshot,
    keys: SubmissionTransportKeys,
    options: SubmissionRunOptions = {},
  ): Promise<SubmissionResult> {
    if (this.running || !this.canStart()) throw new IngestionError(submissionErrors.state)
    const pin = this.capturePin(snapshot, keys)
    this.pin = pin
    this.source = snapshot
    this.result = null
    this.reviewPlan = null
    this.state = {
      ...emptyState(),
      submissionId: pin.submissionId,
      pinId: pin.pinId,
      input: pin.input,
      status: "planning",
      progress: { ...emptyProgress(), status: "planning" },
    }
    return this.execute(pin, options)
  }

  // stop aborts the active request (including a body read), prevents further
  // queue dispatch and invalidates the in-flight handoff while retaining the
  // draft, completed validated results and pending work. It stays resumable.
  stop(): void {
    this.settle("stopped", "stopped")
  }

  // pause is the passive "left the focused flow" state: the draft and work are
  // retained for an in-app return; it is distinct from an explicit stop.
  pause(): void {
    this.settle("paused", "paused")
  }

  // resume/retry continue only the unfinished work against the pinned session.
  // A retryable comparison failure therefore reuses the still-valid
  // extraction/context: no new plan, extraction or assembly dispatch.
  async resume(options: SubmissionResumeOptions = {}): Promise<SubmissionResult> {
    return this.continueWork(options)
  }
  async retry(options: SubmissionResumeOptions = {}): Promise<SubmissionResult> {
    return this.continueWork(options)
  }

  // discard is a separate action from stop: it drops the draft and all work.
  discard(): void {
    this.cancelActive()
    this.pin = null
    this.source = null
    this.result = null
    this.reviewPlan = null
    this.state = {
      ...emptyState(),
      status: "discarded",
      reason: "discarded",
      progress: { ...emptyProgress(), status: "discarded" },
    }
  }

  // refresh re-checks a candidate snapshot. A changed input, Profile,
  // document, credential or protocol invalidates the incompatible receipts and
  // result and blocks saves, while the recoverable text and edits stay in
  // memory (never presented as applicable).
  refresh(candidate: SubmissionPinCandidate): string {
    if (!this.pin) return ""
    const code = submissionPinError(this.pin, candidate)
    if (!code) return ""
    this.invalidate(code)
    return code
  }

  // setDraft replaces the retained text. Changing the pasted text invalidates
  // the pinned receipts and result (they described the old text) but keeps the
  // new draft; a late response can never overwrite it.
  setDraft(input: string): string {
    const changed = input !== this.state.input
    this.state.input = input
    if (!changed || !this.pin || input === this.pin.input) return ""
    this.invalidate(submissionErrors.stale)
    return submissionErrors.stale
  }

  saveGuard(): SubmissionSaveGuard {
    return {
      submissionId: this.state.submissionId,
      pinId: this.state.pinId,
      selectionRevision: this.state.selectionRevision,
    }
  }

  // canSave blocks a stale, expired, stopped, paused, discarded or partial
  // session, an invalid selection, and a delayed save whose captured guard no
  // longer matches the session identity or selection revision.
  canSave(guard?: SubmissionSaveGuard): SubmissionSaveCheck {
    if (this.state.stale) return { allowed: false, code: submissionErrors.stale }
    if (this.state.restartRequired)
      return { allowed: false, code: this.state.error || submissionErrors.state }
    if (this.state.status !== "reviewing" && this.state.status !== "saved")
      return { allowed: false, code: submissionErrors.state }
    if (!this.state.applicable) return { allowed: false, code: submissionErrors.stale }
    const plan = this.getReviewPlan()
    if (!plan) return { allowed: false, code: submissionErrors.state }
    const selectionError = reviewSelectionError(plan)
    if (selectionError) return { allowed: false, code: selectionError }
    if (
      guard &&
      (guard.submissionId !== this.state.submissionId ||
        guard.pinId !== this.state.pinId ||
        guard.selectionRevision !== this.state.selectionRevision)
    )
      return { allowed: false, code: submissionErrors.stale }
    return { allowed: true, code: "" }
  }

  // selectUnit retains reviewed edits by their stable #183 unit id across
  // pause/remount; it bumps selectionRevision so a delayed save is rejected.
  selectUnit(unitId: string, status: SubmissionSelectionStatus): void {
    const plan = this.getReviewPlan()
    if (!plan) throw new IngestionError(submissionErrors.state)
    this.reviewPlan = selectReviewUnit(plan, unitId, status)
    this.state.selectionRevision += 1
  }

  // sessionState is the protocol SubmissionSession projection; it is kept valid
  // for the whole lifecycle (submissionSessionError is never weakened).
  sessionState(): SubmissionSession {
    const session: SubmissionSession = {
      protocolVersion: submissionProtocolVersion,
      submissionId: this.state.submissionId,
      status: this.state.status,
      totalBatches: this.state.plan?.batches.length ?? 0,
      placedBatches: this.state.batches.map((batch) => batch.index),
      selections: this.reviewPlan ? toSubmissionSelections(this.reviewPlan) : [],
    }
    const code = submissionSessionError(session)
    if (code) throw new IngestionError(code)
    return session
  }

  // unsaved reports in-memory work whose loss would be real: a retained draft or
  // a completed review that has not been saved. The #181 beforeunload wiring
  // reads this flag; nothing here is ever persisted.
  unsaved(): boolean {
    return this.state.input.trim().length > 0 || this.state.applicable
  }

  // --- Internals -----------------------------------------------------------

  private canStart(): boolean {
    return (
      this.state.status === "idle" ||
      this.state.status === "stopped" ||
      this.state.status === "failed" ||
      this.state.status === "expired" ||
      this.state.status === "stale" ||
      this.state.status === "discarded"
    )
  }

  private capturePin(snapshot: SubmissionSnapshot, keys: SubmissionTransportKeys): SubmissionPin {
    const profile = withContactFields(snapshot.profile)
    if (
      !snapshot.input.trim() ||
      ingestionInputBytes(snapshot.input) > ingestionMaxBytes ||
      !validProfile(profile)
    )
      throw new IngestionError(submissionErrors.input)
    const document = snapshot.document
      ? (structuredClone(snapshot.document) as ProfileDocument)
      : undefined
    const frozenProfile = structuredClone(profile) as ProfessionalRepository
    const pinId = canonicalForm({
      input: snapshot.input,
      profile: frozenProfile,
      document: document ?? null,
    })
    return {
      protocolVersion: submissionProtocolVersion,
      planVersion: submissionPlanVersion,
      submissionId: this.runtime.id(),
      input: snapshot.input,
      profile: frozenProfile,
      document,
      keys: { apiKey: keys.apiKey, typesafeKey: keys.typesafeKey },
      pinId,
    }
  }

  private continueWork(options: SubmissionResumeOptions): Promise<SubmissionResult> {
    const pin = this.pin
    if (!pin || this.running) throw new IngestionError(submissionErrors.state)
    if (options.candidate) {
      const code = submissionPinError(pin, options.candidate)
      if (code) {
        this.invalidate(code)
        throw new IngestionError(code)
      }
    }
    if (this.state.restartRequired || this.state.stale)
      throw new IngestionError(this.state.error || submissionErrors.state)
    if (!this.hasUnfinishedWork()) throw new IngestionError(submissionErrors.state)
    return this.execute(pin, options)
  }

  private hasUnfinishedWork(): boolean {
    if (this.state.applicable) return false
    const plan = this.state.plan
    if (!plan) return true
    if (this.state.batches.length < plan.batches.length) return true
    if (!this.state.context) return true
    const planned = focusBatches(this.state.context.batches.flatMap((batch) => batch.claimIds))
    return this.state.focus.length < planned.length
  }

  private execute(pin: SubmissionPin, options: SubmissionRunOptions): Promise<SubmissionResult> {
    const token = {}
    const controller = new AbortController()
    this.attempt = { token, controller }
    this.running = true
    this.state.error = ""
    this.state.reason = ""
    const absolute = combineSignals(
      options.signal,
      controller.signal,
      this.runtime.timeout(submissionBudgets.foregroundCapMs),
    )
    return (async () => {
      try {
        await this.run(pin, token, absolute, options)
        return this.result as SubmissionResult
      } catch (error) {
        const failure = error instanceof IngestionError ? error : submissionSignalError(absolute)
        if (this.attempt?.token === token) {
          this.state.error = failure.code
          this.state.status = submissionStatusForError(failure.code)
          this.state.reason = submissionReasonForError(failure.code)
          this.state.progress.status = this.state.status
          if (
            failure.code === submissionErrors.receiptExpired ||
            failure.code === submissionErrors.receiptInvalid
          ) {
            this.state.restartRequired = true
            this.invalidateHandoffs()
          }
          this.emitProgress(options)
        }
        throw failure
      } finally {
        if (this.attempt?.token === token) {
          this.attempt = null
          this.running = false
        }
      }
    })()
  }

  // settle stops or pauses only while an attempt is in flight. It aborts the
  // active request and invalidates its handoff; a later stop/pause on an
  // already-settled session (including a completed review) is inert.
  private settle(status: SubmissionStatus, reason: SubmissionRecoveryReason): void {
    if (!this.running) return
    this.cancelActive()
    this.state.status = status
    this.state.reason = reason
    this.state.progress.status = status
  }

  // cancelActive invalidates the in-flight handoff (so a late response is never
  // recorded or reused) and aborts the active request body read included.
  private cancelActive(): void {
    const requestId = this.state.requestId
    if (requestId && !this.state.cancelledRequestIds.includes(requestId)) {
      this.state.cancelledRequestIds = [...this.state.cancelledRequestIds, requestId]
      for (const handoff of this.state.handoffs)
        if (handoff.requestId === requestId) handoff.valid = false
    }
    this.attempt?.controller.abort(new DOMException("Cancelled", "AbortError"))
    this.attempt = null
    this.running = false
  }

  private invalidate(code: string): void {
    this.cancelActive()
    this.state.stale = true
    this.state.applicable = false
    this.state.status = "stale"
    this.state.reason = "stale"
    this.state.error = code
    this.state.progress.status = "stale"
    this.state.restartRequired = true
    this.invalidateHandoffs()
  }

  private invalidateHandoffs(): void {
    for (const handoff of this.state.handoffs) handoff.valid = false
  }

  private mark(status: SubmissionStatus): void {
    this.state.status = status
    this.state.progress.status = status
  }

  private assertActive(token: object): void {
    if (this.attempt?.token !== token) throw new IngestionError("cancelled")
  }

  private nextRequestId(): string {
    this.requestSeq += 1
    return `${this.state.submissionId}:${this.requestSeq}`
  }

  private recordHandoff(handoff: SubmissionHandoff): void {
    if (this.state.cancelledRequestIds.includes(handoff.requestId)) return
    this.state.handoffs = [...this.state.handoffs, handoff]
  }

  private handoffFor(stage: SubmissionStageKind): SubmissionHandoff | undefined {
    return [...this.state.handoffs].reverse().find((handoff) => handoff.stage === stage)
  }

  // requireHandoff refuses to reuse an expired or invalidated receipt: the
  // session must restart explicitly rather than fall back to a fresh plan.
  private requireHandoff(stage: SubmissionStageKind): SubmissionHandoff {
    const handoff = this.handoffFor(stage)
    if (!handoff || !handoff.valid || handoff.expiresAt <= this.runtime.now())
      throw new IngestionError(submissionErrors.receiptExpired)
    return handoff
  }

  // assertPin stops the queue rather than mixing if the pinned input, Profile,
  // document or version changed since the start action.
  private assertPin(pin: SubmissionPin): void {
    if (!this.source) return
    const code = submissionPinError(pin, {
      input: this.source.input,
      profile: this.source.profile,
      document: this.source.document,
    })
    if (code) throw new IngestionError(code)
  }

  private emitStage(options: SubmissionRunOptions, stage: SubmissionStageKind): void {
    this.state.progress.stage = stage
    options.onStage?.(stage)
  }

  private emitProgress(options: SubmissionRunOptions): void {
    options.onProgress?.({ ...this.state.progress })
  }

  // send composes the caller signal, the attempt's stop/pause signal, the
  // absolute foreground cap and the per-request/unit deadlines for exactly one
  // bounded dispatch. It refuses to start if the attempt was superseded, so a
  // stop prevents any further queue dispatch.
  private async send<T>(
    token: object,
    absolute: AbortSignal,
    requestId: string,
    call: (signal: AbortSignal) => Promise<T>,
  ): Promise<T> {
    this.assertActive(token)
    this.state.requestId = requestId
    const unit = combineSignals(absolute, this.runtime.timeout(submissionBudgets.unitBudgetMs))
    const request = combineSignals(unit, this.runtime.timeout(submissionBudgets.requestTimeoutMs))
    try {
      const value = await call(request)
      this.assertActive(token)
      return value
    } catch (error) {
      throw error instanceof IngestionError ? error : submissionSignalError(request)
    }
  }

  private planBody(pin: SubmissionPin): SubmissionPlanBody {
    return {
      stage: { version: submissionStageVersion, kind: "plan" },
      input: pin.input,
      profile: pin.profile,
      ...(pin.document ? { document: pin.document } : {}),
    }
  }
  private extractBody(
    pin: SubmissionPin,
    planReceipt: string,
    index: number,
  ): SubmissionExtractBody {
    return {
      stage: { version: submissionStageVersion, kind: "extract" },
      input: pin.input,
      profile: pin.profile,
      ...(pin.document ? { document: pin.document } : {}),
      planReceipt,
      batchIndex: index,
    }
  }
  private assembleBody(
    pin: SubmissionPin,
    planReceipt: string,
    batchReceipts: string[],
  ): SubmissionAssembleBody {
    return {
      stage: { version: submissionStageVersion, kind: "assemble" },
      input: pin.input,
      profile: pin.profile,
      ...(pin.document ? { document: pin.document } : {}),
      planReceipt,
      batchReceipts,
    }
  }
  private reconcileBody(
    pin: SubmissionPin,
    contextReceipt: string,
    focus: string[],
  ): SubmissionReconcileBody {
    return {
      stage: { version: submissionStageVersion, kind: "reconcile" },
      input: pin.input,
      profile: pin.profile,
      ...(pin.document ? { document: pin.document } : {}),
      contextReceipt,
      focus: { claimIds: focus },
    }
  }

  private async run(
    pin: SubmissionPin,
    token: object,
    absolute: AbortSignal,
    options: SubmissionRunOptions,
  ): Promise<void> {
    // Plan: provider-free, one request, no provider fan-out. Skipped when a
    // still-valid plan is already retained (resume/retry never re-plans).
    if (!this.state.plan) {
      this.emitStage(options, "plan")
      this.mark("planning")
      const requestId = this.nextRequestId()
      const planRaw = await this.send(token, absolute, requestId, (signal) =>
        this.transport.plan(this.planBody(pin), pin.keys, signal),
      )
      const read = readPlan(planRaw, pin, this.runtime.now())
      this.recordHandoff({
        requestId,
        stage: "plan",
        receipt: read.planReceipt,
        expiresAt: read.expiresAt,
        valid: true,
      })
      this.state.plan = read.plan
      this.state.planReceipt = read.planReceipt
      this.state.pendingBatches = read.plan.batches.length
      this.state.progress.plannedBatches = read.plan.batches.length
      this.emitProgress(options)
    }
    const plan = this.state.plan

    // Sequential extraction: exactly the unfinished planned batches, in order.
    if (this.state.batches.length < plan.batches.length) {
      this.mark("extracting")
      this.requireHandoff("plan")
      for (const batch of plan.batches) {
        if (this.state.batches.some((placed) => placed.index === batch.index)) continue
        this.assertActive(token)
        this.assertPin(pin)
        this.emitStage(options, "extract")
        const requestId = this.nextRequestId()
        const raw = await this.send(token, absolute, requestId, (signal) =>
          this.transport.extract(
            this.extractBody(pin, this.state.planReceipt, batch.index),
            pin.keys,
            signal,
          ),
        )
        const extraction = readExtraction(raw, batch.index, this.runtime.now())
        this.recordHandoff({
          requestId,
          stage: "extract",
          receipt: extraction.batchReceipt,
          expiresAt: extraction.expiresAt,
          valid: true,
        })
        const namespaced = extraction.claims.map((claim) => ({
          ...claim,
          id: globalClaimId(plan.runId, batch.index, claim.id),
        }))
        this.state.batches.push({
          index: batch.index,
          claims: namespaced,
          skippedClaims: extraction.skippedClaims,
        })
        this.state.batchReceipts.push(extraction.batchReceipt)
        this.state.pendingBatches = plan.batches.length - this.state.batches.length
        this.state.progress.extractedBatches = this.state.batches.length
        this.state.progress.claims += namespaced.length
        this.state.progress.skippedClaims += extraction.skippedClaims.length
        this.emitProgress(options)
      }
    }

    // Provider-free assembly over the complete batch receipt set. Skipped when
    // a still-valid context is already retained.
    if (!this.state.context) {
      this.assertActive(token)
      this.assertPin(pin)
      this.requireHandoff("plan")
      for (const handoff of this.state.handoffs)
        if (handoff.stage === "extract" && (!handoff.valid || handoff.expiresAt <= this.runtime.now()))
          throw new IngestionError(submissionErrors.receiptExpired)
      this.mark("assembling")
      this.emitStage(options, "assemble")
      const requestId = this.nextRequestId()
      const assemblyRaw = await this.send(token, absolute, requestId, (signal) =>
        this.transport.assemble(
          this.assembleBody(pin, this.state.planReceipt, [...this.state.batchReceipts]),
          pin.keys,
          signal,
        ),
      )
      const read = readAssembly(assemblyRaw, plan, pin, this.runtime.now())
      this.assertAssembly(read.context)
      this.recordHandoff({
        requestId,
        stage: "assemble",
        receipt: read.contextReceipt,
        expiresAt: read.expiresAt,
        valid: true,
      })
      this.state.context = read.context
      this.state.contextReceipt = read.contextReceipt
      this.state.coverage = read.context.coverage
    }
    const context = this.state.context

    // Sequential reconciliation focus batches, bounded to 30 claims each. Only
    // the unreconciled focus batches are dispatched on a resume/retry.
    const planned = focusBatches(context.batches.flatMap((batch) => batch.claimIds))
    this.state.progress.plannedFocusBatches = planned.length
    this.state.pendingFocusBatches = planned.length - this.state.focus.length
    if (this.state.focus.length < planned.length) {
      this.mark("reconciling")
      this.requireHandoff("assemble")
      for (let index = this.state.focus.length; index < planned.length; index++) {
        const focus = planned[index]
        this.assertActive(token)
        this.assertPin(pin)
        this.emitStage(options, "reconcile")
        const requestId = this.nextRequestId()
        const raw = await this.send(token, absolute, requestId, (signal) =>
          this.transport.reconcile(
            this.reconcileBody(pin, this.state.contextReceipt, focus),
            pin.keys,
            signal,
          ),
        )
        this.state.focus.push(readFocus(raw, focus, context))
        this.state.pendingFocusBatches = planned.length - this.state.focus.length
        this.state.progress.reconciledFocusBatches = this.state.focus.length
        this.emitProgress(options)
      }
    }

    this.assertActive(token)
    const result = this.buildResult(pin, plan, context)
    this.assertResult(result)
    this.result = result
    this.state.applicable = true
    this.state.stale = false
    this.state.restartRequired = false
    this.state.error = ""
    this.state.reason = ""
    this.mark("reviewing")
    this.emitProgress(options)
  }

  // assertAssembly verifies the global reference/coverage relation: the
  // assembled global claim ids must equal the namespaced extraction ids exactly.
  private assertAssembly(context: SubmissionAggregate): void {
    if (context.batches.length !== this.state.batches.length)
      throw new IngestionError(submissionErrors.manifest)
    for (const batch of context.batches) {
      const placed = this.state.batches.find((item) => item.index === batch.index)
      if (!placed) throw new IngestionError(submissionErrors.manifest)
      const expected = placed.claims.map((claim) => claim.id)
      if (
        batch.claimIds.length !== expected.length ||
        batch.claimIds.some((id, index) => id !== expected[index]) ||
        batch.claimCount !== expected.length ||
        batch.skippedCount !== placed.skippedClaims.length
      )
        throw new IngestionError(submissionErrors.manifest)
    }
  }

  private buildResult(
    pin: SubmissionPin,
    plan: SubmissionPlan,
    context: SubmissionAggregate,
  ): SubmissionResult {
    return {
      protocolVersion: submissionProtocolVersion,
      submissionId: pin.submissionId,
      sourceId: plan.sourceId,
      profileId: plan.profileId,
      ...(plan.documentId ? { documentId: plan.documentId } : {}),
      runId: plan.runId,
      plan,
      batches: this.state.batches.map((batch) => ({
        index: batch.index,
        claims: batch.claims,
        skippedClaims: batch.skippedClaims,
      })),
      context,
      focus: [...this.state.focus],
      coverage: context.coverage,
    }
  }

  // assertResult enforces the consolidated invariant plus the global outcome
  // uniqueness the review model requires, before declaring review readiness.
  private assertResult(result: SubmissionResult): void {
    const code = submissionResultError(result)
    if (code) throw new IngestionError(code)
    const claims = new Set<string>()
    for (const batch of result.batches) for (const claim of batch.claims) claims.add(claim.id)
    const outcomes = new Set<string>()
    for (const focus of result.focus)
      for (const outcome of focus.outcomes as { claimId: string }[]) {
        if (outcomes.has(outcome.claimId) || !claims.has(outcome.claimId))
          throw new IngestionError(submissionErrors.manifest)
        outcomes.add(outcome.claimId)
      }
  }
}

// --- Review handoff ---------------------------------------------------------

// toReviewSubmission maps one validated SubmissionResult onto the #183 review
// seam. Operations already carry global claim ids and `approved: false`; the
// review model owns all selection state.
export function toReviewSubmission(result: SubmissionResult): ReviewSubmission {
  const claims = result.batches.flatMap((batch) => batch.claims as IngestionClaim[])
  const operations = result.focus.flatMap((focus) => focus.operations as IngestionOperation[])
  const outcomes = result.focus.flatMap((focus) => focus.outcomes as IngestionOutcome[])
  const outcomeByClaim = new Map(outcomes.map((outcome) => [outcome.claimId, outcome]))
  const operatedClaims = new Set(operations.map((operation) => operation.claimId))
  const unresolvedClaimIds = claims
    .filter(
      (claim) =>
        outcomeByClaim.get(claim.id)?.kind === "unresolved" ||
        (!outcomeByClaim.has(claim.id) && !operatedClaims.has(claim.id)),
    )
    .map((claim) => claim.id)
  return {
    protocolVersion: result.protocolVersion,
    submissionId: result.submissionId,
    runId: result.runId,
    sourceId: result.sourceId,
    profileId: result.profileId,
    ...(result.documentId ? { documentId: result.documentId } : {}),
    planVersion: result.plan.planVersion,
    batches: result.context.batches.map((batch) => ({
      index: batch.index,
      claimIds: [...batch.claimIds],
    })),
    claims,
    operations,
    outcomes,
    unresolvedClaimIds,
  }
}

export function createSubmissionController(
  options: SubmissionControllerOptions,
): SubmissionController {
  return new SubmissionController(options)
}

// --- Session registry -------------------------------------------------------

// The owner outlives an internal page's unmount/remount: the registry returns
// the same in-memory session for a stable key so a component-level remount can
// neither lose nor duplicate it. Nothing here ever touches storage; the whole
// paste, receipts and reviewed edits stay in module memory.
const submissionSessions = new Map<string, SubmissionController>()

export function submissionSessionOwner(
  key: string,
  options: SubmissionControllerOptions,
): SubmissionController {
  const existing = submissionSessions.get(key)
  if (existing) return existing
  const created = new SubmissionController(options)
  submissionSessions.set(key, created)
  return created
}

export function submissionSession(key: string): SubmissionController | undefined {
  return submissionSessions.get(key)
}

// discard drops the draft in place; release also forgets the owner so a later
// mount starts a fresh session.
export function releaseSubmissionSession(key: string): void {
  submissionSessions.delete(key)
}

export function submissionSessionKeys(): string[] {
  return [...submissionSessions.keys()]
}

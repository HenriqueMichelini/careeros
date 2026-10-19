// In-memory foreground submission controller and typed transport (#187).
//
// One `start()` drives a single whole-submission through the bounded v2 stages:
// plan -> sequential extraction batches -> provider-free assembly -> sequential
// reconciliation focus batches -> one validated in-memory review. It reuses the
// existing request/deadline/error seams, keeps every source, receipt and
// unapproved proposal in browser memory and never writes the canonical Profile,
// `localStorage` or `sessionStorage` before explicit approval.
//
// The transport is injectable so controlled tests dispatch the exact envelopes
// and return controlled responses. Recovery/invalidation is #188: this module
// only leaves the pin and retained-partial-state seams it needs.

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
  submissionStageError,
  submissionStageVersion,
  type SubmissionAggregate,
  type SubmissionCoverage,
  type SubmissionFocus,
  type SubmissionFocusResult,
  type SubmissionPlan,
  type SubmissionResult,
  type SubmissionStage,
  type SubmissionStageKind,
} from "./ingestionProtocol"
import { withContactFields } from "./profile"
import type { ProfileDocument } from "./profileDocument"
import type { ProfessionalRepository } from "./types"
import type { ReviewSubmission } from "./ingestionReview"

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
}

// submissionPinError reports whether a candidate snapshot still matches the
// pinned identity. #188 reuses it to invalidate a stale run; the controller
// checks it before every dispatch so a mid-run change stops rather than mixes.
export function submissionPinError(pin: SubmissionPin, candidate: SubmissionPinCandidate): string {
  if (candidate.protocolVersion !== undefined && candidate.protocolVersion !== pin.protocolVersion)
    return submissionErrors.stale
  if (candidate.planVersion !== undefined && candidate.planVersion !== pin.planVersion)
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
  status: string
  submissionId: string
  pinId: string
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
}

export interface SubmissionRunOptions {
  signal?: AbortSignal
  // onStage fires when a stage's work actually begins.
  onStage?: (stage: SubmissionStageKind) => void
  // onProgress fires after each bounded unit completes with updated accounting.
  onProgress?: (progress: SubmissionProgress) => void
}

export interface SubmissionControllerOptions {
  transport: SubmissionTransport
  runtime?: Partial<SubmissionRuntime>
}

function submissionStatusForError(code: string): string {
  switch (code) {
    case submissionErrors.receiptExpired:
      return "expired"
    case submissionErrors.stale:
      return "stale"
    case "cancelled":
      return "stopped"
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
  submissionId: "",
  pinId: "",
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
})

export class SubmissionController {
  private readonly transport: SubmissionTransport
  private readonly runtime: SubmissionRuntime
  private pin: SubmissionPin | null = null
  private source: SubmissionSnapshot | null = null
  private state: SubmissionState = emptyState()
  private result: SubmissionResult | null = null

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
  getResult(): SubmissionResult | null {
    return this.result
  }

  async start(
    snapshot: SubmissionSnapshot,
    keys: SubmissionTransportKeys,
    options: SubmissionRunOptions = {},
  ): Promise<SubmissionResult> {
    if (this.state.status !== "idle") throw new IngestionError(submissionErrors.state)
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
    const pin: SubmissionPin = {
      protocolVersion: submissionProtocolVersion,
      planVersion: submissionPlanVersion,
      submissionId: this.runtime.id(),
      input: snapshot.input,
      profile: frozenProfile,
      document,
      keys: { apiKey: keys.apiKey, typesafeKey: keys.typesafeKey },
      pinId,
    }
    this.pin = pin
    this.source = snapshot
    this.state = { ...emptyState(), submissionId: pin.submissionId, pinId, status: "planning" }
    this.state.progress.status = "planning"
    const absolute = combineSignals(
      options.signal,
      this.runtime.timeout(submissionBudgets.foregroundCapMs),
    )
    try {
      await this.run(pin, absolute, options)
    } catch (error) {
      const failure = error instanceof IngestionError ? error : submissionSignalError(absolute)
      this.state.error = failure.code
      this.state.status = submissionStatusForError(failure.code)
      this.state.progress.status = this.state.status
      this.emitProgress(options)
      throw failure
    }
    return this.result as SubmissionResult
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

  private async send<T>(
    absolute: AbortSignal,
    call: (signal: AbortSignal) => Promise<T>,
  ): Promise<T> {
    const unit = combineSignals(
      absolute,
      this.runtime.timeout(submissionBudgets.unitBudgetMs),
    )
    const request = combineSignals(
      unit,
      this.runtime.timeout(submissionBudgets.requestTimeoutMs),
    )
    try {
      return await call(request)
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
    absolute: AbortSignal,
    options: SubmissionRunOptions,
  ): Promise<void> {
    // Plan: provider-free, one request, no provider fan-out.
    this.emitStage(options, "plan")
    const planRaw = await this.send(absolute, (signal) =>
      this.transport.plan(this.planBody(pin), pin.keys, signal),
    )
    const { plan, planReceipt } = readPlan(planRaw, pin, this.runtime.now())
    this.state.plan = plan
    this.state.planReceipt = planReceipt
    this.state.progress.plannedBatches = plan.batches.length
    this.emitProgress(options)

    // Sequential extraction: exactly the planned batches, in index order.
    this.state.status = "extracting"
    this.state.progress.status = "extracting"
    for (const batch of plan.batches) {
      this.assertPin(pin)
      this.emitStage(options, "extract")
      const raw = await this.send(absolute, (signal) =>
        this.transport.extract(this.extractBody(pin, planReceipt, batch.index), pin.keys, signal),
      )
      const extraction = readExtraction(raw, batch.index, this.runtime.now())
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
      this.state.progress.extractedBatches += 1
      this.state.progress.claims += namespaced.length
      this.state.progress.skippedClaims += extraction.skippedClaims.length
      this.emitProgress(options)
    }

    // Provider-free assembly over the complete batch receipt set.
    this.assertPin(pin)
    this.state.status = "assembling"
    this.state.progress.status = "assembling"
    this.emitStage(options, "assemble")
    const assemblyRaw = await this.send(absolute, (signal) =>
      this.transport.assemble(
        this.assembleBody(pin, planReceipt, [...this.state.batchReceipts]),
        pin.keys,
        signal,
      ),
    )
    const { context, contextReceipt } = readAssembly(assemblyRaw, plan, pin, this.runtime.now())
    this.assertAssembly(context)
    this.state.context = context
    this.state.contextReceipt = contextReceipt
    this.state.coverage = context.coverage

    // Sequential reconciliation focus batches, bounded to 30 claims each.
    const planned = focusBatches(context.batches.flatMap((batch) => batch.claimIds))
    this.state.progress.plannedFocusBatches = planned.length
    this.state.status = "reconciling"
    this.state.progress.status = "reconciling"
    for (const focus of planned) {
      this.assertPin(pin)
      this.emitStage(options, "reconcile")
      const raw = await this.send(absolute, (signal) =>
        this.transport.reconcile(this.reconcileBody(pin, contextReceipt, focus), pin.keys, signal),
      )
      this.state.focus.push(readFocus(raw, focus, context))
      this.state.progress.reconciledFocusBatches += 1
      this.emitProgress(options)
    }

    const result = this.buildResult(pin, plan, context)
    this.assertResult(result)
    this.result = result
    this.state.status = "reviewing"
    this.state.progress.status = "reviewing"
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

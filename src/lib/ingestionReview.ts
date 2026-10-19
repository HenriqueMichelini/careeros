// Destination-based review/selection model (#183).
//
// One validated whole-submission result plus one pinned Profile snapshot become
// deterministic destination groups and dependency-safe selection units, with no
// React, provider call, storage, semantic matching, alias expansion, conflict
// resolution or canonical write. The #187 coordinator feeds `ReviewInput` and
// renders `ReviewPlan`; the #189 save path consumes `selectedReviewOperations`.
//
// Batch/source identity is carried per unit (`batchIndexes`, `sourceId`) because
// the assembled submission legitimately exceeds one bounded batch (30 claims /
// 60 operations) and must not be flattened into one anonymous operation list.
//
// This layer never matches on values: wording, claim qualifiers (`meaning`) and
// period labels are presentation data. Selection validity comes only from the
// identity/support edges the submission itself declares.

import {
  beforeValue,
  previewIngestion,
  previewValues,
  type IngestionClaim,
  type IngestionOperation,
  type IngestionOutcome,
  type IngestionTarget,
} from "./ingestion"
import {
  globalClaimId,
  submissionProtocolVersion,
  submissionSelectionError,
  type SubmissionSelection,
  type SubmissionSelectionStatus,
} from "./ingestionProtocol"
import { profileView, type ProfileDocument } from "./profileDocument"
import type { ProfessionalRepository } from "./types"

export class ReviewError extends Error {
  constructor(public readonly code: string) {
    super(code)
  }
}

// The seam the #187 coordinator feeds after assembly. `batches` is the
// assembled aggregate membership (SubmissionAggregate.batches), not the plan
// manifest: it carries the global claim ids placed in each bounded batch.
export interface ReviewBatchMembership {
  index: number
  claimIds: string[]
}
export interface ReviewSubmission {
  protocolVersion: string
  submissionId: string
  runId: string
  sourceId: string
  profileId: string
  documentId?: string
  planVersion: string
  batches: ReviewBatchMembership[]
  claims: IngestionClaim[]
  operations: IngestionOperation[]
  outcomes: IngestionOutcome[]
  unresolvedClaimIds: string[]
}
export interface ReviewInput {
  submission: ReviewSubmission
  snapshot: ProfileDocument
}

export type ReviewEntryKind = "profile" | "existing" | "new"
export type ReviewUnitKind = "addition" | "replacement" | "removal" | "evidence" | "duplicate" | "conflict" | "unsupported" | "unresolved" | "clarification"
export type ReviewSelectionState = "none" | "partial" | "all"

export interface ReviewIdentity {
  field: string
  value: string
}
export interface ReviewUnitValue {
  field: string
  action: IngestionOperation["action"]
  before: string
  proposed: string
}
export interface ReviewUnit {
  id: string
  kind: ReviewUnitKind
  destinationId: string
  target: IngestionTarget | ""
  entryId: string
  entryKind: ReviewEntryKind
  identity: ReviewIdentity[]
  field: string
  values: ReviewUnitValue[]
  // Material units change or remove saved content (replacement, removal, a new
  // entry, or an append onto a non-empty field). Presentation hint only.
  material: boolean
  // The coherent new-entry identity unit carrying the anchor operation.
  required: boolean
  selectable: boolean
  blockedReason: string
  status: SubmissionSelectionStatus
  // Unit ids that must also be selected for this unit to be savable. Same
  // destination edges are cascaded; cross-destination edges stay visible.
  dependencies: string[]
  dependents: string[]
  operationIndexes: number[]
  claimIds: string[]
  batchIndexes: number[]
  sourceId: string
  finding: IngestionOperation["finding"] | ""
  outcomeKind: IngestionOutcome["kind"] | ""
  question: string
  meaning?: IngestionClaim["meaning"]
}
export interface ReviewDestination {
  id: string
  target: IngestionTarget | ""
  entryId: string
  entryKind: ReviewEntryKind
  identity: ReviewIdentity[]
  unitIds: string[]
  selectableUnitIds: string[]
  batchIndexes: number[]
}
export interface ReviewPlan {
  protocolVersion: string
  submissionId: string
  runId: string
  sourceId: string
  profileId: string
  planVersion: string
  snapshot: ProfileDocument
  operations: IngestionOperation[]
  destinations: ReviewDestination[]
  units: ReviewUnit[]
}

export interface ReviewPreviewField {
  target: IngestionTarget
  entryId: string
  field: string
  action: IngestionOperation["action"]
  change: "create" | "append" | "replace" | "remove" | "evidence"
  before: string
  after: string
  unitIds: string[]
}
export interface ReviewPreview {
  fields: ReviewPreviewField[]
  selectedUnitCount: number
  selectedOperationCount: number
  error: string
  complete: boolean
  view: ProfessionalRepository | null
}

// Required new-entry identity fields, mirroring ingestion.ts `requiredFields`.
// They form the coherent unit a new entry cannot be saved without.
const requiredEntryFields: Record<string, string[]> = {
  experience: ["company", "title"],
  projects: ["name"],
  education: ["degree", "institution"],
  certifications: ["name"],
  languages: ["name"],
}
// Identity fields distinguish same-name entries (e.g. two stints at one
// employer). Presentation only: this layer never matches on them.
const identityEntryFields: Record<string, string[]> = {
  experience: ["company", "title", "startDate", "endDate"],
  projects: ["name"],
  education: ["degree", "institution"],
  certifications: ["name", "issuer"],
  languages: ["name"],
}
const controlKinds = new Set<ReviewUnitKind>([
  "duplicate",
  "conflict",
  "unsupported",
  "unresolved",
  "clarification",
])

function namespacedBy(
  runId: string,
  batchIndex: number,
  claimId: string,
): boolean {
  const prefix = globalClaimId(runId, batchIndex, "")
  return claimId.startsWith(prefix) && claimId.length > prefix.length
}
function kindForAction(action: IngestionOperation["action"]): ReviewUnitKind {
  switch (action) {
    case "update":
      return "replacement"
    case "remove":
      return "removal"
    case "evidence":
      return "evidence"
    default:
      return "addition"
  }
}
function noOpKind(
  kind: IngestionOutcome["kind"] | "",
  unresolved: boolean,
): ReviewUnitKind {
  if (unresolved) return "unresolved"
  switch (kind) {
    case "exact_duplicate":
      return "duplicate"
    case "contradiction":
    case "correction":
      return "conflict"
    case "unsupported":
      return "unsupported"
    case "clarification":
      return "clarification"
    default:
      return "unresolved"
  }
}
function distinct<T>(values: T[]): T[] {
  return [...new Set(values)]
}
function identityFor(
  target: IngestionTarget | "",
  entryId: string,
  entryKind: ReviewEntryKind,
  operationIndexes: number[],
  operations: IngestionOperation[],
  view: ProfessionalRepository,
): ReviewIdentity[] {
  if (entryKind === "profile" || !target) return []
  const names = identityEntryFields[target] ?? []
  if (entryKind === "existing") {
    const items =
      view[
        (target as keyof ProfessionalRepository)
      ] as unknown as Record<string, unknown>[] | undefined ?? []
    const item = items.find((entry) => entry.id === entryId)
    return names
      .filter((field) => String(item?.[field] ?? "").trim())
      .map((field) => ({ field, value: String(item?.[field]) }))
  }
  return names
    .map((field) => {
      const index = [...operationIndexes]
        .reverse()
        .find(
          (candidate) =>
            operations[candidate].field === field &&
            String(operations[candidate].value).trim(),
        )
      return index === undefined
        ? null
        : { field, value: String(operations[index].value) }
    })
    .filter((identity): identity is ReviewIdentity => !!identity)
}

interface DestinationDraft {
  id: string
  target: IngestionTarget | ""
  entryId: string
  entryKind: ReviewEntryKind
  stale: boolean
  operationIndexes: number[]
}
interface UnitDraft {
  kind: ReviewUnitKind
  destination: DestinationDraft
  operationIndexes: number[]
  claimIds: string[]
  required: boolean
  blockedReason: string
}

// buildReviewPlan is the only constructor: it starts with no selection and
// returns an immutable-shaped plan whose unit ids and dependency edges are
// stable across identical inputs.
export function buildReviewPlan(input: ReviewInput): ReviewPlan {
  const submission = input.submission
  if (submission.protocolVersion !== submissionProtocolVersion)
    throw new ReviewError("version")
  if (
    !submission.submissionId ||
    !submission.runId ||
    !submission.sourceId ||
    !submission.profileId ||
    !submission.planVersion
  )
    throw new ReviewError("input")
  const snapshot = input.snapshot
  const view = profileView(snapshot)
  const operations = submission.operations

  const batchByClaim = new Map<string, number>()
  for (const batch of submission.batches) {
    if (!Number.isInteger(batch.index) || batch.index < 0)
      throw new ReviewError("manifest_incomplete")
    for (const id of batch.claimIds) {
      if (batchByClaim.has(id)) throw new ReviewError("manifest_incomplete")
      batchByClaim.set(id, batch.index)
    }
  }
  const claimById = new Map<string, IngestionClaim>()
  for (const claim of submission.claims) {
    if (!claim.id || claimById.has(claim.id))
      throw new ReviewError("manifest_incomplete")
    claimById.set(claim.id, claim)
  }
  // A global claim id namespaces the batch it was assembled into. Consuming the
  // membership explicitly keeps that identity instead of flattening it away.
  for (const [id, index] of batchByClaim) {
    if (!claimById.has(id) || !namespacedBy(submission.runId, index, id))
      throw new ReviewError("manifest_incomplete")
  }
  // Every claim is placed so no pending entry loses its batch/source identity.
  for (const claim of submission.claims)
    if (!batchByClaim.has(claim.id))
      throw new ReviewError("manifest_incomplete")

  const unresolved = new Set(submission.unresolvedClaimIds)
  for (const id of unresolved)
    if (!claimById.has(id)) throw new ReviewError("input")
  const outcomeByClaim = new Map<string, IngestionOutcome>()
  for (const outcome of submission.outcomes) {
    if (!claimById.has(outcome.claimId) || outcomeByClaim.has(outcome.claimId))
      throw new ReviewError("input")
    outcomeByClaim.set(outcome.claimId, outcome)
  }

  const opClaimIndexes = new Map<string, number[]>()
  operations.forEach((op, index) => {
    if (!claimById.has(op.claimId)) throw new ReviewError("input")
    opClaimIndexes.set(op.claimId, [
      ...(opClaimIndexes.get(op.claimId) ?? []),
      index,
    ])
  })

  const draftById = new Map<string, DestinationDraft>()
  const drafts: DestinationDraft[] = []
  const destinationFor = (
    target: IngestionTarget | "",
    entryId: string,
  ): DestinationDraft => {
    const id = `${target || "@claim"}#${entryId || "@profile"}`
    const existing = draftById.get(id)
    if (existing) return existing
    const entryKind: ReviewEntryKind = !entryId
      ? "profile"
      : entryId.startsWith("new:")
        ? "new"
        : "existing"
    const items = target
      ? (view[
          (target as keyof ProfessionalRepository)
        ] as unknown as Record<string, unknown>[] | undefined ?? [])
      : []
    const stale =
      entryKind === "existing" && !items.some((e) => e.id === entryId)
    const draft: DestinationDraft = {
      id,
      target,
      entryId,
      entryKind,
      stale,
      operationIndexes: [],
    }
    draftById.set(id, draft)
    drafts.push(draft)
    return draft
  }

  // Destinations appear in first operation order; claim-only destinations
  // (clarification, duplicate, unresolved, conflict) follow.
  operations.forEach((op, index) => {
    destinationFor(op.target, op.entryId).operationIndexes.push(index)
  })

  const unitDrafts: UnitDraft[] = []
  for (const draft of drafts) {
    if (draft.entryKind === "new" && draft.operationIndexes.length) {
      const anchor = draft.entryId.slice(4)
      const required = requiredEntryFields[draft.target] ?? []
      const identityIndexes: number[] = []
      const rest: number[] = []
      for (const index of draft.operationIndexes) {
        const op = operations[index]
        if (required.includes(op.field) || op.claimId === anchor)
          identityIndexes.push(index)
        else rest.push(index)
      }
      const complete = required.every((field) =>
        identityIndexes.some((index) => operations[index].field === field),
      )
      const anchored = identityIndexes.some(
        (index) => operations[index].claimId === anchor,
      )
      if (identityIndexes.length) {
        unitDrafts.push({
          kind: "addition",
          destination: draft,
          operationIndexes: identityIndexes,
          claimIds: distinct(identityIndexes.map((i) => operations[i].claimId)),
          required: true,
          blockedReason: !complete
            ? "incomplete_identity"
            : anchored
              ? ""
              : "incomplete_anchor",
        })
      }
      for (const index of rest) {
        unitDrafts.push({
          kind: "addition",
          destination: draft,
          operationIndexes: [index],
          claimIds: [operations[index].claimId],
          required: false,
          blockedReason: identityIndexes.length ? "" : "incomplete_identity",
        })
      }
      continue
    }
    if (draft.operationIndexes.length) {
      for (const index of draft.operationIndexes) {
        unitDrafts.push({
          kind: kindForAction(operations[index].action),
          destination: draft,
          operationIndexes: [index],
          claimIds: [operations[index].claimId],
          required: false,
          blockedReason: draft.stale ? "stale" : "",
        })
      }
      continue
    }
  }
  // Claim-only dispositions (clarification, duplicate, unsupported, unresolved,
  // conflict) keep their own unit inside the section they named.
  for (const claim of submission.claims) {
    if (opClaimIndexes.has(claim.id)) continue
    const kind = noOpKind(
      outcomeByClaim.get(claim.id)?.kind ?? "",
      unresolved.has(claim.id),
    )
    unitDrafts.push({
      kind,
      destination: destinationFor(claim.targets[0] ?? "", ""),
      operationIndexes: [],
      claimIds: [claim.id],
      required: false,
      blockedReason: kind,
    })
  }

  const units: ReviewUnit[] = unitDrafts.map((draft) => {
    const values = draft.operationIndexes.map((index) => {
      const op = operations[index]
      return {
        field: op.field,
        action: op.action,
        before: beforeValue(view, op),
        proposed: op.value,
      }
    })
    const fields = distinct(values.map((value) => value.field))
    const target = draft.destination.target
    const batchIndexes = distinct(
      draft.claimIds.map((id) => batchByClaim.get(id)!).sort((a, b) => a - b),
    )
    const identity = identityFor(
      target,
      draft.destination.entryId,
      draft.destination.entryKind,
      draft.operationIndexes,
      operations,
      view,
    )
    const material =
      draft.kind === "replacement" ||
      draft.kind === "removal" ||
      (draft.kind === "addition" &&
        (draft.destination.entryKind === "new" ||
          values.some((value) => value.before.trim().length > 0)))
    const first = draft.claimIds[0]
    const meanings = distinct(
      draft.claimIds.map((id) => JSON.stringify(claimById.get(id)?.meaning)),
    )
    const findings = distinct(
      draft.operationIndexes.map((index) => operations[index].finding),
    )
    const outcomeKinds = distinct(
      draft.claimIds.map((id) => outcomeByClaim.get(id)?.kind ?? ""),
    )
    return {
      id: `unit:${JSON.stringify([
        submission.runId,
        target || "@claim",
        draft.destination.entryId || "@profile",
        draft.operationIndexes,
        draft.claimIds,
      ])}`,
      kind: draft.kind,
      destinationId: draft.destination.id,
      target,
      entryId: draft.destination.entryId,
      entryKind: draft.destination.entryKind,
      identity,
      field: fields.length === 1 ? fields[0] : "",
      values,
      material,
      required: draft.required,
      selectable: false,
      blockedReason: draft.blockedReason,
      status: "pending",
      dependencies: [],
      dependents: [],
      operationIndexes: draft.operationIndexes,
      claimIds: draft.claimIds,
      batchIndexes,
      sourceId: submission.sourceId,
      finding: findings.length === 1 ? findings[0] : "",
      outcomeKind: outcomeKinds.length === 1 ? outcomeKinds[0] : "",
      question: claimById.get(first)?.question ?? "",
      meaning: claimById.get(first)?.meaning,
    }
  })

  const unitById = new Map(units.map((unit) => [unit.id, unit]))
  const unitIdsByClaim = new Map<string, string[]>()
  for (const unit of units)
    for (const id of unit.claimIds)
      unitIdsByClaim.set(id, [...(unitIdsByClaim.get(id) ?? []), unit.id])
  const addDependency = (from: ReviewUnit, to: string) => {
    if (from.id !== to && !from.dependencies.includes(to))
      from.dependencies.push(to)
  }

  for (const unit of units) {
    // A new-entry detail cannot be created without the coherent identity unit
    // that carries the anchor operation.
    if (
      unit.entryKind === "new" &&
      unit.kind === "addition" &&
      !unit.required
    ) {
      const identity = units.find(
        (candidate) =>
          candidate.destinationId === unit.destinationId && candidate.required,
      )
      if (identity) addDependency(unit, identity.id)
      else unit.blockedReason = unit.blockedReason || "incomplete_identity"
    }
    for (const index of unit.operationIndexes) {
      const op = operations[index]
      // A claim may not serve two entries of one target: that would borrow
      // another employer's or period's support. The op's own claim is required
      // support in every case, so check it even without explicit ids.
      if (
        operations.some(
          (other, otherIndex) =>
            otherIndex !== index &&
            other.claimId === op.claimId &&
            other.target === op.target &&
            other.entryId !== op.entryId,
        )
      )
        unit.blockedReason = unit.blockedReason || "shared_support"
      for (const supporting of op.supportingClaimIds ?? []) {
        // The op already satisfies the requirement on its own claim.
        if (supporting === op.claimId) continue
        const owners = (unitIdsByClaim.get(supporting) ?? [])
          .map((id) => unitById.get(id)!)
          .filter((owner) => owner.operationIndexes.length > 0)
        if (!owners.length) {
          unit.blockedReason = unit.blockedReason || "dangling_support"
          continue
        }
        for (const owner of owners) {
          if (owner.target === unit.target && owner.entryId !== unit.entryId) {
            unit.blockedReason = unit.blockedReason || "shared_support"
            continue
          }
          addDependency(unit, owner.id)
        }
      }
    }
  }

  // Dependency cycles are surfaced, never resolved silently.
  const colour = new Map<string, 0 | 1 | 2>()
  const cyclic = new Set<string>()
  const visit = (id: string, stack: string[]) => {
    colour.set(id, 1)
    stack.push(id)
    for (const dependency of unitById.get(id)!.dependencies) {
      const state = colour.get(dependency) ?? 0
      if (state === 1)
        for (const node of stack.slice(stack.indexOf(dependency)))
          cyclic.add(node)
      else if (state === 0) visit(dependency, stack)
    }
    stack.pop()
    colour.set(id, 2)
  }
  for (const unit of units)
    if ((colour.get(unit.id) ?? 0) === 0) visit(unit.id, [])
  for (const id of cyclic) unitById.get(id)!.blockedReason = "dependency_cycle"

  // A blocked dependency blocks its dependents: a partial identity or support
  // selection cannot be produced from a unit that is already unsavable.
  let changed = true
  while (changed) {
    changed = false
    for (const unit of units) {
      if (unit.blockedReason) continue
      if (
        unit.dependencies.some((id) => unitById.get(id)!.blockedReason !== "")
      ) {
        unit.blockedReason = "dependency_blocked"
        changed = true
      }
    }
  }
  for (const unit of units) {
    unit.selectable = !controlKinds.has(unit.kind) && unit.blockedReason === ""
    for (const dependency of unit.dependencies)
      unitById.get(dependency)!.dependents.push(unit.id)
  }

  const destinations: ReviewDestination[] = drafts.map((draft) => {
    const owned = units.filter((unit) => unit.destinationId === draft.id)
    return {
      id: draft.id,
      target: draft.target,
      entryId: draft.entryId,
      entryKind: draft.entryKind,
      identity: identityFor(
        draft.target,
        draft.entryId,
        draft.entryKind,
        draft.operationIndexes,
        operations,
        view,
      ),
      unitIds: owned.map((unit) => unit.id),
      selectableUnitIds: owned
        .filter((unit) => unit.selectable)
        .map((unit) => unit.id),
      batchIndexes: distinct(
        owned.flatMap((unit) => unit.batchIndexes).sort((a, b) => a - b),
      ),
    }
  })

  return {
    protocolVersion: submission.protocolVersion,
    submissionId: submission.submissionId,
    runId: submission.runId,
    sourceId: submission.sourceId,
    profileId: submission.profileId,
    planVersion: submission.planVersion,
    snapshot,
    operations,
    destinations,
    units,
  }
}

export function reviewUnit(
  plan: ReviewPlan,
  unitId: string,
): ReviewUnit | undefined {
  return plan.units.find((unit) => unit.id === unitId)
}
export function reviewDestination(
  plan: ReviewPlan,
  destinationId: string,
): ReviewDestination | undefined {
  return plan.destinations.find(
    (destination) => destination.id === destinationId,
  )
}

function withStatuses(
  plan: ReviewPlan,
  selected: Set<string>,
  cleared: Set<string>,
): ReviewPlan {
  return {
    ...plan,
    units: plan.units.map((unit) =>
      selected.has(unit.id)
        ? { ...unit, status: "selected" }
        : cleared.has(unit.id)
          ? { ...unit, status: "pending" }
          : unit,
    ),
  }
}

// Same-destination dependencies are cascaded: they are the coherent identity
// and support structure of one destination, never a separate decision.
// Cross-destination edges stay visible and must be selected explicitly.
function selectionClosure(plan: ReviewPlan, roots: string[]): Set<string> {
  const selected = new Set<string>()
  const queue = [...roots]
  while (queue.length) {
    const id = queue.shift()!
    if (selected.has(id)) continue
    const unit = reviewUnit(plan, id)
    if (!unit || !unit.selectable) continue
    selected.add(id)
    for (const dependency of unit.dependencies) {
      const target = reviewUnit(plan, dependency)!
      if (target.destinationId === unit.destinationId) queue.push(dependency)
    }
  }
  return selected
}
function deselectionClosure(plan: ReviewPlan, roots: string[]): Set<string> {
  const cleared = new Set<string>()
  const queue = [...roots]
  while (queue.length) {
    const id = queue.shift()!
    if (cleared.has(id)) continue
    cleared.add(id)
    const unit = reviewUnit(plan, id)
    if (unit) queue.push(...unit.dependents)
  }
  return cleared
}

export function selectReviewUnit(
  plan: ReviewPlan,
  unitId: string,
  status: SubmissionSelectionStatus,
): ReviewPlan {
  const unit = reviewUnit(plan, unitId)
  if (!unit) throw new ReviewError("input")
  if (status === "selected" && !unit.selectable)
    throw new ReviewError("unselectable")
  if (status === "selected")
    return withStatuses(plan, selectionClosure(plan, [unitId]), new Set())
  return withStatuses(plan, new Set(), deselectionClosure(plan, [unitId]))
}

// Tri-state group toggle over one destination's selectable units.
export function selectReviewGroup(
  plan: ReviewPlan,
  destinationId: string,
  status: "selected" | "pending",
): ReviewPlan {
  const destination = reviewDestination(plan, destinationId)
  if (!destination) throw new ReviewError("input")
  if (status === "selected")
    return withStatuses(
      plan,
      selectionClosure(plan, destination.selectableUnitIds),
      new Set(),
    )
  return withStatuses(
    plan,
    new Set(),
    deselectionClosure(plan, destination.unitIds),
  )
}

export function reviewGroupState(
  plan: ReviewPlan,
  destinationId: string,
): ReviewSelectionState {
  const destination = reviewDestination(plan, destinationId)
  if (!destination || !destination.selectableUnitIds.length) return "none"
  const selected = destination.selectableUnitIds.filter(
    (id) => reviewUnit(plan, id)!.status === "selected",
  ).length
  if (selected === 0) return "none"
  return selected === destination.selectableUnitIds.length ? "all" : "partial"
}

// Scoped bulk selection of displayed additions only: replacements, removals,
// evidence-only changes and control units (conflicts, duplicates, unsupported,
// unresolved, clarification) are never bulk-selected.
export function selectReviewAdditions(
  plan: ReviewPlan,
  destinationId?: string,
): ReviewPlan {
  const roots = plan.units
    .filter(
      (unit) =>
        unit.selectable &&
        unit.kind === "addition" &&
        (!destinationId || unit.destinationId === destinationId),
    )
    .map((unit) => unit.id)
  if (destinationId && !reviewDestination(plan, destinationId))
    throw new ReviewError("input")
  return withStatuses(plan, selectionClosure(plan, roots), new Set())
}

export function clearReviewSelection(plan: ReviewPlan): ReviewPlan {
  return {
    ...plan,
    units: plan.units.map((unit) =>
      unit.status === "pending" ? unit : { ...unit, status: "pending" },
    ),
  }
}

export function reviewPlanError(plan: ReviewPlan): string {
  if (plan.protocolVersion !== submissionProtocolVersion) return "version"
  const unitIds = new Set<string>()
  for (const unit of plan.units) {
    if (!unit.id || unitIds.has(unit.id)) return "manifest_incomplete"
    unitIds.add(unit.id)
    if (submissionSelectionError(unit.status)) return "state"
  }
  const destinationIds = new Set<string>()
  for (const destination of plan.destinations) {
    if (!destination.id || destinationIds.has(destination.id))
      return "manifest_incomplete"
    destinationIds.add(destination.id)
    for (const id of destination.unitIds)
      if (!unitIds.has(id)) return "manifest_incomplete"
  }
  for (const unit of plan.units) {
    if (!destinationIds.has(unit.destinationId)) return "manifest_incomplete"
    for (const dependency of unit.dependencies) {
      const other = reviewUnit(plan, dependency)
      if (!other || !other.dependents.includes(unit.id))
        return "manifest_incomplete"
    }
    for (const dependent of unit.dependents) {
      const other = reviewUnit(plan, dependent)
      if (!other || !other.dependencies.includes(unit.id))
        return "manifest_incomplete"
    }
  }
  return ""
}

export interface ReviewDependencyGap {
  unitId: string
  unitIds: string[]
}

export function reviewMissingDependencies(
  plan: ReviewPlan,
): ReviewDependencyGap[] {
  const missing: ReviewDependencyGap[] = []
  for (const unit of plan.units) {
    if (unit.status !== "selected") continue
    const unitIds = unit.dependencies.filter(
      (id) => reviewUnit(plan, id)!.status !== "selected",
    )
    if (unitIds.length) missing.push({ unitId: unit.id, unitIds })
  }
  return missing
}

// Pre-Save validity of the actual selection. Coherent units and cascading make
// most states impossible; this guard blocks any residual partial identity,
// unsupported or cross-destination selection.
export function reviewSelectionError(plan: ReviewPlan): string {
  const structural = reviewPlanError(plan)
  if (structural) return structural
  for (const unit of plan.units) {
    if (unit.status !== "selected") continue
    if (!unit.selectable) return unit.blockedReason || "blocked_unit_selected"
  }
  if (reviewMissingDependencies(plan).length) return "selection_closure"
  for (const destination of plan.destinations) {
    if (destination.entryKind !== "new") continue
    const units = destination.unitIds.map((id) => reviewUnit(plan, id)!)
    if (!units.some((unit) => unit.status === "selected")) continue
    if (!units.some((unit) => unit.required && unit.status === "selected"))
      return "incomplete_identity"
  }
  return ""
}

// Replacement/removal/append previews computed from the actual selection and
// the pinned snapshot, reusing the canonical ingestion preview semantics so
// distinct details (repeated appends) are preserved exactly.
export function reviewPreview(plan: ReviewPlan): ReviewPreview {
  const view = profileView(plan.snapshot)
  const selectedIndexes = new Set<number>()
  for (const unit of plan.units)
    if (unit.status === "selected")
      for (const index of unit.operationIndexes) selectedIndexes.add(index)
  const operations = plan.operations.map((op, index) => ({
    ...op,
    approved: selectedIndexes.has(index),
  }))
  const keys = new Map<string, number[]>()
  selectedIndexes.forEach((index) => {
    const op = plan.operations[index]
    const key = `${op.target}\u0000${op.entryId}\u0000${op.field}`
    keys.set(key, [...(keys.get(key) ?? []), index])
  })
  const unitIdsFor = (indexes: number[]) =>
    plan.units
      .filter((unit) =>
        unit.operationIndexes.some((index) => indexes.includes(index)),
      )
      .map((unit) => unit.id)
  const fields: ReviewPreviewField[] = []
  for (const indexes of keys.values()) {
    indexes.sort((a, b) => a - b)
    const first = indexes[0]
    const last = indexes.at(-1)!
    const op = plan.operations[last]
    const before = previewValues(view, operations, first).before
    const after = previewValues(view, operations, last).after
    const evidence = indexes.every(
      (index) => plan.operations[index].action === "evidence",
    )
    const change: ReviewPreviewField["change"] = evidence
      ? "evidence"
      : op.action === "remove"
        ? "remove"
        : op.action === "update"
          ? "replace"
          : before.trim()
            ? "append"
            : "create"
    fields.push({
      target: op.target,
      entryId: op.entryId,
      field: op.field,
      action: op.action,
      change,
      before,
      after,
      unitIds: unitIdsFor(indexes),
    })
  }
  let error = reviewSelectionError(plan)
  let resulting: ProfessionalRepository | null = null
  if (!error) {
    try {
      resulting = previewIngestion(view, JSON.stringify(view), operations)
    } catch (previewError) {
      error = "incomplete"
      if (
        previewError &&
        typeof previewError === "object" &&
        "code" in previewError
      )
        error = String((previewError as { code: unknown }).code)
    }
  }
  return {
    fields,
    selectedUnitCount: plan.units.filter((unit) => unit.status === "selected")
      .length,
    selectedOperationCount: selectedIndexes.size,
    error,
    complete: error === "",
    view: error === "" ? resulting : null,
  }
}

// The #189 save path consumes exactly the selected operations; unrelated
// pending units are never approved implicitly.
export function selectedReviewOperations(
  plan: ReviewPlan,
): IngestionOperation[] {
  const indexes = new Set<number>()
  for (const unit of plan.units)
    if (unit.status === "selected")
      for (const index of unit.operationIndexes) indexes.add(index)
  return plan.operations
    .filter((_, index) => indexes.has(index))
    .map((op) => ({ ...op, approved: true }))
}

// SubmissionSelection-compatible state for the #187 session (selectable units
// only, per the submission protocol).
export function toSubmissionSelections(
  plan: ReviewPlan,
): SubmissionSelection[] {
  return plan.units
    .filter((unit) => unit.selectable)
    .map((unit) => ({
      unitId: unit.id,
      status: unit.status,
      dependencies: [...unit.dependencies],
    }))
}

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
  type IngestionSourceReference,
  type IngestionTarget,
} from "./ingestion"
import {
  globalClaimId,
  submissionProtocolVersion,
  submissionSelectionError,
  type SubmissionSelection,
  type SubmissionSelectionStatus,
} from "./ingestionProtocol"
import {
  profileView,
  type ProfileDocument,
  type ProfileFact,
} from "./profileDocument"
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
// One reconciliation focus response and the claims it accounted for. The
// 60-operation bound belongs to a focus response, never to an extraction batch:
// one focus may span batches and one batch may feed several responses.
export interface ReviewFocusMembership {
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
  // Reconciliation responses retained for per-response budget validation. A
  // caller that only needs the review plan may omit it; the stage reader still
  // enforces the per-response cap when the submission is ingested.
  focus?: ReviewFocusMembership[]
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
// One disclosed excerpt of a unit's submitted support. The origin stays explicit
// so an original-paste occurrence is never presented as a clarification answer.
export interface ReviewSource {
  claimId: string
  text: string
  excerpt: string
  origin: "professional_information" | "clarification_answer"
  reference?: IngestionSourceReference
}
export interface ReviewRelatedClaim {
  id: string
  text: string
  source: string
}
export interface ReviewRelatedFact {
  id: string
  revision: number
  field: string
  ownerKind: string
  value: string
  normalization: ProfileFact["normalization"]
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
  // Presentation data for the compact review: deduplicated excerpts with their
  // origin, the comparison reason and the competing statements/facts a
  // disposition references. Never used for matching or selection validity.
  sources: ReviewSource[]
  reason: string
  relatedClaims: ReviewRelatedClaim[]
  relatedFacts: ReviewRelatedFact[]
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
// Deduplicated disclosure excerpts for one unit. Repeated quotations collapse;
// an excerpt keeps the origin of the claim/support it actually came from.
function unitSources(
  claimIds: string[],
  claimById: Map<string, IngestionClaim>,
): ReviewSource[] {
  const seen = new Set<string>()
  const sources: ReviewSource[] = []
  const add = (source: ReviewSource) => {
    if (!source.excerpt) return
    const key = `${source.origin}\u0000${source.excerpt}`
    if (seen.has(key)) return
    seen.add(key)
    sources.push(source)
  }
  for (const id of claimIds) {
    const claim = claimById.get(id)
    if (!claim) continue
    add({
      claimId: id,
      text: claim.text,
      excerpt: claim.source,
      origin: "professional_information",
      ...(claim.sourceReference ? { reference: claim.sourceReference } : {}),
    })
    for (const support of claim.supportingSources ?? [])
      add({
        claimId: id,
        text: claim.text,
        excerpt: support.source,
        origin: support.origin ?? "professional_information",
        ...(support.sourceReference
          ? { reference: support.sourceReference }
          : {}),
      })
  }
  return sources
}
// The comparison reason and the competing statements/facts a disposition
// references, resolved against the pinned snapshot for presentation.
function unitPresentation(
  claimIds: string[],
  claimById: Map<string, IngestionClaim>,
  outcomeByClaim: Map<string, IngestionOutcome>,
  snapshot: ProfileDocument,
): {
  reason: string
  relatedClaims: ReviewRelatedClaim[]
  relatedFacts: ReviewRelatedFact[]
} {
  const reasons: string[] = []
  const claimSeen = new Set<string>()
  const relatedClaims: ReviewRelatedClaim[] = []
  const factSeen = new Set<string>()
  const relatedFacts: ReviewRelatedFact[] = []
  for (const id of claimIds) {
    const outcome = outcomeByClaim.get(id)
    if (!outcome) continue
    if (outcome.reason) reasons.push(outcome.reason)
    for (const relatedId of outcome.relatedClaimIds) {
      const claim = claimById.get(relatedId)
      if (!claim || claimSeen.has(relatedId)) continue
      claimSeen.add(relatedId)
      relatedClaims.push({
        id: relatedId,
        text: claim.text,
        source: claim.source,
      })
    }
    for (const ref of outcome.relatedFacts) {
      const key = `${ref.id}\u0000${ref.revision}`
      if (factSeen.has(key)) continue
      factSeen.add(key)
      const fact = snapshot.facts.find(
        (candidate) =>
          candidate.id === ref.id && candidate.revision === ref.revision,
      )
      if (!fact) continue
      relatedFacts.push({
        id: fact.id,
        revision: fact.revision,
        field: fact.field,
        ownerKind:
          snapshot.entities.find((entity) => entity.id === fact.owner.id)
            ?.kind ?? "profile",
        value: String(fact.value),
        normalization: fact.normalization,
      })
    }
  }
  return {
    reason: distinct(reasons).join(" · "),
    relatedClaims,
    relatedFacts,
  }
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
      sources: unitSources(draft.claimIds, claimById),
      ...unitPresentation(draft.claimIds, claimById, outcomeByClaim, snapshot),
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

function operationTransferKey(op: IngestionOperation): string {
  return [
    op.claimId,
    op.target,
    op.entryId,
    op.field,
    op.action,
    op.proposedValue ?? op.value,
    op.finding,
  ].join("\u0000")
}

function unitTransferKey(
  unit: ReviewUnit,
  operations: IngestionOperation[],
): string {
  return [
    unit.kind,
    unit.destinationId,
    unit.entryId,
    unit.field,
    unit.claimIds.join("\u0001"),
    unit.operationIndexes.map((index) => operationTransferKey(operations[index])).join("\u0001"),
  ].join("\u0000")
}

// transferReviewState merges a newly built review plan after a focused
// clarification with the retained review. It copies reviewed wording for
// unaffected operation identities and selection for unaffected, still-savable
// units; the revised claim and every genuinely dependent unit restart pending.
// The review is deterministic, so this preserves unrelated work without a
// second provider pass or a whole-submission regenerate.
export function transferReviewState(
  previous: ReviewPlan,
  next: ReviewPlan,
  resetClaimIds: string[],
): ReviewPlan {
  const resetClaims = new Set(resetClaimIds)
  const edited = new Map<string, IngestionOperation[]>()
  for (const op of previous.operations) {
    if (resetClaims.has(op.claimId) || op.value === (op.proposedValue ?? op.value)) continue
    const key = operationTransferKey(op)
    edited.set(key, [...(edited.get(key) ?? []), op])
  }
  const operations = next.operations.map((op) => {
    if (resetClaims.has(op.claimId)) return op
    const queue = edited.get(operationTransferKey(op))
    const source = queue?.shift()
    return source
      ? { ...op, value: source.value, proposedValue: op.proposedValue ?? op.value }
      : op
  })
  const view = profileView(next.snapshot)
  const units = next.units.map((unit) => ({
    ...unit,
    values: unit.operationIndexes.map((index) => {
      const op = operations[index]
      return {
        field: op.field,
        action: op.action,
        before: beforeValue(view, op),
        proposed: op.value,
      }
    }),
    identity: identityFor(
      unit.target,
      unit.entryId,
      unit.entryKind,
      unit.operationIndexes,
      operations,
      view,
    ),
  }))
  const destinations = next.destinations.map((destination) => ({
    ...destination,
    identity: identityFor(
      destination.target,
      destination.entryId,
      destination.entryKind,
      units
        .filter((unit) => unit.destinationId === destination.id)
        .flatMap((unit) => unit.operationIndexes),
      operations,
      view,
    ),
  }))
  const reset = new Set(
    units
      .filter((unit) => unit.claimIds.some((id) => resetClaims.has(id)))
      .map((unit) => unit.id),
  )
  let changed = true
  while (changed) {
    changed = false
    for (const unit of units) {
      if (reset.has(unit.id)) continue
      if (unit.dependencies.some((id) => reset.has(id))) {
        reset.add(unit.id)
        changed = true
      }
    }
  }
  const previousByKey = new Map<string, ReviewUnit[]>()
  for (const unit of previous.units) {
    const key = unitTransferKey(unit, previous.operations)
    previousByKey.set(key, [...(previousByKey.get(key) ?? []), unit])
  }
  let carried = units.map((unit) => {
    if (reset.has(unit.id) || !unit.selectable) return unit
    const source = previousByKey
      .get(unitTransferKey(unit, operations))
      ?.shift()
    if (source && (source.status === "selected" || source.status === "saved"))
      return { ...unit, status: source.status }
    return unit
  })
  // A selected unit whose retained dependency no longer accompanies it restarts
  // pending; the affected destination is reviewed again instead of leaking a
  // partial selection into a save.
  changed = true
  while (changed) {
    changed = false
    const statusById = new Map(carried.map((unit) => [unit.id, unit.status]))
    carried = carried.map((unit) => {
      if (unit.status !== "selected") return unit
      if (
        unit.dependencies.some((id) => {
          const status = statusById.get(id)
          return status !== "selected" && status !== "saved"
        })
      ) {
        changed = true
        return { ...unit, status: "pending" as const }
      }
      return unit
    })
  }
  return { ...next, operations, destinations, units: carried }
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

// editReviewUnit is the focused field editor's only model write: it replaces the
// proposed wording of the unit's field and records the machine proposal in
// `proposedValue` so the canonical save never inherits machine support for
// manually changed wording. Selection state is retained: an edit changes what
// would be saved, not whether the unit was chosen. Evidence-only support is
// read-only wording and removals carry no new wording.
export function editReviewUnit(
  plan: ReviewPlan,
  unitId: string,
  field: string,
  value: string,
): ReviewPlan {
  const unit = reviewUnit(plan, unitId)
  if (!unit) throw new ReviewError("input")
  if (unit.kind === "removal" || unit.kind === "evidence")
    throw new ReviewError("readonly")
  const indexes = unit.operationIndexes.filter(
    (index) =>
      plan.operations[index].field === field &&
      plan.operations[index].action !== "remove" &&
      plan.operations[index].action !== "evidence",
  )
  if (!indexes.length) throw new ReviewError("input")
  if (indexes.every((index) => plan.operations[index].value === value))
    return plan
  const edited = new Set(indexes)
  const operations = plan.operations.map((op, index) =>
    edited.has(index)
      ? { ...op, value, proposedValue: op.proposedValue ?? op.value }
      : op,
  )
  return {
    ...plan,
    operations,
    units: plan.units.map((candidate) =>
      candidate.id !== unitId
        ? candidate
        : {
            ...candidate,
            values: candidate.values.map((item) =>
              item.field === field ? { ...item, proposed: value } : item,
            ),
          },
    ),
  }
}

// Whether the unit's saved wording is user-authored rather than the machine
// proposal. Presentation and provenance only: it never upgrades support.
export function reviewUnitEdited(plan: ReviewPlan, unitId: string): boolean {
  const unit = reviewUnit(plan, unitId)
  if (!unit) return false
  return unit.operationIndexes.some(
    (index) =>
      plan.operations[index].proposedValue !== undefined &&
      plan.operations[index].proposedValue !== plan.operations[index].value,
  )
}

export interface ReviewSummary {
  selectable: number
  selected: number
  // Units a person must look at before they can say the review is resolved:
  // conflicts, focused questions, unsupported and uncompared claims, plus
  // changes blocked by identity/support/staleness and not selectable.
  attention: number
  questions: number
  clarifications: number
  conflicts: number
  unsupported: number
  unresolved: number
  blocked: number
  duplicates: number
  evidence: number
  material: number
  newEntries: number
  // No selectable change and nothing needing attention: everything was already
  // represented (or there was nothing to compare). No approval work remains.
  noChange: boolean
}

export function reviewSummary(plan: ReviewPlan): ReviewSummary {
  const units = plan.units
  const count = (kind: ReviewUnitKind) =>
    units.filter((unit) => unit.kind === kind).length
  const control =
    count("conflict") +
    count("clarification") +
    count("unsupported") +
    count("unresolved")
  const blocked = units.filter(
    (unit) => !unit.selectable && !controlKinds.has(unit.kind),
  ).length
  const selectable = units.filter((unit) => unit.selectable).length
  return {
    selectable,
    selected: units.filter((unit) => unit.status === "selected").length,
    attention: control + blocked,
    questions: units.filter((unit) => !!unit.question.trim()).length,
    clarifications: count("clarification"),
    conflicts: count("conflict"),
    unsupported: count("unsupported"),
    unresolved: count("unresolved"),
    blocked,
    duplicates: count("duplicate"),
    evidence: count("evidence"),
    material: units.filter((unit) => unit.material).length,
    newEntries: units.filter(
      (unit) => unit.entryKind === "new" && unit.required,
    ).length,
    noChange: selectable === 0 && control + blocked === 0,
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

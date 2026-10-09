import contract from "../../internal/profiledocument/contract.json"
import type { ProfessionalRepository } from "./types"

export const PROFILE_VERSION = 2 as const
export const NORMALIZATION_POLICY = "exact-alias-v1" as const
export const profileFields = [
  "fullName",
  "email",
  "phone",
  "location",
  "professionalLinks",
  "careerGoals",
  "skills",
  "competencies",
  "tools",
  "employmentStatus",
  "currentSalary",
  "desiredSalary",
  "additionalInfo",
] as const
export const entityFields = {
  experience: [
    "company",
    "title",
    "startDate",
    "endDate",
    "current",
    "location",
    "description",
    "responsibilities",
    "achievements",
  ],
  projects: ["name", "description", "technologies", "url", "highlights"],
  education: ["degree", "institution", "location", "graduationDate", "details"],
  certifications: ["name", "issuer", "date", "credentialId", "url"],
  languages: ["name", "proficiency"],
} as const
export type EntityKind = keyof typeof entityFields
export type Json = null | boolean | number | string | Json[] | {
  [key: string]: Json
}
export interface ProfileRef {
  profileId: string
  id: string
  revision: number
}
export interface ProfileOrigin {
  kind: "existing_profile" | "manual_edit" | "accepted_proposal" | "ai_review"
  original: "unknown" | "user" | "source"
}
export interface ProfileEntity {
  id: string
  revision: number
  kind: EntityKind
  legacyId: string
  order: number
}
export interface ProfileFact {
  id: string
  revision: number
  owner: ProfileRef
  context: ProfileRef[]
  field: string
  order: number
  kind: "legacy_block" | "statement"
  value: Json
  assertion: "affirmed" | "negated" | "unknown"
  intent: "actual" | "aspiration" | "unknown"
  certainty: "certain" | "uncertain" | "unknown"
  temporal: { wording: string; precision: "exact" | "approximate" | "unknown" }
  normalization: {
    observed: string
    canonical: string | null
    policy: typeof NORMALIZATION_POLICY
  }
  origin: ProfileOrigin
  approval: "unreviewed" | "approved"
  support: "unsupported" | "supported" | "invalidated"
}
export interface ProfileEvidence {
  id: string
  revision: number
  excerpt: string
  origin: string
  approval: "approved"
}
export interface ProfileLink {
  id: string
  kind: "supports" | "role_context" | "project_context" | "period_context"
  from: ProfileRef
  to: ProfileRef
  state: "active" | "invalidated"
}
export interface ProfileDocument {
  version: typeof PROFILE_VERSION
  id: string
  revision: number
  normalizationPolicy: typeof NORMALIZATION_POLICY
  entities: ProfileEntity[]
  facts: ProfileFact[]
  evidence: ProfileEvidence[]
  links: ProfileLink[]
}
export class ProfileValidationError extends Error {}
const record = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === "object" && !Array.isArray(value)
const own = (value: object, key: string) =>
  Object.prototype.hasOwnProperty.call(value, key)
const groups = Object.keys(entityFields) as EntityKind[]
const positive = (value: unknown): value is number =>
  Number.isSafeInteger(value) && value as number > 0
const order = (value: unknown) =>
  Number.isSafeInteger(value) && value as number >= 0
const text = (value: unknown): value is string => typeof value === "string"
const nonempty = (value: unknown): value is string =>
  text(value) && value.length > 0
const oneOf = (value: unknown, choices: readonly string[]) =>
  text(value) && choices.includes(value)
const json = (value: unknown): value is Json =>
  value === null ||
  text(value) ||
  typeof value === "boolean" ||
  (typeof value === "number" && Number.isFinite(value)) ||
  (Array.isArray(value)
    ? value.every(json)
    : record(value) && Object.values(value).every(json))

export function emptyProfileView(): ProfessionalRepository {
  return Object.fromEntries([
    ...profileFields.map((field) => [field, ""]),
    ...groups.map((group) => [group, []]),
  ]) as unknown as ProfessionalRepository
}

// Exact approved spellings only. Ambiguous acronyms and product relationships
// deliberately have no aliases. Normalization never parses a legacy block.
export function normalizeTerm(observed: string) {
  const aliases: Record<string, string> = {
    JavaScript: "JavaScript",
    Javascript: "JavaScript",
    TypeScript: "TypeScript",
    Typescript: "TypeScript",
  }
  return {
    observed,
    canonical: own(aliases, observed) ? aliases[observed] : null,
    policy: NORMALIZATION_POLICY,
  }
}

function validView(value: unknown): value is ProfessionalRepository {
  if (!record(value) || !Object.values(value).every(json)) return false
  if (!profileFields.every((field) => !own(value, field) || text(value[field])))
    return false
  return groups.every(
    (group) =>
      !own(value, group) ||
      (Array.isArray(value[group]) &&
        (value[group] as unknown[]).every(
          (item, index, items) =>
            record(item) &&
            nonempty(item.id) &&
            items.findIndex(
              (other) => record(other) && other.id === item.id,
            ) === index &&
            Object.values(item).every(json) &&
            entityFields[group].every(
              (field) =>
                !own(item, field) ||
                (field === "current"
                  ? typeof item[field] === "boolean"
                  : text(item[field])),
            ),
        )),
  )
}

export function migrateProfile(value: unknown, id: string): ProfileDocument {
  if (!nonempty(id) || !validView(value))
    throw new ProfileValidationError(
      "Invalid legacy Profile; original retained",
    )
  const doc: ProfileDocument = {
    version: 2,
    id,
    revision: 1,
    normalizationPolicy: NORMALIZATION_POLICY,
    entities: [],
    facts: [],
    evidence: [],
    links: [],
  }
  const add = (
    owner: ProfileRef,
    field: string,
    value: Json,
    position: number,
  ) => {
    doc.facts.push({
      id: crypto.randomUUID(),
      revision: 1,
      owner,
      context: [],
      field,
      order: position,
      kind: "legacy_block",
      value,
      assertion: "unknown",
      intent: "unknown",
      certainty: "unknown",
      temporal: {
        wording: /(?:date|duration)$/i.test(field) && text(value) ? value : "",
        precision: "unknown",
      },
      normalization: {
        observed: text(value) ? value : "",
        canonical: null,
        policy: NORMALIZATION_POLICY,
      },
      origin: { kind: "existing_profile", original: "unknown" },
      approval: "unreviewed",
      support: "unsupported",
    })
  }
  Object.entries(value).forEach(([field, value], index) => {
    if (groups.includes(field as EntityKind)) {
      ;(value as unknown as Record<string, Json>[]).forEach(
        (entry, position) => {
          const entity: ProfileEntity = {
            id: crypto.randomUUID(),
            revision: 1,
            kind: field as EntityKind,
            legacyId: entry.id as string,
            order: position,
          }
          doc.entities.push(entity)
          Object.entries(entry)
            .filter(([key]) => key !== "id")
            .forEach(([key, v], i) =>
              add({ profileId: id, id: entity.id, revision: 1 }, key, v, i),
            )
        },
      )
    } else add({ profileId: id, id, revision: 1 }, field, value as Json, index)
  })
  if (!validateProfileDocument(doc))
    throw new ProfileValidationError("Invalid migration")
  return doc
}

export function profileView(doc: ProfileDocument): ProfessionalRepository {
  const view = Object.assign(
    Object.create(null),
    emptyProfileView(),
  ) as Record<string, Json>
  const fields = (id: string) =>
    Object.fromEntries(
      doc.facts
        .filter((f) => f.owner.id === id)
        .sort((a, b) => a.order - b.order)
        .map((f) => [f.field, structuredClone(f.value)]),
    )
  Object.assign(view, fields(doc.id))
  for (const group of groups)
    view[group] = doc.entities
      .filter((e) => e.kind === group)
      .sort((a, b) => a.order - b.order)
      .map((e) => ({
        ...Object.fromEntries(entityFields[group].map(field => [field, field === "current" ? false : ""])),
        ...fields(e.id), id: e.legacyId,
      }))
  return view as unknown as ProfessionalRepository
}

export function validateProfileDocument(
  value: unknown,
): value is ProfileDocument {
  if (
    !validContractShape(value, contract) ||
    !record(value) ||
    value.version !== 2 ||
    !nonempty(value.id) ||
    !positive(value.revision) ||
    value.normalizationPolicy !== NORMALIZATION_POLICY ||
    ![value.entities, value.facts, value.evidence, value.links].every(
      Array.isArray,
    )
  )
    return false
  const d = value as unknown as ProfileDocument
  const nodes = new Map<string, { revision: number; kind: string }>([
    [d.id, { revision: d.revision, kind: "profile" }],
  ])
  const add = (id: unknown, revision: unknown, kind: string) => {
    if (!nonempty(id) || !positive(revision) || nodes.has(id)) return false
    nodes.set(id, { revision, kind })
    return true
  }
  if (
    !d.entities.every(
      (e) =>
        record(e) &&
        oneOf(e.kind, groups) &&
        nonempty(e.legacyId) &&
        order(e.order) &&
        add(e.id, e.revision, e.kind),
    )
  )
    return false
  if (!d.facts.every((f) => record(f) && add(f.id, f.revision, "fact")))
    return false
  if (
    !d.evidence.every(
      (e) =>
        record(e) &&
        nonempty(e.excerpt) &&
        nonempty(e.origin) &&
        e.approval === "approved" &&
        add(e.id, e.revision, "evidence"),
    )
  )
    return false
  const ref = (r: ProfileRef, kinds?: string[]) =>
    record(r) &&
    r.profileId === d.id &&
    nodes.has(r.id) &&
    nodes.get(r.id)!.revision === r.revision &&
    (!kinds || kinds.includes(nodes.get(r.id)!.kind))
  const slots = new Set<string>()
  if (
    !d.facts.every((f) => {
      const slot = JSON.stringify([f.owner?.id, f.field])
      if (slots.has(slot)) return false
      slots.add(slot)
      return (
        !(f.owner.id === d.id && groups.includes(f.field as EntityKind)) &&
        ref(f.owner, ["profile", ...groups]) &&
        Array.isArray(f.context) &&
        f.context.every((r) => ref(r, groups)) &&
        nonempty(f.field) &&
        f.field !== "id" &&
        order(f.order) &&
        oneOf(f.kind, ["legacy_block", "statement"]) &&
        json(f.value) &&
        oneOf(f.assertion, ["affirmed", "negated", "unknown"]) &&
        oneOf(f.intent, ["actual", "aspiration", "unknown"]) &&
        oneOf(f.certainty, ["certain", "uncertain", "unknown"]) &&
        record(f.temporal) &&
        text(f.temporal.wording) &&
        oneOf(f.temporal.precision, ["exact", "approximate", "unknown"]) &&
        record(f.normalization) &&
        text(f.normalization.observed) &&
        (f.normalization.canonical === null ||
          (f.kind === "statement" &&
            normalizeTerm(f.normalization.observed).canonical ===
              f.normalization.canonical)) &&
        f.normalization.policy === NORMALIZATION_POLICY &&
        record(f.origin) &&
        ((f.origin.kind === "existing_profile" &&
          f.origin.original === "unknown") ||
          (f.origin.kind === "manual_edit" && f.origin.original === "user") ||
          (f.origin.kind === "accepted_proposal" &&
            f.origin.original === "unknown") ||
          (f.origin.kind === "ai_review" && f.origin.original === "unknown")) &&
        oneOf(f.approval, ["unreviewed", "approved"]) &&
        oneOf(f.support, ["unsupported", "supported", "invalidated"])
      )
    })
  )
    return false
  const linkIds = new Set<string>()
  if (
    !d.links.every((l) => {
      if (
        !record(l) ||
        !nonempty(l.id) ||
        nodes.has(l.id) ||
        linkIds.has(l.id) ||
        !oneOf(l.state, ["active", "invalidated"]) ||
        !oneOf(l.kind, [
          "supports",
          "role_context",
          "project_context",
          "period_context",
        ])
      )
        return false
      linkIds.add(l.id)
      // Historical invalidated links may retain removed or old revisions, but never another Profile.
      if (l.state === "invalidated")
        return [l.from, l.to].every(
          (r) =>
            record(r) &&
            r.profileId === d.id &&
            nonempty(r.id) &&
            positive(r.revision),
        )
      return (
        ref(l.from, ["fact"]) &&
        ref(
          l.to,
          l.kind === "supports"
            ? ["evidence"]
            : l.kind === "role_context"
              ? ["experience"]
              : l.kind === "project_context"
                ? ["projects"]
                : groups,
        )
      )
    })
  )
    return false
  if (
    !d.facts.every(
      (f) =>
        f.support !== "supported" ||
        (f.approval === "approved" &&
          d.links.some(
            (l) =>
              l.kind === "supports" &&
              l.state === "active" &&
              l.from.id === f.id &&
              l.from.revision === f.revision,
          )),
    )
  )
    return false
  return (
    validView(profileView(d)) &&
    groups.every((group) => {
      const entities = d.entities.filter((e) => e.kind === group)
      return new Set(entities.map((e) => e.order)).size === entities.length
    })
  )
}

// Transitional whole-field edits invalidate support; wording changes never inherit evidence.
export function replaceProfileView(
  doc: ProfileDocument,
  next: ProfessionalRepository,
  origin: ProfileOrigin = { kind: "manual_edit", original: "user" },
): ProfileDocument {
  if (!validateProfileDocument(doc) || !validView(next))
    throw new ProfileValidationError("Invalid Profile edit")
  const candidate = migrateProfile(next, doc.id)
  candidate.revision = doc.revision + 1
  candidate.evidence = structuredClone(doc.evidence)
  candidate.links = structuredClone(doc.links)
  const oldEntities = new Map(
    doc.entities.map((e) => [JSON.stringify([e.kind, e.legacyId]), e]),
  )
  for (const e of candidate.entities) {
    const previous = oldEntities.get(JSON.stringify([e.kind, e.legacyId]))
    const generatedId = e.id
    if (previous) {
      e.id = previous.id
      const before = doc.facts
        .filter((f) => f.owner.id === previous.id)
        .map((f) => [f.field, f.value])
      const after = candidate.facts
        .filter((f) => f.owner.id === generatedId)
        .map((f) => [f.field, f.value])
      e.revision =
        previous.revision +
        (JSON.stringify(before) === JSON.stringify(after) ? 0 : 1)
    }
    for (const f of candidate.facts.filter((f) => f.owner.id === generatedId))
      f.owner = { profileId: doc.id, id: e.id, revision: e.revision }
  }
  for (let i = 0; i < candidate.facts.length; i++) {
    const f = candidate.facts[i]
    if (f.owner.id === doc.id) f.owner.revision = candidate.revision
    const previous = doc.facts.find(
      (old) => old.owner.id === f.owner.id && old.field === f.field,
    )
    if (
      previous &&
      JSON.stringify(previous.value) === JSON.stringify(f.value)
    ) {
      candidate.facts[i] = {
        ...structuredClone(previous),
        owner: f.owner,
        order: f.order,
      }
    } else {
      if (previous) {
        f.id = previous.id
        f.revision = previous.revision + 1
      }
      f.origin = origin
      f.approval = origin.kind === "ai_review" ? "unreviewed" : "approved"
      f.support =
        previous?.support === "supported" || previous?.support === "invalidated"
          ? "invalidated"
          : "unsupported"
    }
  }
  const revisions = new Map([
    [doc.id, candidate.revision],
    ...candidate.entities.map((e) => [e.id, e.revision] as [string, number]),
    ...candidate.facts.map((f) => [f.id, f.revision] as [string, number]),
    ...candidate.evidence.map((e) => [e.id, e.revision] as [string, number]),
  ])
  for (const l of candidate.links)
    if ([l.from, l.to].some((r) => revisions.get(r.id) !== r.revision))
      l.state = "invalidated"
  for (const f of candidate.facts) {
    const previous = doc.facts.find((old) => old.id === f.id)
    const contextChanged =
      (previous &&
        previous.owner.revision !== f.owner.revision &&
        f.owner.id !== doc.id) ||
      f.context.some((r) => revisions.get(r.id) !== r.revision) ||
      candidate.links.some(l => l.from.id === f.id && l.kind !== "supports" && l.state === "invalidated" && doc.links.some(old => old.id === l.id && old.state === "active"))
    if (contextChanged) {
      if (f.support === "supported") f.support = "invalidated"
      for (const l of candidate.links)
        if (l.from.id === f.id) l.state = "invalidated"
    }
    f.context = f.context.filter((r) => revisions.get(r.id) === r.revision)
    if (
      f.support === "supported" &&
      !candidate.links.some(
        (l) =>
          l.kind === "supports" && l.state === "active" && l.from.id === f.id,
      )
    )
      f.support = "invalidated"
  }
  if (!validateProfileDocument(candidate))
    throw new ProfileValidationError("Invalid replacement")
  return candidate
}

// The same deliberately small JSON Schema subset is evaluated in Go. The
// schema owns wire shape; domain validation above owns reference semantics.
function validContractShape(value: unknown, schema: unknown): boolean {
  const rule = schema as {
    type?: string | string[]
    enum?: unknown[]
    minimum?: number
    minLength?: number
    properties?: Record<string, unknown>
    required?: string[]
    additionalProperties?: boolean
    items?: unknown
  }
  const types = Array.isArray(rule.type)
    ? rule.type
    : rule.type
      ? [rule.type]
      : []
  if (
    types.length &&
    !types.some((type) =>
      type === "null"
        ? value === null
        : type === "array"
          ? Array.isArray(value)
          : type === "object"
            ? record(value)
            : type === "integer"
              ? Number.isSafeInteger(value)
              : typeof value === type,
    )
  )
    return false
  if (rule.enum && !rule.enum.includes(value)) return false
  if (rule.minimum !== undefined && value as number < rule.minimum) return false
  if (rule.minLength !== undefined && (value as string).length < rule.minLength)
    return false
  if (record(value) && rule.properties) {
    if (!rule.required!.every((key) => own(value, key))) return false
    if (
      rule.additionalProperties === false &&
      Object.keys(value).some((key) => !own(rule.properties!, key))
    )
      return false
    return Object.entries(rule.properties).every(([key, child]) =>
      validContractShape(value[key], child),
    )
  }
  return (
    !Array.isArray(value) ||
    !rule.items ||
    value.every((item) => validContractShape(item, rule.items))
  )
}

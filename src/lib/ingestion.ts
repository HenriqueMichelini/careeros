import { ProfessionalRepository, ExperienceEntry, ProjectEntry } from "./types"

export type IngestionTarget = "careerGoals" | "skills" | "competencies" | "experience" | "tools" | "projects" | "employmentStatus" | "currentSalary" | "desiredSalary" | "additionalInfo"
export interface IngestionClaim {
  id: string
  source: string
  text: string
  targets: IngestionTarget[]
  question: string
}
export interface IngestionOperation {
  claimId: string
  target: IngestionTarget
  entryId: string
  field: string
  action: "add" | "update" | "remove"
  value: string
  finding: "addition" | "overlap" | "conflict" | "in_place"
  approved: boolean
}
export interface IngestionResult {
  claims: IngestionClaim[]
  operations: IngestionOperation[]
  unverifiedClaimCount: number
  unresolvedClaimIds: string[]
  unplacedOperationCount: number
}

const scalarFields = [
  "careerGoals",
  "skills",
  "competencies",
  "tools",
  "employmentStatus",
  "currentSalary",
  "desiredSalary",
  "additionalInfo",
]
const experienceFields = [
  "company",
  "title",
  "startDate",
  "endDate",
  "current",
  "location",
  "description",
  "responsibilities",
  "achievements",
]
const projectFields = [
  "name",
  "description",
  "technologies",
  "url",
  "highlights",
]
const statuses = [
  "employed-full-time",
  "employed-part-time",
  "employed-contract",
  "freelance",
  "looking",
  "open",
  "unemployed",
  "student",
]
const legacyStatuses = [
  "Employed", "Employed — Full-time", "Employed — Part-time",
  "Employed — Contract", "Freelance / Self-employed",
  "Actively looking for work", "Open to opportunities (not actively searching)",
  "Unemployed", "Student",
]
const profileFields = [...scalarFields, "experience", "projects"]
export class IngestionError extends Error {
  constructor(public readonly code: string) {
    super(code)
  }
}
const record = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === "object" && !Array.isArray(value)
const keys = (value: Record<string, unknown>, expected: readonly string[]) =>
  Object.keys(value).length === expected.length &&
  expected.every((key) => key in value)
const string = (value: unknown, max = 2000): value is string =>
  typeof value === "string" && value.length <= max
const hasTarget = (
  targets: IngestionTarget[],
  value: unknown,
): value is IngestionTarget =>
  typeof value === "string" && targets.includes(value as IngestionTarget)
const entry = (
  profile: ProfessionalRepository,
  target: IngestionTarget,
  id: string,
) =>
  target === "experience"
    ? profile.experience.find((e) => e.id === id)
    : target === "projects"
      ? profile.projects.find((p) => p.id === id)
      : undefined
const normalizedFact = (value: string) =>
  value
    .trim()
    .toLocaleLowerCase()
    .replace(/^(?:•|-)\s+/, "")
    .replace(/[.;,\s]+$/, "")
    .replace(/\s+/g, " ")
const duplicate = (before: string, value: string) =>
  before
    .split(/[\n,;]+/)
    .some(
      (part) =>
        normalizedFact(part) === normalizedFact(value),
    )

export function validProfile(profile: ProfessionalRepository): boolean {
  if (
    !record(profile) ||
    !keys(profile, profileFields) ||
    scalarFields.some(
      (f) => !string(profile[(f as keyof ProfessionalRepository)], 12 << 10),
    )
  )
    return false
  if (profile.employmentStatus && !statuses.includes(profile.employmentStatus) && !legacyStatuses.includes(profile.employmentStatus))
    return false
  if (
    !Array.isArray(profile.experience) ||
    !Array.isArray(profile.projects) ||
    profile.experience.length > 40 ||
    profile.projects.length > 40
  )
    return false
  const ids = new Set<string>()
  for (const [items, fields] of [
    [profile.experience, ["id", ...experienceFields]],
    [profile.projects, ["id", ...projectFields]],
  ] as const) {
    for (const item of items) {
      if (
        !record(item) ||
        !keys(item, fields) ||
        !string(item.id, 100) ||
        !item.id ||
        ids.has(item.id)
      )
        return false
      ids.add(item.id)
      if (fields.some((f) => f !== "current" && !string(item[f], 12 << 10)))
        return false
      if ("current" in item && typeof item.current !== "boolean") return false
    }
  }
  return true
}

export function validateIngestionResult(
  raw: unknown,
  input: string,
  profile: ProfessionalRepository,
): IngestionResult {
  if (
    !record(raw) ||
    !keys(raw, ["claims", "operations", "unverifiedClaimCount", "unresolvedClaimIds", "unplacedOperationCount"]) ||
    !Array.isArray(raw.claims) ||
    !Array.isArray(raw.operations) ||
    typeof raw.unverifiedClaimCount !== "number" ||
    !Number.isInteger(raw.unverifiedClaimCount) ||
    raw.unverifiedClaimCount < 0 ||
    raw.unverifiedClaimCount > 30 ||
    !Array.isArray(raw.unresolvedClaimIds) ||
    typeof raw.unplacedOperationCount !== "number" ||
    !Number.isInteger(raw.unplacedOperationCount) ||
    raw.unplacedOperationCount < 0 ||
    raw.unplacedOperationCount > 60 ||
    raw.claims.length > 30 ||
    raw.claims.length + raw.unverifiedClaimCount > 30 ||
    raw.operations.length > 60
  )
    throw new IngestionError("invalid_output")
  const claims: IngestionClaim[] = []
  const ids = new Set<string>()
  for (const item of raw.claims) {
    if (
      !record(item) ||
      !keys(item, ["id", "source", "text", "targets", "question"]) ||
      !string(item.id, 40) ||
      !item.id ||
      ids.has(item.id) ||
      !string(item.source, 1000) ||
      !item.source ||
      !input.includes(item.source) ||
      !string(item.text, 1000) ||
      !item.text.trim() ||
      !Array.isArray(item.targets) ||
      item.targets.length > 10 ||
      item.targets.some((t: unknown) => !profileFields.includes(t as string)) ||
      !string(item.question, 500)
    )
      throw new IngestionError("invalid_output")
    ids.add(item.id)
    claims.push(item as unknown as IngestionClaim)
  }
  const unresolvedClaimIds = raw.unresolvedClaimIds as unknown[]
  if (unresolvedClaimIds.length > claims.length || new Set(unresolvedClaimIds).size !== unresolvedClaimIds.length ||
      unresolvedClaimIds.some(id => typeof id !== "string" || !ids.has(id)))
    throw new IngestionError("invalid_output")
  const operations: IngestionOperation[] = []
  for (const item of raw.operations) {
    if (
      !record(item) ||
      !keys(item, [
        "claimId",
        "target",
        "entryId",
        "field",
        "action",
        "value",
        "finding",
      ]) ||
      !string(item.claimId, 40) ||
      !string(item.target, 40) ||
      !string(item.entryId, 100) ||
      !string(item.field, 40) ||
      !string(item.value) ||
      !string(item.finding, 100)
    )
      throw new IngestionError("invalid_output")
    const claim = claims.find((c) => c.id === item.claimId)
    if (
      !claim ||
      unresolvedClaimIds.includes(claim.id) ||
      claim.question ||
      !hasTarget(claim.targets, item.target) ||
      !["add", "update", "remove"].includes(item.action as string) ||
      !["addition", "overlap", "conflict", "in_place"].includes(
        item.finding as string,
      )
    )
      throw new IngestionError("invalid_output")
    if (
      scalarFields.includes(item.target)
        ? item.field !== item.target || item.entryId !== ""
        : !(
            item.target === "experience" ? experienceFields : projectFields
          ).includes(item.field as string) ||
          !((item.entryId as string).startsWith("new:")
            ? claims.some(
                anchor =>
                  anchor.id === (item.entryId as string).slice(4) &&
                  anchor.targets.includes(item.target as IngestionTarget) &&
                  !anchor.question &&
                  !unresolvedClaimIds.includes(anchor.id),
              )
            : entry(profile, item.target, item.entryId as string))
    )
      throw new IngestionError("invalid_output")
    if (item.entryId.startsWith("new:") && item.action !== "add")
      throw new IngestionError("invalid_output")
    if (
      item.field === "current" && item.action !== "remove" &&
      !["true", "false"].includes(item.value as string)
    )
      throw new IngestionError("invalid_output")
    if (
      item.target === "employmentStatus" &&
      item.action !== "remove" &&
      !statuses.includes(item.value as string)
    )
      throw new IngestionError("invalid_output")
    if (item.action !== "remove" && !(item.value as string).trim())
      throw new IngestionError("invalid_output")
    if (item.action === "remove" && item.value !== "")
      throw new IngestionError("invalid_output")
    operations.push({ ...item, approved: false } as IngestionOperation)
  }
  const newGroups = new Map<string, IngestionOperation[]>()
  for (const operation of operations) {
    if (!operation.entryId.startsWith("new:")) continue
    const key = operation.target + "/" + operation.entryId
    newGroups.set(key, [...(newGroups.get(key) ?? []), operation])
  }
  for (const group of newGroups.values()) {
    const anchor = group[0].entryId.slice(4)
    const fields = new Set(group.map(operation => operation.field))
    if (
      !group.some(operation => operation.claimId === anchor) ||
      (group[0].target === "experience"
        ? !fields.has("company") || !fields.has("title")
        : !fields.has("name"))
    )
      throw new IngestionError("invalid_output")
  }
  operations.sort((a, b) => claims.findIndex(c => c.id === a.claimId) - claims.findIndex(c => c.id === b.claimId))
  return { claims, operations, unverifiedClaimCount: raw.unverifiedClaimCount as number,
    unresolvedClaimIds: unresolvedClaimIds as string[], unplacedOperationCount: raw.unplacedOperationCount as number }
}

export async function ingestProfile(
  input: string,
  profile: ProfessionalRepository,
  apiKey: string,
  signal?: AbortSignal,
): Promise<IngestionResult> {
  if (
    !input.trim() ||
    new TextEncoder().encode(input).length > 30000 ||
    !validProfile(profile)
  )
    throw new IngestionError("input")
  let response: Response
  try {
    response = await fetch("/api/profile/ingest", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-OpenAI-Api-Key": apiKey,
      },
      body: JSON.stringify({ input, profile: profile }),
      cache: "no-store",
      signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(55_000)]) : AbortSignal.timeout(55_000),
    })
  } catch (error) {
    throw new IngestionError(
      error instanceof DOMException && error.name === "TimeoutError"
        ? "timeout"
        : "outage",
    )
  }
  const raw = await response.json().catch(() => null)
  if (!response.ok)
    throw new IngestionError(
      record(raw) &&
        typeof raw.error === "string" &&
        [
          "input",
          "key",
          "rate_limit",
          "outage",
          "timeout",
          "truncated",
          "invalid_output",
        ].includes(raw.error)
        ? raw.error
        : "outage",
    )
  return validateIngestionResult(raw, input, profile)
}

export function beforeValue(
  profile: ProfessionalRepository,
  op: IngestionOperation,
): string {
  if (scalarFields.includes(op.target))
    return profile[(op.target as keyof ProfessionalRepository)] as string
  const found = entry(profile, op.target, op.entryId)
  return found
    ? String((found as unknown as Record<string, unknown>)[op.field] ?? "")
    : ""
}

export function afterValue(
  profile: ProfessionalRepository,
  op: IngestionOperation,
): string {
  return fieldResult(beforeValue(profile, op), op)
}

function fieldResult(before: string, op: IngestionOperation): string {
  if (op.field === "current") return op.action === "remove" ? "false" : op.value
  if (op.field === "employmentStatus") return op.action === "remove" ? "" : op.value.trim()
  if (op.action === "remove") return ""
  return op.action === "add" && before.trim()
    ? duplicate(before, op.value) ? before : before.trimEnd() + "\n" + op.value.trim()
    : op.value.trim()
}

export function previewValues(profile: ProfessionalRepository, ops: IngestionOperation[], index: number): {before: string; after: string} {
  const selected = ops[index]
  let before = beforeValue(profile, selected)
  for (const prior of ops.slice(0,index)) {
    if (prior.approved && prior.target === selected.target && prior.entryId === selected.entryId && prior.field === selected.field) before = fieldResult(before,prior)
  }
  return {before,after:fieldResult(before,selected)}
}

export function applyIngestion(
  profile: ProfessionalRepository,
  snapshot: string,
  ops: IngestionOperation[],
): ProfessionalRepository {
  if (JSON.stringify(profile) !== snapshot || !validProfile(profile))
    throw new IngestionError("stale")
  const next: ProfessionalRepository = structuredClone(profile)
  const created = new Map<string, ExperienceEntry | ProjectEntry>()
  const approvedOps = ops.filter((o) => o.approved)
  for (const op of approvedOps) {
    const claim: IngestionClaim = {
      id: op.claimId,
      source: "",
      text: "",
      targets: [op.target],
      question: "",
    }
    if (
      !string(op.value) ||
      !hasTarget(claim.targets, op.target) ||
      !["add", "update", "remove"].includes(op.action)
    )
      throw new IngestionError("invalid_output")
    if (op.action === "remove" && op.value !== "") throw new IngestionError("invalid_output")
    if (op.field === "current" && op.action !== "remove" && !["true","false"].includes(op.value)) throw new IngestionError("invalid_output")
    if (scalarFields.includes(op.target)) {
      if (op.field !== op.target || op.entryId)
        throw new IngestionError("invalid_output")
      const field =
        op.target as keyof Pick<ProfessionalRepository, "careerGoals" | "skills" | "competencies" | "tools" | "employmentStatus" | "currentSalary" | "desiredSalary" | "additionalInfo">
      const prior = next[field]
      next[field] = fieldResult(prior,op)
      if (
        field === "employmentStatus" &&
        next[field] &&
        !statuses.includes(next[field])
      )
        throw new IngestionError("invalid_output")
    } else {
      const fields =
        op.target === "experience" ? experienceFields : projectFields
      if (!fields.includes(op.field) || !op.entryId)
        throw new IngestionError("invalid_output")
      let target = entry(
        next,
        op.target,
        op.entryId,
      ) as unknown as Record<string, unknown> | undefined
      if (op.entryId.startsWith("new:")) {
        const anchorId = op.entryId.slice(4)
        if (
          !anchorId || op.action !== "add" ||
          !approvedOps.some(
            candidate =>
              candidate.claimId === anchorId &&
              candidate.target === op.target &&
              candidate.entryId === op.entryId,
          )
        )
          throw new IngestionError("invalid_output")
        const groupKey = op.target + "/" + op.entryId
        if (!created.has(groupKey)) {
          const id = crypto.randomUUID()
          const fresh =
            op.target === "experience"
              ? {
                  id,
                  company: "",
                  title: "",
                  startDate: "",
                  endDate: "",
                  current: false,
                  location: "",
                  description: "",
                  responsibilities: "",
                  achievements: "",
                } as ExperienceEntry
              : {
                  id,
                  name: "",
                  description: "",
                  technologies: "",
                  url: "",
                  highlights: "",
                } as ProjectEntry
          created.set(groupKey, fresh)
          if (op.target === "experience")
            next.experience.push(fresh as ExperienceEntry)
          else next.projects.push(fresh as ProjectEntry)
        }
        target = (created.get(groupKey) as unknown as Record<string, unknown>)
      }
      if (!target) throw new IngestionError("stale")
      const prior = String(target[op.field] ?? "")
      const result = fieldResult(prior,op)
      target[op.field] = op.field === "current" ? result === "true" : result
    }
  }
  if (
    [...created.values()].some((e) =>
      "company" in e ? !e.company.trim() || !e.title.trim() : !e.name.trim(),
    ) ||
    !validProfile(next)
  )
    throw new IngestionError("incomplete")
  const freshExperience = new Set<string>()
  const freshProjects = new Set<string>()
  for (const fresh of created.values()) {
    if ("company" in fresh) {
      const key = normalizedFact(fresh.company) + "/" + normalizedFact(fresh.title)
      if (
        freshExperience.has(key) ||
        profile.experience.some(
          (existing) =>
            normalizedFact(existing.company) + "/" +
              normalizedFact(existing.title) === key,
        )
      )
        throw new IngestionError("incomplete")
      freshExperience.add(key)
    } else {
      const key = normalizedFact(fresh.name)
      if (
        freshProjects.has(key) ||
        profile.projects.some(
          existing => normalizedFact(existing.name) === key,
        )
      )
        throw new IngestionError("incomplete")
      freshProjects.add(key)
    }
  }
  return next
}

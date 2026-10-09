import type {
  ProfileDocument,
  ProfileRef,
  ProfileFact,
} from "./profileDocument"
import { parseFieldDecision, type FieldDecision } from "./fieldDecision"
import { ProfessionalRepository } from "./types"
import { withContactFields, emptyContact, validQualifications } from "./profile"

export type IngestionTarget = "careerGoals" | "skills" | "competencies" | "experience" | "tools" | "projects" | "employmentStatus" | "currentSalary" | "desiredSalary" | "additionalInfo" | "fullName" | "email" | "phone" | "location" | "professionalLinks" | "education" | "certifications" | "languages"
export interface IngestionSourceReference {
  version: 1
  sourceId: string
  preparationVersion: "structure-v1"
  segmentId: string
  occurrenceId: string
  originalStart: number
  originalEnd: number
}
export interface IngestionMeaning {
  assertion: "affirmed" | "negated" | "unknown"
  intent: "actual" | "aspiration" | "unknown"
  certainty: "certain" | "uncertain" | "unknown"
  temporal: {
    wording: string
    precision: "exact" | "approximate" | "unknown"
  }
}
export interface IngestionClaim {
  meaning?: IngestionMeaning
  supportingSources?: {
    source: string
    sourceReference?: IngestionSourceReference
  }[]
  sourceReference?: IngestionSourceReference
  id: string
  source: string
  text: string
  targets: IngestionTarget[]
  question: string
}
export interface IngestionOperation {
  supportingClaimIds?: string[]
  proposedValue?: string
  claimId: string
  target: IngestionTarget
  entryId: string
  field: string
  action: "add" | "update" | "remove" | "evidence"
  value: string
  finding: "addition" | "overlap" | "conflict" | "in_place"
  approved: boolean
}
export const ingestionOutcomeKinds = [
  "change",
  "exact_duplicate",
  "overlap",
  "additional_support",
  "contradiction",
  "correction",
  "clarification",
  "unsupported",
  "unresolved",
] as const
export interface IngestionOutcome {
  claimId: string
  kind: typeof ingestionOutcomeKinds[number]
  reason: string
  relatedFacts: ProfileRef[]
  relatedClaimIds: string[]
  operationIndexes?: number[]
}
export interface IngestionCoverage {
  validClaims: number
  invalidClaims: number
  discoveryComplete: false
  capacity: "within_limit" | "possibly_exhausted"
}
export interface IngestionSkippedClaim {
  index: number
  reason: string
  text: string
  source: string
  shortened: boolean
}
export interface IngestionSourceRange {
  Start: number
  End: number
}
export interface IngestionPortionRequest {
  index: number
  bytes: number
}
export interface IngestionContinuation {
  sourceId: string
  index: number
  total: number
  bytes: number
  regions: IngestionSourceRange[][]
  remaining: IngestionSourceRange[] | null
  planComplete: boolean
  processed: boolean
}
export interface IngestionResult {
  continuation?: IngestionContinuation

  outcomes?: IngestionOutcome[]
  coverage?: IngestionCoverage
  skippedClaims?: IngestionSkippedClaim[]
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
  "Employed",
  "Employed — Full-time",
  "Employed — Part-time",
  "Employed — Contract",
  "Freelance / Self-employed",
  "Actively looking for work",
  "Open to opportunities (not actively searching)",
  "Unemployed",
  "Student",
]
const profileFields = [...scalarFields, "experience", "projects"]
const contactFields = Object.keys(emptyContact)
const writableScalars = [...scalarFields, ...contactFields]
const collectionFields: Record<string, string[]> = {
  experience: experienceFields,
  projects: projectFields,
  education: ["degree", "institution", "location", "graduationDate", "details"],
  certifications: ["name", "issuer", "date", "credentialId", "url"],
  languages: ["name", "proficiency"],
}
const targets = [...writableScalars, ...Object.keys(collectionFields)]
const identityFields = (target: string) =>
  target === "experience"
    ? ["company", "title", "startDate", "endDate"]
    : target === "certifications"
      ? ["name", "issuer"]
      : requiredFields(target)
const requiredFields = (target: string) =>
  target === "experience"
    ? ["company", "title"]
    : target === "education"
      ? ["degree", "institution"]
      : ["name"]
export const ingestionInputBytes = (input: string) =>
  new TextEncoder().encode(input).length
export const ingestionMaxBytes = 30000
export class IngestionError extends Error {
  constructor(
    public readonly code: string,
    public readonly decision?: FieldDecision,
  ) {
    super(code)
  }
}
const record = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === "object" && !Array.isArray(value)
const keys = (value: Record<string, unknown>, expected: readonly string[]) =>
  Object.keys(value).length === expected.length &&
  expected.every((key) => key in value)
const string = (value: unknown, max = 2000): value is string =>
  typeof value === "string" && ingestionInputBytes(value) <= max
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
  collectionFields[target]
    ? (profile[(target as keyof ProfessionalRepository)] as unknown as {
        id: string
      }[] | undefined)?.find((item) => item.id === id)
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
    .some((part) => normalizedFact(part) === normalizedFact(value))

export function validProfile(profile: ProfessionalRepository): boolean {
  if (
    !record(profile) ||
    !(
      keys(profile, profileFields) ||
      keys(profile, [...profileFields, ...contactFields]) ||
      keys(profile, [
        ...profileFields,
        ...contactFields,
        "education",
        "certifications",
        "languages",
      ])
    ) ||
    contactFields.some(
      (f) =>
        f in profile &&
        !string(profile[(f as keyof ProfessionalRepository)], 2000),
    ) ||
    scalarFields.some(
      (f) => !string(profile[(f as keyof ProfessionalRepository)], 12 << 10),
    )
  )
    return false
  if (
    profile.employmentStatus &&
    !statuses.includes(profile.employmentStatus) &&
    !legacyStatuses.includes(profile.employmentStatus)
  )
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
  if ("education" in profile) {
    if (!validQualifications(profile)) return false
    for (const item of [
      ...profile.education,
      ...profile.certifications,
      ...profile.languages,
    ]) {
      if (ids.has(item.id)) return false
      ids.add(item.id)
    }
  }
  return true
}

// Ranges are UTF-8 bytes, not JavaScript UTF-16 indices. Fatal decoding rejects
// references that split a character. The source excerpt stays in original form.
export function validSourceReference(
  value: unknown,
  input: string,
  excerpt: string,
): boolean {
  if (
    !record(value) ||
    !keys(value, [
      "version",
      "sourceId",
      "preparationVersion",
      "segmentId",
      "occurrenceId",
      "originalStart",
      "originalEnd",
    ]) ||
    value.version !== 1 ||
    value.preparationVersion !== "structure-v1" ||
    [value.sourceId, value.segmentId, value.occurrenceId].some(
      (id) => typeof id !== "string" || !/^[a-f0-9]{64}$/.test(id),
    ) ||
    !Number.isInteger(value.originalStart) ||
    !Number.isInteger(value.originalEnd)
  )
    return false
  const start = value.originalStart as number,
    end = value.originalEnd as number
  const bytes = new TextEncoder().encode(input)
  if (start < 0 || end <= start || end > bytes.length) return false
  try {
    return (
      new TextDecoder("utf-8", { fatal: true }).decode(
        bytes.slice(start, end),
      ) === excerpt
    )
  } catch {
    return false
  }
}

async function sourceIdentity(input: string): Promise<string> {
  // Match Go encoding/json's HTML and line-separator escaping for the pinned
  // shared Source identity namespace, including the complete original paste.
  const identity = JSON.stringify([
    "normalization-v1",
    "structure-v1",
    "professional_information",
    input,
  ]).replace(
    /[<>&\u2028\u2029]/g,
    (c) => "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0"),
  )
  const digest = await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(identity),
  )
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("")
}

export function validateIngestionResult(
  raw: unknown,
  input: string,
  profile: ProfessionalRepository,
  document?: ProfileDocument,
): IngestionResult {
  if (
    !record(raw) ||
    !keys(
      Object.fromEntries(
        Object.entries(raw).filter(
          ([key]) =>
            !["outcomes", "coverage", "skippedClaims", "continuation"].includes(
              key,
            ),
        ),
      ),
      [
        "claims",
        "operations",
        "unverifiedClaimCount",
        "unresolvedClaimIds",
        "unplacedOperationCount",
      ],
    ) ||
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
      !keys(
        Object.fromEntries(
          Object.entries(item).filter(
            ([k]) =>
              !["sourceReference", "meaning", "supportingSources"].includes(k),
          ),
        ),
        ["id", "source", "text", "targets", "question"],
      ) ||
      !string(item.id, 40) ||
      !item.id ||
      ids.has(item.id) ||
      !string(item.source, 1000) ||
      !item.source ||
      !input.includes(item.source) ||
      ("sourceReference" in item &&
        !validSourceReference(item.sourceReference, input, item.source)) ||
      !string(item.text, 1000) ||
      !item.text.trim() ||
      !Array.isArray(item.targets) ||
      item.targets.length > targets.length ||
      item.targets.some((t: unknown) => !targets.includes(t as string)) ||
      !string(item.question, 500)
    )
      throw new IngestionError("invalid_output")
    if (item.meaning !== undefined && !validMeaning(item.meaning))
      throw new IngestionError("invalid_output")
    if (
      item.supportingSources !== undefined &&
      (!Array.isArray(item.supportingSources) ||
        item.supportingSources.length > 12 ||
        item.supportingSources.some(
          (s) =>
            !record(s) ||
            !keys(s, ["source", "sourceReference"]) ||
            !string(s.source, 1000) ||
            !s.source ||
            !validSourceReference(s.sourceReference, input, s.source),
        ))
    )
      throw new IngestionError("invalid_output")
    if (item.meaning !== undefined) {
      const meaning = item.meaning as unknown as IngestionMeaning
      const sources = [
        item.source,
        ...(
          item.supportingSources as { source: string }[] | undefined ?? []
        ).map((s) => s.source),
      ].join("\n")
      if (
        (meaning.temporal.precision !== "unknown" &&
          !meaning.temporal.wording) ||
        (meaning.temporal.wording &&
          !sources.includes(meaning.temporal.wording))
      )
        throw new IngestionError("invalid_output")
    }
    ids.add(item.id)
    claims.push(item as unknown as IngestionClaim)
  }
  const unresolvedClaimIds = raw.unresolvedClaimIds as unknown[]
  if (
    unresolvedClaimIds.length > claims.length ||
    new Set(unresolvedClaimIds).size !== unresolvedClaimIds.length ||
    unresolvedClaimIds.some((id) => typeof id !== "string" || !ids.has(id))
  )
    throw new IngestionError("invalid_output")
  const operations: IngestionOperation[] = []
  for (const item of raw.operations) {
    if (
      !record(item) ||
      !keys(
        Object.fromEntries(
          Object.entries(item).filter(([k]) => k !== "supportingClaimIds"),
        ),
        ["claimId", "target", "entryId", "field", "action", "value", "finding"],
      ) ||
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
      item.finding === "conflict" ||
      !hasTarget(claim.targets, item.target) ||
      !["add", "update", "remove", "evidence"].includes(
        item.action as string,
      ) ||
      !["addition", "overlap", "conflict", "in_place"].includes(
        item.finding as string,
      )
    )
      throw new IngestionError("invalid_output")
    if (
      writableScalars.includes(item.target)
        ? item.field !== item.target || item.entryId !== ""
        : !(collectionFields[item.target] ?? []).includes(
            item.field as string,
          ) ||
          !((item.entryId as string).startsWith("new:")
            ? claims.some(
                (anchor) =>
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
      item.field === "current" &&
      item.action !== "remove" &&
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
    if (
      item.supportingClaimIds !== undefined &&
      (!Array.isArray(item.supportingClaimIds) ||
        !item.supportingClaimIds.length ||
        item.supportingClaimIds.length > 12 ||
        new Set(item.supportingClaimIds).size !==
          item.supportingClaimIds.length ||
        !item.supportingClaimIds.includes(claim.id) ||
        item.supportingClaimIds.some(
          (id) =>
            !claims.some(
              (c) =>
                c.id === id &&
                !c.question &&
                !unresolvedClaimIds.includes(c.id),
            ),
        ))
    )
      throw new IngestionError("invalid_output")
    operations.push({
      ...item,
      proposedValue: item.value,
      approved: false,
    } as IngestionOperation)
  }
  const newGroups = new Map<string, IngestionOperation[]>()
  for (const operation of operations) {
    if (!operation.entryId.startsWith("new:")) continue
    const key = operation.target + "/" + operation.entryId
    newGroups.set(key, [...(newGroups.get(key) ?? []), operation])
  }
  for (const group of newGroups.values()) {
    const anchor = group[0].entryId.slice(4)
    const fields = new Set(group.map((operation) => operation.field))
    if (
      !group.some((operation) => operation.claimId === anchor) ||
      requiredFields(group[0].target).some((field) => !fields.has(field))
    )
      throw new IngestionError("invalid_output")
  }
  const ledger = validateOutcomeLedger(raw, claims, operations, input, document)
  return {
    ...ledger,
    claims,
    operations,
    unverifiedClaimCount: raw.unverifiedClaimCount as number,
    unresolvedClaimIds: unresolvedClaimIds as string[],
    unplacedOperationCount: raw.unplacedOperationCount as number,
  }
}

export function validMeaning(value: unknown): value is IngestionMeaning {
  if (
    !record(value) ||
    !keys(value, ["assertion", "intent", "certainty", "temporal"]) ||
    !record(value.temporal) ||
    !keys(value.temporal, ["wording", "precision"])
  )
    return false
  return (
    ["affirmed", "negated", "unknown"].includes(String(value.assertion)) &&
    ["actual", "aspiration", "unknown"].includes(String(value.intent)) &&
    ["certain", "uncertain", "unknown"].includes(String(value.certainty)) &&
    string(value.temporal.wording, 1000) &&
    ["exact", "approximate", "unknown"].includes(
      String(value.temporal.precision),
    )
  )
}

function validateOutcomeLedger(
  raw: Record<string, unknown>,
  claims: IngestionClaim[],
  operations: IngestionOperation[],
  input: string,
  document?: ProfileDocument,
): Pick<IngestionResult, "outcomes" | "coverage" | "skippedClaims"> {
  const fail = (): never => {
    throw new IngestionError("invalid_output")
  }
  if (!("outcomes" in raw) && !document) return {}
  if (
    !Array.isArray(raw.outcomes) ||
    raw.outcomes.length !== claims.length ||
    !record(raw.coverage) ||
    !keys(raw.coverage, [
      "validClaims",
      "invalidClaims",
      "discoveryComplete",
      "capacity",
    ]) ||
    raw.coverage.validClaims !== claims.length ||
    raw.coverage.invalidClaims !== raw.unverifiedClaimCount ||
    raw.coverage.discoveryComplete !== false ||
    raw.coverage.capacity !==
      (claims.length + Number(raw.unverifiedClaimCount) === 30
        ? "possibly_exhausted"
        : "within_limit") ||
    !Array.isArray(raw.skippedClaims) ||
    raw.skippedClaims.length !== raw.unverifiedClaimCount
  )
    return fail()
  const skippedIndexes = new Set<number>()
  for (const item of raw.skippedClaims) {
    if (
      !record(item) ||
      !keys(item, ["index", "reason", "text", "source", "shortened"]) ||
      !Number.isInteger(item.index) ||
      Number(item.index) < 1 ||
      Number(item.index) > 30 ||
      skippedIndexes.has(Number(item.index)) ||
      !string(item.reason, 100) ||
      !item.reason ||
      !string(item.text, 1000) ||
      !string(item.source, 1000) ||
      (item.source && !input.includes(item.source)) ||
      typeof item.shortened !== "boolean"
    )
      return fail()
    skippedIndexes.add(Number(item.index))
  }
  const accounted = new Set<string>()
  const referencedOperations = new Set<number>()
  for (const item of raw.outcomes) {
    if (
      !record(item) ||
      !keys(
        Object.fromEntries(
          Object.entries(item).filter(([k]) => k !== "operationIndexes"),
        ),
        ["claimId", "kind", "reason", "relatedFacts", "relatedClaimIds"],
      ) ||
      !string(item.claimId, 40) ||
      accounted.has(item.claimId) ||
      !claims.some((c) => c.id === item.claimId) ||
      !ingestionOutcomeKinds.includes(item.kind as IngestionOutcome["kind"]) ||
      !string(item.reason, 500) ||
      !item.reason.trim() ||
      !Array.isArray(item.relatedFacts) ||
      item.relatedFacts.length > 12 ||
      !Array.isArray(item.relatedClaimIds) ||
      item.relatedClaimIds.length > 12 ||
      new Set(item.relatedClaimIds).size !== item.relatedClaimIds.length ||
      item.relatedClaimIds.some(
        (id) =>
          typeof id !== "string" ||
          id === item.claimId ||
          !claims.some((c) => c.id === id),
      )
    )
      return fail()
    accounted.add(item.claimId)
    const factIds = new Set<string>()
    for (const ref of item.relatedFacts) {
      if (
        !record(ref) ||
        !keys(ref, ["profileId", "id", "revision"]) ||
        !string(ref.id, 100) ||
        !ref.id ||
        !string(ref.profileId, 100) ||
        !Number.isSafeInteger(ref.revision) ||
        Number(ref.revision) < 1 ||
        factIds.has(ref.id)
      )
        return fail()
      factIds.add(ref.id)
      if (
        !document ||
        ref.profileId !== document.id ||
        !document.facts.some(
          (f) => f.id === ref.id && f.revision === ref.revision,
        )
      )
        return fail()
    }
    if (
      ["exact_duplicate", "additional_support"].includes(String(item.kind)) &&
      (!item.relatedFacts.length ||
        !document ||
        item.relatedFacts.some(
          (ref) =>
            !exactIngestionFact(
              claims.find((c) => c.id === item.claimId)!,
              document.facts.find((f) => f.id === ref.id)!,
              document,
            ),
        ))
    )
      return fail()
    if (
      ["overlap", "contradiction", "correction"].includes(String(item.kind)) &&
      item.relatedFacts.length + item.relatedClaimIds.length === 0
    )
      return fail()
    const indexes = item.operationIndexes ?? []
    if (
      !Array.isArray(indexes) ||
      indexes.length > 60 ||
      new Set(indexes).size !== indexes.length
    )
      return fail()
    for (const index of indexes) {
      if (
        !Number.isInteger(index) ||
        index < 0 ||
        index >= operations.length ||
        operations[index].claimId !== item.claimId ||
        referencedOperations.has(index)
      )
        return fail()
      referencedOperations.add(index)
      const op = operations[index]
      if (op.action === "evidence") {
        if (
          item.kind !== "additional_support" ||
          !document ||
          !item.relatedFacts.some((ref) =>
            document.facts.some(
              (f) =>
                f.id === ref.id &&
                f.field === op.field &&
                f.value === op.value &&
                (f.owner.id === document.id
                  ? op.target === op.field && op.entryId === ""
                  : document.entities.some(
                      (e) =>
                        e.id === f.owner.id &&
                        e.kind === op.target &&
                        e.legacyId === op.entryId,
                    )),
            ),
          )
        )
          return fail()
      } else if (!["change", "overlap"].includes(String(item.kind)))
        return fail()
    }
    if (
      ["change", "overlap", "additional_support"].includes(
        String(item.kind),
      ) !==
      indexes.length > 0
    )
      return fail()
    if (
      claims.find((c) => c.id === item.claimId)?.question &&
      !["clarification", "contradiction", "correction"].includes(
        String(item.kind),
      )
    )
      return fail()
    if (
      (raw.unresolvedClaimIds as string[]).includes(item.claimId) !==
      (item.kind === "unresolved")
    )
      return fail()
  }
  if (referencedOperations.size !== operations.length) return fail()
  return {
    outcomes: raw.outcomes as IngestionOutcome[],
    coverage: raw.coverage as unknown as IngestionCoverage,
    skippedClaims: raw.skippedClaims as IngestionSkippedClaim[],
  }
}

export function ingestionIdentityFacts(
  doc: ProfileDocument,
  fact: ProfileFact,
): ProfileFact[] {
  return doc.facts.filter(
    (f) =>
      (f.owner.id === fact.owner.id ||
        fact.context.some((r) => r.id === f.owner.id)) &&
      [
        "company",
        "title",
        "startDate",
        "endDate",
        "name",
        "degree",
        "institution",
        "issuer",
      ].includes(f.field) &&
      typeof f.value === "string" &&
      !!f.value,
  )
}

// Only exact wording and the pinned alias policy can establish equivalence.
// A generated summary never establishes the meaning of its original source.
function exactIngestionFact(
  claim: IngestionClaim,
  fact: ProfileFact,
  doc: ProfileDocument,
): boolean {
  const aliases: Record<string, string> = {
    Javascript: "JavaScript",
    Typescript: "TypeScript",
  }
  const alias = (value: string) => aliases[value] ?? value
  const capability = ["skills", "tools", "competencies"]
  const use =
    claim.targets.length === 1 && claim.targets[0] === "skills"
      ? /^(?:I use|Eu uso)\s+([a-z][a-z0-9_+#-]{0,59})[.!]?$/i.exec(
          claim.source.trim(),
        )?.[1]
      : undefined
  const value = alias(use ?? claim.source.trim())
  if (typeof fact.value !== "string" || claim.question) return false
  const meaning = claim.meaning
  if (fact.kind === "statement" && meaning) {
    if (
      meaning.assertion !== fact.assertion ||
      meaning.intent !== fact.intent ||
      meaning.certainty !== fact.certainty ||
      meaning.temporal.wording !== fact.temporal.wording ||
      meaning.temporal.precision !== fact.temporal.precision
    )
      return false
  } else if (
    meaning &&
    (meaning.assertion !== "affirmed" ||
      meaning.intent !== "actual" ||
      meaning.certainty !== "certain" ||
      meaning.temporal.wording !== "")
  )
    return false
  if (
    fact.kind === "statement" &&
    !meaning &&
    (fact.assertion !== "affirmed" ||
      fact.intent !== "actual" ||
      fact.certainty !== "certain" ||
      fact.temporal.wording !== "")
  )
    return false
  const sharedCapability =
    fact.owner.id === doc.id &&
    !fact.context.length &&
    capability.includes(fact.field) &&
    claim.targets.length === 1 &&
    capability.includes(claim.targets[0])
  if (
    !(
      alias(fact.value) === value ||
      (sharedCapability &&
        fact.value
          .split(/[\n,;]+/)
          .some((part) => alias(part.trim()) === value))
    )
  )
    return false
  const entity = doc.entities.find((e) => e.id === fact.owner.id)
  if (
    !sharedCapability &&
    !claim.targets.includes((entity?.kind ?? fact.field) as IngestionTarget)
  )
    return false
  if (fact.owner.id !== doc.id || fact.context.length) {
    const identity = ingestionIdentityFacts(doc, fact)
    const source = [
      claim.source,
      ...(claim.supportingSources ?? []).map((s) => s.source),
    ].join("\n")
    if (
      !identity.length ||
      identity.some((f) => !source.includes(String(f.value)))
    )
      return false
  }
  return true
}

export async function ingestProfile(
  input: string,
  profile: ProfessionalRepository,
  apiKey: string,
  signal?: AbortSignal,
  typesafeKey = "",
  document?: ProfileDocument,
  portion?: IngestionPortionRequest,
): Promise<IngestionResult> {
  if (
    !input.trim() ||
    ingestionInputBytes(input) > ingestionMaxBytes ||
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
        "X-TypeSafe-Api-Key": typesafeKey,
      },
      body: JSON.stringify({
        input,
        profile: withContactFields(profile),
        ...(document ? { document } : {}),
        ...(portion ? { portion } : {}),
      }),
      cache: "no-store",
      signal: signal
        ? AbortSignal.any([signal, AbortSignal.timeout(55_000)])
        : AbortSignal.timeout(55_000),
    })
  } catch (error) {
    throw new IngestionError(
      error instanceof DOMException && error.name === "TimeoutError"
        ? "timeout"
        : "outage",
    )
  }
  const raw = await response.json().catch(() => null)
  const decision = record(raw)
    ? parseFieldDecision(raw.decision, "professional_information")
    : null
  if (decision && decision.outcome.kind !== "accept") {
    if (!record(raw) || !keys(raw, ["decision"]))
      throw new IngestionError("invalid_output")
    throw new IngestionError("field_decision", decision)
  }
  if (record(raw) && "decision" in raw && !decision)
    throw new IngestionError("invalid_output")
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
          "preparation",
          "capacity",
        ].includes(raw.error)
        ? raw.error
        : "outage",
    )
  if (!decision || !record(raw)) throw new IngestionError("invalid_output")
  const { decision: _decision, ...proposals } = raw
  if (!("outcomes" in proposals)) throw new IngestionError("invalid_output")
  const result = validateIngestionResult(proposals, input, profile, document)
  if (
    result.claims.some(
      (claim) => claim.sourceReference || claim.supportingSources?.length,
    )
  ) {
    const expected = await sourceIdentity(input)
    if (
      result.claims.some(
        (claim) =>
          (claim.sourceReference &&
            claim.sourceReference.sourceId !== expected) ||
          claim.supportingSources?.some(
            (s) => s.sourceReference?.sourceId !== expected,
          ),
      )
    )
      throw new IngestionError("invalid_output")
  }
  if (portion) {
    const progress = raw.continuation
    const size = ingestionInputBytes(input)
    const range = (r: unknown) =>
      record(r) &&
      keys(r, ["Start", "End"]) &&
      typeof r.Start === "number" &&
      typeof r.End === "number" &&
      Number.isInteger(r.Start) &&
      Number.isInteger(r.End) &&
      r.Start >= 0 &&
      r.End > r.Start &&
      r.End <= size
    if (
      !record(progress) ||
      !keys(progress, [
        "sourceId",
        "index",
        "total",
        "bytes",
        "regions",
        "remaining",
        "planComplete",
        "processed",
      ]) ||
      progress.sourceId !== (await sourceIdentity(input)) ||
      progress.index !== portion.index ||
      progress.bytes !== portion.bytes ||
      typeof progress.total !== "number" ||
      !Number.isInteger(progress.total) ||
      progress.total < 1 ||
      progress.total > 256 ||
      portion.index >= progress.total ||
      typeof progress.planComplete !== "boolean" ||
      typeof progress.processed !== "boolean" ||
      !Array.isArray(progress.regions) ||
      progress.regions.length !== progress.total ||
      !progress.regions.every(
        (rs) => Array.isArray(rs) && rs.length > 0 && rs.every(range),
      ) ||
      !(
        progress.remaining === null ||
        (Array.isArray(progress.remaining) && progress.remaining.every(range))
      ) ||
      (progress.processed &&
        (result.unverifiedClaimCount > 0 ||
          result.claims.length >= 30 ||
          result.operations.length >= 60 ||
          result.unplacedOperationCount > 0))
    )
      throw new IngestionError("invalid_output")
    const ordered = [
      ...progress.regions.flat(),
      ...(progress.remaining ?? []),
    ] as IngestionSourceRange[]
    if (
      ordered[0]?.Start !== 0 ||
      ordered.at(-1)?.End !== size ||
      ordered.some((r, i) => i > 0 && r.Start !== ordered[i - 1].End) ||
      progress.planComplete !== !progress.remaining?.length
    )
      throw new IngestionError("invalid_output")
    result.continuation = (progress as unknown as IngestionContinuation)
  } else if (raw.continuation !== undefined)
    throw new IngestionError("invalid_output")
  return result
}

export function beforeValue(
  profile: ProfessionalRepository,
  op: IngestionOperation,
): string {
  if (writableScalars.includes(op.target))
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
  if (op.action === "evidence") return before
  if (op.field === "current") return op.action === "remove" ? "false" : op.value
  if (
    [
      "employmentStatus",
      ...contactFields.filter((field) => field !== "professionalLinks"),
      "startDate",
      "endDate",
      "graduationDate",
      "date",
      "proficiency",
      "credentialId",
      "url",
    ].includes(op.field)
  )
    return op.action === "remove" ? "" : op.value.trim()
  if (op.action === "remove") return ""
  return op.action === "add" && before.trim()
    ? duplicate(before, op.value)
      ? before
      : before.trimEnd() + "\n" + op.value.trim()
    : op.value.trim()
}

export function previewValues(
  profile: ProfessionalRepository,
  ops: IngestionOperation[],
  index: number,
): {
  before: string
  after: string
} {
  const selected = ops[index]
  let before = beforeValue(profile, selected)
  for (const prior of ops.slice(0, index)) {
    if (
      prior.approved &&
      prior.target === selected.target &&
      prior.entryId === selected.entryId &&
      prior.field === selected.field
    )
      before = fieldResult(before, prior)
  }
  return { before, after: fieldResult(before, selected) }
}

export function applyIngestion(
  profile: ProfessionalRepository,
  snapshot: string,
  ops: IngestionOperation[],
): ProfessionalRepository {
  if (JSON.stringify(profile) !== snapshot || !validProfile(profile))
    throw new IngestionError("stale")
  const next: ProfessionalRepository = structuredClone(
    withContactFields(profile),
  )
  const created = new Map<string, Record<string, unknown>>()
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
      !["add", "update", "remove", "evidence"].includes(op.action)
    )
      throw new IngestionError("invalid_output")
    if (op.action === "remove" && op.value !== "")
      throw new IngestionError("invalid_output")
    if (
      op.field === "current" &&
      op.action !== "remove" &&
      !["true", "false"].includes(op.value)
    )
      throw new IngestionError("invalid_output")
    if (writableScalars.includes(op.target)) {
      if (op.field !== op.target || op.entryId)
        throw new IngestionError("invalid_output")
      const values = next as unknown as Record<string, string>
      const field = op.target
      const prior = values[field]
      values[field] = fieldResult(prior, op)
      if (
        field === "employmentStatus" &&
        values[field] &&
        !statuses.includes(values[field])
      )
        throw new IngestionError("invalid_output")
    } else {
      const fields = collectionFields[op.target] ?? []
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
          !anchorId ||
          op.action !== "add" ||
          !approvedOps.some(
            (candidate) =>
              candidate.claimId === anchorId &&
              candidate.target === op.target &&
              candidate.entryId === op.entryId,
          )
        )
          throw new IngestionError("invalid_output")
        const groupKey = op.target + "/" + op.entryId
        if (!created.has(groupKey)) {
          const id = crypto.randomUUID()
          const fresh: Record<string, unknown> = { id }
          for (const field of fields)
            fresh[field] = field === "current" ? false : ""
          created.set(groupKey, fresh)
          const collection = next[
            (op.target as keyof ProfessionalRepository)
          ] as unknown as Record<string, unknown>[]
          collection.push(fresh)
        }
        target = (created.get(groupKey) as unknown as Record<string, unknown>)
      }
      if (!target) throw new IngestionError("stale")
      const prior = String(target[op.field] ?? "")
      const result = fieldResult(prior, op)
      target[op.field] = op.field === "current" ? result === "true" : result
    }
  }
  for (const [groupKey, fresh] of created) {
    const target = groupKey.split("/")[0]
    if (requiredFields(target).some((field) => !String(fresh[field]).trim()))
      throw new IngestionError("incomplete")
    const identity = (item: Record<string, unknown>) =>
      identityFields(target)
        .map((field) => normalizedFact(String(item[field])))
        .join("/")
    const collection = next[
      (target as keyof ProfessionalRepository)
    ] as unknown as Record<string, unknown>[]
    if (
      collection.filter((item) => identity(item) === identity(fresh)).length > 1
    )
      throw new IngestionError("incomplete")
  }
  if (!validProfile(next)) throw new IngestionError("incomplete")
  return next
}

import type {
  ProfileDocument,
  ProfileRef,
  ProfileFact,
  ProfileEvidence,
} from "./profileDocument"
import type { ProfessionalRepository } from "./types"
import type { Locale } from "./i18n"
import type { CvDensity } from "./cvPreferences"
import type { CvChoices } from "./cv"

export const CURATED_CV_KEY = "careeros_curated_cv_v1"
export interface CvFact {
  id: string
  section: string
  entryId: string
  field: string
  text: string
  reference?: ProfileRef
  owner?: ProfileRef
  kind?: ProfileFact["kind"]
  assertion?: ProfileFact["assertion"]
  intent?: ProfileFact["intent"]
  certainty?: ProfileFact["certainty"]
  support?: ProfileFact["support"]
  evidence?: ProfileEvidence[]
  context?: ProfileRef[]
}
// ADR 0003 statement-support contract (issue #125). One review unit exists per
// generated summary sentence and per optional rewrite, keyed by the
// server-assigned id `summary:<index>` / `wording:<sourceId>`.
export const CV_SUPPORT_POLICY = "cv-statement-support-v1"
// ADR 0003: switching the production request to the v3 statement-support
// contract is the coordinated activation owned by #128. Until then the client
// keeps the legacy v2 request, but still consumes a v3 response whenever an
// activated backend (or a controlled fixture) returns one.
export const CV_SUPPORT_REQUEST = false
export type CvSupportStatus = "supported" | "uncertain" | "unsupported"
export interface CvSupportStatement {
  id: string
  text: string
  sourceIds: string[]
  status: CvSupportStatus
  reason: string
}
export interface CvSupport {
  policyVersion: string
  assessment: "complete" | "unavailable"
  statements: CvSupportStatement[]
}
export interface CvResult {
  summary: {
    sourceId?: string
    sourceIds?: string[]
    text: string
  }[]
  selected: string[]
  wording?: Record<string, string>
  // Present only for the opt-in v3 response; absence means legacy v2 wording
  // without a per-statement machine judgment.
  support?: CvSupport
}
export interface CuratedCv {
  version: 1 | 2 | 3
  sourceProfileId?: string
  summarySources?: CvResult["summary"]
  // ADR 0003 / issue #126: per-statement accepted decisions and user provenance.
  // Present only on version 3 snapshots produced from an assessed v3 result.
  provenance?: CvReviewRecord[]
  context?: {
    reference: ProfileRef
    value: unknown
  }[]
  density?: CvDensity
  choices?: CvChoices
  repository: ProfessionalRepository
  summary: string
  sources: CvFact[]
  cvLanguage: Locale
}
const substantive = new Set([
  "skills",
  "competencies",
  "tools",
  "description",
  "responsibilities",
  "achievements",
  "highlights",
  "degree",
  "details",
  "name",
  "proficiency",
])
export function cvFacts(repo: ProfessionalRepository): CvFact[] {
  const facts: CvFact[] = []
  const add = (
    section: string,
    entryId: string,
    field: string,
    text: string,
    split = false,
  ) => {
    const seen = new Set<string>()
    for (const value of split
      ? text.split(
          field === "skills" || field === "competencies" || field === "tools"
            ? /\n|,/
            : /\n/,
        )
      : [text]) {
      const cleaned = value.replace(/^[-•*]\s*/, "").trim()
      if (!cleaned || seen.has(cleaned.toLowerCase())) continue
      seen.add(cleaned.toLowerCase())
      facts.push({
        id: `f${facts.length}`,
        section,
        entryId,
        field,
        text: cleaned,
      })
    }
  }
  for (const field of ["skills", "competencies", "tools"] as const)
    add(
      field === "competencies" ? "skills" : field,
      "",
      field,
      repo[field],
      true,
    )
  for (const e of repo.experience) {
    for (const field of [
      "title",
      "company",
      "startDate",
      "endDate",
      "description",
      "responsibilities",
      "achievements",
    ] as const)
      add("experience", e.id, field, e[field], substantive.has(field))
    if (e.current) add("experience", e.id, "current", "true")
  }
  for (const p of repo.projects)
    for (const field of [
      "name",
      "description",
      "technologies",
      "highlights",
    ] as const)
      add("projects", p.id, field, p[field], substantive.has(field))
  for (const e of repo.education)
    for (const field of [
      "degree",
      "institution",
      "graduationDate",
      "details",
    ] as const)
      add("education", e.id, field, e[field])
  for (const c of repo.certifications)
    for (const field of ["name", "issuer", "date"] as const)
      add("certifications", c.id, field, c[field])
  for (const l of repo.languages)
    for (const field of ["name", "proficiency"] as const)
      add("languages", l.id, field, l[field])
  return facts
}
// Canonical facts are indivisible, including migrated legacy blocks. IDs never
// depend on position, spelling, or a compatibility-view split.
export function stableCvFacts(doc: ProfileDocument): CvFact[] {
  const allowed: Record<string, readonly string[]> = {
    skills: ["skills", "competencies"],
    tools: ["tools"],
    experience: [
      "title",
      "company",
      "startDate",
      "endDate",
      "current",
      "description",
      "responsibilities",
      "achievements",
    ],
    projects: ["name", "description", "technologies", "highlights"],
    education: ["degree", "institution", "graduationDate", "details"],
    certifications: ["name", "issuer", "date"],
    languages: ["name", "proficiency"],
  }
  return doc.facts.flatMap((f) => {
    const entity = doc.entities.find((e) => e.id === f.owner.id)
    const section =
      entity?.kind ?? (f.field === "competencies" ? "skills" : f.field)
    const text =
      f.value === true ? "true" : typeof f.value === "string" ? f.value : ""
    if (!allowed[section]?.includes(f.field) || !text.trim()) return []
    const evidenceIds = doc.links
      .filter(
        (l) =>
          l.kind === "supports" &&
          l.state === "active" &&
          l.from.id === f.id &&
          l.from.revision === f.revision,
      )
      .map((l) => l.to.id)
    return [
      {
        id: f.id,
        section,
        entryId: entity?.legacyId ?? "",
        field: f.field,
        text,
        reference: { profileId: doc.id, id: f.id, revision: f.revision },
        owner: structuredClone(f.owner),
        context: structuredClone(f.context),
        kind: f.kind,
        assertion: f.assertion,
        intent: f.intent,
        certainty: f.certainty,
        support: f.support,
        evidence: structuredClone(
          doc.evidence.filter((e) => evidenceIds.includes(e.id)),
        ),
      },
    ]
  })
}

// Numeric and explicit factual-strength guards complement cited sources and human review.
export function groundedCvText(text: unknown, source: string): text is string {
  if (
    typeof text !== "string" ||
    !text.trim() ||
    new TextEncoder().encode(text).length > 12_288
  )
    return false
  const normalize = (s: string) =>
    s.toLowerCase()
      .normalize("NFD")
      .replace(/[\u0300-\u036f]/g, "")
  const output = normalize(text),
    input = normalize(source)
  const numbers: string[] = source.match(/\d+(?:[.,]\d+)*(?:%|\+)?/g) || []
  if (
    (text.match(/\d+(?:[.,]\d+)*(?:%|\+)?/g) || []).some(
      (n) => !numbers.includes(n),
    )
  )
    return false
  const markers = [
    "ceo",
    "cto",
    "cfo",
    "cio",
    "director",
    "manager",
    "managed",
    "senior",
    "principal",
    "owner",
    "owned",
    "led",
    "lead",
    "leader",
    "certified",
    "certification",
    "fluent",
    "fluency",
    "native",
    "expert",
    "master",
    "doctor",
    "certificado",
    "certificacao",
    "fluente",
    "fluencia",
    "nativo",
    "especialista",
    "mestre",
    "doutor",
  ]
  if (
    markers.some(
      (m) =>
        new RegExp(`\\b${m}\\b`).test(output) &&
        !new RegExp(`\\b${m}\\b`).test(input),
    )
  )
    return false
  // Legacy blocks have no inferred atomic qualifiers. Explicit aspirational or
  // uncertain wording is retained verbatim rather than upgraded by paraphrasing.
  const qualified =
    /\b(?:hope|want|aspire|aspiring|plan|wish|might|maybe|perhaps|possibly|unknown|uncertain|talvez|espero|pretendo|desejo|incerto|desconhecido)\b/
  if (qualified.test(input) && text !== source) return false
  const negatives = /\b(?:not|never|without|nao|nunca|sem)\b/
  if (negatives.test(input) && !negatives.test(output)) return false
  return true
}
// Explicit user-authored corrections (#126) are the user's own assertion. They
// still require a valid shape and unchanged cited references, but the
// deterministic numeric/marker grounding applies to machine text only. The
// `authored` override carries the exact accepted user text per retained summary
// index and wording source, so validation never accepts unrecorded text.
export interface CvAuthoredText {
  summary?: (string | undefined)[]
  wording?: Record<string, string>
}
export function userAuthoredCvText(text: unknown): text is string {
  return (
    typeof text === "string" &&
    !!text.trim() &&
    new TextEncoder().encode(text).length <= 12_288
  )
}
function authoredWordingValid(
  id: string,
  text: unknown,
  authored: CvAuthoredText | undefined,
  fact: CvFact,
): boolean {
  if (authored?.wording?.[id] === text) return userAuthoredCvText(text)
  return (
    groundedCvText(text, fact.text) &&
    !(
      fact.kind === "statement" &&
      (fact.intent !== "actual" ||
        fact.assertion !== "affirmed" ||
        fact.certainty !== "certain") &&
      text !== fact.text
    )
  )
}
export function validateCvResult(
  value: unknown,
  facts: CvFact[],
  authored?: CvAuthoredText,
): value is CvResult {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false
  const r = value as CvResult
  if (
    !["selected,summary", "selected,summary,wording"].includes(
      Object.keys(r).sort().join(","),
    ) ||
    !Array.isArray(r.selected) ||
    !r.selected.length ||
    r.selected.length > facts.length ||
    !Array.isArray(r.summary) ||
    !r.summary.length ||
    r.summary.length > 6
  )
    return false
  const byId = new Map(facts.map((f) => [f.id, f]))
  const selected = new Set(r.selected)
  if (
    selected.size !== r.selected.length ||
    r.selected.some(
      (id) =>
        typeof id !== "string" ||
        !byId.has(id) ||
        (byId.get(id)!.kind === "statement" &&
          (byId.get(id)!.intent !== "actual" ||
            byId.get(id)!.assertion !== "affirmed" ||
            byId.get(id)!.certainty !== "certain")),
    )
  )
    return false
  if (
    facts.some(
      (f) =>
        f.entryId &&
        f.kind === "statement" &&
        (f.intent !== "actual" ||
          f.assertion !== "affirmed" ||
          f.certainty !== "certain") &&
        ![
          "description",
          "responsibilities",
          "achievements",
          "highlights",
          "details",
        ].includes(f.field) &&
        r.selected.some(
          (id) =>
            byId.get(id)?.section === f.section &&
            byId.get(id)?.entryId === f.entryId,
        ),
    )
  )
    return false
  if (
    r.wording !== undefined &&
    (!r.wording ||
      typeof r.wording !== "object" ||
      Array.isArray(r.wording) ||
      Object.entries(r.wording).some(
        ([id, text]) =>
          !selected.has(id) ||
          ![
            "description",
            "responsibilities",
            "achievements",
            "highlights",
            "details",
          ].includes(byId.get(id)!.field) ||
          !authoredWordingValid(id, text, authored, byId.get(id)!),
      ))
  )
    return false
  return r.summary.every((s, index) => {
    if (
      !s ||
      !["sourceId,text", "sourceIds,text"].includes(
        Object.keys(s).sort().join(","),
      )
    )
      return false
    const ids = summarySourceIds(s)
    if (
      !ids.length ||
      ids.length > 12 ||
      new Set(ids).size !== ids.length ||
      ids.some(
        (id) =>
          typeof id !== "string" ||
          !selected.has(id) ||
          !substantive.has(byId.get(id)!.field),
      )
    )
      return false
    const sources = ids.map((id) => byId.get(id)!)
    // Combining claims across employers/projects obscures ownership.
    const owners = new Set(
      sources.filter((f) => f.entryId).map((f) => `${f.section}:${f.entryId}`),
    )
    if (owners.size > 1) return false
    // An explicit user-authored rewrite is accepted with shape and reference
    // checks only; it is recorded with user provenance, never machine support.
    if (authored?.summary?.[index] === s.text) return userAuthoredCvText(s.text)
    if (
      sources.some(
        (f) =>
          f.kind === "statement" &&
          (f.intent !== "actual" ||
            f.assertion !== "affirmed" ||
            f.certainty !== "certain"),
      ) &&
      (sources.length !== 1 || s.text !== sources[0].text)
    )
      return false
    return groundedCvText(s.text, sources.map((f) => f.text).join("\n"))
  })
}
export function summarySourceIds(s: CvResult["summary"][number]): string[] {
  return Array.isArray(s.sourceIds)
    ? s.sourceIds
    : typeof s.sourceId === "string"
      ? [s.sourceId]
      : []
}

// Canonical review units repeat the server's ordering: summary items first, then
// rewrites by ascending source id. Support records must match these exactly.
export function cvSupportUnits(result: CvResult) {
  const units: {
    id: string
    kind: "summary" | "wording"
    text: string
    sourceIds: string[]
  }[] = result.summary.map((s, index) => ({
    id: `summary:${index}`,
    kind: "summary" as const,
    text: s.text,
    sourceIds: summarySourceIds(s),
  }))
  const wording = result.wording ?? {}
  for (const id of Object.keys(wording).sort())
    units.push({
      id: `wording:${id}`,
      kind: "wording" as const,
      text: wording[id],
      sourceIds: [id],
    })
  return units
}

// The assessment is untrusted client-side input: require one record per review
// unit, exact repeated text/citations and known statuses. A missing, duplicated,
// reordered or misassociated record fails closed; an unavailable assessment must
// already present every unit as uncertain.
export function validateCvSupport(
  value: unknown,
  result: CvResult,
): value is CvSupport {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false
  const support = value as CvSupport
  if (
    !["assessment,policyVersion,statements"].includes(
      Object.keys(support).sort().join(","),
    ) ||
    support.policyVersion !== CV_SUPPORT_POLICY ||
    !["complete", "unavailable"].includes(support.assessment) ||
    !Array.isArray(support.statements)
  )
    return false
  const units = cvSupportUnits(result)
  if (!units.length || support.statements.length !== units.length) return false
  const byId = new Map<string, CvSupportStatement>()
  for (const statement of support.statements) {
    if (
      !statement ||
      !["id,reason,sourceIds,status,text"].includes(
        Object.keys(statement).sort().join(","),
      ) ||
      typeof statement.id !== "string" ||
      typeof statement.text !== "string" ||
      typeof statement.reason !== "string" ||
      !Array.isArray(statement.sourceIds) ||
      statement.sourceIds.some((id) => typeof id !== "string") ||
      !["supported", "uncertain", "unsupported"].includes(statement.status) ||
      new TextEncoder().encode(statement.reason).length > 500 ||
      byId.has(statement.id)
    )
      return false
    if (
      support.assessment === "unavailable" &&
      statement.status !== "uncertain"
    )
      return false
    byId.set(statement.id, statement)
  }
  return units.every((unit) => {
    const statement = byId.get(unit.id)
    return (
      !!statement &&
      statement.text === unit.text &&
      statement.sourceIds.length === unit.sourceIds.length &&
      statement.sourceIds.every((id, index) => id === unit.sourceIds[index])
    )
  })
}

// Review decisions either remove a whole review unit (discard/revert) or
// replace/keep its wording through an explicit per-statement edit or
// confirmation. Discarding a summary sentence drops it from the accepted
// summary. Reverting a rewrite drops the proposed wording and falls back to the
// verbatim source fact. `editedText`/`confirmedIds` (#126) are explicit
// user-authored corrections; ordinary typing or global acceptance never
// constitutes a confirmation.
export interface CvReviewDecisions {
  discardedSummaryIds: string[]
  revertedWordingIds: string[]
  editedText?: Record<string, string>
  confirmedIds?: string[]
}
export const emptyCvReview: CvReviewDecisions = {
  discardedSummaryIds: [],
  revertedWordingIds: [],
}
export interface CvReviewUnit {
  id: string
  kind: "summary" | "wording"
  text: string
  sourceIds: string[]
  status: CvSupportStatus
  reason: string
}
export type CvReviewOrigin = "generated" | "user_edited" | "user_confirmed"
export type CvReviewDecision = "keep_generated" | "discard" | "revert_to_source" | "edit" | "confirm"
// The original machine judgment is retained only as history. It never becomes
// the active status of user-authored text, and it does not authenticate it.
export interface CvReviewOriginal {
  text: string
  sourceIds: string[]
  status: CvSupportStatus
  reason: string
}
export interface CvReviewRecord {
  id: string
  kind: "summary" | "wording"
  decision: CvReviewDecision
  origin: CvReviewOrigin
  text: string
  sourceIds: string[]
  original?: CvReviewOriginal
}
export function cvReviewUnits(result: CvResult): CvReviewUnit[] | null {
  if (!result.support) return null
  const statusById = new Map(
    result.support.statements.map((statement) => [statement.id, statement]),
  )
  return cvSupportUnits(result).flatMap((unit) => {
    const statement = statusById.get(unit.id)
    return statement
      ? [{ ...unit, status: statement.status, reason: statement.reason }]
      : []
  })
}
export function cvReviewRemoved(
  unit: Pick<CvReviewUnit, "id" | "kind">,
  decisions: CvReviewDecisions,
): boolean {
  return unit.kind === "summary"
    ? decisions.discardedSummaryIds.includes(unit.id)
    : decisions.revertedWordingIds.includes(unit.id.slice("wording:".length))
}
// An explicit edit only counts when it changes the machine wording. Typing the
// exact generated text is not an edit and never promotes an unresolved unit;
// the user must confirm it explicitly instead.
export function cvReviewEdited(
  unit: Pick<CvReviewUnit, "id" | "text">,
  decisions: CvReviewDecisions,
): string | undefined {
  const text = decisions.editedText?.[unit.id]
  return typeof text === "string" && text.trim() && text !== unit.text
    ? text
    : undefined
}
export function cvReviewConfirmed(
  unit: Pick<CvReviewUnit, "id">,
  decisions: CvReviewDecisions,
): boolean {
  return !!decisions.confirmedIds?.includes(unit.id)
}
export function cvReviewResolved(
  result: CvResult,
  decisions: CvReviewDecisions,
): boolean {
  const units = cvReviewUnits(result)
  if (!units) return true
  if (
    !units.some(
      (unit) => unit.kind === "summary" && !cvReviewRemoved(unit, decisions),
    )
  )
    return false
  return units.every(
    (unit) =>
      unit.status === "supported" ||
      cvReviewRemoved(unit, decisions) ||
      cvReviewEdited(unit, decisions) !== undefined ||
      cvReviewConfirmed(unit, decisions),
  )
}

export interface CvResolvedReview {
  result: CvResult
  provenance?: CvReviewRecord[]
  authored?: CvAuthoredText
}
// Review resolution revalidates the pending result instead of trusting rendered
// state. Removed units are dropped, user-authored units keep their provenance
// with the machine judgment as history, and every remaining machine unit must
// already be supported. Global acceptance never promotes unresolved text.
export function resolveCvReview(
  result: CvResult,
  facts: CvFact[],
  decisions: CvReviewDecisions,
): CvResolvedReview {
  const { support, ...core } = result
  if (!validateCvResult(core, facts)) throw new Error("invalid_output")
  if (!support) return { result: core }
  if (!validateCvSupport(support, result)) throw new Error("invalid_output")
  const units = cvReviewUnits(result)!
  const byId = new Map(units.map((unit) => [unit.id, unit]))
  const original = (unit: CvReviewUnit): CvReviewOriginal => ({
    text: unit.text,
    sourceIds: [...unit.sourceIds],
    status: unit.status,
    reason: unit.reason,
  })
  const record = (
    unit: CvReviewUnit,
    decision: CvReviewDecision,
    origin: CvReviewOrigin,
    text: string,
    sourceIds: string[],
    history: boolean,
  ): CvReviewRecord => ({
    id: unit.id,
    kind: unit.kind,
    decision,
    origin,
    text,
    sourceIds,
    ...(history ? { original: original(unit) } : {}),
  })
  const provenance: CvReviewRecord[] = []
  const summary: CvResult["summary"] = []
  const authoredSummary: (string | undefined)[] = []
  result.summary.forEach((item, index) => {
    const unit = byId.get(`summary:${index}`)!
    if (cvReviewRemoved(unit, decisions)) {
      provenance.push(record(unit, "discard", "generated", "", [], true))
      return
    }
    const edited = cvReviewEdited(unit, decisions)
    if (edited !== undefined) {
      summary.push({ sourceIds: [...unit.sourceIds], text: edited })
      authoredSummary.push(edited)
      provenance.push(
        record(unit, "edit", "user_edited", edited, [...unit.sourceIds], true),
      )
      return
    }
    if (cvReviewConfirmed(unit, decisions)) {
      summary.push({ sourceIds: summarySourceIds(item), text: item.text })
      authoredSummary.push(undefined)
      provenance.push(
        record(
          unit,
          "confirm",
          "user_confirmed",
          item.text,
          summarySourceIds(item),
          true,
        ),
      )
      return
    }
    if (unit.status !== "supported") throw new Error("unresolved_output")
    summary.push({ sourceIds: summarySourceIds(item), text: item.text })
    authoredSummary.push(undefined)
    provenance.push(
      record(
        unit,
        "keep_generated",
        "generated",
        item.text,
        summarySourceIds(item),
        false,
      ),
    )
  })
  const wording: Record<string, string> = {}
  const authoredWording: Record<string, string> = {}
  for (const id of Object.keys(core.wording ?? {}).sort()) {
    const unit = byId.get(`wording:${id}`)!
    const text = core.wording![id]
    if (decisions.revertedWordingIds.includes(id)) {
      provenance.push(
        record(unit, "revert_to_source", "generated", "", [], true),
      )
      continue
    }
    const edited = cvReviewEdited(unit, decisions)
    if (edited !== undefined) {
      wording[id] = edited
      authoredWording[id] = edited
      provenance.push(record(unit, "edit", "user_edited", edited, [id], true))
      continue
    }
    if (cvReviewConfirmed(unit, decisions)) {
      wording[id] = text
      provenance.push(
        record(unit, "confirm", "user_confirmed", text, [id], true),
      )
      continue
    }
    if (unit.status !== "supported") throw new Error("unresolved_output")
    wording[id] = text
    provenance.push(
      record(unit, "keep_generated", "generated", text, [id], false),
    )
  }
  const resolved: CvResult = {
    summary,
    selected: core.selected,
    ...(core.wording ? { wording } : {}),
  }
  const authored: CvAuthoredText = {
    summary: authoredSummary,
    wording: authoredWording,
  }
  if (!validateCvResult(resolved, facts, authored))
    throw new Error("invalid_output")
  return { result: resolved, provenance, authored }
}
export function acceptCvResult(
  result: CvResult,
  facts: CvFact[],
  decisions: CvReviewDecisions,
): CvResult {
  return resolveCvReview(result, facts, decisions).result
}

// Saved provenance is untrusted on reload: it must have a known decision,
// matching origin, valid cited references and a preserved original judgment for
// every authored/removed unit. Machine support can never be attached to user
// text, so an incomplete or unknown record cannot default to supported.
export function validateCvReviewRecords(
  value: unknown,
  sources: CvFact[],
): value is CvReviewRecord[] {
  if (!Array.isArray(value) || !value.length) return false
  const available = new Set(sources.map((f) => f.id))
  const seen = new Set<string>()
  const decisions: CvReviewDecision[] = [
    "keep_generated",
    "discard",
    "revert_to_source",
    "edit",
    "confirm",
  ]
  const origins: CvReviewOrigin[] = [
    "generated",
    "user_edited",
    "user_confirmed",
  ]
  const removed: CvReviewDecision[] = ["discard", "revert_to_source"]
  for (const record of value as CvReviewRecord[]) {
    if (!record || typeof record !== "object" || Array.isArray(record))
      return false
    if (
      ![
        "decision,id,kind,origin,sourceIds,text",
        "decision,id,kind,origin,original,sourceIds,text",
      ].includes(Object.keys(record).sort().join(","))
    )
      return false
    if (
      typeof record.id !== "string" ||
      !record.id ||
      seen.has(record.id) ||
      !["summary", "wording"].includes(record.kind)
    )
      return false
    seen.add(record.id)
    if (
      !decisions.includes(record.decision) ||
      !origins.includes(record.origin) ||
      typeof record.text !== "string" ||
      new TextEncoder().encode(record.text).length > 12_288 ||
      !Array.isArray(record.sourceIds) ||
      record.sourceIds.some(
        (id) => typeof id !== "string" || !available.has(id),
      )
    )
      return false
    const hasOriginal =
      record.decision === "edit" ||
      record.decision === "confirm" ||
      removed.includes(record.decision)
    if (hasOriginal !== (record.original !== undefined)) return false
    if (record.kind === "summary") {
      if (!/^summary:\d+$/.test(record.id)) return false
      // Authored/kept records cite their sources; removed records keep none.
      if (!removed.includes(record.decision) && !record.sourceIds.length)
        return false
      if (removed.includes(record.decision) && record.sourceIds.length)
        return false
    } else {
      const id = record.id.slice("wording:".length)
      if (record.id !== `wording:${id}`) return false
      if (removed.includes(record.decision)) {
        if (record.sourceIds.length) return false
      } else if (record.sourceIds.length !== 1 || record.sourceIds[0] !== id)
        return false
    }
    // User provenance is never machine support, and removals never keep text.
    if (record.decision === "keep_generated" && record.origin !== "generated")
      return false
    if (record.decision === "edit" && record.origin !== "user_edited")
      return false
    if (record.decision === "confirm" && record.origin !== "user_confirmed")
      return false
    if (removed.includes(record.decision)) {
      if (record.origin !== "generated" || record.text !== "") return false
    } else if (!record.text.trim()) return false
    if (record.original !== undefined) {
      const history = record.original
      if (
        !history ||
        typeof history !== "object" ||
        Array.isArray(history) ||
        !["reason,sourceIds,status,text"].includes(
          Object.keys(history).sort().join(","),
        ) ||
        typeof history.text !== "string" ||
        new TextEncoder().encode(history.text).length > 12_288 ||
        typeof history.reason !== "string" ||
        new TextEncoder().encode(history.reason).length > 500 ||
        !["supported", "uncertain", "unsupported"].includes(history.status) ||
        !Array.isArray(history.sourceIds) ||
        history.sourceIds.some(
          (id) => typeof id !== "string" || !available.has(id),
        )
      )
        return false
      if (
        (record.decision === "edit" || record.decision === "confirm") &&
        (history.sourceIds.length !== record.sourceIds.length ||
          history.sourceIds.some((id, index) => id !== record.sourceIds[index]))
      )
        return false
    }
  }
  return true
}

export function cvSupportStatus(
  saved: CuratedCv,
  doc: ProfileDocument | null,
): "legacy" | "current" | "stale" {
  if (saved.version === 1) return "legacy"
  if (!doc || doc.id !== saved.sourceProfileId) return "stale"
  for (const source of saved.sources) {
    const live = doc.facts.find((f) => f.id === source.reference?.id)
    if (
      !live ||
      live.revision !== source.reference?.revision ||
      live.support !== source.support ||
      (live.value === true ? "true" : live.value) !== source.text ||
      JSON.stringify(live.context) !== JSON.stringify(source.context ?? [])
    )
      return "stale"
    const activeEvidence = doc.links.filter(
      (l) =>
        l.kind === "supports" &&
        l.state === "active" &&
        l.from.id === live.id &&
        l.from.revision === live.revision,
    )
    if (activeEvidence.length !== (source.evidence?.length ?? 0)) return "stale"
    if (
      source.context?.some(
        (r) =>
          !doc.entities.some((e) => e.id === r.id && e.revision === r.revision),
      )
    )
      return "stale"
    for (const evidence of source.evidence ?? []) {
      if (
        !doc.evidence.some(
          (e) =>
            e.id === evidence.id &&
            e.revision === evidence.revision &&
            e.excerpt === evidence.excerpt,
        ) ||
        !doc.links.some(
          (l) =>
            l.kind === "supports" &&
            l.state === "active" &&
            l.from.id === live.id &&
            l.from.revision === live.revision &&
            l.to.id === evidence.id &&
            l.to.revision === evidence.revision,
        )
      )
        return "stale"
    }
  }
  if (
    saved.context?.some(
      (c) =>
        !doc.facts.some(
          (f) =>
            f.id === c.reference.id &&
            f.revision === c.reference.revision &&
            JSON.stringify(f.value) === JSON.stringify(c.value),
        ),
    )
  )
    return "stale"
  return "current"
}

// ADR 0003 / issue #127: saved-CV wording trust. A version 1 or 2 snapshot
// predates the statement assessment, so its summary and any supporting rewrites
// are historical machine wording: `unverified`. Only a version 3 snapshot with
// complete, self-consistent provenance is `assessed`. Trust is derived from the
// stored metadata, never re-observed against the live Profile, and is a separate
// axis from `cvSupportStatus` freshness. Loading an older snapshot can therefore
// never silently upgrade its wording to machine-supported, and a malformed or
// incomplete v3 document that lost its provenance reads as unverified rather
// than assessed.
export type CvWordingTrust = "unverified" | "assessed"
export function cvWordingTrust(saved: CuratedCv): CvWordingTrust {
  return saved.version === 3 &&
    Array.isArray(saved.sources) &&
    validateCvReviewRecords(saved.provenance, saved.sources)
    ? "assessed"
    : "unverified"
}

export function createCuratedCv(
  repo: ProfessionalRepository,
  facts: CvFact[],
  result: CvResult,
  cvLanguage: Locale,
  density: CvDensity = "balanced",
  document?: ProfileDocument,
  review?: CvReviewDecisions,
): CuratedCv {
  // With explicit review decisions, resolution records user provenance and
  // drops removed units. Without them the legacy acceptance path is unchanged:
  // a v3 result carrying any unresolved statement is still rejected.
  const resolved = review ? resolveCvReview(result, facts, review) : undefined
  const { support, ...stripped } = result
  const core = resolved?.result ?? stripped
  if (!resolved) {
    if (!validateCvResult(core, facts)) throw new Error("invalid_output")
    if (support) {
      if (!validateCvSupport(support, result)) throw new Error("invalid_output")
      if (
        support.statements.some((statement) => statement.status !== "supported")
      )
        throw new Error("unresolved_output")
    }
  }
  const provenance = resolved?.provenance
  if (provenance && !document) throw new Error("invalid_output")
  const selected = core.selected.map((id) => facts.find((f) => f.id === id)!)
  if (
    document &&
    selected.some(
      (f) =>
        !f.reference ||
        f.reference.profileId !== document.id ||
        !document.facts.some(
          (live) =>
            live.id === f.reference!.id &&
            live.revision === f.reference!.revision &&
            (live.value === true ? "true" : live.value) === f.text,
        ),
    )
  )
    throw new Error("invalid_output")
  const content = selected.map((f) => ({
    ...f,
    text: core.wording?.[f.id] ?? f.text,
  }))
  const snapshot = structuredClone(repo)
  const has = (section: string, id: string) =>
    selected.some((f) => f.section === section && f.entryId === id)
  const text = (section: string, id: string, field: string) =>
    content
      .filter(
        (f) => f.section === section && f.entryId === id && f.field === field,
      )
      .map((f) => f.text)
      .join("\n")
  snapshot.skills = text("skills", "", "skills")
  snapshot.competencies = text("skills", "", "competencies")
  snapshot.tools = text("tools", "", "tools")
  const ranked = <T extends { id: string }>(section: string, items: T[]) =>
    items
      .filter((e) => has(section, e.id))
      .sort(
        (a, b) =>
          selected.findIndex(
            (f) => f.section === section && f.entryId === a.id,
          ) -
          selected.findIndex(
            (f) => f.section === section && f.entryId === b.id,
          ),
      )
  snapshot.experience = ranked("experience", snapshot.experience)
    .filter((e) => has("experience", e.id))
    .map((e) => ({
      ...e,
      description: text("experience", e.id, "description"),
      responsibilities: text("experience", e.id, "responsibilities"),
      achievements: text("experience", e.id, "achievements"),
    }))
  snapshot.projects = ranked("projects", snapshot.projects)
    .filter((p) => has("projects", p.id))
    .map((p) => ({
      ...p,
      description: text("projects", p.id, "description"),
      technologies: text("projects", p.id, "technologies"),
      highlights: text("projects", p.id, "highlights"),
    }))
  snapshot.education = ranked("education", snapshot.education)
    .filter((e) => has("education", e.id))
    .map((e) => ({ ...e, details: text("education", e.id, "details") }))
  snapshot.certifications = ranked(
    "certifications",
    snapshot.certifications,
  ).filter((c) => has("certifications", c.id))
  snapshot.languages = ranked("languages", snapshot.languages).filter((l) =>
    has("languages", l.id),
  )
  snapshot.careerGoals = ""
  snapshot.additionalInfo = ""
  snapshot.employmentStatus = ""
  snapshot.currentSalary = ""
  snapshot.desiredSalary = ""
  return {
    version: provenance ? 3 : document ? 2 : 1,
    ...(document
      ? {
          sourceProfileId: document.id,
          summarySources: structuredClone(core.summary),
          // Include original contact and protected entity metadata rendered implicitly.
          context: document.facts
            .filter(
              (f) =>
                (f.owner.id === document.id &&
                  [
                    "fullName",
                    "email",
                    "phone",
                    "location",
                    "professionalLinks",
                  ].includes(f.field)) ||
                (f.owner.id !== document.id &&
                  selected.some(
                    (source) =>
                      source.owner?.id === f.owner.id ||
                      source.context?.some((r) => r.id === f.owner.id),
                  ) &&
                  [
                    "title",
                    "company",
                    "startDate",
                    "endDate",
                    "current",
                    "location",
                    "name",
                    "degree",
                    "institution",
                    "graduationDate",
                    "issuer",
                    "date",
                    "credentialId",
                    "url",
                    "proficiency",
                  ].includes(f.field)),
            )
            .map((f) => ({
              reference: {
                profileId: document.id,
                id: f.id,
                revision: f.revision,
              },
              value: structuredClone(f.value),
            })),
        }
      : {}),
    ...(provenance ? { provenance } : {}),
    repository: snapshot,
    summary: core.summary.map((s) => s.text).join(" "),
    sources: structuredClone(selected),
    cvLanguage,
    density,
  }
}
export function parseCuratedCv(raw: string | null): CuratedCv | null {
  try {
    const value = JSON.parse(raw || "null") as CuratedCv & {
      locale?: Locale
    } | null
    if (value && !value.cvLanguage && value.locale) {
      value.cvLanguage = value.locale
      delete value.locale
    }
    if (
      !value ||
      ![1, 2, 3].includes(value.version) ||
      !["en", "pt-BR"].includes(value.cvLanguage) ||
      typeof value.summary !== "string" ||
      !Array.isArray(value.sources) ||
      !value.repository
    )
      return null
    const r = value.repository as unknown as Record<string, unknown>
    if (
      [
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
      ].some((k) => typeof r[k] !== "string")
    )
      return null
    const fields = {
      experience: [
        "id",
        "company",
        "title",
        "startDate",
        "endDate",
        "location",
        "description",
        "responsibilities",
        "achievements",
      ],
      projects: [
        "id",
        "name",
        "description",
        "technologies",
        "url",
        "highlights",
      ],
      education: [
        "id",
        "degree",
        "institution",
        "location",
        "graduationDate",
        "details",
      ],
      certifications: ["id", "name", "issuer", "date", "credentialId", "url"],
      languages: ["id", "name", "proficiency"],
    }
    for (const [key, names] of Object.entries(fields))
      if (
        !Array.isArray(r[key]) ||
        (r[key] as Record<string, unknown>[]).some(
          (e) =>
            !e ||
            names.some((n) => typeof e[n] !== "string") ||
            (key === "experience" && typeof e.current !== "boolean"),
        )
      )
        return null
    if (
      value.sources.some(
        (f) =>
          !f ||
          ["id", "section", "entryId", "field", "text"].some(
            (k) => typeof f[(k as keyof CvFact)] !== "string",
          ),
      )
    )
      return null
    if (value.version === 2 || value.version === 3) {
      const ref = (r: ProfileRef | undefined) =>
        !!r &&
        r.profileId === value.sourceProfileId &&
        typeof r.id === "string" &&
        !!r.id &&
        Number.isSafeInteger(r.revision) &&
        r.revision > 0
      // A v3 snapshot must carry complete, coherent provenance for every
      // retained statement, and its retained summary records must correspond
      // positionally to the stored summary sources. Invalid metadata can never
      // default to machine-supported content.
      const provenance = value.version === 3 ? value.provenance : undefined
      const retained = Array.isArray(provenance)
        ? provenance.filter(
            (record) =>
              record.kind === "summary" && record.decision !== "discard",
          )
        : []
      if (
        typeof value.sourceProfileId !== "string" ||
        !value.sourceProfileId ||
        !Array.isArray(value.context) ||
        value.context.some(
          (c) => !c || !ref(c.reference) || c.value === undefined,
        ) ||
        value.sources.some(
          (f) =>
            !ref(f.reference) ||
            f.reference!.id !== f.id ||
            !ref(f.owner) ||
            !Array.isArray(f.context) ||
            f.context.some((r) => !ref(r)) ||
            !["legacy_block", "statement"].includes(f.kind ?? "") ||
            !Array.isArray(f.evidence) ||
            f.evidence.some(
              (e) =>
                !e ||
                typeof e.id !== "string" ||
                !Number.isSafeInteger(e.revision) ||
                e.revision < 1 ||
                typeof e.excerpt !== "string" ||
                typeof e.origin !== "string" ||
                e.approval !== "approved",
            ),
        ) ||
        !Array.isArray(value.summarySources) ||
        (value.version === 3 &&
          (!validateCvReviewRecords(provenance, value.sources) ||
            retained.length !== value.summarySources.length ||
            retained.some((record, index) => {
              const ids = summarySourceIds(value.summarySources![index])
              return (
                record.text !== value.summarySources![index].text ||
                ids.length !== record.sourceIds.length ||
                ids.some((id, position) => id !== record.sourceIds[position])
              )
            }))) ||
        !validateCvResult(
          {
            summary: value.summarySources,
            selected: value.sources.map((f) => f.id),
          },
          value.sources,
          value.version === 3
            ? {
                summary: value.summarySources.map((_item, index) =>
                  retained[index]?.origin === "user_edited"
                    ? retained[index].text
                    : undefined,
                ),
              }
            : undefined,
        )
      )
        return null
    }
    value.density = ["compact", "balanced", "detailed"].includes(
      value.density || "",
    )
      ? value.density
      : "balanced"
    return value
  } catch {
    return null
  }
}
export class CvGenerationError extends Error {
  constructor(public code: string) {
    super(code)
  }
}
export async function generateCv(
  facts: CvFact[],
  cvLanguage: Locale,
  key: string,
  signal: AbortSignal,
  density: CvDensity = "balanced",
): Promise<CvResult> {
  let response: Response
  const requestSignal = AbortSignal.any([signal, AbortSignal.timeout(30_000)])
  const groups = new Map<string, string>()
  const outbound = facts.map((f) => {
    const group = JSON.stringify([f.section, f.entryId])
    if (f.entryId && !groups.has(group))
      groups.set(group, `entry${groups.size}`)
    return {
      id: f.id,
      section: f.section,
      entryId: f.entryId ? groups.get(group)! : "",
      field: f.field,
      text: f.text,
      ...(f.reference
        ? {
            reference: f.reference,
            owner: f.owner,
            context: f.context,
            kind: f.kind,
            assertion: f.assertion,
            intent: f.intent,
            certainty: f.certainty,
            support: f.support,
          }
        : {}),
    }
  })
  const body = JSON.stringify({
    facts: outbound,
    cvLanguage,
    density,
    ...(CV_SUPPORT_REQUEST ? { statementSupport: true } : {}),
  })
  if (
    !facts.length ||
    facts.length > 500 ||
    new TextEncoder().encode(body).length > 128 * 1024 ||
    facts.some(
      (f) =>
        new TextEncoder().encode(f.text).length > 12_288 || f.id.length > 100,
    )
  )
    throw new CvGenerationError("input")
  try {
    response = await fetch("/api/cv/generate", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-OpenAI-Api-Key": key },
      body,
      signal: requestSignal,
    })
  } catch (error) {
    throw new CvGenerationError(
      error instanceof DOMException && error.name === "TimeoutError"
        ? "timeout"
        : signal.aborted
          ? "cancelled"
          : "outage",
    )
  }
  let payload: unknown
  try {
    payload = await response.json()
  } catch {
    throw new CvGenerationError(
      requestSignal.aborted
        ? signal.aborted
          ? "cancelled"
          : "timeout"
        : "invalid_output",
    )
  }
  if (!response.ok) {
    const code = (payload as { error?: string } | null)?.error
    throw new CvGenerationError(
      code &&
        ["input", "key", "rate_limit", "timeout", "invalid_output"].includes(
          code,
        )
        ? code
        : "outage",
    )
  }
  // v3 is identified by schemaVersion, not by the presence of support, so an
  // unknown version never falls back to accepting unassessed legacy wording.
  if (
    (payload as { schemaVersion?: unknown } | null)?.schemaVersion !== undefined
  ) {
    const value = payload as { schemaVersion: unknown } & CvResult
    const core: CvResult = {
      summary: value.summary,
      selected: value.selected,
      ...(value.wording !== undefined ? { wording: value.wording } : {}),
    }
    if (
      value.schemaVersion !== 3 ||
      !validateCvResult(core, facts) ||
      !validateCvSupport(value.support, core)
    )
      throw new CvGenerationError("invalid_output")
    return { ...core, support: value.support }
  }
  if (CV_SUPPORT_REQUEST) throw new CvGenerationError("invalid_output")
  if (!validateCvResult(payload, facts))
    throw new CvGenerationError("invalid_output")
  return payload
}

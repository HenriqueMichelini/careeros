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
export interface CvResult {
  summary: {
    sourceId?: string
    sourceIds?: string[]
    text: string
  }[]
  selected: string[]
  wording?: Record<string, string>
}
export interface CuratedCv {
  version: 1 | 2
  sourceProfileId?: string
  summarySources?: CvResult["summary"]
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
export function validateCvResult(
  value: unknown,
  facts: CvFact[],
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
          !groundedCvText(text, byId.get(id)!.text) ||
          (byId.get(id)!.kind === "statement" &&
            (byId.get(id)!.intent !== "actual" ||
              byId.get(id)!.assertion !== "affirmed" ||
              byId.get(id)!.certainty !== "certain") &&
            text !== byId.get(id)!.text),
      ))
  )
    return false
  return r.summary.every((s) => {
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

export function createCuratedCv(
  repo: ProfessionalRepository,
  facts: CvFact[],
  result: CvResult,
  cvLanguage: Locale,
  density: CvDensity = "balanced",
  document?: ProfileDocument,
): CuratedCv {
  if (!validateCvResult(result, facts)) throw new Error("invalid_output")
  const selected = result.selected.map((id) => facts.find((f) => f.id === id)!)
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
    text: result.wording?.[f.id] ?? f.text,
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
    version: document ? 2 : 1,
    ...(document
      ? {
          sourceProfileId: document.id,
          summarySources: structuredClone(result.summary),
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
    repository: snapshot,
    summary: result.summary.map((s) => s.text).join(" "),
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
      ![1, 2].includes(value.version) ||
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
    if (value.version === 2) {
      const ref = (r: ProfileRef | undefined) =>
        !!r &&
        r.profileId === value.sourceProfileId &&
        typeof r.id === "string" &&
        !!r.id &&
        Number.isSafeInteger(r.revision) &&
        r.revision > 0
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
        !validateCvResult(
          {
            summary: value.summarySources,
            selected: value.sources.map((f) => f.id),
          },
          value.sources,
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
  const body = JSON.stringify({ facts: outbound, cvLanguage, density })
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
  if (!validateCvResult(payload, facts))
    throw new CvGenerationError("invalid_output")
  return payload
}

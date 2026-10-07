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
}
export interface CvResult {
  summary: {
    sourceId: string
    text: string
  }[]
  selected: string[]
  wording?: Record<string, string>
}
export interface CuratedCv {
  version: 1
  density?: CvDensity
  choices?: CvChoices
  repository: ProfessionalRepository
  summary: string
  sources: CvFact[]
  locale: Locale
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
    r.selected.some((id) => typeof id !== "string" || !byId.has(id))
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
          !groundedCvText(text, byId.get(id)!.text),
      ))
  )
    return false
  return r.summary.every(
    (s) =>
      s &&
      Object.keys(s).sort().join(",") === "sourceId,text" &&
      typeof s.text === "string" &&
      s.text.trim() !== "" &&
      selected.has(s.sourceId) &&
      substantive.has(byId.get(s.sourceId)!.field) &&
      groundedCvText(s.text, byId.get(s.sourceId)!.text),
  )
}
export function createCuratedCv(
  repo: ProfessionalRepository,
  facts: CvFact[],
  result: CvResult,
  locale: Locale,
  density: CvDensity = "balanced",
): CuratedCv {
  if (!validateCvResult(result, facts)) throw new Error("invalid_output")
  const selected = result.selected.map((id) => facts.find((f) => f.id === id)!)
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
    version: 1,
    repository: snapshot,
    summary: result.summary.map((s) => s.text).join(" "),
    sources: selected,
    locale,
    density,
  }
}
export function parseCuratedCv(raw: string | null): CuratedCv | null {
  try {
    const value = JSON.parse(raw || "null") as CuratedCv | null
    if (
      !value ||
      value.version !== 1 ||
      !["en", "pt-BR"].includes(value.locale) ||
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
  locale: Locale,
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
    return { ...f, entryId: f.entryId ? groups.get(group)! : "" }
  })
  try {
    response = await fetch("/api/cv/generate", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-OpenAI-Api-Key": key },
      body: JSON.stringify({ facts: outbound, locale, density }),
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

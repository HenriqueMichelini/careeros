import type {
  ProfileDocument,
  ProfileRef,
  ProfileFact,
} from "./profileDocument"
import type { ProfessionalRepository } from "./types"
import type { Locale } from "./i18n"
import type { QualificationFact } from "./qualificationEvidence"

export const resumeSections = [
  "summary",
  "skills",
  "experience",
  "education",
  "certifications",
  "languages",
] as const
export interface ReviewedResumeClaim {
  section: typeof resumeSections[number]
  kind: "paragraph" | "subheading" | "bullet"
  text: string
  sources: ProfileRef[]
  state: "supported" | "uncertain" | "unsupported"
  concerns: string[]
  removed?: boolean
  userSupport?: {
    original: string
    evidence: string
    previousSources: ProfileRef[]
  }
}
export interface ResumeReview {
  id: string
  version: "resume-review-v1"
  claims: ReviewedResumeClaim[]
  facts: QualificationFact[]
  check: "complete" | "unavailable" | "not_needed"
  sourceSnapshot: ProfileDocument
  identityProfile: ProfessionalRepository
  cvLanguage: Locale
}
export interface AcceptedResume {
  claims: ReviewedResumeClaim[]
  markdown: string
  sourceSnapshot: ProfileDocument
  identityProfile: ProfessionalRepository
  cvLanguage: Locale
}
const titles: Record<Locale, string[]> = {
  en: [
    "Professional Summary",
    "Technical Skills",
    "Professional Experience",
    "Education",
    "Certifications",
    "Languages",
  ],
  "pt-BR": [
    "Resumo Profissional",
    "Competências Técnicas",
    "Experiência Profissional",
    "Educação",
    "Certificações",
    "Idiomas",
  ],
}
export function renderReviewedResume(
  claims: ReviewedResumeClaim[],
  language: Locale,
): string {
  return resumeSections
    .flatMap((section, i) => {
      const lines = claims
        .filter((c) => !c.removed && c.section === section)
        .map(
          (c) =>
            (c.kind === "bullet"
              ? "- "
              : c.kind === "subheading"
                ? "### "
                : "") + c.text,
        )
      return lines.length
        ? ["## " + titles[language][i] + "\n" + lines.join("\n\n")]
        : []
    })
    .join("\n\n")
}
export function resumeReviewCurrent(
  review: Pick<ResumeReview, "sourceSnapshot">,
  doc: ProfileDocument | null,
): boolean {
  // Identity/revision plus exact snapshot protects against stale tabs, changed
  // relationships/evidence and manual edits. It makes no truth claim.
  return !!doc && JSON.stringify(review.sourceSnapshot) === JSON.stringify(doc)
}
export function acceptResume(
  review: ResumeReview,
  doc: ProfileDocument | null,
): AcceptedResume {
  const active = review.claims.filter((c) => !c.removed)
  if (!resumeReviewCurrent(review, doc)) throw new Error("stale")
  if (!active.length || active.some((c) => c.state !== "supported"))
    throw new Error("unresolved")
  return structuredClone({
    claims: active,
    markdown: renderReviewedResume(active, review.cvLanguage),
    sourceSnapshot: review.sourceSnapshot,
    identityProfile: review.identityProfile,
    cvLanguage: review.cvLanguage,
  })
}
export function removeResumeClaim(
  review: ResumeReview,
  index: number,
): ResumeReview {
  if (!review.claims[index]) throw new Error("input")
  const next = structuredClone(review)
  next.claims[index].removed = true
  return next
}
function isResumeStatementText(text: unknown): text is string {
  return (
    typeof text === "string" && !!text.trim() && text.length <= 2000 &&
    !/[\r\n*`]/.test(text) &&
    !/^(?:#{1,6}\s|[-•]\s|\d+[.)]\s)/.test(text.trim())
  )
}
export function correctResume(
  review: ResumeReview,
  index: number,
  evidence: string,
): ResumeReview {
  const original = review.claims[index]
  const text = evidence.trim()
  if (!original || !isResumeStatementText(text))
    throw new Error("input")
  const next = structuredClone(review)
  // The person explicitly supplies and owns the complete replacement assertion.
  // Old references remain audit history only; they never support the new claim.
  next.claims[index] = {
    ...original,
    text,
    state: "supported",
    concerns: [],
    sources: [],
    removed: false,
    userSupport: {
      original: original.userSupport?.original ?? original.text,
      evidence: text,
      previousSources:
        original.userSupport?.previousSources ??
        structuredClone(original.sources),
    },
  }
  return next
}
export function parseResumeReview(
  value: unknown,
  doc: ProfileDocument,
  profile: ProfessionalRepository,
  language: Locale,
  markdown: string,
  confirmations: {
 requirement: string
 userContext: string
}[] = [],
): ResumeReview | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null
  const r = value as ResumeReview
  if (
    r.version !== "resume-review-v1" ||
    !["complete", "unavailable", "not_needed"].includes(r.check) ||
    !Array.isArray(r.claims) ||
    !r.claims.length ||
    r.claims.length > 80 ||
    !Array.isArray(r.facts) ||
    r.facts.length > 500
  )
    return null
  const ids = new Set<string>()
  for (const fact of r.facts) {
    if (!fact || ids.has(fact.id)) return null
    ids.add(fact.id)
    const live = [
      ...doc.facts,
      ...applicationResumeFacts(doc, confirmations),
    ].find((f) => f.id === fact.id)
    if (!live) return null
    for (const key of Object.keys(live) as Array<keyof ProfileFact>) {
      if (
        JSON.stringify(live[key]) !==
        JSON.stringify((fact as unknown as Record<string, unknown>)[key])
      )
        return null
    }
    if (
      !Array.isArray(fact.evidence) ||
      fact.evidence.some(
        (e) =>
          !doc.evidence.some(
            (o) =>
              o.id === e.id &&
              o.revision === e.revision &&
              o.excerpt === e.excerpt &&
              o.approval === "approved",
          ) ||
          !doc.links.some(
            (l) =>
              l.kind === "supports" &&
              l.state === "active" &&
              l.from.id === fact.id &&
              l.from.revision === fact.revision &&
              l.to.id === e.id &&
              l.to.revision === e.revision,
          ),
      )
    )
      return null
  }
  for (const c of r.claims) {
    if (
      !c ||
      !resumeSections.includes(c.section) ||
      !["paragraph", "subheading", "bullet"].includes(c.kind) ||
      !isResumeStatementText(c.text) ||
      !["supported", "uncertain", "unsupported"].includes(c.state) ||
      !Array.isArray(c.concerns) ||
      c.concerns.some((s) => typeof s !== "string" || s.length > 1000) ||
      !Array.isArray(c.sources) ||
      c.sources.length > 16 ||
      c.removed !== undefined ||
      c.userSupport !== undefined
    )
      return null
    if (
      c.sources.some(
        (ref) =>
          !ref ||
          typeof ref.profileId !== "string" ||
          !ref.profileId ||
          typeof ref.id !== "string" ||
          !ref.id ||
          !Number.isSafeInteger(ref.revision) ||
          ref.revision < 1,
      )
    )
      return null
    if (
      c.state !== "unsupported" &&
      (!c.sources.length ||
        c.sources.some(
          (ref) =>
            !ref ||
            ref.profileId !== doc.id ||
            !r.facts.some(
              (f) => f.id === ref.id && f.revision === ref.revision,
            ),
        ))
    )
      return null
    if (r.check !== "complete" && c.state === "supported") return null
    if (c.state === "supported" && c.concerns.length) return null
  }
  if (renderReviewedResume(r.claims, language) !== markdown) return null
  return {
    id: crypto.randomUUID(),
    version: r.version,
    claims: structuredClone(r.claims),
    facts: structuredClone(r.facts),
    check: r.check,
    sourceSnapshot: structuredClone(doc),
    identityProfile: structuredClone(profile),
    cvLanguage: language,
  }
}

// Request-local user evidence for this Application Draft, never a Profile write.
export function applicationResumeFacts(
  doc: ProfileDocument,
  confirmations: {
 requirement: string
 userContext: string
}[],
): ProfileFact[] {
  return confirmations.map((q, i) => {
    const text = q.requirement + (q.userContext ? " — " + q.userContext : "")
    return {
      id: `application-confirmation-${i}`,
      revision: 1,
      owner: { profileId: doc.id, id: doc.id, revision: doc.revision },
      context: [],
      field: "skills",
      order: i,
      kind: "statement",
      value: text,
      assertion: "affirmed",
      intent: "actual",
      certainty: "certain",
      temporal: { wording: "", precision: "unknown" },
      normalization: {
        observed: text,
        canonical: null,
        policy: "exact-alias-v1",
      },
      origin: { kind: "manual_edit", original: "user" },
      approval: "approved",
      support: "unsupported",
    }
  })
}

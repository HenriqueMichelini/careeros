import { completeCoverLetter } from "./cover-letter"
import type { ProfileDocument, ProfileRef } from "./profileDocument"
import type { ProfessionalRepository } from "./types"
import type { Locale } from "./i18n"
import type { QualificationFact } from "./qualificationEvidence"

export const artifactFields = [
  "jobTitle",
  "company",
  "jobSummary",
  "greeting",
  "body",
  "applicationAnswers",
] as const
export type ArtifactField = typeof artifactFields[number]
export interface JobSupport {
  start: number
  end: number
  quote: string
}
export interface ArtifactClaim {
  field: ArtifactField
  text: string
  sources: ProfileRef[]
  jobSources: JobSupport[]
  state: "supported" | "uncertain" | "unsupported"
  concerns: string[]
  nonfactual: boolean
  removed?: boolean
  userSupport?: {
    original: string
    evidence: string
    previousSources: ProfileRef[]
    previousJobSources: JobSupport[]
  }
}
export interface ArtifactReview {
  id: string
  version: "artifact-review-v1"
  check: "complete" | "unavailable"
  claims: ArtifactClaim[]
  facts: QualificationFact[]
  jobPosting: string
  closing: string
  sourceSnapshot: ProfileDocument
  identityProfile: ProfessionalRepository
  cvLanguage: Locale
}
export interface AcceptedArtifacts {
  review: ArtifactReview
  sourceSnapshot: ProfileDocument
  jobTitle: string | null
  company: string | null
  jobSummary: string
  coverLetter: string
  applicationAnswers: string
}
export function artifactsCurrent(
  review: { sourceSnapshot: ProfileDocument },
  doc: ProfileDocument | null,
) {
  return !!doc && JSON.stringify(review.sourceSnapshot) === JSON.stringify(doc)
}
function fieldText(claims: ArtifactClaim[], field: ArtifactField) {
  return claims
    .filter((c) => c.field === field && !c.removed)
    .map((c) => c.text)
    .join("\n\n")
}
export function acceptArtifacts(
  review: ArtifactReview,
  doc: ProfileDocument | null,
): AcceptedArtifacts {
  if (!artifactsCurrent(review, doc)) throw Error("stale")
  if (review.claims.some((c) => !c.removed && c.state !== "supported"))
    throw Error("unresolved")
  const text = (field: ArtifactField) => fieldText(review.claims, field)
  if (!text("jobSummary") || !text("applicationAnswers"))
    throw Error("required")
  const coverLetter = completeCoverLetter(
    { greeting: text("greeting"), body: text("body"), closing: review.closing },
    review.identityProfile.fullName ?? "",
  )
  if (coverLetter === null) throw Error("cover_letter_bounds")
  return structuredClone({
    review,
    sourceSnapshot: review.sourceSnapshot,
    jobTitle: text("jobTitle") || null,
    company: text("company") || null,
    jobSummary: text("jobSummary"),
    coverLetter,
    applicationAnswers: text("applicationAnswers"),
  })
}
export function correctArtifact(
  review: ArtifactReview,
  index: number,
  evidence: string,
): ArtifactReview {
  const original = review.claims[index]
  const text = evidence.trim()
  if (
    !original ||
    !text ||
    text.length > 8000 ||
    (original.field === "greeting" && /[\r\n]/.test(text))
  )
    throw Error("input")
  const next = structuredClone(review)
  next.claims[index] = {
    ...original,
    text,
    sources: [],
    jobSources: [],
    state: "supported",
    concerns: [],
    removed: false,
    nonfactual: false,
    userSupport: {
      original: original.userSupport?.original ?? original.text,
      evidence: text,
      previousSources:
        original.userSupport?.previousSources ?? original.sources,
      previousJobSources:
        original.userSupport?.previousJobSources ?? original.jobSources,
    },
  }
  // Validate application-owned signature/bounds immediately for cover edits.
  if (
    ["body", "greeting"].includes(original.field) &&
    completeCoverLetter(
      {
        greeting: fieldText(next.claims, "greeting"),
        body: fieldText(next.claims, "body"),
        closing: next.closing,
      },
      next.identityProfile.fullName ?? "",
    ) === null
  )
    throw Error("input")
  return next
}
export function removeArtifactClaim(
  review: ArtifactReview,
  index: number,
): ArtifactReview {
  const claim = review.claims[index]
  if (!claim) throw Error("input")
  // Required answers must be corrected; dropping one cannot hide an omission.
  if (claim.field === "applicationAnswers") throw Error("required")
  const next = structuredClone(review)
  next.claims[index].removed = true
  return next
}
export function parseArtifactReview(
  value: unknown,
  doc: ProfileDocument,
  profile: ProfessionalRepository,
  language: Locale,
  jobPosting: string,
  output: {
    jobTitle?: string | null
    company?: string | null
    jobSummary?: string
    coverLetter?: unknown
    applicationAnswers?: string
  },
  facts: QualificationFact[] = [],
): ArtifactReview | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null
  const r = value as ArtifactReview
  if (
    r.version !== "artifact-review-v1" ||
    !["complete", "unavailable"].includes(r.check) ||
    r.jobPosting !== jobPosting ||
    !Array.isArray(r.claims) ||
    !r.claims.length ||
    r.claims.length > 100 ||
    !Array.isArray(r.facts)
  )
    return null
  // Facts are independently validated by the existing resume parser, including
  // approved excerpts and request-local confirmation references.
  if (JSON.stringify(r.facts) !== JSON.stringify(facts)) return null
  if (!output.coverLetter || typeof output.coverLetter !== "object") return null
  const cover = output.coverLetter as {
    greeting: string
    body: string
    closing: string
  }
  if (r.closing !== cover.closing) return null
  const expected = {
    jobTitle: output.jobTitle ?? "",
    company: output.company ?? "",
    jobSummary: output.jobSummary,
    greeting: cover.greeting,
    body: cover.body,
    applicationAnswers: output.applicationAnswers,
  }
  const bytes = new TextEncoder().encode(jobPosting)
  const decoder = new TextDecoder("utf-8", { fatal: true })
  for (const c of r.claims) {
    if (
      !c ||
      !artifactFields.includes(c.field) ||
      typeof c.text !== "string" ||
      !c.text.trim() ||
      c.text.length > 20000 ||
      !["supported", "uncertain", "unsupported"].includes(c.state) ||
      typeof c.nonfactual !== "boolean" ||
      !Array.isArray(c.concerns) ||
      c.concerns.some((s) => typeof s !== "string" || s.length > 1000) ||
      !Array.isArray(c.sources) ||
      c.sources.length > 16 ||
      !Array.isArray(c.jobSources) ||
      c.jobSources.length > 16 ||
      c.userSupport !== undefined ||
      c.removed !== undefined
    )
      return null
    if (
      c.state === "supported" &&
      (r.check !== "complete" ||
        c.concerns.length ||
        (!c.nonfactual && !c.sources.length && !c.jobSources.length))
    )
      return null
    if (
      c.sources.some(
        (ref) =>
          !ref ||
          ref.profileId !== doc.id ||
          !r.facts.some((f) => f.id === ref.id && f.revision === ref.revision),
      )
    )
      return null
    for (const s of c.jobSources) {
      if (
        !s ||
        !Number.isSafeInteger(s.start) ||
        !Number.isSafeInteger(s.end) ||
        s.start < 0 ||
        s.end <= s.start ||
        s.end > bytes.length ||
        typeof s.quote !== "string"
      )
        return null
      try {
        if (
          decoder.decode(bytes.slice(0, s.start)) +
            decoder.decode(bytes.slice(s.start, s.end)) !==
            decoder.decode(bytes.slice(0, s.end)) ||
          decoder.decode(bytes.slice(s.start, s.end)) !== s.quote
        )
          return null
      } catch {
        return null
      }
    }
  }
  for (const field of artifactFields)
    if (fieldText(r.claims, field) !== expected[field]) return null
  return {
    ...structuredClone(r),
    id: crypto.randomUUID(),
    sourceSnapshot: structuredClone(doc),
    identityProfile: structuredClone(profile),
    cvLanguage: language,
  }
}

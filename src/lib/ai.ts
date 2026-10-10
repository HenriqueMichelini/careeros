import { parseArtifactReview } from "./artifactReview"
import { parseResumeReview } from "./resumeReview"
import {
  qualificationProjection,
  parseRequirementMatches,
  type RequirementMatch,
} from "./qualificationEvidence"
import type { ProfileDocument } from "./profileDocument"
import { parseJobContext, type JobContext } from "./jobContext"
import { parseFieldDecision, type FieldDecision } from "./fieldDecision"
import {
  ProfessionalRepository,
  GeneratedMaterials,
  ProfileGap,
  ConfirmedQualification,
} from "./types"
import type { Locale } from "./i18n"
import { completeCoverLetter } from "./cover-letter"
import { careerProfile, cvQualifications, validQualifications } from "./profile"

export class JobPostingValidationError extends Error {
  constructor(public readonly decision: FieldDecision) {
    super(decision.outcome.kind)
    this.name = "JobPostingValidationError"
  }
}

function checkJobDecision(payload: unknown) {
  if (!payload || typeof payload !== "object" || !("decision" in payload))
    return
  const decision = parseFieldDecision(
    (payload as { decision: unknown }).decision,
    "job_posting",
  )
  if (!decision || decision.outcome.kind === "accept")
    throw new ApplicationDraftError("invalid_output")
  throw new JobPostingValidationError(decision)
}

export class QualificationGapsError extends Error {
  constructor(public readonly code: string) {
    super(code)
    this.name = "QualificationGapsError"
  }
}

export class ApplicationDraftError extends Error {
  constructor(public readonly code: string) {
    super(code)
    this.name = "ApplicationDraftError"
  }
}

export async function generateMaterials(
  repo: ProfessionalRepository,
  jobPosting: string,
  apiKey: string,
  confirmedQualifications: ConfirmedQualification[] = [],
  cvLanguage: Locale = "en",
  typesafeKey: string = "",
  jobContext?: JobContext,
  qualificationAnswers: {
    requirement: string
    userContext: string
  }[] = [],
  profileDocument?: ProfileDocument,
  selectedFactIds: string[] = [],
): Promise<GeneratedMaterials> {
  if (!validQualifications(repo)) throw new ApplicationDraftError("input")
  let response: Response
  try {
    response = await fetch("/api/application-draft", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-OpenAI-Api-Key": apiKey,
        "X-TypeSafe-Api-Key": typesafeKey,
      },
      body: JSON.stringify({
        repository: careerProfile(repo),
        qualifications: cvQualifications(repo),
        jobPosting,
        confirmedQualifications,
        cvLanguage,
        ...(profileDocument
          ? {
              profileEvidence: qualificationProjection(profileDocument),
              reviewResume: true,
              reviewArtifacts: true,
              selectedFactIds: [...new Set(selectedFactIds)],
            }
          : {}),
        ...(jobContext ? { jobContext } : {}),
        ...(qualificationAnswers.length ? { qualificationAnswers } : {}),
      }),
      signal: AbortSignal.timeout(30_000),
    })
  } catch (error) {
    throw new ApplicationDraftError(
      error instanceof DOMException && error.name === "TimeoutError"
        ? "timeout"
        : "outage",
    )
  }
  const payload = (await response.json().catch(() => null)) as {
    error?: unknown
    coverLetter?: unknown
  } & Partial<Omit<GeneratedMaterials, "coverLetter">> | null
  checkJobDecision(payload)
  const known = new Set([
    "input",
    "key",
    "rate_limit",
    "outage",
    "timeout",
    "invalid_output",
    "capacity",
    "job_context_stale",
  ])
  if (!response.ok)
    throw new ApplicationDraftError(
      typeof payload?.error === "string" && known.has(payload.error)
        ? payload.error
        : "outage",
    )
  if (
    !payload ||
    ["jobSummary", "resume", "applicationAnswers"].some(
      (key) =>
        typeof payload[(key as keyof GeneratedMaterials)] !== "string" ||
        !(payload[(key as keyof GeneratedMaterials)] as string).trim(),
    )
  )
    throw new ApplicationDraftError("invalid_output")
  if (
    ["jobTitle", "company"].some((key) => {
      const value = payload[(key as "jobTitle" | "company")]
      return value !== null && (typeof value !== "string" || !value.trim())
    })
  )
    throw new ApplicationDraftError("invalid_output")
  if (payload.contextSelection !== undefined) {
    const c = payload.contextSelection
    if (
      !c ||
      c.version !== "application-context-v1" ||
      !Array.isArray(c.sources) ||
      c.sources.some((s) => typeof s !== "string" || !s.trim()) ||
      ![c.budgetExcluded, c.relevanceExcluded, c.bytes].every(
        (n) => Number.isSafeInteger(n) && n >= 0,
      ) ||
      c.bytes > 24 * 1024 ||
      typeof c.complete !== "boolean" ||
      c.complete !== (c.budgetExcluded === 0)
    )
      throw new ApplicationDraftError("invalid_output")
  }
  const resumeReview = profileDocument
    ? parseResumeReview(
        payload.resumeReview,
        profileDocument,
        repo,
        cvLanguage,
        payload.resume!,
        confirmedQualifications,
        qualificationAnswers,
      )
    : undefined
  if (profileDocument && !resumeReview)
    throw new ApplicationDraftError("invalid_output")
  const artifactReview =
    profileDocument && resumeReview
      ? parseArtifactReview(
          payload.artifactReview,
          profileDocument,
          repo,
          cvLanguage,
          jobPosting,
          payload,
          resumeReview.facts,
        )
      : undefined
  if (profileDocument && !artifactReview)
    throw new ApplicationDraftError("invalid_output")
  const fullName = repo.fullName?.trim() || ""
  const coverLetter = completeCoverLetter(payload.coverLetter, fullName)
  if (coverLetter === null) throw new ApplicationDraftError("invalid_output")
  return {
    ...payload,
    resumeReview: resumeReview ?? undefined,
    artifactReview: artifactReview ?? undefined,
    cvLanguage,
    coverLetter,
    coverLetterHasSignature: !!fullName,
  } as GeneratedMaterials
}

interface QualificationAnalysis {
  matches?: RequirementMatch[]
  gaps: ProfileGap[]
  jobContext: JobContext
}
export function findProfileGaps(
  repo: ProfessionalRepository,
  jobPosting: string,
  apiKey: string,
  typesafeKey?: string,
): Promise<ProfileGap[]>
export function findProfileGaps(
  repo: ProfessionalRepository,
  jobPosting: string,
  apiKey: string,
  typesafeKey: string,
  understandJob: true,
  profileDocument?: ProfileDocument,
): Promise<QualificationAnalysis>
export async function findProfileGaps(
  repo: ProfessionalRepository,
  jobPosting: string,
  apiKey: string,
  typesafeKey: string = "",
  understandJob = false,
  profileDocument?: ProfileDocument,
): Promise<ProfileGap[] | QualificationAnalysis> {
  if (!validQualifications(repo)) throw new QualificationGapsError("input")
  let response: Response
  try {
    response = await fetch("/api/qualification-gaps", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-OpenAI-Api-Key": apiKey,
        "X-TypeSafe-Api-Key": typesafeKey,
      },
      body: JSON.stringify({
        repository: careerProfile(repo),
        qualifications: cvQualifications(repo),
        jobPosting,
        ...(understandJob ? { understandJob: true } : {}),
        ...(profileDocument
          ? { profileEvidence: qualificationProjection(profileDocument) }
          : {}),
      }),
      signal: AbortSignal.timeout(profileDocument ? 55_000 : 30_000),
    })
  } catch (error) {
    throw new QualificationGapsError(
      error instanceof DOMException && error.name === "TimeoutError"
        ? "timeout"
        : "outage",
    )
  }
  const payload = (await response.json().catch(() => null)) as {
    error?: unknown
    gaps?: unknown
    jobContext?: unknown
    matches?: unknown
  } | null
  checkJobDecision(payload)
  if (!response.ok) {
    const known = new Set([
      "input",
      "key",
      "rate_limit",
      "outage",
      "timeout",
      "invalid_output",
      "capacity",
      "job_context_stale",
    ])
    throw new QualificationGapsError(
      typeof payload?.error === "string" && known.has(payload.error)
        ? payload.error
        : "outage",
    )
  }
  if (
    !payload ||
    !Array.isArray(payload.gaps) ||
    payload.gaps.length > (profileDocument ? 100 : 5) ||
    payload.gaps.some((gap) => {
      if (!gap || typeof gap !== "object") return true
      const value = gap as Record<string, unknown>
      return (
        (value.kind !== "skill" && value.kind !== "experience") ||
        typeof value.requirement !== "string" ||
        !value.requirement.trim() ||
        typeof value.details !== "string" ||
        !value.details.trim()
      )
    })
  )
    throw new QualificationGapsError("invalid_output")
  if (understandJob) {
    const context = parseJobContext(payload.jobContext, jobPosting)
    if (!context) throw new QualificationGapsError("invalid_output")
    const matches = profileDocument
      ? parseRequirementMatches(
          payload.matches ??
            (context.job.qualifications.length === 0 ? [] : null),
          qualificationProjection(profileDocument),
          context.job.qualifications,
        )
      : undefined
    if (profileDocument && !matches)
      throw new QualificationGapsError("invalid_output")
    return {
      gaps: payload.gaps as ProfileGap[],
      jobContext: context,
      matches: matches ?? undefined,
    }
  }
  return payload.gaps as ProfileGap[]
}

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
        ...(jobContext ? { jobContext } : {}),
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
  const fullName = repo.fullName?.trim() || ""
  const coverLetter = completeCoverLetter(payload.coverLetter, fullName)
  if (coverLetter === null) throw new ApplicationDraftError("invalid_output")
  return {
    ...payload,
    cvLanguage,
    coverLetter,
    coverLetterHasSignature: !!fullName,
  } as GeneratedMaterials
}

interface QualificationAnalysis {
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
): Promise<QualificationAnalysis>
export async function findProfileGaps(
  repo: ProfessionalRepository,
  jobPosting: string,
  apiKey: string,
  typesafeKey: string = "",
  understandJob = false,
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
      }),
      signal: AbortSignal.timeout(30_000),
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
    payload.gaps.length > 5 ||
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
    return { gaps: payload.gaps as ProfileGap[], jobContext: context }
  }
  return payload.gaps as ProfileGap[]
}

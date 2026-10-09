import { parseFieldDecision, type FieldDecision } from "./fieldDecision"
import {
  ProfessionalRepository,
  GeneratedMaterials,
  ProfileGap,
  ConfirmedQualification,
} from './types'
import type { Locale } from "./i18n"
import { completeCoverLetter } from './cover-letter'
import { careerProfile, cvQualifications, validQualifications, profileReviewFields } from './profile'

export class JobPostingValidationError extends Error {
  constructor(public readonly decision: FieldDecision) {
    super(decision.outcome.kind)
    this.name = "JobPostingValidationError"
  }
}

function checkJobDecision(payload: unknown) {
  if (!payload || typeof payload !== "object" || !("decision" in payload)) return
  const decision = parseFieldDecision((payload as { decision: unknown }).decision, "job_posting")
  if (!decision || decision.outcome.kind === "accept") throw new ApplicationDraftError("invalid_output")
  throw new JobPostingValidationError(decision)
}

export class ProfileReviewError extends Error {
  constructor(public readonly code: string) {
    super(code)
    this.name = 'ProfileReviewError'
  }
}

export class QualificationGapsError extends Error {
  constructor(public readonly code: string) {
    super(code)
    this.name = 'QualificationGapsError'
  }
}

export class ApplicationDraftError extends Error {
  constructor(public readonly code: string) {
    super(code)
    this.name = 'ApplicationDraftError'
  }
}

export async function reviewRepository(
  repo: ProfessionalRepository,
  changedSection: string,
  apiKey: string,
  signal?: AbortSignal
): Promise<{ updatedRepo: ProfessionalRepository; summary: string }> {
  const fields = profileReviewFields(changedSection)
  if (!fields.length) throw new ProfileReviewError("input")
  if (!validQualifications(repo)) throw new ProfileReviewError('input')
  const response = await fetch('/api/profile/review', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-OpenAI-Api-Key': apiKey,
    },
    body: JSON.stringify({ repository: careerProfile(repo), changedSection }),
    signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(30_000)]) : AbortSignal.timeout(30_000),
  }).catch((error: unknown) => {
    if (error instanceof DOMException && error.name === 'TimeoutError') {
      throw new ProfileReviewError('timeout')
    }
    throw new ProfileReviewError('outage')
  })

  const payload = await response.json().catch(() => null) as
    | { error?: unknown; updatedRepository?: unknown; summary?: unknown }
    | null
  if (!response.ok) {
    const known = new Set(['input', 'key', 'rate_limit', 'outage', 'timeout', 'invalid_output'])
    const code = typeof payload?.error === 'string' && known.has(payload.error)
      ? payload.error
      : 'outage'
    throw new ProfileReviewError(code)
  }
  if (!payload || typeof payload.summary !== 'string' || !payload.summary.trim() ||
      !isCompleteReviewRepository(payload.updatedRepository, repo)) {
    throw new ProfileReviewError('invalid_output')
  }
  const updated = payload.updatedRepository
  const patch = Object.fromEntries(fields.map(field => [field, updated[field]]))
  return { updatedRepo: { ...repo, ...patch }, summary: payload.summary }
}

function isCompleteReviewRepository(value: unknown, original: ProfessionalRepository): value is ReturnType<typeof careerProfile> {
  if (!value || typeof value !== 'object') return false
  const result = value as Record<string, unknown>
  const textFields = ['careerGoals', 'skills', 'competencies', 'tools', 'employmentStatus', 'currentSalary', 'desiredSalary', 'additionalInfo']
  if (textFields.some((key) => typeof result[key] !== 'string')) return false
  if (!Array.isArray(result.experience) || !Array.isArray(result.projects)) return false
  if (result.experience.length !== original.experience.length || result.projects.length !== original.projects.length) return false
  const sameEntries = (items: unknown[], expected: { id: string }[], fields: string[]) =>
    items.every((item, index) => {
      if (!item || typeof item !== 'object') return false
      const record = item as Record<string, unknown>
      return record.id === expected[index].id && fields.every((field) => typeof record[field] === 'string')
    })
  return sameEntries(result.experience, original.experience, ['company', 'title', 'startDate', 'endDate', 'location', 'description', 'responsibilities', 'achievements']) &&
    result.experience.every((item) => typeof (item as Record<string, unknown>).current === 'boolean') &&
    sameEntries(result.projects, original.projects, ['name', 'description', 'technologies', 'url', 'highlights'])
}

export async function generateMaterials(
  repo: ProfessionalRepository,
  jobPosting: string,
  apiKey: string,
  confirmedQualifications: ConfirmedQualification[] = [],
  cvLanguage: Locale = "en",
  typesafeKey: string = ""
): Promise<GeneratedMaterials> {
  if (!validQualifications(repo)) throw new ApplicationDraftError('input')
  let response: Response
  try {
    response = await fetch('/api/application-draft', {
      method: 'POST', headers: { 'Content-Type': 'application/json', 'X-OpenAI-Api-Key': apiKey, 'X-TypeSafe-Api-Key': typesafeKey },
      body: JSON.stringify({ repository: careerProfile(repo), qualifications: cvQualifications(repo), jobPosting, confirmedQualifications, cvLanguage }), signal: AbortSignal.timeout(30_000),
    })
  } catch (error) {
    throw new ApplicationDraftError(error instanceof DOMException && error.name === 'TimeoutError' ? 'timeout' : 'outage')
  }
  const payload = await response.json().catch(() => null) as { error?: unknown; coverLetter?: unknown } & Partial<Omit<GeneratedMaterials, 'coverLetter'>> | null
  checkJobDecision(payload)
  const known = new Set(['input', 'key', 'rate_limit', 'outage', 'timeout', 'invalid_output', 'capacity'])
  if (!response.ok) throw new ApplicationDraftError(typeof payload?.error === 'string' && known.has(payload.error) ? payload.error : 'outage')
  if (!payload || ['jobSummary','resume','applicationAnswers'].some((key) => typeof payload[key as keyof GeneratedMaterials] !== 'string' || !(payload[key as keyof GeneratedMaterials] as string).trim())) throw new ApplicationDraftError('invalid_output')
  if (['jobTitle', 'company'].some(key => {
    const value = payload[key as 'jobTitle' | 'company']
    return value !== null && (typeof value !== 'string' || !value.trim())
  })) throw new ApplicationDraftError('invalid_output')
  const fullName = repo.fullName?.trim() || ''
  const coverLetter = completeCoverLetter(payload.coverLetter, fullName)
  if (coverLetter === null) throw new ApplicationDraftError('invalid_output')
  return { ...payload, cvLanguage, coverLetter, coverLetterHasSignature: !!fullName } as GeneratedMaterials
}

export async function findProfileGaps(
  repo: ProfessionalRepository,
  jobPosting: string,
  apiKey: string,
  typesafeKey: string = ""
): Promise<ProfileGap[]> {
  if (!validQualifications(repo)) throw new QualificationGapsError('input')
  let response: Response
  try {
    response = await fetch('/api/qualification-gaps', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-OpenAI-Api-Key': apiKey, 'X-TypeSafe-Api-Key': typesafeKey },
      body: JSON.stringify({ repository: careerProfile(repo), qualifications: cvQualifications(repo), jobPosting }),
      signal: AbortSignal.timeout(30_000),
    })
  } catch (error) {
    throw new QualificationGapsError(error instanceof DOMException && error.name === 'TimeoutError' ? 'timeout' : 'outage')
  }
  const payload = await response.json().catch(() => null) as { error?: unknown; gaps?: unknown } | null
  checkJobDecision(payload)
  if (!response.ok) {
    const known = new Set(['input', 'key', 'rate_limit', 'outage', 'timeout', 'invalid_output', 'capacity'])
    throw new QualificationGapsError(typeof payload?.error === 'string' && known.has(payload.error) ? payload.error : 'outage')
  }
  if (!payload || !Array.isArray(payload.gaps) || payload.gaps.length > 5 || payload.gaps.some((gap) => {
    if (!gap || typeof gap !== 'object') return true
    const value = gap as Record<string, unknown>
    return (value.kind !== 'skill' && value.kind !== 'experience') || typeof value.requirement !== 'string' || !value.requirement.trim() || typeof value.details !== 'string' || !value.details.trim()
  })) throw new QualificationGapsError('invalid_output')
  return payload.gaps as ProfileGap[]
}

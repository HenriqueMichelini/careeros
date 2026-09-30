import {
  ProfessionalRepository,
  GeneratedMaterials,
  ProfileGap,
  ConfirmedQualification,
} from './types'

const API_URL = 'https://api.anthropic.com/v1/messages'

export class ProfileReviewError extends Error {
  constructor(public readonly code: string) {
    super(code)
    this.name = 'ProfileReviewError'
  }
}

export class AnthropicWorkflowUnavailableError extends Error {
  constructor() {
    super('This OpenAI API key is available for Profile Review. Application generation will be available after its OpenAI migration.')
    this.name = 'AnthropicWorkflowUnavailableError'
  }
}

const employmentStatusLabels: Record<string, string> = {
  'employed-full-time': 'Employed — Full-time',
  'employed-part-time': 'Employed — Part-time',
  'employed-contract': 'Employed — Contract',
  freelance: 'Freelance / Self-employed',
  looking: 'Actively looking for work',
  open: 'Open to opportunities (not actively searching)',
  unemployed: 'Unemployed',
  student: 'Student',
}

function formatRepo(repo: ProfessionalRepository): string {
  const parts: string[] = []

  if (repo.careerGoals) parts.push(`CAREER GOALS\n${repo.careerGoals}`)
  if (repo.skills) parts.push(`SKILLS\n${repo.skills}`)
  if (repo.competencies) parts.push(`CORE COMPETENCIES\n${repo.competencies}`)

  if (repo.experience.length > 0) {
    const expText = repo.experience.map(e =>
      [
        `${e.title} — ${e.company}${e.location ? ` (${e.location})` : ''}`,
        `${e.startDate} – ${e.current ? 'Present' : e.endDate}`,
        e.description && `Overview: ${e.description}`,
        e.responsibilities && `Responsibilities:\n${e.responsibilities}`,
        e.achievements && `Achievements:\n${e.achievements}`,
      ].filter(Boolean).join('\n')
    ).join('\n\n')
    parts.push(`PROFESSIONAL EXPERIENCE\n${expText}`)
  }

  if (repo.tools) parts.push(`TOOLS & TECHNOLOGIES\n${repo.tools}`)

  if (repo.projects.length > 0) {
    const projText = repo.projects.map(p =>
      [
        p.name,
        p.description,
        p.technologies && `Tech stack: ${p.technologies}`,
        p.highlights && `Highlights: ${p.highlights}`,
        p.url && `URL: ${p.url}`,
      ].filter(Boolean).join('\n')
    ).join('\n\n')
    parts.push(`PERSONAL PROJECTS\n${projText}`)
  }

  if (repo.employmentStatus) {
    const status = employmentStatusLabels[repo.employmentStatus] || repo.employmentStatus
    parts.push(`EMPLOYMENT STATUS\n${status}`)
  }
  if (repo.currentSalary) parts.push(`CURRENT COMPENSATION\n${repo.currentSalary}`)
  if (repo.desiredSalary) parts.push(`DESIRED COMPENSATION\n${repo.desiredSalary}`)
  if (repo.additionalInfo) parts.push(`ADDITIONAL INFORMATION\n${repo.additionalInfo}`)

  return parts.join('\n\n' + '─'.repeat(60) + '\n\n')
}

async function callClaude(apiKey: string, content: string, maxTokens = 4096): Promise<string> {
  if (!apiKey.startsWith('sk-ant-')) throw new AnthropicWorkflowUnavailableError()

  const response = await fetch(API_URL, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'x-api-key': apiKey,
      'anthropic-version': '2023-06-01',
      'anthropic-dangerous-direct-browser-access': 'true',
    },
    body: JSON.stringify({
      model: 'claude-sonnet-5',
      max_tokens: maxTokens,
      messages: [{ role: 'user', content }],
    }),
  })

  if (!response.ok) {
    const err = await response.json().catch(() => ({})) as any
    throw new Error(err?.error?.message || `API error ${response.status}`)
  }

  const data = await response.json() as any
  return data.content[0].text
}

export async function reviewRepository(
  repo: ProfessionalRepository,
  changedSection: string,
  apiKey: string
): Promise<{ updatedRepo: ProfessionalRepository; summary: string }> {
  const response = await fetch('/api/profile/review', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-OpenAI-Api-Key': apiKey,
    },
    body: JSON.stringify({ repository: repo, changedSection }),
    signal: AbortSignal.timeout(30_000),
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
  return { updatedRepo: payload.updatedRepository, summary: payload.summary }
}

function isCompleteReviewRepository(value: unknown, original: ProfessionalRepository): value is ProfessionalRepository {
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
  confirmedQualifications: ConfirmedQualification[] = []
): Promise<GeneratedMaterials> {
  const confirmedContext = confirmedQualifications.length
    ? `\n\nUSER-CONFIRMED QUALIFICATIONS FOR THIS APPLICATION ONLY:\n${confirmedQualifications
        .map(
          ({ kind, requirement, userContext }) =>
            `- ${kind}: ${requirement}${userContext ? `\n  Candidate context: ${userContext}` : ''}`
        )
        .join('\n')}\n\nThe candidate explicitly confirmed each qualification above. Use it as relevant, but never invent examples, employers, dates, proficiency levels, duration, or outcomes. If no context is provided, mention only the qualification itself and do not imply a specific achievement or work history. Do not add these details to the candidate's saved profile.`
    : ''

  const prompt = `You are an expert career coach and professional writer. Generate highly personalized job application materials.

JOB OPPORTUNITY:
${jobPosting}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

CANDIDATE'S PROFESSIONAL PROFILE:
${formatRepo(repo)}
${confirmedContext}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

REQUIREMENTS:
- Draw specifically on the candidate's real experience, skills, and projects — nothing generic
- Tailor every sentence to this exact role and company
- Mirror the tone and vocabulary of the job posting
- Highlight the most relevant qualifications for this specific position
- The resume should be in clean markdown with clear sections and formatting
- The cover letter should be compelling, specific, and under 400 words
- Application answers should address 5-6 common questions for this type of role (Why this role? Why this company? Biggest achievement? Handling challenges? etc.)

Respond ONLY with a valid JSON object (no markdown code blocks) with exactly these fields:
{
  "jobTitle": "exact job title from posting",
  "company": "company name",
  "jobSummary": "2-3 sentence summary of the role and key requirements",
  "resume": "complete formatted resume in markdown",
  "coverLetter": "complete cover letter as plain text",
  "applicationAnswers": "Q&A formatted answers in markdown"
}`

  const text = await callClaude(apiKey, prompt, 8000)

  const match = text.match(/\{[\s\S]*\}/)
  if (!match) throw new Error('AI returned unexpected format')
  return JSON.parse(match[0])
}

export async function findProfileGaps(
  repo: ProfessionalRepository,
  jobPosting: string,
  apiKey: string
): Promise<ProfileGap[]> {
  const prompt = `You are checking whether a candidate's professional profile may omit qualifications they already have.

JOB POSTING:
${jobPosting}

CANDIDATE PROFILE:
${formatRepo(repo)}

Find at most 5 specific skills or types of experience explicitly required or preferred by the posting that are not stated or clearly supported in the profile. This is only a memory prompt for the candidate; do not decide whether they truly have the qualification.

Rules:
- Return only concrete qualifications from the posting, not generic traits or duties.
- Do not list qualifications already supported by equivalent wording in the profile.
- Do not infer a gap just because an exact keyword is absent.
- Prefer the most important and recognizable items.
- If the input is only a URL, too vague, or there are no plausible omitted qualifications, return an empty list.
- Keep requirement concise and details to one short sentence grounded in the posting.

Respond ONLY with valid JSON in this shape. Set kind to either "skill" or "experience":
{"gaps":[{"kind":"skill","requirement":"short qualification name","details":"what the posting asks for"}]}`

  const text = await callClaude(apiKey, prompt, 1200)

  const match = text.match(/\{[\s\S]*\}/)
  if (!match) throw new Error('AI returned unexpected format')

  const parsed = JSON.parse(match[0]) as { gaps?: unknown }
  if (!Array.isArray(parsed.gaps)) throw new Error('AI returned unexpected format')

  return parsed.gaps
    .filter(
      (gap): gap is ProfileGap =>
        typeof gap === 'object' &&
        gap !== null &&
        ((gap as ProfileGap).kind === 'skill' ||
          (gap as ProfileGap).kind === 'experience') &&
        typeof (gap as ProfileGap).requirement === 'string' &&
        typeof (gap as ProfileGap).details === 'string'
    )
    .slice(0, 5)
}

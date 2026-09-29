import { ProfessionalRepository, GeneratedMaterials } from './types'

const API_URL = 'https://api.anthropic.com/v1/messages'

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

  if (repo.employmentStatus) parts.push(`EMPLOYMENT STATUS\n${repo.employmentStatus}`)
  if (repo.currentSalary) parts.push(`CURRENT COMPENSATION\n${repo.currentSalary}`)
  if (repo.desiredSalary) parts.push(`DESIRED COMPENSATION\n${repo.desiredSalary}`)
  if (repo.additionalInfo) parts.push(`ADDITIONAL INFORMATION\n${repo.additionalInfo}`)

  return parts.join('\n\n' + '─'.repeat(60) + '\n\n')
}

async function callClaude(apiKey: string, content: string, maxTokens = 4096): Promise<string> {
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
  const prompt = `You are an expert career coach and professional writing specialist reviewing a career repository.

The user just updated the "${changedSection}" section.

CURRENT REPOSITORY:
${formatRepo(repo)}

FULL REPOSITORY JSON (for structural reference):
${JSON.stringify(repo)}

INSTRUCTIONS:
1. Fix grammatical errors, typos, and awkward phrasing in the changed section
2. Improve clarity, conciseness, and professional tone throughout
3. Ensure consistency across all sections — if the change affects other sections' coherence, update them too
4. Never invent facts — only refine what's already written
5. Preserve all factual content: company names, dates, technologies, project names, numbers

Respond ONLY with a valid JSON object (no markdown, no code blocks) with this exact shape:
{"updatedRepo":${JSON.stringify(repo)},"summary":"1-2 sentence description of changes made"}`

  const text = await callClaude(apiKey, prompt, 6000)

  const match = text.match(/\{[\s\S]*\}/)
  if (!match) throw new Error('AI returned unexpected format')
  return JSON.parse(match[0])
}

export async function generateMaterials(
  repo: ProfessionalRepository,
  jobPosting: string,
  apiKey: string
): Promise<GeneratedMaterials> {
  const prompt = `You are an expert career coach and professional writer. Generate highly personalized job application materials.

JOB OPPORTUNITY:
${jobPosting}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

CANDIDATE'S PROFESSIONAL PROFILE:
${formatRepo(repo)}

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

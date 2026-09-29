import { useState, useCallback } from 'react'
import { useStore } from '../lib/store'
import { reviewRepository } from '../lib/ai'
import { ExperienceEntry, ProjectEntry, ProfessionalRepository } from '../lib/types'

type Section = 'goals' | 'skills' | 'competencies' | 'experience' | 'tools' | 'projects' | 'compensation' | 'other'

const SECTIONS: { id: Section; label: string; desc: string }[] = [
  { id: 'goals', label: 'Career Goals', desc: 'Ambitions, target roles, long-term vision' },
  { id: 'skills', label: 'Skills', desc: 'Technical and professional skills' },
  { id: 'competencies', label: 'Competencies', desc: 'Core strengths and soft skills' },
  { id: 'experience', label: 'Experience', desc: 'Work history, responsibilities, achievements' },
  { id: 'tools', label: 'Tools & Tech', desc: 'Software, frameworks, platforms' },
  { id: 'projects', label: 'Projects', desc: 'Personal and side projects' },
  { id: 'compensation', label: 'Compensation', desc: 'Current and desired salary' },
  { id: 'other', label: 'Other', desc: 'Certifications, education, languages, etc.' },
]

function uid() {
  return Math.random().toString(36).slice(2) + Date.now().toString(36)
}

function Label({ children }: { children: React.ReactNode }) {
  return (
    <label
      className="text-xs uppercase tracking-[0.18em] block mb-2"
      style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
    >
      {children}
    </label>
  )
}

function Field({
  value,
  onChange,
  placeholder,
  rows = 5,
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  rows?: number
}) {
  return (
    <textarea
      rows={rows}
      value={value}
      onChange={e => onChange(e.target.value)}
      placeholder={placeholder}
      className="w-full text-sm p-3 resize-none focus:outline-none transition-colors leading-relaxed"
      style={{
        border: '1px solid var(--color-border)',
        backgroundColor: 'var(--color-card)',
        color: 'var(--color-fg)',
        fontFamily: 'var(--font-sans)',
      }}
      onFocus={e => (e.target.style.borderColor = 'var(--color-fg)')}
      onBlur={e => (e.target.style.borderColor = 'var(--color-border)')}
    />
  )
}

function TextInput({
  value,
  onChange,
  placeholder,
  type = 'text',
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  type?: string
}) {
  return (
    <input
      type={type}
      value={value}
      onChange={e => onChange(e.target.value)}
      placeholder={placeholder}
      className="w-full text-sm px-3 py-2 focus:outline-none transition-colors"
      style={{
        border: '1px solid var(--color-border)',
        backgroundColor: 'var(--color-card)',
        color: 'var(--color-fg)',
        fontFamily: 'var(--font-sans)',
      }}
      onFocus={e => (e.target.style.borderColor = 'var(--color-fg)')}
      onBlur={e => (e.target.style.borderColor = 'var(--color-border)')}
    />
  )
}

function ExperienceForm({
  entry,
  onChange,
  onDelete,
}: {
  entry: ExperienceEntry
  onChange: (e: ExperienceEntry) => void
  onDelete: () => void
}) {
  const u = (field: keyof ExperienceEntry) => (v: string | boolean) =>
    onChange({ ...entry, [field]: v })

  return (
    <div
      className="p-5 mb-4 border"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-card)' }}
    >
      <div className="flex justify-between items-start mb-4">
        <span
          className="text-xs uppercase tracking-widest"
          style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
        >
          Position
        </span>
        <button
          onClick={onDelete}
          className="text-xs transition-opacity hover:opacity-60"
          style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-accent)' }}
        >
          Remove
        </button>
      </div>

      <div className="grid grid-cols-2 gap-4 mb-4">
        <div>
          <Label>Job Title</Label>
          <TextInput value={entry.title} onChange={u('title')} placeholder="Senior Software Engineer" />
        </div>
        <div>
          <Label>Company</Label>
          <TextInput value={entry.company} onChange={u('company')} placeholder="Acme Corp" />
        </div>
        <div>
          <Label>Location</Label>
          <TextInput value={entry.location} onChange={u('location')} placeholder="San Francisco, CA / Remote" />
        </div>
        <div className="grid grid-cols-[1fr_1fr_auto] gap-2 items-end">
          <div>
            <Label>Start Date</Label>
            <TextInput value={entry.startDate} onChange={u('startDate')} placeholder="Jan 2021" />
          </div>
          <div>
            <Label>End Date</Label>
            <TextInput
              value={entry.endDate}
              onChange={u('endDate')}
              placeholder="Dec 2023"
            />
          </div>
          <div className="flex items-center gap-1.5 pb-2">
            <input
              type="checkbox"
              id={`current-${entry.id}`}
              checked={entry.current}
              onChange={e => u('current')(e.target.checked)}
              className="cursor-pointer"
            />
            <label
              htmlFor={`current-${entry.id}`}
              className="text-xs cursor-pointer"
              style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
            >
              Current
            </label>
          </div>
        </div>
      </div>

      <div className="space-y-4">
        <div>
          <Label>Overview / Description</Label>
          <Field
            rows={3}
            value={entry.description}
            onChange={u('description')}
            placeholder="Brief overview of the role and your scope of work."
          />
        </div>
        <div>
          <Label>Responsibilities</Label>
          <Field
            rows={4}
            value={entry.responsibilities}
            onChange={u('responsibilities')}
            placeholder="Key responsibilities — use one per line or bullet points."
          />
        </div>
        <div>
          <Label>Achievements & Impact</Label>
          <Field
            rows={4}
            value={entry.achievements}
            onChange={u('achievements')}
            placeholder="Quantified achievements and notable outcomes. Include metrics where possible."
          />
        </div>
      </div>
    </div>
  )
}

function ProjectForm({
  entry,
  onChange,
  onDelete,
}: {
  entry: ProjectEntry
  onChange: (e: ProjectEntry) => void
  onDelete: () => void
}) {
  const u = (field: keyof ProjectEntry) => (v: string) => onChange({ ...entry, [field]: v })

  return (
    <div
      className="p-5 mb-4 border"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-card)' }}
    >
      <div className="flex justify-between items-start mb-4">
        <span
          className="text-xs uppercase tracking-widest"
          style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
        >
          Project
        </span>
        <button
          onClick={onDelete}
          className="text-xs transition-opacity hover:opacity-60"
          style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-accent)' }}
        >
          Remove
        </button>
      </div>

      <div className="grid grid-cols-2 gap-4 mb-4">
        <div>
          <Label>Project Name</Label>
          <TextInput value={entry.name} onChange={u('name')} placeholder="OpenMetrics Dashboard" />
        </div>
        <div>
          <Label>URL</Label>
          <TextInput value={entry.url} onChange={u('url')} placeholder="https://github.com/you/project" />
        </div>
      </div>

      <div className="space-y-4">
        <div>
          <Label>Description</Label>
          <Field
            rows={3}
            value={entry.description}
            onChange={u('description')}
            placeholder="What is this project and why did you build it?"
          />
        </div>
        <div>
          <Label>Technologies Used</Label>
          <TextInput
            value={entry.technologies}
            onChange={u('technologies')}
            placeholder="React, TypeScript, PostgreSQL, Docker"
          />
        </div>
        <div>
          <Label>Highlights & Impact</Label>
          <Field
            rows={3}
            value={entry.highlights}
            onChange={u('highlights')}
            placeholder="Key features, technical challenges solved, users reached, or results achieved."
          />
        </div>
      </div>
    </div>
  )
}

export default function RepositoryPage() {
  const { state, dispatch } = useStore()
  const [activeSection, setActiveSection] = useState<Section>('goals')
  const [reviewError, setReviewError] = useState('')
  const repo = state.repository

  const updateRepo = useCallback(
    (patch: Partial<ProfessionalRepository>) =>
      dispatch({ type: 'SET_REPO', payload: { ...repo, ...patch } }),
    [repo, dispatch]
  )

  async function handleAiReview() {
    if (!state.apiKey) {
      setReviewError('Set your API key first.')
      return
    }
    setReviewError('')
    dispatch({ type: 'SET_REVIEWING', payload: true })
    try {
      const sectionLabel = SECTIONS.find(s => s.id === activeSection)?.label || activeSection
      const { updatedRepo, summary } = await reviewRepository(repo, sectionLabel, state.apiKey)
      dispatch({ type: 'SET_REPO', payload: updatedRepo })
      dispatch({ type: 'SET_REVIEW_SUMMARY', payload: summary })
    } catch (e: any) {
      setReviewError(e.message || 'Review failed. Try again.')
    } finally {
      dispatch({ type: 'SET_REVIEWING', payload: false })
    }
  }

  function addExperience() {
    const entry: ExperienceEntry = {
      id: uid(),
      company: '',
      title: '',
      startDate: '',
      endDate: '',
      current: false,
      location: '',
      description: '',
      responsibilities: '',
      achievements: '',
    }
    updateRepo({ experience: [entry, ...repo.experience] })
  }

  function updateExperience(id: string, updated: ExperienceEntry) {
    updateRepo({ experience: repo.experience.map(e => (e.id === id ? updated : e)) })
  }

  function deleteExperience(id: string) {
    updateRepo({ experience: repo.experience.filter(e => e.id !== id) })
  }

  function addProject() {
    const entry: ProjectEntry = {
      id: uid(),
      name: '',
      description: '',
      technologies: '',
      url: '',
      highlights: '',
    }
    updateRepo({ projects: [entry, ...repo.projects] })
  }

  function updateProject(id: string, updated: ProjectEntry) {
    updateRepo({ projects: repo.projects.map(p => (p.id === id ? updated : p)) })
  }

  function deleteProject(id: string) {
    updateRepo({ projects: repo.projects.filter(p => p.id !== id) })
  }

  const active = SECTIONS.find(s => s.id === activeSection)!

  return (
    <div className="max-w-6xl mx-auto px-6 py-10 grid grid-cols-[220px_1fr] gap-12">
      {/* Sidebar nav */}
      <aside>
        <p
          className="text-xs uppercase tracking-[0.25em] mb-5"
          style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
        >
          Sections
        </p>
        <nav className="space-y-0.5">
          {SECTIONS.map(({ id, label }) => (
            <button
              key={id}
              onClick={() => setActiveSection(id)}
              className="w-full text-left px-3 py-2.5 text-sm transition-colors block"
              style={{
                backgroundColor: activeSection === id ? 'var(--color-fg)' : 'transparent',
                color: activeSection === id ? 'var(--color-bg)' : 'var(--color-muted-fg)',
              }}
            >
              {label}
            </button>
          ))}
        </nav>

        {/* AI Review */}
        <div className="mt-10 pt-6 border-t" style={{ borderColor: 'var(--color-border)' }}>
          <p
            className="text-xs uppercase tracking-[0.18em] mb-3"
            style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
          >
            AI Review
          </p>
          <p className="text-xs mb-4 leading-relaxed" style={{ color: 'var(--color-muted-fg)' }}>
            Analyzes the entire repository for consistency, clarity, and coherence after changes.
          </p>
          <button
            onClick={handleAiReview}
            disabled={state.isReviewingRepo}
            className="w-full py-2.5 text-xs uppercase tracking-[0.15em] transition-colors"
            style={{
              fontFamily: 'var(--font-mono)',
              border: '1px solid var(--color-fg)',
              backgroundColor: state.isReviewingRepo ? 'var(--color-muted)' : 'transparent',
              color: state.isReviewingRepo ? 'var(--color-muted-fg)' : 'var(--color-fg)',
              cursor: state.isReviewingRepo ? 'not-allowed' : 'pointer',
            }}
          >
            {state.isReviewingRepo ? 'Reviewing...' : 'Review with AI'}
          </button>
          {reviewError && (
            <p
              className="text-xs mt-2"
              style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-accent)' }}
            >
              {reviewError}
            </p>
          )}
          {state.lastReviewSummary && !reviewError && (
            <p
              className="text-xs mt-3 leading-relaxed"
              style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
            >
              Last: {state.lastReviewSummary}
            </p>
          )}
        </div>
      </aside>

      {/* Main content */}
      <div>
        <div className="mb-8">
          <h1
            className="text-5xl font-bold uppercase tracking-tight leading-none mb-2"
            style={{ fontFamily: 'var(--font-display)' }}
          >
            {active.label}
          </h1>
          <p className="text-sm" style={{ color: 'var(--color-muted-fg)' }}>
            {active.desc}
          </p>
        </div>

        {/* Section content */}
        {activeSection === 'goals' && (
          <div>
            <Label>Career Goals & Ambitions</Label>
            <Field
              rows={10}
              value={repo.careerGoals}
              onChange={v => updateRepo({ careerGoals: v })}
              placeholder="Describe your short and long-term career goals. What roles are you targeting? What industries? What kind of impact do you want to make? Where do you see yourself in 3–5 years?"
            />
          </div>
        )}

        {activeSection === 'skills' && (
          <div>
            <Label>Skills</Label>
            <Field
              rows={12}
              value={repo.skills}
              onChange={v => updateRepo({ skills: v })}
              placeholder="List your technical and professional skills. Group them by category if helpful.

Example:
Programming Languages: Python, TypeScript, Rust, Go
Frontend: React, Next.js, Vue, TailwindCSS
Backend: Node.js, FastAPI, Django, PostgreSQL
Cloud: AWS (EC2, S3, Lambda, RDS), GCP, Docker, Kubernetes
..."
            />
          </div>
        )}

        {activeSection === 'competencies' && (
          <div>
            <Label>Core Competencies</Label>
            <Field
              rows={10}
              value={repo.competencies}
              onChange={v => updateRepo({ competencies: v })}
              placeholder="Describe your core strengths, soft skills, and professional competencies.

Examples: Strategic thinking, cross-functional leadership, agile project management, stakeholder communication, data-driven decision making, mentoring junior engineers, system design, technical writing..."
            />
          </div>
        )}

        {activeSection === 'experience' && (
          <div>
            <div className="flex justify-between items-center mb-5">
              <span
                className="text-xs uppercase tracking-[0.18em]"
                style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
              >
                {repo.experience.length} {repo.experience.length === 1 ? 'Position' : 'Positions'}
              </span>
              <button
                onClick={addExperience}
                className="text-xs uppercase tracking-[0.15em] px-4 py-2 transition-colors hover:opacity-75"
                style={{
                  fontFamily: 'var(--font-mono)',
                  backgroundColor: 'var(--color-fg)',
                  color: 'var(--color-bg)',
                }}
              >
                + Add Position
              </button>
            </div>
            {repo.experience.length === 0 ? (
              <div
                className="py-16 text-center border border-dashed"
                style={{ borderColor: 'var(--color-border)' }}
              >
                <p className="text-sm mb-4" style={{ color: 'var(--color-muted-fg)' }}>
                  No positions added yet
                </p>
                <button
                  onClick={addExperience}
                  className="text-xs uppercase tracking-widest px-4 py-2"
                  style={{
                    fontFamily: 'var(--font-mono)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-muted-fg)',
                  }}
                >
                  Add your first position
                </button>
              </div>
            ) : (
              repo.experience.map(entry => (
                <ExperienceForm
                  key={entry.id}
                  entry={entry}
                  onChange={updated => updateExperience(entry.id, updated)}
                  onDelete={() => deleteExperience(entry.id)}
                />
              ))
            )}
          </div>
        )}

        {activeSection === 'tools' && (
          <div>
            <Label>Tools & Technologies</Label>
            <Field
              rows={12}
              value={repo.tools}
              onChange={v => updateRepo({ tools: v })}
              placeholder="List the software, frameworks, platforms, and tools you use.

Examples:
Development: VS Code, Git, GitHub, Docker, Kubernetes, Terraform
Databases: PostgreSQL, MongoDB, Redis, Elasticsearch
Cloud Platforms: AWS, GCP, Azure
Design: Figma, Sketch
Project Management: Jira, Linear, Notion, Confluence
Communication: Slack, Zoom
..."
            />
          </div>
        )}

        {activeSection === 'projects' && (
          <div>
            <div className="flex justify-between items-center mb-5">
              <span
                className="text-xs uppercase tracking-[0.18em]"
                style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
              >
                {repo.projects.length} {repo.projects.length === 1 ? 'Project' : 'Projects'}
              </span>
              <button
                onClick={addProject}
                className="text-xs uppercase tracking-[0.15em] px-4 py-2 transition-colors hover:opacity-75"
                style={{
                  fontFamily: 'var(--font-mono)',
                  backgroundColor: 'var(--color-fg)',
                  color: 'var(--color-bg)',
                }}
              >
                + Add Project
              </button>
            </div>
            {repo.projects.length === 0 ? (
              <div
                className="py-16 text-center border border-dashed"
                style={{ borderColor: 'var(--color-border)' }}
              >
                <p className="text-sm mb-4" style={{ color: 'var(--color-muted-fg)' }}>
                  No projects added yet
                </p>
                <button
                  onClick={addProject}
                  className="text-xs uppercase tracking-widest px-4 py-2"
                  style={{
                    fontFamily: 'var(--font-mono)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-muted-fg)',
                  }}
                >
                  Add your first project
                </button>
              </div>
            ) : (
              repo.projects.map(entry => (
                <ProjectForm
                  key={entry.id}
                  entry={entry}
                  onChange={updated => updateProject(entry.id, updated)}
                  onDelete={() => deleteProject(entry.id)}
                />
              ))
            )}
          </div>
        )}

        {activeSection === 'compensation' && (
          <div className="space-y-8">
            <div>
              <Label>Employment Status</Label>
              <select
                value={repo.employmentStatus}
                onChange={e => updateRepo({ employmentStatus: e.target.value })}
                className="w-full text-sm px-3 py-2 focus:outline-none"
                style={{
                  border: '1px solid var(--color-border)',
                  backgroundColor: 'var(--color-card)',
                  color: 'var(--color-fg)',
                  fontFamily: 'var(--font-sans)',
                }}
              >
                <option>Employed — Full-time</option>
                <option>Employed — Part-time</option>
                <option>Employed — Contract</option>
                <option>Freelance / Self-employed</option>
                <option>Actively looking for work</option>
                <option>Open to opportunities (not actively searching)</option>
                <option>Unemployed</option>
                <option>Student</option>
              </select>
            </div>

            <div className="grid grid-cols-2 gap-6">
              <div>
                <Label>Current Compensation</Label>
                <TextInput
                  value={repo.currentSalary}
                  onChange={v => updateRepo({ currentSalary: v })}
                  placeholder="e.g. $120,000/yr + $20k bonus + equity"
                />
                <p
                  className="text-xs mt-1.5 leading-relaxed"
                  style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
                >
                  Include base, bonus, equity, and benefits if relevant.
                </p>
              </div>
              <div>
                <Label>Desired Compensation</Label>
                <TextInput
                  value={repo.desiredSalary}
                  onChange={v => updateRepo({ desiredSalary: v })}
                  placeholder="e.g. $150,000–$180,000/yr"
                />
                <p
                  className="text-xs mt-1.5 leading-relaxed"
                  style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-muted-fg)' }}
                >
                  Total compensation target, including equity if applicable.
                </p>
              </div>
            </div>
          </div>
        )}

        {activeSection === 'other' && (
          <div>
            <Label>Additional Professional Information</Label>
            <Field
              rows={14}
              value={repo.additionalInfo}
              onChange={v => updateRepo({ additionalInfo: v })}
              placeholder="Include any additional information relevant to your professional background:

Education: degrees, universities, graduation years
Certifications: AWS Solutions Architect, PMP, CPA, etc.
Languages: English (native), Spanish (conversational), etc.
Publications & talks: conference presentations, articles, papers
Awards & recognition: industry awards, hackathon wins, etc.
Volunteer work: relevant volunteer roles or open source contributions
Professional memberships: industry associations, boards, etc.
Geographic preferences: cities, remote/hybrid/on-site preferences
Visa status & work authorization (if relevant)"
            />
          </div>
        )}
      </div>
    </div>
  )
}

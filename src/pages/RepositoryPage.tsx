import { useState, useCallback } from "react"
import { useI18n, useStore } from "../lib/store"
import { reviewRepository } from "../lib/ai"
import { ProfileReviewError } from "../lib/ai"
import {
  ExperienceEntry,
  ProjectEntry,
  ProfessionalRepository,
} from "../lib/types"
import { TranslationKey } from "../lib/i18n"

type Section = "profile" | "goals" | "skills" | "competencies" | "experience" | "tools" | "projects" | "compensation" | "other"

const SECTIONS: {
  id: Section
  labelKey: TranslationKey
  descKey: TranslationKey
}[] = [
  {
    id: "profile",
    labelKey: "repo.section.profile",
    descKey: "repo.section.profileDesc",
  },
  {
    id: "goals",
    labelKey: "repo.section.goals",
    descKey: "repo.section.goalsDesc",
  },
  {
    id: "skills",
    labelKey: "repo.section.skills",
    descKey: "repo.section.skillsDesc",
  },
  {
    id: "competencies",
    labelKey: "repo.section.competencies",
    descKey: "repo.section.competenciesDesc",
  },
  {
    id: "experience",
    labelKey: "repo.section.experience",
    descKey: "repo.section.experienceDesc",
  },
  {
    id: "tools",
    labelKey: "repo.section.tools",
    descKey: "repo.section.toolsDesc",
  },
  {
    id: "projects",
    labelKey: "repo.section.projects",
    descKey: "repo.section.projectsDesc",
  },
  {
    id: "compensation",
    labelKey: "repo.section.compensation",
    descKey: "repo.section.compensationDesc",
  },
  {
    id: "other",
    labelKey: "repo.section.other",
    descKey: "repo.section.otherDesc",
  },
]

const EMPLOYMENT_STATUS_OPTIONS = [
  { value: "employed-full-time", key: "repo.status.employedFullTime" },
  { value: "employed-part-time", key: "repo.status.employedPartTime" },
  { value: "employed-contract", key: "repo.status.employedContract" },
  { value: "freelance", key: "repo.status.freelance" },
  { value: "looking", key: "repo.status.looking" },
  { value: "open", key: "repo.status.open" },
  { value: "unemployed", key: "repo.status.unemployed" },
  { value: "student", key: "repo.status.student" },
] as const

const LEGACY_EMPLOYMENT_STATUS: Record<string, string> = {
  Employed: "employed-full-time",
  "Employed — Full-time": "employed-full-time",
  "Employed — Part-time": "employed-part-time",
  "Employed — Contract": "employed-contract",
  "Freelance / Self-employed": "freelance",
  "Actively looking for work": "looking",
  "Open to opportunities (not actively searching)": "open",
  Unemployed: "unemployed",
  Student: "student",
}

function uid() {
  return Math.random().toString(36).slice(2) + Date.now().toString(36)
}

function Label({ children }: { children: React.ReactNode }) {
  return (
    <label
      className="text-xs uppercase tracking-[0.18em] block mb-2"
      style={{ fontFamily: "var(--font-mono)", color: "var(--color-muted-fg)" }}
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
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className="w-full text-sm p-3 resize-none focus:outline-none transition-colors leading-relaxed"
      style={{
        border: "1px solid var(--color-border)",
        backgroundColor: "var(--color-card)",
        color: "var(--color-fg)",
        fontFamily: "var(--font-sans)",
      }}
      onFocus={(e) => (e.target.style.borderColor = "var(--color-fg)")}
      onBlur={(e) => (e.target.style.borderColor = "var(--color-border)")}
    />
  )
}

function TextInput({
  value,
  onChange,
  placeholder,
  type = "text",
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
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className="w-full text-sm px-3 py-2 focus:outline-none transition-colors"
      style={{
        border: "1px solid var(--color-border)",
        backgroundColor: "var(--color-card)",
        color: "var(--color-fg)",
        fontFamily: "var(--font-sans)",
      }}
      onFocus={(e) => (e.target.style.borderColor = "var(--color-fg)")}
      onBlur={(e) => (e.target.style.borderColor = "var(--color-border)")}
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
  const { t } = useI18n()
  const u = (field: keyof ExperienceEntry) => (v: string | boolean) =>
    onChange({ ...entry, [field]: v })

  return (
    <div
      className="p-5 mb-4 border"
      style={{
        borderColor: "var(--color-border)",
        backgroundColor: "var(--color-card)",
      }}
    >
      <div className="flex justify-between items-start mb-4">
        <span
          className="text-xs uppercase tracking-widest"
          style={{
            fontFamily: "var(--font-mono)",
            color: "var(--color-muted-fg)",
          }}
        >
          {t("repo.position")}
        </span>
        <button
          onClick={onDelete}
          className="text-xs transition-opacity hover:opacity-60"
          style={{
            fontFamily: "var(--font-mono)",
            color: "var(--color-accent)",
          }}
        >
          {t("common.remove")}
        </button>
      </div>

      <div className="grid grid-cols-2 gap-4 mb-4">
        <div>
          <Label>{t("repo.jobTitle")}</Label>
          <TextInput
            value={entry.title}
            onChange={u("title")}
            placeholder={t("repo.jobTitlePlaceholder")}
          />
        </div>
        <div>
          <Label>{t("repo.company")}</Label>
          <TextInput
            value={entry.company}
            onChange={u("company")}
            placeholder={t("repo.companyPlaceholder")}
          />
        </div>
        <div>
          <Label>{t("repo.location")}</Label>
          <TextInput
            value={entry.location}
            onChange={u("location")}
            placeholder={t("repo.locationPlaceholder")}
          />
        </div>
        <div className="grid grid-cols-[1fr_1fr_auto] gap-2 items-end">
          <div>
            <Label>{t("repo.startDate")}</Label>
            <TextInput
              value={entry.startDate}
              onChange={u("startDate")}
              placeholder={t("repo.startDatePlaceholder")}
            />
          </div>
          <div>
            <Label>{t("repo.endDate")}</Label>
            <TextInput
              value={entry.endDate}
              onChange={u("endDate")}
              placeholder={t("repo.endDatePlaceholder")}
            />
          </div>
          <div className="flex items-center gap-1.5 pb-2">
            <input
              type="checkbox"
              id={`current-${entry.id}`}
              checked={entry.current}
              onChange={(e) => u("current")(e.target.checked)}
              className="cursor-pointer"
            />
            <label
              htmlFor={`current-${entry.id}`}
              className="text-xs cursor-pointer"
              style={{
                fontFamily: "var(--font-mono)",
                color: "var(--color-muted-fg)",
              }}
            >
              {t("common.current")}
            </label>
          </div>
        </div>
      </div>

      <div className="space-y-4">
        <div>
          <Label>{t("repo.overview")}</Label>
          <Field
            rows={3}
            value={entry.description}
            onChange={u("description")}
            placeholder={t("repo.overviewPlaceholder")}
          />
        </div>
        <div>
          <Label>{t("repo.responsibilities")}</Label>
          <Field
            rows={4}
            value={entry.responsibilities}
            onChange={u("responsibilities")}
            placeholder={t("repo.responsibilitiesPlaceholder")}
          />
        </div>
        <div>
          <Label>{t("repo.achievements")}</Label>
          <Field
            rows={4}
            value={entry.achievements}
            onChange={u("achievements")}
            placeholder={t("repo.achievementsPlaceholder")}
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
  const { t } = useI18n()
  const u = (field: keyof ProjectEntry) => (v: string) =>
    onChange({ ...entry, [field]: v })

  return (
    <div
      className="p-5 mb-4 border"
      style={{
        borderColor: "var(--color-border)",
        backgroundColor: "var(--color-card)",
      }}
    >
      <div className="flex justify-between items-start mb-4">
        <span
          className="text-xs uppercase tracking-widest"
          style={{
            fontFamily: "var(--font-mono)",
            color: "var(--color-muted-fg)",
          }}
        >
          {t("repo.project")}
        </span>
        <button
          onClick={onDelete}
          className="text-xs transition-opacity hover:opacity-60"
          style={{
            fontFamily: "var(--font-mono)",
            color: "var(--color-accent)",
          }}
        >
          {t("common.remove")}
        </button>
      </div>

      <div className="grid grid-cols-2 gap-4 mb-4">
        <div>
          <Label>{t("repo.projectName")}</Label>
          <TextInput
            value={entry.name}
            onChange={u("name")}
            placeholder={t("repo.projectNamePlaceholder")}
          />
        </div>
        <div>
          <Label>{t("repo.url")}</Label>
          <TextInput
            value={entry.url}
            onChange={u("url")}
            placeholder={t("repo.urlPlaceholder")}
          />
        </div>
      </div>

      <div className="space-y-4">
        <div>
          <Label>{t("repo.description")}</Label>
          <Field
            rows={3}
            value={entry.description}
            onChange={u("description")}
            placeholder={t("repo.descriptionPlaceholder")}
          />
        </div>
        <div>
          <Label>{t("repo.technologiesUsed")}</Label>
          <TextInput
            value={entry.technologies}
            onChange={u("technologies")}
            placeholder={t("repo.technologiesPlaceholder")}
          />
        </div>
        <div>
          <Label>{t("repo.highlights")}</Label>
          <Field
            rows={3}
            value={entry.highlights}
            onChange={u("highlights")}
            placeholder={t("repo.highlightsPlaceholder")}
          />
        </div>
      </div>
    </div>
  )
}

export default function RepositoryPage() {
  const { state, dispatch } = useStore()
  const { t } = useI18n()
  const [activeSection, setActiveSection] = useState<Section>("profile")
  const [reviewError, setReviewError] = useState("")
  const repo = state.repository

  const updateRepo = useCallback(
    (patch: Partial<ProfessionalRepository>) =>
      dispatch({ type: "SET_REPO", payload: { ...repo, ...patch } }),
    [repo, dispatch],
  )

  async function handleAiReview() {
    if (!state.apiKey) {
      setReviewError(t("repo.setApiKeyFirst"))
      return
    }
    setReviewError("")
    dispatch({ type: "SET_REVIEWING", payload: true })
    try {
      const section = SECTIONS.find((s) => s.id === activeSection)
      const sectionLabel = section ? t(section.labelKey) : activeSection
      const { updatedRepo, summary } = await reviewRepository(
        repo,
        sectionLabel,
        state.apiKey,
      )
      dispatch({ type: "SET_REPO", payload: updatedRepo })
      dispatch({ type: "SET_REVIEW_SUMMARY", payload: summary })
    } catch (e: any) {
      const errorKey = e instanceof ProfileReviewError
        ? ({
            input: "repo.reviewErrorInput",
            key: "repo.reviewErrorKey",
            rate_limit: "repo.reviewErrorRateLimit",
            outage: "repo.reviewErrorOutage",
            timeout: "repo.reviewErrorTimeout",
            invalid_output: "repo.reviewErrorInvalidOutput",
          } as const)[e.code as "input" | "key" | "rate_limit" | "outage" | "timeout" | "invalid_output"]
        : undefined
      setReviewError(errorKey ? t(errorKey) : t("repo.reviewFailed"))
    } finally {
      dispatch({ type: "SET_REVIEWING", payload: false })
    }
  }

  function addExperience() {
    const entry: ExperienceEntry = {
      id: uid(),
      company: "",
      title: "",
      startDate: "",
      endDate: "",
      current: false,
      location: "",
      description: "",
      responsibilities: "",
      achievements: "",
    }
    updateRepo({ experience: [entry, ...repo.experience] })
  }

  function updateExperience(id: string, updated: ExperienceEntry) {
    updateRepo({
      experience: repo.experience.map((e) => (e.id === id ? updated : e)),
    })
  }

  function deleteExperience(id: string) {
    updateRepo({ experience: repo.experience.filter((e) => e.id !== id) })
  }

  function addProject() {
    const entry: ProjectEntry = {
      id: uid(),
      name: "",
      description: "",
      technologies: "",
      url: "",
      highlights: "",
    }
    updateRepo({ projects: [entry, ...repo.projects] })
  }

  function updateProject(id: string, updated: ProjectEntry) {
    updateRepo({
      projects: repo.projects.map((p) => (p.id === id ? updated : p)),
    })
  }

  function deleteProject(id: string) {
    updateRepo({ projects: repo.projects.filter((p) => p.id !== id) })
  }

  const active = SECTIONS.find((s) => s.id === activeSection)!
  const employmentStatus =
    LEGACY_EMPLOYMENT_STATUS[repo.employmentStatus] || repo.employmentStatus

  return (
    <div className="max-w-6xl mx-auto px-6 py-10 grid grid-cols-[220px_1fr] gap-12">
      {/* Sidebar nav */}
      <aside>
        <p
          className="text-xs uppercase tracking-[0.25em] mb-5"
          style={{
            fontFamily: "var(--font-mono)",
            color: "var(--color-muted-fg)",
          }}
        >
          {t("repo.sections")}
        </p>
        <nav className="space-y-0.5">
          {SECTIONS.map(({ id, labelKey }) => (
            <button
              key={id}
              onClick={() => setActiveSection(id)}
              className={`w-full text-left px-3 py-2.5 text-sm transition-colors block ${
                id === "profile"
                  ? "font-semibold tracking-[0.12em] mb-2 border"
                  : ""
              }`}
              style={{
                backgroundColor:
                  activeSection === id
                    ? id === "profile"
                      ? "var(--color-accent)"
                      : "var(--color-fg)"
                    : id === "profile"
                      ? "var(--color-muted)"
                      : "transparent",
                color:
                  activeSection === id && id !== "profile"
                    ? "var(--color-bg)"
                    : "var(--color-fg)",
                borderColor:
                  id === "profile" ? "var(--color-accent)" : "transparent",
              }}
            >
              {t(labelKey)}
            </button>
          ))}
        </nav>

        {/* AI Review */}
        <div
          className="mt-10 pt-6 border-t"
          style={{ borderColor: "var(--color-border)" }}
        >
          <p
            className="text-xs uppercase tracking-[0.18em] mb-3"
            style={{
              fontFamily: "var(--font-mono)",
              color: "var(--color-muted-fg)",
            }}
          >
            {t("repo.aiReview")}
          </p>
          <p
            className="text-xs mb-4 leading-relaxed"
            style={{ color: "var(--color-muted-fg)" }}
          >
            {t("repo.aiReviewDescription")}
          </p>
          <button
            onClick={handleAiReview}
            disabled={state.isReviewingRepo}
            className="w-full py-2.5 text-xs uppercase tracking-[0.15em] transition-colors"
            style={{
              fontFamily: "var(--font-mono)",
              border: "1px solid var(--color-fg)",
              backgroundColor: state.isReviewingRepo
                ? "var(--color-muted)"
                : "transparent",
              color: state.isReviewingRepo
                ? "var(--color-muted-fg)"
                : "var(--color-fg)",
              cursor: state.isReviewingRepo ? "not-allowed" : "pointer",
            }}
          >
            {state.isReviewingRepo
              ? t("repo.reviewing")
              : t("repo.reviewWithAi")}
          </button>
          {reviewError && (
            <p
              className="text-xs mt-2"
              style={{
                fontFamily: "var(--font-mono)",
                color: "var(--color-accent)",
              }}
            >
              {reviewError}
            </p>
          )}
          {state.lastReviewSummary && !reviewError && (
            <p
              className="text-xs mt-3 leading-relaxed"
              style={{
                fontFamily: "var(--font-mono)",
                color: "var(--color-muted-fg)",
              }}
            >
              {t("repo.last")}: {state.lastReviewSummary}
            </p>
          )}
        </div>
      </aside>

      {/* Main content */}
      <div>
        <div className="mb-8">
          <h1
            className="text-5xl font-bold uppercase tracking-tight leading-none mb-2"
            style={{ fontFamily: "var(--font-display)" }}
          >
            {t(active.labelKey)}
          </h1>
          <p className="text-sm" style={{ color: "var(--color-muted-fg)" }}>
            {t(active.descKey)}
          </p>
        </div>

        {/* Section content */}
        {activeSection === "profile" && (
          <section
            className="p-5 border"
            style={{
              backgroundColor: "var(--color-muted)",
              borderColor: "var(--color-accent)",
            }}
          >
            <p
              className="text-xs mt-2"
              style={{ color: "var(--color-muted-fg)" }}
            >
              {t("repo.profileIntro")}
            </p>
          </section>
        )}

        {activeSection === "goals" && (
          <div>
            <Label>{t("repo.careerGoals")}</Label>
            <Field
              rows={10}
              value={repo.careerGoals}
              onChange={(v) => updateRepo({ careerGoals: v })}
              placeholder={t("repo.careerGoalsPlaceholder")}
            />
          </div>
        )}

        {activeSection === "skills" && (
          <div>
            <Label>{t("repo.section.skills")}</Label>
            <Field
              rows={12}
              value={repo.skills}
              onChange={(v) => updateRepo({ skills: v })}
              placeholder={t("repo.skillsPlaceholder")}
            />
          </div>
        )}

        {activeSection === "competencies" && (
          <div>
            <Label>{t("repo.competencies")}</Label>
            <Field
              rows={10}
              value={repo.competencies}
              onChange={(v) => updateRepo({ competencies: v })}
              placeholder={t("repo.competenciesPlaceholder")}
            />
          </div>
        )}

        {activeSection === "experience" && (
          <div>
            <div className="flex justify-between items-center mb-5">
              <span
                className="text-xs uppercase tracking-[0.18em]"
                style={{
                  fontFamily: "var(--font-mono)",
                  color: "var(--color-muted-fg)",
                }}
              >
                {repo.experience.length}{" "}
                {repo.experience.length === 1
                  ? t("repo.position")
                  : t("repo.positions")}
              </span>
              <button
                onClick={addExperience}
                className="text-xs uppercase tracking-[0.15em] px-4 py-2 transition-colors hover:opacity-75"
                style={{
                  fontFamily: "var(--font-mono)",
                  backgroundColor: "var(--color-fg)",
                  color: "var(--color-bg)",
                }}
              >
                {t("repo.addPosition")}
              </button>
            </div>
            {repo.experience.length === 0 ? (
              <div
                className="py-16 text-center border border-dashed"
                style={{ borderColor: "var(--color-border)" }}
              >
                <p
                  className="text-sm mb-4"
                  style={{ color: "var(--color-muted-fg)" }}
                >
                  {t("repo.noPositions")}
                </p>
                <button
                  onClick={addExperience}
                  className="text-xs uppercase tracking-widest px-4 py-2"
                  style={{
                    fontFamily: "var(--font-mono)",
                    border: "1px solid var(--color-border)",
                    color: "var(--color-muted-fg)",
                  }}
                >
                  {t("repo.addFirstPosition")}
                </button>
              </div>
            ) : (
              repo.experience.map((entry) => (
                <ExperienceForm
                  key={entry.id}
                  entry={entry}
                  onChange={(updated) => updateExperience(entry.id, updated)}
                  onDelete={() => deleteExperience(entry.id)}
                />
              ))
            )}
          </div>
        )}

        {activeSection === "tools" && (
          <div>
            <Label>{t("repo.tools")}</Label>
            <Field
              rows={12}
              value={repo.tools}
              onChange={(v) => updateRepo({ tools: v })}
              placeholder={t("repo.toolsPlaceholder")}
            />
          </div>
        )}

        {activeSection === "projects" && (
          <div>
            <div className="flex justify-between items-center mb-5">
              <span
                className="text-xs uppercase tracking-[0.18em]"
                style={{
                  fontFamily: "var(--font-mono)",
                  color: "var(--color-muted-fg)",
                }}
              >
                {repo.projects.length}{" "}
                {repo.projects.length === 1
                  ? t("repo.projectsCountSingular")
                  : t("repo.projectsCountPlural")}
              </span>
              <button
                onClick={addProject}
                className="text-xs uppercase tracking-[0.15em] px-4 py-2 transition-colors hover:opacity-75"
                style={{
                  fontFamily: "var(--font-mono)",
                  backgroundColor: "var(--color-fg)",
                  color: "var(--color-bg)",
                }}
              >
                {t("repo.addProject")}
              </button>
            </div>
            {repo.projects.length === 0 ? (
              <div
                className="py-16 text-center border border-dashed"
                style={{ borderColor: "var(--color-border)" }}
              >
                <p
                  className="text-sm mb-4"
                  style={{ color: "var(--color-muted-fg)" }}
                >
                  {t("repo.noProjects")}
                </p>
                <button
                  onClick={addProject}
                  className="text-xs uppercase tracking-widest px-4 py-2"
                  style={{
                    fontFamily: "var(--font-mono)",
                    border: "1px solid var(--color-border)",
                    color: "var(--color-muted-fg)",
                  }}
                >
                  {t("repo.addFirstProject")}
                </button>
              </div>
            ) : (
              repo.projects.map((entry) => (
                <ProjectForm
                  key={entry.id}
                  entry={entry}
                  onChange={(updated) => updateProject(entry.id, updated)}
                  onDelete={() => deleteProject(entry.id)}
                />
              ))
            )}
          </div>
        )}

        {activeSection === "compensation" && (
          <div className="space-y-8">
            <div>
              <Label>{t("repo.employmentStatus")}</Label>
              <select
                value={employmentStatus}
                onChange={(e) =>
                  updateRepo({ employmentStatus: e.target.value })
                }
                className="w-full text-sm px-3 py-2 focus:outline-none"
                style={{
                  border: "1px solid var(--color-border)",
                  backgroundColor: "var(--color-card)",
                  color: "var(--color-fg)",
                  fontFamily: "var(--font-sans)",
                }}
              >
                {EMPLOYMENT_STATUS_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {t(option.key)}
                  </option>
                ))}
              </select>
            </div>

            <div className="grid grid-cols-2 gap-6">
              <div>
                <Label>{t("repo.currentCompensation")}</Label>
                <TextInput
                  value={repo.currentSalary}
                  onChange={(v) => updateRepo({ currentSalary: v })}
                  placeholder={t("repo.currentCompensationPlaceholder")}
                />
                <p
                  className="text-xs mt-1.5 leading-relaxed"
                  style={{
                    fontFamily: "var(--font-mono)",
                    color: "var(--color-muted-fg)",
                  }}
                >
                  {t("repo.compensationNote")}
                </p>
              </div>
              <div>
                <Label>{t("repo.desiredCompensation")}</Label>
                <TextInput
                  value={repo.desiredSalary}
                  onChange={(v) => updateRepo({ desiredSalary: v })}
                  placeholder={t("repo.desiredCompensationPlaceholder")}
                />
                <p
                  className="text-xs mt-1.5 leading-relaxed"
                  style={{
                    fontFamily: "var(--font-mono)",
                    color: "var(--color-muted-fg)",
                  }}
                >
                  {t("repo.desiredCompensationNote")}
                </p>
              </div>
            </div>
          </div>
        )}

        {activeSection === "other" && (
          <div>
            <Label>{t("repo.additionalInfo")}</Label>
            <Field
              rows={14}
              value={repo.additionalInfo}
              onChange={(v) => updateRepo({ additionalInfo: v })}
              placeholder={t("repo.additionalInfoPlaceholder")}
            />
          </div>
        )}
      </div>
    </div>
  )
}

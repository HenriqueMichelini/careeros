import { useState, useCallback, useEffect, useRef } from "react"
import { useI18n, useStore } from "../lib/store"
import { reviewRepository } from "../lib/ai"
import { ProfileReviewError } from "../lib/ai"
import {
  ExperienceEntry,
  ProjectEntry,
  EducationEntry,
  CertificationEntry,
  LanguageEntry,
  ProfessionalRepository,
} from "../lib/types"
import { TranslationKey } from "../lib/i18n"
import { ingestionInputBytes, ingestionMaxBytes, previewValues, applyIngestion, ingestProfile, IngestionError, IngestionOperation, IngestionResult } from "../lib/ingestion"

type Section = "profile" | "goals" | "skills" | "experience" | "projects" | "education" | "certifications" | "languages" | "compensation" | "other"

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
    id: "experience",
    labelKey: "repo.section.experience",
    descKey: "repo.section.experienceDesc",
  },
  {
    id: "projects",
    labelKey: "repo.section.projects",
    descKey: "repo.section.projectsDesc",
  },
  { id: "education", labelKey: "repo.section.education", descKey: "repo.section.educationDesc" },
  { id: "certifications", labelKey: "repo.section.certifications", descKey: "repo.section.certificationsDesc" },
  { id: "languages", labelKey: "repo.section.languages", descKey: "repo.section.languagesDesc" },
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

const destinationKeys: Record<string, TranslationKey> = {
  education: "repo.section.education", certifications: "repo.section.certifications", languages: "repo.section.languages",
  experience: "repo.section.experience", projects: "repo.section.projects", careerGoals: "repo.careerGoals",
  skills: "repo.skills", competencies: "repo.competencies", tools: "repo.tools", additionalInfo: "repo.additionalInfo",
  employmentStatus: "repo.employmentStatus", currentSalary: "repo.currentCompensation", desiredSalary: "repo.desiredCompensation",
  fullName: "repo.fullName", email: "repo.email", phone: "repo.phone", location: "repo.location", professionalLinks: "repo.professionalLinks",
}
function operationFieldKey(op: IngestionOperation): TranslationKey {
  const prefix = {education: "education.", certifications: "certification.", languages: "language."}[op.target as string]
  if (prefix) return ("repo." + prefix + op.field) as TranslationKey
  if (op.field === "current") return "common.current"
  return destinationKeys[op.field] || ("repo." + ({title:"jobTitle", name:"projectName", technologies:"technologiesUsed"}[op.field] || op.field)) as TranslationKey
}

function operationEntryLabel(repo: ProfessionalRepository, operations: IngestionOperation[], op: IngestionOperation): string {
  if (!op.entryId) return ""
  const fields = op.target === "experience" ? ["company", "title"] : op.target === "education" ? ["degree", "institution"] : op.target === "certifications" ? ["name", "issuer"] : ["name"]
  if (op.entryId.startsWith("new:")) return fields.map(field => operations.find(item => item.target === op.target && item.entryId === op.entryId && item.field === field)?.value || "").filter(Boolean).join(" / ")
  const items = repo[op.target as keyof ProfessionalRepository] as unknown as Record<string, unknown>[]
  const item = items.find(item => item.id === op.entryId)
  return item ? fields.map(field => String(item[field] || "")).filter(Boolean).join(" / ") : ""
}

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

function Label({ children, htmlFor }: { children: React.ReactNode; htmlFor?: string }) {
  return (
    <label
      htmlFor={htmlFor}
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
  id,
  value,
  onChange,
  placeholder,
  type = "text",
  maxLength,
}: {
  id?: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
  type?: string
  maxLength?: number
}) {
  return (
    <input
      id={id}
      type={type}
      maxLength={maxLength}
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

function QualificationList<T extends { id: string }>({
  items, fields, title, addLabel, emptyLabel, onAdd, onChange, onDelete,
}: {
  items: T[]
  fields: { key: keyof T & string; label: TranslationKey }[]
  title: string
  addLabel: string
  emptyLabel: string
  onAdd: () => void
  onChange: (item: T) => void
  onDelete: (id: string) => void
}) {
  const { t } = useI18n()
  const [tooLongField, setTooLongField] = useState("")
  const updateField = (item: T, key: keyof T & string, value: string) => {
    if (new TextEncoder().encode(value).length > 2000) {
      setTooLongField(`${item.id}-${key}`)
      return
    }
    setTooLongField("")
    onChange({ ...item, [key]: value })
  }
  return <div>
    <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
      <span className="text-xs uppercase tracking-widest text-[var(--color-muted-fg)]">{title} · {items.length}</span>
      <button type="button" onClick={onAdd} disabled={items.length >= 40} className="bg-[var(--color-fg)] px-4 py-2 text-xs uppercase tracking-widest text-[var(--color-bg)] disabled:cursor-not-allowed disabled:opacity-50">{addLabel}</button>
    </div>
    {items.length >= 40 && <p role="note" className="mb-4 text-xs text-[var(--color-muted-fg)]">{t("repo.qualificationLimit")}</p>}
    {items.length === 0 && <p className="border border-dashed border-[var(--color-border)] px-4 py-8 text-center text-sm text-[var(--color-muted-fg)]">{emptyLabel}</p>}
    {items.map(item => <div key={item.id} className="mb-4 border border-[var(--color-border)] bg-[var(--color-card)] p-4 sm:p-5">
      <div className="mb-4 flex justify-end"><button type="button" onClick={() => onDelete(item.id)} className="text-xs text-[var(--color-accent)]">{t("common.remove")}</button></div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {fields.map(field => <div key={field.key}>
          <Label htmlFor={`${field.key}-${item.id}`}>{t(field.label)}</Label>
          <TextInput id={`${field.key}-${item.id}`} value={String(item[field.key])} maxLength={2000} onChange={value => updateField(item, field.key, value)} />
          {tooLongField === `${item.id}-${field.key}` && <p role="alert" className="mt-1 text-xs text-[var(--color-accent)]">{t("repo.qualificationTooLong")}</p>}
        </div>)}
      </div>
    </div>)}
  </div>
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
  const [ingestionText, setIngestionText] = useState("")
  const [ingestionResult, setIngestionResult] = useState<IngestionResult | null>(null)
  const [ingestionSnapshot, setIngestionSnapshot] = useState("")
  const [ingestionError, setIngestionError] = useState("")
  const [isIngesting, setIsIngesting] = useState(false)
  const ingestionRequest = useRef(0)
  const ingestionController = useRef<AbortController | null>(null)
  const reviewRequest = useRef(0)
  const reviewController = useRef<AbortController | null>(null)
  const repo = state.repository
  const currentReviewContext = useRef({ section: activeSection, repo })
  currentReviewContext.current = { section: activeSection, repo }

  useEffect(() => () => {
    ingestionController.current?.abort()
    reviewRequest.current += 1
    reviewController.current?.abort()
    dispatch({ type: "SET_REVIEWING", payload: false })
  }, [dispatch])

  function changeSection(section: Section) {
    reviewRequest.current += 1
    reviewController.current?.abort()
    currentReviewContext.current.section = section
    dispatch({ type: "SET_REVIEWING", payload: false })
    setReviewError("")
    setActiveSection(section)
  }

  const updateRepo = useCallback(
    (patch: Partial<ProfessionalRepository>) =>
      dispatch({ type: "SET_REPO", payload: { ...repo, ...patch } }),
    [repo, dispatch],
  )

  function discardIngestion() {
    ingestionRequest.current += 1
    ingestionController.current?.abort()
    setIsIngesting(false)
    setIngestionText("")
    setIngestionResult(null)
    setIngestionSnapshot("")
    setIngestionError("")
  }

  function changeIngestionText(value: string) {
    ingestionRequest.current += 1
    ingestionController.current?.abort()
    setIsIngesting(false)
    setIngestionText(value)
    setIngestionResult(null)
    setIngestionError("")
  }

  async function handleIngestion() {
    setIngestionError("")
    setIngestionResult(null)
    if (!state.apiKey) { setIngestionError(t("repo.setApiKeyFirst")); return }
    const requestId = ++ingestionRequest.current
    const controller = new AbortController()
    ingestionController.current = controller
    setIsIngesting(true)
    const snapshot = JSON.stringify(repo)
    try {
      const proposals = await ingestProfile(ingestionText, repo, state.apiKey, controller.signal)
      if (ingestionRequest.current !== requestId) return
      setIngestionSnapshot(snapshot)
      setIngestionResult(proposals)
    } catch (error) {
      if (ingestionRequest.current !== requestId) return
      const code = error instanceof IngestionError ? error.code : "outage"
      const lookup = {
        input:"repo.ingestErrorInput", key:"repo.reviewErrorKey", rate_limit:"repo.reviewErrorRateLimit",
        outage:"repo.reviewErrorOutage", timeout:"repo.reviewErrorTimeout",
        truncated:"repo.ingestErrorTruncated", invalid_output:"repo.ingestErrorInvalidOutput",
      } as const
      setIngestionError(t(lookup[code as keyof typeof lookup] || "repo.reviewFailed"))
    } finally { if (ingestionRequest.current === requestId) setIsIngesting(false) }
  }

  function editOperation(index: number, patch: Partial<IngestionOperation>) {
    setIngestionResult(previous => previous && ({
      ...previous, operations: previous.operations.map((op, i) => i === index ? {...op,...patch} : op),
    }))
  }

  function approveClaim(claimId: string, approved: boolean) {
    setIngestionResult(previous => previous && ({
      ...previous, operations: previous.operations.map(op => op.claimId === claimId ? {...op,approved} : op),
    }))
  }

  function confirmIngestion() {
    if (!ingestionResult?.operations.some(op => op.approved)) return
    try {
      const saved = localStorage.getItem("careeros_repo")
      if (saved && saved !== ingestionSnapshot) throw new IngestionError("stale")
      const updated = applyIngestion(repo, ingestionSnapshot, ingestionResult.operations)
      dispatch({type:"SET_REPO",payload:updated})
      discardIngestion()
    } catch (error) {
      const code = error instanceof IngestionError ? error.code : "invalid_output"
      setIngestionError(t(code === "stale" ? "repo.ingestStale" : "repo.ingestInvalidEdit"))
    }
  }

  async function handleAiReview() {
    if (activeSection === "profile" || currentReviewContext.current.section !== activeSection || state.isReviewingRepo) return
    if (!state.apiKey) {
      setReviewError(t("repo.setApiKeyFirst"))
      return
    }
    setReviewError("")
    const requestId = ++reviewRequest.current
    const controller = new AbortController()
    reviewController.current = controller
    const isCurrent = () => reviewRequest.current === requestId &&
      currentReviewContext.current.section === activeSection && currentReviewContext.current.repo === repo
    dispatch({ type: "SET_REVIEWING", payload: true })
    try {
      const { updatedRepo, summary } = await reviewRepository(
        repo,
        activeSection,
        state.apiKey,
        controller.signal,
      )
      if (!isCurrent()) return
      dispatch({ type: "SET_REPO", payload: updatedRepo })
      dispatch({ type: "SET_REVIEW_SUMMARY", payload: summary })
    } catch (e: any) {
      if (!isCurrent()) return
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
      if (reviewRequest.current === requestId) dispatch({ type: "SET_REVIEWING", payload: false })
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

  function updateEntry<T extends EducationEntry | CertificationEntry | LanguageEntry>(
    section: "education" | "certifications" | "languages", id: string, updated: T,
  ) {
    updateRepo({ [section]: repo[section].map(item => item.id === id ? updated : item) })
  }

  function deleteEntry(section: "education" | "certifications" | "languages", id: string) {
    updateRepo({ [section]: repo[section].filter(item => item.id !== id) })
  }

  const active = SECTIONS.find((s) => s.id === activeSection)!
  const employmentStatus =
    LEGACY_EMPLOYMENT_STATUS[repo.employmentStatus] || repo.employmentStatus

  return (
    <div className="max-w-6xl mx-auto px-4 sm:px-6 py-10 grid grid-cols-1 md:grid-cols-[220px_minmax(0,1fr)] gap-8 md:gap-12">
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
        <nav className="grid grid-cols-3 gap-1 md:block md:space-y-0.5">
          {SECTIONS.map(({ id, labelKey }) => (
            <button
              key={id}
              onClick={() => changeSection(id)}
              className={`w-full text-left px-2 md:px-3 py-2.5 text-xs md:text-sm transition-colors block ${
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
        {activeSection !== "profile" && (
          <button type="button" onClick={() => changeSection("profile")}
            className="mt-5 w-full border px-3 py-2 text-xs text-left uppercase tracking-wide"
            style={{borderColor:"var(--color-accent)",color:"var(--color-accent)"}}>
            {t("repo.quickAdd")}
          </button>
        )}

        {/* AI Review is available only within a supported subsection. */}
        {activeSection !== "profile" && <div
          className="mt-4 md:mt-10 pt-4 md:pt-6 border-t"
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
        </div>}
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
          <div className="space-y-5 min-w-0">
            <section className="p-5 border" style={{backgroundColor:"var(--color-muted)",borderColor:"var(--color-accent)"}}>
              <p className="text-xs mt-2" style={{color:"var(--color-muted-fg)"}}>{t("repo.profileIntro")}</p>
            </section>
            <section className="p-5 border min-w-0" style={{borderColor:"var(--color-border)"}}>
              <h2 className="text-lg font-bold mb-2">{t("repo.contactTitle")}</h2>
              <p className="text-sm mb-4" style={{color:"var(--color-muted-fg)"}}>{t("repo.contactDescription")}</p>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                {([
                  ["fullName", "repo.fullName", "text"],
                  ["email", "repo.email", "email"],
                  ["phone", "repo.phone", "tel"],
                  ["location", "repo.contactLocation", "text"],
                ] as const).map(([field, label, type]) => (
                  <div key={field} className="min-w-0">
                    <Label htmlFor={`contact-${field}`}>{t(label)}</Label>
                    <TextInput id={`contact-${field}`} type={type} value={repo[field]}
                      onChange={value => updateRepo({[field]: value})} />
                  </div>
                ))}
                <div className="min-w-0 sm:col-span-2">
                  <Label htmlFor="contact-professionalLinks">{t("repo.professionalLinks")}</Label>
                  <textarea id="contact-professionalLinks" rows={3} value={repo.professionalLinks}
                    onChange={event => updateRepo({professionalLinks: event.target.value})}
                    placeholder={t("repo.professionalLinksHint")}
                    className="w-full min-w-0 p-3 border text-sm resize-y"
                    style={{backgroundColor:"var(--color-card)",borderColor:"var(--color-border)",color:"var(--color-fg)"}} />
                </div>
              </div>
            </section>
            <section className="p-5 border min-w-0" style={{borderColor:"var(--color-border)"}}>
              <h2 className="text-lg font-bold mb-2">{t("repo.ingestTitle")}</h2>
              <p className="text-sm mb-3">{t("repo.ingestGuide")}</p>
              <p className="text-sm mb-3" style={{color:"var(--color-muted-fg)"}}>{t("repo.ingestPrivacy")}</p>
              <p id="ingestion-limits" className="text-xs mb-3">{t("repo.ingestLimits")}</p>
              <label htmlFor="ingestion-text" className="text-sm block mb-2">{t("repo.ingestLabel")}</label>
              <textarea id="ingestion-text" value={ingestionText} onChange={e => changeIngestionText(e.target.value)} rows={9}
                aria-describedby="ingestion-limits" placeholder={t("repo.ingestPlaceholder")}
                className="w-full min-w-0 p-3 border text-sm resize-y" style={{backgroundColor:"var(--color-card)",borderColor:"var(--color-border)",color:"var(--color-fg)"}} />
              <p className="text-xs mb-3" style={{color:"var(--color-muted-fg)"}}>{ingestionInputBytes(ingestionText)} / 30000 {t("repo.bytes")}</p>
              <div className="flex flex-wrap gap-2">
                <button type="button" onClick={handleIngestion} disabled={isIngesting || !ingestionText.trim() || ingestionInputBytes(ingestionText) > ingestionMaxBytes}
                  className="px-4 py-2 text-sm disabled:opacity-50" style={{backgroundColor:"var(--color-fg)",color:"var(--color-bg)"}}>
                  {isIngesting ? t("repo.ingestWorking") : t("repo.ingestReview")}
                </button>
                <button type="button" onClick={discardIngestion} className="px-4 py-2 text-sm border" style={{borderColor:"var(--color-border)"}}>{t("repo.ingestDiscard")}</button>
              </div>
              {ingestionError && <p role="alert" className="text-sm mt-3" style={{color:"var(--color-accent)"}}>{ingestionError}</p>}
            </section>
            {ingestionResult && <section className="space-y-4" aria-label={t("repo.ingestProposals")}>
              <h2 className="text-xl font-bold">{t("repo.ingestProposals")}</h2>
              <p className="text-sm" style={{color:"var(--color-muted-fg)"}}>{t("repo.ingestReviewNote")}</p>
              <p className="text-xs" role="status">{t("repo.ingestLimits")}</p>
              {(ingestionResult.unverifiedClaimCount > 0 || ingestionResult.unresolvedClaimIds.length > 0 || ingestionResult.unplacedOperationCount > 0) &&
                <p className="text-sm border p-3" role="status" style={{borderColor:"var(--color-border)"}}>{t("repo.ingestPartialNotice")}</p>}
              {ingestionResult.claims.map(claim => {
                const indexed = ingestionResult.operations.map((op,index) => ({op,index})).filter(item => item.op.claimId === claim.id)
                const unresolved = ingestionResult.unresolvedClaimIds.includes(claim.id)
                return <article key={claim.id} className="border p-4 min-w-0" style={{borderColor:"var(--color-border)"}}>
                  <p className="text-sm font-semibold break-words">{claim.text}</p>
                  <p className="text-xs mt-1 break-words" style={{color:"var(--color-muted-fg)"}}>{t("repo.ingestSource")}: “{claim.source}”</p>
                  {claim.question && <p className="text-sm mt-2" role="note">{t("repo.ingestClarify")}: {claim.question}</p>}
                  {unresolved && <p className="text-sm mt-2" role="note">{t("repo.ingestUnresolvedClaim")}</p>}
                  {indexed.length === 0 && !claim.question && !unresolved && <p className="text-sm mt-2">{t("repo.ingestNoChange")}</p>}
                  {indexed.length > 0 && <>
                    <div className="flex flex-wrap gap-2 mt-3">
                      <button type="button" onClick={() => approveClaim(claim.id,true)} className="border px-3 py-1 text-xs" style={{borderColor:"var(--color-border)"}}>{t("repo.ingestApproveClaim")}</button>
                      <button type="button" onClick={() => approveClaim(claim.id,false)} className="border px-3 py-1 text-xs" style={{borderColor:"var(--color-border)"}}>{t("repo.ingestRejectClaim")}</button>
                    </div>
                    {indexed.map(({op,index}) => <div key={index} className="mt-3 p-3 border min-w-0" style={{borderColor:"var(--color-border)"}}>
                      <label className="flex items-start gap-2 text-sm break-words">
                        <input type="checkbox" checked={op.approved} onChange={e => editOperation(index,{approved:e.target.checked})} />
                        {t(("repo.ingestAction."+op.action) as TranslationKey)} · {t(destinationKeys[op.target])}{op.entryId && " / " + operationEntryLabel(repo, ingestionResult.operations, op)} · {t(operationFieldKey(op))} · {t(("repo.ingestFinding."+op.finding) as TranslationKey)}
                      </label>
                      <label className="text-xs block mt-2">{t("repo.ingestAfter")}
                        <textarea value={op.value} onChange={e => editOperation(index,{value:e.target.value})} rows={2}
                          className="w-full p-2 mt-1 border text-sm resize-y" style={{borderColor:"var(--color-border)",backgroundColor:"var(--color-card)",color:"var(--color-fg)"}} />
                      </label>
                      <details className="text-xs mt-2" open={op.action !== "add" || op.finding === "conflict"}>
                        <summary className="cursor-pointer">{t("repo.ingestPreviewField")}</summary>
                        <p className="mt-2 whitespace-pre-wrap break-words">{t("repo.ingestBefore")}: {previewValues(repo,ingestionResult.operations,index).before || "—"}</p>
                        <p className="mt-2 whitespace-pre-wrap break-words">{t("repo.ingestResult")}: {previewValues(repo,ingestionResult.operations,index).after || "—"}</p>
                      </details>
                      {op.action === "remove" && <p className="text-xs mt-1" role="note">{t("repo.ingestRemoval")}</p>}
                    </div>)}
                  </>}
                </article>
              })}
              <div className="flex flex-wrap gap-2">
                <button type="button" onClick={confirmIngestion} disabled={!ingestionResult.operations.some(op => op.approved)}
                  className="px-4 py-2 text-sm disabled:opacity-50" style={{backgroundColor:"var(--color-fg)",color:"var(--color-bg)"}}>{t("repo.ingestApply")}</button>
                <button type="button" onClick={discardIngestion} className="px-4 py-2 border text-sm" style={{borderColor:"var(--color-border)"}}>{t("repo.ingestCancel")}</button>
              </div>
            </section>}
          </div>
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
          <div className="space-y-6">
            <div>
              <Label>{t("repo.skills")}</Label>
              <Field
                rows={12}
                value={repo.skills}
                onChange={(v) => updateRepo({ skills: v })}
                placeholder={t("repo.skillsPlaceholder")}
              />
            </div>
            <div>
              <Label>{t("repo.competencies")}</Label>
              <Field
                rows={10}
                value={repo.competencies}
                onChange={(v) => updateRepo({ competencies: v })}
                placeholder={t("repo.competenciesPlaceholder")}
              />
            </div>
            <div>
              <Label>{t("repo.tools")}</Label>
              <Field
                rows={12}
                value={repo.tools}
                onChange={(v) => updateRepo({ tools: v })}
                placeholder={t("repo.toolsPlaceholder")}
              />
            </div>
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

        {activeSection === "education" && <QualificationList<EducationEntry>
          items={repo.education}
          fields={[
            { key: "degree", label: "repo.education.degree" },
            { key: "institution", label: "repo.education.institution" },
            { key: "location", label: "repo.education.location" },
            { key: "graduationDate", label: "repo.education.graduationDate" },
            { key: "details", label: "repo.education.details" },
          ]}
          title={t("repo.section.education")}
          addLabel={t("repo.education.add")}
          emptyLabel={t("repo.education.empty")}
          onAdd={() => updateRepo({ education: [{ id: uid(), degree: "", institution: "", location: "", graduationDate: "", details: "" }, ...repo.education] })}
          onChange={item => updateEntry("education", item.id, item)}
          onDelete={id => deleteEntry("education", id)}
        />}

        {activeSection === "certifications" && <QualificationList<CertificationEntry>
          items={repo.certifications}
          fields={[
            { key: "name", label: "repo.certification.name" },
            { key: "issuer", label: "repo.certification.issuer" },
            { key: "date", label: "repo.certification.date" },
            { key: "credentialId", label: "repo.certification.credentialId" },
            { key: "url", label: "repo.certification.url" },
          ]}
          title={t("repo.section.certifications")}
          addLabel={t("repo.certification.add")}
          emptyLabel={t("repo.certification.empty")}
          onAdd={() => updateRepo({ certifications: [{ id: uid(), name: "", issuer: "", date: "", credentialId: "", url: "" }, ...repo.certifications] })}
          onChange={item => updateEntry("certifications", item.id, item)}
          onDelete={id => deleteEntry("certifications", id)}
        />}

        {activeSection === "languages" && <QualificationList<LanguageEntry>
          items={repo.languages}
          fields={[
            { key: "name", label: "repo.language.name" },
            { key: "proficiency", label: "repo.language.proficiency" },
          ]}
          title={t("repo.section.languages")}
          addLabel={t("repo.language.add")}
          emptyLabel={t("repo.language.empty")}
          onAdd={() => updateRepo({ languages: [{ id: uid(), name: "", proficiency: "" }, ...repo.languages] })}
          onChange={item => updateEntry("languages", item.id, item)}
          onDelete={id => deleteEntry("languages", id)}
        />}

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
                <option value="">{t("repo.status.unspecified")}</option>
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
            <p className="mb-4 text-sm text-[var(--color-muted-fg)]">{t("repo.otherLegacyNote")}</p>
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

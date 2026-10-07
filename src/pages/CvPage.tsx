import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import {
  CURATED_CV_KEY,
  cvFacts,
  createCuratedCv,
  parseCuratedCv,
  generateCv,
  CvGenerationError,
  type CvResult,
  type CuratedCv,
} from "../lib/cvGeneration"
import CvFontSizeControl from "../components/CvFontSizeControl"
import { useCvFontSize } from "../lib/cvPreferences"
import CvPaper from "../components/CvPaper"
import { useI18n, useStore } from "../lib/store"
import { cvQualifications } from "../lib/profile"
import {
  bulletKey,
  CV_STORAGE_KEY,
  CvChoices,
  CvSection,
  cvSections,
  entryKey,
  parseCvChoices,
  emptyCvChoices,
  professionalLinkHref,
  professionalLinkLabel,
  professionalLinkTarget,
} from "../lib/cv"

function lines(value: string, splitCommas = true) {
  return value
    .split(splitCommas ? /\n|,/ : /\n/)
    .map((part) => part.replace(/^[-•*]\s*/, "").trim())
    .filter(Boolean)
}

interface CvBullet {
  key: string
  source: string
  text: string
}

const A4_WIDTH_PX = (210 * 96) / 25.4
const A4_HEIGHT_PX = (297 * 96) / 25.4

function Section({
  id,
  title,
  children,
  sample = false,
}: {
  id: CvSection
  title: string
  children: React.ReactNode
  sample?: boolean
}) {
  return (
    <section
      data-cv-block={id}
      data-cv-sample={sample || undefined}
      className="mb-7 last:mb-0"
    >
      <h3 className="mb-3 text-[11px] font-bold uppercase tracking-[0.16em] text-[var(--color-accent)]">
        {title}
      </h3>
      {children}
    </section>
  )
}

function CvLink({ value }: { value: string }) {
  const href = professionalLinkHref(professionalLinkTarget(value))
  return href ? (
    <a href={href} className="underline underline-offset-2">
      {professionalLinkLabel(value)}
    </a>
  ) : (
    value
  )
}

export default function CvPage() {
  const typography = useCvFontSize()
  const { state } = useStore()
  const { t } = useI18n()
  const [curated, setCurated] = useState<CuratedCv | null>(() => {
    try {
      return parseCuratedCv(localStorage.getItem(CURATED_CV_KEY))
    } catch {
      return null
    }
  })
  const repo = curated?.repository ?? state.repository
  const { education, certifications, languages } = cvQualifications(repo)
  const previewSlotRef = useRef<HTMLDivElement>(null)
  const boundaryRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  const [previewScale, setPreviewScale] = useState(1)
  const [choices, setChoices] = useState<CvChoices>(() => {
    try {
      return parseCvChoices(
        curated?.choices
          ? JSON.stringify(curated.choices)
          : localStorage.getItem(CV_STORAGE_KEY),
      )
    } catch {
      return parseCvChoices(null)
    }
  })
  const [generation, setGeneration] = useState(false)
  const [generationError, setGenerationError] = useState("")
  const [pending, setPending] = useState<CvResult | null>(null)
  const pendingRevision = useRef("")
  const controller = useRef<AbortController | null>(null)
  const requestId = useRef(0)
  const liveFacts = useMemo(() => cvFacts(state.repository), [state.repository])
  const revision = JSON.stringify([
    state.repository,
    choices,
    curated,
    state.locale,
    state.apiKey,
  ])
  const revisionRef = useRef(revision)
  revisionRef.current = revision
  useEffect(() => {
    controller.current?.abort()
    requestId.current++
    setGeneration(false)
    setPending(null)
  }, [revision])
  useEffect(
    () => () => {
      controller.current?.abort()
      requestId.current++
    },
    [],
  )
  const cancelGeneration = () => {
    controller.current?.abort()
    requestId.current++
    setGeneration(false)
    setPending(null)
  }
  const startGeneration = async () => {
    cancelGeneration()
    const id = ++requestId.current
    const sourceRevision = revisionRef.current
    const abort = new AbortController()
    controller.current = abort
    setGeneration(true)
    setGenerationError("")
    try {
      const result = await generateCv(
        liveFacts,
        state.locale,
        state.apiKey,
        abort.signal,
      )
      if (
        id === requestId.current &&
        sourceRevision === revisionRef.current &&
        !abort.signal.aborted
      ) {
        pendingRevision.current = sourceRevision
        setPending(result)
      }
    } catch (error) {
      if (id === requestId.current && !abort.signal.aborted)
        setGenerationError(
          error instanceof CvGenerationError ? error.code : "outage",
        )
    } finally {
      if (id === requestId.current) setGeneration(false)
    }
  }
  const acceptGeneration = () => {
    if (!pending || pendingRevision.current !== revisionRef.current) {
      setPending(null)
      return
    }
    const saved = createCuratedCv(
      state.repository,
      liveFacts,
      pending,
      state.locale,
    )
    // Preserve the valid previous document if storage cannot accept its replacement.
    const nextChoices = { ...emptyCvChoices(), summary: saved.summary }
    try {
      localStorage.setItem(
        CURATED_CV_KEY,
        JSON.stringify({ ...saved, choices: nextChoices }),
      )
      setCurated(saved)
      setChoices(nextChoices)
      setPending(null)
      setGenerationError("")
    } catch {
      setGenerationError("save")
    }
  }
  const sufficient = liveFacts.some((f) =>
    [
      "skills",
      "competencies",
      "tools",
      "description",
      "responsibilities",
      "achievements",
      "highlights",
      "degree",
      "details",
      "name",
      "proficiency",
    ].includes(f.field),
  )
  const [fit, setFit] = useState({
    overflows: false,
    section: "",
    overflowPercent: 0,
  })
  const [saveError, setSaveError] = useState(false)
  const [exportOverflow, setExportOverflow] = useState(false)
  useEffect(() => {
    document.body.classList.add("cv-print-ready")
    return () => document.body.classList.remove("cv-print-ready")
  }, [])
  const overflows = fit.overflows
  const paperScale = previewScale
  const visible = (section: CvSection) =>
    !choices.hiddenSections.includes(section)
  const selectedEntry = (section: CvSection, id: string) =>
    !choices.hiddenEntries.includes(entryKey(section, id))
  const selectedBullet = (key: string) => !choices.hiddenBullets.includes(key)
  const toggle = (
    field: "hiddenSections" | "hiddenEntries" | "hiddenBullets",
    key: string,
  ) =>
    setChoices((previous) => ({
      ...previous,
      [field]: previous[field].includes(key)
        ? previous[field].filter((item) => item !== key)
        : [...previous[field], key],
    }))
  const setBulletWording = (key: string, text: string) =>
    setChoices((previous) => ({
      ...previous,
      bulletWording: { ...previous.bulletWording, [key]: text },
    }))
  useEffect(() => {
    try {
      if (curated) {
        localStorage.setItem(
          CURATED_CV_KEY,
          JSON.stringify({ ...curated, choices }),
        )
        // Compatibility mirror is secondary; the atomic document is authoritative.
        try {
          localStorage.setItem(CV_STORAGE_KEY, JSON.stringify(choices))
        } catch {
          /* primary document saved */
        }
      } else localStorage.setItem(CV_STORAGE_KEY, JSON.stringify(choices))
      setSaveError(false)
    } catch {
      setSaveError(true)
    }
  }, [choices, curated])
  useEffect(() => setExportOverflow(false), [choices, repo, state.locale])
  const skills = useMemo(() => lines(repo.skills), [repo.skills])
  const competencies = useMemo(
    () => lines(repo.competencies),
    [repo.competencies],
  )
  const tools = useMemo(() => lines(repo.tools), [repo.tools])
  const contactValues: {
    value: string
    kind: "email" | "phone" | "location" | "link"
  }[] = [
    { value: repo.email, kind: "email" },
    { value: repo.phone, kind: "phone" },
    { value: repo.location, kind: "location" },
    ...lines(repo.professionalLinks, false).map((value) => ({
      value,
      kind: "link" as const,
    })),
  ]
  const contact = contactValues
    .map((item, index) => ({
      value: item.value.trim(),
      id: JSON.stringify([index, item.value.trim()]),
      kind: item.kind,
    }))
    .filter((item) => item.value)
  const textKey = (
    section: CvSection,
    field: string,
    index: number,
    value: string,
  ) => entryKey(section, JSON.stringify([field, index, value]))
  const selectedTexts = (section: CvSection, field: string, values: string[]) =>
    values.filter((value, index) =>
      selectedEntry(section, JSON.stringify([field, index, value])),
    )
  const selectedSkills = selectedTexts("skills", "skills", skills)
  const selectedCompetencies = selectedTexts(
    "skills",
    "competencies",
    competencies,
  )
  const selectedTools = selectedTexts("tools", "tools", tools)
  const experience = repo.experience.filter((item) =>
    [
      item.title,
      item.company,
      item.startDate,
      item.endDate,
      item.location,
      item.description,
      item.responsibilities,
      item.achievements,
    ].some(Boolean),
  )
  const projects = repo.projects.filter((item) =>
    [
      item.name,
      item.description,
      item.technologies,
      item.url,
      item.highlights,
    ].some(Boolean),
  )
  const bulletLines = (
    section: CvSection,
    id: string,
    field: string,
    source: string,
  ) =>
    lines(source, false).map((line, index) => {
      const key = bulletKey(section, id, field, index, line)
      return { key, source: line, text: choices.bulletWording[key] ?? line }
    })
  const selectedWording = (
    section: CvSection,
    id: string,
    field: string,
    source: string,
  ) =>
    bulletLines(section, id, field, source)
      .filter((b) => selectedBullet(b.key))
      .map((b) => b.text)
      .join("\n")
  const selectedExperience = experience
    .filter((item) => selectedEntry("experience", item.id))
    .map((item) => ({
      item: {
        ...item,
        description: selectedWording(
          "experience",
          item.id,
          "description",
          item.description,
        ),
      },
      bullets: [
        ...bulletLines(
          "experience",
          item.id,
          "responsibilities",
          item.responsibilities,
        ),
        ...bulletLines(
          "experience",
          item.id,
          "achievements",
          item.achievements,
        ),
      ].filter((bullet) => selectedBullet(bullet.key) && bullet.text.trim()),
    }))
  const selectedProjects = projects
    .filter((item) => selectedEntry("projects", item.id))
    .map((project) => ({
      project: {
        ...project,
        description: selectedWording(
          "projects",
          project.id,
          "description",
          project.description,
        ),
        technologies: selectedWording(
          "projects",
          project.id,
          "technologies",
          project.technologies,
        ),
      },
      bullets: bulletLines(
        "projects",
        project.id,
        "highlights",
        project.highlights,
      ).filter((bullet) => selectedBullet(bullet.key) && bullet.text.trim()),
    }))
  const technicalSkills = [
    ...(visible("skills") ? [...selectedSkills, ...selectedCompetencies] : []),
    ...(visible("tools") ? selectedTools : []),
  ]
  const sampleTechnicalSkills =
    !curated && visible("skills") && !skills.length && !competencies.length && !tools.length
  const sampleExperience =
    !curated && visible("experience") && !experience.length && !projects.length
  const showExperience =
    sampleExperience ||
    (visible("experience") && selectedExperience.length > 0) ||
    (visible("projects") && selectedProjects.length > 0)
  const summary = choices.summary ?? curated?.summary ?? repo.careerGoals
  const sectionLabels: Record<CvSection, string> = {
    contact: t("cv.contact"),
    summary: t("cv.professionalProfile"),
    skills: t("cv.skillsCompetencies"),
    experience: t("cv.experience"),
    education: t("cv.education"),
    certifications: t("cv.certifications"),
    languages: t("cv.languages"),
    tools: t("cv.toolsTechnology"),
    projects: t("cv.selectedProjects"),
    additional: t("cv.additional"),
  }
  const summaryWords = summary.trim() ? summary.trim().split(/\s+/).length : 0
  const exportPdf = async () => {
    const content = contentRef.current
    if (!content) return
    await document.fonts.ready
    const copy = content.cloneNode(true) as HTMLDivElement
    copy.classList.add("cv-export-measure")
    copy
      .querySelectorAll("[data-cv-sample]")
      .forEach((sample) => sample.remove())
    document.body.append(copy)
    const contentHeight = copy.getBoundingClientRect().height
    copy.remove()
    if (contentHeight > A4_HEIGHT_PX - 4) {
      setExportOverflow(true)
      return
    }
    setExportOverflow(false)
    window.print()
  }
  const textControls = (section: CvSection, field: string, values: string[]) =>
    values.map((value, index) => (
      <label
        key={textKey(section, field, index, value)}
        className="ml-5 flex gap-2 py-1 text-xs leading-5"
      >
        <input
          type="checkbox"
          checked={selectedEntry(
            section,
            JSON.stringify([field, index, value]),
          )}
          onChange={() =>
            toggle("hiddenEntries", textKey(section, field, index, value))
          }
        />
        <span className="break-words">{value}</span>
      </label>
    ))
  const entryControls = (
    section: CvSection,
    id: string,
    label: string,
    bullets: CvBullet[] = [],
  ) => (
    <div
      key={entryKey(section, id)}
      className="ml-5 border-l border-[var(--color-border)] pl-3"
    >
      <label className="flex gap-2 py-1 text-xs leading-5">
        <input
          type="checkbox"
          checked={selectedEntry(section, id)}
          onChange={() => toggle("hiddenEntries", entryKey(section, id))}
        />
        <span className="break-words">{label}</span>
      </label>
      {selectedEntry(section, id) &&
        bullets.map((bullet) => (
          <div key={bullet.key} className="ml-4 mb-2">
            <label className="flex gap-2 text-xs leading-5">
              <input
                type="checkbox"
                checked={selectedBullet(bullet.key)}
                onChange={() => toggle("hiddenBullets", bullet.key)}
              />
              <span className="break-words">{bullet.source}</span>
            </label>
            {selectedBullet(bullet.key) && (
              <div className="ml-5 mt-1">
                <textarea
                  aria-label={`${t("cv.editBullet")}: ${bullet.source}`}
                  rows={2}
                  value={bullet.text}
                  onChange={(event) =>
                    setBulletWording(bullet.key, event.target.value)
                  }
                  className="w-full resize-y border border-[var(--color-border)] bg-[var(--color-card)] p-2 text-xs"
                />
                <p className="text-[10px] text-[var(--color-muted-fg)]">
                  {t("cv.bulletGuidance", { count: bullet.text.length })}
                  {bullet.text.length > 160
                    ? ` · ${t("cv.lengthSuggestion")}`
                    : ""}
                </p>
                {choices.bulletWording[bullet.key] !== undefined && (
                  <button
                    type="button"
                    className="text-[10px] underline"
                    onClick={() =>
                      setChoices((previous) => {
                        const bulletWording = { ...previous.bulletWording }
                        delete bulletWording[bullet.key]
                        return { ...previous, bulletWording }
                      })
                    }
                  >
                    {t(curated ? "cv.resetToSnapshot" : "cv.resetToProfile")}
                  </button>
                )}
              </div>
            )}
          </div>
        ))}
    </div>
  )

  useLayoutEffect(() => {
    const slot = previewSlotRef.current
    if (!slot) return
    const measure = () => {
      const next = Math.min(1, slot.getBoundingClientRect().width / A4_WIDTH_PX)
      setPreviewScale((previous) =>
        Math.abs(previous - next) < 0.001 ? previous : next,
      )
    }
    const observer = new ResizeObserver(measure)
    observer.observe(slot)
    measure()
    return () => observer.disconnect()
  }, [])

  useLayoutEffect(() => {
    const boundary = boundaryRef.current
    const content = contentRef.current
    if (!boundary || !content) return
    const measure = () => {
      const pageBottom = boundary.getBoundingClientRect().bottom
      const contentBottom = content.getBoundingClientRect().bottom
      const overflowHeight = Math.max(0, contentBottom - pageBottom)
      const overflowPercent = Math.ceil(
        (overflowHeight / boundary.getBoundingClientRect().height) * 100,
      )
      const blocks = Array.from(
        content.querySelectorAll<HTMLElement>("[data-cv-block]"),
      )
      const first = blocks.find(
        (block) =>
          block.getBoundingClientRect().bottom > pageBottom + 2 * paperScale,
      )
      // Bottom padding can cross the boundary even when the last section does not.
      const next = {
        overflows: overflowHeight > 2 * paperScale,
        section: (first || blocks.at(-1))?.dataset.cvBlock || "header",
        overflowPercent,
      }
      setFit((previous) =>
        previous.overflows === next.overflows &&
        previous.section === next.section &&
        previous.overflowPercent === next.overflowPercent
          ? previous
          : next,
      )
    }
    const observer = new ResizeObserver(measure)
    observer.observe(boundary)
    observer.observe(content)
    measure()
    return () => observer.disconnect()
  }, [choices, state.locale, repo, paperScale, typography.fontSize])

  return (
    <div className="cv-page mx-auto max-w-6xl px-4 py-10 sm:px-6">
      <header className="cv-page-header mb-9">
        <p className="mb-3 text-xs uppercase tracking-[0.25em] text-[var(--color-muted-fg)] [font-family:var(--font-mono)]">
          {t("cv.step")}
        </p>
        <h1 className="text-5xl font-bold uppercase leading-[0.9] tracking-tight sm:text-6xl [font-family:var(--font-display)]">
          {t("cv.titleYourCv")}
          <br />
          <span className="text-[var(--color-accent)]">
            {t("cv.titleYourTemplate")}
          </span>
        </h1>
        <p className="mt-4 max-w-xl text-sm leading-relaxed text-[var(--color-muted-fg)]">
          {t("cv.intro")}
        </p>
      </header>
      <div className="cv-page-grid grid items-start gap-8 lg:grid-cols-[minmax(0,1fr)_330px]">
        <div
          ref={previewSlotRef}
          className="cv-preview-slot order-2 min-w-0 overflow-x-auto lg:order-1"
        >
          <CvPaper
            fontSize={typography.fontSize}
            label={t("cv.documentPreview")}
            scale={paperScale}
            overflows={overflows}
            pageEndLabel={t("cv.pageOneEnds")}
            boundaryRef={boundaryRef}
            contentRef={contentRef}
          >
            <header className="mb-7 border-b-2 border-[var(--color-accent)] pb-6">
              {!curated && !repo.fullName.trim() && (
                <p
                  data-cv-sample="true"
                  className="mb-2 text-[10px] font-semibold uppercase tracking-widest text-[var(--color-muted-fg)]"
                >
                  {t("common.sample")}
                </p>
              )}
              <h2
                data-cv-sample={(!curated && !repo.fullName.trim()) || undefined}
                className="break-words text-5xl font-bold uppercase leading-none tracking-tight [font-family:var(--font-display)]"
              >
                {repo.fullName.trim() || (curated ? "" : t("cv.sampleName"))}
              </h2>
              {visible("contact") &&
                contact.some((item) => selectedEntry("contact", item.id)) && (
                  <ul className="mt-4 flex flex-wrap gap-x-2 text-[11px] leading-5 text-[var(--color-muted-fg)] [font-family:var(--font-mono)]">
                    {contact
                      .filter((item) => selectedEntry("contact", item.id))
                      .map((item) => (
                        <li
                          key={item.id}
                          className="break-all after:ml-2 after:content-['·'] last:after:content-none"
                        >
                          {item.kind === "link" ? (
                            <CvLink value={item.value} />
                          ) : (
                            item.value
                          )}
                        </li>
                      ))}
                  </ul>
                )}
            </header>
            {visible("summary") && (summary.trim() || !curated) && (
              <Section
                id="summary"
                title={t("cv.professionalProfile")}
                sample={!summary.trim()}
              >
                <p className="whitespace-pre-wrap break-words text-[13px] leading-6">
                  {summary.trim() ||
                    (choices.summary === null && !repo.careerGoals.trim() && (
                      <>
                        <span className="text-[10px] uppercase tracking-widest text-[var(--color-muted-fg)]">
                          {t("common.sample")} ·{" "}
                        </span>
                        {t("cv.sampleProfile")}
                      </>
                    ))}
                </p>
              </Section>
            )}
            {(technicalSkills.length > 0 || sampleTechnicalSkills) && (
              <Section
                id="skills"
                title={t("cv.skillsCompetencies")}
                sample={sampleTechnicalSkills}
              >
                {technicalSkills.length > 0 ? (
                  <ul className="list-disc space-y-1 pl-5 text-[12px] leading-5">
                    {technicalSkills.map((skill, index) => (
                      <li key={`${index}-${skill}`} className="break-words">
                        {skill}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-[12px] leading-5">
                    <span className="text-[10px] uppercase tracking-widest text-[var(--color-muted-fg)]">
                      {t("common.sample")} ·{" "}
                    </span>
                    {t("cv.sampleSkills")}
                  </p>
                )}
              </Section>
            )}
            {showExperience && (
              <Section
                id="experience"
                title={t("cv.experience")}
                sample={sampleExperience}
              >
                {visible("experience") && selectedExperience.length > 0 ? (
                  selectedExperience.map(({ item, bullets }) => (
                    <div
                      key={item.id}
                      className="mb-5 break-words text-[12px] leading-5 last:mb-0"
                    >
                      {(item.title || item.company) && (
                        <h4 className="text-[13px] font-semibold">
                          {[item.title, item.company]
                            .filter(Boolean)
                            .join(" · ")}
                        </h4>
                      )}
                      {(item.startDate ||
                        item.endDate ||
                        item.current ||
                        item.location) && (
                        <p className="mt-1 text-[11px] text-[var(--color-muted-fg)]">
                          {[
                            item.startDate && (item.endDate || item.current)
                              ? `${item.startDate} — ${
                                  item.current
                                    ? t("common.present")
                                    : item.endDate
                                }`
                              : item.startDate ||
                                (item.current
                                  ? t("common.present")
                                  : item.endDate),
                            item.location,
                          ]
                            .filter(Boolean)
                            .join(" · ")}
                        </p>
                      )}
                      {item.description && (
                        <p className="mt-2 whitespace-pre-wrap">
                          {item.description}
                        </p>
                      )}
                      {bullets.length > 0 && (
                        <ul className="mt-2 list-disc space-y-1 pl-5">
                          {bullets.map((bullet) => (
                            <li key={bullet.key}>{bullet.text}</li>
                          ))}
                        </ul>
                      )}
                    </div>
                  ))
                ) : sampleExperience ? (
                  <div className="text-[12px] leading-5">
                    <p className="mb-2 text-[10px] uppercase tracking-widest text-[var(--color-muted-fg)]">
                      {t("common.sampleContent")}
                    </p>
                    <h4 className="font-semibold">{t("cv.sampleJobTitle")}</h4>
                    <p className="mt-1 text-[var(--color-muted-fg)]">
                      2021 — {t("common.present")}
                    </p>
                    <p className="mt-2">
                      {t("cv.sampleExperienceDescription")}
                    </p>
                    <ul className="mt-2 list-disc space-y-1 pl-5">
                      <li>{t("cv.sampleAchievementOne")}</li>
                      <li>{t("cv.sampleAchievementTwo")}</li>
                    </ul>
                  </div>
                ) : null}
                {visible("projects") &&
                  selectedProjects.map(({ project, bullets }) => (
                    <div
                      key={project.id}
                      className="mb-4 break-words text-[12px] leading-5 last:mb-0"
                    >
                      {project.name && (
                        <h4 className="text-[13px] font-semibold">
                          {project.name}
                        </h4>
                      )}
                      {project.description && (
                        <p className="mt-1 whitespace-pre-wrap">
                          {project.description}
                        </p>
                      )}
                      {bullets.length > 0 && (
                        <ul className="mt-1 list-disc space-y-1 pl-5">
                          {bullets.map((bullet) => (
                            <li key={bullet.key}>{bullet.text}</li>
                          ))}
                        </ul>
                      )}
                      {project.technologies && (
                        <p className="mt-1 text-[var(--color-muted-fg)]">
                          {project.technologies}
                        </p>
                      )}
                      {project.url && (
                        <p className="mt-1 text-[var(--color-muted-fg)]">
                          <CvLink value={project.url} />
                        </p>
                      )}
                    </div>
                  ))}
              </Section>
            )}
            {visible("education") &&
              education.some((item) => selectedEntry("education", item.id)) && (
                <Section id="education" title={t("cv.education")}>
                  {education
                    .filter((item) => selectedEntry("education", item.id))
                    .map((item) => (
                      <div
                        key={item.id}
                        className="mb-4 break-words text-[12px] leading-5 last:mb-0"
                      >
                        {item.degree && (
                          <h4 className="text-[13px] font-semibold">
                            {item.degree}
                          </h4>
                        )}
                        {[
                          item.institution,
                          item.location,
                          item.graduationDate,
                        ].filter(Boolean).length > 0 && (
                          <p className="mt-1 text-[var(--color-muted-fg)]">
                            {[
                              item.institution,
                              item.location,
                              item.graduationDate,
                            ]
                              .filter(Boolean)
                              .join(" · ")}
                          </p>
                        )}
                        {item.details && (
                          <p className="mt-1 whitespace-pre-wrap">
                            {item.details}
                          </p>
                        )}
                      </div>
                    ))}
                </Section>
              )}
            {visible("certifications") &&
              certifications.some((item) =>
                selectedEntry("certifications", item.id),
              ) && (
                <Section id="certifications" title={t("cv.certifications")}>
                  {certifications
                    .filter((item) => selectedEntry("certifications", item.id))
                    .map((item) => (
                      <div
                        key={item.id}
                        className="mb-4 break-words text-[12px] leading-5 last:mb-0"
                      >
                        <h4 className="text-[13px] font-semibold">
                          {item.name}
                        </h4>
                        {[item.issuer, item.date, item.credentialId].filter(
                          Boolean,
                        ).length > 0 && (
                          <p className="mt-1 text-[var(--color-muted-fg)]">
                            {[item.issuer, item.date, item.credentialId]
                              .filter(Boolean)
                              .join(" · ")}
                          </p>
                        )}
                        {item.url && (
                          <p className="mt-1 break-all text-[var(--color-muted-fg)]">
                            <CvLink value={item.url} />
                          </p>
                        )}
                      </div>
                    ))}
                </Section>
              )}
            {visible("languages") &&
              languages.some((item) => selectedEntry("languages", item.id)) && (
                <Section id="languages" title={t("cv.languages")}>
                  <ul className="space-y-1 text-[12px] leading-5">
                    {languages
                      .filter((item) => selectedEntry("languages", item.id))
                      .map((item) => (
                        <li key={item.id} className="break-words">
                          {[item.name, item.proficiency]
                            .filter(Boolean)
                            .join(" · ")}
                        </li>
                      ))}
                  </ul>
                </Section>
              )}
            {visible("additional") && repo.additionalInfo.trim() && (
              <Section id="additional" title={t("cv.additional")}>
                <p className="whitespace-pre-line break-words text-[12px] leading-5 text-[var(--color-muted-fg)]">
                  {repo.additionalInfo}
                </p>
              </Section>
            )}
          </CvPaper>
        </div>
        <aside className="cv-controls order-1 lg:order-2 lg:sticky lg:top-24">
          <div
            className="mb-4 border border-[var(--color-border)] bg-[var(--color-card)] p-5"
            data-cv-generation
          >
            <h2 className="mb-2 text-xs font-semibold uppercase">
              {t("cv.generateTitle")}
            </h2>
            <p className="mb-3 text-xs leading-5">{t("cv.generatePolicy")}</p>
            <details className="mb-3 text-xs leading-5">
              <summary>{t("cv.generatePrivacyTitle")}</summary>
              <p>{t("cv.generatePrivacy")}</p>
            </details>
            {!sufficient && (
              <p className="mb-3 text-xs">{t("cv.generateInsufficient")}</p>
            )}
            {!state.apiKey && (
              <p className="mb-3 text-xs">{t("cv.generateKey")}</p>
            )}
            {curated && (
              <p className="mb-3 text-xs">{t("cv.generatedSnapshot")}</p>
            )}
            <button
              type="button"
              disabled={!sufficient || !state.apiKey || generation}
              onClick={startGeneration}
              className="w-full border border-[var(--color-border)] p-2 text-xs disabled:opacity-50"
            >
              {t(generation ? "cv.generating" : "cv.generate")}
            </button>
            {generation && (
              <p role="status" className="mt-2 text-xs">
                {t("cv.generateProgress")}
              </p>
            )}
            {(generation || pending) && (
              <button
                type="button"
                onClick={cancelGeneration}
                className="mt-2 text-xs underline"
              >
                {t("cv.generateCancel")}
              </button>
            )}
            {generationError && (
              <p role="alert" className="mt-3 text-xs">
                {t(
                  ({
                    input: "cv.generateErrorInput",
                    key: "cv.generateKey",
                    rate_limit: "cv.generateErrorRate",
                    timeout: "cv.generateErrorTimeout",
                    invalid_output: "cv.generateErrorOutput",
                    save: "cv.saveError",
                  } as const)[(generationError as "input")] ||
                    "cv.generateErrorOutage",
                )}
              </p>
            )}
            {pending && (
              <div className="mt-4 border-t border-[var(--color-border)] pt-3">
                <p className="mb-2 text-xs font-semibold">
                  {t("cv.generateReview")}
                </p>
                <p className="mb-3 text-xs leading-5" data-generated-summary>
                  {pending.summary.map((s) => s.text).join(" ")}
                </p>
                <details className="mb-3 text-xs leading-5">
                  <summary>
                    {t("cv.generateSources", {
                      count: pending.selected.length,
                    })}
                  </summary>
                  <ul className="list-disc pl-4">
                    {liveFacts
                      .filter((f) => pending.selected.includes(f.id))
                      .map((f) => (
                        <li key={f.id}>
                          {sectionLabels[(f.section as CvSection)]}: {f.text}
                          {pending.wording?.[f.id] && (
                            <p className="mt-1 font-semibold">
                              {t("cv.proposedWording")}: {pending.wording[f.id]}
                            </p>
                          )}
                          {pending.summary
                            .filter((s) => s.sourceId === f.id)
                            .map((s, index) => (
                              <p key={index} className="mt-1 font-semibold">
                                {t("cv.professionalProfile")}: {s.text}
                              </p>
                            ))}
                        </li>
                      ))}
                  </ul>
                </details>
                <p className="mb-3 text-xs leading-5">
                  {t("cv.generateReplaceNote")}
                </p>
                <button
                  type="button"
                  onClick={acceptGeneration}
                  className="w-full bg-[var(--color-accent)] p-2 text-xs text-white"
                >
                  {t("cv.generateAccept")}
                </button>
              </div>
            )}
          </div>
          <CvFontSizeControl
            fontSize={typography.fontSize}
            onChange={typography.setFontSize}
            saveError={typography.saveError}
          />
          <div className="mb-4 border border-[var(--color-border)] bg-[var(--color-card)] p-5">
            <h2 className="mb-2 text-xs uppercase tracking-[0.2em] [font-family:var(--font-mono)]">
              {t("cv.curation")}
            </h2>
            <p className="mb-4 text-xs leading-5 text-[var(--color-muted-fg)]">
              {t("cv.curationIntro")}
            </p>
            <p className="mb-4 text-xs leading-5 text-[var(--color-muted-fg)]">
              {t("cv.groupingNote")}
            </p>
            <p
              role="status"
              className={
                overflows
                  ? "mb-4 border-l-2 border-[var(--color-accent)] pl-3 text-xs leading-5"
                  : "mb-4 text-xs leading-5"
              }
            >
              {overflows
                ? t("cv.fitAttention", {
                    section:
                      fit.section === "header"
                        ? t("cv.header")
                        : sectionLabels[(fit.section as CvSection)] ||
                          t("cv.header"),
                    percent: fit.overflowPercent,
                  })
                : t("cv.fits")}
            </p>
            {saveError && (
              <p
                role="alert"
                className="mb-4 text-xs leading-5 text-[var(--color-accent)]"
              >
                {t("cv.saveError")}
              </p>
            )}
            <div className="space-y-3">
              {cvSections.map((section) => (
                <div
                  key={section}
                  className="border-t border-[var(--color-border)] pt-2"
                >
                  <label className="flex gap-2 text-xs font-semibold leading-5">
                    <input
                      type="checkbox"
                      checked={visible(section)}
                      onChange={() => toggle("hiddenSections", section)}
                    />
                    {sectionLabels[section]}
                  </label>
                  {section === "summary" && visible(section) && (
                    <div className="ml-5 mt-2">
                      <textarea
                        aria-label={t("cv.editSummary")}
                        rows={4}
                        value={summary}
                        onChange={(event) =>
                          setChoices((previous) => ({
                            ...previous,
                            summary: event.target.value,
                          }))
                        }
                        className="w-full resize-y border border-[var(--color-border)] bg-[var(--color-card)] p-2 text-xs"
                      />
                      <p className="text-[10px] text-[var(--color-muted-fg)]">
                        {t("cv.summaryGuidance", { count: summaryWords })}
                        {summaryWords > 80
                          ? ` · ${t("cv.lengthSuggestion")}`
                          : ""}
                      </p>
                      {choices.summary !== null && (
                        <button
                          type="button"
                          className="text-[10px] underline"
                          onClick={() =>
                            setChoices((previous) => ({
                              ...previous,
                              summary: null,
                            }))
                          }
                        >
                          {t(
                            curated
                              ? "cv.resetToSnapshot"
                              : "cv.resetToProfile",
                          )}
                        </button>
                      )}
                    </div>
                  )}
                  {visible(section) &&
                    section === "contact" &&
                    contact.map((item) => (
                      <label
                        key={item.id}
                        className="ml-5 flex gap-2 py-1 text-xs leading-5"
                      >
                        <input
                          type="checkbox"
                          checked={selectedEntry(section, item.id)}
                          onChange={() =>
                            toggle("hiddenEntries", entryKey(section, item.id))
                          }
                        />
                        <span className="break-all">{item.value}</span>
                      </label>
                    ))}
                  {visible(section) && section === "skills" && (
                    <>
                      {textControls(section, "skills", skills)}
                      {textControls(section, "competencies", competencies)}
                    </>
                  )}
                  {visible(section) &&
                    section === "tools" &&
                    textControls(section, "tools", tools)}
                  {visible(section) &&
                    section === "experience" &&
                    experience.map((item) =>
                      entryControls(
                        section,
                        item.id,
                        [item.title, item.company]
                          .filter(Boolean)
                          .join(" · ") || t("cv.experience"),
                        [
                          ...bulletLines(
                            section,
                            item.id,
                            "description",
                            item.description,
                          ),
                          ...bulletLines(
                            section,
                            item.id,
                            "responsibilities",
                            item.responsibilities,
                          ),
                          ...bulletLines(
                            section,
                            item.id,
                            "achievements",
                            item.achievements,
                          ),
                        ],
                      ),
                    )}
                  {visible(section) &&
                    section === "projects" &&
                    projects.map((item) =>
                      entryControls(
                        section,
                        item.id,
                        item.name || t("cv.selectedProjects"),
                        [
                          ...bulletLines(
                            section,
                            item.id,
                            "description",
                            item.description,
                          ),
                          ...bulletLines(
                            section,
                            item.id,
                            "technologies",
                            item.technologies,
                          ),
                          ...bulletLines(
                            section,
                            item.id,
                            "highlights",
                            item.highlights,
                          ),
                        ],
                      ),
                    )}
                  {visible(section) &&
                    section === "education" &&
                    education.map((item) =>
                      entryControls(
                        section,
                        item.id,
                        [item.degree, item.institution]
                          .filter(Boolean)
                          .join(" · "),
                      ),
                    )}
                  {visible(section) &&
                    section === "certifications" &&
                    certifications.map((item) =>
                      entryControls(section, item.id, item.name),
                    )}
                  {visible(section) &&
                    section === "languages" &&
                    languages.map((item) =>
                      entryControls(section, item.id, item.name),
                    )}
                </div>
              ))}
            </div>
            <p className="mt-4 text-[10px] leading-5 text-[var(--color-muted-fg)]">
              {t("cv.cvOnlyNote")}
            </p>
          </div>
          <div className="mb-4 border border-[var(--color-border)] bg-[var(--color-card)] p-5">
            <button
              type="button"
              onClick={exportPdf}
              className="w-full bg-[var(--color-fg)] px-4 py-3 text-xs font-semibold uppercase tracking-widest text-[var(--color-card)]"
            >
              {t("cv.exportPdf")}
            </button>
            <p className="mt-3 text-xs leading-5 text-[var(--color-muted-fg)]">
              {t("cv.exportHelp")}
            </p>
            {exportOverflow && (
              <p
                role="alert"
                className="mt-3 border-l-2 border-[var(--color-accent)] pl-3 text-xs leading-5"
              >
                {t("cv.exportOverflow")}
              </p>
            )}
            <p className="mt-3 text-[10px] leading-5 text-[var(--color-muted-fg)]">
              {t("cv.parserNote")}
            </p>
          </div>
          <div className="border border-[var(--color-border)] bg-[var(--color-card)] p-5">
            <p className="mb-4 text-xs uppercase tracking-[0.2em] text-[var(--color-muted-fg)] [font-family:var(--font-mono)]">
              {t("cv.previewSettings")}
            </p>
            <dl className="space-y-4 text-xs">
              <div>
                <dt className="mb-1 text-[var(--color-muted-fg)]">
                  {t("cv.template")}
                </dt>
                <dd>{t("cv.atsCv")}</dd>
              </div>
              <div>
                <dt className="mb-1 text-[var(--color-muted-fg)]">
                  {t("cv.pageFormat")}
                </dt>
                <dd>{overflows ? t("cv.a4Overflow") : t("cv.a4Pages")}</dd>
              </div>
              <div>
                <dt className="mb-2 text-[var(--color-muted-fg)]">
                  {t("cv.accentColor")}
                </dt>
                <dd className="flex items-center gap-2">
                  <span className="inline-block h-4 w-4 border border-[var(--color-border)] bg-[var(--color-accent)]" />
                  {t("cv.red")}
                </dd>
              </div>
            </dl>
            <p className="mt-4 text-[10px] leading-5 text-[var(--color-muted-fg)]">
              {t("cv.settingsNote")}
            </p>
          </div>
          <div className="mt-4 border border-[var(--color-border)] p-5">
            <p className="mb-3 text-xs uppercase tracking-[0.2em] text-[var(--color-muted-fg)] [font-family:var(--font-mono)]">
              {t("cv.profileCoverage")}
            </p>
            {[
              [t("cv.experience"), experience.length > 0, true],
              [t("repo.section.skills"), skills.length > 0, true],
              [t("repo.section.projects"), projects.length > 0, false],
              [t("cv.education"), education.length > 0, false],
              [t("cv.certifications"), certifications.length > 0, false],
              [t("cv.languages"), languages.length > 0, false],
            ].map(([label, complete, hasSample]) => (
              <div
                key={label as string}
                className="flex justify-between border-b border-[var(--color-border)] py-2 text-xs"
              >
                <span>{label}</span>
                <span
                  className={complete ? "" : "text-[var(--color-muted-fg)]"}
                >
                  {complete
                    ? t("cv.added")
                    : hasSample
                      ? t("common.sampleContent")
                      : t("cv.notAdded")}
                </span>
              </div>
            ))}
            <p className="mt-3 text-[10px] leading-5 text-[var(--color-muted-fg)]">
              {t("cv.editProfileNote")}
            </p>
          </div>
        </aside>
      </div>
    </div>
  )
}

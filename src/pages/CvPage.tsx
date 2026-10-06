import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
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

function Section({
  id,
  title,
  children,
}: {
  id: CvSection
  title: string
  children: React.ReactNode
}) {
  return (
    <section data-cv-block={id} className="mb-7 last:mb-0">
      <h3 className="mb-3 text-[11px] font-bold uppercase tracking-[0.16em] text-[var(--color-accent)]">
        {title}
      </h3>
      {children}
    </section>
  )
}

export default function CvPage() {
  const { state } = useStore()
  const { t } = useI18n()
  const repo = state.repository
  const { education, certifications, languages } = cvQualifications(repo)
  const previewSlotRef = useRef<HTMLDivElement>(null)
  const boundaryRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  const [previewScale, setPreviewScale] = useState(1)
  const [choices, setChoices] = useState<CvChoices>(() => {
    try {
      return parseCvChoices(localStorage.getItem(CV_STORAGE_KEY))
    } catch {
      return parseCvChoices(null)
    }
  })
  const [fit, setFit] = useState({
    overflows: false,
    section: "",
    overflowPercent: 0,
  })
  const [saveError, setSaveError] = useState(false)
  const overflows = fit.overflows
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
      localStorage.setItem(CV_STORAGE_KEY, JSON.stringify(choices))
      setSaveError(false)
    } catch {
      setSaveError(true)
    }
  }, [choices])
  const skills = useMemo(() => lines(repo.skills), [repo.skills])
  const competencies = useMemo(
    () => lines(repo.competencies),
    [repo.competencies],
  )
  const tools = useMemo(() => lines(repo.tools), [repo.tools])
  const contact = [
    repo.email,
    repo.phone,
    repo.location,
    ...lines(repo.professionalLinks, false),
  ]
    .map((value, index) => ({
      value: value.trim(),
      id: JSON.stringify([index, value.trim()]),
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
  const selectedExperience = experience
    .filter((item) => selectedEntry("experience", item.id))
    .map((item) => ({
      item,
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
      project,
      bullets: bulletLines(
        "projects",
        project.id,
        "highlights",
        project.highlights,
      ).filter((bullet) => selectedBullet(bullet.key) && bullet.text.trim()),
    }))
  const summary = choices.summary ?? repo.careerGoals
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
                    {t("cv.resetToProfile")}
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
          block.getBoundingClientRect().bottom > pageBottom + 2 * previewScale,
      )
      // Bottom padding can cross the boundary even when the last section does not.
      const next = {
        overflows: overflowHeight > 2 * previewScale,
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
  }, [choices, state.locale, repo, previewScale])

  return (
    <div className="mx-auto max-w-6xl px-4 py-10 sm:px-6">
      <header className="mb-9">
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
      <div className="grid items-start gap-8 lg:grid-cols-[minmax(0,1fr)_330px]">
        <div
          ref={previewSlotRef}
          className="order-2 min-w-0 overflow-x-auto lg:order-1"
        >
          <article
            aria-label={t("cv.documentPreview")}
            className="cv-paper relative mx-auto w-[210mm] border border-[var(--color-border)] bg-[var(--color-card)] shadow-sm"
            style={{ zoom: previewScale }}
          >
            <div
              ref={boundaryRef}
              aria-hidden="true"
              className="cv-page-boundary pointer-events-none absolute left-0 top-0 w-full"
            >
              {overflows && (
                <span className="absolute bottom-0 right-0 bg-[var(--color-accent)] px-2 py-1 text-[10px] font-semibold text-white">
                  {t("cv.pageOneEnds")}
                </span>
              )}
            </div>
            <div ref={contentRef} className="relative px-12 py-11">
              <header className="mb-7 border-b-2 border-[var(--color-accent)] pb-6">
                {!repo.fullName.trim() && (
                  <p className="mb-2 text-[10px] font-semibold uppercase tracking-widest text-[var(--color-muted-fg)]">
                    {t("common.sample")}
                  </p>
                )}
                <h2 className="break-words text-5xl font-bold uppercase leading-none tracking-tight [font-family:var(--font-display)]">
                  {repo.fullName.trim() || t("cv.sampleName")}
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
                            {item.value}
                          </li>
                        ))}
                    </ul>
                  )}
              </header>
              {visible("summary") && (
                <Section id="summary" title={t("cv.professionalProfile")}>
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
              {visible("skills") &&
                ((!skills.length && !competencies.length) ||
                  selectedSkills.length > 0 ||
                  selectedCompetencies.length > 0) && (
                  <Section id="skills" title={t("cv.skillsCompetencies")}>
                    {skills.length || competencies.length ? (
                      <p className="break-words text-[12px] leading-5">
                        {[...selectedSkills, ...selectedCompetencies].join(
                          " · ",
                        )}
                      </p>
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
              {visible("experience") &&
                (experience.length === 0 || selectedExperience.length > 0) && (
                  <Section id="experience" title={t("cv.experience")}>
                    {experience.length ? (
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
                    ) : (
                      <div className="text-[12px] leading-5">
                        <p className="mb-2 text-[10px] uppercase tracking-widest text-[var(--color-muted-fg)]">
                          {t("common.sampleContent")}
                        </p>
                        <h4 className="font-semibold">
                          {t("cv.sampleJobTitle")}
                        </h4>
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
                    )}
                  </Section>
                )}
              {visible("education") &&
                education.some((item) =>
                  selectedEntry("education", item.id),
                ) && (
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
                      .filter((item) =>
                        selectedEntry("certifications", item.id),
                      )
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
                              {item.url}
                            </p>
                          )}
                        </div>
                      ))}
                  </Section>
                )}
              {visible("languages") &&
                languages.some((item) =>
                  selectedEntry("languages", item.id),
                ) && (
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
              {visible("tools") && selectedTools.length > 0 && (
                <Section id="tools" title={t("cv.toolsTechnology")}>
                  <p className="break-words text-[12px] leading-5">
                    {selectedTools.join(" · ")}
                  </p>
                </Section>
              )}
              {visible("projects") && selectedProjects.length > 0 && (
                <Section id="projects" title={t("cv.selectedProjects")}>
                  {selectedProjects.map(({ project, bullets }) => (
                    <div
                      key={project.id}
                      className="mb-4 break-words text-[12px] leading-5 last:mb-0"
                    >
                      {project.name && (
                        <h4 className="font-semibold">{project.name}</h4>
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
                          {project.url}
                        </p>
                      )}
                    </div>
                  ))}
                </Section>
              )}
              {visible("additional") && repo.additionalInfo.trim() && (
                <Section id="additional" title={t("cv.additional")}>
                  <p className="whitespace-pre-line break-words text-[12px] leading-5 text-[var(--color-muted-fg)]">
                    {repo.additionalInfo}
                  </p>
                </Section>
              )}
            </div>
          </article>
        </div>
        <aside className="order-1 lg:order-2 lg:sticky lg:top-24">
          <div className="mb-4 border border-[var(--color-border)] bg-[var(--color-card)] p-5">
            <h2 className="mb-2 text-xs uppercase tracking-[0.2em] [font-family:var(--font-mono)]">
              {t("cv.curation")}
            </h2>
            <p className="mb-4 text-xs leading-5 text-[var(--color-muted-fg)]">
              {t("cv.curationIntro")}
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
                          {t("cv.resetToProfile")}
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
                        bulletLines(
                          section,
                          item.id,
                          "highlights",
                          item.highlights,
                        ),
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

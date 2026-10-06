import { useLayoutEffect, useMemo, useRef, useState } from "react"
import { useI18n, useStore } from "../lib/store"
import { cvQualifications } from "../lib/profile"

function lines(value: string, splitCommas = true) {
  return value
    .split(splitCommas ? /\n|,/ : /\n/)
    .map((part) => part.replace(/^[-•*]\s*/, "").trim())
    .filter(Boolean)
}

function Section({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="mb-7 last:mb-0">
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
  const boundaryRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  const [overflows, setOverflows] = useState(false)
  const skills = useMemo(() => lines(repo.skills), [repo.skills])
  const competencies = useMemo(
    () => lines(repo.competencies),
    [repo.competencies],
  )
  const tools = useMemo(() => lines(repo.tools), [repo.tools])
  const contact = [repo.email, repo.phone, repo.location, ...lines(repo.professionalLinks, false)]
    .map(value => value.trim()).filter(Boolean)
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

  useLayoutEffect(() => {
    const boundary = boundaryRef.current
    const content = contentRef.current
    if (!boundary || !content) return
    const measure = () =>
      setOverflows(
        content.getBoundingClientRect().height >
          boundary.getBoundingClientRect().height + 2,
      )
    const observer = new ResizeObserver(measure)
    observer.observe(boundary)
    observer.observe(content)
    measure()
    return () => observer.disconnect()
  }, [])

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
      <div className="grid items-start gap-8 lg:grid-cols-[minmax(0,1fr)_260px]">
        <article
          aria-label={t("cv.documentPreview")}
          className="cv-paper relative mx-auto w-full max-w-[210mm] border border-[var(--color-border)] bg-[var(--color-card)] shadow-sm"
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
          <div
            ref={contentRef}
            className="relative px-5 py-7 sm:px-8 sm:py-9 md:px-12 md:py-11"
          >
            <header className="mb-7 border-b-2 border-[var(--color-accent)] pb-6">
              {!repo.fullName.trim() && <p className="mb-2 text-[10px] font-semibold uppercase tracking-widest text-[var(--color-muted-fg)]">
                {t("common.sample")}
              </p>}
              <h2 className="break-words text-4xl font-bold uppercase leading-none tracking-tight sm:text-5xl [font-family:var(--font-display)]">
                {repo.fullName.trim() || t("cv.sampleName")}
              </h2>
              {contact.length > 0 && <ul className="mt-4 flex flex-wrap gap-x-2 text-[11px] leading-5 text-[var(--color-muted-fg)] [font-family:var(--font-mono)]">
                {contact.map((value, index) => <li key={index} className="break-all after:ml-2 after:content-['·'] last:after:content-none">{value}</li>)}
              </ul>}
            </header>
            <Section title={t("cv.professionalProfile")}>
              <p className="whitespace-pre-wrap break-words text-[13px] leading-6">
                {repo.careerGoals || (
                  <>
                    <span className="text-[10px] uppercase tracking-widest text-[var(--color-muted-fg)]">
                      {t("common.sample")} ·{" "}
                    </span>
                    {t("cv.sampleProfile")}
                  </>
                )}
              </p>
            </Section>
            <Section title={t("cv.skillsCompetencies")}>
              {skills.length || competencies.length ? (
                <p className="break-words text-[12px] leading-5">
                  {[...skills, ...competencies].join(" · ")}
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
            <Section title={t("cv.experience")}>
              {experience.length ? (
                experience.map((item) => (
                  <div
                    key={item.id}
                    className="mb-5 break-words text-[12px] leading-5 last:mb-0"
                  >
                    {(item.title || item.company) && (
                      <h4 className="text-[13px] font-semibold">
                        {[item.title, item.company].filter(Boolean).join(" · ")}
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
                    {item.responsibilities && (
                      <p className="mt-2 whitespace-pre-wrap">
                        {item.responsibilities}
                      </p>
                    )}
                    {item.achievements && (
                      <ul className="mt-2 list-disc space-y-1 pl-5">
                        {lines(item.achievements, false).map((line, index) => (
                          <li key={index}>{line}</li>
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
                  <h4 className="font-semibold">{t("cv.sampleJobTitle")}</h4>
                  <p className="mt-1 text-[var(--color-muted-fg)]">
                    2021 — {t("common.present")}
                  </p>
                  <p className="mt-2">{t("cv.sampleExperienceDescription")}</p>
                  <ul className="mt-2 list-disc space-y-1 pl-5">
                    <li>{t("cv.sampleAchievementOne")}</li>
                    <li>{t("cv.sampleAchievementTwo")}</li>
                  </ul>
                </div>
              )}
            </Section>
            {education.length > 0 && <Section title={t("cv.education")}>
              {education.map(item => <div key={item.id} className="mb-4 break-words text-[12px] leading-5 last:mb-0">
                {item.degree && <h4 className="text-[13px] font-semibold">{item.degree}</h4>}
                {[item.institution, item.location, item.graduationDate].filter(Boolean).length > 0 &&
                  <p className="mt-1 text-[var(--color-muted-fg)]">{[item.institution, item.location, item.graduationDate].filter(Boolean).join(" · ")}</p>}
                {item.details && <p className="mt-1 whitespace-pre-wrap">{item.details}</p>}
              </div>)}
            </Section>}
            {certifications.length > 0 && <Section title={t("cv.certifications")}>
              {certifications.map(item => <div key={item.id} className="mb-4 break-words text-[12px] leading-5 last:mb-0">
                <h4 className="text-[13px] font-semibold">{item.name}</h4>
                {[item.issuer, item.date, item.credentialId].filter(Boolean).length > 0 &&
                  <p className="mt-1 text-[var(--color-muted-fg)]">{[item.issuer, item.date, item.credentialId].filter(Boolean).join(" · ")}</p>}
                {item.url && <p className="mt-1 break-all text-[var(--color-muted-fg)]">{item.url}</p>}
              </div>)}
            </Section>}
            {languages.length > 0 && <Section title={t("cv.languages")}>
              <ul className="space-y-1 text-[12px] leading-5">
                {languages.map(item => <li key={item.id} className="break-words">{[item.name, item.proficiency].filter(Boolean).join(" · ")}</li>)}
              </ul>
            </Section>}
            {tools.length > 0 && (
              <Section title={t("cv.toolsTechnology")}>
                <p className="break-words text-[12px] leading-5">
                  {tools.join(" · ")}
                </p>
              </Section>
            )}
            {projects.length > 0 && (
              <Section title={t("cv.selectedProjects")}>
                {projects.map((project) => (
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
                    {project.highlights && (
                      <p className="mt-1 whitespace-pre-wrap">
                        {project.highlights}
                      </p>
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
            {repo.additionalInfo.trim() && <Section title={t("cv.additional")}>
              <p className="whitespace-pre-line break-words text-[12px] leading-5 text-[var(--color-muted-fg)]">
                {repo.additionalInfo}
              </p>
            </Section>}
          </div>
        </article>
        <aside className="lg:sticky lg:top-24">
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
            {overflows && (
              <p
                role="status"
                className="mt-4 border-l-2 border-[var(--color-accent)] pl-3 text-xs leading-5"
              >
                {t("cv.overflowNotice")}
              </p>
            )}
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
                  {complete ? t("cv.added") : hasSample ? t("common.sampleContent") : t("cv.notAdded")}
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

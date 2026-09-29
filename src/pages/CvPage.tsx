import { useMemo, useState } from "react"
import { useI18n, useStore } from "../lib/store"

function splitLines(value: string) {
  return value
    .split(/\n|,/)
    .map((item) => item.replace(/^[-•*]\s*/, "").trim())
    .filter(Boolean)
}

function SectionTitle({
  children,
  muted = false,
}: {
  children: React.ReactNode
  muted?: boolean
}) {
  return (
    <h3
      className="text-[10px] font-bold uppercase tracking-[0.2em] mb-3"
      style={{ color: muted ? "var(--color-muted-fg)" : "var(--color-accent)" }}
    >
      {children}
    </h3>
  )
}

export default function CvPage() {
  const { state } = useStore()
  const { t } = useI18n()
  const repo = state.repository
  const skills = useMemo(() => splitLines(repo.skills), [repo.skills])
  const competencies = useMemo(
    () => splitLines(repo.competencies),
    [repo.competencies],
  )
  const tools = useMemo(() => splitLines(repo.tools), [repo.tools])
  const projects = repo.projects.filter(
    (project) => project.name || project.description,
  )
  const experience = repo.experience.filter(
    (item) => item.title || item.company,
  )
  const name = "YOUR NAME"

  return (
    <div className="max-w-6xl mx-auto px-6 py-10">
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-6 mb-9">
        <div>
          <p
            className="text-xs uppercase tracking-[0.25em] mb-3"
            style={{
              fontFamily: "var(--font-mono)",
              color: "var(--color-muted-fg)",
            }}
          >
            {t("cv.step")}
          </p>
          <h1
            className="text-6xl font-bold uppercase tracking-tight leading-[0.9]"
            style={{ fontFamily: "var(--font-display)" }}
          >
            {t("cv.titleYourCv")}
            <br />
            <span style={{ color: "var(--color-accent)" }}>
              {t("cv.titleYourTemplate")}
            </span>
          </h1>
          <p
            className="text-sm max-w-xl mt-4 leading-relaxed"
            style={{ color: "var(--color-muted-fg)" }}
          >
            {t("cv.intro")}
          </p>
        </div>
      </div>

      <div className="grid lg:grid-cols-[minmax(0,1fr)_260px] gap-8 items-start">
        <article
          className="min-h-[900px] shadow-sm"
          style={{
            backgroundColor: "var(--color-card)",
            border: "1px solid var(--color-border)",
          }}
        >
          <div className="p-8 md:p-12">
            <header
              className="pb-7 mb-8 border-b-2"
              style={{ borderColor: "var(--color-accent)" }}
            >
              <h2
                className="text-5xl md:text-6xl font-bold uppercase tracking-tight leading-none"
                style={{ fontFamily: "var(--font-display)" }}
              >
                {name}
              </h2>
              <p
                className="text-sm mt-3"
                style={{ color: "var(--color-muted-fg)" }}
              >
                {repo.careerGoals || (
                  <>
                    <span className="text-[9px] uppercase tracking-widest">
                      {t("common.sample")}
                    </span>{" "}
                    · {t("cv.sampleCareerLine")}
                  </>
                )}
              </p>
              <p
                className="text-[10px] mt-5 tracking-wide"
                style={{
                  color: "var(--color-muted-fg)",
                  fontFamily: "var(--font-mono)",
                }}
              >
                <span className="uppercase tracking-widest">
                  {t("common.sample")}
                </span>{" "}
                · {t("cv.sampleContact")}
              </p>
            </header>

            <section className="mb-8">
              <SectionTitle>{t("cv.professionalProfile")}</SectionTitle>
              <p
                className="text-sm leading-7"
                style={{ color: "var(--color-fg)" }}
              >
                {repo.additionalInfo || repo.careerGoals || (
                  <>
                    <span
                      className="text-[9px] uppercase tracking-widest"
                      style={{ color: "var(--color-muted-fg)" }}
                    >
                      {t("common.sample")} ·{" "}
                    </span>
                    {t("cv.sampleProfile")}
                  </>
                )}
              </p>
            </section>

            <section className="mb-8">
              <SectionTitle>{t("cv.experience")}</SectionTitle>
              {experience.length ? (
                experience.map((item) => (
                  <div key={item.id} className="mb-6">
                    <div className="flex flex-wrap justify-between gap-2">
                      <h4 className="text-sm font-semibold">
                        {item.title}{" "}
                        <span className="font-normal">· {item.company}</span>
                      </h4>
                      <span
                        className="text-[10px] uppercase tracking-wide"
                        style={{
                          color: "var(--color-muted-fg)",
                          fontFamily: "var(--font-mono)",
                        }}
                      >
                        {item.startDate || "2021"} —{" "}
                        {item.current
                          ? t("common.present")
                          : item.endDate || "2024"}
                      </span>
                    </div>
                    <p
                      className="text-xs mt-1 mb-2"
                      style={{ color: "var(--color-muted-fg)" }}
                    >
                      {item.location || t("cv.defaultLocation")}
                    </p>
                    <p className="text-xs leading-6">
                      {item.description || t("cv.defaultDescription")}
                    </p>
                    <ul className="mt-2 space-y-1">
                      {splitLines(item.achievements || item.responsibilities)
                        .slice(0, 3)
                        .map((line, index) => (
                          <li
                            key={index}
                            className="text-xs leading-5 pl-4 relative before:content-['•'] before:absolute before:left-0"
                            style={{ color: "var(--color-muted-fg)" }}
                          >
                            {line}
                          </li>
                        ))}
                    </ul>
                  </div>
                ))
              ) : (
                <div className="mb-6">
                  <p
                    className="text-[9px] uppercase tracking-widest mb-2"
                    style={{ color: "var(--color-muted-fg)" }}
                  >
                    {t("common.sampleContent")}
                  </p>
                  <div className="flex justify-between gap-2">
                    <h4 className="text-sm font-semibold">
                      {t("cv.sampleJobTitle")}
                    </h4>
                    <span
                      className="text-[10px]"
                      style={{
                        color: "var(--color-muted-fg)",
                        fontFamily: "var(--font-mono)",
                      }}
                    >
                      2021 — {t("common.present")}
                    </span>
                  </div>
                  <p
                    className="text-xs mt-1 mb-2"
                    style={{ color: "var(--color-muted-fg)" }}
                  >
                    {t("cv.defaultLocation")}
                  </p>
                  <p className="text-xs leading-6">
                    {t("cv.sampleExperienceDescription")}
                  </p>
                  <ul className="mt-2 space-y-1">
                    <li
                      className="text-xs leading-5 pl-4 relative before:content-['•'] before:absolute before:left-0"
                      style={{ color: "var(--color-muted-fg)" }}
                    >
                      {t("cv.sampleAchievementOne")}
                    </li>
                    <li
                      className="text-xs leading-5 pl-4 relative before:content-['•'] before:absolute before:left-0"
                      style={{ color: "var(--color-muted-fg)" }}
                    >
                      {t("cv.sampleAchievementTwo")}
                    </li>
                  </ul>
                </div>
              )}
              {experience.length > 1 && null}
              {!experience.length && (
                <div
                  className="pt-4 border-t"
                  style={{ borderColor: "var(--color-border)" }}
                >
                  <div className="flex justify-between gap-2">
                    <h4 className="text-sm font-semibold">
                      {t("cv.sampleSecondJob")}
                    </h4>
                    <span
                      className="text-[10px]"
                      style={{
                        color: "var(--color-muted-fg)",
                        fontFamily: "var(--font-mono)",
                      }}
                    >
                      2018 — 2021
                    </span>
                  </div>
                  <p
                    className="text-xs leading-6 mt-2"
                    style={{ color: "var(--color-muted-fg)" }}
                  >
                    {t("cv.sampleSecondJobDescription")}
                  </p>
                </div>
              )}
            </section>

            <div className="grid md:grid-cols-2 gap-8">
              <section>
                <SectionTitle>{t("cv.skillsCompetencies")}</SectionTitle>
                <p className="text-xs leading-6">
                  {[...skills, ...competencies].slice(0, 12).join(" · ") || (
                    <>
                      <span
                        className="text-[9px] uppercase tracking-widest"
                        style={{ color: "var(--color-muted-fg)" }}
                      >
                        {t("common.sample")} ·{" "}
                      </span>
                      {t("cv.sampleSkills")}
                    </>
                  )}
                </p>
              </section>
              <section>
                <SectionTitle>{t("cv.education")}</SectionTitle>
                <p
                  className="text-[9px] uppercase tracking-widest mb-1"
                  style={{ color: "var(--color-muted-fg)" }}
                >
                  {t("common.sampleContent")}
                </p>
                <h4 className="text-sm font-semibold">{t("cv.degree")}</h4>
                <p
                  className="text-xs mt-1"
                  style={{ color: "var(--color-muted-fg)" }}
                >
                  {t("cv.university")}
                </p>
              </section>
              {tools.length > 0 && (
                <section>
                  <SectionTitle>{t("cv.toolsTechnology")}</SectionTitle>
                  <p className="text-xs leading-6">
                    {tools.slice(0, 12).join(" · ")}
                  </p>
                </section>
              )}
              {projects.length > 0 && (
                <section>
                  <SectionTitle>{t("cv.selectedProjects")}</SectionTitle>
                  {projects.slice(0, 2).map((project) => (
                    <div key={project.id} className="mb-3">
                      <h4 className="text-xs font-semibold">{project.name}</h4>
                      <p
                        className="text-xs leading-5 mt-1"
                        style={{ color: "var(--color-muted-fg)" }}
                      >
                        {project.description || project.highlights}
                      </p>
                    </div>
                  ))}
                </section>
              )}
              <section>
                <SectionTitle>{t("cv.additional")}</SectionTitle>
                <p
                  className="text-[9px] uppercase tracking-widest mb-1"
                  style={{ color: "var(--color-muted-fg)" }}
                >
                  {t("common.sampleContent")}
                </p>
                <p
                  className="text-xs leading-6 whitespace-pre-line"
                  style={{ color: "var(--color-muted-fg)" }}
                >
                  {t("cv.languagesCertification")}
                </p>
              </section>
            </div>
          </div>
        </article>

        <aside className="lg:sticky lg:top-24">
          <div
            className="p-5 border"
            style={{
              borderColor: "var(--color-border)",
              backgroundColor: "var(--color-card)",
            }}
          >
            <p
              className="text-xs uppercase tracking-[0.2em] mb-4"
              style={{
                fontFamily: "var(--font-mono)",
                color: "var(--color-muted-fg)",
              }}
            >
              {t("cv.previewSettings")}
            </p>
            <div className="space-y-4">
              <div>
                <label
                  className="text-xs block mb-1"
                  style={{ color: "var(--color-muted-fg)" }}
                >
                  {t("cv.template")}
                </label>
                <p className="text-xs">{t("cv.atsCv")}</p>
              </div>
              <div>
                <label
                  className="text-xs block mb-1"
                  style={{ color: "var(--color-muted-fg)" }}
                >
                  {t("cv.pageFormat")}
                </label>
                <p className="text-xs">{t("cv.a4Pages")}</p>
              </div>
              <div>
                <label
                  className="text-xs block mb-2"
                  style={{ color: "var(--color-muted-fg)" }}
                >
                  {t("cv.accentColor")}
                </label>
                <div className="flex items-center gap-2">
                  <span
                    className="inline-block w-4 h-4 border"
                    style={{
                      backgroundColor: "#C8102E",
                      borderColor: "var(--color-border)",
                    }}
                  />
                  <span className="text-xs">{t("cv.red")}</span>
                </div>
              </div>
            </div>
            <p
              className="text-[10px] leading-5 mt-4"
              style={{ color: "var(--color-muted-fg)" }}
            >
              {t("cv.settingsNote")}
            </p>
          </div>
          <div
            className="p-5 mt-4 border"
            style={{ borderColor: "var(--color-border)" }}
          >
            <p
              className="text-xs uppercase tracking-[0.2em] mb-3"
              style={{
                fontFamily: "var(--font-mono)",
                color: "var(--color-muted-fg)",
              }}
            >
              {t("cv.profileCoverage")}
            </p>
            {[
              [t("cv.experience"), experience.length > 0],
              [t("repo.section.skills"), skills.length > 0],
              [t("repo.section.projects"), projects.length > 0],
            ].map(([label, complete]) => (
              <div
                key={label as string}
                className="flex justify-between py-2 border-b text-xs"
                style={{ borderColor: "var(--color-border)" }}
              >
                <span>{label}</span>
                <span
                  style={{
                    color: complete
                      ? "var(--color-fg)"
                      : "var(--color-muted-fg)",
                  }}
                >
                  {complete ? t("cv.added") : t("common.sampleContent")}
                </span>
              </div>
            ))}
            <p
              className="text-[10px] leading-5 mt-3"
              style={{ color: "var(--color-muted-fg)" }}
            >
              {t("cv.editProfileNote")}
            </p>
          </div>
        </aside>
      </div>
    </div>
  )
}

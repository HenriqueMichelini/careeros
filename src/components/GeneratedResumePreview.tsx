import { Fragment, useLayoutEffect, useMemo, useRef, useState } from "react"
import CvFontSizeControl from "./CvFontSizeControl"
import { useCvPreferences } from "../lib/cvPreferences"
import CvPaper from "./CvPaper"
import { useI18n } from "../lib/store"
import {
  professionalLinkHref,
  professionalLinkLabel,
  professionalLinkTarget,
} from "../lib/cv"
import { parseResumeHeader, parseResumeMarkdown } from "../lib/resume"
import { ProfessionalRepository } from "../lib/types"

const A4_WIDTH_PX = (210 * 96) / 25.4

function plainText(value: string) {
  return value
    .replace(/\*\*([^*]+)\*\*/g, "$1")
    .replace(/(?<!\w)\*([^*]+)\*(?!\w)/g, "$1")
    .replace(/`([^`]+)`/g, "$1")
}

function inlineText(value: string) {
  const links = /\[([^\]]+)\]\(([^\s)]+)\)/g
  const parts: React.ReactNode[] = []
  let from = 0
  for (const match of value.matchAll(links)) {
    const index = match.index ?? 0
    if (index > from) parts.push(plainText(value.slice(from, index)))
    const href = professionalLinkHref(match[2])
    parts.push(
      href ? (
        <a key={index} href={href} className="underline underline-offset-2">
          {plainText(match[1])}
        </a>
      ) : (
        plainText(match[1])
      ),
    )
    from = index + match[0].length
  }
  if (from < value.length) parts.push(plainText(value.slice(from)))
  return parts
}

export default function GeneratedResumePreview({
  resume,
  profile,
  onEditProfile,
}: {
  resume: string
  profile: ProfessionalRepository
  onEditProfile: () => void
}) {
  const typography = useCvPreferences()
  const { t } = useI18n()
  const slotRef = useRef<HTMLDivElement>(null)
  const boundaryRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  const [fitScale, setFitScale] = useState(1)
  const paperScale = fitScale
  const [overflows, setOverflows] = useState(false)
  const parsed = useMemo(() => parseResumeHeader(resume), [resume])
  const sections = useMemo(
    () => parseResumeMarkdown(parsed.body),
    [parsed.body],
  )
  const labels = {
    summary: t("cv.professionalProfile"),
    experience: t("cv.experience"),
    skills: t("cv.skillsCompetencies"),
    projects: t("cv.selectedProjects"),
    education: t("cv.education"),
    certifications: t("cv.certifications"),
    languages: t("cv.languages"),
    tools: t("cv.toolsTechnology"),
    additional: t("cv.additional"),
    contact: t("cv.contact"),
  }
  const profileContacts = [
    profile.email,
    profile.phone,
    profile.location,
    ...profile.professionalLinks.split("\n"),
  ]
    .map((item) => item.trim())
    .filter(Boolean)
  const name = profile.fullName.trim() || parsed.name
  const contacts = profileContacts.length ? profileContacts : parsed.contacts
  const visibleSections = sections.filter(
    (section) =>
      section.title ||
      !section.blocks.every((block) =>
        block.text
          .split(/\s*[|·]\s*/)
          .every((part) =>
            contacts.includes(
              part.replace(/^\[[^\]]+\]\(([^)]+)\)$/, "$1").trim(),
            ),
          ),
      ),
  )

  useLayoutEffect(() => {
    const slot = slotRef.current
    if (!slot) return
    const measure = () =>
      setFitScale(Math.min(1, slot.getBoundingClientRect().width / A4_WIDTH_PX))
    const observer = new ResizeObserver(measure)
    observer.observe(slot)
    measure()
    return () => observer.disconnect()
  }, [])
  useLayoutEffect(() => {
    const boundary = boundaryRef.current
    const content = contentRef.current
    if (!boundary || !content) return
    const measure = () =>
      setOverflows(
        content.getBoundingClientRect().bottom >
          boundary.getBoundingClientRect().bottom + 2 * paperScale,
      )
    const observer = new ResizeObserver(measure)
    observer.observe(boundary)
    observer.observe(content)
    measure()
    return () => observer.disconnect()
  }, [resume, profile, paperScale, typography.fontSize])

  return (
    <div className="generated-resume-preview">
      <CvFontSizeControl
        fontSize={typography.fontSize}
        onChange={typography.setFontSize}
        saveError={typography.saveError}
      />
      <p
        role="status"
        className="mb-4 text-xs leading-5 text-[var(--color-muted-fg)]"
      >
        {overflows ? t("results.resumeOverflow") : t("results.resumeFits")}
      </p>
      {(!name || contacts.length === 0) && (
        <div className="mb-4 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs leading-5 text-[var(--color-muted-fg)]">
          <span>{t("results.resumeMissingIdentity")}</span>
          <button
            type="button"
            onClick={onEditProfile}
            className="underline underline-offset-2 text-[var(--color-accent)]"
          >
            {t("nav.profile")}
          </button>
        </div>
      )}
      <div ref={slotRef} className="cv-preview-slot min-w-0 overflow-x-auto">
        <CvPaper
          fontSize={typography.fontSize}
          label={t("results.resume")}
          className="generated-resume-paper"
          scale={paperScale}
          overflows={overflows}
          pageEndLabel={t("cv.pageOneEnds")}
          boundaryRef={boundaryRef}
          contentRef={contentRef}
        >
          {(name || contacts.length > 0) && (
            <header className="mb-3 border-b-2 border-[var(--color-accent)] pb-2.5">
              {name && (
                <h2 className="break-words text-[31px] font-bold uppercase leading-none tracking-tight [font-family:var(--font-display)]">
                  {name}
                </h2>
              )}
              {contacts.length > 0 && (
                <ul className="mt-2 flex flex-wrap gap-x-1.5 text-[10px] leading-4 text-[var(--color-muted-fg)] [font-family:var(--font-mono)]">
                  {contacts.map((item, index) => {
                    const target = professionalLinkTarget(item)
                    const href = item.includes("@")
                      ? null
                      : professionalLinkHref(target)
                    return (
                      <li
                        key={`${index}-${item}`}
                        className="break-all after:ml-2 after:content-['·'] last:after:content-none"
                      >
                        {profileContacts.length && href ? (
                          <a
                            href={href}
                            className="underline underline-offset-2"
                          >
                            {professionalLinkLabel(item)}
                          </a>
                        ) : (
                          inlineText(item)
                        )}
                      </li>
                    )
                  })}
                </ul>
              )}
            </header>
          )}
          {visibleSections.map((section, sectionIndex) => (
            <section
              key={sectionIndex}
              data-cv-block={section.id ?? section.title}
              className="mb-3 last:mb-0"
            >
              <h3 className="mb-1.5 text-[10px] font-bold uppercase tracking-[0.14em] text-[var(--color-accent)]">
                {section.id
                  ? labels[section.id]
                  : section.title || t("results.resume")}
              </h3>
              {section.blocks.map((block, blockIndex) => (
                <Fragment key={blockIndex}>
                  {block.kind === "subheading" ? (
                    <h4 className="mt-1.5 mb-0.5 break-words text-[11.5px] font-semibold first:mt-0">
                      {inlineText(block.text)}
                    </h4>
                  ) : block.kind === "bullet" ? (
                    <p className="break-words pl-4 text-[11px] leading-[1.4] before:-ml-3 before:mr-1.5 before:content-['•']">
                      {inlineText(block.text)}
                    </p>
                  ) : block.kind === "numbered" ? (
                    <p className="break-words pl-4 text-[11px] leading-[1.4]">
                      {inlineText(block.text)}
                    </p>
                  ) : (
                    <p className="mb-1 whitespace-pre-wrap break-words text-[11px] leading-[1.4] last:mb-0">
                      {inlineText(block.text)}
                    </p>
                  )}
                </Fragment>
              ))}
            </section>
          ))}
        </CvPaper>
      </div>
    </div>
  )
}

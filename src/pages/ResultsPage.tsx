import { useEffect, useRef, useState } from "react"
import { useI18n, useStore } from "../lib/store"
import { Page } from "../lib/types"
import { TranslationKey } from "../lib/i18n"
import { parseResumeHeader } from "../lib/resume"
import GeneratedResumePreview from "../components/GeneratedResumePreview"

type Tab = "summary" | "resume" | "cover" | "answers"
type PdfTarget = "resume" | "cover"
const A4_HEIGHT_PX = (297 * 96) / 25.4

const TABS: { id: Tab; labelKey: TranslationKey }[] = [
  { id: "summary", labelKey: "results.summary" },
  { id: "resume", labelKey: "results.resume" },
  { id: "cover", labelKey: "results.coverLetter" },
  { id: "answers", labelKey: "results.applicationQa" },
]

function copyToClipboard(text: string) {
  navigator.clipboard.writeText(text).catch(() => {
    const el = document.createElement("textarea")
    el.value = text
    document.body.appendChild(el)
    el.select()
    document.execCommand("copy")
    document.body.removeChild(el)
  })
}

function MarkdownContent({ content }: { content: string }) {
  return (
    <pre
      className="text-sm leading-relaxed whitespace-pre-wrap"
      style={{
        fontFamily: "var(--font-mono)",
        color: "var(--color-fg)",
      }}
    >
      {content}
    </pre>
  )
}

interface Props {
  setPage: (p: Page) => void
}

export default function ResultsPage({ setPage }: Props) {
  const { state } = useStore()
  const { t } = useI18n()
  const [activeTab, setActiveTab] = useState<Tab>("summary")
  const [copied, setCopied] = useState(false)
  const [pdfOverflow, setPdfOverflow] = useState(false)
  const resumePreviewRef = useRef<HTMLDivElement>(null)
  const printCleanupRef = useRef<(() => void) | null>(null)
  const materials = state.generatedMaterials

  useEffect(() => {
    return () => printCleanupRef.current?.()
  }, [])
  useEffect(() => setPdfOverflow(false), [activeTab, materials?.resume])

  if (!materials) {
    return (
      <div className="max-w-6xl mx-auto px-6 py-24 text-center">
        <p
          className="text-xs uppercase tracking-[0.25em] mb-5"
          style={{
            fontFamily: "var(--font-mono)",
            color: "var(--color-muted-fg)",
          }}
        >
          {t("results.noResults")}
        </p>
        <h1
          className="text-5xl font-bold uppercase tracking-tight mb-6"
          style={{ fontFamily: "var(--font-display)" }}
        >
          {t("results.nothingGenerated")}
        </h1>
        <p className="text-sm mb-8" style={{ color: "var(--color-muted-fg)" }}>
          {t("results.noResultsDescription")}
        </p>
        <button
          onClick={() => setPage("home")}
          className="text-sm uppercase tracking-[0.15em] px-8 py-3"
          style={{
            fontFamily: "var(--font-display)",
            backgroundColor: "var(--color-fg)",
            color: "var(--color-bg)",
          }}
        >
          {t("results.goToApply")}
        </button>
      </div>
    )
  }

  const tabContent: Record<Tab, string> = {
    summary: materials.jobSummary,
    resume: materials.resume,
    cover: materials.coverLetter,
    answers: materials.applicationAnswers,
  }

  function handleCopy() {
    copyToClipboard(tabContent[activeTab])
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  async function savePdf(target: PdfTarget) {
    if (!materials) return
    await document.fonts.ready
    let printContent: HTMLElement
    if (target === "resume") {
      const paper = resumePreviewRef.current?.querySelector<HTMLElement>(".cv-paper")
      if (!paper) return
      const measure = paper.cloneNode(true) as HTMLElement
      measure.classList.add("cv-export-measure")
      measure.style.zoom = "1"
      document.body.append(measure)
      const height = measure.querySelector<HTMLElement>(".cv-paper-content")?.getBoundingClientRect().height ?? 0
      measure.remove()
      if (height > A4_HEIGHT_PX - 4) {
        setPdfOverflow(true)
        return
      }
      printContent = paper.cloneNode(true) as HTMLElement
    } else {
      const cover = document.querySelector<HTMLElement>(".results-cover-print")
      if (!cover) return
      const measure = cover.cloneNode(true) as HTMLElement
      measure.classList.add("cv-export-measure")
      measure.style.display = "block"
      document.body.append(measure)
      const height = measure.getBoundingClientRect().height
      measure.remove()
      if (height > A4_HEIGHT_PX - 4) {
        setPdfOverflow(true)
        return
      }
      printContent = cover.cloneNode(true) as HTMLElement
    }
    setPdfOverflow(false)
    printCleanupRef.current?.()
    const printRoot = document.createElement("div")
    printRoot.className = "results-print-root"
    printRoot.append(printContent)
    document.body.append(printRoot)
    const printClass = target === "resume" ? "results-print-resume" : "results-print-cover"
    document.body.classList.remove("results-print-resume", "results-print-cover")
    document.body.classList.add(printClass)
    const previousTitle = document.title
    const name = state.repository.fullName.trim() || parseResumeHeader(materials.resume).name || materials.company || "CareerOS"
    document.title = `${name} - ${target === "resume" ? "Resume" : "Cover Letter"}`
    const finish = () => {
      window.removeEventListener("afterprint", finish)
      document.body.classList.remove(printClass)
      printRoot.remove()
      document.title = previousTitle
      if (printCleanupRef.current === finish) printCleanupRef.current = null
    }
    printCleanupRef.current = finish
    window.addEventListener("afterprint", finish)
    try {
      window.print()
    } catch (error) {
      finish()
      throw error
    }
  }

  return (
    <div className="results-page max-w-6xl mx-auto px-6 py-10">
      {/* Header */}
      <div className="results-page-header mb-10 grid items-start gap-6 sm:grid-cols-[minmax(0,1fr)_auto] sm:gap-8">
        <div className="min-w-0">
          <p
            className="text-xs uppercase tracking-[0.25em] mb-3"
            style={{
              fontFamily: "var(--font-mono)",
              color: "var(--color-muted-fg)",
            }}
          >
            {t("results.generatedApplication")}
          </p>
          <h1
            className="break-words text-5xl font-bold uppercase tracking-tight leading-tight"
            style={{ fontFamily: "var(--font-display)" }}
          >
            {materials.jobTitle || t("results.applicationMaterials")}
          </h1>
          {materials.company && (
            <p
              className="text-lg mt-1"
              style={{ color: "var(--color-muted-fg)" }}
            >
              {materials.company}
            </p>
          )}
        </div>

        <div className="flex flex-wrap gap-3 sm:pt-4">
          <button
            onClick={() => setPage("home")}
            className="text-xs uppercase tracking-[0.15em] px-4 py-2.5 transition-opacity hover:opacity-75"
            style={{
              fontFamily: "var(--font-mono)",
              backgroundColor: "var(--color-fg)",
              color: "var(--color-bg)",
            }}
          >
            {t("results.newApplication")}
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div
        className="results-tabs flex w-full gap-0 mb-0 overflow-x-auto border-b"
        style={{ borderColor: "var(--color-border)" }}
      >
        {TABS.map(({ id, labelKey }) => (
          <button
            key={id}
            onClick={() => setActiveTab(id)}
            className="shrink-0 px-5 py-2.5 text-xs uppercase tracking-[0.18em] border-b-2 transition-colors -mb-px"
            style={{
              fontFamily: "var(--font-mono)",
              borderBottomColor:
                activeTab === id ? "var(--color-fg)" : "transparent",
              color:
                activeTab === id ? "var(--color-fg)" : "var(--color-muted-fg)",
            }}
          >
            {t(labelKey)}
          </button>
        ))}
      </div>

      {/* Content */}
      <div
        className="results-content mt-0 min-w-0 p-4 sm:p-8"
        style={{
          backgroundColor: "var(--color-card)",
          border: "1px solid var(--color-border)",
          borderTop: "none",
        }}
      >
        {activeTab === "summary" ? (
          <div>
            <div className="grid grid-cols-3 gap-6 mb-8">
              <div
                className="p-5"
                style={{ border: "1px solid var(--color-border)" }}
              >
                <p
                  className="text-xs uppercase tracking-widest mb-2"
                  style={{
                    fontFamily: "var(--font-mono)",
                    color: "var(--color-muted-fg)",
                  }}
                >
                  {t("results.role")}
                </p>
                <p className="text-sm font-medium">
                  {materials.jobTitle || "—"}
                </p>
              </div>
              <div
                className="p-5"
                style={{ border: "1px solid var(--color-border)" }}
              >
                <p
                  className="text-xs uppercase tracking-widest mb-2"
                  style={{
                    fontFamily: "var(--font-mono)",
                    color: "var(--color-muted-fg)",
                  }}
                >
                  {t("results.company")}
                </p>
                <p className="text-sm font-medium">
                  {materials.company || "—"}
                </p>
              </div>
              <div
                className="p-5"
                style={{ border: "1px solid var(--color-border)" }}
              >
                <p
                  className="text-xs uppercase tracking-widest mb-2"
                  style={{
                    fontFamily: "var(--font-mono)",
                    color: "var(--color-muted-fg)",
                  }}
                >
                  {t("results.materials")}
                </p>
                <p className="text-sm font-medium">
                  {t("results.materialsList")}
                </p>
              </div>
            </div>
            <div>
              <p
                className="text-xs uppercase tracking-[0.2em] mb-3"
                style={{
                  fontFamily: "var(--font-mono)",
                  color: "var(--color-muted-fg)",
                }}
              >
                {t("results.roleSummary")}
              </p>
              <p
                className="text-sm leading-relaxed max-w-2xl"
                style={{ color: "var(--color-fg)" }}
              >
                {materials.jobSummary}
              </p>
            </div>
          </div>
        ) : activeTab === "resume" ? (
          <div>
            <div className="results-resume-actions flex flex-wrap justify-end gap-3 mb-4">
              <button
                onClick={() => savePdf("resume")}
                className="inline-flex items-center gap-2 bg-[var(--color-accent)] px-3 py-1.5 text-xs uppercase tracking-widest text-white transition-opacity hover:opacity-85"
                style={{ fontFamily: "var(--font-mono)" }}
              >
                <svg aria-hidden="true" width="15" height="16" viewBox="0 0 20 22" fill="none" stroke="currentColor" strokeWidth="1.7">
                  <path d="M4 1.5h8l4 4V19a1.5 1.5 0 0 1-1.5 1.5h-10A1.5 1.5 0 0 1 3 19V3a1.5 1.5 0 0 1 1-1.5Z" />
                  <path d="M12 1.5V6h4M6 15.5h8M6 12.5h8" />
                </svg>
                {t("results.savePdf")}
              </button>
            </div>
            {pdfOverflow && <p role="alert" className="mb-4 text-xs text-[var(--color-accent)]">{t("results.pdfOverflow")}</p>}
            <p className="results-pdf-help mb-4 text-xs text-[var(--color-muted-fg)]">{t("results.pdfHelp")}</p>
            <div ref={resumePreviewRef}>
              <GeneratedResumePreview
                resume={materials.resume}
                profile={state.repository}
                onEditProfile={() => setPage("repository")}
              />
            </div>
          </div>
        ) : (
          <div>
            <div className="results-other-actions flex flex-wrap justify-end gap-3 mb-4">
              <button
                onClick={handleCopy}
                className="bg-[var(--color-fg)] px-3 py-1.5 text-xs uppercase tracking-widest text-white transition-opacity hover:opacity-85"
                style={{ fontFamily: "var(--font-mono)" }}
              >
                {copied ? t("common.copied") : t("common.copyText")}
              </button>
              {activeTab === "cover" && (
                <button
                  onClick={() => savePdf("cover")}
                  className="inline-flex items-center gap-2 bg-[var(--color-accent)] px-3 py-1.5 text-xs uppercase tracking-widest text-white transition-opacity hover:opacity-85"
                  style={{ fontFamily: "var(--font-mono)" }}
                >
                  <svg aria-hidden="true" width="15" height="16" viewBox="0 0 20 22" fill="none" stroke="currentColor" strokeWidth="1.7">
                    <path d="M4 1.5h8l4 4V19a1.5 1.5 0 0 1-1.5 1.5h-10A1.5 1.5 0 0 1 3 19V3a1.5 1.5 0 0 1 1-1.5Z" />
                    <path d="M12 1.5V6h4M6 15.5h8M6 12.5h8" />
                  </svg>
                  {t("results.savePdf")}
                </button>
              )}
            </div>
            {activeTab === "cover" && pdfOverflow && <p role="alert" className="mb-4 text-xs text-[var(--color-accent)]">{t("results.coverPdfOverflow")}</p>}
            {activeTab === "cover" && <p className="results-pdf-help mb-4 text-xs text-[var(--color-muted-fg)]">{t("results.pdfHelp")}</p>}
            <MarkdownContent content={tabContent[activeTab]} />
          </div>
        )}
      </div>
      <div className="results-cover-print" aria-hidden="true">{materials.coverLetter}</div>
    </div>
  )
}

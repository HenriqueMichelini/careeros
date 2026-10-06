import { useRef, useState } from "react"
import { useI18n, useStore } from "../lib/store"
import { Page } from "../lib/types"
import { TranslationKey } from "../lib/i18n"
import GeneratedResumePreview from "../components/GeneratedResumePreview"

type Tab = "summary" | "resume" | "cover" | "answers"

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
  const resumePreviewRef = useRef<HTMLDivElement>(null)
  const materials = state.generatedMaterials

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
    const text =
      activeTab === "resume"
        ? (resumePreviewRef.current?.querySelector<HTMLElement>(
            ".cv-paper-content",
          )?.innerText ?? materials!.resume)
        : tabContent[activeTab]
    copyToClipboard(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="max-w-6xl mx-auto px-6 py-10">
      {/* Header */}
      <div className="mb-10 grid items-start gap-6 sm:grid-cols-[minmax(0,1fr)_auto] sm:gap-8">
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
            onClick={handleCopy}
            className="text-xs uppercase tracking-[0.15em] px-4 py-2.5 border transition-colors"
            style={{
              fontFamily: "var(--font-mono)",
              borderColor: "var(--color-border)",
              color: copied ? "var(--color-accent)" : "var(--color-muted-fg)",
            }}
          >
            {copied ? t("common.copied") : t("common.copy")}
          </button>
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
        className="flex w-full gap-0 mb-0 overflow-x-auto border-b"
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
        className="mt-0 min-w-0 p-4 sm:p-8"
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
            <div className="flex justify-end mb-4">
              <button
                onClick={handleCopy}
                className="text-xs uppercase tracking-widest px-3 py-1.5 transition-colors"
                style={{
                  fontFamily: "var(--font-mono)",
                  border: "1px solid var(--color-border)",
                  color: copied
                    ? "var(--color-accent)"
                    : "var(--color-muted-fg)",
                }}
              >
                {copied ? t("common.copied") : t("common.copyText")}
              </button>
            </div>
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
            <div className="flex justify-end mb-4">
              <button
                onClick={handleCopy}
                className="text-xs uppercase tracking-widest px-3 py-1.5 transition-colors"
                style={{
                  fontFamily: "var(--font-mono)",
                  border: "1px solid var(--color-border)",
                  color: copied
                    ? "var(--color-accent)"
                    : "var(--color-muted-fg)",
                }}
              >
                {copied ? t("common.copied") : t("common.copyText")}
              </button>
            </div>
            <MarkdownContent content={tabContent[activeTab]} />
          </div>
        )}
      </div>
    </div>
  )
}

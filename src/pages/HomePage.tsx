import { useI18n, useStore } from "../lib/store"
import { generateMaterials } from "../lib/ai"
import { Page } from "../lib/types"

interface Props {
  setPage: (p: Page) => void
}

function StatusDot({ ok }: { ok: boolean }) {
  return (
    <span
      className="inline-block w-1.5 h-1.5 rounded-full flex-shrink-0"
      style={{
        backgroundColor: ok ? "var(--color-fg)" : "var(--color-border)",
      }}
    />
  )
}

function StatusRow({
  label,
  ok,
  detail,
}: {
  label: string
  ok: boolean
  detail: string
}) {
  return (
    <div
      className="flex items-start gap-3 py-3 border-b"
      style={{ borderColor: "var(--color-border)" }}
    >
      <StatusDot ok={ok} />
      <div className="flex-1 min-w-0">
        <span
          className="text-xs uppercase tracking-[0.18em] block"
          style={{
            fontFamily: "var(--font-mono)",
            color: ok ? "var(--color-fg)" : "var(--color-muted-fg)",
          }}
        >
          {label}
        </span>
        <span
          className="text-xs block mt-0.5 truncate"
          style={{
            fontFamily: "var(--font-mono)",
            color: "var(--color-muted-fg)",
          }}
        >
          {detail}
        </span>
      </div>
    </div>
  )
}

export default function HomePage({ setPage }: Props) {
  const { state, dispatch } = useStore()
  const { t } = useI18n()
  const jobPosting = state.jobPosting

  const hasRepo = !!(
    state.repository.careerGoals ||
    state.repository.experience.length ||
    state.repository.skills
  )
  const hasPosting = jobPosting.trim().length > 30
  const canGenerate =
    hasRepo && hasPosting && !!state.apiKey && !state.isGenerating

  async function handleGenerate() {
    if (!canGenerate) return
    dispatch({ type: "SET_GENERATING", payload: true })
    try {
      const materials = await generateMaterials(
        state.repository,
        jobPosting,
        state.apiKey,
      )
      dispatch({ type: "SET_MATERIALS", payload: materials })
      setPage("results")
    } catch (e: any) {
      alert(e.message || t("home.generationFailed"))
    } finally {
      dispatch({ type: "SET_GENERATING", payload: false })
    }
  }

  return (
    <div className="max-w-6xl mx-auto px-6 py-14">
      {/* Header */}
      <div className="mb-12">
        <p
          className="text-xs uppercase tracking-[0.3em] mb-5"
          style={{
            fontFamily: "var(--font-mono)",
            color: "var(--color-muted-fg)",
          }}
        >
          {t("home.step")}
        </p>
        <h1
          className="text-7xl font-bold uppercase leading-[0.9] tracking-tight mb-5"
          style={{ fontFamily: "var(--font-display)" }}
        >
          {t("home.titleFind")}
          <br />
          <span style={{ color: "var(--color-accent)" }}>
            {t("home.titleNext")}
          </span>
        </h1>
        <p
          className="text-sm max-w-md leading-relaxed"
          style={{ color: "var(--color-muted-fg)" }}
        >
          {t("home.intro")}
        </p>
      </div>

      <div className="grid grid-cols-[1fr_300px] gap-12 items-start">
        {/* Textarea */}
        <div>
          <label
            className="text-xs uppercase tracking-[0.2em] block mb-2"
            style={{
              fontFamily: "var(--font-mono)",
              color: "var(--color-muted-fg)",
            }}
          >
            {t("home.jobPosting")}
          </label>
          <textarea
            value={jobPosting}
            onChange={(e) =>
              dispatch({ type: "SET_JOB_POSTING", payload: e.target.value })
            }
            placeholder={t("home.jobPostingPlaceholder")}
            className="w-full h-[440px] text-sm p-4 resize-none focus:outline-none transition-colors leading-relaxed"
            style={{
              fontFamily: "var(--font-mono)",
              border: "1px solid var(--color-border)",
              backgroundColor: "var(--color-card)",
              color: "var(--color-fg)",
            }}
            onFocus={(e) => (e.target.style.borderColor = "var(--color-fg)")}
            onBlur={(e) => (e.target.style.borderColor = "var(--color-border)")}
          />
          <div
            className="flex justify-between items-center mt-2 text-xs"
            style={{
              fontFamily: "var(--font-mono)",
              color: "var(--color-muted-fg)",
            }}
          >
            <span>
              {t("home.characterCount", { count: jobPosting.trim().length })}
            </span>
            {jobPosting && (
              <button
                onClick={() =>
                  dispatch({ type: "SET_JOB_POSTING", payload: "" })
                }
                className="hover:opacity-60 transition-opacity"
              >
                {t("common.clear")}
              </button>
            )}
          </div>
        </div>

        {/* Sidebar */}
        <div className="pt-6">
          <p
            className="text-xs uppercase tracking-[0.2em] mb-1"
            style={{
              fontFamily: "var(--font-mono)",
              color: "var(--color-muted-fg)",
            }}
          >
            {t("home.checklist")}
          </p>
          <div className="mb-8">
            <StatusRow
              label={t("home.apiKey")}
              ok={!!state.apiKey}
              detail={
                state.apiKey ? t("home.configured") : t("home.setApiKeyAbove")
              }
            />
            <StatusRow
              label={t("home.profile")}
              ok={hasRepo}
              detail={
                hasRepo ? t("home.repositoryReady") : t("home.goToProfile")
              }
            />
            <StatusRow
              label={t("home.posting")}
              ok={hasPosting}
              detail={
                hasPosting
                  ? t("home.characterCount", {
                      count: jobPosting.trim().length,
                    })
                  : t("home.pasteJobDetails")
              }
            />
          </div>

          <button
            onClick={handleGenerate}
            disabled={!canGenerate}
            className="w-full py-4 text-sm uppercase tracking-[0.2em] font-bold transition-colors"
            style={{
              fontFamily: "var(--font-display)",
              backgroundColor: canGenerate
                ? "var(--color-fg)"
                : "var(--color-muted)",
              color: canGenerate ? "var(--color-bg)" : "var(--color-muted-fg)",
              cursor: canGenerate ? "pointer" : "not-allowed",
            }}
          >
            {state.isGenerating
              ? t("home.generating")
              : t("home.generateMaterials")}
          </button>

          {state.isGenerating && (
            <p
              className="text-xs text-center mt-3 leading-relaxed"
              style={{
                fontFamily: "var(--font-mono)",
                color: "var(--color-muted-fg)",
              }}
            >
              {t("home.generatingNote")}
            </p>
          )}

          <div
            className="mt-8 pt-6 border-t"
            style={{ borderColor: "var(--color-border)" }}
          >
            <p
              className="text-xs uppercase tracking-[0.2em] mb-4"
              style={{
                fontFamily: "var(--font-mono)",
                color: "var(--color-muted-fg)",
              }}
            >
              {t("home.output")}
            </p>
            {([
              "home.tailoredResume",
              "home.coverLetter",
              "home.applicationQa",
            ] as const).map((key, i) => (
              <div key={key} className="flex items-center gap-3 mb-3">
                <span
                  className="text-xs w-4"
                  style={{
                    fontFamily: "var(--font-mono)",
                    color: "var(--color-accent)",
                  }}
                >
                  {String(i + 1).padStart(2, "0")}
                </span>
                <span className="text-sm" style={{ color: "var(--color-fg)" }}>
                  {t(key)}
                </span>
              </div>
            ))}
          </div>

          {state.generatedMaterials && (
            <div
              className="mt-4 pt-4 border-t"
              style={{ borderColor: "var(--color-border)" }}
            >
              <button
                onClick={() => setPage("results")}
                className="text-xs uppercase tracking-[0.15em] w-full py-2 border transition-colors hover:opacity-75"
                style={{
                  fontFamily: "var(--font-mono)",
                  borderColor: "var(--color-fg)",
                  color: "var(--color-fg)",
                }}
              >
                {t("home.viewPreviousResults")}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

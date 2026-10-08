import type { FieldDecision } from "../lib/fieldDecision"
import type { TranslationKey } from "../lib/i18n"
import LanguageSelector from "../components/LanguageSelector"
import { useEffect, useRef, useState } from "react"
import { useI18n, useStore } from "../lib/store"
import {
  ApplicationDraftError,
  JobPostingValidationError,
  findProfileGaps,
  generateMaterials,
  QualificationGapsError,
} from "../lib/ai"
import { ConfirmedQualification, Page, ProfileGap } from "../lib/types"

interface Props {
  setPage: (p: Page) => void
}

const gapErrorTranslationKeys = {
  input: "home.gapErrorInput",
  key: "home.gapErrorKey",
  rate_limit: "home.gapErrorRateLimit",
  outage: "home.gapErrorOutage",
  timeout: "home.gapErrorTimeout",
  invalid_output: "home.gapErrorInvalidOutput",
} as const

const draftErrorTranslationKeys = {
  input: "home.draftErrorInput",
  key: "home.draftErrorKey",
  rate_limit: "home.draftErrorRateLimit",
  outage: "home.draftErrorOutage",
  timeout: "home.draftErrorTimeout",
  invalid_output: "home.draftErrorInvalidOutput",
} as const

type StepState = "pending" | "ready" | "processing" | "confirmation" | "complete" | "failed"
type RequestStep = {
  state: StepState
  error?: keyof typeof draftErrorTranslationKeys
}

function StatusRow({
  label,
  status,
  detail,
}: {
  label: string
  status: StepState
  detail: string
}) {
  const { t } = useI18n()
  const color =
    status === "failed"
      ? "var(--color-status-error)"
      : status === "ready" || status === "complete"
        ? "var(--color-status-success)"
        : "var(--color-fg)"
  return (
    <div
      className="flex items-start gap-3 py-3 border-b"
      style={{ borderColor: "var(--color-border)" }}
      data-checklist-state={status}
    >
      <span
        aria-hidden="true"
        className="inline-block w-2 h-2 mt-1 rounded-full flex-shrink-0"
        style={{ backgroundColor: color }}
      />
      <div
        className="flex-1 min-w-0"
        style={{ fontFamily: "var(--font-mono)" }}
      >
        <span
          className="text-xs uppercase tracking-[0.18em] block"
          style={{ color }}
        >
          {label} · {t(`home.status.${status}`)}
        </span>
        <span
          className="text-xs block mt-1 break-words"
          style={{ color: "var(--color-muted-fg)" }}
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
  const [profileGaps, setProfileGaps] = useState<ProfileGap[]>([])
  const [confirmedGapIndices, setConfirmedGapIndices] = useState<Set<number>>(
    new Set(),
  )
  const [gapNotes, setGapNotes] = useState<Record<number, string>>({})
  const [showGapPrompt, setShowGapPrompt] = useState(false)
  const [qualification, setQualification] = useState<RequestStep>({
    state: "pending",
  })
  const [draft, setDraft] = useState<RequestStep>({ state: "pending" })
  const [jobDecision, setJobDecision] = useState<FieldDecision | null>(null)
  const [revisionRequiredFor, setRevisionRequiredFor] = useState<{ text: string; kind: "request_rephrasing" | "reject_attack" } | null>(null)
  const requestId = useRef(0)
  const inputs = JSON.stringify([
    state.repository,
    state.jobPosting,
    state.apiKey,
    state.typesafeKey,
    state.cvLanguage,
  ])
  useEffect(() => {
    setJobDecision(null)
    setQualification({ state: "pending" })
    setDraft({ state: "pending" })
    setShowGapPrompt(false)
    return () => {
      requestId.current += 1
      dispatch({ type: "SET_GENERATING", payload: false })
    }
  }, [inputs, dispatch])
  const isCheckingRequirements = qualification.state === "processing"
  const failure = qualification.error || draft.error
  const jobOutcome = jobDecision?.outcome || (revisionRequiredFor?.text === state.jobPosting.trim() ? { kind: revisionRequiredFor.kind } : null)
  const validationMessage = jobOutcome
    ? t((jobOutcome.kind === "service_failure" ? `field.failure.${jobOutcome.reason}` : `jobField.${jobOutcome.kind}`) as TranslationKey)
    : ""
  const failureMessage = validationMessage || (qualification.error
    ? t(gapErrorTranslationKeys[qualification.error])
    : draft.error
      ? t(draftErrorTranslationKeys[draft.error])
      : "")

  function recordFailure(error: unknown, step: "qualification" | "draft") {
    if (error instanceof JobPostingValidationError) {
      setJobDecision(error.decision)
      if (error.decision.outcome.kind === "request_rephrasing" || error.decision.outcome.kind === "reject_attack") {
        setRevisionRequiredFor({ text: state.jobPosting.trim(), kind: error.decision.outcome.kind })
      }
      setShowGapPrompt(false)
      setQualification({ state: "pending" })
      setDraft({ state: "pending" })
      return
    }
    const code =
      error instanceof QualificationGapsError ||
      error instanceof ApplicationDraftError
        ? error.code as keyof typeof draftErrorTranslationKeys
        : "outage"
    const update = step === "qualification" ? setQualification : setDraft
    update({ state: "failed", error: code })
  }

  function dismissConfirmation() {
    setShowGapPrompt(false)
    setQualification({ state: "pending" })
    setDraft({ state: "pending" })
  }
  const jobPosting = state.jobPosting

  const hasRepo = !!(
    state.repository.careerGoals ||
    state.repository.experience.length ||
    state.repository.skills
  )
  const hasPosting = jobPosting.trim().length > 0
  const canGenerate =
    hasRepo && hasPosting && !!state.apiKey && !!state.typesafeKey.trim() &&
    jobPosting.trim() !== revisionRequiredFor?.text && !state.isGenerating

  async function handleGenerate() {
    if (!canGenerate) return
    setJobDecision(null)
    const id = ++requestId.current
    dispatch({ type: "SET_GENERATING", payload: true })
    setQualification({ state: "processing" })
    setDraft({ state: "pending" })
    let step: "qualification" | "draft" = "qualification"
    try {
      const gaps = await findProfileGaps(
        state.repository,
        jobPosting,
        state.apiKey,
        state.typesafeKey,
      )
      if (id !== requestId.current) return
      if (gaps.length > 0) {
        setProfileGaps(gaps)
        setConfirmedGapIndices(new Set())
        setGapNotes({})
        setQualification({ state: "confirmation" })
        setShowGapPrompt(true)
        return
      }
      setQualification({ state: "complete" })
      step = "draft"
      setDraft({ state: "processing" })
      const materials = await generateMaterials(
        state.repository,
        jobPosting,
        state.apiKey,
        [],
        state.cvLanguage,
        state.typesafeKey,
      )
      if (id !== requestId.current) return
      setDraft({ state: "complete" })
      dispatch({ type: "SET_MATERIALS", payload: materials })
      setPage("results")
    } catch (error) {
      if (id === requestId.current) recordFailure(error, step)
    } finally {
      if (id === requestId.current)
        dispatch({ type: "SET_GENERATING", payload: false })
    }
  }

  async function generateFromGapPrompt(useConfirmedQualifications: boolean) {
    if (state.isGenerating) return

    const confirmedQualifications: ConfirmedQualification[] =
      useConfirmedQualifications
        ? profileGaps.flatMap((gap, index) =>
            confirmedGapIndices.has(index)
              ? [
                  {
                    kind: gap.kind,
                    requirement: gap.requirement,
                    userContext: gapNotes[index]?.trim() || "",
                  },
                ]
              : [],
          )
        : []

    setJobDecision(null)
    const id = ++requestId.current
    setQualification({ state: "complete" })
    setDraft({ state: "processing" })
    dispatch({ type: "SET_GENERATING", payload: true })
    try {
      const materials = await generateMaterials(
        state.repository,
        jobPosting,
        state.apiKey,
        confirmedQualifications,
        state.cvLanguage,
        state.typesafeKey,
      )
      if (id !== requestId.current) return
      setDraft({ state: "complete" })
      dispatch({ type: "SET_MATERIALS", payload: materials })
      setShowGapPrompt(false)
      setPage("results")
    } catch (error) {
      if (id === requestId.current) recordFailure(error, "draft")
    } finally {
      if (id === requestId.current)
        dispatch({ type: "SET_GENERATING", payload: false })
    }
  }

  function toggleGap(index: number, checked: boolean) {
    setDraft({ state: "pending" })
    setConfirmedGapIndices((current) => {
      const next = new Set(current)
      if (checked) next.add(index)
      else next.delete(index)
      return next
    })
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
          className="text-6xl sm:text-7xl font-bold uppercase leading-[0.9] tracking-tight mb-5"
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

      <div className="grid grid-cols-1 lg:grid-cols-[1fr_300px] gap-12 items-start">
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
          <p className="mt-2 text-xs text-[var(--color-muted-fg)]">{t("home.postingLimit")}</p>
          <details className="mt-3 text-xs">
            <summary className="cursor-pointer">{t("field.settings")}</summary>
            <label htmlFor="job-typesafe-key" className="block mt-2">{t("field.typesafeKey")}</label>
            <input id="job-typesafe-key" type="password" autoComplete="off" value={state.typesafeKey}
              onChange={event => dispatch({ type: "SET_TYPESAFE_KEY", payload: event.target.value })}
              className="w-full p-2 border border-[var(--color-border)] bg-[var(--color-card)]" />
            <p className="mt-2">{t("jobField.disclosure")}</p>
          </details>
          <details id="job-input-use-rule" className="mt-3 text-xs">
            <summary className="cursor-pointer">{t("field.ruleTitle")}</summary>
            <p className="mt-2">{t("field.rule")}</p>
          </details>
          {!state.typesafeKey.trim() && <p className="mt-2 text-xs">{t("jobField.typesafeMissing")}</p>}
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
          <div className="mb-6">
            <p className="mb-2 text-xs text-[var(--color-muted-fg)]">{t("language.cv")}</p>
            <LanguageSelector kind="cv" />
          </div>
          <p
            className="text-xs uppercase tracking-[0.2em] mb-1"
            style={{
              fontFamily: "var(--font-mono)",
              color: "var(--color-muted-fg)",
            }}
          >
            {t("home.checklist")}
          </p>
          <div className="mb-8" aria-live="polite" aria-atomic="true">
            <StatusRow
              label={t("home.apiKey")}
              status={
                failure === "key"
                  ? "failed"
                  : state.apiKey
                    ? "ready"
                    : "pending"
              }
              detail={
                failure === "key"
                  ? failureMessage
                  : state.apiKey
                    ? t("home.keyReady")
                    : t("home.setApiKeyAbove")
              }
            />
            <StatusRow
              label={t("home.profile")}
              status={
                failure === "input" ? "failed" : hasRepo ? "ready" : "pending"
              }
              detail={
                failure === "input"
                  ? failureMessage
                  : hasRepo
                    ? t("home.repositoryReady")
                    : t("home.goToProfile")
              }
            />
            <StatusRow
              label={t("home.posting")}
              status={
                failure === "input"
                  ? "failed"
                  : hasPosting
                    ? "ready"
                    : "pending"
              }
              detail={
                failure === "input"
                  ? failureMessage
                  : hasPosting
                    ? t("home.characterCount", {
                        count: jobPosting.trim().length,
                      })
                    : t("home.pasteJobDetails")
              }
            />
          </div>
          {failureMessage && (
            <p
              role="alert"
              className="text-sm mb-4 break-words"
              style={{ color: "var(--color-status-error)" }}
            >
              {failureMessage} {jobOutcome?.kind === "reject_attack" && <a href="#job-input-use-rule" className="underline" onClick={() => { const rule = document.querySelector<HTMLDetailsElement>("#job-input-use-rule"); if (rule) rule.open = true }}>{t("field.ruleTitle")}</a>}
            </p>
          )}

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
            {isCheckingRequirements
              ? t("home.checkingRequirements")
              : state.isGenerating
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
              {isCheckingRequirements
                ? t("home.checkingRequirementsNote")
                : t("home.generatingNote")}
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

      {showGapPrompt && (
        <div
          className="fixed inset-0 z-[100] flex items-center justify-center bg-black/55 p-4"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget && !state.isGenerating) {
              dismissConfirmation()
            }
          }}
        >
          <section
            role="dialog"
            aria-modal="true"
            aria-labelledby="profile-gap-title"
            aria-describedby="profile-gap-description"
            className="w-full max-w-2xl max-h-[90vh] overflow-y-auto p-6 sm:p-8"
            style={{
              backgroundColor: "var(--color-card)",
              border: "1px solid var(--color-border)",
              color: "var(--color-fg)",
            }}
          >
            <div className="flex items-start justify-between gap-6 mb-6">
              <div>
                <p
                  className="text-xs uppercase tracking-[0.2em] mb-3"
                  style={{
                    fontFamily: "var(--font-mono)",
                    color: "var(--color-accent)",
                  }}
                >
                  {t("home.gapPromptEyebrow")}
                </p>
                <h2
                  id="profile-gap-title"
                  className="text-4xl font-bold uppercase tracking-tight leading-none"
                  style={{ fontFamily: "var(--font-display)" }}
                >
                  {t("home.gapPromptTitle")}
                </h2>
              </div>
              <button
                type="button"
                onClick={dismissConfirmation}
                disabled={state.isGenerating}
                className="text-xs uppercase tracking-[0.12em] pt-1 transition-opacity hover:opacity-60 disabled:opacity-40"
                style={{
                  fontFamily: "var(--font-mono)",
                  color: "var(--color-muted-fg)",
                }}
              >
                {t("home.backToPosting")}
              </button>
            </div>

            <p
              id="profile-gap-description"
              className="text-sm leading-relaxed mb-6 max-w-xl"
              style={{ color: "var(--color-muted-fg)" }}
            >
              {t("home.gapPromptDescription")}
            </p>

            {failureMessage && (
              <p
                role="alert"
                className="text-sm mb-4"
                style={{ color: "var(--color-status-error)" }}
              >
                {failureMessage}
              </p>
            )}
            <div className="space-y-3">
              {profileGaps.map((gap, index) => {
                const checked = confirmedGapIndices.has(index)
                return (
                  <div
                    key={`${gap.kind}-${gap.requirement}-${index}`}
                    className="p-4 sm:p-5"
                    style={{
                      border: `1px solid ${
                        checked ? "var(--color-fg)" : "var(--color-border)"
                      }`,
                      backgroundColor: checked
                        ? "var(--color-bg)"
                        : "transparent",
                    }}
                  >
                    <label className="flex items-start gap-3 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={(event) =>
                          toggleGap(index, event.target.checked)
                        }
                        disabled={state.isGenerating}
                        className="mt-1 h-4 w-4 flex-shrink-0"
                        style={{ accentColor: "var(--color-accent)" }}
                      />
                      <span className="min-w-0 flex-1">
                        <span className="flex flex-wrap items-center gap-2 mb-1">
                          <span className="font-medium text-sm">
                            {gap.requirement}
                          </span>
                          <span
                            className="text-[10px] uppercase tracking-[0.14em] px-2 py-1"
                            style={{
                              fontFamily: "var(--font-mono)",
                              color: "var(--color-muted-fg)",
                              backgroundColor: "var(--color-muted)",
                            }}
                          >
                            {t(
                              gap.kind === "skill"
                                ? "home.gapTypeSkill"
                                : "home.gapTypeExperience",
                            )}
                          </span>
                        </span>
                        <span
                          className="block text-xs leading-relaxed"
                          style={{ color: "var(--color-muted-fg)" }}
                        >
                          {gap.details}
                        </span>
                        <span
                          className="block text-xs mt-3"
                          style={{ color: "var(--color-fg)" }}
                        >
                          {t(
                            gap.kind === "skill"
                              ? "home.gapConfirmSkill"
                              : "home.gapConfirmExperience",
                          )}
                        </span>
                      </span>
                    </label>

                    {checked && (
                      <div className="ml-7 mt-4">
                        <label
                          htmlFor={`gap-note-${index}`}
                          className="block text-[10px] uppercase tracking-[0.15em] mb-2"
                          style={{
                            fontFamily: "var(--font-mono)",
                            color: "var(--color-muted-fg)",
                          }}
                        >
                          {t("home.gapExampleLabel")}
                        </label>
                        <textarea
                          id={`gap-note-${index}`}
                          value={gapNotes[index] || ""}
                          onChange={(event) => {
                            setDraft({ state: "pending" })
                            setGapNotes((current) => ({
                              ...current,
                              [index]: event.target.value,
                            }))
                          }}
                          disabled={state.isGenerating}
                          rows={2}
                          placeholder={t("home.gapExamplePlaceholder")}
                          className="w-full resize-y p-3 text-xs leading-relaxed focus:outline-none"
                          style={{
                            fontFamily: "var(--font-mono)",
                            border: "1px solid var(--color-border)",
                            backgroundColor: "var(--color-card)",
                            color: "var(--color-fg)",
                          }}
                        />
                      </div>
                    )}
                  </div>
                )
              })}
            </div>

            <p
              className="text-xs leading-relaxed mt-5"
              style={{ color: "var(--color-muted-fg)" }}
            >
              {t("home.gapPromptPrivacyNote")}
            </p>

            <div
              className="flex flex-col-reverse sm:flex-row sm:justify-between gap-3 mt-7 pt-5 border-t"
              style={{ borderColor: "var(--color-border)" }}
            >
              <button
                type="button"
                onClick={() => void generateFromGapPrompt(false)}
                disabled={state.isGenerating}
                className="text-xs uppercase tracking-[0.12em] px-4 py-3 border transition-opacity hover:opacity-70 disabled:opacity-40"
                style={{
                  fontFamily: "var(--font-mono)",
                  borderColor: "var(--color-border)",
                  color: "var(--color-muted-fg)",
                }}
              >
                {t("home.generateWithoutThese")}
              </button>
              <button
                type="button"
                onClick={() => void generateFromGapPrompt(true)}
                disabled={state.isGenerating || confirmedGapIndices.size === 0}
                className="text-xs uppercase tracking-[0.12em] px-4 py-3 transition-opacity hover:opacity-75 disabled:opacity-40"
                style={{
                  fontFamily: "var(--font-mono)",
                  backgroundColor: "var(--color-fg)",
                  color: "var(--color-bg)",
                  cursor:
                    state.isGenerating || confirmedGapIndices.size === 0
                      ? "not-allowed"
                      : "pointer",
                }}
              >
                {state.isGenerating
                  ? t("home.generating")
                  : t("home.generateWithConfirmed", {
                      count: confirmedGapIndices.size,
                    })}
              </button>
            </div>
          </section>
        </div>
      )}
    </div>
  )
}

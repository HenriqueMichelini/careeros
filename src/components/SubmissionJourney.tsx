import { useCallback, useEffect, useRef, useState } from "react"
import { useI18n, useStore } from "../lib/store"
import {
  IngestionError,
  ingestionInputBytes,
  ingestionMaxBytes,
} from "../lib/ingestion"
import {
  createSubmissionTransport,
  submissionSessionOwner,
  toReviewSubmission,
  type SubmissionController,
  type SubmissionState,
} from "../lib/ingestionSubmission"
import { saveSubmissionSelect } from "../lib/ingestionSave"
import {
  reviewSummary,
  type ReviewPlan,
  type ReviewSummary,
} from "../lib/ingestionReview"
import { submissionCapacityExhausted } from "../lib/ingestionProtocol"
import { SubmissionReview } from "./SubmissionReview"
import type { FieldDecision } from "../lib/fieldDecision"
import type { ProfessionalRepository } from "../lib/types"
import type { TranslationKey } from "../lib/i18n"

// One paste-to-review-to-save journey (#192). The component owns only
// presentation and in-memory foreground actions: it starts the retained
// submission controller with the pinned Profile snapshot, shows one concise
// processing status with a nondestructive Stop, renders the destination-based
// review, and saves exactly the explicitly selected changes through the
// canonical transaction. Nothing is written before an acknowledged save.
const sessionKey = "profile-submission-journey"

function snapshotState(controller: SubmissionController): SubmissionState {
  const state = controller.getState()
  return {
    ...state,
    progress: { ...state.progress },
    batches: [...state.batches],
    batchReceipts: [...state.batchReceipts],
    focus: [...state.focus],
    handoffs: [...state.handoffs],
    cancelledRequestIds: [...state.cancelledRequestIds],
    clarification: { ...state.clarification },
  }
}

function profileEmpty(repository: ProfessionalRepository): boolean {
  return Object.values(repository as unknown as Record<string, unknown>).every(
    (value) =>
      Array.isArray(value)
        ? value.length === 0
        : typeof value === "string"
          ? value.trim().length === 0
          : value === null || value === undefined,
  )
}

function failureKey(code: string): TranslationKey {
  const lookup = {
    input: "repo.ingestErrorInput",
    field_decision: "field.working",
    key: "repo.reviewErrorKey",
    rate_limit: "repo.reviewErrorRateLimit",
    outage: "repo.reviewErrorOutage",
    timeout: "repo.ingestTimeout",
    cancelled: "repo.ingestCancelled",
    configuration: "repo.ingestConfiguration",
    receipt_invalid: "repo.ingestReceiptInvalid",
    receipt_expired: "repo.ingestReceiptExpired",
    truncated: "repo.ingestErrorTruncated",
    invalid_output: "repo.ingestErrorInvalidOutput",
    preparation: "repo.ingestErrorPreparation",
    capacity: "repo.ingestErrorCapacity",
    stale: "repo.ingestStale",
  } as const
  return lookup[code as keyof typeof lookup] || "repo.reviewFailed"
}

function saveFailureKey(code: string): TranslationKey {
  const lookup = {
    stale: "repo.submission.saveStale",
    lock: "repo.submission.saveLock",
    storage: "repo.submission.saveStorage",
    validation: "repo.submission.saveValidation",
  } as const
  return lookup[code as keyof typeof lookup] || "repo.submission.saveFailed"
}

// Disclosures remain reachable while processing, recovering or reviewing, not
// only in the editable draft. They are collapsed until someone asks for them.
function SubmissionDisclosures() {
  const { t } = useI18n()
  return (
    <details className="text-sm mb-4" data-submission-disclosures>
      <summary className="cursor-pointer">
        {t("repo.submission.disclosures")}
      </summary>
      <p className="mt-2">{t("field.disclosure")}</p>
      <details className="mt-2">
        <summary className="cursor-pointer">{t("field.ruleTitle")}</summary>
        <p className="mt-2">{t("field.rule")}</p>
      </details>
    </details>
  )
}

export default function SubmissionJourney() {
  const { state, dispatch, profileDocument, refreshCanonicalProfile } =
    useStore()
  const repository = state.repository
  const { t } = useI18n()
  const [controller] = useState(() =>
    submissionSessionOwner(sessionKey, {
      transport: createSubmissionTransport(),
    }),
  )
  const initial = controller.getState()
  // A retained field decision does not carry its typed outcome across a page
  // remount. Keep the safe part: the rejected text must be revised before a
  // fresh request, and the reason is explained without exposing machine detail.
  const initialDecision =
    initial.status === "failed" && initial.error === "field_decision"
  const [flow, setFlow] = useState<SubmissionState>(() =>
    snapshotState(controller),
  )
  const [text, setText] = useState(() => initial.input || "")
  const [editing, setEditing] = useState(
    () =>
      initialDecision ||
      (!initial.applicable &&
        (initial.status === "idle" || initial.status === "discarded")),
  )
  const [open, setOpen] = useState(
    () =>
      profileEmpty(repository) ||
      initial.input.trim().length > 0 ||
      initial.applicable,
  )
  const [decision, setDecision] = useState<FieldDecision | null>(null)
  const [blockedInput, setBlockedInput] = useState(() =>
    initialDecision ? initial.input.trim() : "",
  )
  const [runFeedback, setRunFeedback] = useState(() =>
    initialDecision
      ? t("repo.submission.decisionRetained")
      : initial.status === "failed" && initial.error
        ? t(failureKey(initial.error))
        : "",
  )
  const [saveError, setSaveError] = useState("")
  const [saving, setSaving] = useState(false)
  const [completed, setCompleted] = useState(false)
  const [discardIntent, setDiscardIntent] = useState<"edit" | "close">("close")
  const discardDialog = useRef<HTMLDialogElement | null>(null)

  const sync = useCallback(() => {
    setFlow(snapshotState(controller))
  }, [controller])

  const emit = sync

  const bytes = ingestionInputBytes(text)
  const tooLong = bytes > ingestionMaxBytes
  const processing =
    flow.status === "planning" ||
    flow.status === "extracting" ||
    flow.status === "assembling" ||
    flow.status === "reconciling"
  const invalidated = flow.stale || flow.restartRequired
  const hasRetained = flow.input.trim().length > 0 && !flow.applicable
  const recoverable =
    !processing &&
    !flow.applicable &&
    (flow.status === "stopped" ||
      flow.status === "paused" ||
      flow.status === "failed" ||
      flow.status === "expired" ||
      flow.status === "stale") &&
    hasRetained
  const resumable =
    recoverable && !invalidated && !decision && flow.status !== "expired"
  const plan: ReviewPlan | null = controller.getReviewPlan()
  const reviewing = flow.status === "reviewing" && flow.applicable && !!plan
  // Completion feedback belongs to this mount. A later remount with a retained
  // saved session returns to the closed entry point instead of repeating it.
  const saved = completed && open
  const summary: ReviewSummary | null = plan ? reviewSummary(plan) : null
  const saveCheck = reviewing ? controller.canSave(controller.saveGuard()) : null
  const partial =
    flow.progress.skippedClaims > 0 ||
    (flow.coverage?.invalidClaims ?? 0) > 0 ||
    flow.coverage?.capacity === submissionCapacityExhausted
  const missingKeys = !state.apiKey
    ? t("repo.setApiKeyFirst")
    : !state.typesafeKey.trim()
      ? t("field.typesafeMissing")
      : ""

  const currentCandidate = useCallback(
    () => ({
      input: flow.input,
      profile: structuredClone(repository),
      document: profileDocument ? structuredClone(profileDocument) : undefined,
      keys: {
        apiKey: state.apiKey,
        typesafeKey: state.typesafeKey,
      },
    }),
    [
      flow.input,
      repository,
      state.apiKey,
      state.typesafeKey,
      profileDocument,
    ],
  )

  // A canonical Profile, credential or draft change invalidates retained
  // receipts before the next dispatch. The review stays in memory but is never
  // presented as savable; the paste is retained for an explicit restart.
  useEffect(() => {
    if (!flow.input.trim() || !controller.getPin()) return
    const code = controller.refresh(currentCandidate())
    if (code) sync()
  }, [currentCandidate, controller, flow.input, sync])

  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => {
      if (!controller.unsaved() && !text.trim()) return
      event.preventDefault()
      event.returnValue = ""
    }
    window.addEventListener("beforeunload", warn)
    return () => window.removeEventListener("beforeunload", warn)
  }, [controller, text])

  const handleRunError = (error: unknown, submitted: string) => {
    if (error instanceof IngestionError && error.decision) {
      const kind = error.decision.outcome.kind
      if (kind === "request_rephrasing" || kind === "reject_attack")
        setBlockedInput(submitted.trim())
      // A field decision always requires revised input. Show the retained text
      // as editable rather than making the person discover a separate action.
      setEditing(true)
      setDecision(error.decision)
      setRunFeedback("")
      return
    }
    setDecision(null)
    const code = error instanceof IngestionError ? error.code : "outage"
    setRunFeedback(t(failureKey(code)))
  }

  const startRun = async () => {
    if (state.profileError) {
      setRunFeedback(
        t(
          state.profileError === "stale"
            ? "repo.ingestStale"
            : "repo.submission.profileUnavailable",
        ),
      )
      return
    }
    if (!text.trim()) {
      setRunFeedback(t("field.request_information"))
      return
    }
    if (!state.apiKey) {
      setRunFeedback(t("repo.setApiKeyFirst"))
      return
    }
    if (!state.typesafeKey.trim()) {
      setRunFeedback(t("field.typesafeMissing"))
      return
    }
    if (blockedInput && text.trim() === blockedInput) return
    const current = controller.getState()
    if (
      current.status !== "idle" &&
      current.status !== "discarded" &&
      !current.applicable
    )
      controller.discard()
    setDecision(null)
    setRunFeedback("")
    setSaveError("")
    setEditing(false)
    const submitted = text
    try {
      await controller.start(
        {
          input: submitted,
          profile: structuredClone(repository),
          document: profileDocument
            ? structuredClone(profileDocument)
            : undefined,
        },
        { apiKey: state.apiKey, typesafeKey: state.typesafeKey },
        { onStage: emit, onProgress: emit },
      )
    } catch (error) {
      handleRunError(error, submitted)
    } finally {
      sync()
    }
  }

  const resumeRun = async (mode: "resume" | "retry") => {
    setRunFeedback("")
    try {
      const options = {
        candidate: currentCandidate(),
        onStage: emit,
        onProgress: emit,
      }
      if (mode === "resume") await controller.resume(options)
      else await controller.retry(options)
    } catch (error) {
      handleRunError(error, flow.input)
    } finally {
      sync()
    }
  }

  const openDiscard = (intent: "edit" | "close") => {
    setDiscardIntent(intent)
    discardDialog.current?.showModal()
  }

  const confirmDiscard = () => {
    discardDialog.current?.close()
    const retained = flow.input || text
    controller.discard()
    setDecision(null)
    setRunFeedback("")
    setSaveError("")
    setBlockedInput("")
    setCompleted(false)
    if (discardIntent === "edit") {
      setText(retained)
      setEditing(true)
      setOpen(true)
    } else {
      setText("")
      setEditing(profileEmpty(repository))
      setOpen(profileEmpty(repository))
    }
    sync()
  }

  const startEditing = () => openDiscard("edit")

  const save = async () => {
    const guard = controller.saveGuard()
    const check = controller.canSave(guard)
    const candidate = controller.getResult()
    const reviewPlan = controller.getReviewPlan()
    const pin = controller.getPin()
    if (!check.allowed || !candidate || !reviewPlan || !pin?.document) {
      setSaveError(check.code || "state")
      sync()
      return
    }
    if (!summary || summary.selected === 0) {
      setSaveError("empty")
      return
    }
    if (flow.clarification.status === "answering") return
    setSaving(true)
    setSaveError("")
    try {
      const outcome = await saveSubmissionSelect(
        {
          submission: toReviewSubmission(candidate),
          snapshot: pin.document,
          plan: reviewPlan,
          input: pin.input,
        },
        { storage: localStorage },
      )
      if (!outcome.acknowledged) {
        setSaveError(outcome.status === "empty" ? "empty" : outcome.code || "storage")
        sync()
        return
      }
      controller.completeSave()
      setCompleted(true)
      setText("")
      setEditing(false)
      sync()
      await refreshCanonicalProfile()
    } finally {
      setSaving(false)
    }
  }

  const mutate = (action: () => void) => {
    action()
    setSaveError("")
    sync()
  }

  const stageStatus = (() => {
    if (flow.status === "planning") return t("repo.ingestWorking")
    if (flow.status === "extracting")
      return `${t("repo.ingestExtracting")} ${flow.progress.extractedBatches} / ${flow.progress.plannedBatches}`
    if (flow.status === "assembling") return t("repo.submission.assembling")
    if (flow.status === "reconciling")
      return `${t("repo.ingestReconciling")} ${flow.progress.reconciledFocusBatches} / ${flow.progress.plannedFocusBatches}`
    return ""
  })()

  const feedback = decision
    ? t(`field.${decision.outcome.kind}` as TranslationKey)
    : runFeedback

  return (
    <section
      className="p-5 border min-w-0"
      style={{ borderColor: "var(--color-border)" }}
      aria-labelledby="submission-title"
      data-submission-journey
    >
      <h2 id="submission-title" className="text-lg font-bold mb-1">
        {t("repo.ingestTitle")}
      </h2>
      {!open && (
        <>
          <p className="text-sm mb-3">{t("repo.submission.closed")}</p>
          <button
            type="button"
            onClick={() => {
              setCompleted(false)
              setOpen(true)
              setEditing(true)
            }}
            className="px-4 py-2 text-sm"
            style={{
              backgroundColor: "var(--color-fg)",
              color: "var(--color-bg)",
            }}
          >
            {t("repo.submission.add")}
          </button>
        </>
      )}

      {open && !reviewing && !saved && (
        <>
          {(editing || !hasRetained) && (
            <>
              <p className="text-sm mb-3">{t("repo.submission.guide")}</p>
              <details className="text-sm mb-3">
                <summary className="cursor-pointer">
                  {t("field.settings")}
                </summary>
                <label htmlFor="typesafe-key" className="block mt-2">
                  {t("field.typesafeKey")}
                </label>
                <input
                  id="typesafe-key"
                  type="password"
                  autoComplete="off"
                  value={state.typesafeKey}
                  onChange={(e) =>
                    dispatch({
                      type: "SET_TYPESAFE_KEY",
                      payload: e.target.value,
                    })
                  }
                  className="w-full min-w-0 p-2 border mt-1"
                  style={{
                    backgroundColor: "var(--color-card)",
                    borderColor: "var(--color-border)",
                  }}
                />
                <p className="mt-2">{t("field.disclosure")}</p>
              </details>
              <details id="input-use-rule" className="text-sm mb-3">
                <summary className="cursor-pointer">
                  {t("field.ruleTitle")}
                </summary>
                <p className="mt-2">{t("field.rule")}</p>
              </details>
              <label htmlFor="submission-text" className="text-sm block mb-2">
                {t("repo.ingestLabel")}
              </label>
              <textarea
                id="submission-text"
                value={text}
                onChange={(e) => {
                  setText(e.target.value)
                  setRunFeedback("")
                  setDecision(null)
                }}
                rows={9}
                aria-describedby={feedback ? "submission-feedback" : undefined}
                aria-busy={processing}
                placeholder={t("repo.ingestPlaceholder")}
                className="w-full min-w-0 p-3 border text-sm resize-y"
                style={{
                  backgroundColor: "var(--color-card)",
                  borderColor: "var(--color-border)",
                  color: "var(--color-fg)",
                }}
              />
              <p
                className="text-xs mb-3"
                style={{ color: "var(--color-muted-fg)" }}
              >
                {bytes} / {ingestionMaxBytes} {t("repo.bytes")}
              </p>
              {tooLong && (
                <p role="alert" className="text-sm mb-3">
                  {t("repo.submission.tooLong")}
                </p>
              )}
              {missingKeys && (
                <p role="status" className="text-sm mb-3">
                  {missingKeys}
                </p>
              )}
            </>
          )}

          {processing && (
            <div
              role="status"
              aria-live="polite"
              className="text-sm mb-3 space-y-2"
            >
              <p>{stageStatus}</p>
              <button
                type="button"
                onClick={() => {
                  controller.stop()
                  sync()
                }}
                className="border px-3 py-2 text-sm"
                style={{ borderColor: "var(--color-border)" }}
              >
                {t("repo.submission.stop")}
              </button>
            </div>
          )}
          {processing && <SubmissionDisclosures />}

          {hasRetained && !editing && !processing && (
            <div className="text-sm mb-3 space-y-2">
              <p className="font-semibold">{t("repo.submission.retained")}</p>
              <pre
                className="max-h-40 overflow-auto whitespace-pre-wrap break-words text-xs p-2 border"
                style={{ borderColor: "var(--color-border)" }}
              >
                {flow.input}
              </pre>
            </div>
          )}

          {recoverable && !processing && !editing && (
            <div className="text-sm mb-3 space-y-2" role="status">
              {invalidated && (
                <p role="alert">{t("repo.submission.invalidated")}</p>
              )}
              <div className="flex flex-wrap gap-2">
                {resumable && (
                  <button
                    type="button"
                    onClick={() =>
                      resumeRun(
                        flow.status === "stopped" || flow.status === "paused"
                          ? "resume"
                          : "retry",
                      )
                    }
                    className="px-4 py-2 text-sm"
                    style={{
                      backgroundColor: "var(--color-fg)",
                      color: "var(--color-bg)",
                    }}
                  >
                    {flow.status === "stopped" || flow.status === "paused"
                      ? t("repo.submission.resume")
                      : t("repo.submission.retry")}
                  </button>
                )}
                <button
                  type="button"
                  onClick={startEditing}
                  className="border px-4 py-2 text-sm"
                  style={{ borderColor: "var(--color-border)" }}
                >
                  {invalidated
                    ? t("repo.submission.restart")
                    : t("repo.submission.editText")}
                </button>
                <button
                  type="button"
                  onClick={() => openDiscard("close")}
                  className="border px-4 py-2 text-sm"
                  style={{ borderColor: "var(--color-border)" }}
                >
                  {t("repo.ingestDiscard")}
                </button>
              </div>
            </div>
          )}
          {recoverable && !processing && !editing && (
            <SubmissionDisclosures />
          )}

          {!processing && !saved && editing && (
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={startRun}
                disabled={
                  !editing ||
                  !text.trim() ||
                  tooLong ||
                  !!state.profileError ||
                  (!!blockedInput && text.trim() === blockedInput)
                }
                className="px-4 py-2 text-sm disabled:opacity-50"
                style={{
                  backgroundColor: "var(--color-fg)",
                  color: "var(--color-bg)",
                }}
              >
                {t("repo.submission.prepare")}
              </button>
              {(text.trim() || hasRetained) && (
                <button
                  type="button"
                  onClick={() => openDiscard("close")}
                  className="border px-4 py-2 text-sm"
                  style={{ borderColor: "var(--color-border)" }}
                >
                  {t("repo.ingestDiscard")}
                </button>
              )}
            </div>
          )}

          {feedback && (
            <p
              id="submission-feedback"
              role="alert"
              className="text-sm mt-3"
              style={{ color: "var(--color-accent)" }}
            >
              {feedback}{" "}
              {decision?.outcome.kind === "reject_attack" && (
                <a
                  href="#input-use-rule"
                  onClick={() => {
                    const rule =
                      document.querySelector<HTMLDetailsElement>(
                        "#input-use-rule",
                      )
                    if (rule) rule.open = true
                  }}
                  className="underline"
                >
                  {t("field.ruleTitle")}
                </a>
              )}
            </p>
          )}
        </>
      )}

      {reviewing && plan && (
        <>
          <details className="text-sm mb-4">
            <summary className="cursor-pointer">
              {t("repo.submission.submitted", { bytes })}
            </summary>
            <pre
              className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap break-words text-xs p-2 border"
              style={{ borderColor: "var(--color-border)" }}
            >
              {flow.input}
            </pre>
          </details>
          {partial && (
            <p role="note" className="text-sm mb-4">
              {t("repo.submission.partial")}
            </p>
          )}
          {state.profileError === "stale" && (
            <p role="alert" className="text-sm mb-4">
              {t("repo.ingestStale")}
            </p>
          )}
          <SubmissionDisclosures />
          <SubmissionReview
            plan={plan}
            disabled={saving || !!state.profileError}
            onSelectUnit={(unitId, selected) =>
              mutate(() =>
                controller.selectUnit(
                  unitId,
                  selected ? "selected" : "pending",
                ),
              )
            }
            onSelectGroup={(destinationId, selected) =>
              mutate(() =>
                controller.selectGroup(
                  destinationId,
                  selected ? "selected" : "pending",
                ),
              )
            }
            onSelectAdditions={() => mutate(() => controller.selectAdditions())}
            onClearSelection={() => mutate(() => controller.clearSelection())}
            onEditUnit={(unitId, field, value) =>
              mutate(() => controller.editUnit(unitId, field, value))
            }
            onAnswerUnit={async (unitId, answer) => {
              const unit = plan.units.find(
                (candidate) => candidate.id === unitId,
              )
              try {
                if (!unit) throw new IngestionError("input")
                await controller.answerClarification(
                  unit.claimIds[0],
                  answer,
                  { onStage: emit, onProgress: emit },
                )
              } finally {
                sync()
              }
            }}
          />
          <div
            className="mt-5 border-t pt-4 flex flex-wrap items-center gap-3"
            style={{ borderColor: "var(--color-border)" }}
            data-submission-actions
          >
            <button
              type="button"
              onClick={save}
              disabled={
                saving ||
                !!state.profileError ||
                flow.clarification.status === "answering" ||
                !summary ||
                summary.selected === 0 ||
                !saveCheck?.allowed
              }
              className="px-4 py-2 text-sm disabled:opacity-50"
              style={{
                backgroundColor: "var(--color-fg)",
                color: "var(--color-bg)",
              }}
            >
              {saving
                ? t("repo.submission.saving")
                : saveError
                  ? t("repo.submission.retrySave")
                  : t("repo.submission.save")}
            </button>
            <p className="text-sm" role="status" aria-live="polite">
              {summary && summary.selected > 0
                ? t("repo.submission.selection", {
                    selected: summary.selected,
                    selectable: summary.selectable,
                    attention: summary.attention,
                  })
                : t("repo.submission.selectionNone", {
                    attention: summary?.attention ?? 0,
                  })}
            </p>
            <button
              type="button"
              onClick={() => openDiscard("close")}
              disabled={saving}
              className="border px-4 py-2 text-sm disabled:opacity-50"
              style={{ borderColor: "var(--color-border)" }}
            >
              {t("repo.ingestDiscard")}
            </button>
          </div>
          {saveError && (
            <p role="alert" className="text-sm mt-3">
              {t(saveFailureKey(saveError))}
            </p>
          )}
        </>
      )}

      {saved && (
        <div className="space-y-3" role="status">
          <p className="text-sm font-semibold">{t("repo.submission.saved")}</p>
          <p className="text-sm">{t("repo.submission.savedNote")}</p>
          <button
            type="button"
            onClick={() => {
              controller.discard()
              setCompleted(false)
              setText("")
              setEditing(true)
              setOpen(true)
              sync()
            }}
            className="px-4 py-2 text-sm"
            style={{
              backgroundColor: "var(--color-fg)",
              color: "var(--color-bg)",
            }}
          >
            {t("repo.submission.addMore")}
          </button>
        </div>
      )}

      <dialog
        ref={discardDialog}
        aria-labelledby="submission-discard-title"
        aria-describedby="submission-discard-description"
        className="p-0 max-w-md w-[calc(100%-2rem)] backdrop:bg-black/50"
        style={{
          backgroundColor: "var(--color-card)",
          color: "var(--color-fg)",
          border: "1px solid var(--color-border)",
        }}
      >
        <div className="p-5">
          <h2 id="submission-discard-title" className="text-lg font-bold mb-2">
            {t("repo.discardTitle")}
          </h2>
          <p id="submission-discard-description" className="text-sm mb-4">
            {t("repo.submission.discardDescription")}
          </p>
          <div className="flex flex-wrap justify-end gap-2">
            <button
              type="button"
              onClick={() => discardDialog.current?.close()}
              className="border px-4 py-2 text-sm"
              style={{ borderColor: "var(--color-border)" }}
            >
              {t("repo.keepEditing")}
            </button>
            <button
              type="button"
              onClick={confirmDiscard}
              className="px-4 py-2 text-sm"
              style={{
                backgroundColor: "var(--color-fg)",
                color: "var(--color-bg)",
              }}
            >
              {t("repo.ingestDiscard")}
            </button>
          </div>
        </div>
      </dialog>
    </section>
  )
}

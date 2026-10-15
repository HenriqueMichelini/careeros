import { useEffect, useRef, useState } from "react"
import { useI18n, useStore } from "../lib/store"
import {
  beforeValue,
  ingestProfile,
  IngestionError,
  mergeClarification,
  previewIngestion,
  type ApplyAnswerReview,
  type IngestionResult,
} from "../lib/ingestion"
import { profileView, type ProfileDocument } from "../lib/profileDocument"
import { profileFieldKey } from "../lib/profileLabels"
import type { TranslationKey } from "../lib/i18n"
import { IngestionClarification } from "./IngestionClarification"
import {
  ClaimOutcomeDetails,
  IngestionCoverageNotice,
} from "./IngestionOutcomes"

export interface ApplyAnswer {
  id: string
  text: string
  requirement: string
  language: "en" | "pt-BR"
}
type Review = {
  answerId: string
  input: string
  snapshot: ProfileDocument
  result: IngestionResult
}

export default function ApplyProfileEnrichment({
  answers,
  visible,
  generationActive,
  onBusyChange,
}: {
  answers: ApplyAnswer[]
  visible: boolean
  generationActive: boolean
  onBusyChange: (busy: boolean) => void
}) {
  const { state, profileDocument, applyIngestionProposal } = useStore()
  const { t } = useI18n()
  const [inputs, setInputs] = useState<Record<string, string>>({})
  const [targets, setTargets] = useState<Record<string, string>>({})
  const [review, setReview] = useState<Review | null>(null)
  const [reviewInvalidated, setReviewInvalidated] = useState(false)
  const [busy, setBusy] = useState(false)
  const [saving, setSaving] = useState(false)
  const [feedback, setFeedback] = useState("")
  const [savedIds, setSavedIds] = useState<string[]>([])
  const [dismissed, setDismissed] = useState<string[]>([])
  const [blocked, setBlocked] = useState<string | null>(null)
  const attempt = useRef(0)
  const controller = useRef<AbortController | null>(null)
  const inFlight = useRef(false)
  const current = useRef({
    profileDocument,
    generationActive,
    profileError: state.profileError,
  })
  current.current = {
    profileDocument,
    generationActive,
    profileError: state.profileError,
  }
  const stale =
    !!review &&
    JSON.stringify(review.snapshot) !== JSON.stringify(profileDocument)
  function setWorking(value: boolean) {
    inFlight.current = value
    setBusy(value)
    onBusyChange(value)
  }
  function cancel() {
    if (saving) return
    attempt.current++
    controller.current?.abort()
    setWorking(false)
  }
  useEffect(
    () => () => {
      attempt.current++
      controller.current?.abort()
    },
    [],
  )
  useEffect(() => {
    // Late responses cannot adopt another Profile snapshot. Preserve recovery text.
    attempt.current++
    controller.current?.abort()
    if (state.profileError) setReviewInvalidated(true)
    if (!saving) setWorking(false)
  }, [profileDocument, generationActive, state.profileError])

  function showError(error: unknown) {
    if (error instanceof IngestionError && error.decision) {
      const outcome = error.decision.outcome
      if (
        outcome.kind === "reject_attack" ||
        outcome.kind === "request_rephrasing"
      )
        setBlocked(review?.input ?? null)
      setFeedback(
        t(
          (outcome.kind === "service_failure"
            ? `field.failure.${outcome.reason}`
            : `field.${outcome.kind}`) as TranslationKey,
        ),
      )
    } else
      setFeedback(
        t(
          error instanceof IngestionError && error.code === "stale"
            ? "repo.ingestStale"
            : "enrichment.failed",
        ),
      )
  }
  async function requestReview(
    answer: ApplyAnswer,
    clarification?: {
      claimId: string
      answer: string
    },
  ) {
    if (
      inFlight.current ||
      generationActive ||
      !profileDocument ||
      state.profileError
    )
      return
    const input = inputs[answer.id] ?? answer.text
    if (!input.trim() || blocked === input) return
    const snapshot = clarification
      ? review?.snapshot
      : structuredClone(profileDocument)
    if (!snapshot || (clarification && stale)) throw new IngestionError("stale")
    const selected = profileDocument.entities.find(
      (e) => e.id === targets[answer.id],
    )
    if (targets[answer.id] && !selected) {
      setFeedback(t("repo.ingestStale"))
      return
    }
    const metadata: ApplyAnswerReview = clarification
      ? review!.result.applyAnswer!
      : {
          version: 1,
          requestId: crypto.randomUUID(),
          language: answer.language,
          profile: {
            profileId: snapshot.id,
            id: snapshot.id,
            revision: snapshot.revision,
          },
          ...(selected
            ? {
                target: {
                  profileId: snapshot.id,
                  id: selected.id,
                  revision: selected.revision,
                },
              }
            : {}),
        }
    const id = ++attempt.current
    controller.current = new AbortController()
    setWorking(true)
    setFeedback("")
    if (!clarification) setReview(null)
    try {
      const result = await ingestProfile(
        input,
        profileView(snapshot),
        state.apiKey,
        controller.current.signal,
        state.typesafeKey,
        snapshot,
        undefined,
        clarification
          ? {
              claim: review!.result.claims.find(
                (c) => c.id === clarification.claimId,
              )!,
              answer: clarification.answer,
            }
          : undefined,
        metadata,
      )
      if (id !== attempt.current) {
        if (clarification) throw new IngestionError("cancelled")
        return
      }
      if (
        JSON.stringify(current.current.profileDocument) !==
          JSON.stringify(snapshot) ||
        current.current.generationActive ||
        current.current.profileError
      )
        throw new IngestionError("stale")
      setReviewInvalidated(false)
      setReview({
        answerId: answer.id,
        input,
        snapshot,
        result: clarification
          ? mergeClarification(
              review!.result,
              clarification.claimId,
              result,
              input,
              profileView(snapshot),
              snapshot,
            )
          : result,
      })
    } catch (error) {
      if (id !== attempt.current) {
        if (clarification) throw new IngestionError("cancelled")
        return
      }
      if (clarification) throw error
      showError(error)
      if (
        error instanceof IngestionError &&
        ["reject_attack", "request_rephrasing"].includes(
          error.decision?.outcome.kind ?? "",
        )
      )
        setBlocked(input)
    } finally {
      if (id === attempt.current) setWorking(false)
    }
  }
  async function save() {
    if (
      inFlight.current ||
      !review ||
      reviewInvalidated ||
      stale ||
      generationActive ||
      state.profileError ||
      !review.result.operations.some((o) => o.approved)
    )
      return
    setWorking(true)
    setSaving(true)
    setFeedback("")
    try {
      const saved = await applyIngestionProposal(
        review.snapshot,
        review.input,
        review.result,
      )
      if (saved) {
        setSavedIds((ids) => [...ids, review.answerId])
        setReview(null)
        setFeedback("")
      } else setFeedback(t("enrichment.failed"))
    } catch (error) {
      showError(error)
    } finally {
      setSaving(false)
      setWorking(false)
    }
  }
  if (!visible || !answers.some((a) => !dismissed.includes(a.id))) return null
  const disabled = busy || generationActive || !!state.profileError
  const view = review ? profileView(review.snapshot) : null
  const preview =
    review && view
      ? (() => {
          try {
            return previewIngestion(
              view,
              JSON.stringify(view),
              review.result.operations,
            )
          } catch {
            return null
          }
        })()
      : null
  const ownerLabel = (doc: ProfileDocument, id: string) =>
    doc.facts
      .filter(
        (f) =>
          f.owner.id === id && ["name", "company", "title"].includes(f.field),
      )
      .map((f) => String(f.value))
      .join(" · ")
  return (
    <section
      data-apply-enrichment
      className="max-w-6xl mx-auto px-6 py-8 space-y-4 break-words"
      aria-label={t("enrichment.title")}
    >
      <h2 className="text-xl font-bold">{t("enrichment.title")}</h2>
      <p className="text-sm">{t("enrichment.disclosure")}</p>
      <p className="text-sm">{t("enrichment.continuity")}</p>
      {feedback && <p role="status">{feedback}</p>}
      {stale && <p role="alert">{t("repo.ingestStale")}</p>}
      {state.profileError && <p role="alert">{t("enrichment.reload")}</p>}
      {answers
        .filter((a) => !dismissed.includes(a.id))
        .map((answer) => (
          <article
            key={answer.id}
            className="border p-4 space-y-3 min-w-0"
            style={{ borderColor: "var(--color-border)" }}
          >
            <p className="text-sm">{answer.requirement}</p>
            <label className="block text-sm">
              {t(
                savedIds.includes(answer.id)
                  ? "enrichment.savedAnswer"
                  : "enrichment.answer",
              )}
              <textarea
                data-enrichment-answer
                className="block w-full bg-transparent border p-2 mt-2"
                rows={3}
                maxLength={2000}
                value={inputs[answer.id] ?? answer.text}
                readOnly={disabled || savedIds.includes(answer.id)}
                onChange={(e) => {
                  setInputs((v) => ({ ...v, [answer.id]: e.target.value }))
                  setReview(null)
                  setFeedback("")
                }}
              />
            </label>
            {savedIds.includes(answer.id) ? (
              <p data-enrichment-saved role="status">
                {t("enrichment.saved")}
              </p>
            ) : (
              <>
                <label className="block text-sm">
                  {t("enrichment.owner")}
                  <select
                    className="block w-full bg-transparent border p-2 mt-2"
                    value={targets[answer.id] ?? ""}
                    disabled={disabled}
                    onChange={(e) => {
                      setTargets((v) => ({ ...v, [answer.id]: e.target.value }))
                      setReview(null)
                    }}
                  >
                    <option value="">{t("enrichment.noOwner")}</option>
                    {profileDocument?.entities
                      .filter((e) =>
                        ["projects", "experience"].includes(e.kind),
                      )
                      .map((e) => (
                        <option value={e.id} key={e.id}>
                          {ownerLabel(profileDocument, e.id)}
                        </option>
                      ))}
                  </select>
                </label>
                <button
                  data-enrichment-review
                  type="button"
                  disabled={
                    disabled ||
                    !(inputs[answer.id] ?? answer.text).trim() ||
                    blocked === (inputs[answer.id] ?? answer.text)
                  }
                  onClick={() => void requestReview(answer)}
                  className="border px-3 py-2 text-sm disabled:opacity-50"
                >
                  {t("enrichment.review")}
                </button>
              </>
            )}
            <button
              data-enrichment-dismiss
              type="button"
              disabled={disabled}
              className="border px-3 py-2 text-sm ml-2 disabled:opacity-50"
              onClick={() => {
                if (review?.answerId === answer.id) setReview(null)
                setDismissed((ids) => [...ids, answer.id])
              }}
            >
              {t(
                savedIds.includes(answer.id)
                  ? "enrichment.hide"
                  : "enrichment.dismiss",
              )}
            </button>
            {review?.answerId === answer.id && (
              <div className="space-y-3">
                <IngestionCoverageNotice result={review.result} />
                {review.result.claims.map((claim) => (
                  <div
                    key={claim.id}
                    className="border p-3 space-y-2 min-w-0"
                    style={{ borderColor: "var(--color-border)" }}
                  >
                    <p>{claim.text}</p>
                    <blockquote className="whitespace-pre-wrap">
                      {t("repo.ingestSource")}: “{claim.source}”
                    </blockquote>
                    {claim.meaning && (
                      <p className="text-sm">
                        {[
                          claim.meaning.assertion,
                          claim.meaning.intent,
                          claim.meaning.certainty,
                          claim.meaning.temporal.precision,
                        ]
                          .map((v) => t(`profile.facts.${v}` as TranslationKey))
                          .join(" · ")}{" "}
                        · {claim.meaning.temporal.wording}
                      </p>
                    )}
                    {claim.supportingSources?.map((s, i) => (
                      <blockquote key={i}>
                        {t("repo.clarificationEvidence")}: “{s.source}”
                      </blockquote>
                    ))}
                    <ClaimOutcomeDetails
                      outcome={review.result.outcomes?.find(
                        (o) => o.claimId === claim.id,
                      )}
                      document={review.snapshot}
                      claims={review.result.claims}
                    />
                    {(claim.question ||
                      review.result.outcomes?.some(
                        (o) =>
                          o.claimId === claim.id &&
                          [
                            "clarification",
                            "contradiction",
                            "correction",
                          ].includes(o.kind),
                      )) && (
                      <IngestionClarification
                        claimId={claim.id}
                        question={claim.question}
                        busy={busy}
                        disabled={
                          stale || generationActive || !!state.profileError
                        }
                        onAnswer={(text) =>
                          requestReview(answer, {
                            claimId: claim.id,
                            answer: text,
                          })
                        }
                        onCancel={cancel}
                      />
                    )}
                  </div>
                ))}
                {review.result.operations.map((op, index) => {
                  const entity = review.snapshot.entities.find(
                    (e) => e.kind === op.target && e.legacyId === op.entryId,
                  )
                  const projected = preview
                    ? beforeValue(preview, op)
                    : op.value
                  return (
                    <div
                      key={index}
                      className="border p-3 space-y-2"
                      style={{ borderColor: "var(--color-border)" }}
                    >
                      <label>
                        <input
                          data-enrichment-operation
                          type="checkbox"
                          checked={op.approved}
                          disabled={disabled || stale}
                          onChange={(e) =>
                            setReview(
                              (r) =>
                                r && {
                                  ...r,
                                  result: {
                                    ...r.result,
                                    operations: r.result.operations.map(
                                      (o, i) =>
                                        i === index
                                          ? { ...o, approved: e.target.checked }
                                          : o,
                                    ),
                                  },
                                },
                            )
                          }
                        />{" "}
                        {t(
                          profileFieldKey(
                            op.target === op.field ? "profile" : op.target,
                            op.field,
                          ),
                        )}{" "}
                        ·{" "}
                        {entity
                          ? ownerLabel(review.snapshot, entity.id)
                          : op.entryId
                            ? t("enrichment.newOwner")
                            : t("enrichment.profile")}{" "}
                        ·{" "}
                        {t(`repo.ingestAction.${op.action}` as TranslationKey)}
                      </label>
                      <p className="whitespace-pre-wrap text-sm">
                        {t("enrichment.proposed")}: {op.value || "—"}
                      </p>
                      <p className="whitespace-pre-wrap text-sm">
                        {t("repo.ingestBefore")}:{" "}
                        {beforeValue(view!, op) || "—"}
                      </p>
                      <p className="whitespace-pre-wrap text-sm">
                        {t("repo.ingestAfter")}: {projected || "—"}
                      </p>
                      {(op.supportingClaimIds ?? [op.claimId]).map((id) => (
                        <blockquote key={id} className="text-sm">
                          {t("repo.ingestAcceptedSource")}: “
                          {
                            review.result.claims.find((c) => c.id === id)
                              ?.source
                          }
                          ”
                        </blockquote>
                      ))}
                    </div>
                  )
                })}
                {!preview && <p role="alert">{t("enrichment.dependencies")}</p>}
                <button
                  data-enrichment-save
                  type="button"
                  disabled={
                    disabled ||
                    stale ||
                    reviewInvalidated ||
                    !preview ||
                    !review.result.operations.some((o) => o.approved)
                  }
                  onClick={() => void save()}
                  className="border px-3 py-2 text-sm disabled:opacity-50"
                >
                  {t("enrichment.save")}
                </button>
              </div>
            )}
          </article>
        ))}
      {busy && !saving && (
        <button
          data-enrichment-cancel
          type="button"
          onClick={cancel}
          className="border px-3 py-2 text-sm"
        >
          {t("enrichment.cancel")}
        </button>
      )}
      <p className="text-xs">{t("enrichment.inspect")}</p>
    </section>
  )
}

import CareerSourceSupport from "./CareerSourceSupport"
import { useState } from "react"
import { useI18n } from "../lib/store"
import {
  acceptResume,
  correctResume,
  removeResumeClaim,
  resumeReviewCurrent,
  type AcceptedResume,
  type ResumeReview,
} from "../lib/resumeReview"
import type { ProfileDocument } from "../lib/profileDocument"
import type { TranslationKey } from "../lib/i18n"

export default function ResumeReviewPanel({
  review,
  accepted,
  document,
  onAccept,
}: {
  review: ResumeReview
  accepted?: AcceptedResume
  document: ProfileDocument | null
  onAccept: (review: ResumeReview, accepted: AcceptedResume) => void
}) {
  const { t } = useI18n()
  const [draft, setDraft] = useState(review)
  const [editing, setEditing] = useState(!accepted)
  const [correction, setCorrection] = useState<number | null>(null)
  const [evidence, setEvidence] = useState("")
  const [error, setError] = useState(false)
  const current = resumeReviewCurrent(draft, document)
  const active = draft.claims.filter((c) => !c.removed)
  const ready =
    current &&
    !!active.length &&
    active.every((c) => c.state === "supported") &&
    correction === null
  const buttonClass =
    "border px-3 py-2 text-xs disabled:opacity-40 border-[var(--color-border)]"
  function cancel() {
    setDraft(review)
    setCorrection(null)
    setEvidence("")
    setError(false)
    setEditing(false)
  }
  return (
    <section
      className="resume-review mb-6 space-y-4 border-b pb-6 border-[var(--color-border)]"
      aria-label={t("resumeReview.title")}
    >
      <h2 className="text-lg font-semibold">{t("resumeReview.title")}</h2>
      <p className="text-sm text-[var(--color-muted-fg)]">
        {t("resumeReview.limits")}
      </p>
      {!current && <p role="alert">{t("resumeReview.stale")}</p>}
      {!editing ? (
        <button
          className={buttonClass}
          onClick={() => {
            setDraft(review)
            setEditing(true)
          }}
        >
          {t("resumeReview.edit")}
        </button>
      ) : (
        <>
          {draft.check === "unavailable" && (
            <p role="status">{t("resumeReview.unavailable")}</p>
          )}
          <ol className="space-y-4">
            {draft.claims.map(
              (claim, index) =>
                !claim.removed && (
                  <li
                    key={index}
                    className="space-y-2 border p-3 border-[var(--color-border)]"
                  >
                    <p className="text-xs uppercase tracking-wide">
                      {t(
                        claim.userSupport
                          ? "resumeReview.userAuthored"
                          : ("resumeReview." + claim.state) as TranslationKey,
                      )}
                    </p>
                    <p className="text-sm whitespace-pre-wrap break-words">
                      {claim.text}
                    </p>
                    {claim.concerns.length > 0 && (
                      <ul className="text-sm text-[var(--color-accent)]">
                        {claim.concerns.map((concern, i) => (
                          <li key={i}>
                            {t("resumeReview.concern")}:{" "}
                            {t(
                              ("resumeReview.reason." +
                                concern) as TranslationKey,
                            ) ===
                            "resumeReview.reason." + concern
                              ? concern
                              : t(
                                  ("resumeReview.reason." +
                                    concern) as TranslationKey,
                                )}
                          </li>
                        ))}
                      </ul>
                    )}
                    <details className="text-sm">
                      <summary className="cursor-pointer">
                        {t("resumeReview.support")}
                      </summary>
                      {claim.userSupport ? (
                        <p className="mt-2 whitespace-pre-wrap">
                          {t("resumeReview.userAuthored")}:{" "}
                          {claim.userSupport.evidence}
                        </p>
                      ) : (
                        <CareerSourceSupport
                          sources={claim.sources}
                          facts={draft.facts}
                          profileId={draft.sourceSnapshot.id}
                        />
                      )}
                    </details>
                    <div className="flex flex-wrap gap-2">
                      <button
                        className={buttonClass}
                        disabled={!current}
                        onClick={() => {
                          setCorrection(index)
                          setEvidence(claim.text)
                          setError(false)
                        }}
                      >
                        {t("resumeReview.correct")}
                      </button>
                      <button
                        className={buttonClass}
                        onClick={() => {
                          setDraft(removeResumeClaim(draft, index))
                          if (correction === index) setCorrection(null)
                        }}
                      >
                        {t("resumeReview.remove")}
                      </button>
                    </div>
                    {correction === index && (
                      <div className="space-y-2">
                        <label
                          className="block text-sm"
                          htmlFor={"resume-correction-" + index}
                        >
                          {t("resumeReview.correctionLabel")}
                        </label>
                        <p className="text-xs text-[var(--color-muted-fg)]">
                          {t("resumeReview.correctionHelp")}
                        </p>
                        <textarea
                          id={"resume-correction-" + index}
                          className="w-full min-h-24 border p-2 text-sm bg-[var(--color-bg)] border-[var(--color-border)]"
                          value={evidence}
                          maxLength={2000}
                          onChange={(e) => setEvidence(e.target.value)}
                        />
                        {error && (
                          <p role="alert">
                            {t("resumeReview.correctionError")}
                          </p>
                        )}
                        <div className="flex flex-wrap gap-2">
                          <button
                            className={buttonClass}
                            disabled={!current}
                            onClick={() => {
                              try {
                                setDraft(correctResume(draft, index, evidence))
                                setCorrection(null)
                                setError(false)
                              } catch {
                                setError(true)
                              }
                            }}
                          >
                            {t("resumeReview.useCorrection")}
                          </button>
                          <button
                            className={buttonClass}
                            onClick={() => {
                              setCorrection(null)
                              setError(false)
                            }}
                          >
                            {t("resumeReview.cancel")}
                          </button>
                        </div>
                      </div>
                    )}
                  </li>
                ),
            )}
          </ol>
          <p className="text-sm">{t("resumeReview.acceptHelp")}</p>
          <div className="flex flex-wrap gap-2">
            <button
              className={buttonClass}
              disabled={!ready}
              onClick={() => {
                onAccept(draft, acceptResume(draft, document))
                setEditing(false)
              }}
            >
              {t("resumeReview.accept")}
            </button>
            {accepted && (
              <button className={buttonClass} onClick={cancel}>
                {t("resumeReview.cancel")}
              </button>
            )}
            <button
              className={buttonClass}
              onClick={() => {
                setDraft(review)
                setCorrection(null)
                setError(false)
              }}
            >
              {t("resumeReview.reset")}
            </button>
          </div>
        </>
      )}
      {accepted && (
        <p className="text-sm" role="status">
          {editing
            ? t("resumeReview.lastAccepted")
            : t("resumeReview.accepted")}
        </p>
      )}
    </section>
  )
}

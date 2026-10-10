import CareerSourceSupport from "./CareerSourceSupport"
import { useState } from "react"
import { useI18n } from "../lib/store"
import type { ProfileDocument } from "../lib/profileDocument"
import {
  acceptArtifacts,
  artifactClaimResolved,
  artifactsCurrent,
  correctArtifact,
  removeArtifactClaim,
  type ArtifactReview,
  type AcceptedArtifacts,
} from "../lib/artifactReview"
import type { TranslationKey } from "../lib/i18n"

export default function ArtifactReviewPanel({
  review,
  accepted,
  document,
  onAccept,
}: {
  review: ArtifactReview
  accepted?: AcceptedArtifacts
  document: ProfileDocument | null
  onAccept: (review: ArtifactReview, accepted: AcceptedArtifacts) => void
}) {
  const { t } = useI18n()
  const [draft, setDraft] = useState(review)
  const [editing, setEditing] = useState(!accepted)
  const [showAll, setShowAll] = useState(false)
  const [index, setIndex] = useState<number | null>(null)
  const [text, setText] = useState("")
  const [error, setError] = useState(false)
  const current = artifactsCurrent(draft, document)
  const ready =
    current &&
    index === null &&
    draft.claims.every((c) => c.removed || artifactClaimResolved(c))
  const button =
    "border px-3 py-2 text-xs disabled:opacity-40 border-[var(--color-border)]"
  return (
    <section
      aria-label={t("artifactReview.title")}
      className="artifact-review mb-6 space-y-4 border-b pb-6 border-[var(--color-border)]"
    >
      <h2 className="text-lg font-semibold">{t("artifactReview.title")}</h2>
      <p className="text-sm text-[var(--color-muted-fg)]">
        {t("resumeReview.limits")}
      </p>
      {!current && <p role="alert">{t("resumeReview.stale")}</p>}
      {!editing ? (
        <button className={button} onClick={() => setEditing(true)}>
          {t("resumeReview.edit")}
        </button>
      ) : (
        <>
          {draft.check === "unavailable" && (
            <p role="status">{t("resumeReview.unavailable")}</p>
          )}
          <button className={button} onClick={() => setShowAll(!showAll)}>
            {t(showAll ? "artifactReview.concernsOnly" : "artifactReview.all")}
          </button>
          <ol className="space-y-4">
            {draft.claims.map(
              (c, i) =>
                !c.removed &&
                (showAll || c.state !== "supported" || index === i) && (
                  <li
                    key={i}
                    className="space-y-2 border p-3 border-[var(--color-border)]"
                  >
                    <p className="text-xs uppercase">
                      {t(("artifactReview.field." + c.field) as TranslationKey)}{" "}
                      · {t(("resumeReview." + c.state) as TranslationKey)}
                    </p>
                    <p className="text-sm whitespace-pre-wrap break-words">
                      {c.text}
                    </p>
                    {c.concerns.map((reason, n) => (
                      <p key={n} className="text-sm text-[var(--color-accent)]">
                        {t("resumeReview.concern")}:{" "}
                        {t(
                          ("resumeReview.reason." + reason) as TranslationKey,
                        ) ===
                        "resumeReview.reason." + reason
                          ? reason
                          : t(
                              ("resumeReview.reason." +
                                reason) as TranslationKey,
                            )}
                      </p>
                    ))}
                    <details className="text-sm">
                      <summary>{t("resumeReview.support")}</summary>
                      {c.userSupport ? (
                        <p>
                          {t("resumeReview.userAuthored")}:{" "}
                          {c.userSupport.evidence}
                        </p>
                      ) : (
                        <>
                          <CareerSourceSupport
                            sources={c.sources}
                            facts={draft.facts}
                            profileId={draft.sourceSnapshot.id}
                          />
                          {c.jobSources.map((s, n) => (
                            <blockquote
                              key={n}
                              className="mt-2 border-l pl-3 whitespace-pre-wrap"
                            >
                              {t("artifactReview.jobSource")}: {s.quote}
                            </blockquote>
                          ))}
                        </>
                      )}
                    </details>
                    <div className="flex flex-wrap gap-2">
                      <button
                        className={button}
                        disabled={!current}
                        onClick={() => {
                          setIndex(i)
                          setText(c.text)
                          setError(false)
                        }}
                      >
                        {t("resumeReview.correct")}
                      </button>
                      {c.field !== "applicationAnswers" && (
                        <button
                          className={button}
                          disabled={!current}
                          onClick={() => {
                            setDraft(removeArtifactClaim(draft, i))
                            setIndex(null)
                          }}
                        >
                          {t("resumeReview.remove")}
                        </button>
                      )}
                    </div>
                    {index === i && (
                      <div className="space-y-2">
                        <label htmlFor={"artifact-correction-" + i}>
                          {t("resumeReview.correctionLabel")}
                        </label>
                        <p className="text-xs">
                          {t("artifactReview.correctionHelp")}
                        </p>
                        <textarea
                          id={"artifact-correction-" + i}
                          className="w-full min-h-24 border p-2 text-sm bg-[var(--color-bg)] border-[var(--color-border)]"
                          maxLength={8000}
                          value={text}
                          onChange={(e) => setText(e.target.value)}
                        />
                        {error && (
                          <p role="alert">{t("artifactReview.error")}</p>
                        )}
                        <button
                          className={button}
                          disabled={!current}
                          onClick={() => {
                            try {
                              setDraft(correctArtifact(draft, i, text))
                              setIndex(null)
                              setError(false)
                            } catch {
                              setError(true)
                            }
                          }}
                        >
                          {t("resumeReview.useCorrection")}
                        </button>
                        <button
                          className={button}
                          onClick={() => setIndex(null)}
                        >
                          {t("resumeReview.cancel")}
                        </button>
                      </div>
                    )}
                  </li>
                ),
            )}
          </ol>
          <p className="text-sm">{t("artifactReview.acceptHelp")}</p>
          {error && index === null && (
            <p role="alert">{t("artifactReview.error")}</p>
          )}
          <div className="flex flex-wrap gap-2">
            <button
              className={button}
              disabled={!ready}
              onClick={() => {
                try {
                  onAccept(draft, acceptArtifacts(draft, document))
                  setEditing(false)
                  setError(false)
                } catch {
                  setError(true)
                }
              }}
            >
              {t("artifactReview.accept")}
            </button>
            <button
              className={button}
              onClick={() => {
                setDraft(review)
                setIndex(null)
                setError(false)
                if (accepted) setEditing(false)
              }}
            >
              {t(accepted ? "resumeReview.cancel" : "resumeReview.reset")}
            </button>
          </div>
        </>
      )}
      {accepted && (
        <p role="status" className="text-sm">
          {t(editing ? "resumeReview.lastAccepted" : "artifactReview.accepted")}
        </p>
      )}
    </section>
  )
}

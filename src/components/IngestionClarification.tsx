import { useState } from "react"
import { useI18n } from "../lib/store"
import { IngestionError, ingestionInputBytes } from "../lib/ingestion"

export function IngestionClarification({
  claimId,
  question,
  busy,
  disabled,
  onAnswer,
  onCancel,
}: {
  claimId: string
  question: string
  busy: boolean
  disabled: boolean
  onAnswer: (answer: string) => Promise<void>
  onCancel: () => void
}) {
  const { t } = useI18n()
  const [answer, setAnswer] = useState("")
  const [feedback, setFeedback] = useState("")
  const [blocked, setBlocked] = useState<string | null>(null)
  return (
    <form
      className="mt-3 space-y-2 min-w-0"
      onSubmit={async (e) => {
        e.preventDefault()
        if (
          disabled ||
          busy ||
          !answer.trim() ||
          ingestionInputBytes(answer) > 1000 ||
          blocked === answer.trim()
        )
          return
        setFeedback("")
        try {
          await onAnswer(answer)
          setFeedback(t("repo.clarificationRevised"))
        } catch (error) {
          if (error instanceof IngestionError && error.code === "cancelled")
            return
          const decision =
            error instanceof IngestionError
              ? error.decision?.outcome
              : undefined
          if (decision) {
            if (["request_rephrasing", "reject_attack"].includes(decision.kind))
              setBlocked(answer.trim())
            setFeedback(
              t(
                (decision.kind === "service_failure"
                  ? `field.failure.${decision.reason}`
                  : `field.${decision.kind}`) as Parameters<typeof t>[0],
              ),
            )
          } else
            setFeedback(
              t(
                error instanceof IngestionError && error.code === "stale"
                  ? "repo.ingestStale"
                  : "repo.clarificationFailed",
              ),
            )
        }
      }}
    >
      <p className="text-sm">{question || t("repo.clarificationConflict")}</p>
      <label className="block text-sm" htmlFor={`clarification-${claimId}`}>
        {t("repo.clarificationAnswer")}
      </label>
      <textarea
        id={`clarification-${claimId}`}
        value={answer}
        disabled={busy || disabled}
        onChange={(e) => setAnswer(e.target.value)}
        rows={3}
        className="w-full border p-2 text-sm bg-transparent"
        aria-describedby={`clarification-note-${claimId}`}
      />
      <p id={`clarification-note-${claimId}`} className="text-xs">
        {t("repo.clarificationNote")} · {ingestionInputBytes(answer)} / 1000{" "}
        {t("repo.bytes")}
      </p>
      <div className="flex flex-wrap gap-2">
        <button
          type="submit"
          className="border px-3 py-2 text-sm disabled:opacity-50"
          disabled={
            disabled ||
            busy ||
            !answer.trim() ||
            ingestionInputBytes(answer) > 1000 ||
            blocked === answer.trim()
          }
        >
          {busy ? t("field.working") : t("repo.clarificationReview")}
        </button>
        {busy && (
          <button
            type="button"
            className="border px-3 py-2 text-sm"
            onClick={onCancel}
          >
            {t("repo.ingestCancel")}
          </button>
        )}
      </div>
      {feedback && (
        <p role="status" className="text-sm break-words">
          {feedback}
        </p>
      )}
    </form>
  )
}

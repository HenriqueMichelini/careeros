import { useEffect, useRef, useState } from "react"
import { useI18n } from "../lib/store"
import type { TranslationKey } from "../lib/i18n"
import { IngestionError, ingestionInputBytes } from "../lib/ingestion"
import { destinationLabelKey, profileFieldKey } from "../lib/profileLabels"
import {
  reviewGroupState,
  reviewSummary,
  reviewUnitEdited,
  type ReviewDestination,
  type ReviewIdentity,
  type ReviewPlan,
  type ReviewUnit,
} from "../lib/ingestionReview"

// Compact destination-based review presenter (#190). It renders one
// `ReviewPlan` and reports every interaction back to its owner: it owns no
// selection, no provider call, no storage and no canonical write. Values,
// identities, excerpts, comparison reasons and related facts come from the
// model, so the rendered account never recomputes matching.
export interface SubmissionReviewProps {
  plan: ReviewPlan
  disabled?: boolean
  onSelectUnit: (unitId: string, selected: boolean) => void
  onSelectGroup: (destinationId: string, selected: boolean) => void
  onSelectAdditions: () => void
  onClearSelection: () => void
  onEditUnit: (unitId: string, field: string, value: string) => void
  // Optional protected clarification. When absent the attention rows stay
  // explanatory only; the presenter never dispatches on its own.
  onAnswerUnit?: (unitId: string, answer: string) => Promise<void>
  onLeaveUnitOut?: (unitId: string) => void
}

const muted = { color: "var(--color-muted-fg)" } as const
const cardBorder = { borderColor: "var(--color-border)" } as const

const entryTargets = new Set([
  "experience",
  "projects",
  "education",
  "certifications",
  "languages",
])

function identityText(identity: ReviewIdentity[]): string {
  return identity
    .map((item) => item.value)
    .filter((value) => value.trim())
    .join(" · ")
}

// Provider outcomes carry stable machine codes. Translate the known codes for
// the visible comparison copy and never leak an unknown enum into the UI.
const reasonKeys: Record<string, TranslationKey> = {
  validated_operation: "review.reason.validated_operation",
  verified_exact_alias_or_wording:
    "review.reason.verified_exact_alias_or_wording",
  source_ambiguity: "review.reason.source_ambiguity",
  unresolved: "review.reason.unresolved",
  unknown_owner: "review.reason.unknown_owner",
  duplicate: "review.reason.duplicate",
  fabricated: "review.reason.fabricated",
  no_source: "review.reason.no_source",
  // Server-owned reconciliation reasons are not provider prose. They must not
  // fall back to the “comparison reason unavailable” copy.
  no_validated_disposition: "repo.ingestReason.no_validated_disposition",
  no_validated_operation: "repo.ingestReason.no_validated_operation",
  support_already_retained: "repo.ingestReason.support_already_retained",
  identity_not_established: "repo.ingestReason.identity_not_established",
  invalid_evidence_update: "repo.ingestReason.invalid_evidence_update",
}
const findingKeys: Record<string, TranslationKey> = {
  addition: "review.finding.addition",
  in_place: "review.finding.in_place",
  overlap: "review.finding.overlap",
}
const outcomeKeys: Record<string, TranslationKey> = {
  change: "review.outcome.change",
  exact_duplicate: "review.outcome.exact_duplicate",
  overlap: "review.outcome.overlap",
  additional_support: "review.outcome.additional_support",
  contradiction: "review.outcome.contradiction",
  correction: "review.outcome.correction",
  clarification: "review.outcome.clarification",
  unsupported: "review.outcome.unsupported",
  unresolved: "review.outcome.unresolved",
}
const blockedReasons = new Set([
  "incomplete_identity",
  "incomplete_anchor",
  "stale",
  "shared_support",
  "dangling_support",
  "dependency_cycle",
  "dependency_blocked",
  "state",
  "clarification",
  "conflict",
  "unsupported",
  "unresolved",
])

function localizedCode(
  value: string,
  keys: Record<string, TranslationKey>,
  fallback: TranslationKey,
  t: (key: TranslationKey) => string,
): string {
  const parts = value
    .split(" · ")
    .map((part) => part.trim())
    .filter(Boolean)
  const translated = parts.map((part) => {
    const key = keys[part]
    if (key) return t(key)
    // Unknown free-form wording can stay as written; unknown enum-like codes
    // use the localized fallback instead of exposing an internal identifier.
    return /^[a-z][a-z0-9_]*$/.test(part) ? t(fallback) : part
  })
  return [...new Set(translated)].join(" · ")
}

function blockedText(
  reason: string,
  t: (key: TranslationKey) => string,
): string {
  return t(
    blockedReasons.has(reason)
      ? (`review.blocked.${reason}` as TranslationKey)
      : "review.blocked.state",
  )
}

// Accessible names need the destination value for repeated controls, but a
// multi-line excerpt should not become a paragraph-long checkbox label.
function shortName(value: string, fallback: string): string {
  const normalized = value.replace(/\s+/g, " ").trim()
  if (!normalized) return fallback
  return normalized.length > 96
    ? `${normalized.slice(0, 93).trimEnd()}…`
    : normalized
}

function meaningNotes(
  unit: ReviewUnit,
  t: (key: TranslationKey) => string,
): string[] {
  const meaning = unit.meaning
  if (!meaning) return []
  const notes: string[] = []
  if (meaning.assertion === "negated") notes.push(t("review.meaningNegated"))
  if (meaning.intent === "aspiration") notes.push(t("review.meaningAspiration"))
  if (meaning.certainty === "uncertain")
    notes.push(t("review.meaningUncertain"))
  if (meaning.certainty === "unknown") notes.push(t("review.meaningUnknown"))
  if (meaning.temporal.precision === "approximate")
    notes.push(
      meaning.temporal.wording
        ? `${t("review.meaningApproximate")}: ${meaning.temporal.wording}`
        : t("review.meaningApproximate"),
    )
  if (meaning.temporal.precision === "unknown")
    notes.push(t("review.meaningUnknownDate"))
  return notes
}

function groupAriaLabel(
  destination: ReviewDestination,
  section: string,
): string {
  const identity = identityText(destination.identity)
  return identity ? `${section} · ${identity}` : section
}

interface ReviewValuesProps {
  unit: ReviewUnit
  target: string
}

function ReviewValues({ unit, target }: ReviewValuesProps) {
  const { t } = useI18n()
  return (
    <div className="space-y-2 min-w-0">
      {unit.values.map((value, index) => {
        const fieldLabel = t(profileFieldKey(target, value.field))
        // A profile field already names its destination; only entry subfields
        // need a second label inside the destination group.
        const showFieldLabel = value.field !== target
        if (value.action === "remove")
          return (
            <div key={index} className="min-w-0">
              {showFieldLabel && (
                <p className="text-xs" style={muted}>
                  {fieldLabel}
                </p>
              )}
              <p className="text-sm break-words whitespace-pre-wrap">
                {t("review.remove")}: {value.before || "—"}
              </p>
            </div>
          )
        return (
          <div key={index} className="min-w-0">
            {showFieldLabel && (
              <p className="text-xs" style={muted}>
                {fieldLabel}
              </p>
            )}
            <p className="text-sm break-words whitespace-pre-wrap">
              {value.proposed || "—"}
            </p>
            {(value.action === "update" || value.before.trim()) && (
              <p
                className="text-sm break-words whitespace-pre-wrap line-through"
                style={muted}
              >
                {t("review.currentValue")}: {value.before || "—"}
              </p>
            )}
          </div>
        )
      })}
    </div>
  )
}

function SourceDisclosure({
  unit,
  context = true,
}: {
  unit: ReviewUnit
  context?: boolean
}) {
  const { t } = useI18n()
  const showRelated = context && unit.relatedClaims.length > 0
  if (
    !unit.sources.length &&
    !showRelated &&
    (!context || (!unit.relatedFacts.length && !unit.reason)) &&
    !unit.finding &&
    !unit.outcomeKind &&
    !unit.batchIndexes.length
  )
    return null
  const summaryValue = shortName(
    unit.values.map((value) => value.proposed || value.before).find(Boolean) ||
      unit.question ||
      unit.sources[0]?.excerpt ||
      "",
    t("review.source"),
  )
  return (
    <details className="mt-2 min-w-0" data-review-source>
      <summary
        className="cursor-pointer text-xs min-h-9 py-2"
        aria-label={t("review.sourceFor", { value: summaryValue })}
      >
        {t("review.source")}
      </summary>
      <div className="mt-2 space-y-2 min-w-0">
        {unit.sources.map((source, index) => (
          <div
            key={`${source.origin}-${source.claimId}-${index}`}
            className="min-w-0"
          >
            <p
              className="text-xs break-words whitespace-pre-wrap"
              style={muted}
            >
              {t(
                source.origin === "clarification_answer"
                  ? "review.sourceClarification"
                  : "review.sourceOriginal",
              )}
              : “{source.excerpt}”
            </p>
            <details className="mt-1">
              <summary className="cursor-pointer text-xs min-h-9 py-2">
                {t("review.sourceDetails")}
              </summary>
              <div className="mt-1 space-y-1 text-xs break-words">
                <p className="whitespace-pre-wrap">{source.text}</p>
                <p style={muted}>
                  {t("review.sourceClaimId")}: {source.claimId}
                </p>
                {source.reference && (
                  <p style={muted}>
                    {t("review.sourceOccurrence")}: {source.reference.segmentId}{" "}
                    · {source.reference.originalStart}–
                    {source.reference.originalEnd}
                  </p>
                )}
              </div>
            </details>
          </div>
        ))}
        {context &&
          unit.relatedFacts.map((fact) => (
            <div key={`${fact.id}-${fact.revision}`} className="min-w-0">
              <p
                className="text-xs break-words whitespace-pre-wrap"
                style={muted}
              >
                {t("review.inProfile")}: {fact.value}
              </p>
              <p className="text-xs break-words" style={muted}>
                {t("review.sourceRevision")}: {fact.revision} ·{" "}
                {t("review.normalization")}: {fact.normalization.observed} →{" "}
                {fact.normalization.canonical ?? "—"}
              </p>
            </div>
          ))}
        {showRelated &&
          unit.relatedClaims.map((claim) => (
            <p
              key={claim.id}
              className="text-xs break-words whitespace-pre-wrap"
              style={muted}
            >
              {t("review.relatedStatement")}: “{claim.source}”
            </p>
          ))}
        {context && unit.reason && (
          <p className="text-xs break-words" style={muted}>
            {t("review.sourceComparison")}:{" "}
            {localizedCode(
              unit.reason,
              reasonKeys,
              "review.reason.other",
              t,
            )}
          </p>
        )}
        {(unit.finding || unit.outcomeKind || unit.batchIndexes.length > 0) && (
          <details className="mt-1">
            <summary className="cursor-pointer text-xs min-h-9 py-2">
              {t("review.technicalDetails")}
            </summary>
            <div className="mt-1 space-y-1 text-xs break-words" style={muted}>
              <p>
                {t("review.sourceIdentifier")}: {unit.sourceId}
              </p>
              {unit.batchIndexes.length > 0 && (
                <p>
                  {t("review.sourceBatch")}: {unit.batchIndexes.join(", ")}
                </p>
              )}
              {unit.finding && (
                <p>
                  {t("review.sourceFinding")}:{" "}
                  {localizedCode(
                    unit.finding,
                    findingKeys,
                    "review.finding.other",
                    t,
                  )}
                </p>
              )}
              {unit.outcomeKind && (
                <p>
                  {t("review.sourceOutcome")}:{" "}
                  {localizedCode(
                    unit.outcomeKind,
                    outcomeKeys,
                    "review.outcome.other",
                    t,
                  )}
                </p>
              )}
            </div>
          </details>
        )}
      </div>
    </details>
  )
}

function UnitEditor({
  unit,
  target,
  disabled,
  editButtonId,
  onSave,
  onCancel,
}: {
  unit: ReviewUnit
  target: string
  disabled: boolean
  editButtonId: string
  onSave: (drafts: Record<string, string>) => void
  onCancel: () => void
}) {
  const { t } = useI18n()
  const editable = unit.values.filter(
    (value) => value.action !== "remove" && value.action !== "evidence",
  )
  const [drafts, setDrafts] = useState<Record<string, string>>(() =>
    Object.fromEntries(editable.map((value) => [value.field, value.proposed])),
  )
  return (
    <form
      className="mt-2 space-y-3 border p-3 min-w-0"
      style={cardBorder}
      onSubmit={(event) => {
        event.preventDefault()
        onSave(drafts)
      }}
    >
      {editable.map((value) => {
        const fieldLabel = t(profileFieldKey(target, value.field))
        const changed = drafts[value.field] !== value.proposed
        return (
          <div key={value.field} className="min-w-0">
            <label
              className="block text-sm"
              htmlFor={`${editButtonId}-${value.field}`}
            >
              {fieldLabel}
            </label>
            <textarea
              id={`${editButtonId}-${value.field}`}
              className="w-full min-w-0 border p-2 mt-1 text-sm resize-y bg-transparent"
              rows={2}
              disabled={disabled}
              value={drafts[value.field] ?? ""}
              onChange={(event) =>
                setDrafts({ ...drafts, [value.field]: event.target.value })
              }
            />
            {changed && (
              <p
                className="text-xs mt-1 break-words whitespace-pre-wrap"
                style={muted}
              >
                {t("review.preview")}: {value.before || "—"} →{" "}
                {drafts[value.field] || "—"}
              </p>
            )}
          </div>
        )
      })}
      <div className="flex flex-wrap gap-2">
        <button
          type="submit"
          className="border px-3 py-2 text-sm disabled:opacity-50"
          style={cardBorder}
          disabled={disabled}
        >
          {t("review.editSave")}
        </button>
        <button
          type="button"
          className="border px-3 py-2 text-sm"
          style={cardBorder}
          disabled={disabled}
          onClick={onCancel}
        >
          {t("review.editCancel")}
        </button>
      </div>
    </form>
  )
}

function AttentionUnit({
  unit,
  disabled,
  onAnswerUnit,
  onLeaveUnitOut,
}: {
  unit: ReviewUnit
  disabled: boolean
  onAnswerUnit: SubmissionReviewProps["onAnswerUnit"]
  onLeaveUnitOut: SubmissionReviewProps["onLeaveUnitOut"]
}) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const [answerOpen, setAnswerOpen] = useState(false)
  const [answer, setAnswer] = useState("")
  const [blocked, setBlocked] = useState("")
  const [feedback, setFeedback] = useState("")
  const [leftOut, setLeftOut] = useState(false)
  const [busy, setBusy] = useState(false)
  const [expired, setExpired] = useState(false)
  const answerButton = useRef<HTMLButtonElement | null>(null)
  const leaveButton = useRef<HTMLButtonElement | null>(null)
  const status = useRef<HTMLParagraphElement | null>(null)
  const safeId = unit.id.replace(/[^a-zA-Z0-9_-]+/g, "-")
  const answerId = `answer-${safeId}`
  const question = unit.question
  const kindLabel = t(`review.attention.${unit.kind}` as TranslationKey)
  const attentionName = shortName(
    question || unit.sources[0]?.excerpt || kindLabel,
    kindLabel,
  )
  const trimmed = answer.trim()
  const bytes = ingestionInputBytes(answer)
  const tooLong = bytes > 1000
  const unavailable = !onAnswerUnit
  const submit = async () => {
    if (
      !onAnswerUnit ||
      disabled ||
      busy ||
      expired ||
      !trimmed ||
      tooLong ||
      blocked === trimmed
    )
      return
    setBusy(true)
    setFeedback("")
    try {
      await onAnswerUnit(unit.id, answer)
      setAnswer("")
      setAnswerOpen(false)
      setFeedback(t("repo.clarificationRevised"))
    } catch (error) {
      if (error instanceof IngestionError && error.code === "cancelled") return
      const code = error instanceof IngestionError ? error.code : ""
      const kind =
        error instanceof IngestionError
          ? error.decision?.outcome.kind ?? ""
          : ""
      if (kind === "request_rephrasing" || kind === "reject_attack") {
        setBlocked(trimmed)
        setFeedback(t(`field.${kind}` as TranslationKey))
      } else if (kind) {
        setFeedback(t(`field.${kind}` as TranslationKey))
      } else if (code === "receipt_expired" || code === "receipt_invalid") {
        setExpired(true)
        setFeedback(t("review.answerExpired"))
      } else if (code === "answer_unchanged") {
        setBlocked(trimmed)
        setFeedback(t("review.answerUnchanged"))
      } else {
        setFeedback(t("repo.clarificationFailed"))
      }
      setTimeout(() => status.current?.focus(), 0)
    } finally {
      setBusy(false)
    }
  }
  return (
    <details
      className="mt-1 min-w-0"
      data-review-attention
      open={open}
      onToggle={(event) => setOpen(event.currentTarget.open)}
    >
      <summary className="cursor-pointer min-w-0 min-h-9" data-review-attention-summary>
        <span className="text-sm font-semibold block">{kindLabel}</span>
        {question && (
          <span className="text-sm break-words whitespace-pre-wrap block mt-1">
            {question}
          </span>
        )}
        {unit.relatedFacts.map((fact) => (
          <span
            key={`${fact.id}-${fact.revision}`}
            className="text-sm break-words whitespace-pre-wrap block mt-1"
          >
            {t("review.inProfile")}: {fact.value}
          </span>
        ))}
        {unit.relatedClaims.map((claim) => (
          <span
            key={claim.id}
            className="text-sm break-words whitespace-pre-wrap block mt-1"
          >
            {t("review.relatedStatement")}: “{claim.source}”
          </span>
        ))}
        {unit.reason && (
          <span
            className="text-xs break-words whitespace-pre-wrap block mt-1"
            style={muted}
          >
            {t("review.attentionReason")}:{" "}
            {localizedCode(
              unit.reason,
              reasonKeys,
              "review.reason.other",
              t,
            )}
          </span>
        )}
        {unit.blockedReason && (
          <span className="text-xs block mt-1" style={muted}>
            {t("review.blocked")}: {blockedText(unit.blockedReason, t)}
          </span>
        )}
      </summary>
      <div className="mt-3 space-y-3 min-w-0">
        {unit.question && (
          <div className="min-w-0">
            <p className="text-xs" style={muted}>
              {t("review.question")}
            </p>
            <p className="text-sm font-semibold break-words whitespace-pre-wrap">
              {unit.question}
            </p>
          </div>
        )}
        <SourceDisclosure unit={unit} context={false} />
        {leftOut && (
          <p className="text-sm" role="status">
            {t("review.keptUnresolved")}
          </p>
        )}
        {feedback && (
          <p
            ref={status}
            tabIndex={-1}
            className="text-sm break-words"
            role={busy ? "status" : "alert"}
          >
            {feedback}
          </p>
        )}
        {answerOpen ? (
          <form
            className="space-y-2 min-w-0"
            data-review-answer
            onSubmit={(event) => {
              event.preventDefault()
              void submit()
            }}
          >
            <label className="block text-sm" htmlFor={answerId}>
              {t("review.answerLabel")}
            </label>
            <textarea
              id={answerId}
              autoFocus
              aria-label={t("review.answerFor", { value: attentionName })}
              className="w-full min-w-0 border p-2 text-sm resize-y bg-transparent"
              rows={3}
              disabled={disabled || busy}
              value={answer}
              onChange={(event) => setAnswer(event.target.value)}
            />
            <p className="text-xs" style={muted}>
              {bytes} / 1,000
            </p>
            {tooLong && (
              <p className="text-sm" role="alert">
                {t("review.answerTooLong")}
              </p>
            )}
            <p className="text-xs break-words" style={muted}>
              {t("repo.clarificationNote")}
            </p>
            <div className="flex flex-wrap gap-2">
              <button
                type="submit"
                className="border px-3 py-2 text-sm disabled:opacity-50"
                style={cardBorder}
                disabled={
                  disabled ||
                  busy ||
                  expired ||
                  !trimmed ||
                  tooLong ||
                  blocked === trimmed
                }
              >
                {busy
                  ? t("review.answerStatus")
                  : t("repo.clarificationReview")}
              </button>
              <button
                type="button"
                className="border px-3 py-2 text-sm"
                style={cardBorder}
                disabled={busy}
                onClick={() => {
                  setAnswerOpen(false)
                  setTimeout(() => answerButton.current?.focus(), 0)
                }}
              >
                {t("review.editCancel")}
              </button>
            </div>
          </form>
        ) : (
          <div className="flex flex-wrap gap-2">
            <button
              ref={answerButton}
              type="button"
              data-review-answer-open
              aria-label={t("review.answerFor", { value: attentionName })}
              className="border px-3 py-2 text-sm disabled:opacity-50"
              style={cardBorder}
              disabled={disabled || unavailable || expired}
              onClick={() => {
                setAnswerOpen(true)
                setFeedback("")
              }}
            >
              {t("review.answer")}
            </button>
            <button
              ref={leaveButton}
              type="button"
              data-review-leave-out
              aria-label={t("review.leaveOutFor", { value: attentionName })}
              className="border px-3 py-2 text-sm disabled:opacity-50"
              style={cardBorder}
              disabled={disabled || leftOut}
              onClick={() => {
                setLeftOut(true)
                setAnswerOpen(false)
                onLeaveUnitOut?.(unit.id)
                setTimeout(() => answerButton.current?.focus(), 0)
              }}
            >
              {t("review.leaveOut")}
            </button>
          </div>
        )}
      </div>
    </details>
  )
}

function ReviewUnitRow({
  unit,
  plan,
  destination,
  disabled,
  onSelectUnit,
  onEditUnit,
  onAnswerUnit,
  onLeaveUnitOut,
}: {
  unit: ReviewUnit
  plan: ReviewPlan
  destination: ReviewDestination
  disabled: boolean
  onSelectUnit: SubmissionReviewProps["onSelectUnit"]
  onEditUnit: SubmissionReviewProps["onEditUnit"]
  onAnswerUnit: SubmissionReviewProps["onAnswerUnit"]
  onLeaveUnitOut: SubmissionReviewProps["onLeaveUnitOut"]
}) {
  const { t } = useI18n()
  const [editing, setEditing] = useState(false)
  const editButton = useRef<HTMLButtonElement | null>(null)
  const safeId = unit.id.replace(/[^a-zA-Z0-9_-]+/g, "-")
  const editButtonId = `edit-${safeId}`
  const target = unit.target || destination.target
  const section = t(destinationLabelKey(target))
  const edited = reviewUnitEdited(plan, unit.id)
  const valueSummary =
    unit.values
      .map((value) => value.proposed || value.before)
      .find((value) => value) ?? unit.question
  const accessibleValue = shortName(valueSummary, section)
  const checkboxId = `select-${safeId}`
  const control =
    unit.kind === "conflict" ||
    unit.kind === "clarification" ||
    unit.kind === "unsupported" ||
    unit.kind === "unresolved"
  const editable =
    unit.selectable &&
    unit.kind !== "removal" &&
    unit.kind !== "evidence" &&
    unit.values.some(
      (value) => value.action !== "remove" && value.action !== "evidence",
    )
  const notes = meaningNotes(unit, t)

  return (
    <div
      className="mt-3 border p-3 min-w-0 focus:outline-2 focus:outline-offset-2"
      style={{ ...cardBorder, outlineColor: "var(--color-fg)" }}
      tabIndex={-1}
      data-review-unit={unit.id}
      data-review-kind={unit.kind}
    >
      <div className="flex items-start gap-3 min-w-0">
        {unit.selectable ? (
          <label className="-ml-2 mt-0.5 flex min-h-11 min-w-11 shrink-0 cursor-pointer items-start justify-center pt-3">
            <input
              id={checkboxId}
              type="checkbox"
              className="h-5 w-5"
              checked={unit.status === "selected"}
              disabled={disabled}
              aria-label={t("review.selectUnit", {
                value: accessibleValue,
              })}
              onChange={(event) => onSelectUnit(unit.id, event.target.checked)}
            />
          </label>
        ) : (
          <span className="mt-1" aria-hidden>
            ·
          </span>
        )}
        <div className="min-w-0 flex-1">
          {(entryTargets.has(target) || unit.kind === "evidence") && (
            <p className="text-xs" style={muted}>
              {unit.kind === "evidence"
                ? `${t("review.attachSource")} · ${section}`
                : section}
            </p>
          )}
          {unit.kind === "duplicate" && (
            <>
              <p className="text-sm font-semibold">{t("review.duplicate")}</p>
              <p className="text-sm break-words whitespace-pre-wrap">
                {unit.sources[0]?.excerpt || unit.claimIds.join(", ")}
              </p>
              {unit.relatedFacts.map((fact) => (
                <p
                  key={`${fact.id}-${fact.revision}`}
                  className="text-sm break-words"
                  style={muted}
                >
                  {t("review.inProfile")}: {fact.value}
                </p>
              ))}
            </>
          )}
          {(unit.kind === "conflict" ||
            unit.kind === "clarification" ||
            unit.kind === "unsupported" ||
            unit.kind === "unresolved") && (
            <AttentionUnit
              unit={unit}
              disabled={disabled}
              onAnswerUnit={onAnswerUnit}
              onLeaveUnitOut={onLeaveUnitOut}
            />
          )}
          {(unit.kind === "addition" ||
            unit.kind === "replacement" ||
            unit.kind === "removal" ||
            unit.kind === "evidence") && (
            <ReviewValues unit={unit} target={target} />
          )}
          {unit.kind === "evidence" && (
            <p className="text-xs mt-1 break-words" style={muted}>
              {t("review.evidenceNote")}
            </p>
          )}
          {!unit.selectable &&
            (unit.kind === "addition" ||
              unit.kind === "replacement" ||
              unit.kind === "removal" ||
              unit.kind === "evidence") && (
              <p className="text-sm mt-1" role="note">
                {t("review.blocked")}:{" "}
                {blockedText(unit.blockedReason || "state", t)}
              </p>
            )}
          {notes.length > 0 && (
            <p className="text-xs mt-1 break-words" style={muted}>
              {notes.join(" · ")}
            </p>
          )}
          {edited && (
            <p className="text-xs mt-1" style={muted}>
              {t("review.edited")}
            </p>
          )}
          <div className="flex flex-wrap items-center gap-3 mt-2">
            {editable && (
              <button
                id={editButtonId}
                ref={editButton}
                type="button"
                className="text-xs underline min-h-9 min-w-11 text-left"
                aria-expanded={editing}
                aria-controls={`${editButtonId}-editor`}
                aria-label={
                  editing
                    ? t("review.cancelEditUnit", {
                        value: accessibleValue,
                      })
                    : t("review.editUnitName", {
                        value: accessibleValue,
                      })
                }
                disabled={disabled}
                onClick={() => setEditing(!editing)}
              >
                {editing ? t("review.editCancel") : t("review.edit")}
              </button>
            )}
          </div>
          {editing && (
            <div id={`${editButtonId}-editor`}>
              <UnitEditor
                unit={unit}
                target={target}
                disabled={disabled}
                editButtonId={editButtonId}
                onCancel={() => {
                  setEditing(false)
                  editButton.current?.focus()
                }}
                onSave={(drafts) => {
                  for (const value of unit.values)
                    if (
                      value.action !== "remove" &&
                      value.action !== "evidence" &&
                      drafts[value.field] !== value.proposed
                    )
                      onEditUnit(unit.id, value.field, drafts[value.field])
                  setEditing(false)
                  editButton.current?.focus()
                }}
              />
            </div>
          )}
          {!control && <SourceDisclosure unit={unit} context />}
        </div>
      </div>
    </div>
  )
}

function GroupToggle({
  destination,
  plan,
  disabled,
  section,
  onSelectGroup,
}: {
  destination: ReviewDestination
  plan: ReviewPlan
  disabled: boolean
  section: string
  onSelectGroup: SubmissionReviewProps["onSelectGroup"]
}) {
  const { t } = useI18n()
  const state = reviewGroupState(plan, destination.id)
  const ref = useRef<HTMLInputElement | null>(null)
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = state === "partial"
  }, [state])
  if (!destination.selectableUnitIds.length) return null
  return (
    <label className="flex min-h-11 cursor-pointer items-center gap-2 text-xs">
      <input
        ref={ref}
        type="checkbox"
        className="h-5 w-5 shrink-0"
        checked={state === "all"}
        disabled={disabled}
        aria-label={t("review.selectGroup", {
          destination: groupAriaLabel(destination, section),
        })}
        onChange={(event) =>
          onSelectGroup(destination.id, event.target.checked)
        }
      />
      {t("review.selectGroupLabel")}
    </label>
  )
}

export function SubmissionReview({
  plan,
  disabled = false,
  onSelectUnit,
  onSelectGroup,
  onSelectAdditions,
  onClearSelection,
  onEditUnit,
  onAnswerUnit,
  onLeaveUnitOut,
}: SubmissionReviewProps) {
  const { t } = useI18n()
  const summary = reviewSummary(plan)
  const destinations = plan.destinations.filter((destination) =>
    destination.unitIds.some((unitId) =>
      plan.units.some((unit) => unit.id === unitId),
    ),
  )
  const hasAdditions = plan.units.some(
    (unit) => unit.selectable && unit.kind === "addition",
  )
  const duplicateUnits = plan.units.filter((unit) => unit.kind === "duplicate")

  if (summary.noChange)
    return (
      <section className="space-y-3 min-w-0" aria-label={t("review.heading")}>
        <h2 id="review-heading" tabIndex={-1} className="text-xl font-bold">
        {t("review.heading")}
      </h2>
        <div className="border p-4 min-w-0" style={cardBorder} role="status">
          <p className="text-sm font-semibold">{t("review.noChange")}</p>
          <p className="text-sm mt-1" style={muted}>
            {t("review.noChangeNote")}
          </p>
        </div>
        {duplicateUnits.length > 0 && (
          <details className="min-w-0">
            <summary className="cursor-pointer text-sm min-h-9 py-2">
              {t("review.duplicateSummary", { count: duplicateUnits.length })}
            </summary>
            <ul className="mt-2 space-y-2 min-w-0">
              {duplicateUnits.map((unit) => (
                <li
                  key={unit.id}
                  className="text-sm break-words whitespace-pre-wrap"
                >
                  {unit.sources[0]?.excerpt || unit.claimIds.join(", ")}
                  <SourceDisclosure unit={unit} />
                </li>
              ))}
            </ul>
          </details>
        )}
      </section>
    )

  return (
    <section className="space-y-4 min-w-0" aria-label={t("review.heading")}>
      <h2 id="review-heading" tabIndex={-1} className="text-xl font-bold">
        {t("review.heading")}
      </h2>
      <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm" role="status">
        <span>{t("review.summaryChanges", { count: summary.selectable })}</span>
        {summary.attention > 0 && (
          <span>
            {t("review.summaryAttention", { count: summary.attention })}
          </span>
        )}
        {summary.conflicts > 0 && (
          <span data-attention="conflict">
            {t("review.attention.conflict")} ({summary.conflicts})
          </span>
        )}
        {summary.clarifications > 0 && (
          <span data-attention="clarification">
            {t("review.attention.clarification")} ({summary.clarifications})
          </span>
        )}
        {summary.unsupported > 0 && (
          <span data-attention="unsupported">
            {t("review.attention.unsupported")} ({summary.unsupported})
          </span>
        )}
        {summary.unresolved > 0 && (
          <span data-attention="unresolved">
            {t("review.attention.unresolved")} ({summary.unresolved})
          </span>
        )}
        {summary.blocked > 0 && (
          <span data-attention="blocked">
            {t("review.summaryBlocked", { count: summary.blocked })}
          </span>
        )}
        {summary.duplicates > 0 && (
          <span>
            {t("review.summaryDuplicates", { count: summary.duplicates })}
          </span>
        )}
        {summary.selected > 0 && (
          <span>
            {t("review.summarySelected", { count: summary.selected })}
          </span>
        )}
      </div>
      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          className="border px-3 py-2 text-sm disabled:opacity-50"
          style={cardBorder}
          disabled={disabled || !hasAdditions}
          onClick={onSelectAdditions}
        >
          {t("review.selectAdditions")}
        </button>
        <button
          type="button"
          className="border px-3 py-2 text-sm disabled:opacity-50"
          style={cardBorder}
          disabled={disabled || summary.selected === 0}
          onClick={onClearSelection}
        >
          {t("review.clearSelection")}
        </button>
      </div>
      {destinations.map((destination) => {
        const section = t(destinationLabelKey(destination.target))
        const identity = identityText(destination.identity)
        const units = destination.unitIds
          .map((unitId) => plan.units.find((unit) => unit.id === unitId)!)
          .filter(Boolean)
        const duplicates = units.filter((unit) => unit.kind === "duplicate")
        const rest = units.filter((unit) => unit.kind !== "duplicate")
        return (
          <article
            key={destination.id}
            className="border p-4 min-w-0 focus:outline-2 focus:outline-offset-2"
            style={{ ...cardBorder, outlineColor: "var(--color-fg)" }}
            tabIndex={-1}
            data-review-destination={destination.id}
          >
            <header className="flex flex-wrap items-start justify-between gap-3 min-w-0">
              <div className="min-w-0">
                <h3 className="font-bold break-words">
                  {destination.entryKind === "new"
                    ? `${t("review.newEntry")} · ${section}`
                    : section}
                </h3>
                {identity && (
                  <p className="text-sm break-words" style={muted}>
                    {identity}
                  </p>
                )}
              </div>
              <GroupToggle
                destination={destination}
                plan={plan}
                disabled={disabled}
                section={section}
                onSelectGroup={onSelectGroup}
              />
            </header>
            {rest.map((unit) => (
              <ReviewUnitRow
                key={unit.id}
                unit={unit}
                plan={plan}
                destination={destination}
                disabled={disabled}
                onSelectUnit={onSelectUnit}
                onEditUnit={onEditUnit}
                onAnswerUnit={onAnswerUnit}
                onLeaveUnitOut={onLeaveUnitOut}
              />
            ))}
            {duplicates.length > 0 && (
              <details className="mt-3 min-w-0">
                <summary className="cursor-pointer text-sm min-h-9 py-2">
                  {t("review.duplicateSummary", { count: duplicates.length })}
                </summary>
                <ul className="mt-2 space-y-2 min-w-0">
                  {duplicates.map((unit) => (
                    <li
                      key={unit.id}
                      className="text-sm break-words whitespace-pre-wrap"
                      style={muted}
                    >
                      {unit.sources[0]?.excerpt || unit.claimIds.join(", ")}
                      <SourceDisclosure unit={unit} />
                    </li>
                  ))}
                </ul>
              </details>
            )}
          </article>
        )
      })}
    </section>
  )
}

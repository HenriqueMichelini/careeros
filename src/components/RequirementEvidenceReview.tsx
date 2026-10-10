import { profileFieldKey } from "../lib/profileLabels"
import type { RequirementMatch } from "../lib/qualificationEvidence"
import { useI18n } from "../lib/store"

export default function RequirementEvidenceReview({
  matches,
}: {
  matches: RequirementMatch[]
}) {
  const { t } = useI18n()
  if (!matches.length) return null
  return (
    <div className="space-y-4 mb-6" aria-label={t("requirementEvidence.title")}>
      <h3 className="font-medium">{t("requirementEvidence.title")}</h3>
      {matches.map((m) => (
        <article
          key={m.requirementIndex}
          className="border border-[var(--color-border)] p-4 text-sm"
          data-requirement-state={m.state}
        >
          <p className="font-medium break-words">
            {m.requirement.source.quote}
          </p>
          <p className="text-xs my-2">
            {t(`requirementEvidence.${m.state}`)} ·{" "}
            {t(`jobContext.${m.requirement.importance}`)}
          </p>
          <p className="break-words">{m.explanation}</p>
          {!m.complete && (
            <p className="mt-2" role="status">
              {t("requirementEvidence.incomplete", { count: m.excluded })}
            </p>
          )}
          {m.question && <p className="mt-2 font-medium">{m.question}</p>}
          <details className="mt-3">
            <summary className="cursor-pointer">
              {t("requirementEvidence.inspect")}
            </summary>
            <p className="text-xs my-2">
              {t("requirementEvidence.traceability")}
            </p>
            {m.facts.map((f) => (
              <div
                key={f.id}
                className="mt-3 border-t border-[var(--color-border)] pt-2 break-words"
                data-fact-id={f.id}
              >
                <p className="text-xs">
                  {t(profileFieldKey(f.section, f.field))}
                  {m.factIds.includes(f.id)
                    ? ` · ${t("requirementEvidence.cited")}`
                    : ""}
                </p>
                <p className="whitespace-pre-wrap">{String(f.value)}</p>
                {f.temporal?.wording && <p>{f.temporal.wording}</p>}
                <p className="text-xs">
                  {t(
                    `profile.facts.${f.assertion}` as import("../lib/i18n").TranslationKey,
                  )}{" "}
                  ·{" "}
                  {t(
                    `profile.facts.${f.intent}` as import("../lib/i18n").TranslationKey,
                  )}{" "}
                  ·{" "}
                  {t(
                    `profile.facts.${f.certainty}` as import("../lib/i18n").TranslationKey,
                  )}
                </p>
                {f.evidence.map((e) => (
                  <blockquote
                    key={e.id}
                    className="border-l-2 pl-3 mt-1 whitespace-pre-wrap"
                  >
                    {e.excerpt}
                  </blockquote>
                ))}
              </div>
            ))}
          </details>
        </article>
      ))}
    </div>
  )
}

import { profileFieldKey } from "../lib/profileLabels"
import type { TranslationKey } from "../lib/i18n"
import type {
  QualificationFact,
  RequirementMatch,
} from "../lib/qualificationEvidence"
import { useI18n } from "../lib/store"

export default function RequirementEvidenceReview({
  matches,
}: {
  matches: RequirementMatch[]
}) {
  const { t } = useI18n()
  if (!matches.length) return null
  const semantics = (value: string) =>
    t(("profile.facts." + value) as TranslationKey)
  const identityFields = new Set([
    "company",
    "title",
    "startDate",
    "endDate",
    "name",
    "institution",
    "graduationDate",
  ])
  return (
    <div className="space-y-4 mb-6" aria-label={t("requirementEvidence.title")}>
      <h3 className="font-medium">{t("requirementEvidence.title")}</h3>
      {matches.map((m) => {
        const groups = new Map<string, QualificationFact[]>()
        for (const fact of m.facts)
          groups.set(fact.owner.id, [
            ...(groups.get(fact.owner.id) ?? []),
            fact,
          ])
        const identity = (ownerId: string) => {
          const facts = groups.get(ownerId) ?? []
          if (facts.some((f) => f.owner.id === f.owner.profileId))
            return t("home.profile")
          const parts = facts
            .filter(
              (f) => identityFields.has(f.field) && typeof f.value === "string",
            )
            .map((f) => String(f.value))
          if (facts.some((f) => f.field === "current" && f.value === true))
            parts.push(t("common.current"))
          return (
            parts.join(" · ") || t("requirementEvidence.contextUnavailable")
          )
        }
        return (
          <article
            key={m.requirementIndex}
            className="border border-[var(--color-border)] p-4 text-sm"
            data-requirement-state={m.state}
          >
            <p className="font-medium break-words">
              {m.requirement.source.quote}
            </p>
            <p className="text-xs my-2">
              {t(("requirementEvidence." + m.state) as TranslationKey)} ·{" "}
              {t(("jobContext." + m.requirement.importance) as TranslationKey)}
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
              {Array.from(groups, ([ownerId, facts]) => (
                <section
                  key={ownerId}
                  className="mt-4 border-t border-[var(--color-border)] pt-3 break-words"
                  data-evidence-owner={ownerId}
                >
                  <h4 className="font-medium">{identity(ownerId)}</h4>
                  {facts.map((f) => (
                    <div
                      key={f.id}
                      className="mt-3 break-words"
                      data-fact-id={f.id}
                    >
                      <p className="text-xs">
                        {t(profileFieldKey(f.section, f.field))}
                        {m.factIds.includes(f.id)
                          ? " · " + t("requirementEvidence.cited")
                          : ""}
                      </p>
                      <p className="whitespace-pre-wrap">{String(f.value)}</p>
                      {f.context.length > 0 && (
                        <p className="text-xs mt-1" data-linked-context={f.id}>
                          {t("requirementEvidence.linkedContext")}:{" "}
                          {f.context.map((c) => identity(c.id)).join("; ")}
                        </p>
                      )}
                      {f.temporal?.wording && <p>{f.temporal.wording}</p>}
                      <p className="text-xs">
                        {semantics(f.assertion)} · {semantics(f.intent)} ·{" "}
                        {semantics(f.certainty)}
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
                </section>
              ))}
            </details>
          </article>
        )
      })}
    </div>
  )
}

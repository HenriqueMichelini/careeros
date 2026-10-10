import { useI18n } from "../lib/store"
import type { ProfileRef } from "../lib/profileDocument"
import type { QualificationFact } from "../lib/qualificationEvidence"

export default function CareerSourceSupport({
  sources,
  facts,
  profileId,
}: {
  sources: ProfileRef[]
  facts: QualificationFact[]
  profileId: string
}) {
  const { t } = useI18n()
  return (
    <>
      {sources.map((ref, i) => {
        const fact = facts.find(
          (f) => f.id === ref.id && f.revision === ref.revision,
        )
        return (
          <div key={i} className="mt-2 break-words">
            <p className="text-xs">
              {ref.id} · {t("resumeReview.revision")} {ref.revision}
            </p>
            {fact ? (
              <>
                <p className="whitespace-pre-wrap">{String(fact.value)}</p>
                <p className="text-xs text-[var(--color-muted-fg)]">
                  {t("resumeReview.context")}:{" "}
                  {facts
                    .filter(
                      (f) =>
                        f.owner.id !== profileId &&
                        (f.owner.id === fact.owner.id ||
                          fact.context.some((c) => c.id === f.owner.id)) &&
                        [
                          "company",
                          "title",
                          "startDate",
                          "endDate",
                          "name",
                          "institution",
                          "degree",
                          "proficiency",
                        ].includes(f.field),
                    )
                    .map((f) => String(f.value))
                    .join(" · ") || t("resumeReview.generalContext")}
                </p>
                {fact.evidence.map((e) => (
                  <blockquote
                    key={e.id}
                    className="ml-3 border-l pl-3 whitespace-pre-wrap border-[var(--color-border)]"
                  >
                    {e.excerpt}
                  </blockquote>
                ))}
              </>
            ) : (
              <p>{t("resumeReview.reason.invalid_or_stale_citation")}</p>
            )}
          </div>
        )
      })}
    </>
  )
}

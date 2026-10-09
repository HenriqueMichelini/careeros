import { ingestionIdentityFacts } from "../lib/ingestion"
import type {
  IngestionClaim,
  IngestionOutcome,
  IngestionResult,
} from "../lib/ingestion"
import type { ProfileDocument } from "../lib/profileDocument"
import { profileFieldKey } from "../lib/profileLabels"
import { useI18n } from "../lib/store"
import type { TranslationKey } from "../lib/i18n"

export function IngestionCoverageNotice({
  result,
}: {
  result: IngestionResult
}) {
  const { t } = useI18n()
  return (
    <div
      className="text-sm border p-3 space-y-2 break-words"
      role="status"
      style={{ borderColor: "var(--color-border)" }}
    >
      <p>
        {t("repo.ingestCoverage")}: {result.claims.length} ·{" "}
        {t("repo.ingestInvalidCount")}: {result.unverifiedClaimCount} ·{" "}
        {t("repo.ingestUnplacedCount")}: {result.unplacedOperationCount}
      </p>
      <p>{t("repo.ingestCoverageLimit")}</p>
      {result.coverage?.capacity === "possibly_exhausted" && (
        <p>{t("repo.ingestCapacityReached")}</p>
      )}
      {!!result.skippedClaims?.length && (
        <ul className="list-disc pl-5">
          {result.skippedClaims.map((item) => (
            <li key={item.index}>
              {t("repo.ingestSkippedClaim")} {item.index}:{" "}
              {t(("repo.ingestSkipped." + item.reason) as TranslationKey)}
              {item.text && (
                <p className="mt-1 whitespace-pre-wrap">
                  {t("repo.ingestUnvalidatedWording")}
                  {item.shortened ? ` (${t("repo.ingestShortened")})` : ""}: “
                  {item.text}”
                </p>
              )}
              {item.source ? (
                <blockquote className="mt-1 whitespace-pre-wrap">
                  {t("repo.ingestSource")}: “{item.source}”
                </blockquote>
              ) : (
                <p className="mt-1">{t("repo.ingestNoValidSource")}</p>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export function ClaimOutcomeDetails({
  outcome,
  document,
  claims,
}: {
  outcome?: IngestionOutcome
  document: ProfileDocument | null
  claims: IngestionClaim[]
}) {
  const { t } = useI18n()
  if (!outcome)
    return <p className="text-sm mt-2">{t("repo.ingestUnresolvedClaim")}</p>
  const reasonKey = ("repo.ingestReason." + outcome.reason) as TranslationKey
  const translated = t(reasonKey)
  return (
    <div className="mt-3 space-y-2 text-sm break-words">
      <p className="font-semibold">
        {t(("repo.ingestOutcome." + outcome.kind) as TranslationKey)}
      </p>
      <p>
        {translated === reasonKey
          ? t(("repo.ingestReason." + outcome.kind) as TranslationKey)
          : translated}
      </p>
      {translated === reasonKey && (
        <details>
          <summary className="cursor-pointer">
            {t("repo.ingestComparisonDetail")}
          </summary>
          <p className="mt-2 whitespace-pre-wrap">{outcome.reason}</p>
        </details>
      )}
      {outcome.relatedFacts.map((ref) => {
        const fact = document?.facts.find(
          (f) => f.id === ref.id && f.revision === ref.revision,
        )
        if (!fact || !document) return null
        const owner = document.entities.find((e) => e.id === fact.owner.id)
        const identity = ingestionIdentityFacts(document, fact)
        const evidence = document.links
          .filter(
            (l) =>
              l.kind === "supports" &&
              l.state === "active" &&
              l.from.id === fact.id &&
              l.from.revision === fact.revision,
          )
          .flatMap((l) =>
            document.evidence.filter(
              (e) => e.id === l.to.id && e.revision === l.to.revision,
            ),
          )
        return (
          <details
            key={ref.id}
            className="border p-2"
            style={{ borderColor: "var(--color-border)" }}
          >
            <summary className="cursor-pointer">
              {t("repo.ingestRelatedFact")}:{" "}
              {t(profileFieldKey(owner?.kind ?? "profile", fact.field))} ·{" "}
              {String(fact.value)}
            </summary>
            <div className="mt-2 space-y-2 whitespace-pre-wrap">
              <p>
                {[
                  fact.assertion,
                  fact.intent,
                  fact.certainty,
                  fact.temporal.precision,
                ]
                  .map((value) => t(`profile.facts.${value}` as TranslationKey))
                  .join(" · ")}
                {fact.temporal.wording && ` · ${fact.temporal.wording}`}
              </p>
              {!!identity.length && (
                <p>
                  {identity
                    .map(
                      (f) =>
                        `${t(profileFieldKey(owner?.kind ?? "profile", f.field))}: ${String(f.value)}`,
                    )
                    .join(" · ")}
                </p>
              )}
              <p>
                {t("repo.ingestFactReference")}: {ref.id} ·{" "}
                {t("repo.ingestRevision")} {ref.revision}
              </p>
              {!!fact.context.length && (
                <p>
                  {t("repo.ingestContextReferences")}:{" "}
                  {fact.context
                    .map((r) => `${r.id} (${r.revision})`)
                    .join(", ")}
                </p>
              )}
              {evidence.length ? (
                evidence.map((e) => (
                  <blockquote key={e.id}>
                    {t("repo.ingestAcceptedSource")}: “{e.excerpt}”
                  </blockquote>
                ))
              ) : (
                <p>{t("repo.ingestNoAcceptedSource")}</p>
              )}
            </div>
          </details>
        )
      })}
      {outcome.relatedClaimIds.map((id) => {
        const claim = claims.find((c) => c.id === id)
        return (
          claim && (
            <blockquote key={id}>
              {t("repo.ingestRelatedStatement")}: {claim.text}
              <br />
              {t("repo.ingestSource")}: “{claim.source}”
            </blockquote>
          )
        )
      })}
    </div>
  )
}

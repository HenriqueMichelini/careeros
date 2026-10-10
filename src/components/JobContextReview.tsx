import type { JobContext, JobEvidence } from "../lib/jobContext"
import { useI18n } from "../lib/store"

export default function JobContextReview({ context }: { context: JobContext }) {
  const { t } = useI18n()
  const job = context.job
  function source(ref: JobEvidence) {
    return (
      <details className="mt-1 text-xs text-[var(--color-muted-fg)]">
        <summary className="cursor-pointer">{t("jobContext.source")}</summary>
        {ref.original.map((range, index) => (
          <blockquote
            key={index}
            className="whitespace-pre-wrap break-words mt-1 border-l-2 pl-2"
          >
            {range.text}
          </blockquote>
        ))}
      </details>
    )
  }
  return (
    <div className="my-5 space-y-4 text-sm break-words">
      <p className="text-xs text-[var(--color-muted-fg)]">
        {t("jobContext.disclosure")}
      </p>
      <dl className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {(["jobTitle", "company", "seniority", "location"] as const).map(
          (key) => (
            <div key={key}>
              <dt className="text-xs text-[var(--color-muted-fg)]">
                {t(`jobContext.${key}`)}
              </dt>
              <dd>
                {job[key]?.quote || t("jobContext.unknown")}
                {job[key] && source(job[key])}
              </dd>
            </div>
          ),
        )}
      </dl>
      {([
        "responsibilities",
        "qualifications",
        "applicationRequirements",
      ] as const).map((key) => (
        <section key={key}>
          <h3 className="font-medium mb-2">{t(`jobContext.${key}`)}</h3>
          {!job[key].length && (
            <p className="text-xs text-[var(--color-muted-fg)]">
              {t("jobContext.unknown")}
            </p>
          )}
          <ul className="space-y-3">
            {job[key].map((item, index) => (
              <li key={index}>
                <p>{item.source.quote}</p>
                <p className="text-xs text-[var(--color-muted-fg)]">
                  {t(`jobContext.${item.importance}`)}
                </p>
                {source(item.source)}
              </li>
            ))}
          </ul>
        </section>
      ))}
      <p className="text-xs text-[var(--color-muted-fg)]">
        {t("jobContext.edit")}
      </p>
    </div>
  )
}

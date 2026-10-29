import type { Page } from "../lib/types"
import { hasQualificationEvidence } from "../lib/qualificationEvidence"
import { useI18n, useStore } from "../lib/store"

export default function JourneyGuidance({
  page,
  setPage,
}: {
  page: Page
  setPage: (page: Page) => void
}) {
  const { state, profileDocument, profileSaving, accountId, profileSaved } =
    useStore()
  const { t } = useI18n()
  const profileReady =
    !state.profileError &&
    !profileSaving &&
    profileSaved &&
    hasQualificationEvidence(profileDocument)
  const profileMessage = accountId
    ? "journey.accountProfile"
    : "journey.profile"
  const guidance = {
    landing: "journey.apply",
    home: "journey.apply",
    repository: profileMessage,
    cv: "journey.cv",
    results: "journey.results",
  } as const
  const needsProfile = page === "home" && !profileReady

  return (
    <section
      aria-label={t("journey.context")}
      className="journey-guidance mx-auto max-w-6xl px-4 pt-6 sm:px-6"
    >
      <div
        className="flex flex-wrap items-start justify-between gap-3 border-b pb-4 text-sm"
        style={{ borderColor: "var(--color-border)" }}
      >
        <div className="min-w-0 max-w-2xl">
          <p className="font-semibold">
            {t(accountId ? "journey.accountMode" : "journey.localMode")}
          </p>
          <p className="mt-1" style={{ color: "var(--color-muted-fg)" }}>
            {t(guidance[page])}
          </p>
          {(page === "repository" || needsProfile) && (
            <p className="mt-2" role="status" aria-atomic="true">
              {state.profileError
                ? t("journey.profileUnavailable")
                : profileSaving
                  ? t(
                      accountId
                        ? "journey.accountSaving"
                        : "journey.profileSaving",
                    )
                  : profileReady
                    ? t(
                        accountId
                          ? "journey.accountReady"
                          : "journey.profileReady",
                      )
                    : t("journey.profileNeeded")}
            </p>
          )}
        </div>
        {(needsProfile || (page === "repository" && profileReady)) && (
          <button
            type="button"
            className="border px-4 py-2 font-semibold"
            style={{ borderColor: "var(--color-fg)" }}
            onClick={() => setPage(needsProfile ? "repository" : "home")}
          >
            {t(needsProfile ? "journey.openProfile" : "journey.continueApply")}
          </button>
        )}
      </div>
    </section>
  )
}

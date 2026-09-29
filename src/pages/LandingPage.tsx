import { Page } from "../lib/types"
import { TranslationKey } from "../lib/i18n"
import { useI18n } from "../lib/store"
import LanguageSelector from "../components/LanguageSelector"

interface Props {
  setPage: (page: Page) => void
}

const steps = [
  {
    number: "01",
    titleKey: "landing.stepOneTitle",
    descriptionKey: "landing.stepOneDescription",
  },
  {
    number: "02",
    titleKey: "landing.stepTwoTitle",
    descriptionKey: "landing.stepTwoDescription",
  },
  {
    number: "03",
    titleKey: "landing.stepThreeTitle",
    descriptionKey: "landing.stepThreeDescription",
  },
] satisfies {
  number: string
  titleKey: TranslationKey
  descriptionKey: TranslationKey
}[]

const outputs: TranslationKey[] = [
  "home.tailoredResume",
  "home.coverLetter",
  "home.applicationQa",
]
const storySignals: TranslationKey[] = [
  "landing.productThinking",
  "landing.crossFunctionalLeadership",
  "landing.customerEmpathy",
]
const highlights = [
  ["01", "landing.oneSource"],
  ["02", "landing.threeOutputs"],
  ["03", "landing.clearerStep"],
] satisfies [string, TranslationKey][]

function Brand() {
  return (
    <span className="flex items-center gap-2.5">
      <span
        className="inline-block h-5 w-5"
        style={{ backgroundColor: "var(--color-accent)" }}
        aria-hidden="true"
      />
      <span
        className="text-base font-bold uppercase tracking-[0.14em]"
        style={{ fontFamily: "var(--font-display)" }}
      >
        CareerOS
      </span>
    </span>
  )
}

function Arrow() {
  return <span aria-hidden="true">↗</span>
}

export default function LandingPage({ setPage }: Props) {
  const { t } = useI18n()

  return (
    <div
      className="min-h-screen overflow-hidden"
      style={{ backgroundColor: "var(--color-bg)" }}
    >
      <header
        className="border-b"
        style={{ borderColor: "var(--color-border)" }}
      >
        <div className="mx-auto flex h-[4.5rem] max-w-7xl items-center justify-between gap-6 px-6 lg:px-10">
          <a href="#top" aria-label={t("landing.careerOsHome")}>
            <Brand />
          </a>

          <nav
            className="hidden items-center gap-8 md:flex"
            aria-label={t("landing.marketingNavigation")}
          >
            <a
              href="#how-it-works"
              className="text-[11px] uppercase tracking-[0.18em] transition-opacity hover:opacity-60"
              style={{
                fontFamily: "var(--font-mono)",
                color: "var(--color-muted-fg)",
              }}
            >
              {t("landing.howItWorks")}
            </a>
            <a
              href="#outputs"
              className="text-[11px] uppercase tracking-[0.18em] transition-opacity hover:opacity-60"
              style={{
                fontFamily: "var(--font-mono)",
                color: "var(--color-muted-fg)",
              }}
            >
              {t("landing.whatYouGet")}
            </a>
          </nav>

          <div className="flex items-center gap-3">
            <LanguageSelector />
            <button
              onClick={() => setPage("home")}
              className="flex w-36 items-center justify-center gap-2 border px-4 py-2.5 text-[11px] uppercase tracking-[0.16em] transition-colors hover:bg-black hover:text-white"
              style={{
                fontFamily: "var(--font-mono)",
                borderColor: "var(--color-fg)",
                color: "var(--color-fg)",
              }}
            >
              {t("landing.openApp")} <Arrow />
            </button>
          </div>
        </div>
      </header>

      <main id="top">
        <section className="mx-auto grid max-w-7xl gap-14 px-6 pb-20 pt-16 lg:grid-cols-[minmax(0,0.92fr)_minmax(420px,1.08fr)] lg:items-center lg:gap-20 lg:px-10 lg:pb-28 lg:pt-24">
          <div>
            <div
              className="mb-7 flex items-center gap-2 text-[11px] uppercase tracking-[0.25em]"
              style={{
                fontFamily: "var(--font-mono)",
                color: "var(--color-muted-fg)",
              }}
            >
              <span
                className="h-1.5 w-1.5 rounded-full"
                style={{ backgroundColor: "var(--color-accent)" }}
              />
              {t("landing.tagline")}
            </div>

            <h1
              className="max-w-xl text-[4.7rem] font-bold uppercase leading-[0.86] tracking-[-0.035em] sm:text-[6.7rem] lg:text-[7.6rem]"
              style={{ fontFamily: "var(--font-display)" }}
            >
              {t("landing.titleStop")}
              <br />
              <span style={{ color: "var(--color-accent)" }}>
                {t("landing.titleFromScratch")}
              </span>
            </h1>

            <p
              className="mt-8 max-w-lg text-base leading-7"
              style={{ color: "var(--color-muted-fg)" }}
            >
              {t("landing.intro")}
            </p>

            <div className="mt-9 flex flex-wrap items-center gap-4">
              <button
                onClick={() => setPage("home")}
                className="flex items-center gap-3 px-6 py-3.5 text-sm font-semibold uppercase tracking-[0.16em] transition-opacity hover:opacity-80"
                style={{
                  fontFamily: "var(--font-display)",
                  backgroundColor: "var(--color-fg)",
                  color: "var(--color-bg)",
                }}
              >
                {t("landing.tryCareerOs")} <Arrow />
              </button>
              <a
                href="#how-it-works"
                className="px-2 py-3.5 text-[11px] uppercase tracking-[0.18em] transition-opacity hover:opacity-60"
                style={{
                  fontFamily: "var(--font-mono)",
                  color: "var(--color-fg)",
                }}
              >
                {t("landing.seeHow")}
              </a>
            </div>

            <p
              className="mt-6 text-[10px] uppercase tracking-[0.18em]"
              style={{
                fontFamily: "var(--font-mono)",
                color: "var(--color-muted-fg)",
              }}
            >
              {t("landing.profileOpportunityMove")}
            </p>
          </div>

          <div className="relative min-h-[430px] sm:min-h-[500px]">
            <div
              className="absolute right-2 top-1 h-52 w-52 rounded-full border sm:right-10 sm:h-72 sm:w-72"
              style={{ borderColor: "var(--color-border)" }}
              aria-hidden="true"
            />
            <div
              className="absolute bottom-0 left-1 h-20 w-20 sm:left-8 sm:h-28 sm:w-28"
              style={{ backgroundColor: "var(--color-accent)" }}
              aria-hidden="true"
            />

            <div
              className="absolute left-0 top-10 w-[calc(100%-1.5rem)] max-w-[570px] border shadow-[10px_10px_0_var(--color-fg)] sm:left-8 sm:top-14 sm:w-[calc(100%-2rem)]"
              style={{
                borderColor: "var(--color-fg)",
                backgroundColor: "var(--color-card)",
              }}
            >
              <div
                className="flex items-center justify-between border-b px-4 py-3"
                style={{ borderColor: "var(--color-border)" }}
              >
                <span
                  className="text-[9px] uppercase tracking-[0.2em]"
                  style={{
                    fontFamily: "var(--font-mono)",
                    color: "var(--color-muted-fg)",
                  }}
                >
                  {t("landing.studioLabel")}
                </span>
                <span
                  className="h-2 w-2 rounded-full"
                  style={{ backgroundColor: "var(--color-accent)" }}
                />
              </div>

              <div className="grid sm:grid-cols-[1.15fr_0.85fr]">
                <div
                  className="border-b p-6 sm:border-b-0 sm:border-r"
                  style={{ borderColor: "var(--color-border)" }}
                >
                  <div className="flex items-start justify-between gap-5">
                    <div>
                      <p
                        className="text-[9px] uppercase tracking-[0.2em]"
                        style={{
                          fontFamily: "var(--font-mono)",
                          color: "var(--color-muted-fg)",
                        }}
                      >
                        {t("landing.targetOpportunity")}
                      </p>
                      <h2
                        className="mt-4 text-3xl font-semibold leading-none"
                        style={{ fontFamily: "var(--font-display)" }}
                      >
                        Product Designer
                      </h2>
                      <p
                        className="mt-2 text-xs"
                        style={{ color: "var(--color-muted-fg)" }}
                      >
                        Wayfinder · Remote
                      </p>
                    </div>
                    <span
                      className="border px-2 py-1 text-[9px] uppercase tracking-[0.14em]"
                      style={{
                        fontFamily: "var(--font-mono)",
                        borderColor: "var(--color-accent)",
                        color: "var(--color-accent)",
                      }}
                    >
                      {t("landing.ready")}
                    </span>
                  </div>

                  <div className="mt-10">
                    <div className="mb-3 flex items-center justify-between">
                      <p
                        className="text-[9px] uppercase tracking-[0.2em]"
                        style={{
                          fontFamily: "var(--font-mono)",
                          color: "var(--color-muted-fg)",
                        }}
                      >
                        {t("landing.storySignals")}
                      </p>
                      <span
                        className="text-[10px]"
                        style={{
                          fontFamily: "var(--font-mono)",
                          color: "var(--color-accent)",
                        }}
                      >
                        {t("landing.found", { count: 4 })}
                      </span>
                    </div>
                    {storySignals.map((signalKey) => (
                      <div
                        key={signalKey}
                        className="flex items-center gap-3 border-t py-2.5"
                        style={{ borderColor: "var(--color-border)" }}
                      >
                        <span
                          className="h-1.5 w-1.5"
                          style={{ backgroundColor: "var(--color-accent)" }}
                        />
                        <span
                          className="text-xs"
                          style={{ color: "var(--color-fg)" }}
                        >
                          {t(signalKey)}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>

                <div
                  className="p-6"
                  style={{ backgroundColor: "var(--color-muted)" }}
                >
                  <p
                    className="text-[9px] uppercase tracking-[0.2em]"
                    style={{
                      fontFamily: "var(--font-mono)",
                      color: "var(--color-muted-fg)",
                    }}
                  >
                    {t("landing.yourApplication")}
                  </p>
                  <div className="mt-6 space-y-3">
                    {outputs.map((outputKey, index) => (
                      <div
                        key={outputKey}
                        className="flex items-center gap-3 border-b pb-3"
                        style={{ borderColor: "var(--color-border)" }}
                      >
                        <span
                          className="text-[10px]"
                          style={{
                            fontFamily: "var(--font-mono)",
                            color: "var(--color-accent)",
                          }}
                        >
                          {String(index + 1).padStart(2, "0")}
                        </span>
                        <span className="text-xs font-medium">
                          {t(outputKey)}
                        </span>
                      </div>
                    ))}
                  </div>
                  <div
                    className="mt-12 border-t pt-4"
                    style={{ borderColor: "var(--color-border)" }}
                  >
                    <p
                      className="text-[10px] leading-5"
                      style={{ color: "var(--color-muted-fg)" }}
                    >
                      {t("landing.builtFrom")}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </section>

        <div
          className="border-y"
          style={{ borderColor: "var(--color-border)" }}
        >
          <div className="mx-auto grid max-w-7xl gap-0 sm:grid-cols-3 lg:px-10">
            {highlights.map(([number, labelKey], index) => (
              <div
                key={number}
                className={`flex items-center gap-4 px-6 py-5 ${
                  index < 2 ? "border-b sm:border-b-0 sm:border-r" : ""
                }`}
                style={{ borderColor: "var(--color-border)" }}
              >
                <span
                  className="text-xs"
                  style={{
                    fontFamily: "var(--font-mono)",
                    color: "var(--color-accent)",
                  }}
                >
                  {number}
                </span>
                <span
                  className="text-xs uppercase tracking-[0.12em]"
                  style={{ fontFamily: "var(--font-mono)" }}
                >
                  {t(labelKey)}
                </span>
              </div>
            ))}
          </div>
        </div>

        <section
          id="how-it-works"
          className="mx-auto max-w-7xl scroll-mt-8 px-6 py-20 lg:px-10 lg:py-28"
        >
          <div className="grid gap-12 lg:grid-cols-[0.8fr_1.2fr] lg:gap-24">
            <div>
              <p
                className="mb-5 text-[11px] uppercase tracking-[0.25em]"
                style={{
                  fontFamily: "var(--font-mono)",
                  color: "var(--color-muted-fg)",
                }}
              >
                {t("landing.calmWorkflow")}
              </p>
              <h2
                className="max-w-md text-5xl font-bold uppercase leading-[0.9] tracking-[-0.02em] sm:text-6xl"
                style={{ fontFamily: "var(--font-display)" }}
              >
                {t("landing.bestApplications")}{" "}
                <span style={{ color: "var(--color-accent)" }}>
                  {t("landing.you")}
                </span>
              </h2>
              <p
                className="mt-6 max-w-sm text-sm leading-6"
                style={{ color: "var(--color-muted-fg)" }}
              >
                {t("landing.workflowIntro")}
              </p>
            </div>

            <div>
              {steps.map((step) => (
                <div
                  key={step.number}
                  className="grid gap-4 border-t py-6 sm:grid-cols-[56px_0.8fr_1fr] sm:gap-6"
                  style={{ borderColor: "var(--color-border)" }}
                >
                  <span
                    className="text-xs"
                    style={{
                      fontFamily: "var(--font-mono)",
                      color: "var(--color-accent)",
                    }}
                  >
                    {step.number}
                  </span>
                  <h3
                    className="text-2xl leading-none"
                    style={{ fontFamily: "var(--font-display)" }}
                  >
                    {t(step.titleKey)}
                  </h3>
                  <p
                    className="max-w-xs text-sm leading-6"
                    style={{ color: "var(--color-muted-fg)" }}
                  >
                    {t(step.descriptionKey)}
                  </p>
                </div>
              ))}
              <div
                className="border-t"
                style={{ borderColor: "var(--color-border)" }}
              />
            </div>
          </div>
        </section>

        <section
          id="outputs"
          className="scroll-mt-8 border-y"
          style={{ borderColor: "var(--color-border)" }}
        >
          <div className="mx-auto grid max-w-7xl gap-12 px-6 py-20 lg:grid-cols-[1fr_1fr] lg:items-center lg:gap-24 lg:px-10 lg:py-24">
            <div>
              <p
                className="mb-5 text-[11px] uppercase tracking-[0.25em]"
                style={{
                  fontFamily: "var(--font-mono)",
                  color: "var(--color-muted-fg)",
                }}
              >
                {t("landing.lessBusywork")}
              </p>
              <h2
                className="max-w-lg text-5xl font-bold uppercase leading-[0.9] tracking-[-0.02em] sm:text-6xl"
                style={{ fontFamily: "var(--font-display)" }}
              >
                {t("landing.oneProfile")}
                <br />
                <span style={{ color: "var(--color-accent)" }}>
                  {t("landing.threePrecise")}
                </span>
              </h2>
            </div>

            <div
              className="border p-6 sm:p-8"
              style={{
                borderColor: "var(--color-fg)",
                backgroundColor: "var(--color-card)",
              }}
            >
              <div
                className="flex items-center justify-between border-b pb-5"
                style={{ borderColor: "var(--color-border)" }}
              >
                <span
                  className="text-[10px] uppercase tracking-[0.2em]"
                  style={{
                    fontFamily: "var(--font-mono)",
                    color: "var(--color-muted-fg)",
                  }}
                >
                  {t("landing.readyWhen")}
                </span>
                <span
                  className="h-2 w-2"
                  style={{ backgroundColor: "var(--color-accent)" }}
                />
              </div>
              <div
                className="divide-y"
                style={{ borderColor: "var(--color-border)" }}
              >
                {outputs.map((outputKey, index) => (
                  <div
                    key={outputKey}
                    className="flex items-center justify-between py-5"
                  >
                    <div className="flex items-center gap-4">
                      <span
                        className="text-xs"
                        style={{
                          fontFamily: "var(--font-mono)",
                          color: "var(--color-accent)",
                        }}
                      >
                        {String(index + 1).padStart(2, "0")}
                      </span>
                      <span
                        className="text-lg"
                        style={{ fontFamily: "var(--font-display)" }}
                      >
                        {t(outputKey)}
                      </span>
                    </div>
                    <Arrow />
                  </div>
                ))}
              </div>
              <p
                className="mt-5 text-xs leading-5"
                style={{ color: "var(--color-muted-fg)" }}
              >
                {t("landing.startWithWork")}
              </p>
            </div>
          </div>
        </section>

        <section className="mx-auto max-w-7xl px-6 py-20 lg:px-10 lg:py-28">
          <div
            className="relative overflow-hidden px-6 py-14 sm:px-12 sm:py-20 lg:px-20"
            style={{
              backgroundColor: "var(--color-fg)",
              color: "var(--color-bg)",
            }}
          >
            <div
              className="absolute -right-14 -top-24 h-64 w-64 rounded-full border border-white/20"
              aria-hidden="true"
            />
            <div className="relative max-w-2xl">
              <p
                className="mb-5 text-[11px] uppercase tracking-[0.25em]"
                style={{ fontFamily: "var(--font-mono)", color: "#A9A8A3" }}
              >
                {t("landing.nextStarts")}
              </p>
              <h2
                className="text-5xl font-bold uppercase leading-[0.88] tracking-[-0.02em] sm:text-7xl"
                style={{ fontFamily: "var(--font-display)" }}
              >
                {t("landing.makeStory")}{" "}
                <span style={{ color: "#FF3659" }}>{t("landing.use")}</span>
              </h2>
              <button
                onClick={() => setPage("home")}
                className="mt-9 flex items-center gap-3 px-6 py-3.5 text-sm font-semibold uppercase tracking-[0.16em] transition-opacity hover:opacity-80"
                style={{
                  fontFamily: "var(--font-display)",
                  backgroundColor: "var(--color-bg)",
                  color: "var(--color-fg)",
                }}
              >
                {t("landing.openCareerOs")} <Arrow />
              </button>
            </div>
          </div>
        </section>
      </main>

      <footer
        className="border-t"
        style={{ borderColor: "var(--color-border)" }}
      >
        <div className="mx-auto flex max-w-7xl flex-col gap-3 px-6 py-6 sm:flex-row sm:items-center sm:justify-between lg:px-10">
          <Brand />
          <p
            className="text-[10px] uppercase tracking-[0.16em]"
            style={{
              fontFamily: "var(--font-mono)",
              color: "var(--color-muted-fg)",
            }}
          >
            {t("landing.footer")}
          </p>
        </div>
      </footer>
    </div>
  )
}

import { useI18n, useStore } from "../lib/store"

export default function LanguageSelector({
  kind = "site",
}: {
  kind?: "site" | "cv"
}) {
  const { uiLocale, setUiLocale, t } = useI18n()

  const { state, dispatch } = useStore()
  const label = t(kind === "cv" ? "language.cv" : "language.site")
  const value = kind === "cv" ? state.cvLanguage : uiLocale

  return (
      <select
        aria-label={label}
        value={value}
        onChange={(event) =>
          kind === "cv"
            ? dispatch({
                type: "SET_CV_LANGUAGE",
                payload: event.target.value as typeof uiLocale,
              })
            : setUiLocale(event.target.value as typeof uiLocale)
        }
        className="w-28 shrink-0 text-xs px-2 py-1.5 border bg-transparent focus:outline-none"
        style={{
          fontFamily: "var(--font-mono)",
          borderColor: "var(--color-border)",
          color: "var(--color-muted-fg)",
        }}
      >
        <option value="en">{t("language.english")}</option>
        <option value="pt-BR">{t("language.portuguese")}</option>
      </select>
  )
}

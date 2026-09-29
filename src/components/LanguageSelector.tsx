import { useI18n } from "../lib/store"

export default function LanguageSelector() {
  const { locale, setLocale, t } = useI18n()

  return (
    <select
      aria-label={t("language.label")}
      value={locale}
      onChange={(event) => setLocale(event.target.value as typeof locale)}
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

import { useState } from "react"
import { Page } from "../lib/types"
import { useI18n, useStore } from "../lib/store"
import LanguageSelector from "./LanguageSelector"

interface Props {
  page: Page
  setPage: (p: Page) => void
  children: React.ReactNode
}

export default function Layout({ page, setPage, children }: Props) {
  const { state, dispatch } = useStore()
  const { t } = useI18n()
  const [showKey, setShowKey] = useState(false)
  const [keyDraft, setKeyDraft] = useState(state.apiKey)

  const navItems: { id: Page; label: string }[] = [
    { id: "home", label: t("nav.apply") },
    { id: "repository", label: t("nav.profile") },
    { id: "cv", label: t("nav.cv") },
    { id: "results", label: t("nav.results") },
  ]

  function saveKey() {
    dispatch({ type: "SET_API_KEY", payload: keyDraft.trim() })
    setShowKey(false)
  }

  if (page === "landing") {
    return (
      <div
        className="min-h-screen"
        style={{ backgroundColor: "var(--color-bg)", color: "var(--color-fg)" }}
      >
        {children}
      </div>
    )
  }

  return (
    <div
      className="min-h-screen flex flex-col"
      style={{ backgroundColor: "var(--color-bg)", color: "var(--color-fg)" }}
    >
      <header
        className="sticky top-0 z-50 border-b"
        style={{
          borderColor: "var(--color-border)",
          backgroundColor: "var(--color-bg)",
        }}
      >
        <div className="max-w-6xl mx-auto px-6 h-14 flex items-center justify-between gap-8">
          {/* Wordmark */}
          <div className="flex items-center gap-2 flex-shrink-0">
            <div
              className="w-5 h-5 flex-shrink-0"
              style={{ backgroundColor: "var(--color-accent)" }}
            />
            <span
              className="text-base font-bold tracking-[0.12em] uppercase select-none"
              style={{ fontFamily: "var(--font-display)" }}
            >
              CareerOS
            </span>
          </div>

          {/* Nav */}
          <nav className="flex items-center gap-1">
            {navItems.map(({ id, label }) => (
              <button
                key={id}
                onClick={() => setPage(id)}
                className="px-4 py-1.5 text-xs uppercase tracking-[0.18em] transition-colors"
                style={{
                  fontFamily: "var(--font-mono)",
                  backgroundColor:
                    page === id ? "var(--color-fg)" : "transparent",
                  color:
                    page === id ? "var(--color-bg)" : "var(--color-muted-fg)",
                }}
              >
                {label}
              </button>
            ))}
          </nav>

          {/* Preferences and API key */}
          <div className="grid grid-cols-[7rem_12rem] gap-3 flex-shrink-0">
            <LanguageSelector />
            <button
              onClick={() => {
                setKeyDraft(state.apiKey)
                setShowKey((v) => !v)
              }}
              className="w-full text-xs px-3 py-1.5 border text-center transition-colors"
              style={{
                fontFamily: "var(--font-mono)",
                borderColor: state.apiKey
                  ? "var(--color-border)"
                  : "var(--color-accent)",
                color: state.apiKey
                  ? "var(--color-muted-fg)"
                  : "var(--color-accent)",
              }}
            >
              {state.apiKey ? t("api.keyConfigured") : t("api.setKey")}
            </button>
          </div>
        </div>

        {showKey && (
          <div
            className="border-t"
            style={{
              borderColor: "var(--color-border)",
              backgroundColor: "var(--color-card)",
            }}
          >
            <div className="max-w-6xl mx-auto px-6 py-3 flex items-center gap-4">
              <span
                className="text-xs uppercase tracking-widest flex-shrink-0"
                style={{
                  fontFamily: "var(--font-mono)",
                  color: "var(--color-muted-fg)",
                }}
              >
                {t("api.anthropicKey")}
              </span>
              <input
                type="password"
                value={keyDraft}
                onChange={(e) => setKeyDraft(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && saveKey()}
                placeholder={t("api.placeholder")}
                className="flex-1 text-sm bg-transparent border-b py-1 focus:outline-none transition-colors"
                style={{
                  fontFamily: "var(--font-mono)",
                  borderColor: "var(--color-border)",
                }}
                autoFocus
              />
              <button
                onClick={saveKey}
                className="text-xs uppercase tracking-widest px-4 py-1.5 transition-opacity hover:opacity-75 flex-shrink-0"
                style={{
                  fontFamily: "var(--font-mono)",
                  backgroundColor: "var(--color-fg)",
                  color: "var(--color-bg)",
                }}
              >
                {t("common.save")}
              </button>
            </div>
          </div>
        )}
      </header>

      <main className="flex-1">{children}</main>
    </div>
  )
}

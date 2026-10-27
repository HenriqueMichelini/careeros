import { useEffect, useRef, useState, type ReactNode } from "react"
import { AccountProfile, csrfHeaders } from "../lib/accountProfile"
import { getInitialLocale } from "../lib/i18n"
import { StoreProvider } from "../lib/store"

interface Session {
  authenticated: boolean
  account?: { id: string }
  expiresAt?: string
}

export default function AccountGate({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null)
  const [error, setError] = useState(false)
  const [client, setClient] = useState<AccountProfile | null>(null)
  const [checking, setChecking] = useState(true)
  const sessionKey = useRef("")
  const current = useRef<AccountProfile | null>(null)
  const epoch = useRef(0)
  const pt = getInitialLocale() === "pt-BR"
  const probe = async () => {
    const ticket = ++epoch.current
    // Hide while checking, retaining same-account unsaved work. Dispose before
    // displaying another account; never attach A's client or queue to B.
    setChecking(true)
    try {
      const response = await fetch("/api/auth/session", {
        credentials: "same-origin",
        cache: "no-store",
        signal: AbortSignal.timeout(10_000),
      })
      if (!response.ok) throw new Error()
      const value: Session = await response.json()
      if (ticket !== epoch.current) return
      if (
        typeof value.authenticated !== "boolean" ||
        (value.authenticated &&
          (!value.account?.id ||
            !value.expiresAt ||
            !Number.isFinite(Date.parse(value.expiresAt))))
      )
        throw new Error()
      setSession(value)
      setError(false)
      const key = value.authenticated
        ? `${value.account!.id}:${value.expiresAt}`
        : ""
      if (key !== sessionKey.current || !current.current) {
        current.current?.dispose()
        current.current = null
        setClient(null)
      }
      if (value.authenticated && !current.current) {
        const next = new AccountProfile(value.account!.id)
        current.current = next
        setClient(next)
      }
      sessionKey.current = key
      setChecking(false)
    } catch {
      if (ticket === epoch.current) {
        setError(true)
        setChecking(true)
      }
    }
  }
  useEffect(() => {
    void probe()
    const focus = () => void probe()
    const visible = () => {
      if (document.visibilityState === "visible") void probe()
    }
    window.addEventListener("focus", focus)
    document.addEventListener("visibilitychange", visible)
    return () => {
      epoch.current++
      current.current?.dispose()
      window.removeEventListener("focus", focus)
      document.removeEventListener("visibilitychange", visible)
    }
  }, [])
  useEffect(() => {
    if (!session?.expiresAt) return
    const timer = setTimeout(
      () => void probe(),
      Math.min(
        2_147_483_647,
        Math.max(0, Date.parse(session.expiresAt) - Date.now()),
      ),
    )
    return () => clearTimeout(timer)
  }, [session?.expiresAt])
  const logout = async () => {
    ++epoch.current
    current.current?.dispose()
    current.current = null
    setClient(null)
    setSession(null)
    try {
      const response = await fetch("/api/auth/logout", {
        method: "POST",
        credentials: "same-origin",
        headers: csrfHeaders(),
      })
      if (!response.ok) throw new Error()
      await probe()
    } catch {
      setError(true)
    }
  }
  if (!client)
    return (
      <main className="mx-auto max-w-xl space-y-4 p-8">
        <h1 className="text-2xl font-semibold">CareerOS</h1>
        {error ? (
          <>
            <p role="alert">
              {pt
                ? "Não foi possível verificar sua sessão. Tente novamente."
                : "Your session could not be checked. Try again."}
            </p>
            <button onClick={() => void probe()}>
              {pt ? "Tentar novamente" : "Try again"}
            </button>
          </>
        ) : session?.authenticated === false ? (
          <>
            <p>
              {pt
                ? "Entre para criar e salvar seu Perfil na sua conta."
                : "Sign in to create and save your account Profile."}
            </p>
            <a
              className="inline-block rounded bg-indigo-700 px-4 py-2 text-white"
              href="/api/auth/google/start"
            >
              {pt ? "Entrar com Google" : "Sign in with Google"}
            </a>
          </>
        ) : (
          <p role="status">
            {pt ? "Verificando sessão…" : "Checking session…"}
          </p>
        )}
      </main>
    )
  return (
    <>
      {checking && (
        <div role={error ? "alert" : "status"} className="p-4">
          {error ? (
            <button onClick={() => void probe()}>
              {pt
                ? "Verificação indisponível. Tentar novamente"
                : "Session check unavailable. Try again"}
            </button>
          ) : pt ? (
            "Verificando sessão…"
          ) : (
            "Checking session…"
          )}
        </div>
      )}
      <div hidden={checking}>
        <StoreProvider
          key={`${client.accountId}:${session?.expiresAt}`}
          account={client}
        >
          <div className="flex items-center justify-between border-b px-4 py-2 text-sm">
            <span>
              {pt
                ? "Perfil da conta · salvar explicitamente"
                : "Account Profile · explicit Save"}
            </span>
            <button onClick={() => void logout()}>
              {pt ? "Sair" : "Sign out"}
            </button>
          </div>
          {children}
        </StoreProvider>
      </div>
    </>
  )
}

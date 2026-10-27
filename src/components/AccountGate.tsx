import { useEffect, useRef, useState, type ReactNode } from "react"
import { AccountProfile, csrfHeaders } from "../lib/accountProfile"
import { getInitialLocale } from "../lib/i18n"
import { StoreProvider } from "../lib/store"

interface Session {
  authenticated: boolean
  account?: { id: string }
  expiresAt?: string
}

export default function AccountGate({
  children,
}: Readonly<{ children: ReactNode }>) {
  const [session, setSession] = useState<Session | null>(null)
  const [error, setError] = useState(false)
  const [client, setClient] = useState<AccountProfile | null>(null)
  const [checking, setChecking] = useState(true)
  const [logoutIntent, setLogoutIntent] = useState(false)
  const clients = useRef(new Map<string, AccountProfile>())
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
      if (!response.ok) throw new Error("Session request failed")
      const value: Session = await response.json()
      if (ticket !== epoch.current) return
      if (
        typeof value.authenticated !== "boolean" ||
        (value.authenticated &&
          (!value.account?.id ||
            !value.expiresAt ||
            !Number.isFinite(Date.parse(value.expiresAt))))
      )
        throw new Error("Invalid session response")
      setSession(value)
      setError(false)
      const accountId = value.authenticated ? value.account!.id : null
      if (current.current?.accountId !== accountId) {
        current.current?.dispose()
        current.current = null
        setClient(null)
        setLogoutIntent(false)
      }
      if (accountId && !current.current) {
        const next =
          clients.current.get(accountId) ?? new AccountProfile(accountId)
        clients.current.set(accountId, next)
        next.resume()
        current.current = next
        setClient(next)
      }
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
      for (const retained of clients.current.values()) retained.dispose()
      window.removeEventListener("focus", focus)
      document.removeEventListener("visibilitychange", visible)
    }
  }, [])
  useEffect(() => {
    if (!session?.authenticated || !session.expiresAt) return
    const timer = setTimeout(
      () => void probe(),
      Math.min(
        2_147_483_647,
        Math.max(0, Date.parse(session.expiresAt) - Date.now()),
      ),
    )
    return () => clearTimeout(timer)
  }, [session?.authenticated, session?.expiresAt])
  const logout = async (discard = false) => {
    if (discard) {
      try {
        current.current?.discardRecovery()
      } catch {
        setError(true)
        return
      }
    }
    setLogoutIntent(false)
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
      if (!response.ok) throw new Error("Logout request failed")
      await probe()
    } catch {
      setError(true)
    }
  }
  const pendingInTab = [...clients.current.values()].some((retained) =>
    retained.hasRecovery(),
  )
  const downloadPending = () => {
    if (!client) return
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(client.recoveryExport(), null, 2)], {
        type: "application/json",
      }),
    )
    const anchor = document.createElement("a")
    anchor.href = url
    anchor.download = "careeros-pending-save-recovery.json"
    anchor.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  if (!client)
    return (
      <SignedOutGate
        pt={pt}
        error={error}
        session={session}
        pendingInTab={pendingInTab}
        retry={() => void probe()}
      />
    )
  return (
    <>
      <SessionCheck
        checking={checking}
        error={error}
        pt={pt}
        retry={() => void probe()}
      />
      <div hidden={checking}>
        <StoreProvider key={client.accountId} account={client}>
          <div className="flex items-center justify-between border-b px-4 py-2 text-sm">
            <span>
              {pt
                ? "Perfil da conta · salvar explicitamente"
                : "Account Profile · explicit Save"}
            </span>
            <button
              data-sign-out
              onClick={() =>
                client.hasRecovery() ? setLogoutIntent(true) : void logout()
              }
            >
              {pt ? "Sair" : "Sign out"}
            </button>
          </div>
          {logoutIntent && (
            <PendingLogout
              pt={pt}
              error={error}
              download={downloadPending}
              keep={() => void logout()}
              discard={() => void logout(true)}
              cancel={() => setLogoutIntent(false)}
            />
          )}
          {children}
        </StoreProvider>
      </div>
    </>
  )
}

function SessionCheck({
  checking,
  error,
  pt,
  retry,
}: Readonly<{
  checking: boolean
  error: boolean
  pt: boolean
  retry: () => void
}>) {
  if (!checking) return null
  if (error)
    return (
      <div role="alert" className="p-4">
        <button onClick={retry}>
          {pt
            ? "Verificação indisponível. Tentar novamente"
            : "Session check unavailable. Try again"}
        </button>
      </div>
    )
  return (
    <output className="block p-4">
      {pt ? "Verificando sessão…" : "Checking session…"}
    </output>
  )
}

function SignedOutGate({
  pt,
  error,
  session,
  pendingInTab,
  retry,
}: Readonly<{
  pt: boolean
  error: boolean
  session: Session | null
  pendingInTab: boolean
  retry: () => void
}>) {
  let content = (
    <output>{pt ? "Verificando sessão…" : "Checking session…"}</output>
  )
  if (error) {
    content = (
      <>
        <p role="alert">
          {pt
            ? "Não foi possível verificar sua sessão. Tente novamente."
            : "Your session could not be checked. Try again."}
        </p>
        <button onClick={retry}>{pt ? "Tentar novamente" : "Try again"}</button>
      </>
    )
  } else if (session?.authenticated === false) {
    content = <SignInContent pt={pt} pendingInTab={pendingInTab} />
  }
  return (
    <main className="mx-auto max-w-xl space-y-4 p-8">
      <h1 className="text-2xl font-semibold">CareerOS</h1>
      {content}
    </main>
  )
}

function PendingLogout({
  pt,
  error,
  download,
  keep,
  discard,
  cancel,
}: Readonly<{
  pt: boolean
  error: boolean
  download: () => void
  keep: () => void
  discard: () => void
  cancel: () => void
}>) {
  return (
    <dialog
      open
      aria-label={
        pt
          ? "Salvar recuperação antes de sair"
          : "Keep recovery before signing out"
      }
      data-pending-logout
      className="static m-0 w-full max-w-none border-0 border-b p-4"
    >
      {error && (
        <p role="alert">
          {pt
            ? "Não foi possível descartar a recuperação. Baixe uma cópia e tente novamente."
            : "Recovery could not be discarded. Download a copy and try again."}
        </p>
      )}
      <p>
        {pt
          ? "Um salvamento ainda não foi confirmado. Manter ou descartar a cópia de recuperação? Descartar não desfaz um salvamento no servidor."
          : "A Save is not confirmed. Keep or discard its recovery copy? Discarding does not undo a server Save."}
      </p>
      <div className="flex flex-wrap gap-4 py-2">
        <button data-logout-download onClick={download}>
          {pt ? "Baixar cópia de recuperação" : "Download recovery copy"}
        </button>
        <button data-logout-keep onClick={keep}>
          {pt
            ? "Sair e manter recuperação nesta aba"
            : "Sign out and keep recovery in this tab"}
        </button>
        <button data-logout-discard onClick={discard}>
          {pt
            ? "Descartar recuperação e sair"
            : "Discard recovery and sign out"}
        </button>
        <button data-logout-cancel onClick={cancel}>
          {pt ? "Cancelar" : "Cancel"}
        </button>
      </div>
    </dialog>
  )
}

function SignInContent({
  pt,
  pendingInTab,
}: Readonly<{
  pt: boolean
  pendingInTab: boolean
}>) {
  return (
    <>
      <p>
        {pt
          ? "Entre para criar e salvar seu Perfil na sua conta."
          : "Sign in to create and save your account Profile."}
      </p>
      <a
        className="inline-block rounded bg-indigo-700 px-4 py-2 text-white"
        href="/api/auth/google/start"
        target={pendingInTab ? "_blank" : undefined}
        rel={pendingInTab ? "noopener" : undefined}
      >
        {pt ? "Entrar com Google" : "Sign in with Google"}
      </a>
      {pendingInTab && (
        <p>
          {pt
            ? "O trabalho pendente continua nesta aba. Entre na outra aba e volte aqui para recuperar somente o trabalho da mesma conta."
            : "Pending work remains in this tab. Sign in in the other tab, then return here to recover work for that same account."}
        </p>
      )}
    </>
  )
}

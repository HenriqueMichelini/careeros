import { useEffect, useRef, useState, type ReactNode } from "react"
import { AccountProfile, csrfHeaders } from "../lib/accountProfile"
import { getInitialLocale } from "../lib/i18n"
import PilotAccess from "./PilotAccess"
import {readLifecycle} from "../lib/accountLifecycle"
import {ProfileStorageError} from "../lib/profileStorage"
import AccountDeletion, { deletionEvent, deletionSignalKey } from "./AccountDeletion"
import AccountLifecycle from "./AccountLifecycle"
import { StoreProvider } from "../lib/store"

interface Session {
  authenticated: boolean
  deletionStatusOnly?: boolean
  account?: { id: string }
  expiresAt?: string
}

export default function AccountGate({
  children,
  pilot = false,
}: Readonly<{
 children: ReactNode
 pilot?: boolean
}>) {
  const [session, setSession] = useState<Session | null>(null)
  const [error, setError] = useState(false)
  const [client, setClient] = useState<AccountProfile | null>(null)
  const [checking, setChecking] = useState(true)
  const [logoutIntent, setLogoutIntent] = useState(false)
  const [deletionPending,setDeletionPending] = useState(false)
  const [careerDenied,setCareerDenied] = useState(false)
  const careerReadAllowed = useRef(!pilot)
  const clients = useRef(new Map<string, AccountProfile>())
  const current = useRef<AccountProfile | null>(null)
  const epoch = useRef(0)
  const pt = getInitialLocale() === "pt-BR"
  const withholdCareer = (accountId: string) => {
    if (current.current?.accountId !== accountId) return
    careerReadAllowed.current = false
    setCareerDenied(true)
    window.dispatchEvent(new CustomEvent("careeros-career-read-denied", { detail: { accountId } }))
  }
  const probe = async () => {
    const ticket = ++epoch.current
    careerReadAllowed.current=false
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
      if(value.deletionStatusOnly&&value.account?.id){
        const old=clients.current.get(value.account.id)
        old?.forgetDeletedWork();clients.current.delete(value.account.id)
        window.dispatchEvent(new CustomEvent(deletionEvent,{detail:{accountId:value.account.id}}))
      }
      let canRead=true
      if(accountId&&pilot){canRead=(await readLifecycle(accountId)).careerReadAllowed}
      if(ticket!==epoch.current)return
      setCareerDenied(!canRead)
      if(!canRead&&accountId)window.dispatchEvent(new CustomEvent("careeros-career-read-denied",{detail:{accountId}}))
      if(accountId&&current.current?.accountId===accountId){
        if(ticket!==epoch.current)return
        const before=JSON.stringify(current.current.deletedProfileIds)
        if(canRead){try{await current.current.open()}catch(e){if(e instanceof ProfileStorageError&&e.code==="forbidden"){canRead=false;setCareerDenied(true);window.dispatchEvent(new CustomEvent("careeros-career-read-denied",{detail:{accountId}}))}else throw e}}
        if(ticket!==epoch.current)return
        if(before!==JSON.stringify(current.current.deletedProfileIds)){
          window.dispatchEvent(new CustomEvent(deletionEvent,{detail:{accountId}}))
          window.location.reload()
          return
        }
      }
      if (current.current?.accountId !== accountId) {
        current.current?.dispose()
        current.current = null
        setClient(null)
        setLogoutIntent(false)
        setDeletionPending(false)
      }
      if (accountId && !current.current) {
        const next =
          clients.current.get(accountId) ?? new AccountProfile(accountId)
        clients.current.set(accountId, next)
        next.resume()
        current.current = next
        setClient(next)
      }
      careerReadAllowed.current=canRead&&accountId!==null
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
    const deleted=(event:Event)=>{
      const owner=(event as CustomEvent<{accountId?:string}>).detail?.accountId
      if(!owner||!current.current||owner!==current.current.accountId)return
      setDeletionPending(true);current.current.forgetDeletedWork()
    }
    const otherTab=(event:StorageEvent)=>{
      if(event.key!==deletionSignalKey||!event.newValue)return
      try{const v=JSON.parse(event.newValue);if(v.accountId===current.current?.accountId){deleted(new CustomEvent(deletionEvent,{detail:v}));window.location.reload()}}catch{/* Invalid metadata is not authority. */}
    }
    window.addEventListener(deletionEvent,deleted)
    window.addEventListener("storage",otherTab)
    document.addEventListener("visibilitychange", visible)
    return () => {
      epoch.current++
      for (const retained of clients.current.values()) retained.dispose()
      window.removeEventListener("focus", focus)
      window.removeEventListener(deletionEvent,deleted)
      window.removeEventListener("storage",otherTab)
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
    if (!client || !careerReadAllowed.current || careerDenied || checking) return
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
  if(!checking&&session?.deletionStatusOnly&&session.account?.id)
    return <AccountDeletion key={session.account.id} accountId={session.account.id} pt={pt}/>
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
      {!checking&&<AccountDeletion key={client.accountId} accountId={client.accountId} account={client} pt={pt} withhold={()=>setDeletionPending(true)}/>}
      {!checking&&pilot&&<AccountLifecycle key={client.accountId} accountId={client.accountId} pt={pt} denied={()=>{withholdCareer(client.accountId);void probe()}} readDenied={()=>withholdCareer(client.accountId)} expired={()=>window.dispatchEvent(new CustomEvent(deletionEvent,{detail:{accountId:client.accountId}}))}/>}
      <div hidden={checking||deletionPending}>
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
            canDownload={!careerDenied&&!checking}
            keep={() => void logout()}
            discard={() => void logout(true)}
            cancel={() => setLogoutIntent(false)}
          />
        )}
        <PilotFrame pilot={pilot} accountId={client.accountId} pt={pt}>
          <div hidden={careerDenied}>
            <StoreProvider key={client.accountId} account={client}>
              {children}
            </StoreProvider>
          </div>
        </PilotFrame>
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
  canDownload,
  keep,
  discard,
  cancel,
}: Readonly<{
  pt: boolean
  error: boolean
  download: () => void
  canDownload: boolean
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
        {canDownload&&<button data-logout-download onClick={download}>
          {pt ? "Baixar cópia de recuperação" : "Download recovery copy"}
        </button>}
        {!canDownload&&<p>{pt?"O download de recuperação fica oculto até que o acesso ao Perfil seja verificado.":"Recovery download is withheld until Profile access is verified."}</p>}
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

function PilotFrame({
  pilot,
  accountId,
  pt,
  children,
}: Readonly<{
  pilot: boolean
  accountId: string
  pt: boolean
  children: ReactNode
}>) {
  return pilot ? (
    <PilotAccess key={accountId} accountId={accountId} pt={pt}>
      {children}
    </PilotAccess>
  ) : (
    children
  )
}

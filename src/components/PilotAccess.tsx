import { useEffect, useRef, useState, type ReactNode } from "react"
import { PilotAccessClient, type PilotStatus } from "../lib/pilotAccess"

const labels: Record<string, [string, string]> = {
  uninvited: [
    "An account-bound invitation and eligibility clearance are required.",
    "É necessário um convite vinculado à conta e liberação de elegibilidade.",
  ],
  invited: [
    "Your invitation is awaiting activation or eligibility clearance.",
    "Seu convite aguarda ativação ou liberação de elegibilidade.",
  ],
  invitation_expired: [
    "Your invitation expired. Ask the operator to review a reissue.",
    "Seu convite expirou. Peça ao operador uma análise para reemissão.",
  ],
  invitation_revoked: [
    "Your invitation was revoked.",
    "Seu convite foi revogado.",
  ],
  active: [
    "Your 14-day pilot access is active. Five eligible complete Draft deliveries are included. Standalone workflows do not deduct Draft allowance.",
    "Seu acesso piloto de 14 dias está ativo. Estão incluídas cinco entregas elegíveis de Rascunho completo. Fluxos independentes não descontam essa quantidade.",
  ],
  retrieval_only: [
    "Pilot access ended. You can view/export your retained Profile and retrieve/export previously delivered complete Drafts during the retrieval window. New AI generation and Profile Save are unavailable.",
    "O acesso piloto terminou. Você pode visualizar/exportar seu Perfil retido e recuperar/exportar Rascunhos completos já entregues durante o prazo de recuperação. Nova geração por IA e salvamento do Perfil estão indisponíveis.",
  ],
  expired: [
    "The retrieval/export window ended. Access expiry is separate from deletion completion.",
    "O prazo de recuperação/exportação terminou. O fim do acesso é distinto da conclusão da exclusão.",
  ],
  suspended: [
    "Pilot access is suspended. Your account identity has not changed.",
    "O acesso piloto está suspenso. A identidade da sua conta não mudou.",
  ],
  revoked: [
    "Pilot access was revoked. Your account identity has not changed.",
    "O acesso piloto foi revogado. A identidade da sua conta não mudou.",
  ],
}

export default function PilotAccess({
  accountId,
  pt,
  children,
}: Readonly<{
 accountId: string
 pt: boolean
 children: ReactNode
}>) {
  const [client] = useState(() => new PilotAccessClient(accountId))
  const [status, setStatus] = useState<PilotStatus | null>(null)
  const [error, setError] = useState(false)
  const [busy, setBusy] = useState(false)
  const [checking, setChecking] = useState(true)
  const retained = useRef(false)
  const mutating = useRef(false)
  const epoch = useRef(0)
  const probe = async () => {
    if (mutating.current) return
    const ticket = ++epoch.current
    setChecking(true)
    try {
      const value = await client.refresh()
      if (ticket !== epoch.current) return
      setStatus(value)
      setError(false)
      setChecking(false)
    } catch {
      if (ticket === epoch.current) {
        setError(true)
        setChecking(false)
      }
    }
  }
  useEffect(() => {
    client.resume()
    void probe()
    const focus = () => void probe()
    window.addEventListener("focus", focus)
    const interval = setInterval(focus, 30_000)
    return () => {
      ++epoch.current
      client.dispose()
      clearInterval(interval)
      window.removeEventListener("focus", focus)
    }
  }, [client])
  useEffect(() => {
    const boundary =
      status?.state === "active" ? status.expiresAt : status?.retrieveUntil
    if (!boundary) return
    const remaining = Date.parse(boundary) - Date.now()
    if (remaining <= 0) return
    const timer = setTimeout(
      () => void probe(),
      Math.min(2_147_483_647, remaining),
    )
    return () => clearTimeout(timer)
  }, [status?.expiresAt, status?.retrieveUntil, status?.state])
  const activate = async () => {
    const ticket = ++epoch.current
    mutating.current = true
    setBusy(true)
    try {
      const value = await client.activate()
      if (ticket === epoch.current) {
        setStatus(value)
        setError(false)
      }
    } catch {
      if (ticket === epoch.current) setError(true)
    } finally {
      mutating.current = false
      if (ticket === epoch.current) {
        setBusy(false)
        setChecking(false)
      }
    }
  }
  if (status?.canRetrieve) retained.current = true
  return (
    <>
      <section
        aria-label={pt ? "Acesso piloto" : "Pilot access"}
        className="border-b bg-slate-50 p-4 text-sm"
      >
        <p role="status" data-pilot-status>
          {checking
            ? pt
              ? "Verificando acesso…"
              : "Checking access…"
            : error
              ? pt
                ? "Não foi possível confirmar o acesso. Seu trabalho não salvo foi preservado. Entre novamente, se necessário, e verifique o status; a ativação não será reenviada automaticamente."
                : "Access could not be confirmed. Unsaved work is preserved. Sign in again if needed, then check status; activation will not be resubmitted automatically."
              : status
                ? labels[status.state]?.[pt ? 1 : 0]
                : ""}
        </p>
        {status?.expiresAt && (
          <p>
            {pt ? "Fim do acesso: " : "Access ends: "}
            <time dateTime={status.expiresAt}>
              {new Date(status.expiresAt).toLocaleString(pt ? "pt-BR" : "en")}
            </time>
          </p>
        )}
        {status?.retrieveUntil && (
          <p>
            {pt ? "Recuperação/exportação até: " : "Retrieve/export until: "}
            <time dateTime={status.retrieveUntil}>
              {new Date(status.retrieveUntil).toLocaleString(
                pt ? "pt-BR" : "en",
              )}
            </time>
          </p>
        )}
        {status?.canActivate && (
          <button
            data-pilot-activate
            disabled={busy || checking || error}
            onClick={() => void activate()}
          >
            {pt ? "Ativar meu acesso de 14 dias" : "Activate my 14-day access"}
          </button>
        )}
        <button data-pilot-refresh disabled={busy} onClick={() => void probe()}>
          {pt ? "Verificar status" : "Check status"}
        </button>
        {retained.current && !status?.canStandalone && (
          <p>
            {pt
              ? "Seu trabalho não salvo continua nesta aba. O fim do acesso não confirma exclusão."
              : "Unsaved work remains in this tab. Access expiry does not confirm deletion."}
          </p>
        )}
      </section>
      {retained.current && (
        <div hidden={checking || error || !status?.canRetrieve}>{children}</div>
      )}
    </>
  )
}

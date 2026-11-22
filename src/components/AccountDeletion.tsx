import { useEffect, useRef, useState } from "react"
import { AccountDeletion as DeletionClient, DeletionError, type DeletionKind, type DeletionStatus } from "../lib/accountDeletion"
import type { AccountProfile } from "../lib/accountProfile"

export const deletionEvent = "careeros-deletion-fence"
export const deletionSignalKey = "careeros.deletion.signal.v1"
function tabStorage() { try { return sessionStorage } catch { return undefined } }

// This control is outside the career workspace so an ambiguous confirmation
// can withhold every protected copy while retaining metadata/status controls.
export default function AccountDeletion({ accountId, account, pt, withhold }: {
  accountId: string, account?: AccountProfile, pt: boolean, withhold?: () => void,
}) {
  const [client] = useState(() => new DeletionClient(accountId,tabStorage()))
  const [prepared,setPrepared] = useState<{kind: DeletionKind,profileId:string,revision:number}|null>(null)
  const [status,setStatus] = useState<DeletionStatus|null>(null)
  const [message,setMessage] = useState("")
  const [busy,setBusy] = useState(false)
  const live=useRef(true)
  useEffect(()=>{ live.current=true;if(client.pendingOperation)void recover(false);return()=>{live.current=false;client.cancel()} },[client])
  function error(e: unknown) {
    setMessage(e instanceof DeletionError && e.code === "authentication"
      ? pt ? "Entre novamente para verificar o status. O acesso ao conteúdo permanece bloqueado." : "Sign in again to check status. Content access stays blocked."
      : pt ? "Não foi possível confirmar o status. Verifique antes de tentar outra operação." : "Status could not be confirmed. Check it before starting another operation.")
  }
  function committed(v: DeletionStatus) {
    const detail={accountId,operationId:v.operationId}
    window.dispatchEvent(new CustomEvent(deletionEvent,{detail}))
    try { localStorage.setItem(deletionSignalKey,JSON.stringify(detail)) } catch { /* Server fences remain authoritative. */ }
    window.location.reload()
  }
  async function recover(reload: boolean) {
    setBusy(true);setMessage("")
    try { const v=await client.recover();if(!live.current)return;setStatus(v);if(reload)committed(v) }
    catch(e){if(live.current)error(e)}finally{if(live.current)setBusy(false)}
  }
  async function prepare(kind: DeletionKind) {
    if(!account || (client.pendingOperation && !status&&!client.canCorrect))return
    setBusy(true);setMessage("")
    try {
      const doc=kind==="profile"?await account.open():null
      if(!live.current)return
      if(kind==="profile"&&!account.hasSavedProfile){setMessage(pt?"Não há Perfil salvo para excluir.":"There is no saved Profile to delete.");return}
      if(!client.canCorrect)client.newOperation();setPrepared({kind,profileId:doc?.id??"",revision:doc?.revision??0})
    }catch(e){if(live.current)error(e)}finally{if(live.current)setBusy(false)}
  }
  async function confirm() {
    if(!prepared)return
    setBusy(true);setMessage("");withhold?.()
    try{const v=await client.confirm(prepared);if(!live.current)return;setStatus(v);setPrepared(null);committed(v)}
    catch(e){if(live.current){setPrepared(null);error(e)}}finally{if(live.current)setBusy(false)}
  }
  return <section data-account-deletion className="border-b p-4 text-sm">
    <h2>{pt?"Excluir dados da conta":"Delete account data"}</h2>
    {account&&<div className="flex gap-4"><button data-delete-profile disabled={busy||Boolean(client.pendingOperation&&!status&&!client.canCorrect)} onClick={()=>void prepare("profile")}>{pt?"Excluir Perfil":"Delete Profile"}</button><button data-close-account disabled={busy||Boolean(client.pendingOperation&&!status&&!client.canCorrect)} onClick={()=>void prepare("account")}>{pt?"Encerrar conta":"Close account"}</button></div>}
    {prepared&&<div role="group" aria-label={pt?"Confirmar exclusão":"Confirm deletion"}>
      <p>{prepared.kind==="profile" ? pt?"Excluir o Perfil salvo e todo o histórico de Candidaturas vinculado a ele? Sua conta e os direitos ainda disponíveis continuam com os prazos originais. Esta ação não devolve usos consumidos.":"Delete the saved Profile and all Draft history linked to it? Your account and unused rights keep their original deadlines. This does not refund consumed uses."
        :pt?"Encerrar a conta, revogar seu acesso e excluir todo o conteúdo profissional? Entrar novamente permitirá apenas verificar a exclusão, sem restaurar conteúdo ou direitos.":"Close the account, revoke access and delete all career content? Signing in again will only let you check deletion, without restoring content or rights."}</p>
      <p>{pt?"A exclusão começa após esta confirmação. Exportar é opcional e não atrasa a exclusão. Cópias de recuperação e de destinatários são verificadas separadamente.":"Deletion starts after this confirmation. Export is optional and does not delay deletion. Recovery and recipient copies are checked separately."}</p>
      <button data-confirm-deletion disabled={busy} onClick={()=>void confirm()}>{pt?"Confirmar exclusão":"Confirm deletion"}</button>
      <button disabled={busy} onClick={()=>setPrepared(null)}>{pt?"Voltar":"Go back"}</button>
    </div>}
    <button data-deletion-status disabled={busy} onClick={()=>void recover(Boolean(client.pendingOperation&&!status))}>{pt?"Verificar status da exclusão":"Check deletion status"}</button>
    {busy&&<p role="status">{pt?"Verificando…":"Checking…"}</p>}
    {status&&<div data-deletion-result role="status"><p>{status.state==="active_content_removed"?pt?"Conteúdo ativo removido.":"Active content removed.":status.state==="failed"?pt?"A remoção ativa falhou e precisa ser concluída. O acesso permanece bloqueado.":"Active removal failed and needs completion. Access stays blocked.":pt?"Exclusão iniciada; remoção ativa pendente. O acesso ao conteúdo afetado está bloqueado.":"Deletion started; active removal pending. Access to affected content is blocked."}</p><p>{pt?"Cópias de recuperação e de destinatários: remoção não verificada. O prazo original para cópias de recuperação não é reiniciado.":"Recovery and recipient copies: removal not verified. The original recovery-copy deadline does not restart."}</p></div>}
    {message&&<p role="alert">{message} <a href="/api/auth/google/start">{pt?"Entrar novamente":"Sign in again"}</a></p>}
  </section>
}

import {useEffect,useRef,useState} from "react"
import {LifecycleError,readLifecycle,type LifecycleStatus} from "../lib/accountLifecycle"

export default function AccountLifecycle({accountId,pt,denied,readDenied,expired}:Readonly<{accountId:string;pt:boolean;denied:()=>void;readDenied:()=>void;expired:()=>void}>){
  const [status,setStatus]=useState<LifecycleStatus|null>(null),[error,setError]=useState(false),[refresh,setRefresh]=useState(0)
  const notified=useRef(false)
  useEffect(()=>{
    const controller=new AbortController();let active=true,timer:ReturnType<typeof setTimeout>|undefined
    void readLifecycle(accountId,controller.signal).then(v=>{
      if(!active)return
      if(!v.careerReadAllowed)readDenied()
      setStatus(v);setError(false)
      if(v.contentExpired&&!notified.current){notified.current=true;expired()}
      if(v.nextCheckAt)timer=setTimeout(()=>setRefresh(n=>n+1),Math.min(2_147_483_647,Math.max(1,Date.parse(v.nextCheckAt)-Date.now()+20)))
    }).catch(e=>{if(!active)return;setStatus(null);setError(true);if(e instanceof LifecycleError&&(e.status===401||e.status===403))denied()})
    return()=>{active=false;controller.abort();clearTimeout(timer)}
  },[accountId,refresh])
  if(error)return <aside data-account-lifecycle className="border-b p-3 text-sm" role="alert"><p>{pt?"Status de expiração indisponível.":"Expiry status unavailable."}</p><button data-lifecycle-refresh onClick={()=>setRefresh(n=>n+1)}>{pt?"Tentar novamente":"Try again"}</button></aside>
  if(!status||!status.notices.length&&!status.cleanup.length&&!status.residuals.length&&!status.removalPending&&!status.invitationCleanup.length)return null
  const when=(s:string)=>new Date(s).toLocaleString(pt?"pt-BR":"en")
  return <aside data-account-lifecycle className="border-b p-3 text-sm" aria-label={pt?"Expiração e remoção":"Expiry and removal"}>
    {status.notices.map(n=><div key={n.id} data-lifecycle-notice>
      <p>{n.kind==="access_ended"?(pt?"O acesso ao piloto terminou. Nenhuma nova geração de IA está incluída.":"Pilot access ended. New AI generation is no longer included."):(pt?"O conteúdo retido será removido do armazenamento ativo em breve.":"Retained content will be removed from active storage soon.")}</p>
      <p>{pt?"Remoção prevista: ":"Removal due: "}<time dateTime={n.removalAt}>{when(n.removalAt)}</time></p>
      {n.emailState!=="sent"&&<p data-lifecycle-email>{n.operatorSignal?(pt?"A entrega do e-mail falhou ou não foi confirmada. O aviso continua aqui e o operador foi sinalizado. O prazo não muda.":"Email delivery failed or is unconfirmed. This notice remains here and an operator signal is recorded. The deadline is unchanged."):(pt?"Aviso por e-mail pendente. O prazo não muda.":"Email notice pending. The deadline is unchanged.")}</p>}
    </div>)}
    {status.cleanup.map(c=><p key={c.operationId} data-lifecycle-cleanup>{c.state==="active_content_removed"?(pt?"Conteúdo ativo removido.":"Active content removed."):c.state==="failed"?(pt?"A remoção do conteúdo ativo falhou; o acesso continua bloqueado.":"Active-content removal failed; access remains blocked."):(pt?"Remoção do conteúdo ativo pendente; o acesso está bloqueado.":"Active-content removal pending; access is blocked.")}</p>)}
    {status.removalPending&&<p data-lifecycle-removal-pending>{pt?"A remoção do conteúdo ativo está pendente. O prazo de acesso terminou.":"Active-content removal is pending. The access window has ended."}</p>}
    {status.invitationCleanup.map(i=><p key={i.invitationId} data-lifecycle-invitation>{i.state==="active_payload_removed"?(pt?"Dados exclusivos do convite não ativado foram removidos. Isso não inicia um período de acesso ou recuperação de conteúdo.":"Unused invitation-only data was removed. This starts no access or career-content retrieval window."):(pt?"A remoção dos dados exclusivos do convite está pendente. O convite não permite ativação.":"Invitation-only data removal is pending. The invitation cannot activate access.")}</p>)}
    {status.residuals.length>0&&<p data-lifecycle-residual>{pt?"Cópias de recuperação e de destinatários têm status separado. Sua remoção não foi verificada; o prazo não comprova exclusão.":"Recovery and recipient copies have separate status. Removal is not verified; a deadline does not prove disposal."}</p>}
    <button data-lifecycle-refresh onClick={()=>setRefresh(n=>n+1)}>{pt?"Atualizar status":"Refresh status"}</button>
  </aside>
}

import { useEffect, useMemo, useRef, useState } from "react"
import { DraftRecoveryClient, type DraftClaim, type RecoveredDraft } from "../lib/draftRecovery"
import { useStore } from "../lib/store"

const messages = {
  en: {
    title: "Application Draft", intro: "Create a complete Draft and reopen its original version after an interruption.", job: "Job Posting", language: "Draft language", english: "English", portuguese: "Portuguese", question: "Employer question (optional)", answer: "Your application-only answer", context: "This answer is used only for this application. It is not saved into Profile.", generate: "Generate Draft", another: "Generate another Draft", status: "Check this Draft", waiting: "Checking…", recover: "Recovering the original Draft…", pending: "Work may still be running, or its outcome is unknown. Check this same Action; it will not be generated again automatically.", incomplete: "This Action has no complete reviewable bundle. Its status is preserved; checking it does not generate again.", closed: "This Action is closed. Its content is unavailable.", failed: "The response could not be confirmed. Keep this Action and check its status before starting another generation.", auth: "Sign in again to recover this same Draft. Your pending work is preserved in this view.", input: "Use the saved Profile and a Job Posting before generating.", storage: "Browser recovery storage is unavailable. No generation was sent.", version: "Original version", source: "Profile revision", historical: "The Profile has changed. This is the exact original snapshot, not a newly checked Draft.", ready: "Complete bundle available for review. Opening here does not record delivery, Accept or allowance consumption.", summary: "Role summary", resume: "Resume", cover: "Cover letter", answers: "Employer answers", none: "No employer answers were requested.", support: "Frozen support review", career: "Profile source", temporary: "Application-only source", posting: "Original Job Posting", supported: "Supported", previous: "Previous complete Draft", noSave: "Generation and recovery do not save Profile changes or accept these artifacts.", originalContext: "Original application-only context",
  },
  "pt-BR": {
    title: "Rascunho de candidatura", intro: "Crie um rascunho completo e reabra a versão original após uma interrupção.", job: "Anúncio da vaga", language: "Idioma do rascunho", english: "Inglês", portuguese: "Português", question: "Pergunta da empresa (opcional)", answer: "Sua resposta somente para esta candidatura", context: "Esta resposta é usada somente nesta candidatura. Ela não é salva no Perfil.", generate: "Gerar rascunho", another: "Gerar outro rascunho", status: "Verificar este rascunho", waiting: "Verificando…", recover: "Recuperando o rascunho original…", pending: "O trabalho pode continuar em execução ou seu resultado ser desconhecido. Verifique a mesma Ação; ela não será gerada novamente de forma automática.", incomplete: "Esta Ação não tem um conjunto completo para revisão. Seu status foi mantido; verificá-la não gera novamente.", closed: "Esta Ação foi encerrada. Seu conteúdo está indisponível.", failed: "Não foi possível confirmar a resposta. Mantenha esta Ação e verifique seu status antes de gerar novamente.", auth: "Entre novamente para recuperar o mesmo rascunho. O trabalho pendente foi mantido nesta tela.", input: "Use o Perfil salvo e um anúncio de vaga antes de gerar.", storage: "O armazenamento de recuperação do navegador está indisponível. Nenhuma geração foi enviada.", version: "Versão original", source: "Revisão do Perfil", historical: "O Perfil mudou. Este é o retrato original exato, não um rascunho reavaliado.", ready: "Conjunto completo disponível para revisão. A abertura aqui não registra entrega, aceite ou consumo de créditos.", summary: "Resumo da vaga", resume: "Currículo", cover: "Carta de apresentação", answers: "Respostas à empresa", none: "A empresa não solicitou respostas.", support: "Revisão de suporte preservada", career: "Fonte do Perfil", temporary: "Fonte somente desta candidatura", posting: "Anúncio original da vaga", supported: "Com suporte", previous: "Rascunho completo anterior", noSave: "Gerar e recuperar não salvam alterações no Perfil nem aceitam estes materiais.", originalContext: "Contexto original somente desta candidatura",
  },
}
type Labels = typeof messages.en

function SourceReview({ result, labels }: { result: RecoveredDraft; labels: Labels }) {
  const b = result.bundle!
  const fact = (reference: DraftClaim["sources"][number]) => b.support.facts.find(f => f.reference.kind === reference.kind && f.reference.id === reference.id && f.reference.revision === reference.revision)
  const wording = (snapshot: unknown) => {
    if (!snapshot || typeof snapshot !== "object") return ""
    const record = snapshot as Record<string, unknown>
    const value = record.value ?? record.userContext
    return typeof value === "string" ? value : value === undefined ? "" : JSON.stringify(value)
  }
  return <details className="border rounded p-4" data-draft-support>
    <summary className="cursor-pointer font-medium">{labels.support}</summary>
    <ul className="mt-3 space-y-4">{[...b.support.resume, ...b.support.artifacts].map((claim, i) => <li key={i} className="border-t pt-3">
      <p className="whitespace-pre-wrap break-words">{claim.text}</p><p className="text-sm mt-1">{labels.supported}</p>
      {claim.sources.map((reference, j) => <blockquote key={j} className="border-l pl-3 mt-2 text-sm whitespace-pre-wrap break-words"><span className="font-medium">{reference.kind === "profile" ? labels.career : labels.temporary}: </span>{wording(fact(reference)?.snapshot)}</blockquote>)}
      {claim.jobSources.map((source, j) => <blockquote key={j} className="border-l pl-3 mt-2 text-sm whitespace-pre-wrap break-words"><span className="font-medium">{labels.posting}: </span>{source.quote}</blockquote>)}
    </li>)}</ul>
  </details>
}

function BundleView({ result, labels }: { result: RecoveredDraft; labels: Labels }) {
  const b = result.bundle!
  return <article className="space-y-5" data-draft-result={result.action.id} data-draft-version={result.artifactVersion}>
    <header className="border rounded p-4 space-y-2"><p role="status">{labels.ready}</p><p className="text-sm break-words">{labels.version} {result.artifactVersion} · {labels.source} {b.binding.profileSource.revision} · {b.binding.language} · {result.action.id}</p>{result.historical && <p role="status">{labels.historical}</p>}</header>
    {[{ label: labels.summary, content: b.roleSummary, id: "summary" }, { label: labels.resume, content: [b.identity.fullName, b.identity.email, b.identity.phone, b.identity.location, b.identity.professionalLinks, b.resume].filter(Boolean).join("\n\n"), id: "resume" }, { label: labels.cover, content: b.coverLetter, id: "cover" }, { label: labels.answers, content: b.employerAnswers.state === "none_requested" ? labels.none : b.employerAnswers.text, id: "answers" }].map(artifact => <section className="border rounded p-5" key={artifact.id} data-draft-artifact={artifact.id}><h2 className="text-xl font-semibold mb-3">{artifact.label}</h2><pre className="whitespace-pre-wrap break-words font-sans text-sm leading-relaxed">{artifact.content}</pre></section>)}
    <SourceReview result={result} labels={labels} />
    <details className="border rounded p-4"><summary>{labels.posting}</summary><p className="mt-3 whitespace-pre-wrap break-words">{result.job?.posting}</p></details>
    {!!result.context && !!(result.context.answers.length || result.context.confirmations.length) && <details className="border rounded p-4"><summary>{labels.originalContext}</summary>{[...result.context.answers, ...result.context.confirmations].map((a, i) => <div className="mt-3" key={i}><p className="font-medium">{a.requirement}</p><p className="whitespace-pre-wrap break-words">{a.userContext}</p></div>)}</details>}
  </article>
}

export default function DraftRecoveryWorkspace() {
  const { accountId, profileDocument, profileSaved, state } = useStore()
  const labels = messages[state.uiLocale]
  const client = useMemo(() => accountId ? new DraftRecoveryClient(accountId) : null, [accountId])
  const [job, setJob] = useState("")
  const [language, setLanguage] = useState<"en" | "pt-BR">(state.cvLanguage)
  const [question, setQuestion] = useState("")
  const [answer, setAnswer] = useState("")
  const [result, setResult] = useState<RecoveredDraft | null>(null)
  const [previous, setPrevious] = useState<RecoveredDraft | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const edited = useRef(false)
  const controller = useRef<AbortController | null>(null)
  const owner = useRef(accountId)
  owner.current = accountId

  async function run(create: boolean) {
    if (!client || !accountId) return
    const captured = accountId
    controller.current?.abort();const request = new AbortController();controller.current = request
    setBusy(true);setError("")
    if (create && result?.bundle) { setPrevious(result);setResult(null) }
    try {
      if (create && (!profileDocument || !profileSaved || state.profileError)) throw new Error("draft_input")
      const next = create ? await client.create({ profileSource: { id: profileDocument!.id, revision: profileDocument!.revision }, job: { posting: job }, language, answers: question.trim() && answer.trim() ? [{ requirement: question, userContext: answer }] : [], confirmations: [], selectedFactIds: [] }, request.signal) : await client.recover(request.signal)
      if (owner.current !== captured || request.signal.aborted) return
      setResult(next)
      if (!edited.current && next.job) setJob(next.job.posting)
    } catch (e) {
      if (owner.current !== captured || request.signal.aborted || (e instanceof Error && e.message === "draft_superseded")) return
      const code = e instanceof Error ? e.message : "draft_outcome_unknown"
      setError(code === "draft_authentication_required" ? labels.auth : code === "draft_input" ? labels.input : code === "draft_recovery_storage" ? labels.storage : labels.failed)
    } finally { if (owner.current === captured && controller.current === request) setBusy(false) }
  }
  useEffect(() => {
    setResult(null);setPrevious(null);setError("")
    if (client?.hasKey()) void run(false)
    return () => { controller.current?.abort();client?.cancel() }
  }, [client])

  const blocked = !client || busy || !profileSaved || !!state.profileError || !profileDocument || !job.trim() || client.state === "uncertain" || client.state === "pending"
  return <main className="max-w-6xl mx-auto px-6 py-10 space-y-6" data-draft-workspace>
    <header><h1 className="text-3xl font-bold">{labels.title}</h1><p className="mt-2 text-sm">{labels.intro}</p></header>
    <section className="border rounded p-5 space-y-4">
      <label className="block text-sm">{labels.job}<textarea data-draft-job className="block border rounded w-full mt-2 p-3 min-h-32" value={job} disabled={busy} onChange={e => { edited.current = true;setJob(e.target.value) }} /></label>
      <label className="block text-sm">{labels.language}<select data-draft-language className="block border rounded p-2 mt-2" disabled={busy} value={language} onChange={e => setLanguage(e.target.value as "en" | "pt-BR")}><option value="en">{labels.english}</option><option value="pt-BR">{labels.portuguese}</option></select></label>
      <details><summary>{labels.question}</summary><input aria-label={labels.question} className="block border rounded p-2 w-full mt-3" disabled={busy} value={question} onChange={e => setQuestion(e.target.value)} /><label className="block mt-3 text-sm">{labels.answer}<textarea className="block border rounded p-2 w-full mt-2" disabled={busy} value={answer} onChange={e => setAnswer(e.target.value)} /></label><p className="text-sm mt-2">{labels.context}</p></details>
      <div className="flex flex-wrap gap-3"><button data-draft-generate className="border rounded px-4 py-2 disabled:opacity-50" disabled={blocked} onClick={() => void run(true)}>{client?.hasKey() ? labels.another : labels.generate}</button>{client?.hasKey() && <button data-draft-recover className="border rounded px-4 py-2" disabled={busy} onClick={() => void run(false)}>{busy ? labels.waiting : labels.status}</button>}</div>
      <p className="text-sm">{labels.noSave}</p>
    </section>
    {busy && <p role="status">{labels.recover}</p>}
    {error && <p role="alert" className="border rounded p-4">{error}</p>}
    {result?.state === "pending" && <p role="status">{labels.pending}</p>}
    {result?.state === "incomplete" && <p role="status">{labels.incomplete}</p>}
    {(result?.state === "closed" || result?.state === "deleted") && <p role="status">{labels.closed}</p>}
    {result?.bundle && <BundleView result={result} labels={labels} />}
    {previous?.bundle && <details className="border rounded p-4"><summary>{labels.previous}</summary><div className="mt-4"><BundleView result={previous} labels={labels} /></div></details>}
  </main>
}

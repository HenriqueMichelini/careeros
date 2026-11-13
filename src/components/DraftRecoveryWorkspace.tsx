import { useEffect, useMemo, useRef, useState } from "react"
import { DraftRecoveryClient, type DraftClaim, type DraftInput, type RecoveredDraft } from "../lib/draftRecovery"
import { findProfileGaps, QualificationGapsError, type QualificationAnalysis } from "../lib/ai"
import { ManagedQualification } from "../lib/managedQualification"
import { useStore } from "../lib/store"
import JobContextReview from "./JobContextReview"
import RequirementEvidenceReview from "./RequirementEvidenceReview"

const messages = {
  en: {
    title: "Application Draft", intro: "Create a complete Draft and reopen its original version after an interruption.", job: "Job Posting", language: "Draft language", english: "English", portuguese: "Portuguese", question: "Employer question (optional)", answer: "Your application-only answer", context: "This answer is used only for this application. It is not saved into Profile.", generate: "Generate Draft", another: "Generate another Draft", status: "Check this Draft", waiting: "Checking…", recover: "Recovering the original Draft…", pending: "Work may still be running, or its outcome is unknown. Check this same Action; it will not be generated again automatically.", incomplete: "This Action has no complete reviewable bundle. Its status is preserved; checking it does not generate again.", closed: "This Action is closed. Its content is unavailable.", failed: "The response could not be confirmed. Keep this Action and check its status before starting another generation.", auth: "Sign in again to recover this same Draft. Your pending work is preserved in this tab.", denied: "Draft access is unavailable. Check again after access is restored.", accountChanged: "Your account changed. Reopen the workspace with the current account.", absent: "No Draft was found for this request. You can submit it again.", input: "Use the saved Profile and a Job Posting before generating.", storage: "Browser recovery storage is unavailable. No generation was sent.", version: "Original version", source: "Profile revision", historical: "The Profile has changed. This is the exact original snapshot, not a newly checked Draft.", ready: "Complete bundle available for review. Opening here does not record delivery, Accept or allowance consumption.", summary: "Role summary", resume: "Resume", cover: "Cover letter", answers: "Employer answers", none: "No employer answers were requested.", support: "Frozen support review", career: "Profile source", temporary: "Application-only source", posting: "Original Job Posting", supported: "Supported", previous: "Previous complete Draft", noSave: "Generation and recovery do not save Profile changes or accept these artifacts.", originalContext: "Original application-only context",
  },
  "pt-BR": {
    title: "Rascunho de candidatura", intro: "Crie um rascunho completo e reabra a versão original após uma interrupção.", job: "Anúncio da vaga", language: "Idioma do rascunho", english: "Inglês", portuguese: "Português", question: "Pergunta da empresa (opcional)", answer: "Sua resposta somente para esta candidatura", context: "Esta resposta é usada somente nesta candidatura. Ela não é salva no Perfil.", generate: "Gerar rascunho", another: "Gerar outro rascunho", status: "Verificar este rascunho", waiting: "Verificando…", recover: "Recuperando o rascunho original…", pending: "O trabalho pode continuar em execução ou seu resultado ser desconhecido. Verifique a mesma Ação; ela não será gerada novamente de forma automática.", incomplete: "Esta Ação não tem um conjunto completo para revisão. Seu status foi mantido; verificá-la não gera novamente.", closed: "Esta Ação foi encerrada. Seu conteúdo está indisponível.", failed: "Não foi possível confirmar a resposta. Mantenha esta Ação e verifique seu status antes de gerar novamente.", auth: "Entre novamente para recuperar o mesmo rascunho. O trabalho pendente foi mantido nesta aba.", denied: "O acesso ao rascunho está indisponível. Verifique novamente quando o acesso for restabelecido.", accountChanged: "Sua conta mudou. Reabra o espaço de trabalho com a conta atual.", absent: "Nenhum rascunho foi encontrado para esta solicitação. Você pode enviá-la novamente.", input: "Use o Perfil salvo e um anúncio de vaga antes de gerar.", storage: "O armazenamento de recuperação do navegador está indisponível. Nenhuma geração foi enviada.", version: "Versão original", source: "Revisão do Perfil", historical: "O Perfil mudou. Este é o retrato original exato, não um rascunho reavaliado.", ready: "Conjunto completo disponível para revisão. A abertura aqui não registra entrega, aceite ou consumo de créditos.", summary: "Resumo da vaga", resume: "Currículo", cover: "Carta de apresentação", answers: "Respostas à empresa", none: "A empresa não solicitou respostas.", support: "Revisão de suporte preservada", career: "Fonte do Perfil", temporary: "Fonte somente desta candidatura", posting: "Anúncio original da vaga", supported: "Com suporte", previous: "Rascunho completo anterior", noSave: "Gerar e recuperar não salvam alterações no Perfil nem aceitam estes materiais.", originalContext: "Contexto original somente desta candidatura",
  },
}
type Labels = typeof messages.en

const managedMessages = {
  en: {
    analyze: "Analyze Job Posting",
    analyzing: "Analyzing the Job Posting…",
    analysisFailed: "The analysis could not be confirmed. Check its status or use the original Job Posting.",
    analysisStatus: "Check analysis status",
    analysisUnknown: "The analysis outcome is unavailable. You can use the original Job Posting for a Draft.",
    retryAnalysis: "Retry analysis",
    retryDraft: "Retry this Draft",
    capacity: "This Draft exceeded an available limit. Check the Action before starting new work.",
    incomplete: "A required check could not establish a complete Draft. Check the Action for its status.",
    jobRejected: "The Job Posting needs revision before it can be used.",
    timeout: "The Draft timed out. Check this Action for its current status.",
  },
  "pt-BR": {
    analyze: "Analisar anúncio da vaga",
    analyzing: "Analisando o anúncio da vaga…",
    analysisFailed: "Não foi possível confirmar a análise. Verifique o status ou use o anúncio original.",
    analysisStatus: "Verificar status da análise",
    analysisUnknown: "O resultado da análise está indisponível. Você pode usar o anúncio original para gerar um rascunho.",
    retryAnalysis: "Tentar análise novamente",
    retryDraft: "Tentar este rascunho novamente",
    capacity: "Este rascunho excedeu um limite disponível. Verifique a Ação antes de iniciar outro trabalho.",
    incomplete: "Uma verificação obrigatória não confirmou um rascunho completo. Consulte o status da Ação.",
    jobRejected: "Revise o anúncio da vaga antes de usá-lo.",
    timeout: "O tempo limite do rascunho foi atingido. Consulte o status desta Ação.",
  },
}

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

export interface DraftEditor {
  job: string
  language: "en" | "pt-BR"
  question: string
  answer: string
}

type DraftEditors = Map<string, DraftEditor>

export default function DraftRecoveryWorkspace({ editors }: { editors: DraftEditors }) {
  const { accountId } = useStore()
  return accountId ? <AccountDraftRecoveryWorkspace key={accountId} accountId={accountId} editors={editors} /> : null
}

function AccountDraftRecoveryWorkspace({ accountId, editors }: { accountId: string; editors: DraftEditors }) {
  const { profileDocument, profileSaved, profileSaving, state } = useStore()
  const labels = messages[state.uiLocale]
  const managedLabels = managedMessages[state.uiLocale]
  const managed = import.meta.env.VITE_CAREEROS_PILOT_MODE === "1"
  const client = useMemo(() => new DraftRecoveryClient(accountId), [accountId])
  const qualification = useMemo(() => new ManagedQualification(accountId), [accountId])
  const emptyEditor = (): DraftEditor => ({ job: "", language: state.cvLanguage, question: "", answer: "" })
  const [editor, setEditor] = useState(() => editors.get(accountId) ?? emptyEditor())
  const { job, language, question, answer } = editor
  const [result, setResult] = useState<RecoveredDraft | null>(null)
  const [previous, setPrevious] = useState<RecoveredDraft | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const controller = useRef<AbortController | null>(null)
  const [analysis, setAnalysis] = useState<{ binding: string; result: QualificationAnalysis } | null>(null)
  const [analyzing, setAnalyzing] = useState(false)
  const [analysisAttempted, setAnalysisAttempted] = useState(false)
  const analysisBinding = JSON.stringify([profileDocument?.id, profileDocument?.revision, job])
  const currentAnalysisBinding = useRef(analysisBinding)
  currentAnalysisBinding.current = analysisBinding
  const currentAnalysis = profileSaved && !profileSaving && !state.profileError && analysis?.binding === analysisBinding ? analysis.result : null
  const inputBinding = JSON.stringify([analysisBinding, language, question, answer, currentAnalysis?.draftContextProof])
  const submitted = useRef<{ binding: string; input: DraftInput } | null>(null)

  function hideProtectedWork() {
    setResult(null)
    setPrevious(null)
    setAnalysis(null)
    submitted.current = null
  }

  function updateEditor(change: Partial<DraftEditor>) {
    if (change.job !== undefined && change.job !== job) {
      setAnalysis(null)
      setAnalysisAttempted(false)
      qualification.reset()
    }
    const next = { ...editor, ...change }
    editors.set(accountId, next)
    setEditor(next)
  }

  async function run(command: "create" | "recover" | "retry") {
    controller.current?.abort()
    const request = new AbortController()
    controller.current = request
    setBusy(true)
    setError("")
    if (command === "create" && result?.bundle) {
      setPrevious(result)
      setResult(null)
    }
    try {
      let next: RecoveredDraft
      if (command === "recover") {
        next = await client.recover(request.signal)
      } else if (command === "retry") {
        if (!submitted.current || submitted.current.binding !== inputBinding) throw new Error("draft_input")
        next = await client.retry(submitted.current.input, request.signal)
      } else {
        if (!profileDocument || !profileSaved || profileSaving || state.profileError) throw new Error("draft_input")
        const input: DraftInput = {
          profileSource: { id: profileDocument.id, revision: profileDocument.revision },
          job: { posting: job },
          language,
          answers: question.trim() && answer.trim() ? [{ requirement: question, userContext: answer }] : [],
          confirmations: [],
          selectedFactIds: [],
          ...(currentAnalysis?.draftContextProof
            ? { jobContext: currentAnalysis.jobContext, qualificationProof: currentAnalysis.draftContextProof }
            : {}),
        }
        submitted.current = { binding: inputBinding, input }
        next = await client.create(input, request.signal)
      }
      if (!request.signal.aborted) {
        if (next.state === "closed" || next.state === "deleted") hideProtectedWork()
        setResult(next)
      }
    } catch (e) {
      if (request.signal.aborted || (e instanceof Error && e.message === "draft_superseded")) return
      const code = e instanceof Error ? e.message : "draft_outcome_unknown"
      if (code === "draft_authentication_required" || code === "draft_access_denied" || code === "draft_account_changed") {
        hideProtectedWork()
      }
      if (code === "draft_account_changed") setEditor(emptyEditor())
      const message: Record<string, string> = {
        draft_authentication_required: labels.auth,
        draft_access_denied: labels.denied,
        draft_account_changed: labels.accountChanged,
        draft_not_found: labels.absent,
        draft_input: labels.input,
        draft_recovery_storage: labels.storage,
        draft_capacity: managedLabels.capacity,
        draft_incomplete: managedLabels.incomplete,
        draft_job_rejected: managedLabels.jobRejected,
        draft_timeout: managedLabels.timeout,
      }
      setError(message[code] ?? labels.failed)
    } finally {
      if (controller.current === request && !request.signal.aborted) setBusy(false)
    }
  }

  async function analyze(checkStatus = false) {
    if (!profileDocument || !profileSaved || profileSaving || !job.trim() || state.profileError) return
    controller.current?.abort()
    const request = new AbortController()
    controller.current = request
    setBusy(true)
    setAnalyzing(true)
    setError("")
    try {
      if (checkStatus) {
        const status = await qualification.status(request.signal)
        if (!request.signal.aborted && !status.retryAllowed) setError(managedLabels.analysisUnknown)
        return
      }
      setAnalysisAttempted(true)
      const next = await findProfileGaps(state.repository, job, "", "", true, profileDocument, request.signal, undefined, accountId, qualification.command(analysisBinding))
      if (!request.signal.aborted && currentAnalysisBinding.current === analysisBinding) {
        setAnalysis({ binding: analysisBinding, result: next })
      }
    } catch (e) {
      if (request.signal.aborted) return
      if (e instanceof QualificationGapsError) {
        qualification.knownFailure(e.retryAllowed)
        if (["authentication_required", "account_changed", "forbidden", "qualification_denied"].includes(e.code)) hideProtectedWork()
      }
      setError(managedLabels.analysisFailed)
    } finally {
      if (controller.current === request && !request.signal.aborted) {
        setBusy(false)
        setAnalyzing(false)
      }
    }
  }

  useEffect(() => {
    if (client.hasKey()) void run("recover")
    return () => {
      controller.current?.abort()
      client.cancel()
    }
  }, [client])

  const sourceBlocked = busy || !profileSaved || profileSaving || !!state.profileError || !profileDocument || !job.trim()
  const blocked = sourceBlocked || client.state === "uncertain" || client.state === "pending"
  const retryDraft = managed && result?.retryAllowed && submitted.current?.binding === inputBinding
  return <main className="max-w-6xl mx-auto px-6 py-10 space-y-6" data-draft-workspace>
    <header><h1 className="text-3xl font-bold">{labels.title}</h1><p className="mt-2 text-sm">{labels.intro}</p></header>
    <section className="border rounded p-5 space-y-4">
      <label className="block text-sm">{labels.job}<textarea data-draft-job className="block border rounded w-full mt-2 p-3 min-h-32" value={job} disabled={busy} onChange={e => updateEditor({ job: e.target.value })} /></label>
      <label className="block text-sm">{labels.language}<select data-draft-language className="block border rounded p-2 mt-2" disabled={busy} value={language} onChange={e => updateEditor({ language: e.target.value as DraftEditor["language"] })}><option value="en">{labels.english}</option><option value="pt-BR">{labels.portuguese}</option></select></label>
      <details><summary>{labels.question}</summary><input aria-label={labels.question} className="block border rounded p-2 w-full mt-3" disabled={busy} value={question} onChange={e => updateEditor({ question: e.target.value })} /><label className="block mt-3 text-sm">{labels.answer}<textarea className="block border rounded p-2 w-full mt-2" disabled={busy} value={answer} onChange={e => updateEditor({ answer: e.target.value })} /></label><p className="text-sm mt-2">{labels.context}</p></details>
      {managed && <div className="flex flex-wrap gap-3">
        <button data-draft-analyze className="border rounded px-4 py-2 disabled:opacity-50" disabled={sourceBlocked || !!currentAnalysis} onClick={() => void analyze()}>{qualification.retryAvailable() ? managedLabels.retryAnalysis : managedLabels.analyze}</button>
        {analysisAttempted && !currentAnalysis && <button className="border rounded px-4 py-2" disabled={busy} onClick={() => void analyze(true)}>{managedLabels.analysisStatus}</button>}
      </div>}
      {currentAnalysis && <div data-draft-analysis>
        <JobContextReview context={currentAnalysis.jobContext} />
        <RequirementEvidenceReview matches={currentAnalysis.matches ?? []} />
      </div>}
      <div className="flex flex-wrap gap-3"><button data-draft-generate className="border rounded px-4 py-2 disabled:opacity-50" disabled={blocked} onClick={() => void run("create")}>{client.hasKey() && client.state !== "absent" ? labels.another : labels.generate}</button>{retryDraft && <button data-draft-retry className="border rounded px-4 py-2" disabled={sourceBlocked} onClick={() => void run("retry")}>{managedLabels.retryDraft}</button>}{client.hasKey() && <button data-draft-recover className="border rounded px-4 py-2" disabled={busy} onClick={() => void run("recover")}>{busy ? labels.waiting : labels.status}</button>}</div>
      <p className="text-sm">{labels.noSave}</p>
    </section>
    {busy && <p role="status">{analyzing ? managedLabels.analyzing : labels.recover}</p>}
    {error && <p role="alert" className="border rounded p-4">{error}</p>}
    {result?.state === "pending" && <p role="status">{labels.pending}</p>}
    {result?.state === "incomplete" && <p role="status">{labels.incomplete}</p>}
    {(result?.state === "closed" || result?.state === "deleted") && <p role="status">{labels.closed}</p>}
    {result?.bundle && <BundleView result={result} labels={labels} />}
    {previous?.bundle && <details className="border rounded p-4"><summary>{labels.previous}</summary><div className="mt-4"><BundleView result={previous} labels={labels} /></div></details>}
  </main>
}

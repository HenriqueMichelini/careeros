import { useEffect, useRef, useState } from "react"
import type { DraftArtifact, RecoveredDraft } from "../lib/draftRecovery"
import { DraftReviewClient } from "../lib/draftReview"
import { emptyProfileView } from "../lib/profileDocument"
import GeneratedResumePreview from "./GeneratedResumePreview"

const labels = {
  en: { copy: "Copy accepted text", pdf: "Save accepted PDF", copied: "Accepted snapshot copied.", failed: "The export could not be completed. Try again or keep this Draft open.", overflow: "This accepted snapshot does not fit the supported A4 page. PDF was not opened; no content was clipped.", original: "Original version", source: "Profile revision", historical: "Historical snapshot — the Profile has changed. Original support has not been rechecked.", summary: "Role summary", resume: "Resume", cover: "Cover letter", answers: "Employer answers", none: "No employer answers were requested." },
  "pt-BR": { copy: "Copiar texto aceito", pdf: "Salvar PDF aceito", copied: "Retrato aceito copiado.", failed: "Não foi possível concluir a exportação. Tente novamente ou mantenha este rascunho aberto.", overflow: "Este retrato aceito não cabe na página A4 suportada. O PDF não foi aberto; nenhum conteúdo foi cortado.", original: "Versão original", source: "Revisão do Perfil", historical: "Retrato histórico — o Perfil mudou. O suporte original não foi reavaliado.", summary: "Resumo da vaga", resume: "Currículo", cover: "Carta de apresentação", answers: "Respostas à empresa", none: "A empresa não solicitou respostas." },
}

export default function AcceptedDraftExports({ result, client, historical, onFresh, onDenied }: {
  result: RecoveredDraft
  client: DraftReviewClient
  historical: boolean
  onFresh: (result: RecoveredDraft) => void
  onDenied: (code: string) => void
}) {
  const language = result.bundle!.binding.language
  const t = labels[language]
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState("")
  const preview = useRef<HTMLDivElement>(null)
  const sequence = useRef(0)
  const cleanupPrint = useRef<(() => void) | null>(null)
  const exportFocus = useRef<HTMLElement | null>(null)
  useEffect(() => () => { sequence.current++; cleanupPrint.current?.() }, [result.action.id, result.artifactVersion, result.bundleDigest])
  useEffect(() => { if (!busy && exportFocus.current?.isConnected) exportFocus.current.focus({ preventScroll: true }) }, [busy])

  const disclosure = (value: RecoveredDraft) => [
    `${t.original} ${value.artifactVersion} · ${t.source} ${value.bundle!.binding.profileSource.revision} · ${value.bundle!.binding.profileSource.id}`,
    `${value.bundle!.binding.language} · ${value.action.id}`,
    ...(historical || value.historical ? [t.historical] : []),
  ].join("\n")
  const wording = (value: RecoveredDraft, artifact: DraftArtifact) => {
    const b = value.bundle!
    return artifact === "summary" ? b.roleSummary : artifact === "resume" ? [b.identity.fullName, b.identity.email, b.identity.phone, b.identity.location, b.identity.professionalLinks, b.resume].filter(Boolean).join("\n\n") : artifact === "cover" ? b.coverLetter : b.employerAnswers.state === "none_requested" ? t.none : b.employerAnswers.text
  }

  async function exportSnapshot(artifact: DraftArtifact, kind: "copy" | "pdf") {
    const attempt = ++sequence.current
    const current = () => sequence.current === attempt
    const focus = document.activeElement
    exportFocus.current = focus instanceof HTMLElement ? focus : null
    setBusy(true); setStatus("")
    try {
      if (kind === "pdf") await document.fonts.ready
      const value = await client.exactAccepted(result)
      if (!current()) return
      onFresh(value)
      const text = `${disclosure(value)}\n\n${t[artifact]}\n\n${wording(value, artifact)}`
      if (kind === "copy") {
        let copied = false
        try { if (navigator.clipboard?.writeText && current()) { await navigator.clipboard.writeText(text); copied = true } } catch { /* Explicit fallback below. */ }
        if (!copied && current()) {
          const area = document.createElement("textarea"); area.value = text; area.readOnly = true; area.style.position = "fixed"; area.style.opacity = "0"
          try { document.body.append(area); area.select(); copied = document.execCommand("copy") === true } finally { area.remove() }
        }
        if (current()) setStatus(copied ? "copied" : "failed")
        return
      }

      let content: HTMLElement
      if (artifact === "resume") {
        const paper = preview.current?.querySelector<HTMLElement>(".cv-paper")
        if (!paper) throw new Error("draft_export_unavailable")
        content = paper.cloneNode(true) as HTMLElement
        const note = document.createElement("p"); note.textContent = disclosure(value); note.style.cssText = "font-size:9px;white-space:pre-wrap;overflow-wrap:anywhere;margin-bottom:10px"
        content.querySelector(".cv-paper-content")?.prepend(note)
      } else {
        content = document.createElement("div"); content.className = "results-cover-print"; content.textContent = text
      }
      const measure = content.cloneNode(true) as HTMLElement
      measure.classList.add("cv-export-measure"); measure.style.display = "block"; measure.style.zoom = "1"
      document.body.append(measure)
      const body = measure.querySelector<HTMLElement>(".cv-paper-content")
      const height = body ? body.getBoundingClientRect().bottom - measure.getBoundingClientRect().top + parseFloat(getComputedStyle(measure).paddingBottom || "0") : measure.getBoundingClientRect().height
      const overflow = height > 297 * 96 / 25.4 - 4 || measure.scrollWidth > measure.getBoundingClientRect().width + 2
      measure.remove()
      if (overflow) { setStatus("overflow"); return }
      if (!current()) return
      cleanupPrint.current?.()
      const root = document.createElement("div"); root.className = "results-print-root"; root.append(content); document.body.append(root)
      const printClass = artifact === "resume" ? "results-print-resume" : "results-print-cover"
      const title = document.title
      document.title = `${value.bundle!.identity.fullName || "CareerOS"} - ${t[artifact]} - ${value.action.id}`
      document.body.classList.add(printClass)
      const finish = () => { window.removeEventListener("afterprint", finish); root.remove(); document.body.classList.remove(printClass); document.title = title; if (cleanupPrint.current === finish) cleanupPrint.current = null; if (current() && focus instanceof HTMLElement) focus.focus({ preventScroll: true }) }
      cleanupPrint.current = finish; window.addEventListener("afterprint", finish)
      try { window.print() } catch (e) { finish(); throw e }
    } catch (e) {
      if (!current()) return
      const code = e instanceof Error ? e.message : ""
      if (["draft_authentication_required", "draft_access_denied", "draft_account_changed"].includes(code)) onDenied(code)
      else if (code !== "draft_superseded") setStatus("failed")
    } finally {
      if (current()) setBusy(false)
    }
  }

  return <div className="space-y-4" data-draft-exports>
    <p className="text-sm whitespace-pre-wrap break-words" data-draft-export-disclosure>{disclosure(result)}</p>
    <div className="space-y-3">{(["summary", "resume", "cover", "answers"] as DraftArtifact[]).map(artifact => <div className="flex flex-wrap items-center gap-3" key={artifact}><span>{t[artifact]}</span><button className="border rounded px-3 py-2 disabled:opacity-50" data-draft-copy={artifact} disabled={busy} onClick={() => void exportSnapshot(artifact, "copy")}>{t.copy}</button><button className="border rounded px-3 py-2 disabled:opacity-50" data-draft-pdf={artifact} disabled={busy} onClick={() => void exportSnapshot(artifact, "pdf")}>{t.pdf}</button></div>)}</div>
    {status && <p data-draft-export-status={status} role={status === "copied" ? "status" : "alert"}>{status === "copied" ? t.copied : status === "overflow" ? t.overflow : t.failed}</p>}
    <div ref={preview} data-draft-accepted-preview><GeneratedResumePreview resume={result.bundle!.resume} cvLanguage={language} profile={{ ...emptyProfileView(), ...result.bundle!.identity }} onEditProfile={() => {}} readOnlyIdentity /></div>
  </div>
}

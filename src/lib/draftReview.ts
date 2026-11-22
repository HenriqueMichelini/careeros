import { parseResult, type DraftAction, type DraftArtifact, type RecoveredDraft } from "./draftRecovery"
import { workflowHeaders } from "./workflowRequest"

type StoragePort = Pick<Storage, "getItem" | "setItem" | "removeItem">
type ReviewPointer = { creationKey: string; action: DraftAction; artifactVersion: number; bundleDigest: string }
const token = (v: unknown): v is string => typeof v === "string" && /^[A-Za-z0-9_-]{1,128}$/.test(v)
function pointer(value: unknown, owner: string): value is ReviewPointer {
  if (!value || typeof value !== "object") return false
  const p = value as ReviewPointer
  return token(p.creationKey) && p.artifactVersion === 1 && typeof p.bundleDigest === "string" && /^[a-f0-9]{64}$/.test(p.bundleDigest) && !!p.action && p.action.owner === owner && p.action.workflow === "application_draft" && token(p.action.namespace) && token(p.action.id)
}

// Review has no generation or browser-content-storage capability. Opaque command
// keys survive an uncertain reply in this instance; a reload recovers by GET.
export class DraftReviewClient {
  private sequence = 0
  private previousSequence = 0
  private readonly commands = new Map<string, string>()
  constructor(readonly accountId: string, private readonly transport: typeof fetch = fetch, private readonly storage: StoragePort | null = typeof sessionStorage === "undefined" ? null : sessionStorage) {}
  cancel() { this.sequence++; this.previousSequence++ }
  private get storageKey() { return `careeros_draft_review_previous:${encodeURIComponent(this.accountId)}` }

  // One bounded opaque pointer preserves the prior complete review when the
  // latest creation key advances. Career wording remains server-authorized GET
  // data, never browser storage. Persist before permitting a replacement send.
  remember(result: RecoveredDraft) {
    if (result.state !== "ready" || !result.review || !pointer(result, this.accountId) || !parseResult(result, this.accountId, result.creationKey)) throw new Error("draft_invalid_result")
    const selected: ReviewPointer = { creationKey: result.creationKey, action: { ...result.action }, artifactVersion: result.artifactVersion, bundleDigest: result.bundleDigest! }
    try { if (!this.storage) throw new Error(); this.storage.setItem(this.storageKey, JSON.stringify(selected)) } catch { throw new Error("draft_recovery_storage") }
    this.previousSequence++
  }

  async previous(signal?: AbortSignal): Promise<RecoveredDraft | null> {
    const sequence = ++this.previousSequence
    let selected: ReviewPointer
    try { const saved = JSON.parse(this.storage?.getItem(this.storageKey) ?? "null"); if (!pointer(saved, this.accountId)) return null; selected = saved } catch { return null }
    const response = await this.transport(`/api/drafts/${encodeURIComponent(selected.creationKey)}/versions/${selected.artifactVersion}`, { headers: workflowHeaders(this.accountId), signal: this.signal(signal) }).catch(() => { throw new Error("draft_outcome_unknown") })
    const payload = await response.json().catch(() => null)
    if (sequence !== this.previousSequence) throw new Error("draft_superseded")
    this.denied(response.status)
    if (response.status === 404 && payload?.error === "draft_not_found") { this.storage?.removeItem(this.storageKey); return null }
    if (!response.ok && response.status !== 410) throw new Error("draft_outcome_unknown")
    const result = parseResult(payload, this.accountId, selected.creationKey)
    if (!result || JSON.stringify(result.action) !== JSON.stringify(selected.action) || result.artifactVersion !== selected.artifactVersion) throw new Error("draft_invalid_result")
    if (result.state === "closed" || result.state === "deleted") { this.storage?.removeItem(this.storageKey); return null }
    if (result.state !== "ready" || !result.review || result.bundleDigest !== selected.bundleDigest) throw new Error("draft_invalid_result")
    return structuredClone(result)
  }

  async exactAccepted(selected: RecoveredDraft, signal?: AbortSignal): Promise<RecoveredDraft> {
    const sequence = ++this.sequence
    const response = await this.transport(`/api/drafts/${encodeURIComponent(selected.creationKey)}/versions/${selected.artifactVersion}`, { headers: workflowHeaders(this.accountId), signal: this.signal(signal) }).catch(() => { throw new Error("draft_outcome_unknown") })
    const payload = await response.json().catch(() => null)
    this.current(sequence); this.denied(response.status)
    if (response.status === 410) throw new Error("draft_access_denied")
    if (!response.ok) throw new Error("draft_outcome_unknown")
    const result = parseResult(payload, this.accountId, selected.creationKey)
    if (!result || result.state !== "ready" || !result.review?.accepted || result.bundleDigest !== selected.bundleDigest || result.artifactVersion !== selected.artifactVersion || result.action.id !== selected.action.id || result.action.namespace !== selected.action.namespace) throw new Error("draft_invalid_result")
    return structuredClone(result)
  }

  async accepted(signal?: AbortSignal): Promise<RecoveredDraft | null> {
    const sequence = ++this.sequence
    const response = await this.transport("/api/drafts/accepted", { headers: workflowHeaders(this.accountId), signal: this.signal(signal) }).catch(() => { throw new Error("draft_outcome_unknown") })
    const payload = await response.json().catch(() => null)
    this.current(sequence)
    this.denied(response.status)
    if (response.status === 404 && payload?.error === "draft_not_found") return null
    if (!response.ok && response.status !== 410) throw new Error("draft_outcome_unknown")
    const key = payload?.creationKey
    if (typeof key !== "string" || !/^[A-Za-z0-9_-]{1,128}$/.test(key)) throw new Error("draft_invalid_result")
    const result = parseResult(payload, this.accountId, key)
    if (!result) throw new Error("draft_invalid_result")
    return result.state === "ready" && result.review?.accepted ? structuredClone(result) : null
  }

  async decide(result: RecoveredDraft, artifact: DraftArtifact, operation: "review" | "accept", signal?: AbortSignal): Promise<RecoveredDraft> {
    if (result.action.owner !== this.accountId || result.state !== "ready" || !result.delivered || !result.review || !result.bundleDigest) throw new Error("draft_access_denied")
    const binding = JSON.stringify([result.action, result.artifactVersion, result.bundleDigest, result.review.revision, artifact, operation])
    let decisionKey = this.commands.get(binding)
    if (!decisionKey) { decisionKey = crypto.randomUUID(); this.commands.set(binding, decisionKey) }
    const sequence = ++this.sequence
    const response = await this.transport(`/api/drafts/${encodeURIComponent(result.creationKey)}/versions/${result.artifactVersion}/review`, {
      method: "POST", headers: { ...workflowHeaders(this.accountId), "Content-Type": "application/json" },
      body: JSON.stringify({ decisionKey, expectedRevision: result.review.revision, bundleDigest: result.bundleDigest, artifact, operation }), signal: this.signal(signal),
    }).catch(() => { throw new Error("draft_outcome_unknown") })
    const payload = await response.json().catch(() => null)
    this.current(sequence)
    this.denied(response.status)
    if (response.status === 409) throw new Error("draft_review_conflict")
    if (!response.ok) throw new Error("draft_outcome_unknown")
    const next = parseResult(payload, this.accountId, result.creationKey)
    if (!next || JSON.stringify(next.action) !== JSON.stringify(result.action) || next.artifactVersion !== result.artifactVersion || next.bundleDigest !== result.bundleDigest || !next.review) throw new Error("draft_invalid_result")
    return structuredClone(next)
  }

  private signal(signal?: AbortSignal) { return signal ? AbortSignal.any([signal, AbortSignal.timeout(30_000)]) : AbortSignal.timeout(30_000) }
  private current(sequence: number) { if (sequence !== this.sequence) throw new Error("draft_superseded") }
  private denied(status: number) {
    if (status === 401) throw new Error("draft_authentication_required")
    if (status === 403) throw new Error("draft_access_denied")
  }
}

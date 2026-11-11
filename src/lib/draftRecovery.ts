import { workflowHeaders } from "./workflowRequest"

export interface DraftSource { id: string; revision: number }
export interface DraftInput {
  profileSource: DraftSource
  job: { posting: string; analysis?: unknown }
  answers: { requirement: string; userContext: string }[]
  confirmations: { requirement: string; userContext: string }[]
  selectedFactIds: string[]
  language: "en" | "pt-BR"
}
export interface DraftAction { namespace: string; id: string; owner: string; workflow: "application_draft" }
export interface DraftClaim { field?: string; text: string; state: "supported"; nonfactual: boolean; sources: { kind: string; profileId: string; id: string; revision: number }[]; jobSources: { start: number; end: number; quote: string }[]; concerns: string[] }
export interface RecoveredDraft {
  action: DraftAction; creationKey: string
  state: "pending" | "incomplete" | "ready" | "closed" | "deleted"
  artifactVersion: number; admittedAt: string; deadline: string
  historical: boolean; cleanupQueued: boolean
  job?: { posting: string; analysis?: unknown }
  context?: { answers: { requirement: string; userContext: string }[]; confirmations: { requirement: string; userContext: string }[]; selectedFactIds: string[] }
  bundle?: {
    schema: "application-draft-bundle-v1"
    binding: { action: DraftAction; profileSource: DraftSource; language: "en" | "pt-BR"; sourceDigest: string; inputDigest: string; generationVersion: string; reviewVersion: string }
    identity: { fullName: string; email: string; phone: string; location: string; professionalLinks: string }
    jobTitle: string | null; company: string | null
    roleSummary: string; resume: string; coverLetter: string
    employerAnswers: { state: "complete" | "none_requested"; text: string; requests: { id: string; source: { start: number; end: number; quote: string }; answer: string }[] }
    support: { version: string; check: "complete"; facts: { reference: { kind: string; profileId: string; id: string; revision: number }; snapshot: unknown }[]; resume: DraftClaim[]; artifacts: DraftClaim[]; concerns: string[]; completion: { version: string; state: string; inputDigest: string; candidateDigest: string; answerRequests: unknown[]; concerns: string[] } }
  }
}

type StoragePort = Pick<Storage, "getItem" | "setItem" | "removeItem">
const token = (v: unknown): v is string => typeof v === "string" && /^[A-Za-z0-9_-]{1,128}$/.test(v)
const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v)
const text = (v: unknown): v is string => typeof v === "string" && !!v.trim()
const actionEqual = (a: DraftAction, b: DraftAction) => a.namespace === b.namespace && a.id === b.id && a.owner === b.owner && a.workflow === b.workflow
const digest = (v: unknown): v is string => typeof v === "string" && /^[a-f0-9]{64}$/.test(v)

// This validates the recovery envelope, not semantic quality or billing. The
// server's frozen required-check snapshots remain the original review evidence.
function parseResult(value: unknown, account: string, key: string): RecoveredDraft | null {
  if (!object(value) || !object(value.action)) return null
  const r = value as unknown as RecoveredDraft
  if (r.creationKey !== key || r.action.owner !== account || r.action.workflow !== "application_draft" || !token(r.action.namespace) || !token(r.action.id) || !["pending", "incomplete", "ready", "closed", "deleted"].includes(r.state) || ![0, 1].includes(r.artifactVersion) || !Number.isFinite(Date.parse(r.admittedAt)) || !Number.isFinite(Date.parse(r.deadline)) || typeof r.historical !== "boolean" || typeof r.cleanupQueued !== "boolean") return null
  if (r.state !== "ready") return r.bundle === undefined && r.job === undefined && r.context === undefined ? r : null
  const b = r.bundle
  if (r.artifactVersion !== 1 || !b || b.schema !== "application-draft-bundle-v1" || !object(b.binding) || !object(b.binding.action) || !actionEqual(r.action, b.binding.action) || !object(b.binding.profileSource) || !text(b.binding.profileSource.id) || !Number.isSafeInteger(b.binding.profileSource.revision) || b.binding.profileSource.revision < 1 || !["en", "pt-BR"].includes(b.binding.language) || !digest(b.binding.sourceDigest) || !digest(b.binding.inputDigest) || b.binding.generationVersion !== "controlled-complete-draft-v1" || b.binding.reviewVersion !== "application-draft-review-snapshot-v1" || !r.job || !text(r.job.posting)) return null
  if (![b.roleSummary, b.resume, b.coverLetter].every(text) || !object(b.identity) || !["fullName", "email", "phone", "location", "professionalLinks"].every(k => typeof b.identity[k as keyof typeof b.identity] === "string") || ![b.jobTitle, b.company].every(v => v === null || text(v))) return null
  const s = b.support
  if (!s || s.version !== "application-draft-review-snapshot-v1" || s.check !== "complete" || !Array.isArray(s.facts) || !Array.isArray(s.resume) || !s.resume.length || !Array.isArray(s.artifacts) || !s.artifacts.length || !Array.isArray(s.concerns) || s.concerns.length || [...s.resume, ...s.artifacts].some(c => !object(c) || !text(c.text) || c.state !== "supported" || !Array.isArray(c.concerns) || c.concerns.length || !Array.isArray(c.sources) || !Array.isArray(c.jobSources))) return null
  const c = s.completion
  if (!c || c.version !== "complete-draft-eligibility-v1" || c.state !== "complete" || c.inputDigest !== b.binding.inputDigest || !digest(c.candidateDigest) || !Array.isArray(c.answerRequests) || !Array.isArray(c.concerns) || c.concerns.length) return null
  const a = b.employerAnswers
  if (!a || !["complete", "none_requested"].includes(a.state) || typeof a.text !== "string" || !Array.isArray(a.requests) || (a.state === "none_requested" ? !!a.text || !!a.requests.length || !!c.answerRequests.length : !text(a.text) || !a.requests.length || a.requests.length !== c.answerRequests.length)) return null
  return r
}

export class DraftRecoveryClient {
  private key: string | null = null
  private action: DraftAction | null = null
  private sequence = 0
  private readonly storageKey: string
  state: RecoveredDraft["state"] | "idle" | "uncertain" | "absent" = "idle"

  constructor(readonly accountId: string, private readonly storage: StoragePort | null = typeof sessionStorage === "undefined" ? null : sessionStorage, private readonly transport: typeof fetch = fetch) {
    this.storageKey = `careeros_draft_recovery:${encodeURIComponent(accountId)}`
    try { const key = storage?.getItem(this.storageKey); if (token(key)) { this.key = key; this.state = "uncertain" } } catch { /* Creation will fail before sending if recovery storage is unavailable. */ }
  }
  hasKey() { return this.key !== null }
  cancel() { this.sequence++ }
  async create(input: DraftInput, signal?: AbortSignal): Promise<RecoveredDraft> {
    if (this.state === "uncertain" || this.state === "pending") throw new Error("draft_recover_first")
    if (!input.profileSource || !text(input.profileSource.id) || !Number.isSafeInteger(input.profileSource.revision) || input.profileSource.revision < 1 || !text(input.job.posting)) throw new Error("draft_input")
    // The authenticated absence can race another request. Reuse its key so
    // the server still enforces the original binding and generation witness.
    const key = this.state === "absent" && this.key ? this.key : crypto.randomUUID()
    try { if (!this.storage) throw new Error(); this.storage.setItem(this.storageKey, key) } catch { throw new Error("draft_recovery_storage") }
    this.key = key; this.action = null; this.state = "uncertain"
    return this.request(key, "POST", input, signal)
  }
  async recover(signal?: AbortSignal): Promise<RecoveredDraft> {
    if (!this.key) throw new Error("draft_not_found")
    return this.request(this.key, "GET", undefined, signal)
  }
  private async request(key: string, method: "GET" | "POST", input?: DraftInput, signal?: AbortSignal): Promise<RecoveredDraft> {
    const sequence = ++this.sequence
    let confirmedAbsent = false
    try {
      const response = await this.transport(method === "POST" ? "/api/drafts" : `/api/drafts/${encodeURIComponent(key)}/versions/1`, {
        ...(method === "POST" ? { method, body: JSON.stringify(input) } : {}),
        headers: { ...workflowHeaders(this.accountId), ...(method === "POST" ? { "Content-Type": "application/json", "X-CareerOS-Action-Key": key } : {}) },
        signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(30_000)]) : AbortSignal.timeout(30_000),
      })
      const payload = await response.json().catch(() => null)
      if (sequence !== this.sequence || key !== this.key) throw new Error("draft_superseded")
      if (response.status === 401) throw new Error("draft_authentication_required")
      if (response.status === 403) throw new Error("draft_access_denied")
      if (response.status === 409 && object(payload) && payload.error === "account_changed") {
        throw new Error("draft_account_changed")
      }
      if (method === "GET" && !this.action && response.status === 404 && object(payload) && payload.error === "draft_not_found") {
        confirmedAbsent = true
        throw new Error("draft_not_found")
      }
      if (!response.ok && response.status !== 410) throw new Error("draft_outcome_unknown")
      const result = parseResult(payload, this.accountId, key)
      if (!result || (this.action && !actionEqual(this.action, result.action))) throw new Error("draft_invalid_result")
      this.action = { ...result.action }; this.state = result.state
      return structuredClone(result)
    } catch (error) {
      if (sequence !== this.sequence || key !== this.key) throw new Error("draft_superseded")
      this.state = confirmedAbsent ? "absent" : "uncertain"
      if (error instanceof Error && error.message.startsWith("draft_")) throw error
      throw new Error("draft_outcome_unknown")
    }
  }
}

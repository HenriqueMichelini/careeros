import { parseResult, type RecoveredDraft } from "./draftRecovery"
import { workflowHeaders } from "./workflowRequest"

export const openingPolicy = "complete-draft-opening-local-20261122-v1"
export const openingParts = ["role_summary", "resume", "cover_letter", "employer_answers", "support"] as const

interface OpeningManifest {
  schema: typeof openingPolicy
  bundleDigest: string
  artifactVersion: 1
  requiredParts: string[]
}
interface OpeningReceipt { receipt: string; bundleDigest: string; manifest: OpeningManifest }

function completeAvailability(parts: readonly string[]) {
  return parts.length === openingParts.length && openingParts.every(part => parts.includes(part)) && new Set(parts).size === openingParts.length
}

// This client is invoked only by a deliberate complete-review opening after the
// owning React article's required artifacts/support have committed. It has no
// generation, storage, Accept or allowance-override capability.
export class DraftOpeningClient {
  private sequence = 0
  constructor(readonly accountId: string, private readonly transport: typeof fetch = fetch, private readonly withholdProtected: () => void = () => {}) {}
  cancel() { this.sequence++ }

  async open(selected: RecoveredDraft, availableParts: readonly string[] | (() => readonly string[]), signal?: AbortSignal): Promise<RecoveredDraft> {
    const availability = () => typeof availableParts === "function" ? availableParts() : availableParts
    if (!parseResult(selected, this.accountId, selected.creationKey)) throw new Error("draft_invalid_result")
    if (!completeAvailability(availability()) || selected.state !== "ready" || !selected.bundleDigest || selected.opening?.policy !== openingPolicy || selected.opening.state !== "reserved" || selected.delivered) throw new Error("draft_opening_incomplete")
    const sequence = ++this.sequence
    const issued = await this.post(`/api/drafts/${encodeURIComponent(selected.creationKey)}/versions/1/opening`, {}, sequence, signal, selected) as OpeningReceipt
    const terminal = this.terminal(issued, selected)
    if (terminal) return terminal
    if (!issued || typeof issued.receipt !== "string" || !/^[a-f0-9]{32}\.[a-f0-9]{64}$/.test(issued.receipt) || issued.bundleDigest !== selected.bundleDigest || issued.manifest?.schema !== openingPolicy || issued.manifest.bundleDigest !== selected.bundleDigest || issued.manifest.artifactVersion !== selected.artifactVersion || !Array.isArray(issued.manifest.requiredParts) || JSON.stringify(issued.manifest.requiredParts) !== JSON.stringify(openingParts)) throw new Error("draft_opening_incomplete")
    if (!completeAvailability(availability())) throw new Error("draft_opening_incomplete")
    const payload = await this.post(`/api/drafts/${encodeURIComponent(selected.creationKey)}/versions/1/opening/confirm`, issued, sequence, signal, selected)
    const closed = this.terminal(payload, selected)
    if (closed) return closed
    const result = parseResult(payload, this.accountId, selected.creationKey)
    if (!result || JSON.stringify(result.action) !== JSON.stringify(selected.action) || result.artifactVersion !== selected.artifactVersion || result.bundleDigest !== selected.bundleDigest || !result.delivered || result.opening?.state !== "delivered") throw new Error("draft_invalid_result")
    return structuredClone(result)
  }

  private terminal(payload: unknown, selected: RecoveredDraft): RecoveredDraft | null {
    if (!selected.terminalPolicy) return null
    const result = parseResult(payload, this.accountId, selected.creationKey)
    return result && ["closed", "deleted"].includes(result.state) && result.terminalPolicy === selected.terminalPolicy && JSON.stringify(result.action) === JSON.stringify(selected.action) && result.artifactVersion === selected.artifactVersion ? structuredClone(result) : null
  }

  private async post(path: string, body: unknown, sequence: number, signal?: AbortSignal, selected?: RecoveredDraft): Promise<unknown> {
    let response = await this.transport(path, {
      method: "POST", headers: { ...workflowHeaders(this.accountId), "Content-Type": "application/json" }, body: JSON.stringify(body),
      signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(30_000)]) : AbortSignal.timeout(30_000),
    }).catch(() => { throw new Error("draft_outcome_unknown") })
    let payload = await response.json().catch(() => null)
    // A terminal Action rejection is distinct from global account denial. In
    // this explicit composition only an exact authorized terminal GET can
    // establish that distinction; arbitrary 403/404 payloads never grant it.
    if (response.status === 403 && selected?.terminalPolicy) {
      if (sequence !== this.sequence || signal?.aborted) throw new Error("draft_superseded")
      this.withholdProtected()
      response = await this.transport(`/api/drafts/${encodeURIComponent(selected.creationKey)}/versions/1`, {
        headers: workflowHeaders(this.accountId), signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(30_000)]) : AbortSignal.timeout(30_000),
      }).catch(() => { throw new Error("draft_outcome_unknown") })
      payload = await response.json().catch(() => null)
      if (sequence !== this.sequence || signal?.aborted) throw new Error("draft_superseded")
      if (response.status === 410 && this.terminal(payload, selected)) return payload
    }
    if (sequence !== this.sequence || signal?.aborted) throw new Error("draft_superseded")
    if (response.status === 401) throw new Error("draft_authentication_required")
    if (response.status === 403 || response.status === 404 || response.status === 410) throw new Error("draft_access_denied")
    if (!response.ok) throw new Error("draft_outcome_unknown")
    return payload
  }
}

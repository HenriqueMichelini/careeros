import { workflowHeaders } from "./workflowRequest"

export interface QualificationCommand {
  actionKey: string
  retryKey?: string
}
export interface QualificationStatus {
  actionId: string
  state: "ready" | "incomplete" | "unknown" | "known_failure" | "result_unavailable"
  attempt: number
  retryAllowed: boolean
  resultAvailable: false
}

// Only an opaque replay key survives this tab's reload. Career content and
// results remain transient; a status read never starts inference.
export class ManagedQualification {
  private key: string | null
  private binding: string | null = null
  private retryKey?: string
  private retryAllowed = false
  private readonly storageKey: string
  constructor(
    private accountId: string,
    private storage?: Storage,
  ) {
    this.storageKey = `careeros.qualification.${accountId}`
    try {
      this.key = storage?.getItem(this.storageKey) ?? null
    } catch {
      this.key = null
    }
  }
  command(binding: string): QualificationCommand {
    if (this.binding !== null && this.binding !== binding) this.reset()
    this.binding = binding
    if (!this.key) {
      this.key = crypto.randomUUID()
      try {
        this.storage?.setItem(this.storageKey, this.key)
      } catch {
        /* ephemeral replay key remains usable */
      }
    }
    if (this.retryAllowed && !this.retryKey) this.retryKey = crypto.randomUUID()
    this.retryAllowed = false
    return {
      actionKey: this.key,
      ...(this.retryKey ? { retryKey: this.retryKey } : {}),
    }
  }
  knownFailure(allowed: boolean) {
    this.retryAllowed = allowed
  }
  reset() {
    this.key = null
    this.binding = null
    this.retryKey = undefined
    this.retryAllowed = false
    try {
      this.storage?.removeItem(this.storageKey)
    } catch {
      /* transient state is authoritative in this tab */
    }
  }
  async status(signal?: AbortSignal): Promise<QualificationStatus> {
    if (!this.key) throw new Error("qualification_no_action")
    const response = await fetch(
      `/api/ai/qualification/actions/${encodeURIComponent(this.key)}`,
      { headers: workflowHeaders(this.accountId), signal },
    )
    const value: unknown = await response.json()
    if (!response.ok || !value || typeof value !== "object")
      throw new Error("qualification_status_unavailable")
    const s = value as QualificationStatus
    if (
      !new Set([
        "ready",
        "incomplete",
        "unknown",
        "known_failure",
        "result_unavailable",
      ]).has(s.state) ||
      !Number.isInteger(s.attempt) ||
      s.attempt < 1 ||
      s.attempt > 2 ||
      typeof s.retryAllowed !== "boolean" ||
      s.resultAvailable !== false
    )
      throw new Error("qualification_status_unavailable")
    this.retryAllowed = s.retryAllowed
    return s
  }
}

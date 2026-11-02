import { csrfHeaders } from "./workflowRequest"

export interface PilotStatus {
  accountId: string
  state: string
  invitationId?: string
  invitationExpiresAt?: string
  expiresAt?: string
  retrieveUntil?: string
  canActivate: boolean
  canDraft: boolean
  canStandalone: boolean
  canRetrieve: boolean
  draftAllowance: number
}

// This client never supplies authority and never auto-replays activation.
// Profile drafts and pending Saves stay owned by the existing account client.
export class PilotAccessClient {
  status: PilotStatus | null = null
  private live = true
  private lease = 0
  private pending: {
    operationId: string
    invitationId: string
    confirm: true
  } | null = null
  constructor(
    readonly accountId: string,
    private request: typeof fetch = fetch,
  ) {}
  dispose() {
    this.live = false
    this.lease++
  }
  resume() {
    this.live = true
  }
  private async call(
    path: string,
    method = "GET",
    body?: string,
  ): Promise<PilotStatus> {
    if (!this.live) throw new Error("Account changed")
    const lease = this.lease
    const response = await this.request(path, {
      method,
      body,
      credentials: "same-origin",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        "X-CareerOS-Account": this.accountId,
        ...csrfHeaders(),
      },
      signal: AbortSignal.timeout(10_000),
    })
    const value = await response.json()
    if (
      !this.live ||
      lease !== this.lease ||
      !response.ok ||
      value.accountId !== this.accountId ||
      ![
        "uninvited",
        "invited",
        "invitation_expired",
        "invitation_revoked",
        "active",
        "suspended",
        "revoked",
        "retrieval_only",
        "expired",
      ].includes(value.state) ||
      ["canActivate", "canDraft", "canStandalone", "canRetrieve"].some(
        (key) => typeof value[key] !== "boolean",
      ) ||
      value.draftAllowance !== 5 ||
      ["expiresAt", "retrieveUntil", "invitationExpiresAt"].some(
        (key) =>
          value[key] !== undefined &&
          (typeof value[key] !== "string" ||
            !Number.isFinite(Date.parse(value[key]))),
      ) ||
      (value.canActivate &&
        (typeof value.invitationId !== "string" ||
          !/^[a-zA-Z0-9_-]{1,128}$/.test(value.invitationId)))
    )
      throw new Error("Access status unavailable")
    this.status = value
    return value
  }
  async refresh() {
    return this.call("/api/pilot/status")
  }
  async activate() {
    if (!this.pending) {
      if (!this.status?.canActivate || !this.status.invitationId)
        throw new Error("Activation unavailable")
      this.pending = {
        operationId: crypto.randomUUID(),
        invitationId: this.status.invitationId,
        confirm: true,
      }
    }
    try {
      const status = await this.call(
        "/api/pilot/activate",
        "POST",
        JSON.stringify(this.pending),
      )
      this.pending = null
      return status
    } catch {
      // A read can establish successful activation after response loss. It
      // never submits a new mutation after sign-in, focus, reload or timeout.
      const status = await this.refresh()
      if (status.expiresAt) this.pending = null
      return status
    }
  }
}

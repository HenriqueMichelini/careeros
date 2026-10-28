import {
  emptyProfileView,
  migrateProfile,
  validateProfileDocument,
  type ProfileDocument,
} from "./profileDocument"
import { ProfileStorageError } from "./profileStorage"
import { csrfHeaders } from "./workflowRequest"
export { csrfHeaders } from "./workflowRequest"

export type SaveState = "idle" | "pending" | "failed" | "uncertain" | "confirmed"
interface SaveOutcome {
  operationId: string
  profileId: string
  committedRevision: number
}
export interface ProfileReply {
  accountId: string
  document: ProfileDocument | null
  outcome?: SaveOutcome
}
interface PendingSave {
  operationId: string
  expectedRevision: number
  document: ProfileDocument
}
type RecoveryStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">
function tabRecoveryStorage(): RecoveryStorage | null {
  try {
    return typeof sessionStorage === "undefined" ? null : sessionStorage
  } catch {
    return null
  }
}

function replyError(
  status: number,
  body: { error?: string },
): ProfileStorageError {
  if (status === 404) return new ProfileStorageError("not_found")
  if (status === 401) return new ProfileStorageError("authentication")
  if (body?.error === "account_changed")
    return new ProfileStorageError("account_changed")
  if (status === 409) return new ProfileStorageError("stale")
  if (status === 422) return new ProfileStorageError("validation")
  return new ProfileStorageError("storage")
}

// Account-scoped pending-work recovery only. The tab checkpoint contains no
// credentials and cannot supply account authority: open still requires a
// successful current server read before StoreProvider exposes any draft.
export class AccountProfile {
  active = true
  state: SaveState = "idle"
  historicalRevision: number | null = null
  private exists = false
  get hasSavedProfile() {
    return this.exists
  }
  checkpointStored = false
  private lease = 0
  private pending: PendingSave | null = null
  private invalidCheckpoint: string | null = null
  private get recoveryKey() {
    return `careeros_pending_profile_v1:${this.accountId}`
  }
  constructor(
    readonly accountId: string,
    private readonly notify: () => void = () => {},
    private readonly request: typeof fetch = fetch,
    private readonly storage: RecoveryStorage | null = tabRecoveryStorage(),
  ) {
    let raw: string | null = null
    try {
      raw = storage?.getItem(this.recoveryKey) ?? null
    } catch {
      /* Memory recovery remains available. */
    }
    if (raw !== null) {
      try {
        const saved = JSON.parse(raw)
        const p = saved.pendingSave
        if (
          saved.version !== 1 ||
          saved.accountId !== accountId ||
          !p ||
          typeof p.operationId !== "string" ||
          !/^[a-zA-Z0-9_-]{1,128}$/.test(p.operationId) ||
          !Number.isSafeInteger(p.expectedRevision) ||
          p.expectedRevision < 0 ||
          !validateProfileDocument(p.document) ||
          (p.expectedRevision > 0 &&
            p.document.revision !== p.expectedRevision + 1)
        )
          throw new Error("Invalid pending Profile checkpoint")
        this.pending = p
        this.state = "uncertain"
        this.checkpointStored = true
      } catch {
        this.invalidCheckpoint = raw
        this.state = "failed"
      }
    }
  }
  dispose() {
    this.active = false
    this.lease++
    if (this.pending) this.state = "uncertain"
  }
  resume() {
    this.active = true
  }
  private assertActive(lease = this.lease) {
    if (!this.active || lease !== this.lease)
      throw new ProfileStorageError("account_changed")
  }
  private checkpoint() {
    try {
      this.storage?.setItem(
        this.recoveryKey,
        JSON.stringify(this.recoveryExport()),
      )
      this.checkpointStored = this.storage !== null
    } catch {
      this.checkpointStored = false
    }
  }
  private status(state: SaveState) {
    this.assertActive()
    this.state = state
    this.notify()
  }
  private async call(
    path: string,
    init: RequestInit = {},
  ): Promise<ProfileReply> {
    const lease = this.lease
    this.assertActive(lease)
    const response = await this.request(path, {
      ...init,
      credentials: "same-origin",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        "X-CareerOS-Account": this.accountId,
        ...csrfHeaders(),
        ...init.headers,
      },
      signal: AbortSignal.timeout(15_000),
    })
    this.assertActive(lease)
    const body = await response.json()
    this.assertActive(lease)
    if (!response.ok) throw replyError(response.status, body)
    if (body.accountId !== this.accountId)
      throw new ProfileStorageError("account_changed")
    if (body.document !== null && !validateProfileDocument(body.document))
      throw new ProfileStorageError("validation")
    return body
  }
  async open(): Promise<ProfileDocument> {
    const lease = this.lease
    const reply = await this.call("/api/profile/current")
    this.assertActive(lease)
    this.exists = reply.document !== null
    return (
      reply.document ?? migrateProfile(emptyProfileView(), crypto.randomUUID())
    )
  }
  private confirm(reply: ProfileReply): ProfileDocument {
    const pending = this.pending
    if (
      !pending ||
      !reply.document ||
      reply.outcome?.operationId !== pending.operationId ||
      reply.outcome.profileId !== pending.document.id ||
      reply.outcome.committedRevision !== pending.document.revision ||
      reply.document.id !== pending.document.id ||
      reply.document.revision < pending.document.revision
    )
      throw new ProfileStorageError("validation")
    this.historicalRevision = reply.outcome.committedRevision
    this.exists = true
    this.pending = null
    try {
      this.storage?.removeItem(this.recoveryKey)
    } catch {
      this.checkpointStored = false
    }
    this.status("confirmed")
    // Return CURRENT authority, which may be newer than the historical outcome.
    return reply.document
  }
  async save(
    expected: ProfileDocument,
    candidate: ProfileDocument,
  ): Promise<ProfileDocument> {
    this.assertActive()
    if (
      !validateProfileDocument(candidate) ||
      candidate.id !== expected.id ||
      candidate.revision !== expected.revision + 1
    )
      throw new ProfileStorageError("validation")
    if (this.hasRecovery()) throw new ProfileStorageError("uncertain")
    this.pending = {
      operationId: crypto.randomUUID(),
      expectedRevision: this.exists ? expected.revision : 0,
      document: structuredClone(candidate),
    }
    this.checkpoint()
    return this.retry()
  }
  async retry(): Promise<ProfileDocument> {
    const lease = this.lease
    this.assertActive(lease)
    if (!this.pending) throw new ProfileStorageError("validation")
    this.status("pending")
    const confirm = (reply: ProfileReply) => {
      this.assertActive(lease)
      return this.confirm(reply)
    }
    try {
      return confirm(
        await this.call("/api/profile/current", {
          method: "PUT",
          body: JSON.stringify(this.pending),
        }),
      )
    } catch (error) {
      this.assertActive(lease)
      const definite =
        error instanceof ProfileStorageError &&
        ["validation", "stale", "account_changed", "authentication"].includes(
          error.code,
        )
      if (definite) {
        this.status("failed")
        throw error
      }
      // The response may be lost after commit. A read never repeats the write.
      try {
        return confirm(
          await this.call(
            `/api/profile/operations/${this.pending!.operationId}`,
          ),
        )
      } catch {
        this.assertActive(lease)
        this.status("uncertain")
        throw new ProfileStorageError("uncertain")
      }
    }
  }
  async refresh(): Promise<ProfileDocument> {
    const lease = this.lease
    if (this.pending) {
      const reply = await this.call(
        `/api/profile/operations/${this.pending.operationId}`,
      )
      this.assertActive(lease)
      return this.confirm(reply)
    }
    return this.open()
  }
  hasRecovery() {
    return this.pending !== null || this.invalidCheckpoint !== null
  }
  recoveryExport() {
    return {
      version: 1,
      accountId: this.accountId,
      pendingSave: this.recovery(),
      ...(this.invalidCheckpoint === null
        ? {}
        : { invalidCheckpoint: this.invalidCheckpoint }),
    }
  }
  discardRecovery() {
    // This discards a local recovery copy, never cancels/undoes a server commit.
    // Invalidate in-flight continuations before allowing another operation.
    try {
      this.storage?.removeItem(this.recoveryKey)
    } catch {
      throw new ProfileStorageError("storage")
    }
    this.lease++
    this.pending = null
    this.invalidCheckpoint = null
    this.state = "idle"
    this.notify()
  }
  recovery() {
    return this.pending ? structuredClone(this.pending) : null
  }
}

// Selected #258 D03/D04 cleanup is a domain deletion policy. It is applied
// before Save and independently enforced by Go; the byte store never rewrites.
export function applyAccountRetention(
  before: ProfileDocument,
  candidate: ProfileDocument,
): ProfileDocument {
  const next = structuredClone(candidate)
  const remaining = new Set(next.facts.map((f) => f.id))
  const removed = new Set(
    before.facts.filter((f) => !remaining.has(f.id)).map((f) => f.id),
  )
  const affected = new Set(
    before.links
      .filter((l) => l.kind === "supports" && removed.has(l.from.id))
      .map((l) => l.to.id),
  )
  const referenced = new Set(
    next.links
      .filter((l) => l.kind === "supports" && remaining.has(l.from.id))
      .map((l) => l.to.id),
  )
  next.evidence = next.evidence.filter(
    (e) => referenced.has(e.id) && !affected.has(e.id),
  )
  const kept = new Set(next.evidence.map((e) => e.id))
  for (const link of next.links)
    if (link.kind === "supports" && !kept.has(link.to.id)) {
      link.state = "invalidated"
      const fact = next.facts.find((f) => f.id === link.from.id)
      if (fact?.support === "supported") fact.support = "invalidated"
    }
  return next
}

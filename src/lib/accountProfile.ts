import {
  emptyProfileView,
  migrateProfile,
  validateProfileDocument,
  type ProfileDocument,
} from "./profileDocument"
import { ProfileStorageError } from "./profileStorage"

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

export function csrfHeaders(): Record<string, string> {
  const token = document.cookie
    .split(";")
    .map((v) => v.trim())
    .find((v) => v.startsWith("__Host-careeros_csrf="))
    ?.split("=")[1]
  return token ? { "X-CSRF-Token": decodeURIComponent(token) } : {}
}

// One client per mounted account workspace. Nothing here reads browser Profile
// storage, persists career content, or treats a client account ID as authority.
export class AccountProfile {
  active = true
  state: SaveState = "idle"
  historicalRevision: number | null = null
  private exists = false
  private pending: {
    operationId: string
    expectedRevision: number
    document: ProfileDocument
  } | null = null
  constructor(
    readonly accountId: string,
    private notify: () => void = () => {},
    private request: typeof fetch = fetch,
  ) {}
  dispose() {
    this.active = false
  }
  private assertActive() {
    if (!this.active) throw new ProfileStorageError("account_changed")
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
    this.assertActive()
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
    this.assertActive()
    const body = await response.json()
    this.assertActive()
    if (!response.ok)
      throw new ProfileStorageError(
        response.status === 404
          ? "not_found"
          : response.status === 401
            ? "authentication"
            : body?.error === "account_changed"
              ? "account_changed"
              : response.status === 409
                ? "stale"
                : response.status === 422
                  ? "validation"
                  : "storage",
      )
    if (body.accountId !== this.accountId)
      throw new ProfileStorageError("account_changed")
    if (body.document !== null && !validateProfileDocument(body.document))
      throw new ProfileStorageError("validation")
    return body
  }
  async open(): Promise<ProfileDocument> {
    const reply = await this.call("/api/profile/current")
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
    this.status("confirmed")
    // Return CURRENT authority, which may be newer than the historical outcome.
    return reply.document
  }
  async save(
    expected: ProfileDocument,
    candidate: ProfileDocument,
  ): Promise<ProfileDocument> {
    if (
      !validateProfileDocument(candidate) ||
      candidate.id !== expected.id ||
      candidate.revision !== expected.revision + 1
    )
      throw new ProfileStorageError("validation")
    if (this.pending) throw new ProfileStorageError("uncertain")
    this.pending = {
      operationId: crypto.randomUUID(),
      expectedRevision: this.exists ? expected.revision : 0,
      document: structuredClone(candidate),
    }
    return this.retry()
  }
  async retry(): Promise<ProfileDocument> {
    this.assertActive()
    if (!this.pending) throw new ProfileStorageError("validation")
    this.status("pending")
    try {
      return this.confirm(
        await this.call("/api/profile/current", {
          method: "PUT",
          body: JSON.stringify(this.pending),
        }),
      )
    } catch (error) {
      this.assertActive()
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
        return this.confirm(
          await this.call(
            `/api/profile/operations/${this.pending!.operationId}`,
          ),
        )
      } catch {
        this.status("uncertain")
        throw new ProfileStorageError("uncertain")
      }
    }
  }
  async refresh(): Promise<ProfileDocument> {
    if (this.state === "failed") {
      this.pending = null
      return this.open()
    }
    if (this.pending)
      return this.confirm(
        await this.call(`/api/profile/operations/${this.pending.operationId}`),
      )
    return this.open()
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

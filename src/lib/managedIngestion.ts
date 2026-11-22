import { ManagedQualification } from "./managedQualification"

// Each foreground request belongs to one complete root Field Submission.
// Stage receipts/answers stay transient; only the opaque account-scoped key is
// retained for explicit status recovery, which never recreates provider sends.
export class ManagedIngestion extends ManagedQualification {
  private lastStage?: string
  private failedStage?: string
  constructor(accountId: string, storage?: Storage, scope?: string) { super(accountId, storage, "ingestion", scope) }
  override knownFailure(allowed: boolean) { this.failedStage=allowed?this.lastStage:undefined;super.knownFailure(allowed) }
  override reset() { this.lastStage=undefined;this.failedStage=undefined;super.reset() }
  override async status(signal?:AbortSignal) { const value=await super.status(signal);if(value.retryAllowed)this.failedStage=this.lastStage;return value }
  stage(body: object) {
    const value = body as { input?: unknown; profile?: unknown; document?: unknown;stage?:{kind?:string} }
    const stage=JSON.stringify(body)
    if(value.stage?.kind==="clarify"&&this.failedStage&&stage!==this.failedStage){super.knownFailure(false);this.failedStage=undefined}
    this.lastStage=stage
    return this.command(JSON.stringify({ input: value.input, profile: value.profile, ...(value.document ? { document: value.document } : {}) }))
  }
}

// Match the existing in-memory submission-controller lifetime. A Profile
// remount must not give its status button a different retry owner from the
// transport retained by that controller. Different accounts never share one.
const ingestionOwners = new Map<string, ManagedIngestion>()
export function managedIngestionOwner(scope: string, accountId: string, storage?: Storage): ManagedIngestion {
  const key = JSON.stringify([scope, accountId])
  const existing = ingestionOwners.get(key)
  if (existing) return existing
  const owner = new ManagedIngestion(accountId, storage, scope)
  ingestionOwners.set(key, owner)
  return owner
}
export function forgetManagedIngestion(accountId: string) {
  for(const [key,owner] of ingestionOwners)if(JSON.parse(key)[1]===accountId){owner.reset();ingestionOwners.delete(key)}
}

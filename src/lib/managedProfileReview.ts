import { ManagedQualification } from "./managedQualification"

// Career input and proposals stay in memory. Only the account/section-scoped
// opaque recovery key survives reload; status never generates or saves content.
export class ManagedProfileReview extends ManagedQualification {
  private hasResult = false
  constructor(accountId: string, storage?: Storage, section?: string) {
    super(accountId, storage, "profile-review", section)
  }
  override command(binding: string) {
    // A deliberate rewrite after a successful proposal is a NEW Action.
    if (this.hasResult) this.reset()
    return super.command(binding)
  }
  override async status(signal?: AbortSignal) {
    const value = await super.status(signal)
    // Finished success is authoritative metadata, not recovered proposal
    // content. Rotate only on the NEXT deliberate command; unknown work keeps
    // its identity and known failure keeps the existing explicit Retry path.
    this.hasResult = value.state === "result_unavailable" && !value.retryAllowed
    return value
  }
  completed() { this.hasResult = true; this.knownFailure(false) }
  override reset() { this.hasResult = false; super.reset() }
}
const owners = new Map<string, ManagedProfileReview>()
export function managedProfileReviewOwner(accountId: string, section: string, storage?: Storage) {
  const key = JSON.stringify([accountId, section])
  let owner = owners.get(key)
  if (!owner) { owner = new ManagedProfileReview(accountId, storage, section); owners.set(key, owner) }
  return owner
}

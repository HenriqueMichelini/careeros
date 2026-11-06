import { ManagedQualification, type QualificationStatus } from "./managedQualification"

// Recovery stores only an account-scoped opaque key, never CV/source content.
// A reviewable uncertain candidate is manual work, never generation Retry.
export class ManagedStandaloneCV extends ManagedQualification {
  private root: string | null = null
  private candidate = false
  private review = false
  private waiting = false
  constructor(accountId: string, storage?: Storage) { super(accountId, storage, "cv") }
  override command(binding: string) {
    // Reload retains no source/content binding. A deliberate new choice must
    // reach the server's root-scoped review latch; a caller cannot clear it.
    // Merely reading status never rotates the stored recovery key.
    if (this.review && this.root === null) this.reset()
    if (this.root !== null && this.root !== binding) this.reset()
    if (this.review) throw new Error("managed_review_required")
    if (this.waiting) throw new Error("managed_status")
    if (this.candidate) this.reset()
    this.root = binding
    return super.command(binding)
  }
  pending() { this.waiting = true }
  completed(unresolved: boolean) {
    this.waiting = false
    this.candidate = true
    this.review = unresolved
    super.knownFailure(false)
  }
  override knownFailure(allowed: boolean) { this.waiting = !allowed; super.knownFailure(allowed) }
  observe(value: QualificationStatus & {reviewRequired?: boolean}) {
    this.review = value.reviewRequired === true
    this.candidate = value.state === "result_unavailable"
    this.waiting = !this.review && !this.candidate && value.state !== "known_failure" && value.state !== "ready"
    super.knownFailure(value.retryAllowed && !this.review)
  }
  override async status(signal?: AbortSignal) { const value = await super.status(signal); this.observe(value); return value }
  override reset() { this.root = null; this.waiting = this.review = this.candidate = false; super.reset() }
}
const owners = new Map<string, ManagedStandaloneCV>()
export function managedStandaloneCVOwner(accountId: string, storage?: Storage) {
  let owner = owners.get(accountId)
  if (!owner) { owner = new ManagedStandaloneCV(accountId, storage); owners.set(accountId, owner) }
  return owner
}

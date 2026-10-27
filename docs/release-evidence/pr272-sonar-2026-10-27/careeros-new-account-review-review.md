# New-account persistence independent review

Reviewed 26 October 2026, America/Sao_Paulo.

- Base: `591b2889196f277c96b130192a68bbc700a2137f`
- Head: `16ba1b5d7e01d9f1cc782967f508a4631071ec9d`
- Diff: `git diff 591b2889196f277c96b130192a68bbc700a2137f...16ba1b5d7e01d9f1cc782967f508a4631071ec9d`
- Implementation: `/tmp/careeros-new-account-persistence`, `codex/new-account-persistence`.
- Isolated review: `/home/henriquemichelini/.codex/worktrees/review-new-account-persistence/careeros`, detached at the exact head.

Scope: the owner's new-account persistence assignment, PostgreSQL/bytea implementation decision recorded in the patch, existing Google/direct-Go/opaque-session contract and #258 provenance rules. Legacy browser-data import is deferred. Production provider/runtime/retention/restore work and the existing five guest AI Functions are explicit exclusions, not findings against this slice.

## Standards

**0 documented violations, 0 material smell findings.** Independent Standards review found that the patch follows the applicable documented conventions. The new owner-authorized account/database scope supersedes the dated v1 restrictions. Publication is held for exact-text approval under the assignment. Repeated transaction setup and independent frontend/backend validation do not justify a blocking refactor by themselves.

## Spec

1. **[P2] Preserve drafts and uncertain operations across session renewal/expiry.** `src/components/AccountGate.tsx:45–51,165–168`. The contract says: “If unresolved, the state remains uncertain, retains the frozen request/edits in memory and offers recovery download” (`docs/new-account-persistence.md:41`). The workspace identity includes `expiresAt`, so the same account receiving a replacement session disposes its client and remounts the store. An unauthenticated session probe also unmounts it. The real Chrome probe created an uncertain Save with `REAUTH_UNSAVED_SENTINEL`; changing only the same account's session expiry restored the older saved Profile, cleared the editor and removed the recovery operation. An unauthenticated probe followed by sign-in to the same account also lost `EXPIRED_UNSAVED_SENTINEL` and its recovery state. Preserve account-scoped recovery across these transitions while fencing access and preventing cross-account display/replay. Automatic expiry must not silently discard work, and explicit logout needs a deliberate recovery/discard path where work is pending.

2. **[P2] Check initiating-session expiry after acquiring account locks.** `internal/accountstore/store.go:218–227`. The inherited auth contract says: “A missing, revoked or expired initiating session cannot authorize insertion” (`docs/migration/04-authentication-user-management/README.md:79`). Both expiry comparisons use the caller's `now`, captured before waiting for account locks. The independent real-PostgreSQL regression held the account lock, started replacement while the initiating session was valid, waited across natural expiry, verified database expiry and released the lock. The replacement nevertheless committed: `initiating_session_expired_before_lock_release=true replacement_created=true Create_error=<nil>`. Evaluate expiry with current database time after lock acquisition and retain a deterministic regression. Preserve both existing logout/replacement orderings and positive controls.

No additional material Spec findings or scope creep were identified. These are reproduced defects, not a comprehensive security-scan result.

## Verification and evidence

- Both supplied commit identities resolve; the diff is nonempty and contains one implementation commit.
- Every entry in `/tmp/careeros-new-account-evidence-2026-10-26/SHA256SUMS` verifies. All 41 source hashes in `commit.json` match the exact reviewed head.
- Retained final logs support the reported local Go race, 313 frontend-test and browser passes; these broad suites were not independently rerun for this review. Existing successes did not cover the two reproduced schedules above.
- The retained base/head comparison records the same 13 failing Evaluation Lab top-level tests. The unchanged ProcessRunner manifest has 28 stale entries at base and 34 at head; the six additions are patch-owned. This is not a green whole-Go-suite result.
- Root independently executed the actual React/Chrome session-transition probe with synthetic transport. Source: `session-loss-probe.mjs`; result: `session-loss-probe-result.json`; screenshot: `/tmp/careeros-account-browser-a9Hn7a/account-workspace.png`.
- The independent Spec reviewer executed the PostgreSQL expiry regression using an overlay and a temporary loopback-only container. Source: `persistence_expiry_overlay_test.go`; configuration: `expiry-overlay.json`; retained failing output: `session-replacement-expiry.log`. The temporary container was removed.
- Source whitespace checks pass; both implementation and review worktrees remain clean. No implementation edits, issue writes, push, provider calls, merge or deployment occurred during review.

Recommendation: return the two findings for scoped repairs before publication. Preserve the reviewed commit and original logs; verify the repair diff and rerun the new regressions plus directly affected checks rather than repeating a broad audit.

Standards: 0 findings, no worst issue. Spec: 2 P2 findings; worst priority within Spec is P2.
Repair the two independently reproduced P2 findings against CareerOS commit `16ba1b5d7e01d9f1cc782967f508a4631071ec9d` on `codex/new-account-persistence` in `/tmp/careeros-new-account-persistence`.

Read `/tmp/careeros-new-account-review/review.md` and the retained probes before editing. Follow AGENTS.md, RTK and the canonical GitHub workflow. Preserve original evidence and the reviewed commit; make a focused follow-up commit, with additive repair evidence.

1. AccountGate currently keys the account client/store by account ID plus session expiry and disposes it when a probe becomes unauthenticated. Actual Chrome confirmed an uncertain Save's pending edits and operation disappear on same-account replacement and on expiry followed by reauthentication. Preserve recoverable work across same-account session transitions. Keep signed-out/other-account content fenced; do not display or replay A's state under B. Handle explicit logout deliberately if work is pending. Do not introduce legacy reads, silent uploads, persistent credentials or account-authority fallback to browser storage. Choose the smallest safe recovery design consistent with the existing local implementation scope.

2. PostgreSQL Sessions.Create checks the initiating session against a caller-supplied time captured before account-lock waiting. A replacement can wait past the initiating session's expiry and still commit. Evaluate validity against current database time after the lock is obtained, preserving the common lock order, atomic replacement/revocation contract and current status/generation checks.

Independent reproducers:
- `/tmp/careeros-new-account-review/session-loss-probe.mjs`
- `/tmp/careeros-new-account-review/session-loss-probe-result.json`
- `/tmp/careeros-new-account-review/persistence_expiry_overlay_test.go` (the additional test is at the end; the file includes the original tests for Go overlay use)
- `/tmp/careeros-new-account-review/expiry-overlay.json`
- `/tmp/careeros-new-account-review/session-replacement-expiry.log`

Add permanent regressions with independently expected outcomes. Show the existing head fails and the repaired head passes for both findings. Cover same-account replacement, expiry/reauthentication, different-account isolation/late replies, recovery identity and deliberate discard; cover replacement blocked across natural session expiry, valid replacement, another device, fresh sign-in and both logout/replacement orderings. Use actual React and isolated PostgreSQL with synthetic accounts, not a fake database. The previous review PostgreSQL container has been removed.

Rerun the new regressions and directly affected auth/account race and frontend/type/build checks. Broaden only for new failures or unresolved concerns. Use `GOCACHE=/tmp/careeros-go-cache`. Preserve and accurately qualify inherited Evaluation Lab failures and patch-owned manifest drift; do not change/reapprove the manifest to make it green.

Report exact repaired head, changed behavior, failing-before/passing-after evidence and remaining limitations. Update the prepared publication drafts as needed for the final implementation. Keep the existing publication/provider/runtime/spending/deployment boundaries: commit locally; no automatic remote publication, push, merge, provisioning or live Google/provider execution. No broad reimplementation or repeated whole security audit is required.

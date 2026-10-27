# Focused new-account session repair verification

Reviewed on 26 October 2026, America/Sao_Paulo.

- Reviewed parent: `16ba1b5d7e01d9f1cc782967f508a4631071ec9d`.
- Repaired head: `bc77c4beec5f6d1d476a0b5c1f0e5f27d6f8547a`; direct child of the reviewed parent.
- Original feature base: `591b2889196f277c96b130192a68bbc700a2137f`.
- Exact diff: `git diff 16ba1b5d7e01d9f1cc782967f508a4631071ec9d...bc77c4beec5f6d1d476a0b5c1f0e5f27d6f8547a`.
- Isolated review checkout: `/home/henriquemichelini/.codex/worktrees/review-new-account-session-repairs/careeros`.
- Implementation checkout: `/tmp/careeros-new-account-persistence`, branch `codex/new-account-persistence`.

This is a focused review against the two findings and repair assignment in `/tmp/careeros-new-account-review/`. It does not repeat the original whole-feature review or constitute a broad security audit.

## Standards

**0 documented violations and 0 material heuristic findings.** The independent Standards reviewer found the account-scoped recovery, request lease checks and database-clock checks consistent with the assignment and documented boundaries. No blocking refactor is warranted by repeated guards at independent asynchronous continuations.

## Spec

**Both earlier P2 findings are independently verified closed; 0 new actionable findings in this repair diff.**

1. Pending Save recovery: account workspace identity no longer follows session expiry. Account-scoped recovery and request leases preserve the frozen pending operation while fencing signed-out/other-account display and old replies. The current authenticated server read still gates reconstructed recovery. Three actual React/Chrome scenarios independently pass: same-account replacement, actual frontend expiry followed by reauthentication, and account switching/logout cancel-keep-discard/reload. They assert retained wording, unchanged operation bytes, no automatic PUT, no legacy reads and no credentials in the checkpoint. The nine account-client contracts also pass, including old-lease rejection and checkpoint storage failure without server-authority fallback.

2. Initiating-session expiry after lock waiting: validation and removal use current database `clock_timestamp()` after account locking. The unchanged original independent PostgreSQL overlay now passes under the race detector: `initiating_session_expired_before_lock_release=true replacement_created=false Create_error=invalid or expired sign-in transaction`. The three permanent focused tests also pass under the race detector, covering natural expiry, both logout/replacement orderings, valid replacement despite a deliberately incorrect caller clock, another device and fresh sign-in.

## New execution and integrity evidence

- Root execution: `browser-sessions.log`, covering the three actual Chrome scenarios. Browser evidence directories are `/tmp/careeros-account-browser-8WfkGR`, `/tmp/careeros-account-browser-TGwTgo`, `/tmp/careeros-account-browser-0iu5le`.
- Root execution: `account-profile.log`, 9 tests passed, no failures/skips.
- Independent Spec reviewer execution: `independent-expiry-overlay.log`, `targeted-replacement-race.log`; adapted overlay configuration `expiry-overlay.json`. Original overlay source stayed byte-identical. The temporary synthetic loopback PostgreSQL container was removed after verification.
- All entries in the original implementation and additive repair SHA256SUMS inventories verify. All 11 changed-source hashes match the exact repaired checkout. All six recorded original review/probe hashes and all seven publication-draft hashes verify.
- Source whitespace checks pass. Both implementation and review worktrees remain clean.

The implementation agent's broader 316-test, repeated-race, type/vet/build passes remain retained author evidence; only the focused checks above were independently rerun here. The whole-Go suite was not rerun, and the retained 13 Evaluation Lab top-level failures remain unresolved. Recorded ProcessRunner drift is 28 at the original published base, 34 at reviewed parent, and 34 after repair; the manifest is unchanged.

The tab checkpoint contains the frozen pending Save only. It is untrusted recovery, not saved server authority or guaranteed durable storage; arbitrary unsubmitted form state is outside its persistence guarantee. Production provider/runtime, whole hosted-route enforcement, retention and disaster-recovery gates remain separate.

Recommendation: the two defects no longer block the bounded local feature's publication review at exact head `bc77c4beec5f6d1d476a0b5c1f0e5f27d6f8547a`. Proposed publication texts remain in `/tmp/careeros-new-account-publication-2026-10-26`. Publication requires the owner's exact-text approval and fresh deployment-prevention verification before any push/PR creation. No merge or deployment is recommended or authorized by this review.

Standards: 0 findings, no worst issue. Spec: 0 new findings, both earlier P2 findings closed.

# #289 never-paid expiry and approved content cleanup — local evidence

Status: source frozen and final checks complete; independent immutable reviews are pending. This report describes synthetic local verification, not hosted operations, participant readiness or actual email/recipient disposal.

Base: `63b20b29d16de955c77869c9faf417dc642e9a75`, branch `codex/batch-277-qualification-20261102`, isolated worktree `/home/henriquemichelini/workstation/careeros/.local-worktrees/qualification-277-20261102`. The companion `2026-11-22-never-paid-cleanup-summary.json` records exact source blobs, receipt hashes, commands, counts, skips and final comparisons. The frozen manifest contains 24 runtime files, 13 fixtures and two engineering documents. A fixture-only refreeze added the bilingual end-to-end retention expectation; production code did not change between the two freezes.

## Selected scope and authority

The canonical issue is [#289](https://github.com/HenriqueMichelini/careeros/issues/289). The selected [expiry/notice contract](https://github.com/HenriqueMichelini/careeros/issues/289#issuecomment-6644628888) requires in-app and account-email notices at access end and seven days before removal, without extending rights when delivery fails. The [invitation contract](https://github.com/HenriqueMichelini/careeros/issues/289#issuecomment-6645201526) separately cleans never-activated invitation payload after expiry/revocation; it creates no career-content retention period.

The implementation consumes the [30-day retained-read policy](https://github.com/HenriqueMichelini/careeros/issues/258#issuecomment-6550859196), [unused-excerpt decision](https://github.com/HenriqueMichelini/careeros/issues/258#issuecomment-6324889579), [deleted-detail decision](https://github.com/HenriqueMichelini/careeros/issues/258#issuecomment-6324928436), [recovery-copy ceiling](https://github.com/HenriqueMichelini/careeros/issues/258#issuecomment-6324994944), [deletion/earliest-end contract](https://github.com/HenriqueMichelini/careeros/issues/288#issuecomment-6645200501), and separately retained [financial authority](https://github.com/HenriqueMichelini/careeros/issues/283#issuecomment-6645082417). The frozen interface is `docs/never-paid-cleanup-contract.md`; operational limitations are in `docs/managed-never-paid-cleanup-local.md`.

The bounded worker uses current account/pilot authority and original timestamps. It inventories bound Draft content even when the canonical Profile row is absent, fences before purge and leaves failures closed. Ordinary access end preserves the selected 30-day read/export period while generation remains denied. Unknown inherited access ends create no fabricated retrieval deadline. Explicit deletion remains immediate and independent.

## Current checks

- Focused PostgreSQL/race: 20 passing events, zero failures; one explicit fresh-child helper skip in the parent process. Event counts include parent/package events and are not a count of unique cases.
- Node: 452 passing tests across 32 separately executed files. This is composite coverage: original successful files plus capable-sandbox reruns of five subprocess-limited files, and the corrected bilingual retention fixture. It is not one successful original aggregate.
- TypeScript, account/pilot build and scoped vet pass. Default vet retains 15 diagnostics; the summary records exact baseline comparison and excluded analyzers. No declared-Go-1.24 execution claim is made.
- Six final actual React → Vite → authenticated Go/private PostgreSQL browser cells pass: EN/PT notice and removal metadata, same-owner late export cancellation, and career-read denial with metadata/pending recovery separation.
- Relevant Go/race passes with 1,422 passing events, zero failures, 14 skip events (helper/optional/no-test packages), all 37 canonical admission IDs passing and no failed packages.
- Full Go/race terminates with exit 1, 2,590 passing events and 26 failure events: the same 25 Evaluation Lab test/subtest paths plus its package failure as immutable base `63b20b29`. There are no new or missing failure paths. All six Lab/CV/dispatch/HTTP dependency trees are unchanged. The exact paths, commands, raw counts and retained baseline chain are in the companion summary; no new base aggregate was run. The full suite is not green.

## Recovery, loss and cleanup controls

The permanent tests cover exact deadline boundaries and post-lock trusted time; mixed delivered/undelivered/orphan content; stable cleanup and notice identities; known email failure versus unknown durable send intent; bounded retry; actual default/inherited role denial; a normally built local CLI; reissue with fresh eligibility; retained invitation-deny markers; raw Profile value preservation; and sealed minimal recovery metadata. No callback or inference occurs inside a database transaction.

The physical schedule uses fsync-enabled disposable PostgreSQL, an immediate stop/restart and a fresh child server process. Exact status bytes match before and after restart through the public canonical/account-alias paths. The synced witness contains five controlled provider-stage callbacks and two fake-email attempts. Replayed work adds no callbacks. Primary Profile/Draft content is absent, the already-delivered customer effect remains one, and the unknown obligation retains 36,688 microUSD and one remote occupancy. This is physical local proof, distinct from rebuilding a Harness and from hosted restore assurance.

Excerpt cleanup preserves untouched raw Fact values, numeric lexemes, Unicode/NUL and unrelated evidence. Active/current references determine retained excerpts; historical invalidated links do not independently retain bytes. Other facts remain while affected support is invalidated. No semantic text matching or global historical-Draft deletion is performed. Minimal pruning/invitation/notice/residual handoffs preserve original anchors and denial identity without retaining removed source text.

Recovery-copy obligations use the original durable fence plus 720 hours, never retry/restart time. Recipient deadlines remain null; fixture pending/failed/verified evidence is not a provider/legal erasure guarantee. Essential expense, remote occupancy, replay, customer terminal markers and necessary keys are not timer-deleted or automatically compacted.

## Browser scope and failed checkpoints

The six final browser cells use the real application and router, with private SQL clock transitions, fake email and an injected account-scoped pending checkpoint. At read-window end, metadata remains available while protected children/recovery downloads are withheld. A previously authorized native export response released after the known fence creates no new object URL. Independently entered pending wording and opaque identities survive for deliberate authorized recovery; no automatic Save, activation, cleanup POST or inference is introduced. Previously downloaded customer-held files are outside centralized erasure control.

Permanent RED evidence includes the first missing implementation/route, invitation-only payload retention, old invitation denial lost after reissue, late native export after known removal, career-only 403 blocking metadata, header pending-recovery access during denial, and the corrected direct shared-kernel invitation-marker bypass. Their later GREEN receipts remain separate.

An earlier invitation test used an unsupported workflow name and appeared green without proving the gate. It is excluded from the positive proof. The corrected test first establishes ordinary admission, then demonstrates the restored-mutable-state marker denial. Invalid orphan fixture setup, PostgreSQL type inference/JSONB-NUL experiments and browser selector/navigation failures are recorded as fixture or introduced implementation failures, not policy evidence. The final implementation uses no JSONB conversion of raw career documents.

The first isolated Node aggregate failed five files with `spawnSync ... EPERM`; its raw output is retained. Capable-sandbox reruns exposed two real EN/PT expectations that retained orphaned excerpts for historical traceability. Those expectations conflict with selected D03 and were corrected to assert removal while preserving unrelated evidence, stale-result denial, invalidated links and independent accepted CV state. The corrected file passes all nine tests. No production change was needed for those assertions.

## Remaining boundaries

The local CLI, fake email and residual observers do not configure a scheduler, SMTP, real recipient API or hosted disposal. New boolean predicates use validated/schema-qualified application relations, a fixed safe search path, PUBLIC revocation and narrow EXECUTE permissions; raw metadata/content table access remains denied to AI runtime/reconciliation roles. Role fixtures and immutable security review assess these claims separately.

No real provider access, spending, grants, participants, configuration, deployment, hosted recovery, GitHub mutation, push, PR, merge or closure was performed. The linked-worktree managed scanner remains unsupported (`Review changes requires a non-bare Git worktree with a resolvable HEAD.`); no completed scanner is claimed. Independent Standards, Spec and read-only security review of the final immutable full delta remain required before local success.

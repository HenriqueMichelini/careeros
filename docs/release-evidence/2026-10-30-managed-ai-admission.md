# Local managed AI admission and dispatch verification

30 October 2026; verification updated 31 October, America/Sao_Paulo. Base: `f48c60a178affa2bfe1988d5225dc5aebcf02daf`, the verified PR #274 documentation merge. Branch: `codex/managed-ai-admission-local`. Worktree: `/home/henriquemichelini/workstation/careeros/.local-worktrees/ai-admission`.

This records the separately authorized local implementation of the [reviewed contract](../research/careeros-managed-ai-admission-design-2026-10-29.md), not completion of GitHub #249/#258 or launch readiness. Their current bodies and decisions were refreshed read-only on 31 October and match the prior snapshots; criteria, blockers and #52 requirements were unchanged. All financial values, customer units, grants, clocks, reconciliation evidence, providers and recovery authorities here are explicitly synthetic. No provider client is installed in the admission laboratory.

## Implemented scope

- An isolated Go/PostgreSQL authority binds an account-scoped creation key, Action, workflow, stage, attempt and dispatch to keyed canonical inputs and the authoritative Profile where applicable. Each Action freezes a server-owned execution registry and persisted policy envelope; recovered Actions cannot acquire changed prompt/schema/model/tier/rate identities or operating limits. Exact replay cannot resend; changed or unverifiable replay fails closed. Known-failure retries create bounded new attempts without erasing prior obligations. Explicit clarification children share the finite stage allocation cap with provider and provider-free stages.
- Atomic account/global/provider reservations precede every controlled provider send. Database-clock session, grant and policy checks occur after relevant lock waits and again at delayed handoff. Durable intent is one-way: cancellation, timeout, ambiguous commit, process loss and local worker completion preserve possible expense and remote occupancy. The controlled adapter cannot follow redirects, retry or fall back to its caller's network client.
- Provider observations are append-only, bounded non-content metadata, separate from expense evidence. Missing, invalid and unsupported categories/rates stay explicit; cached/reasoning subsets are not counted twice. Estimates, confirmed bills, conservative floors and maximum breaches remain distinct, with versioned idempotent reconciliation and independently authorized remote-terminal proof.
- Synthetic customer reservation/delivery/Accept/export/cancellation/deadline transitions are separate from provider expense. Only Application Draft Actions can reserve or consume synthetic Draft units, with the invariant also enforced in SQL. Actual workflow parsers and support reviews keep their existing quality consequences. No runtime constructor installs a synthetic controller or grants, persists Draft content or performs customer deductions.
- Provider-free ingestion plan/assemble stages receive durable replay bindings. Legacy, v1 and v2 ingestion retain their protocol checks and reached-leg accounting. All five authenticated workflow constructors and actual Function aliases have positive and negative controls.
- The fixture database uses independently authenticated restricted runtime/reconciliation roles and a denied routine-support role. Runtime cannot reconcile bills, release possibly-sent holds, forge delivery effects or escape via RESET ROLE. Ordinary diagnostics exclude ledger linkage and content. Account/Profile deletion retains financial and replay protection without career payload.
- An external in-memory recovery gateway models the opening prerequisite. Closure invalidates captured capabilities permanently; SQL rewind, epoch changes and endpoint changes cannot reopen it. Tests restore actual older SQL rows, reconcile missing obligations explicitly, and require complete synthetic retirement/exclusion proofs before pruning replay detail. Protected HTTP result writes remain tracked until they return: a blocked writer prevents successful drain, reopening and pruning. Cancellation cannot recall bytes already handed to a writer; these checks establish no hosted or network-delivery fence.

## Verification and limits

The reproducible entrypoint is `rtk proxy python3 scripts/test-ai-admission.py --evidence <new-local-directory>`. It creates a private Unix-socket-only disposable PostgreSQL cluster, runs Go tests with race instrumentation and fails if any A01–A37 case is absent, skipped or failed. The [acceptance matrix](ai-admission-acceptance-matrix.md) specifies the independent arithmetic and minimum proof for each case. Case labels are supplemented by actual SQL, independent-process, real-handler and explicit synthetic-state assertions.

**Execution: all 37/37 acceptance cases passed, with zero acceptance skips and zero failed packages, under Go race instrumentation and disposable PostgreSQL.** The [machine-readable receipt](ai-admission-acceptance-results.json) records the final consolidated run, including the review regressions and missing-authority cases. Raw run logs remain local under the sibling `ai-admission-evidence` directory, outside the source commit. Independent Standards, Spec and security reviews are separately bound to the final immutable base/head; this document records execution rather than claiming those reviews in advance.

Frontend: `npm test` passed 360/360 with zero skips; `tsc --noEmit` passed; `npm run build` passed with its existing `__dirname` future-loader and large-chunk warnings. Shared full-value Go/TypeScript Profile projection parity ran as part of the frontend suite; no projection code changed. Vite used a private cache, and generated `dist` files were restored in this worktree.

Full Go comparison: the baseline and implementation runs have the same 13 top-level failures (11 failing subtests) confined to `internal/evaluationlab` process-runner tests. They report `source_changed` / approved source snapshot preparation unavailable; no new failing test name appeared. The admission runner excludes that unrelated package and does not alter its approved-source manifest or suppress its requirements. `GOFLAGS=-buildvcs=false` only disables VCS build stamping; it changes no assertions or selected tests. These inherited failures remain failures.

The 13 inherited top-level failures are:

```
TestProcessRunnerExecutesAllowlistedGoContractsWithoutInference
TestProcessRunnerCannotReadHostSentinelsOrReachHostLoopback
TestProcessRunnerCancellationStopsDetachedChildAndGrandchild
TestProcessRunnerDeadlineRetainsPartialTreeAndUnreachedTest
TestProcessRunnerBoundsOutputAndStopsFollowingTests
TestProcessRunnerReusesSectionBrowserHarnessWithScreenshotEvidence
TestProcessRunnerExecutesExistingNodeCanonicalContracts
TestProcessRunnerCrashRetainsDistinctOutcome
TestProcessRunnerUsesOfflineFieldHelpers
TestProcessRunnerCDPPDFIsDistinctAndSurvivesExport
TestProcessRunnerExistingWorkflowSelectorsExecute
TestProcessRunnerAssertionRegressionCompletesWithFail
TestProcessRunnerAdditionalBrowserInventoryRetainsOutcomes
```

## Settled policy, implementation choices and operating blockers

The selected policy is unchanged: conservative unknown exposure; provider-specific accounting; restricted non-content ledger access; and exactly 168 elapsed hours from durable dispatch intent as eligibility for reviewed conservative financial closure. The durable-commit bound is conservative when commit timing is uncertain. Eligibility never automatically closes, erases, releases budget/capacity or changes customer rights. No other lifecycle clock is inferred.

The first independent Standards/Spec review identified four gaps: combined child/stage limits, non-Draft use of Draft units, missing conservative-closure audit evidence, and unfrozen Action execution policy. A security check also reproduced a pending-result-write omission from the recovery drain. A later exact-head Spec review found that the clarification-parent validator was unwired: a valid child could execute another claim's signed request. Repairs enforce child scope before reservation and intent, deny unrelated provider-free stages, and add both real-handler and direct-authority regressions. The glossary naming and duplicated current-authority checks identified by Standards were corrected. Conservative closure now atomically retains an immutable reviewer/disposition/amount/policy/time record through compaction and namespace-marker pruning. No acceptance expectation was relaxed; final independent reviews must use the corrected immutable head.

Routine choices include keyed canonical JSON binding, checked integer SYN arithmetic, account-before-session locking, sorted shared-scope locks, one-use intent claim, immutable gateway leases, explicit child identities, column grants and guarded SQL release. They are local safeguards, not additional owner approval questions or production infrastructure selections. There was no weakening of the approved acceptance outcomes.

Remaining live operating prerequisites include provider/category/model/tier/rate and finite-maximum fit; all numerical gross-spend, token/input, concurrency, stage, dispatch and attempt limits; actual grant authority; trusted commit/clock health; separately authorized reconciliation/adjustment and provider-terminal evidence; retention/deletion/replay/key/backup horizons; actual gateway/exclusion/recovery authority; and recoverable Draft delivery/customer-rights policy. #52 quality, real-user usefulness, accepted-result cost and hosted evidence remain outstanding. These tests select none of those limits or policies.

Managed live execution, actual ledger collection/cleanup, actual grants, durable content delivery, deductions, provider-account access, spending, hosted configuration and deployment remain disabled. No push, PR, comment or remote issue mutation was performed for this implementation. The next owner gate is separate approval to publish this local branch for review.

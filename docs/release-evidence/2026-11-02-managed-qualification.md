# Managed qualification verification — 2 November 2026

Local synthetic evidence only. No provider/account access, network provider request, secret access, real participant/grant, spending, remote mutation or deployment occurred. Independent Standards/Spec/security review of the final immutable commit remains a separate coordinating step.

Base: local integration `ae88bca69d9dfbf07c7cdeb8acc8cb6444d33b66`, tree `7e86353d5a92dbfc975344636ad1d5b80fcde6b8`, containing both merged main `bafe318` and preserved #276 `448d06f`. The original dirty research checkout and completed admission worktree were preserved. The final commit containing this evidence is the review checkpoint; no product-source changes are made by documentation generation.

## Passing evidence

- Final relevant Go package set, actual disposable PostgreSQL, `-race -count=1`: **961 passing test events**, zero failed packages. Canonical admission-runner summary independently confirms **37/37 acceptance case IDs**, zero unmet/not-executed cases. Events are not a count of independent acceptance cases. Raw logs: `/tmp/careeros-277-qualification-check-1793657563965675900/go-tests.jsonl`; receipt `/tmp/careeros-277-final-admission-cases.json`. This run follows the final numeric-envelope fingerprint and qualification retry-key hardening. Two opt-in/helper tests skip by design: `TestJourneyDemoServer` and `TestPilotRestartChild`; their parent browser/process tests run separately or pass. Package-level no-test events are not missing acceptance cases.
- New qualification integration tests exercise actual account/grant/router/qualification adapters: canonical/direct/account aliases; caller credential and stale/missing/revoked/expired authority denials; immutable changed input/Profile; whole complete-request bounds; known-failure retry idempotency; crash after durable intent with zero resend; five-action rolling limit with unchanged five-Draft allowance; account/global active and remote capacity races; reviewed gross-money last-global-money race. The later test-only fixture mutex and accurate expiry-test name receive a final focused check noted below.
- New #277 restart coverage reconstructs a Harness and its pools against retained PostgreSQL. It is **not a fresh OS process or physical database restart**. The existing shared-authority process cases `TestA01_DuplicateAcrossProcesses`, `TestA04_AtomicFinalBudgetAcrossProcesses`, `TestA07_ReservationCrashAndProcessAbandonClaim`, `TestA09_ProcessCrashAfterInvocationBeforeObservation`, `TestPostgresSurvivesAllApplicationProcessRestarts` and `TestPilotApplicationProcessRestart` actually reran and passed in the 961-event package set. No physical PostgreSQL restart mode was enabled.
- Final frontend: **365/365 tests pass**, including the shared **44-case full-value Go/TypeScript Profile projection corpus** and new stable-key/GET-only status tests. Log `/tmp/careeros-277-final-frontend.log`. An initial sandboxed invocation failed with `spawnSync go EPERM`; it is an environment failure, not a green test run. The permitted local-subprocess invocation passes.
- Final TypeScript `npx tsc --noEmit` passes. Final pilot build passes, output `/tmp/careeros-277-final-pilot-dist`, log `/tmp/careeros-277-final-build.log`. Existing Vite configuration/chunk-size warnings remain.
- Final EN/PT local headless Chromium actual App/AccountGate/PilotAccess/HomePage composition passes: no key controls or credential headers, explicit stable creation/retry identity, exact unchanged retry body, GET-only recovery/no automatic inference, unsupported Draft closure, honest unavailable transient result status, deliberate new Action and spending pause preserving the prior job understanding. Browser transport is synthetic; it is independent from real PostgreSQL protocol evidence. Evidence directories `/tmp/careeros-account-browser-IFvfAH` and `/tmp/careeros-account-browser-0ikshS`.
- Scoped `go vet -composites=false ./internal/accountapi ./internal/aicontrol ./backend/qualification-gaps` passes. Unmodified default composites diagnostics are retained below.
- Final test-only change serializes a shared fake responder in the money race and renames the expiry-only test accurately. Its focused PostgreSQL/race check passes **2 test events**, zero skips/failures, at `/tmp/careeros-277-qualification-check-1793657813314184380`. Product Go runtime is unchanged from the 961-event/all-37-case checkpoint.

## Full Go suite is not green

The full `go test -race -json -count=1 -timeout=180s ./...` with a fresh matching loopback `careeros_test` database exits **1**, with **2,199 passing test events** and only Evaluation Lab ProcessRunner failures: 13 top-level tests plus 11 subtests. Log `/tmp/careeros-277-qualification-check-1793657366347079020/go-tests.jsonl`. This checkpoint precedes only the final numeric execution-envelope/retry-key hardening and subsequent UI fixes; the relevant final Go package set and final UI checks above cover those changes.

A fresh `-race` ProcessRunner run on the immutable **combined pre-edit tree**, archived at `/tmp/careeros-277-prerequisite-source`, exits 1 with the **identical 24 failing test paths**. Log `/tmp/careeros-277-base-processrunner.jsonl`. These are current baseline comparisons, not reused #276 historical failures. The approved source manifest was not changed or reapproved. The source-snapshot guard reports `source_changed`; derivative assertions include a child tree never reached and absent offline/browser evidence.

<!-- Exact failure paths and diagnostics appended by local evidence extraction. -->

Exact retained Evaluation Lab paths (package `internal/evaluationlab`):

- `TestProcessRunnerAdditionalBrowserInventoryRetainsOutcomes`
- `TestProcessRunnerAdditionalBrowserInventoryRetainsOutcomes/browser-application-reuse`
- `TestProcessRunnerAdditionalBrowserInventoryRetainsOutcomes/browser-canonical-edit`
- `TestProcessRunnerAdditionalBrowserInventoryRetainsOutcomes/browser-enrichment`
- `TestProcessRunnerAdditionalBrowserInventoryRetainsOutcomes/browser-ingestion`
- `TestProcessRunnerAssertionRegressionCompletesWithFail`
- `TestProcessRunnerBoundsOutputAndStopsFollowingTests`
- `TestProcessRunnerCDPPDFIsDistinctAndSurvivesExport`
- `TestProcessRunnerCancellationStopsDetachedChildAndGrandchild`
- `TestProcessRunnerCannotReadHostSentinelsOrReachHostLoopback`
- `TestProcessRunnerCrashRetainsDistinctOutcome`
- `TestProcessRunnerDeadlineRetainsPartialTreeAndUnreachedTest`
- `TestProcessRunnerExecutesAllowlistedGoContractsWithoutInference`
- `TestProcessRunnerExecutesExistingNodeCanonicalContracts`
- `TestProcessRunnerExistingWorkflowSelectorsExecute`
- `TestProcessRunnerExistingWorkflowSelectorsExecute/go-canonical`
- `TestProcessRunnerExistingWorkflowSelectorsExecute/go-cv`
- `TestProcessRunnerExistingWorkflowSelectorsExecute/go-ingestion`
- `TestProcessRunnerExistingWorkflowSelectorsExecute/go-job-application`
- `TestProcessRunnerExistingWorkflowSelectorsExecute/node-artifact-review`
- `TestProcessRunnerExistingWorkflowSelectorsExecute/node-ingestion`
- `TestProcessRunnerExistingWorkflowSelectorsExecute/node-job-reuse`
- `TestProcessRunnerReusesSectionBrowserHarnessWithScreenshotEvidence`
- `TestProcessRunnerUsesOfflineFieldHelpers`

Baseline default `go vet` has the same 14 existing unkeyed `aiadmission.Call` composite-literal diagnostics in unchanged `internal/accountapi/ai_admission_test.go`, at lines 117 (7), 172 (2), 174 (2), and 177 (3). Fresh baseline log `/tmp/careeros-277-base-vet.log`. These diagnostics were not repaired as unrelated work; the explicit composites-disabled check passes.

The archived baseline was independently checked against all 2045 tracked Git blobs of integration `ae88bca`; mismatches: 0.

# #276 local invited-access verification — 31 October 2026

Base: `f48c60a178affa2bfe1988d5225dc5aebcf02daf`. Worktree: `/tmp/careeros-issue-276`, branch `codex/issue-276-invited-access`. This records synthetic local source verification; independent Standards/Spec/security review is pending at this checkpoint.

| Check | Result |
| --- | --- |
| `go test -race ./internal/accountapi ./internal/accountstore ./internal/auth ./cmd/pilot-local -count=1`, disposable PostgreSQL 18.4 | PASS; complete account/auth suites, nine pilot behavior tests plus subprocess helper (ordinary top-level helper invocation skips) |
| Dedicated `CAREEROS_PILOT_RESTART_DATABASE=1` / `TestPilotApplicationProcessRestart` | PASS; actual restart of the dedicated disposable PostgreSQL container, then a new application process recovered grant, Profile and unchanged activation replay |
| `node tests/pilot-access.test.mjs` | PASS, 3 client cases |
| `node tests/pilot-browser.check.mjs` | PASS, EN/PT actual React/Chromium StrictMode, synthetic transport |
| `node tests/account-browser.check.mjs` | PASS, retained legacy account/session recovery |
| `npx tsc --noEmit` | PASS |
| `go vet ./internal/accountapi ./internal/accountstore ./cmd/pilot-local` | PASS |
| `npm test` with required local subprocess permissions | PASS, 363 tests, 0 failures, 0 skips |
| Pilot Vite production build to `/tmp/careeros-276-pilot-build-final` | PASS; inherited large-bundle/config-loader warnings |
| `go test ./...` | FAIL only in `internal/evaluationlab`; same 13 top-level failure names reproduced in a fresh immutable-base worktree |
| `git diff --check` | PASS |

The first sandboxed npm invocation failed in four existing files because its Go subprocesses received EPERM/VCS errors. The permitted rerun completed all 363 tests. No failed attempt is counted as passing.

Full Go is not green. The matching base/head Evaluation Lab failures are:

- `TestProcessRunnerExecutesAllowlistedGoContractsWithoutInference`
- `TestProcessRunnerCannotReadHostSentinelsOrReachHostLoopback`
- `TestProcessRunnerCancellationStopsDetachedChildAndGrandchild`
- `TestProcessRunnerDeadlineRetainsPartialTreeAndUnreachedTest`
- `TestProcessRunnerBoundsOutputAndStopsFollowingTests`
- `TestProcessRunnerReusesSectionBrowserHarnessWithScreenshotEvidence`
- `TestProcessRunnerExecutesExistingNodeCanonicalContracts`
- `TestProcessRunnerCrashRetainsDistinctOutcome`
- `TestProcessRunnerUsesOfflineFieldHelpers`
- `TestProcessRunnerCDPPDFIsDistinctAndSurvivesExport`
- `TestProcessRunnerExistingWorkflowSelectorsExecute`
- `TestProcessRunnerAssertionRegressionCompletesWithFail`
- `TestProcessRunnerAdditionalBrowserInventoryRetainsOutcomes`

These failures reject the pre-existing approved-source snapshot/manifest (`source_changed`); detached-tree and browser assertions then fail because their guarded harness never executes. The manifest was not edited, weakened or reapproved. Exact local logs are `/tmp/careeros-276-full-go.log` and `/tmp/careeros-276-base-evaluationlab.log`; the failure-name lists match.

Pilot tests exercise two-account substitution, missing eligibility, stable activation replay, twelve concurrent redemption requests across six identities, exactly five permanent slots, no slot reset after revocation, changed command replay, actual local operator command with live opaque-session/role checks, expiry/reissue, Profile read versus Save/generation during the 30-day window, current session/grant after blocking locks, and late fake result suppression. All data and providers are synthetic. The synthetic operator CLI was exercised only against disposable local PostgreSQL.

`accountapi.NewPilot` remains an explicit injected local composition. Production `FromEnv` and hosted Function composition are unchanged; no actual participant, role, eligibility decision, invitation or grant has been provisioned in an operational system. Numerical provider/action/attempt/spend bounds, managed dispatch, customer-consumption settlement, delivered Draft storage/retrieval, notices and deletion execution remain their separate downstream work. There was no provider call, spend, push, PR, merge, issue mutation, deployment or launch.

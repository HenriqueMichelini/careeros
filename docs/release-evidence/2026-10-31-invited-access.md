# #276 local invited-access verification — 31 October 2026

Base: `f48c60a178affa2bfe1988d5225dc5aebcf02daf`. Worktree: `/tmp/careeros-issue-276`, branch `codex/issue-276-invited-access`. This records synthetic local source verification. Immutable implementation checkpoint `961b825867aceaded4d00b86846d537ea063e87d` received independent Standards review (zero hard violations, one nonblocking duplication heuristic in the Profile transactions), Spec review (zero findings) and security review (zero confirmed findings; all 19 changed files inspected and the three client cases run). The subsequent combined-browser follow-up changes only test fixtures, a browser check and documentation; final-head delta readbacks are coordinated separately.

| Check | Result |
| --- | --- |
| `go test -race ./internal/accountapi ./internal/accountstore ./internal/auth ./cmd/pilot-local -count=1`, disposable PostgreSQL 18.4 | PASS; complete account/auth suites, nine pilot behavior tests plus subprocess helper (ordinary top-level helper invocation skips) |
| Dedicated `CAREEROS_PILOT_RESTART_DATABASE=1` / `TestPilotApplicationProcessRestart` | PASS; actual restart of the dedicated disposable PostgreSQL container, then a new application process recovered grant, Profile and unchanged activation replay |
| `node tests/pilot-access.test.mjs` | PASS, 3 client cases |
| `node tests/pilot-browser.check.mjs` | PASS, EN/PT actual React/Chromium StrictMode, synthetic transport |
| `node tests/account-browser.check.mjs` | PASS, retained legacy account/session recovery; rerun after the combined fixture extension |
| `node tests/pilot-account-browser.check.mjs` | PASS, combined actual AccountGate/PilotAccess/StoreProvider StrictMode in EN/PT; real session-expiry timer, same-account reauthentication, account-switch isolation, stable grant expiry, no automatic activation/Save, exact explicit Save Retry |
| `npx tsc --noEmit` | PASS |
| `go vet ./internal/accountapi ./internal/accountstore ./cmd/pilot-local` | PASS |
| `npm test` with required local subprocess permissions | PASS at implementation checkpoint, 363 tests, 0 failures, 0 skips; unchanged `.test.mjs` suite not repeated for the test-only browser follow-up |
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

## Combined browser follow-up

The initial combined case failed because the existing account browser transport did not enable its pilot composition. The fixture now opts into the actual `AccountGate pilot` path in StrictMode, preserves independent per-account grant expiry, records activation bytes and all unsafe HTTP requests, and retains the original non-pilot default. No product-source defect or policy/runtime change was required.

Both EN/PT cases pass with: one explicit activation despite lost response; pending Save wording preserved through the actual AccountGate session-expiry timer and same-account reauthentication; unchanged displayed grant expiry; the other account left unactivated without exposing the pending wording; no new unsafe request from focus, expiry, reauthentication or account switching; and one explicit Retry using exactly the original Save bytes/base/operation identity. The explicit activation plus first Save account for two unsafe requests; only the deliberate Retry increases that count to three. Legacy Profile reads remain zero.

Final combined logs: `/tmp/careeros-276-combined-browser-final.log`, EN evidence `/tmp/careeros-account-browser-eW4vGs`, PT evidence `/tmp/careeros-account-browser-zQ8Zis`. The existing non-pilot browser rerun passed with `/tmp/careeros-account-browser-BM2CF4`. TypeScript and `git diff --check` pass for the follow-up. Go/runtime code is unchanged from the reviewed implementation checkpoint; its PostgreSQL/race/restart and inherited full-suite limitations above are reused without being relabeled as fresh final-head runs.

# Managed ingestion verification — 3 November 2026

Local synthetic evidence only: fake providers, synthetic accounts/content and private disposable PostgreSQL. No actual provider request, provider/secret access, spending, real invitation/grant/admission, remote GitHub mutation, configuration, deployment or recovery operation occurred. Independent Standards/Spec/security review remains required; this document is a review checkpoint, not a success or launch claim.

Base: `9a759e5b0ae99e2328f535a2168fb9d3ae9d544c`, tree `f6e89bf413b967cd7208b3243edbaeb3676b95c6`, on the existing cumulative branch `codex/batch-277-qualification-20261102` at `.local-worktrees/qualification-277-20261102`. The commit containing this report freezes the tested runtime plus documentation for independent review. All prior completed commits remain ancestors; the dirty research checkout and other worktrees are untouched.

Contract and route inventory: [managed-ingestion-local.md](../managed-ingestion-local.md). Owner policy links are exact frozen selections, not current-price lookups. Evidence paths under `/tmp` are ephemeral raw logs; the durable summary below retains outcomes and exact failure names even if those temporary files are later removed.

## Final passing checks

- Relevant Go package set, actual loopback `careeros_test` PostgreSQL, `-race -count=1`: **994 passing test events**, zero failures, **37/37 shared acceptance IDs**. Events are not a count of independent acceptance cases. Raw log `/tmp/careeros-278-check-zi63wjzs/go-tests.jsonl`, canonical runner summary `/tmp/careeros-278-check-zi63wjzs/acceptance-results.json`. The two named opt-in/helper skips are inherited `TestJourneyDemoServer` and `TestPilotRestartChild`; their parent/process coverage runs. Package-level no-test events are distinct from skipped tests/cases. All37 IDs pass with no unmet/missing case.
- Runtime is unchanged after that run. One subsequent **test-only assertion extension**, focused PostgreSQL/race **1 passing event**, verifies a second valid cleared account cannot read A's status or borrow A's signed receipt through canonical/direct/account aliases. It sends zero providers. Log `/tmp/careeros-278-check-9p_ereuw/go-tests.jsonl`. Earlier wrong-account-header coverage remains.
- The matrix invokes actual public signed account/grant/staged ingestion routes/adapters: provider-free plan/assembly, whole-original extraction/reconciliation and four answer-bound clarifications, signed receipt replay/mixed-Action denial, expanded-source plan exceeding20 extraction batches before any extraction, caller envelope/header controls, current/expired/revoked/suspended/session authority, full-request and cumulative token limits, stage counts including failed claims, and all aliases.
- Fresh failed-clarification regression: unchanged failed answer is202/status-only; changed answers remain same Action/attempt and count the failed child toward four; explicit stable unchanged Retry reaches attempt2 and its replay never resends. Initial focused log `/tmp/careeros-278-check-52g8h_ne/go-tests.jsonl` (3 events); these cases also pass in the final994-event run. The final client suite has4 passing cases, including remount and account/field isolation.
- Absolute foreground deadline regression was first RED for claimed-zero-dispatch replay, post-claim expiry and expiry between provider legs; all three pass after the post-lock/pre-send fix, including the final run. The first persisted non-clarification request is not reset by stages/status/replay. Explicit focused clarification retains its separate foreground operation and unchanged real receipt expiry.
- Saved Profile comparison is inside Open's account-locked transaction. A request waiting on the account lock sees the newly changed valid canonical Profile and returns409 before creating a stage/sending. The final994-event run includes the valid fixture. Standalone corrected focused log `/tmp/careeros-278-check-9l6pzhym/go-tests.jsonl` (1 event).
- Cross-workflow compatibility seeds qualification Actions/unknown holds **before** opening ingestion composition. Same global/account occupancy and expense remain, inherited qualification duplicate still returns202/no resend, and a two-account race for the shared pilot's final1,344microUSD admits exactly one TypeSafe intent, no OpenAI leg, with total10,000,000. There is no copied pool or claim that an entire297,012microUSD attempt was reserved upfront.
- New recovery test reconstructs a Harness against retained PostgreSQL after durable intent, then status remains unknown/nonretryable with zero resend and retained money/occupancy. It is **not a fresh OS process or physical PostgreSQL restart**. Existing shared process cases rerun in the relevant package set; physical database restart was not enabled.
- Restricted-ledger sentinel checks find neither private original career text nor answer text in Action/stage/child/dispatch/observation/evidence rows. Fake terminal proof is distinct from incomplete usage/expense: occupancy may release while the conservative financial maximum remains.
- Full Node suite: **369/369 tests pass**, including the shared **44-case full-value Go/TypeScript Profile projection corpus**, ingestion protocol/review/Save regressions and4 new managed ingestion cases. TypeScript `npx tsc --noEmit` passes. Pilot build passes to `/tmp/careeros-278-final-pilot-dist` (log `/tmp/careeros-278-final-build.log`); existing Vite configuration/output/chunk-size warnings remain. Go `gofmt -l` on all touched Go paths is empty; `git diff --check` passes.
- Fresh actual App/AccountGate/PilotAccess/Profile EN/PT Chromium passes: one Action across plan/extract/assembly/reconcile; no key controls/caller credential headers; zero Save before deliberate selection/Save; acknowledged Save retains evidence/links; GET-only recovery and no career content in session storage. Final normal EN/PT directories `/tmp/careeros-account-browser-WiGK7W`, `/tmp/careeros-account-browser-5eliGi`. Final unknown→Profile navigation/remount→GET-only recovery→known-failure deliberate retry EN/PT directories `/tmp/careeros-account-browser-Ddt1mt`, `/tmp/careeros-account-browser-QUrK9u`: exact stage body/root key reused, no automatic plan/inference/Save. Browser responses are synthetic and separate from PostgreSQL protocol evidence.
- Scoped `go vet -composites=false ./internal/accountapi ./internal/aicontrol ./backend/profile-ingestion ./internal/workflowauth` passes. Default composites diagnostics are retained below.

## Full Go is not green

Final compatible `go test -race -json -count=1 -timeout=300s ./...` against fresh private loopback PostgreSQL exits1 with **2,232 passing test events**. Its only failing package is `internal/evaluationlab`, with13 top-level ProcessRunner failures plus11 subtests. Raw final log `/tmp/careeros-278-check-o19m84hn/go-tests.jsonl`.

A fresh `-race` ProcessRunner run on the exact immutable **9a759e5 base**, archived at `/tmp/careeros-278-base-9fae0iga`, has exactly the same24 failure paths. Archive verification checked all**2,058 tracked Git blobs**, zero mismatches, before running the baseline. Machine-readable comparison `/tmp/careeros-278-lab-baseline-comparison.json` confirms identical sets. Baseline log `/tmp/careeros-278-base-processrunner.jsonl`. Both retain `source_changed` snapshot denial and derivative missing/unreached child/offline/browser outcomes. No source manifest/hash approval was weakened or updated.

Exact retained Lab paths:

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

Default `go vet` retains the exact same14 inherited unkeyed `aiadmission.Call` literals in unchanged `internal/accountapi/ai_admission_test.go`: line117(7),172(2),174(2),177(3). Fresh exact-base vet reproduces them. The explicit composites-disabled scoped check passes; default vet is not described as green.

## Earlier nonfinal checkpoints retained distinctly

- Before the final fixes, the broad package run `/tmp/careeros-278-check-bi5p1m6k` had682 passing events/all37 IDs but failed the newly introduced backend test import cycle and stale-supplied-Profile negative. The corresponding early full run `/tmp/careeros-278-check-0xtjokz1` had1,920 passing events plus those introduced failures and the24 Lab paths. Neither aggregate passed. The runtime protection was extracted unchanged into neutral `internal/workflowauth`, so actual accountapi and inherited backend admission tests use the same implementation without weakening assertions/case IDs. Ingestion snapshot equality was added within the existing account-locked authority transaction.
- `/tmp/careeros-278-check-yk7ayeak` reached all37 IDs/992 passing events but failed a new lock-race test because its synthetic document revision/reference fixture was invalid. `/tmp/careeros-278-check-ru9f5hk_` additionally had a transient10-second `TestCVRetainedReplayUsesNoDispatchesAndKeepsSemanticFailures` deadline failure under concurrent broad checks, besides that invalid fixture and24 Lab paths. These are not relabeled baseline-only runs. The valid canonical race fixture passes; final compatible relevant/full runs above contain neither failure.
- The credential-envelope test was RED at `/tmp/careeros-278-check-env80jao` because top-level caller controls were ignored. Explicit control-field rejection makes it green in the final package run. This rejects envelope authority controls without inspecting valid career-text values.

## Existing bilingual browser regression

The separate inherited `tests/submission-journey.check.mjs` run passed the baseline empty/populated/duplicate/invalid/partial/clarification/stale flows, plus integrated EN desktop/mobile rich-review and failure/keyboard cells. It then stopped progressing after `save-failure-pt-BR-1440.png` at the PT desktop Save-retry boundary. Owned local CDP inspection did not respond within5seconds. The run was terminated with exit130; it is **incomplete**, not a pass or a confirmed product regression. Partial screenshots remain under `/tmp/careeros-submission-journey-1IKAk6`. The complete new managed EN/PT browser passes above are independent evidence.

Both affected PT matrix cells pass separately on unchanged product source and unchanged assertions, using an owned temporary copy of the inherited harness with bounded CDP/180-second terminal timeouts: desktop rich-review/Save-retry and failure/keyboard (`/tmp/careeros-submission-journey-EzA6On`, log `/tmp/careeros-278-legacy-pt.log`), and mobile rich-review/Save-retry and failure/keyboard (`/tmp/careeros-submission-journey-mifbck`, log `/tmp/careeros-278-legacy-pt-mobile.log`). These complete the individual EN/PT matrix-cell coverage; they do not turn the original interrupted aggregate into a pass. No product assertion failure was reproduced on the isolated PT cells. The temporary harness is removed before commit.

## Final frontend recovery repair and source binding

A targeted actual-App navigation regression was RED: the retained submission controller used its original managed transport/client, while a remounted Profile status control created another client; deliberate Retry lacked its header. Log `/tmp/careeros-278-managed-remount-red.log` retains the failure. The fix retains the managed client with the same controller/workflow-field scope and authenticated account, with scope-isolated opaque storage keys. Full original/Profile/document changes remain part of the financial root binding. Different accounts and independent fields never share a client/key. The final EN/PT actual-App regression passes after leaving/re-entering Profile: original text and Action remain, status is GET-only, and only a deliberate unchanged Retry emits its stable key. No automatic inference or Save occurs.

After this frontend-only repair, final full Node369/369 (log `/tmp/careeros-278-final-node.log`), TypeScript, pilot build and actual-App EN/PT browser all rerun and pass. The Go runtime source is unchanged from the994/2,232-event runs. The adjacent JSON summary records the exact tested runtime Go blob IDs and suite timestamps; the latest runtime Go modification (`2026-11-03T23:50:45Z`) precedes both Go runs (`23:51:12Z` / `23:51:16Z`). The later two-account assertion extension is covered by its focused passing run above. No further whole-Go rerun is implied for frontend-only changes.

## Review boundary

Independent final-head Standards/Spec/security reviews are coordinated after this immutable checkpoint; any repair requires focused verification and explicit final-head readbacks. Nothing here enables actual egress, supplies provider-account assurance, selects a hosted/production retention/recovery design or admits a real participant. This is the bounded evidence handoff to existing #194, with no new tracker or GitHub comment.

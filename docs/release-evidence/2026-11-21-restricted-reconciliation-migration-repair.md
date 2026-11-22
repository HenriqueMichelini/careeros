# Unknown-obligation migration repair (#283)

This checkpoint addresses the third confirmed P2 finding, discovered by final-head Spec and security review at `3879397d4144cc5270867d9c9f5b377b3c26b24d` (tree `4cbdfa4bd6001075b163f136dc7eb571b886ed9f`). Those reviews closed the original two P2 findings; the independent closure matrix had 25 passing PostgreSQL/race events. Original implementation and first repair reports remain unchanged.

## Reproduction and narrow repair

An old compacted unknown/max obligation has no preceding financial command. The initializer's inner lateral join therefore omitted it when introducing `attribution_floor`: counted exposure stayed 60 but the floor became zero. A subsequent bill of 90 followed by a corrected revision of 10 could erase the lost-attribution floor. The captured independent RED and the permanent old-schema regression are recorded separately in the summary.

The migration now retains the no-prior-financial-command row with a left lateral join and treats that absence conservatively. It preserves the existing counted floor without increasing count or releasing any obligation. No other financial policy, role, allowance, admission or remote-termination rule changes.

The permanent regression creates the unknown obligation, explicitly retires detail after the original-intent 30-day opportunity, removes the new column using only the disposable migrator, and reopens through the normal initializer. It checks the original count/floor and unknown slot, higher evidence, exact duplicate recovery, denied lower preview/apply, containment, and unchanged counted/occupied values across every scope. Ordinary all-needed deferral, bill-only unnecessary usage retirement, known lost attribution and prior-schema retained/lost correction controls run alongside it. The permanent RED had seven passing events plus the expected unknown-floor failure; the focused GREEN passed nine events.

## Validation and limitations

Final check results, exact 17 runtime/8 fixture blob IDs and raw receipt hashes are in the accompanying migration summary.

| Check | Result |
| --- | --- |
| Final relevant Go/PostgreSQL/race | 1,206 passing events, zero failures, all 37 canonical admission IDs. |
| Final direct full Go/PostgreSQL/race | Exit 1; 2,454 passing events, all retained 24 Lab failure paths plus the additional CV replay deadline, and the Lab package failure. |
| Focused migration/retained/lost controls | 9 passing events after the permanent RED. |
| Physical recovery | 2 passing events, real private fsync PostgreSQL crash/restart and fresh zero-input-key command process. |
| Vet | Fresh default 15 diagnostics byte-identical to exact-base output; scoped exclusions `-composites=false -stdversion=false` pass. |
| Frontend and browser | Historical 426 Node/TSC/build and ten actual EN/PT cells reused with 344 unchanged dependency blobs; not relabeled as new initializer end-to-end runs. |

The extra current CV replay failure is `TestCVRetainedReplayUsesNoDispatchesAndKeepsSemanticFailures` at `internal/evaluationlab/cv_evidence_test.go:74`, with `deadline_exceeded` and the unchanged 10,000 ms cap (11.61 seconds in the aggregate). One bounded isolated exact-`ea8b97d` run reproduced the same failure in 11.14 seconds with no quota/symlink error. Evaluation Lab, semantic evaluator, CV receipt, CV handler and dispatch/http dependency blobs are unchanged since that base. The retained #279 report separately records the same path in an exact-`65358fa` baseline. These provide pre-existing-path evidence; they do not turn the current full aggregate green or make its set an exact match to the retained 24-path full run. No further full repeat, unrelated repair or timeout change was made. The full Go aggregate remains subject to its separately recorded exact-base Lab failures. Go counts are events, including subtests and package completion, rather than unique test counts; canonical admission IDs are checked independently.

The first concurrent relevant/full attempts failed under disposable PostgreSQL disk-quota pressure. The relevant attempt recorded 1,002 passing events before quota/recovery cascades; the full attempt recorded 2,217 passing events with additional failures, including a load deadline. Neither is labeled green or baseline-matching. Raw failed summaries, logs and hashes are retained. Only generated base archives/export executables from four stopped worker-owned runs were removed, preserving their receipt files. The failed gates were then rerun serially with a fresh worker-owned repo-backed temporary root through a short `/tmp` alias, avoiding Unix socket-length limits. Relevant passed, but Lab correctly rejected the symlink in the full run (2,387 passing events, 88 failure entries). The final full rerun used the direct nonsymlink private `/home/henriquemichelini/workstation/careeros/.q64` directory, with its longest socket path verified at 105 bytes. After all processes exited, its receipts moved under the owned `.local-recovery/reconciliation-283-migration-7j5rtufm/full-direct-receipts` directory; the temporary source-root directory no longer exists. No runtime/test assertion was changed for the environment failure.

The prior 426 Node tests (including 44 Profile and 44 CV parity vectors), TypeScript/build and ten actual EN/PT browser cells are reused as historical evidence at the first repair head. Their frontend, router, handler and fixture dependency hashes are unchanged by this two-path migration delta. They are not described as fresh end-to-end runs of the new initializer. Final Go/PG/race directly exercises old-schema initialization, and the dedicated real private PostgreSQL restart/fresh-process schedule was rerun for the changed constructor. No hosted recovery is claimed.

The managed security scanner limitation remains: `Review changes requires a non-bare Git worktree with a resolvable HEAD`. No scanner retry or managed completion is claimed. Independent Standards remains zero hard violations/two nonblocking judgments; final Spec/security closure of this migration finding is pending the immutable repair head. No later ticket or external operation was attempted.

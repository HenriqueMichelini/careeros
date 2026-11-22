# Deletion review repair — local active execution

**22 November 2026. All required checks are terminal within the inherited limitations below. This is a review-ready repair record, not ticket success or operational readiness. Exact repaired-head independent reviews remain required.**

The original review head is `3502e6ddbe7c2704a9639809a541885ac447059a`, tree `f35ec666e31d1e0aa22986549bc4e27a1b7a70d1`, over cumulative base `5269458cecf6064a5e26676ec660302ca44ff40e`. Work remains in the same `codex/batch-277-qualification-20261102` branch and durable worktree. The original [report](2026-11-22-account-deletion.md), summary and frozen source manifest are unchanged historical evidence.

Original Standards review reported zero hard findings and one nonblocking duplicated residual-deadline expression. Spec and security independently confirmed the same P2: Profile deletion changed an unfinished qualification Action to `deleted`, and the active-count query stopped counting it before its callback returned. No other original finding was confirmed. The [selected qualification policy](https://github.com/HenriqueMichelini/careeros/issues/256#issuecomment-6589173667) separates one account/two global active Actions from one account/two global outstanding calls. Deletion cannot serve as evidence that either execution ended.

## Repair and authority boundaries

Qualification, ingestion, Profile-review and standalone-CV wrappers now acquire a content-free local invocation lease before entering their actual handler. The lease binds namespace, Action, attempt, random invocation coordinate and the existing owner-bound input fingerprint. Its captured one-use return closure updates only that exact row after the handler returns. Multiple invocations must all return. Database failure, process loss or absent return evidence remains conservative.

Admission counts running local leases independently of customer/content state and workflow result completion. A truthful local return does not change `execution_finished`, terminal Action state, result/proof eligibility, Draft customer effects, remote occupancy, numeric expense or unknown holds. The existing Draft active/unknown branch remains unchanged. Deleted results remain denied. No customer completion endpoint or caller assertion is introduced.

New Actions atomically receive the database-owned `local_execution_tracking` default. Upgrade first adds false for existing rows, then sets the default for future admissions. Runtime has neither INSERT nor UPDATE privilege on that field. It cannot insert an already-returned local lease or update an existing one; the new insertion guard and existing column/table privileges enforce that boundary. The guard is not SECURITY DEFINER, has fixed `pg_catalog,pg_temp` search path, queries no content/identity rows and grants no new role elevation. Only the existing code-owned reviewer connection is used by the captured return closure.

An Action deleted after Open but before local handler entry is known never started and leaks no local slot. The same admission tracking covers deliberate attempt 2 without changing Action, input, execution or retry identities. Inherited false-tracking unfinished rows remain held even after a newer invocation returns: that does not prove an older untracked callback returned or establish old-writer exclusion. No timer or deletion TTL releases them. Local bookkeeping rows do not consume selected billable/protocol-stage limits. The [contract](../account-deletion-contract.md) and [local guide](../managed-account-deletion-local.md) record these interfaces.

## Permanent RED and final focused proof

- The original reviewer PG/race RED remains copied at `.q88/repair-capacity-20261122/original-review-red.jsonl`, SHA256 `c01ab7308da217a6c282cedebe7cfacea2e2804388936661880f01e28b68a59f`. The original head remains immutable.
- The permanent actual-router regression was RED on original runtime at `.q88/careeros-281-check-pc6m1o28`: two callbacks blocked, deletion committed, and a third account was incorrectly admitted. Its extended GREEN proves the slot remains held until actual return, the other account retains its own active slot, deleted content/proof is withheld, and global/account/provider money and occupancy survive unchanged. Global exposure stays 2,688 microUSD/two unknown calls; deleted account exposure stays 1,344 microUSD/one unknown call. A third Action can be admitted after local return but still cannot send through those remote holds.
- The intermediate markerless predicate leaked a never-started admission slot. Both initial and deliberate-attempt-2 schedules were RED at `j27bc78x`; atomic admission tracking repairs them without a callback, dispatch or financial release.
- Ordinary runtime stage INSERT could forge a returned marker, despite UPDATE being denied. A permanent restricted-role control was RED at `lgkdde00`; the insertion guard now denies it. Explicit tracking-field INSERT/UPDATE and lease UPDATE are also denied. No raw-table grant was widened.
- A returned newer invocation incorrectly suppressed the inherited unfinished fallback. `9yyv603i` retains that RED. The final predicate holds inherited false-tracking unfinished rows regardless of newer return markers.
- The final focused matrix at `jwp6r1ip` passed **14 events**, exit 0: blocked callback/local return; all-invocations/idempotent return under Profile deletion and account closure; inherited no-proof and newer-return conservatism; initial/attempt-2 admission gaps; old-schema upgrade preserving binding/execution bytes; existing qualification cancellation/proof denial; and restricted SQL-role controls. Event counts include subtests/packages, not unique cases.

The expanded intermediate run `gwfae2vs` failed because its account-closure fixture incorrectly supplied Profile ID/revision. Only that request shape was corrected; its failed receipt is retained. The earlier relevant run `822gzpey` passed 1,406 events/all37 before the final inherited-row correction and is explicitly intermediate. It is not final-source evidence. All completed development phases, failures and raw receipt hashes remain in the [repair summary](2026-11-22-account-deletion-review-repair-summary.json).

## Final source and checks

The [repair manifest](2026-11-22-account-deletion-review-repair-frozen-source.json) binds **37 runtime and 16 fixture files**, covering the original 30/13 plus all repaired wrappers, local execution/schema/stage code and new regressions. Full independent review must cover base `5269458` through the commit containing this repair, not merely the small repair patch.

| Check | Actual result |
| --- | --- |
| Final focused PG/race | 14 passing events, exit 0, `jwp6r1ip` |
| Final relevant serial Go/PG/race | 1,407 passing events, exit 0, all 37 canonical admission IDs pass, zero failed packages, `gd443glx` |
| Final full serial Go/race | 2,574 passing events, exit 1; exactly 25 retained Lab test/subtest paths plus package failure, no new/missing failures, `udl1l76q` |
| Default vet | Exit 1; same 15 exact baseline diagnostic lines, only output ordering differs |
| Scoped vet | Exit 0 with explicit `-composites=false -stdversion=false` |
| Node/TSC/build | Inherited original 450 isolated Node tests and successful TSC/local build; all 177 frontend/test dependency blobs equal the original review head |
| Actual App browser | Original seven EN/PT/lifecycle cells inherited with unchanged frontend; no fresh App execution against repaired Go wrappers claimed |

The current relevant suite executes the actual public routers and the existing real private fsync-enabled PostgreSQL immediate crash/restart plus fresh-process status/cleanup controls. Its passing local fixture is not hosted recovery or old-writer exclusion. Standalone browser/server and private child selectors are skipped outside their parent harness; the optional genuine-base exporter remains skipped without its executable. The machine summary records exact skipped names rather than claiming every helper ran independently.

Both Go runners use serial packages, race detection, `-vet=off`, `-count=1` and a 600-second whole-package harness ceiling, with a short real repo-backed TMPDIR. Product inference/customer deadlines and assertions are unchanged. Default vet retains 14 inherited unkeyed-literal diagnostics and the inherited `os.Root.RemoveAll` stdversion diagnostic. The toolchain is Go1.26.4; no declared-Go1.24 execution proof is claimed. The final full failure set exactly equals the retained original and exact-base set. All six recorded Lab/CV/dispatch dependency trees remain unchanged. This reuses verified immutable baseline provenance rather than rerunning the base, and does not turn either exit-1 check green.

## Limits and remaining review

Only synthetic accounts, controlled providers and disposable local PostgreSQL were exercised. No actual deletion, recipient/provider access, grant/admission, spending, hosted configuration/recovery/deployment, GitHub mutation or push occurred. The original dirty research checkout and cumulative commits are preserved.

Managed security scanning remains unsupported for the linked worktree (resolvable-HEAD setup limitation). No managed scan completed and no unsupported setup/probe was retried. Final Standards, Spec and complete read-only immutable security followups must explicitly assess this repaired head and close the P2 before #288 local success. Existing residual-copy/recipient uncertainty and independent financial/replay/deny survivor boundaries remain unchanged.

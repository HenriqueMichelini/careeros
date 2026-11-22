# Restricted reconciliation review repairs (#283)

The repair checkpoint preserves the selected local synthetic contract and closes the two confirmed P2 findings from original review head `864f457fff07694b831a44182f1cd7112b8a0c46` (tree `448422977c0ff8895bfb35602af5b0e2c191e856`), over exact base `ea8b97defb6d0c9593f2bf959bfe007d32ac93c8`. The original report and source manifest remain unchanged. The accompanying repair summary binds all 17 runtime and 8 fixture blobs, captured original and permanent RED receipts, and final check receipts with hashes. Final independent followups are pending.

## Confirmed findings and repairs

Spec and security reproduced a configured reconciliation coverage validator suppressing attributable usage when it rejected or errored. The actual router continued after TypeSafe usage worth 2,688 microUSD exceeded its 1,344 reservation. The repair records conservative observation and above-bound containment before optional D validation. Nil, accepting, rejecting, erroring and missing-proof validators are covered under active and customer-closed authority. Without validated D, category/uncertainty and exposure remain conservative; valid U=130, D=7 settles to 137 while the independent remote slot remains held. No extra OpenAI leg follows containment, and customer allowance stays unchanged. Same-amount supported settlement does not duplicate the already-recorded breach event; higher evidence still records an event.

Spec also reproduced a reviewed 30-day all-needed deferral being marked compacted, preventing an otherwise fully attributable reviewed bill correction from 15 to 10. The repair distinguishes a purpose review from actual loss of attribution necessary for that exact obligation. All-needed deferral and retirement of usage unnecessary to an accepted bill permit a correction only while matching source/revision, request, provider/model/tier/rate/currency/billing namespace and the prior accepted financial evidence remain verifiable. Wrong request and changed-content replay still deny. Removing required attribution records an irreversible numeric attribution floor. A later review or higher bill cannot recreate lost attribution or lower beneath that floor. Higher evidence, exact replay/conflict markers, exposure and remote holds survive.

Prior-schema migration covers both the old false-compacted all-retained state and actual required-detail loss. It preserves count and occupancy, permits only the former exact correction, and does not treat a surviving replay marker as restored attribution.

## Test-first and final evidence

The permanent RED run reproduced rejecting/erroring/missing-proof observation failures and both attributable correction failures. It had exit 1 and 6 passing events; controls stayed separate. The repaired focused matrix passed 19 events, and the prior-schema migration matrix passed 4. Captured original reviewer RED receipts and the permanent RED are retained with their hashes in the summary.

| Final check | Result |
| --- | --- |
| Relevant Go/PostgreSQL/race | 1,205 passing events, zero failed packages, all 37 canonical admission IDs. |
| Full Go/PostgreSQL/race | 2,454 passing events, exit 1; exact same 24 Lab test paths plus package failure as original #283 and retained exact-base evidence. |
| Genuine base upgrade | Immutable `ea8b97d` exporter and real old persisted bindings included in relevant/full runs. |
| Physical restart | 2 passing events: actual private fsync PostgreSQL immediate stop/restart plus fresh zero-input-key CLI; original replay, surviving evidence, deny/deletion and unknown holds preserved. |
| Actual browser | 10 EN/PT React→Vite→authenticated Go→PostgreSQL cells pass on repaired runtime. |
| Node and parity | 426/426, no failures/skips; 44 Profile and 44 CV full-value vectors. |
| TypeScript and pilot/recovery build |Pass; exact mode flags and receipts in summary. |
| Default vet | 15 exact-base diagnostics, byte-identical output. |
| Scoped vet |Pass with explicit `-composites=false -stdversion=false`. |

Go event counts include subtests and package completions; they are not unique test counts. The canonical 37-ID result is separately recorded. The ten browser cells exercise actual local routes, response loss, protected denials, reauthentication and account isolation. The physical schedule does not claim hosted recovery or automatic old-pool repair.

The full Go suite remains red. Baseline provenance reuses the original committed exact-base comparison because the final failure set matches exactly; historical baseline temporary raw files were already cleared. No baseline rerun or raw-hash recomputation is claimed. Current repair raw receipts were available and hashed when this report was written. Default vet retains fourteen unkeyed test literals and `internal/evaluationlab/store_manage.go:74:19` using `os.Root.RemoveAll` (`s.root.RemoveAll`) under module Go1.24 despite requiring Go1.25. Verification used Go1.26.4; scoped exclusions do not establish declared-Go1.24 full-green coverage.

## Independent review limitations

Original Standards reported zero hard violations and two nonblocking judgments (repeated command-kind branches and duplicated rate estimation). Original Spec's two P2 findings and security's shared observation finding are preserved as repaired candidates pending exact-head followups. Security completed its independent read-only assessment across all 26 paths, but its managed scanner failed with `Review changes requires a non-bare Git worktree with a resolvable HEAD`. No managed report completion or scanner no-findings result is claimed.

All data, providers, observations, roles and command executions are local synthetic/disposable fixtures. No provider account access, real calls/spending, participant grants, deployment, hosted recovery or remote GitHub mutation occurred. No later ticket was investigated.

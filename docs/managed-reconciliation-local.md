# Restricted reconciliation in the local synthetic composition

The contract for [#283](https://github.com/HenriqueMichelini/careeros/issues/283) is the [owner selection of 4 November](https://github.com/HenriqueMichelini/careeros/issues/283#issuecomment-6645082417). It builds on the reviewed #277–#282 authority at `ea8b97defb6d0c9593f2bf959bfe007d32ac93c8`, the [restricted ledger classes](https://github.com/HenriqueMichelini/careeros/issues/258#issuecomment-6511528310), [conservative exposure direction](https://github.com/HenriqueMichelini/careeros/issues/249#issuecomment-6511521383), and [frozen USD rates/shared pools](https://github.com/HenriqueMichelini/careeros/issues/256#issuecomment-6589173667). The version is `restricted-reconciliation-local-20261104-v1`.

This is synthetic-only source: in-memory fake providers, synthetic accounts and disposable PostgreSQL. It installs no actual provider/bill/notification adapter, hosted role, grant, participant, configuration or recovery operation. It adds no career content or invoice retention. The Go command is normally buildable and its existing authenticated-session path is exercised as a real child process.

## Commands and authority

`cmd/reconciliation-local` requires `--synthetic-only`, a literal loopback `*_test` database and an explicit `careeros_ai_test_*` schema. Alternate routing/DSN options fail closed. The database URL and existing CareerOS session token are process-local environment inputs; no key/account/bill files are discovered. Do not print either input. Normal execution connects as the exact independently authenticated reconciliation database role. Its minimal account reader uses only ID, status and generation.

A separate `--mode install` migrator command installs the first authenticated synthetic owner as the only manual operator, with bounded explicit elevation. Runtime/reconciliation connections cannot install or extend their own elevation. Repeating installation cannot replace the owner or renew the existing elevation. Participant grant rights are unchanged.

The modes are `preview`, `apply`, `status`, `events` and `rates`. JSON command input is at most 64 KiB, rejects duplicate/unknown fields, and has no caller credentials, target-owner impersonation, `ReleaseFloor`, `terminal=true`, FX selection or arbitrary payload field. Actor identity comes from the actual session; dispatch ownership is resolved in the database. Operator and target account may differ.

Preview computes the effect under current actor/session, namespace, Action, dispatch and original allocation locks. Its token binds the command, current contribution/version, exact session/generation and elevation version. Apply requires that preview and expected version. Command/source-revision digests and the exact result commit with every original-scope delta. Concurrent stale versions conflict; duplicate/lost-acknowledgment commands return the recorded original effect. A source revision cannot be changed by relabeling its command. Revoke/restore invalidates an old elevation preview. Expiry after blocking work rolls back all scopes and audit effects.

| Kind | Accepted authority and effect |
| --- | --- |
| `observation` | Bounded attributable numerical observation; no financial or capacity adjustment. |
| `usage` | Complete compatible usage at the dispatch's frozen contract. Count upward-rounded U plus justified unaccounted D, preserving disputed floor. Label supported estimate. |
| `maximum` | Explicit reviewed conservative disposition only for financially unresolved calls at 168 elapsed hours from original persisted durable-intent anchor. No age replacement of accepted estimates/bills. |
| `bill` | Attributable gross bill; complete coverage is required to replace an estimate's contribution. Credits/discounts do not replenish gross pools. |
| `corrected_bill` | Explicit exact superseding source revision, complete attributable coverage and reviewed correction. A larger local version alone cannot lower expense. |
| `unpriced` | Explicit attributable unsupported currency/category/rate evidence; retain prior conservative exposure, persist the unsupported reference and contain admission. No automatic FX. |
| `rate_revision` | Immutable separately recorded reference values, including missing categories/currency. Never automatically reprice old dispatches or change admission policy. |
| `terminal` | Exact request/provider proof from an independently installed code-owned fake terminal record. Financial evidence, EOF and operator assertion do not qualify. |
| `compact` | Explicit purpose review after 30 elapsed days from the original durable dispatch intent. Retire only detail without a current purpose; preserve obligations and replay markers. |
| `reopen` | Explicit current owner record after compatible reviewed reconciliation and an independently recorded local bounds review. Preserve old exposure/audit/customer state. |

Default compatible provider observations also go through the restricted command. Without a validated D they can only preserve or raise the original conservative floor; an above-maximum U still commits the higher exposure and containment. The code-owned original dispatch observer retains this narrow financial capability after customer session/grant closure. It records a runtime-observer actor and does not acquire human reconciliation authority, mark a bill, release money/capacity or create further inference rights. Public explicit runtime settlement still checks the current exact owner/session.

Runtime does not receive the manual financial command capability. `SettleSavedObservation` accepts dispatch identity only, reads immutable normalized provider usage, checks the actual owner/session and frozen contract, and invokes an explicitly code-owned `SetSyntheticUsageCoverage` validator. No validator or incomplete/missing attribution/categories/D proof retains the maximum. A validator supplies a bounded attribution reference and justified D, never an arbitrary caller percentage. Default compositions without a trustworthy full-charge contract keep the inherited conservative floor. Synthetic public rates and observations remain estimates, never bills.

## Money, capacity and containment

Amounts are integer microUSD under `qualification-usd-20261102-v1`, or explicitly separate SYN units for inherited artificial fixtures. The original model/tier/rate/currency/billing namespace is frozen per dispatch in the reservation transaction. An isolated migrator backfills only exact known inherited execution versions. Unknown versions fail closed. Cached/cache-write input partitions ordinary input; reasoning is already part of output. Each category is counted once, with upward rounding and overflow checks.

All adjustments use the existing allocation rows and account/provider/global pools. No workflow gains a copy. Counted U+D and provider usage U remain separate. Unreleased differences continue across rolling windows even after independent remote termination. Lifetime pools never reset. No command changes customer Draft allowance or proves delivery/Accept.

Above-maximum accepted evidence commits the higher obligation even beyond pool ceilings and a restricted actionable event. Unknown valuation retains the prior hold and also emits containment. The local namespace is the safely isolated managed gateway: its inference admission is stopped pending explicit owner review; no narrower safe live bound contract is claimed. A fake signal sink can fail without reopening anything. There is no actual notification driver.

Containment is checked at new Action, new attempt, reservation, durable claim and final handoff under the namespace fence. Provider-free authorized identity/status/result rights retain their existing independent authority. Financial commands continue after containment, external gateway closure, target grant/customer closure or namespace retirement, provided their operator/session authority is current. Financial settlement does not release remote occupancy. Code-owned fake terminal proof is a separate restricted command and releases occupancy once; no live terminal proof contract is supplied.

## Clocks and detail retirement

The168-hour eligibility anchor and the 30-day attribution-opportunity anchor are each the original persisted dispatch intent, never a later stage, Retry, status, period, bill or compaction. Neither timer automatically closes, deletes, releases money/capacity or changes customer rights.

At or after 30 days, the elevated operator supplies an explicit purpose-review reference. Usage, request-ID and source-reference detail still needed for a dispute, adjustment or independent capacity proof is retained specifically for that purpose. Any deferral records a next owner review strictly after the review and no later than seven elapsed days from that review. Passing that next-review date triggers no erasure/release.

Compaction preserves original dispatch/account/Action/attempt and allocation linkage, original intent and terminal/deletion anchors, category/revision/correction references, counted amounts, unresolved differences/remote holds, deny state and keyed replay/conflict markers. Retired observation identity rows remain, preventing delayed append from repopulating removed detail. Necessary typed contract metadata and compact numeric closure audits survive. There is no career content, raw provider response, invoice, credential or credential hash in these tables.

Later exact duplicate evidence recovers its original effect; changed replay conflicts. Higher attributable evidence still increases exposure and contains admission. A lower claim cannot use detail that was retired. Conservative overcount can remain. Account/content deletion does not erase these active purposes or authorize retaining additional career content.

## Keys and retirement

Input bindings default to `input_v1`. New bindings use an explicit current version; active old versions retain their exact verifier. Existing Action keys resolve the original version before comparing root/Profile/execution/stage bindings. Rotation changes neither identity nor frozen bytes and cannot resend durable intent. Missing or changed live verifiers fail initialization. No current-policy/key fallback interprets an old identity as new.

Financial command/audit/terminal/conflict verification uses a separate named local audit domain, independent of input rotation. Before detail or input-key retirement, inherited evidence signatures are verified using their original key and sealed with an independent audit/lineage marker. Missing original detail/key prevents retirement. Original command IDs, conflict bindings and obligations survive; independent verification does not depend on a destroyed input key.

There is no timer-based key destruction. `RetireInputKey` requires the explicit elevated owner, an irrevocably retired/drained independent local gateway, matching durable namespace deny state, and a code-owned verifier for the whole-namespace checkpoint: every old entrypoint/delayed sender excluded, current non-restored authority reconciled, result/capability/recovery-copy horizons closed, and surviving obligations verifiable independently. A caller checkpoint label or restored generation bump cannot supply this proof. Input material is omitted from the successful restarted configuration; the fresh financial CLI uses zero input keys. Essential audit/deny/expense/deletion/remote markers are not pruned by this command.

The older `SyntheticReviewer` APIs remain laboratory capabilities used by inherited admission tests, not the new operator interface or production authority. Their original fixture review-clock/retirement tests retain their historical contract. The restricted command path enforces the owner-selected 168-hour/30-day purpose protocol, complete financial evidence and containment. Inherited replay markers are upgraded before detail/key retirement; no namespace-pruning helper is used for 30-day compaction.

## Verification boundaries

Permanent tests exercise actual restricted CLI processes, separately elevated actors, current sessions, original shared allocations, independent SYN and selected-USD arithmetic, complete/missing/conflicting evidence, races, post-lock expiry, compaction and key removal. The actual qualification router establishes the USD case: 3095 TypeSafe input tokens round upward to130 microUSD; justified D7 counts137 rather than the original1344. The other reached qualification legs and five customer Draft rights remain unchanged.

`tests/draft-recovery-pg.py --legacy-base` builds an immutable `ea8b97d` archive with a test-only exporter. The deliberately terminated old child leaves genuine base-generated root/execution/dispatch/evidence bytes, which current migration and rotation must preserve. Exported synthetic session material stays in a private temporary file and is never committed or printed.

The dedicated `--reconciliation-physical-restart` schedule is limited to its single controlled test. It performs a real private fsync PostgreSQL immediate stop/restart and then reads via fresh connections and a fresh key-free CLI process. Harness reconstruction without that flag is labeled separately. This proves local synthetic durability/replay behavior, not hosted recovery, live account terms/bills, cryptographic key-erasure guarantees or production notification delivery.

The release evidence binds exact tested runtime/fixture blobs and records inherited full-suite/vet limitations. Independent Standards/Spec/security review remains required before this slice is locally complete.

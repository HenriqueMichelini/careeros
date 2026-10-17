# Disabled live capability design v1 — #155

Frozen implementation/review base: `9564cfd32084e56a7154b1d27632de5917018627` (merged #154). This document must receive Standards and Spec review before guard implementation. The scope is a disabled authorization/accounting capability tested with controlled transports. No experiment, credential discovery, deployment or real network dispatch is authorized. CV calibration stays blocked by delivered #124–#128 contracts/evidence.

## Boundary and public seams

Add `LiveGuard` in `internal/evaluationlab` beside the existing offline/controlled runner, using the private `FileStore` for immutable preflights and a durable dispatch journal. Existing runner modes remain offline/controlled; the application does not construct a real provider transport. Server enablement only exposes the capability/preflight path. It never makes an old runner plan, flag or import executable as a paid run. A future real experiment still requires immediate separate human cohort/spend approval and a deliberately supplied transport.

Public seams under test are guard preflight/authorization/dispatch/cancel/status, FileStore journal recovery, and authenticated HTTP capability/preflight. The launcher defaults to disabled. An explicit `--enable-live-guard` may enable preflight capability; it does not authorize a run. HTTP reports that real execution has no transport and cannot dispatch. No browser live mode or credential persistence is added.

## Exact preflight

A `LivePreflight` is an exportable versioned JSON artifact with server-generated identity, current server-instance nonce, expiry, canonical hash, validated frozen Lab plan and the following fields:

- Fixture/cohort order and exact fixture/label/source/prompt/scorer/schema/strategy/runtime identities, configured model identities, fixed recipients, returned-model expectations and one changed comparison dimension where applicable.
- Logical cases, ordered HTTP legs and unique provider-dispatch identities. Each dispatch names its exact case/leg/model/recipient and conservative token reservation. No arbitrary provider/model URLs; fixed recipient allowlist only.
- Hard maxima for logical cases, HTTP attempts, provider calls, reserved token units, per-call output tokens, elapsed deadline, retained artifacts and one active run/concurrency=1.
- Stop on cancellation, first operational/contract/capacity/usage-overrun failure, unknown final state or retention failure; no retry, repair, fallback, automatic resume or replacement dispatch.
- Private retention policy and explicit exposure groups, pricing snapshot/version/date when supplied, otherwise unavailable cost. Token reservations/caps are not a dollar cap or a claim of provider-observed tokens.

Preflight creation revalidates the existing planner's fixed synthetic fixtures and frozen plan. It rejects missing identities, unsupported models/recipients, nonpositive/excessive caps, duplicate dispatch IDs, mismatched ordered legs and comparisons with changed labels/quality/budgets/concurrency. Comparison schedules interleave configurations per case, retain language/pair identity, and change exactly one declared dimension; no pooling/population claim.

The canonical hash binds every approval field and expiry. Enablement, plan validation and human authorization are separate. Any edit requires a new preflight. Exports contain no credentials, secrets or authorization token.

## Local human authorization and credentials

`Authorize` requires enabled capability, the exact current unexpired preflight hash, a configured human reviewer, explicit human actor/type, and typed confirmation of that hash, cohort, recipients and caps. These are local attestations, not external identity authentication; agent/model actors cannot approve. Controlled tests use a separate origin and cannot authorize a real experiment.

Credentials are passed explicitly to that one guard instance in memory only, indexed by fixed recipient. They never come from environment/files/product storage, URLs or imports. Require only recipients in the exact preflight; reject missing/extra credentials. Do not echo them, hash them into artifacts, retain them in journal/history/exports, or include them in errors/logs. A fresh per-run authorization token is process-local. The durable journal retains its public approval receipt, never its credential/token/nonce secrets. Clear owned references on completion, cancellation, expiry, failure and shutdown; no guarantee of physical memory erasure is claimed.

The server's current HTTP path exposes capability status and preflight export only. Run authorization/execution is a trusted local guard API seam verified with controlled transports. This prevents the shipped browser/API from turning a synthetic test approval into paid execution. No real transport is included in this slice.

## Atomic dispatch and evidence

`Dispatch(ctx, authorization, exactDispatchID, controlledTransport)` holds admission under one mutex. It validates enabled/current authorization, hash/expiry, cancellation/deadline, exact dispatch plan identity and unused ID, and reserves logical/HTTP/provider/token/artifact budgets before invoking the transport. Persist the intent atomically before releasing admission. A write failure prevents dispatch and revokes authorization. Only one guard run is active; concurrent calls cannot overspend or double-dispatch. Tokens are conservatively reserved before calling and are never refunded by cancellation/failure or unknown usage.

A request is counted once after durable admission, including failed, canceled, refused and unknown-final requests. Capture configured/returned model, start/end/time, observed token components/pricing when present, outcome and safe unavailable reasons. Never retain request credentials or raw headers. Every gate/review/sequence provider leg needs its own exact ID. A skipped/unreached leg is not a dispatched call. The first failing dispatch stops later admission; successful terminal completion revokes credentials as well.

Cancellation propagates through the supplied context. Cancellation before admission prevents the call; cancellation after admission preserves reservations and the attempted-call count. Unknown or partial usage/pricing makes full cost unavailable, with known subtotal separate. Failed/canceled priced usage remains counted. No unknown call is silently priced as zero. Finite paired observations include failed/unreached cases and returned-model/time provenance, not population quality claims.

The store keeps a bounded private journal in a reserved metadata directory independent of native run files. Preflights/approval/dispatch intents/results are append-only records with hashes and original links. On restart all in-flight intents become unknown-final; previous authorization is unusable because credentials/tokens/instance nonce are absent. Repeated clicks and imported approval artifacts never create a second dispatch. The guard has no resume operation. Exported controlled guard reports separate admission attempts, dispatches, unknown-final, failed/unreached legs, simulated usage, unavailable real cost/quality and missing evidence. Existing M10–M12/M17–M23/M28 definitions apply without new release thresholds.

## Verification before any future experiment

Use synthetic fixtures and controlled transports only. Retain a concrete preflight, approval receipt without secrets and versioned controlled guard report. Test disabled mode; altered hash/plan/dimension; expiry; missing/extra recipient credentials; racing caps and exact-ID double clicks; all operational failure classes; cancellation before/after admission; persistence failure; restart with unknown-final intent; absent/partial pricing/usage; token reservation exhaustion; model/recipient mismatch; controlled fair interleave; sentinel absence from exports/history/errors and zero real network calls. Run public HTTP/runner/store regressions and final two-axis review before merge.

Guard implementation closure proves technical enforcement only. Real cohort/spend decisions remain with Henrique and require separate explicit authorization immediately before dispatch. Delivered CV owner contracts are additionally required before CV calibration. The capability must continue to report unavailable real execution until a separately authorized integration supplies a real transport.

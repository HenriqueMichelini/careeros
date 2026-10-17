# Controlled live-guard capability v1

Issue #155 implements the reviewed [design](design.v1.md). Frozen base: `9564cfd32084e56a7154b1d27632de5917018627`. Both design review axes cleared `c29ad04a359de3385f182f9a218b85b71e1476e3` before implementation. This is controlled guard verification, not a real experiment or authorization to spend. No provider transport is installed in the Lab. CV calibration remains gated by delivered #124–#128 contracts/evidence; #153 stays pending for that owner delivery at Henrique's direction.

## Server and public contracts

The launcher defaults to disabled. `npm run lab -- --enable-live-guard` enables preflight creation only. `--human-reviewer` identifies a local domain reviewer; neither flag authorizes dispatch. Existing runner modes stay offline/controlled. Authenticated `GET /lab/api/live/capability` always reports `realExecutionAvailable: false`. With explicit enablement, `POST /lab/api/live/preflights` accepts a strict `LivePreflightRequest`; `GET /lab/api/live/preflights/{id}` exports owned metadata. HTTP authorization/dispatch remain forbidden, including after reload, import or credential presence. No browser live-mode switch or credential input/storage is added.

The trusted local `LiveGuard` API exposes `Preflight`, `Approval`, `Authorize`, `Dispatch`, `Cancel`, `Report`, exports and `FreezeComparison`. Its callback type is deliberately `ControlledGuardTransport`. The shipped application never constructs a real HTTP/provider client for this path. Any future real integration, synthetic cohort and spend need separate explicit human authorization immediately before execution, plus a new reviewed provider-message/token-boundary integration. A controlled approval artifact cannot authorize that integration.

## Binding and authority

Preflights bind the owned planner's exact synthetic cohort/order, fixture/label/source/prompt/scorer/schema/strategy/runtime identities, configuration/model/reasoning/output settings, fixed recipients, unique logical/HTTP/provider-step identities, token reservations, expiry, stop policy and private retention. Unknown/missing source identities, altered plans, arbitrary recipients/models and malformed bounds are rejected. CV fixtures are unavailable pending owner delivery.

`payloadHash` binds the exact frozen synthetic step input. `payloadKind` explicitly states that this is **not** a captured provider message/request. There is no claim that provider tokenization, hidden overhead or real request/response behavior has been verified. Conservative reserved token units and per-callback output ceilings are tested controls; they are not real provider-observed usage or a dollar cap.

Enablement, owned-plan validation and per-run human authorization are separate. Authorization requires the configured human ID/type, exact current unexpired challenge hash, typed confirmation and rationale. Agent/model actors fail. This is local human attestation, not external identity authentication. Tests use `controlled-human-example` and `controlled-test` origin; they are not Henrique's approval of an experiment or semantic gold.

The public instance identity is exportable. A separate unpredictable authorization token and explicitly supplied recipient credentials remain in memory only. Credentials are copied into one run, checked against retained metadata to reject accidental echoes, never discovered from files/environment/product storage, and never placed in URLs/history/exports/logs. Only typed usage and server-owned metadata survive controlled transport observations; arbitrary errors/headers/metadata are discarded. Completion, cancel, expiry, failure, shutdown and restart revoke owned references. Physical memory erasure is not claimed.

## Admission, recovery and budgets

One guard owns a private store, which also retains its existing exclusive writer lock. One active run and one in-flight callback are allowed. The requested dispatch ID must match the exact next schedule index. Admission reserves logical/HTTP/provider/token/retention budgets under a mutex and commits an intent before invoking the callback. The in-flight slot remains occupied through callback completion and durable result retention. Concurrent distinct IDs, duplicate clicks and out-of-order calls fail. No retry, repair, fallback or resume operation exists.

Failures/refusals/truncation/malformed responses/rate limits stop later admission. Cancellation propagates but cannot refund an admitted request. A failed intent write prevents the callback and does not invent a call. A missing durable result recovers as unknown-final and cannot be redispatched. Journal snapshots are immutable and hash-linked, with an atomic current head. An uncommitted first planned snapshot may be retained as unavailable metadata without authority; ambiguous/corrupt guard evidence fails closed. Private files/directories use 0600/0700 and existing rooted path protections.

Ceilings: 512 logical cases, 2,048 workflow HTTP legs, 4,096 provider admissions, 1,000,000 reserved token units, 16,384 output tokens, 60-second run deadline, ten-minute maximum approval lifetime, 1 MiB record/export and 128 retained preflights. The run's lower frozen artifact cap applies; the entire guard journal is capped at 16 MiB. Journal capacity is reserved before callbacks. Actual host storage failures still produce conservative unknown-final evidence; they never authorize redispatch.

Reports distinguish admitted logical cases, workflow HTTP legs and provider callbacks. These are controlled admissions, not proof of external HTTP requests. All failed/canceled/unknown-final admissions retain their count/reservation. Raw invalid usage remains inspectable but is excluded from trusted aggregates/pricing. Dated exact-model price snapshots are hash-bound; partial prices show a known simulated subtotal and leave the full total unavailable. Unknown finality also makes full cost unavailable. No actual charge or accepted-result cost is inferred.

## Frozen comparisons and metrics

`FreezeComparison` pairs owned single-case preflights with equivalent fixture/label/source/quality/budget identities, step coverage/reservations and price snapshots, changing exactly one supported declared dimension. Current complete evidence supports configuration comparisons. The schedule alternates case/config order; authorization binds both preflight and comparison hashes and rejects a later paired run until earlier runs complete. Failure/restart stops continuation. Each run still needs its own immediate human authorization. Languages and exposure groups stay separate; no population claim or new release threshold is made.

M17–M19 expose finite guard outcomes/admissions; M21 retains nullable simulated token components; M22 measures callback duration, not provider speed/percentiles; M23 labels controlled estimates and unavailable real charges. M10–M12, M20 and M28 remain unavailable where guard metadata cannot establish semantic populations, reuse or independent privacy/network monitoring. Separate sentinel/network-denial tests are finite verification, not universal privacy proof. Existing curated integrity requirements and #52 gates are unchanged.

## Representative evidence

The `examples/` v1 artifacts come from public controlled tests:

- `preflight.v1.json`, `approval.v1.json`, `guard-success.v1.json`: three bounded synthetic provider legs, secret-free local test receipt and unavailable real cost/quality.
- `unknown-final.v1.json`: restart after result-retention failure preserves the admitted intent, reservation and inability to resume.
- `comparison.v1.json`, `comparison-observations.v1.json`: finite EN/PT configuration interleaving with exact per-run correspondence; controlled outcomes only.

Separate deliberate regeneration (IDs/source observations change):

```sh
LAB_GUARD_EVIDENCE_DIR=/tmp/lab-guard-examples GOCACHE=/tmp/careeros-go-cache go test ./internal/evaluationlab -run 'Test(ControlledGuardAuthorization|GuardRestart|GuardFrozenComparison)' -count=1
```

Required controlled tests cover disabled/enabled preflight-only HTTP, altered hash/plan/recipient, expiry, agent rejection, credential sentinels, racing caps and distinct IDs, duplicate/out-of-order calls, cancellation, failure classes, usage overflow, absent/partial prices, pre/post-dispatch retention failures, unknown-final restart, sequence counters, fair interleaving and zero attempted external networking. Full verification and final review results are recorded in the PR/issue; these artifacts do not prove real model quality, live calibration or release acceptance.

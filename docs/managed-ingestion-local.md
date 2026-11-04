# Managed staged Profile ingestion (local synthetic composition)

Issue [#278](https://github.com/HenriqueMichelini/careeros/issues/278) extends the same authority used by the locally completed qualification slice. Its base is `9a759e5b0ae99e2328f535a2168fb9d3ae9d544c`, including #276 and merged #275. An open remote issue does not imply that this local batch publishes, merges or closes it.

The selected contract is [#256, 3 November ingestion policy](https://github.com/HenriqueMichelini/careeros/issues/256#issuecomment-6612492774), cross-referenced by [#249](https://github.com/HenriqueMichelini/careeros/issues/249#issuecomment-6612495412) and [#278](https://github.com/HenriqueMichelini/careeros/issues/278#issuecomment-6612497601). Frozen provider/rate identities and shared pools remain [#256, 2 November](https://github.com/HenriqueMichelini/careeros/issues/256#issuecomment-6589173667). The bounded Q1/Q3 disposition remains [the prerequisite audit](research/careeros-277-bounded-prerequisites-2026-11-02.md).

## Explicit composition and authority

`aicontrol.IngestionSyntheticConfig` enables ingestion alongside qualification in the **same existing namespace**, policy rows, account/global money pools and capacity authority. `accountapi.NewSyntheticIngestion(service, store, harness, signingKey)` composes real authenticated account/grant checks, the actual v2 staged ingestion handler, inherited qualification routes and an ingestion status GET. Every dependency must be explicitly supplied. There is no environment/production selection of this composition, and only a code-installed fake responder can execute. The separately exported injected ingestion handler also requires a managed controller and cannot fall back to its legacy HTTP client.

Canonical `/api/profile/ingest`, direct Function `/.netlify/functions/profile-ingestion` and account Function aliases share this composition. Ingestion legacy/v1 protocol bodies remain closed in managed mode. Other managed workflows remain closed until their own slices. Ordinary legacy execution is unchanged. Header and top-level envelope credentials, caller attempts and dispatch identifiers are rejected; credential-like words inside career text are not scanned as controls.

The verified account and current opaque session supply identity. The complete original input, complete supplied Profile, optional canonical document, protocol/source version, saved Profile fingerprint, selected policy and server stage/model/tier identity are immutable bindings. Supplied Profile/document equality is checked against the saved canonical Profile **inside Open's account-locked transaction**, not against an earlier HTTP snapshot. Current session, grant, saved Profile, gate, time, money and capacity are rechecked after locks and before durable intent/one-use invocation. A changed root/Profile requires a new Action. Receipt signing is purpose-separated by verified account and immutable Action, so receipts cannot be borrowed across either.

The restricted ledger retains keyed bindings and bounded identifiers only. Career text, answers, contexts and generated content are transient; they are not copied into financial rows. Status is honest non-content metadata, not durable content retrieval. No Draft allowance is reserved, deducted or delivered by this slice. A provider-free plan/assembly/status/review sends no provider request. Proposals become saved Profile data only through the existing deliberate review selection and acknowledged explicit Save, retaining provenance.

## Selected bounds

| Bound | Selected value |
| --- | --- |
| Ingestion Actions/account/grant; rolling 24h | 10; 3 |
| Attempts/Action | Initial plus one deliberate known-failure Retry |
| Extract/reconcile/clarify requests per attempt | 20 / 20 / 4, including failed/canceled requests |
| TypeSafe/OpenAI calls per attempt | 48 / 48 |
| TypeSafe input per call / cumulative | 32,000 / 1,536,000 tokens |
| OpenAI input per call / cumulative | 64,000 / 900,000 tokens |
| OpenAI output per call / cumulative | Existing 6,000 / 240,000 tokens |
| Attempt/Action expense | 300,000 / 600,000 microUSD |
| Shared account rolling 24h / grant total | 300,000 / 2,000,000 microUSD |
| Shared global rolling 24h / pilot total | 1,500,000 / 10,000,000 microUSD |
| Active actions and outstanding calls | 1/account; 2/global, sequential/no queue |

Independent cumulative arithmetic using the frozen conservative rates is `(1,536,000×42 + 900,000×125 + 240,000×500)/1,000 = 297,012 microUSD`. TypeSafe 32,000 input reserves 1,344 microUSD; OpenAI 64,000 input plus 6,000 output reserves 11,000. Each reached dispatch rounds upward independently. There is **no flat or upfront 297,012 reservation**: a reached TypeSafe intent can hold 1,344 even if the subsequent OpenAI leg is denied. Missing trustworthy full-request bounds, wrong model/tier/reasoning, cumulative token ceilings or any shared limit block the next send. These synthetic bounds and frozen rate fixtures do not prove real tokenizer, account-tier, pricing or billing compatibility.

The 256-portion planner is unchanged. A complete plan requiring more than 20 extraction batches returns explicit capacity before extraction; it is never shortened to fit. Existing 30,000-byte original, 2,000-byte prepared portions, 30-claim/60-operation focus, 1,000-byte answer and context limits remain. Whole-original Jev validation and exact source identities remain in the actual adapters. Every clarification validates both the whole original and answer before extraction/comparison; the selected claim, answer and context are bound before dispatch.

## Retry, deadline and recovery behavior

The first persisted non-clarification protocol request fixes the attempt's absolute 300-second foreground deadline. It is checked again after locks and before every send. Stage claims, idle time, status and replay never reset it; a claim made before a crash with zero dispatches cannot reopen it later. Existing handler/server 27-second, browser 29-second, unit 65-second and signed-receipt ten-minute limits remain. Focused clarification is a separately explicit foreground operation using the existing handler/client limits, sharing its Action's counters and unchanged real receipt expiry. A deliberate permitted new financial attempt has its own first-request deadline witness; it cannot extend an old signed receipt.

An unchanged reached stage, including a known-failed clarification answer, returns status and never resends. A changed answer is a new explicit bounded clarification child in the **same Action and same attempt**, counting failed children toward four. Only an unchanged authoritatively known-failed command with an explicit stable Retry key can allocate attempt two. Replaying that key cannot allocate another attempt or resend durable intent. Changed Retry keys conflict. Unknown dispatches are not retryable; unknown money and occupancy survive time-window changes and Harness reconstruction.

Fake terminal proof releases occupancy only after authoritative synthetic evidence. Missing/partial usage keeps conservative expense counted; neither HTTP success nor local timeout proves a remote terminal outcome or zero expense. The inherited 168-hour reviewed-closure eligibility and closed restore boundary remain; this slice adds no reconciliation override, retention clock or automatic release.

The client keeps only an opaque account- and workflow-field-scoped recovery key in session storage. Its in-memory managed client shares the retained controller lifetime across Profile navigation; independent fields/accounts cannot share it. Stage content and results remain transient. GET status does not recreate a plan or infer, and does not automatically activate, mutate or Save. Account-keyed component/session ownership isolates account switches; existing unsaved-work and source-currentness rules remain. Known failure requires a deliberate retry click; unknown processing exposes status recovery without a mutation retry.

## Verification and handoff

See [the #278 verification record](release-evidence/2026-11-03-managed-ingestion.md) for fresh runs, exact-base comparison, skips and retained failures. This is the bounded cross-feature evidence handoff to existing #194; no extra ticket or remote progress comment is created. PostgreSQL/race evidence exercises actual signed routes/adapters; Chromium evidence separately exercises the actual App/account/grant/Profile UI with synthetic browser responses. Neither establishes live quality, participant admission, production storage, hosted recovery or launch readiness.

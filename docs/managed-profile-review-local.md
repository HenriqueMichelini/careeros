# Managed Profile review (local synthetic composition)

This is the local implementation of [#279](https://github.com/HenriqueMichelini/careeros/issues/279), based on cumulative commit `65358faafe8a88e9539bdd91e59a863f25a7ff65`. Its ancestors include locally reviewed #277 `9a759e5b0ae99e2328f535a2168fb9d3ae9d544c`, #276 `448d06fb796526c5cc895336b8a5df8ce554fa0d` and merged #275 `bafe318bfd638ef758a1fc1f6d324eebc71c6e08`. GitHub remains the canonical tracker; local implementation does not close or remotely integrate these issues.

The selected contract is [#256, 4 November Profile-review policy](https://github.com/HenriqueMichelini/careeros/issues/256#issuecomment-6624202734), with [parent handoff](https://github.com/HenriqueMichelini/careeros/issues/249#issuecomment-6624203219) and [#279 prerequisite readback](https://github.com/HenriqueMichelini/careeros/issues/279#issuecomment-6624203636). Frozen provider/rates/shared pools remain [the 2 November selection](https://github.com/HenriqueMichelini/careeros/issues/256#issuecomment-6589173667). The [bounded Q1/Q3 disposition](research/careeros-277-bounded-prerequisites-2026-11-02.md), [qualification authority](managed-qualification-local.md) and [ingestion authority](managed-ingestion-local.md) remain inherited.

## Explicit composition and authority

`aicontrol.ProfileReviewSyntheticConfig` adds Profile review to `IngestionSyntheticConfig` in the **same namespace**. It does not change qualification/ingestion execution identities or reset any existing scope rows. `accountapi.NewSyntheticProfileReview` composes the authenticated account/grant/Profile router, actual two-stage review handler, inherited qualification/ingestion routes and read-only review status. Every dependency is explicitly injected. No environment/Function/hosted startup selects this composition; only an installed in-memory fake responder can execute. Production egress, credentials and provider accounts remain unavailable.

Canonical `/api/profile/review`, direct `/.netlify/functions/profile-review` and account Function aliases share the same authority. Caller credential headers/top-level fields and caller attempt/dispatch controls are rejected. The overwritten adapter credential reference is a synthetic local marker, never an actual credential or caller spending authority. Generic prompt-envelope follow-up ownership is unchanged; no generic relay or additional validator is introduced.

Admission binds the verified account, authoritative saved Profile fingerprint and exact root document/section/locale. Under the account lock, the full or canonical section-scoped request must match the saved document using the existing section-scope implementation. Unsaved/absent Profiles are denied: ingestion's first-paste exception is not generalized. All scoped facts, context, evidence, revision and policy/execution identity remain frozen. A source/Profile/section/locale/policy change requires a new Action; old exposure remains. The ledger retains keyed non-content bindings and bounded identifiers, never Profile text, generated proposals or credential hashes.

Before reservation, durable intent, handoff and protected response writes, the same authority rechecks account/session/grant/gate/time, saved Profile and immutable execution policy. Profile review reuses the protected result fence through the actual outer pilot flush, retaining currentness locks through the write, without a database lock across a provider call. The first persisted request witness fixes each attempt's absolute 25-second deadline. A crash/replay with zero dispatches cannot restart that clock; a permitted new financial attempt has a separate witness. Existing request/document/fact/provider-envelope/workflow bounds remain.

## Selected envelope and exposure

Policy ID: `profile-review-local-20261104-v1`; rate ID: `qualification-usd-20261102-v1`. OpenAI remains `gpt-6-luna`, reasoning `none`, explicit `standard`.

| Limit | Selected value |
| --- | --- |
| Actions/account/grant; rolling 24h | 30; 6 |
| Attempts/Action | Initial plus one deliberate unchanged-root known-failure retry |
| Calls/attempt | Two sequential OpenAI stages; zero TypeSafe |
| Complete input/call; cumulative/attempt | 64,000; 128,000 tokens |
| Output/call; cumulative/attempt | 6,000; 12,000 tokens |
| Attempt / Action expense ceiling | 30,000 / 60,000 microUSD |
| Shared account rolling day / grant | 300,000 / 2,000,000 microUSD |
| Shared global rolling day / pilot | 1,500,000 / 10,000,000 microUSD |
| Active Actions / outstanding calls | 1/account, 2/global, sequential/no queue |

Independent exact arithmetic is `(128000×125 + 12000×500)/1000 = 22000 microUSD`; one maximum reached call is 11,000. These are conservative historical reference exposures, not flat charges, upfront whole-attempt reservations or observed bills. Each reached request reserves its trustworthy complete input bound plus the full 6,000 output ceiling, rounded upward. The second request includes the generated proposal, instructions and schema; missing/oversized bounds deny before its send. No fact dropping, truncation, skipped support check, reduced output ceiling, model/tier substitution, fallback or deadline extension squeezes work in.

Account/global money, Action occupancy and outstanding-call authority include older qualification/ingestion rows and unknown holds. Failed/canceled admitted Actions count. Missing/partial/incompatible usage cannot become zero cost; existing category partition/subset checks and conservative estimates remain. Fake terminal proof may release only synthetic remote occupancy; missing usage retains money. Cancellation, restart, day rollover and 168-hour reviewed-closure eligibility never release money/capacity automatically or reset grant/pilot totals.

Stable duplicate creation/replay/status returns existing identity without resending durable intent. An unknown outcome cannot retry. One deliberate known-failure Retry uses a stable command key and new server attempt identity, with all current checks; no completed-stage reuse is assumed, so the new attempt follows the two-call path. A successful new rewrite is a fresh Action. Reconstructed Harness/pool evidence is distinct from an OS-process or physical-database restart.

## Proposal and Save ownership

The actual pilot Profile page uses an account/section-scoped managed client. Only its opaque recovery key enters session storage; career text and proposals stay in memory. Navigation reuses that owner, GET status never dispatches, and known failure needs a deliberate user click. EN/PT managed errors preserve the saved Profile and pending work; locale/source currentness withholds outdated replies. Proposal generation, support checking, display, editing and rejection never Save or debit Application Draft allowance. Existing review validation/provenance/unknown states and separate acknowledged explicit Profile Save remain authoritative, including lost-Save-reply recovery.

See [verification evidence](release-evidence/2026-11-04-managed-profile-review.md). This is local synthetic source/evidence handoff for existing #194, not remote publication, participant admission, production retention/recovery, real tokenizer/billing/model-quality verification, hosting, deployment or launch readiness.

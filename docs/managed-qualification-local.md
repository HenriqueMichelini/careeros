# Local synthetic managed qualification

This implements the bounded local slice of [#277](https://github.com/HenriqueMichelini/careeros/issues/277), using the [owner-selected qualification policy](https://github.com/HenriqueMichelini/careeros/issues/256#issuecomment-6589173667) and [bounded Q1/Q3 disposition](research/careeros-277-bounded-prerequisites-2026-11-02.md). GitHub remains the canonical tracker. No later workflow is enabled.

The durable branch integrates merged main `bafe318bfd638ef758a1fc1f6d324eebc71c6e08` and preserved #276 `448d06fb796526c5cc895336b8a5df8ce554fa0d`. Local integration `ae88bca69d9dfbf07c7cdeb8acc8cb6444d33b66` has the independently audited combined tree `7e86353d5a92dbfc975344636ad1d5b80fcde6b8`. This is local Git ancestry, not remote integration or issue closure.

`accountapi.NewSyntheticQualification` composes the actual qualification handler, authenticated saved Profile, #276 active subject-bound grant and existing restricted `aicontrol` ledger. It is selected only by explicit Go composition with `QualificationSyntheticConfig`; production startup and Function entrypoints never select it. Its responder is an in-memory fake and bypasses the HTTP client entirely. Static fake credential references satisfy the existing adapter contract; they are neither actual credentials nor caller spending authority. Caller credential headers/envelope fields are rejected. Qualification canonical/Function/account aliases share this composition, while other managed workflows remain unavailable.

The pilot frontend uses the actual AccountGate/PilotAccess/HomePage and omits key controls and key headers. It retains an opaque account-scoped Action creation key in sessionStorage, with career input, frozen context and results only in memory. Identical submissions recover the same Action. A known failure can authorize one deliberate retry with a stable retry-command key and a server-owned new attempt; unknown work cannot retry. A GET status read never starts inference or promises recovery of lost content. A deliberate new analysis gets a new creation key. Paused generation preserves the previous analysis and unused Draft allowance.

## Selected envelope and accounting

All amounts are integer microUSD, with fractional usage calculations rounded upward. Rate identity is `qualification-usd-20261102-v1`, TypeSafe `jev-1.13.0`, and OpenAI `gpt-6-luna`/reasoning `none`/explicit `standard` synthetic tier identity. The complete selected numeric envelope, rate identity, execution schema/prompt identities, persisted policy and scope ceilings are included in the immutable Action execution fingerprint. These frozen public-reference rates are not current account terms, measured usage or confirmed bills. Live wire/tier compatibility and a trustworthy production tokenizer remain unverified; actual egress stays unavailable.

| Limit | Value |
| --- | --- |
| Account actions | 20 per grant, 5 in rolling 24 hours |
| Attempts | 2 per Action; initial plus one deliberate known-failure retry |
| Calls per attempt | 1 TypeSafe and 2 OpenAI, sequential; only reached stages create rows |
| TypeSafe input | 32,000 full-request tokens |
| OpenAI input/output | 64,000 / 8,000 per call; output 16,000 per attempt |
| Attempt / Action | US$0.03 / US$0.06 |
| Account rolling day / grant | US$0.30 / US$2.00 |
| Global rolling day / pilot | US$1.50 / US$10.00 |
| Active Actions and outstanding calls | 1 per account, 2 globally |

At maximum selected token bounds, the independently expected full-attempt maximum is **US$0.025344 = 25,344 microUSD**: TypeSafe 1,344 plus two OpenAI maxima of 12,000 each. This implementation reserves **per reached dispatch**, not the entire attempt upfront. If only TypeSafe's durable intent exists, its hold is 1,344, not 25,344. Each reached leg checks aggregate attempt/Action and rolling/lifetime money, call/output maxima and capacity atomically under the existing policy/scope locks, then rechecks authority/time/bounds before the one-use intent and handoff. A partial workflow can pause when its next leg cannot be admitted. It never queues, silently truncates, skips original validation, retries, substitutes a model/tier or follows redirects.

The complete serialized provider request, including state/questions/prompt/context/schema, is passed to a code-installed **synthetic** trustworthy-bound seam. Missing or oversized bounds deny dispatch. Test `bytes + 64` bounds establish only an artificial fake-provider contract; they are not a production tokenization claim. Maximum input pricing uses the highest selected applicable input rate, including cache writes. Usage parsing preserves provider/model/tier/rate and category missingness. Cached input and cache writes partition input, and reasoning stays a subset of output. Missing/malformed/incompatible observations cannot reduce the conservative hold. Observed estimates remain estimates; no actual bill is inferred.

Explicit, code-owned fake terminal evidence is independent of usage/body completeness. It may release the corresponding synthetic remote slot, without reducing its money hold. HTTP success, body EOF, errors, timeout, cancellation, restart, clock rollover or 168 hours alone provide no terminal proof. Unresolved expense remains in rolling pools across day boundaries. Pilot lifetime totals never reset. Synthetic reviewed evidence and the separate reviewer role remain outside HTTP authority.

Durable intent is permanently non-resendable across duplicate submissions, lost acknowledgments and reconstructed Harness/pool objects. Existing independently rerun process-crash cases also exercise the shared authority. The 168-hour threshold remains reviewed-closure eligibility only; it causes no automatic erasure, money/capacity release, grant change or customer deadline. Restore opening still requires the independent synthetic gateway; no database row or fresh object proves an operational Q/D/R/F fence.

No career request/output content, provider credential or credential hash is stored in the ledger. Qualification does not reserve/debit Draft allowance, create durable result history, deliver a durable Draft or change Profile authority, whole-original Jev validation, source/version checks, incomplete outcomes, review semantics or the shared full-value projection corpus. Real providers, real tokenizer/terms/account compatibility, collection/cleanup, reconciliation authority, grant admission, operational recovery, hosting, deployment and spending remain disabled and separately gated.

See [verification evidence](release-evidence/2026-11-02-managed-qualification.md) for exact checkpoints and retained baseline failures.

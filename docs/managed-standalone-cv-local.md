# Managed standalone CV — local synthetic composition

This implements the bounded local slice of [#280](https://github.com/HenriqueMichelini/careeros/issues/280), under the exact [owner selection of 5 November 2026](https://github.com/HenriqueMichelini/careeros/issues/280#issuecomment-6671863934), policy `standalone-cv-local-20261105-v1`. GitHub remains the canonical tracker. Local commits do not close or remotely integrate the issue.

The cumulative base is `280f6954afb5d6a719d070cd8fe7bf62d059f99c`, including reviewed #279 `3d7e50b`/`280f695`, #278 `a389c04`/`65358fa`, #277 `304efae`/`9a759e5`, #276 `448d06f`, and merged #275 `bafe318`. Inherited contracts are [qualification](managed-qualification-local.md), [ingestion](managed-ingestion-local.md), [Profile review](managed-profile-review-local.md), and the [bounded Q1/Q3 disposition](research/careeros-277-bounded-prerequisites-2026-11-02.md). Frozen model/rates and shared pools remain the [2 November selection](https://github.com/HenriqueMichelini/careeros/issues/256#issuecomment-6589173667); [standalone rights](https://github.com/HenriqueMichelini/careeros/issues/256#issuecomment-6550695862) have zero Application Draft deduction.

## Explicit source path

`aicontrol.StandaloneCVSyntheticConfig` adds CV to `ProfileReviewSyntheticConfig` in the same namespace, without changing existing qualification, ingestion or review fingerprints. `accountapi.NewSyntheticStandaloneCV` composes the authenticated public CV handler, grant/account authority, direct Function and account aliases, all inherited managed paths, and provider-free CV status. Only code-installed fake responders and trustworthy synthetic bound functions can execute. Production startup, environment and Function composition do not select this constructor; actual egress, credential files/accounts and real grants remain unavailable.

The server rejects caller credential headers/fields/dispatch overrides and omitted/false `statementSupport`. Legacy locale and default-density spellings normalize before immutable root assignment, preventing them from relabeling the same unresolved assessment. The complete root binds the account, exact saved Profile/document fingerprint and stable source references/revisions, language, density, every provider-visible input, mandatory support contract and selected execution/budget/provider/rate identity.

`cvsource.Facts` mirrors the complete current outbound `outboundCvFacts(stableCvFacts(document))` view: original indivisible values, source order, stable references, owner/context and qualifiers; professional allowlisted fields only; entry groups anonymized consistently; evidence bodies and private/extra fields excluded. It does not use the broader career view or impose a full-Profile requirement. Both sparse skill and education sources remain valid. Matching occurs under the same account locks as Action creation, not only in a preflight read. Final response writes through the actual outer pilot flush recheck account/session/grant, saved Profile, execution policy and gate authority.

## Fixed assessed workflow and accounting

| Limit | Selected value |
| --- | --- |
| Admitted CV Actions/account/grant; rolling day | 8; 2 |
| Attempts | Initial plus one deliberate known generation/transport failure Retry with no reviewable candidate |
| Calls/attempt | Up to two sequential OpenAI requests: generation then independent support; zero TypeSafe |
| Complete input/call; cumulative attempt | 64,000; 128,000 tokens |
| Output/call; cumulative attempt | 4,000; 8,000 tokens |
| Expense ceiling/attempt; Action | 25,000; 50,000 microUSD |
| Shared account rolling day/grant | 300,000 / 2,000,000 microUSD |
| Shared global rolling day/pilot | 1,500,000 / 10,000,000 microUSD |
| Active Actions/outstanding calls | 1/account, 2 globally; sequential/no queue |

The fixed model is `gpt-6-luna`, reasoning `none`, explicit `standard`, rate ID `qualification-usd-20261102-v1`. Independent integer arithmetic gives `(128000 × 125 + 8000 × 500)/1000 = 20000 microUSD` per maximum attempt, 10,000 per reached call and 40,000 over two attempts. These are conservative historical reference exposures, distinct from 25,000/50,000 ceilings, observed usage and bills. Each reached dispatch atomically reserves its complete trustworthy input bound plus the full 4,000 output reserve. Unreached assessment is not charged an invented expense. The response-dependent second envelope includes generated review units and cited context. Missing/incompatible/oversized bounds deny before send; no truncation, fact dropping, output reduction, model/tier substitution, checker-only retry or deadline extension forces a fit.

The incoming body remains 128 KiB, with 1–500 facts and the existing 12 KiB per-fact and identity/context guards. The assessment's existing 128 KiB bound covers its INNER payload, not its whole provider envelope; generation has no separate asserted complete-body byte cap. A persisted per-attempt request witness fixes the 25-second inference/handler budget across zero-dispatch crashes and replay. The browser retains its 30-second budget. A permitted financial Retry gets a fresh witness; an old attempt never restarts its clock.

All old workflow expense and unknown occupancy contribute to the actual shared pools. Missing/incompatible usage preserves conservative exposure; existing provider category/subset accounting remains unchanged. An explicit synthetic terminal proof can release only occupancy, never infer zero expense from missing usage. Cancellation, restart, day rollover and 168-hour financial-review eligibility do not release unknown money/capacity or reset lifetime limits. The restricted ledger holds only keyed bindings, identifiers, budgets and outcome metadata, never CV/source/output content.

## Candidate, Retry and local acceptance

A structurally valid generated candidate with uncertain, unsupported or unavailable assessment stays schema 3 and available for manual review. Capacity/deadline failure after generation does not turn it into generation failure or discard it. Its already connected response may be returned with all statements uncertain/unavailable, under fresh account/Profile/policy/grant authority; this provider-free return does not admit another send or extend the inference clock.

A server-owned, account/root/saved-Profile/execution-scoped noncontent `cv_review_required` latch prevents a new creation key from laundering the same unresolved assessment. Caller assertions of local Accept cannot clear it. A genuinely changed source/language/density has a new binding; supported successful work permits a deliberate generate-another Action even for equal input. A valid candidate never grants Retry. Only a known generation/transport failure with no candidate may authorize the explicit stable Retry command and new server attempt. Unknown intent remains status-only and permanently non-resendable, even after Harness reconstruction and 169 hours; reconstruction is not a physical database or OS-process restart.

The actual pilot CV page omits caller keys, retains only an opaque account recovery key in session storage and keeps source/candidate content in memory. GET status never sends inference, saves content, rotates a key or claims durable content retrieval. Unknown work cannot silently dispatch; known failure requires a deliberate Retry. After reload there is no retained source binding: a deliberate new choice is checked against the server latch, which remains authoritative. Prior accepted CV snapshots stay intact until a replacement passes existing validation and its single local atomic write succeeds.

Existing statement-level edit, confirmation, discard/revert and at-least-one-reviewed-summary protections remain. Whole-CV Accept cannot promote unresolved machine wording. Actual authored/confirmed wording carries user provenance rather than inherited machine support. Legacy saved versions 1/2 remain locally readable/exportable with unverified wording status and zero calls. Display/copy/PDF and EN/PT behavior remain; no server CV history/content-retention class, durable result-recovery promise or Draft 30-minute clock is added.

See [verification evidence](release-evidence/2026-11-06-managed-standalone-cv.md). This source/evidence handoff remains local synthetic work, not real tokenizer/billing/model-quality verification, hosted qualification, participant admission, deployment or launch readiness. Historical #65/#52 quality limits remain.

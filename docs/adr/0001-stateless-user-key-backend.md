# Keep the v1 backend stateless and use user-provided AI keys

V1 keeps profiles and application drafts in the browser while the backend handles fixed AI workflows with user-provided keys. Profiles, proposals and keys remain under the existing browser-local ownership model; the backend does not persist them. This avoids accounts, a database and shared AI billing. The trade-offs are device-local data, browser-persisted keys, and a future migration step if server-side storage is added in v2.

## Approved provider amendment — 2026-10-07

Henrique approved TypeSafe Jev `jev-1.13.0` with the evaluation's explicit two-Choice rubric for field classification, retaining OpenAI for generation. This supersedes the original OpenAI-only provider decision for classification. The approval explicitly covers user-owned TypeSafe keys/credits, TypeSafe as an additional data recipient and this ADR amendment. It does not approve a shared key, shared billing or server-side persistence.

The categorical v1 policy blocks the whole submission for detected attacks, requests rephrasing for uncertain attacks and routes content sufficiency separately. Numeric confidence is not a safety gate. Operational failures stop processing; retries are deliberate, with no automatic provider fallback. Accepted text remains untrusted data and must pass downstream schema/domain/source validation and explicit Profile proposal review/apply.

Integration must disclose the additional recipient and TypeSafe key/credit requirement. TypeSafe's documented no-Input-training and US-hosting statements are documentation evidence; default retention is not numerically specified and no account-level retention audit was performed. These disclosed limits accompany the approved proposal. See the [evaluation and decision record](../evaluations/field-validation.md) for measurements, operational budgets and remaining checks.

## Implementation status

Generation workflows continue to use OpenAI. Issue #29's local evaluation is complete after the [user-authorized drafting fix and successful EN/PT timing pairs](../evaluations/field-validation-draft-fixed-results.md).

The [#30 implementation](../profile-field-validation.md) applies the approved Jev classifier before professional-information extraction/comparison, with a 3-second classifier deadline inside a shared 52-second workflow bound. It provides browser-local TypeSafe key settings, recipient/credit/privacy disclosure, typed feedback and input-use terms. The #31 implementation also gates qualification-gap checking and Application Draft generation on Job Posting validation. #32 preserves unknown title/company metadata. [Combined verification](../evaluations/field-validation-integration-results.md) records local evidence and the unresolved single-fact proposal outcome.

Local handler/browser checks do not establish live integration latency, production load reliability or deployment status. Maximum-input provider performance and load still require release verification.

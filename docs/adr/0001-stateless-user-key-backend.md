# Keep the v1 backend stateless and use user-provided AI keys

V1 keeps profiles and application drafts in the browser while the backend handles fixed AI workflows with user-provided keys. Profiles, proposals and keys remain under the existing browser-local ownership model; the backend does not persist them. This avoids accounts, a database and shared AI billing. The trade-offs are device-local data, browser-persisted keys, and a future migration step if server-side storage is added in v2.

## Approved provider amendment — 2026-10-07

Henrique approved TypeSafe Jev `jev-1.13.0` with the evaluation's explicit two-Choice rubric for field classification, retaining OpenAI for generation. This supersedes the original OpenAI-only provider decision for classification. The approval explicitly covers user-owned TypeSafe keys/credits, TypeSafe as an additional data recipient and this ADR amendment. It does not approve a shared key, shared billing or server-side persistence.

The categorical v1 policy blocks the whole submission for detected attacks, requests rephrasing for uncertain attacks and routes content sufficiency separately. Numeric confidence is not a safety gate. Operational failures stop processing; retries are deliberate, with no automatic provider fallback. Accepted text remains untrusted data and must pass downstream schema/domain/source validation and explicit Profile proposal review/apply.

Integration must disclose the additional recipient and TypeSafe key/credit requirement. TypeSafe's documented no-Input-training and US-hosting statements are documentation evidence; default retention is not numerically specified and no account-level retention audit was performed. These disclosed limits accompany the approved proposal. See the [evaluation and decision record](../evaluations/field-validation.md) for measurements, operational budgets and remaining checks.

## Implementation status

The currently deployed/implemented generation workflows continue to use OpenAI. Jev classification is approved architecture, not an integration or deployment claim. Issue #29 remains open while successful Application Draft overhead is resolved or explicitly accepted as unavailable; dependent integration work has not started. The proposed 3-second classifier target must fit inside shared remaining browser/workflow deadlines and still requires integration verification, including maximum inputs and load.

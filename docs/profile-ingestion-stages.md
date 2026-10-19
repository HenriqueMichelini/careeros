# Bounded Profile ingestion

Profile review uses two sequential HTTP requests to `/api/profile/ingest`:
`stage: {version: 1, kind: "extract"}` followed by
`stage: {version: 1, kind: "reconcile"}` with the returned `receipt`.
The source, Profile, optional canonical document, portion, clarification and Apply
context must remain equivalent across both requests. Object key order is ignored;
values and array order are preserved. Requests without `stage` retain the legacy
one-response protocol for older clients.

Each staged handler has a 27-second deadline from entry. Each OpenAI call has at
most 24 seconds, constrained by remaining time with 500 ms reserved for output
validation/serialization. The browser bounds each request, including body reads,
to 29 seconds and each explicit review attempt to 65 seconds. The unchanged
classifier has a three-second cap; both requests validate the complete original,
and clarification requests independently validate the question/answer context.
Normal staged ingestion therefore makes two Jev calls and two OpenAI calls.
No request waits for both OpenAI stages.

## Whole-submission amendment

The v1 flow above remains a **bounded unit**: at most one extraction result. The
whole-submission contract ([docs/submission-protocol.md](./submission-protocol.md),
issue #182) layers versioned `plan`/`extract`/`assemble`/`reconcile` stages on the
same route so one foreground action can run many bounded units sequentially.

It **amends #178's former per-click attempt**: the 27-second handlers, 29-second
browser requests and 65-second bounded-unit budgets are retained unchanged, and a
new **absolute 300-second foreground submission cap** is added. The cap includes
planning and all local work, finite manifest-based sequential dispatch, assembly
and reconciliation, with first-failure stop. There is no completion-time
guarantee, no rolling timeout reset per batch, no parallel provider fan-out and
no automatic retry/replan/fallback. The v1 staged and legacy callers, including
Apply enrichment, keep their existing behavior.

## Required server configuration

Before deploying updated clients, configure `PROFILE_INGESTION_SIGNING_KEY` in
Netlify environment variables, with the **Functions** scope for the intended
Deploy Preview, branch and Production contexts. Use a cryptographically random
secret of at least 32 characters (for example, a base64 encoding of 32 random
bytes). Keep it stable across invocations within an environment. Set the same
variable in the local Go API process for development. A missing/short key causes
staged requests to fail with HTTP 503 / `configuration` before provider dispatch.
Legacy requests do not require it.

This is a new server signing secret, independent of user OpenAI/TypeSafe keys.
Do not expose it through a `VITE_` variable, commit it, send it to an AI provider,
or log its value. Rotating it invalidates pending receipts, requiring an explicit
new review. The server has no generated fallback key and stores no receipt state.

Receipts use HMAC-SHA256 with domain separation, expire after ten minutes, and
bind the complete validated extraction result to the exact review context and
workflow versions. The payload is authenticated, **not encrypted**; it contains
transient claim excerpts already available to the reviewing browser. Maximum
decoded receipt payload is 128 KiB; the reconciliation body limit is 320 KiB.
Existing 30,000-byte source and Profile/document validation limits remain.
Capacity failures reject rather than truncate. Invalid/expired/changed receipts
are rejected before classification or generation.

## Review and recovery

The UI shows extraction and comparison as they start. It retains a receipt only
in the owning component's memory after a retryable comparison failure. A new
explicit Review action can retry comparison without extraction while that receipt
remains valid. Successful review, cancellation, edited text/context, unmount and
Profile changes invalidate it. Expiration reports a restart requirement without
silently extracting again; the next deliberate Review starts extraction.

HTML gateway 504 responses map to timeout; structured field decisions are still
preserved. Raw proxy pages, receipts, source content and keys never enter
diagnostics. Preparation, field-validation, extraction and reconciliation stage
metadata continue to use the existing sink. An extraction-only HTTP 200 is not
a completed Profile proposal, semantic acceptance or a saved Profile.

## Verification boundary

Issue #178 follows #41; deployed end-to-end acceptance remains under #52.
Controlled transport/browser tests exercise deadlines, handoff integrity,
explicit retry, cancellation, bilingual/mobile review and save boundaries.
They do not establish real-model quality or deployed latency. The reported
HTML inactivity 504 identifies a gateway failure but does not identify its owner.

Before release, pin deployment ID and commit, verify signing configuration, then
obtain separate authorization for an exact synthetic outbound payload and fixed
call cap. Record whole-request/stage durations and privacy-safe invocation
evidence, including failures and unavailable usage. No automatic retries or
provider/model fallback are part of this workflow.

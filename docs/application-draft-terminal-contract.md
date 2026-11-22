# Application Draft terminal resolution contract v1

This freezes the local engineering interface for [#286](https://github.com/HenriqueMichelini/careeros/issues/286) on base `1ecb2e0668e1c58373a5b3bb0fcb5768302e2efb`. The selected [handoff](https://github.com/HenriqueMichelini/careeros/issues/286#issuecomment-6645013429) consumes the [opening contract](https://github.com/HenriqueMichelini/careeros/issues/285#issuecomment-6644976301), [original deadline](https://github.com/HenriqueMichelini/careeros/issues/286#issuecomment-6644606585), [same-Action Retry](https://github.com/HenriqueMichelini/careeros/issues/255#issuecomment-6380266161) and [bounded managed Draft policy](https://github.com/HenriqueMichelini/careeros/issues/282#issuecomment-6645199452). It is a source contract, not a parallel issue tracker or verification result.

## Composition and public seams

An explicit local managed-terminal constructor composes the existing managed Draft graph and actual finite-five pilot reservation/opening authority. It accepts only the existing synthetic Harness and disposable PostgreSQL restrictions. Ordinary constructors and production Function/environment startup do not silently activate the new path. No external provider transport, account, credential or real grant is opened.

The agreed test seam is the authenticated Draft router/client with real disposable PostgreSQL and controlled providers. The first vertical slice is managed create → exact owner/Action cancellation → provider-free terminal retrieval, including duplicate cancellation and a late opening attempt. Later slices use the existing deliberate Retry admission and complete-opening protocol.

`POST /api/drafts/{key}/cancel` accepts only the exact server-issued Action identity. The authenticated owner and opaque creation key select the existing Action; caller flags, timestamps, allowance values and provider state have no authority. Unknown fields and mismatched owner/namespace/Action/workflow fail closed. A confirmed request returns the same authoritative Result: terminal no-consumption if cancellation won, or the original delivered result if opening won. Duplicate cancellation is idempotent. A lost request/reply remains uncertain until exact GET status recovers it.

`GET /api/drafts/{key}` and its exact-version/account aliases remain provider-free. The existing `POST /api/drafts` with `X-CareerOS-Retry-Key` is the explicit same-Action Retry command. Missing or changed input/source/context proof/policy binding cannot become Retry. Generate-another/rewrite/changed inputs use an explicit new creation key and independently admitted Action. The browser's local `cancel()` sequence invalidation is not the server cancellation command.

## Customer and provider outcomes

The shared Action, original durable admission, grant/activation and customer reservation are unchanged by status, receipt, Retry, browser abort or process restart. The applicable opening deadline stays strictly before `min(original admission + 30 elapsed minutes, original grant expiry, earlier authoritative terminal fence)`. Neither the 25-second workflow budget nor the 168-hour financial-review clock resets or substitutes for this boundary.

| Prior authoritative outcome | Winning event | Customer/result outcome |
| --- | --- | --- |
| Undelivered and still eligible | Complete bound opening wins before every fence | Original single delivered effect; duplicate cancellation cannot refund or add another. |
| Undelivered | Confirmed cancellation wins | Shared Action becomes terminal canceled, purpose result closes/queues cleanup, customer reservation releases once, no consumption. |
| Undelivered | Original deadline, grant expiry, revocation/deletion or stale source wins | Same terminal no-consumption contract; late opening, publication or Retry cannot revive it. |
| Delivered | Later cancel, Profile change, duplicate ACK, Accept/export or retrieval | Original one effect remains; historical content still follows current read/deletion rights. |
| Running/unknown | Lost reply, abort, timeout or local cancel | GET status first; no inferred terminal result, resend, new attempt or capacity release. |
| Known retryable failure and recoverable | Deliberate same-binding Retry passes fresh admission within original window | Exactly one second attempt and one purpose generation witness; all original intents/exposure remain. |
| Closed/deleted/canceled | Duplicate command, later event, restart or older-row restore | Existing terminal outcome; no second release, debit, revived witness or refreshed clock. |

All decisions hold the existing account/session and required grant/Action/customer locks, then recheck trusted database time and current authority after waits and immediately before committing. No transaction spans inference. Commands that observe a winning terminal fence persist that observation before denying further work. This includes Retry/recovery entry points, rather than relying on a later unrelated GET to close the Action.

Cancellation and customer-reservation release do not prove remote termination. Existing known/unknown money, dispatch intent, usage, provider occupancy and replay evidence remain unchanged. A new Action after terminal closure is still independently limited by those original shared pools.

## Retry classification and UI

The existing selected managed graph permits at most two attempts: initial plus one deliberate Retry of a known terminal field-validation/generation failure. Every reached dispatch must have compatible observed/independent terminal evidence. Unsupported/uncertain review, generic HTTP success/failure, running/unknown intent or missing evidence does not automatically qualify. No checker-only retry, hidden fallback or automatic resend is introduced. Original deadline and current allowance/grant/source/money/capacity can deny an otherwise numerically permitted Retry.

The EN/PT workspace distinguishes local request interruption, pending status recovery, pending cancellation, confirmed terminal no-consumption and prior delivery. It preserves independent same-account input and exact prior accepted/partial review identities. A failure or canceled replacement never overwrites that work. All protected copies still hide on current authority loss. A normal full opening remains the committed-artifact/support acknowledgment flow; Accept/export are separate.

## Independent verification expectations

Real public-route PostgreSQL tests cover both event orders against opening: cancel, original deadline, grant expiry, revoke/delete and Profile change, including blocked-row crossings and strict equality. Expected zero/one customer effects are independent literals; provider sends and unknown obligations are compared separately. Explicit Retry preserves original identity/time, allocates one attempt and never resends an old durable intent. Multi-process races, lost request/reply, real private PostgreSQL/fresh-server restart and bounded older-row restoration retain exact outcomes.

Actual React → authenticated Go → private PostgreSQL EN/PT cells verify status-first unknown recovery, deliberate cancel/Retry, full opening, lost acknowledgments, no hidden sends and preservation of older accepted work. Local restore tests retain independently held terminal/recovery authority; they do not establish hosted restore safety. All implementation and verification remain synthetic, fake-provider and disposable-storage work, with no provider, participant, deployment or GitHub mutation.

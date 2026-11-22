# Complete Application Draft opening contract v1

This freezes the engineering interface for [#285](https://github.com/HenriqueMichelini/careeros/issues/285) on cumulative base `1d5f7056131ce8749f31342053967ced49abbca8`. The [owner-selected opening contract](https://github.com/HenriqueMichelini/careeros/issues/285#issuecomment-6644976301) supersedes the earlier unapproved proposal. It consumes [#281's complete retained bundle](https://github.com/HenriqueMichelini/careeros/issues/281#issuecomment-6644621228), [five eligible deliveries per activated pilot grant](https://github.com/HenriqueMichelini/careeros/issues/256#issuecomment-6550695862), [the original-admission clock](https://github.com/HenriqueMichelini/careeros/issues/286#issuecomment-6644606585), and the existing historical/read/deletion fences. This is a source contract, not a ticket tracker or verification result.

## Local authority and activation boundary

The test seam is the authenticated Draft router/client over the existing repository. A separate explicit local opening constructor receives the controlled provider and the actual finite pilot-grant authority. Existing controlled/managed constructors do not silently activate it. No production Function or environment mounts the protocol. Managed terminal integration remains #286; it is not an upstream blocker or permission to implement that slice early.

Customer reservations reference the real `careeros_pilot_access` identity and original activation epoch, fixed five-delivery allowance and expiry. They never use `ai_fixture_grant.units`. The shared Action remains the identity, original durable admission remains the clock anchor, and every namespace counts against the same grant. Reservation precedes the one-use generation witness. The account lock serializes last-right races; one committed reservation/delivery occupies one unit. Reissuing an invitation, retrying, changing namespace, deleting content or recreating a process cannot create a new grant or reset usage.

Reservation and settlement are narrow code-owned commands in the purpose repository. Restricted AI runtime/reconciliation roles gain neither raw debit/release writes nor access to retained career content. The synthetic `Reviewer.Customer` boolean flags remain laboratory fixtures, not receipt or production authority. Provider allocations, expenses, remote states and capacity are untouched by customer settlement.

## Receipt and application confirmation

`POST /api/drafts/{key}/versions/1/opening` issues/retrieves a server receipt only for the current owner's exact retained complete bundle and an existing eligible reservation. It binds the shared Action, immutable bundle version/digest, complete manifest, current authenticated session, original grant epoch and the selected policy version. Receipt identity is persisted before reply; a lost issuance reply can recover the same receipt for that session without inference. A new session must obtain its own bound receipt. Browser-supplied ownership, delivery, completeness, deadline or allowance fields have no authority.

`POST /api/drafts/{key}/versions/1/opening/confirm` accepts the issued receipt and its exact manifest acknowledgment. It validates issuance, owner/session, Action/version/digest, complete retained content/support and the corresponding reservation. The application sends this command only after every required artifact and support snapshot is available in the actual review experience. Merely fetching the receipt or result, receiving HTTP 200, showing a partial artifact, Accept or export cannot confirm delivery. The receipt is evidence of the application protocol, not human comprehension.

The manifest fixes the role summary, resume, final cover letter with its frozen signature, explicit required/none-requested employer answers, support/completion snapshots, original source binding and artifact/review versions. It is verified against the retained exact bundle, never reconstructed using today's Profile. No additional career-content copy or prompt/history class is retained.

`GET` returns the original exact Action/version and authoritative reservation/delivery outcome without inference. A confirmation committed before response loss is recovered rather than resent as new delivery. Duplicate ACK, cross-tab ACK, repeat retrieval, reauthentication, human Accept and export have at most the original one effect. Invalid/partial/mixed/forged evidence has zero effect.

`GET /api/draft-allowance` reports total, reserved, consumed and remaining real pilot rights. This metadata path is separate from `/api/drafts/{key}` so it cannot shadow a successfully admitted opaque creation key.

## Ordering and original clocks

The lock order is existing account/session authority, actual pilot grant, shared Action and customer reservation, then purpose content/receipt. All winning outcomes recheck session, grant/source/deletion authority and `clock_timestamp()` after waits and immediately before commit. No inference runs inside a database transaction.

The applicable confirmation deadline is the minimum of original durable admission plus 30 elapsed minutes and original grant expiry, subject to any earlier authoritative terminal fence. Eligibility is strict: confirmation must win **before** the deadline. Reload, status, new receipt, same-Action Retry, process restart and restored data cannot refresh the original anchors. The 168-hour provider financial-review clock is unrelated.

| Prior authoritative outcome | Winning event | Committed customer result |
| --- | --- | --- |
| Reserved and undelivered | Valid complete confirmation before every fence/deadline | Delivered; exactly one consumed right; reservation no longer held. |
| Reserved and undelivered | Deadline/expiry or authoritative cancellation/revocation/deletion wins | Terminal no-consumption; customer reservation released once; late confirmation remains ineligible. |
| Ready but not yet delivered | Source Profile identity/revision changes before confirmation | Terminal stale without consumption; purpose content inaccessible/queued; new generation requires explicit new Action. |
| Delivered | Profile edit, duplicate ACK, reopen, Accept/export or later cancellation | Original single effect retained; current historical/read/deletion rights still govern content. |
| Terminal no-consumption | Late publication/ACK, duplicate event, restart or restore | Original terminal result; no debit, repeated release, revived Action or reset clock. |
| Any | Lost request/reply or browser abort without authoritative outcome | Recover exact existing state; do not infer success, cancellation, zero provider expense or new generation permission. |

Both event orders are tested for confirmation versus deadline, grant expiry, source edit, revocation/deletion and the selected injected cancellation fence, including blocking-lock crossings, duplicate events, lost acknowledgments and actual process/PostgreSQL restart. A stale request/transaction-start/browser timestamp cannot win. #286 integrates its deliberate cancellation/Retry orchestration; #285 freezes and exercises the shared terminal outcome contract through controlled authority.

Terminal customer resolution denies/queues undelivered purpose content using #281's interface and preserves independently required replay/expense evidence. It never proves remote termination or releases provider occupancy. Delivered content retains the original post-grant 30-day access window and earlier deletion fences; that is not measured from receipt issuance or publication.

## Independently expected verification

Public-route PostgreSQL tests independently expect one reservation before generation, zero consumption for ready/unopened or partial acknowledgment, one winning delivery for duplicate/cross-tab races, and no extra effect after response loss/restart. Last-unit races use an actual finite-five pilot grant; wrong account/session/version/manifest and raw runtime debit/release attempts fail closed. Scope/expense/unknown capacity and unrelated Profile state are compared before/after customer transitions.

The real local React browser exposes all required artifacts/support before ACK. Partial-opening and withheld-ACK fixtures show zero consumption; full opening and lost-confirmation reply recover the same exact version/effect. EN/PT status remains honest and preserves independent unsaved input. These are synthetic local protocol checks, not live model-quality, hosted recovery, actual grant/admission or deployment evidence. The combined operational path stays unactivated until #286 integrates terminal resolution.

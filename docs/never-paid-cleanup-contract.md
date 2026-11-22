# Never-paid lifecycle and purpose-bound cleanup

Current implementation slice: [#289](https://github.com/HenriqueMichelini/careeros/issues/289), from cumulative local base `63b20b29d16de955c77869c9faf417dc642e9a75`. This freezes engineering interfaces for synthetic verification; it does not configure a scheduler, send account email or operate a participant/provider account.

## Selected sources

- Never-paid access and notices: [6550859196](https://github.com/HenriqueMichelini/careeros/issues/258#issuecomment-6550859196), supplemented by the in-app/account-email and failure rule in [6644628888](https://github.com/HenriqueMichelini/careeros/issues/289#issuecomment-6644628888).
- Never-activated invitations: [6645201526](https://github.com/HenriqueMichelini/careeros/issues/289#issuecomment-6645201526).
- Excerpt cleanup: [D03](https://github.com/HenriqueMichelini/careeros/issues/258#issuecomment-6324889579) and [D04](https://github.com/HenriqueMichelini/careeros/issues/258#issuecomment-6324928436).
- Deletion and earliest known access end: [6645200501](https://github.com/HenriqueMichelini/careeros/issues/288#issuecomment-6645200501); undelivered content: [6644621228](https://github.com/HenriqueMichelini/careeros/issues/281#issuecomment-6644621228) and [6644606585](https://github.com/HenriqueMichelini/careeros/issues/286#issuecomment-6644606585).
- Recovery-copy ceiling: [D06](https://github.com/HenriqueMichelini/careeros/issues/258#issuecomment-6324994944); restricted survivors: [6645082417](https://github.com/HenriqueMichelini/careeros/issues/283#issuecomment-6645082417).

## Clocks and records

| Class | Original clock / eligibility | Effect and survivors |
| --- | --- | --- |
| Activated pilot | Earliest trustworthy scheduled expiry/effective early end | No new AI at end; existing read/export rights end strictly at end +30 elapsed days. Unknown inherited early end remains unknown and denied, never assigned a new window. Pauses/holds preserve original clocks. |
| Retained career content | End +30 elapsed days, or earlier explicit deletion | A trusted bounded cleanup worker reuses Profile tombstones and purpose fences before removing canonical Profile/provenance and bound Draft/history. Sign-in and original finite grant history remain. Each original Profile identity has its own idempotent removal operation; no broad content hash deletion. |
| Undelivered Draft | Original admission +30 minutes or earlier terminal event | Observe the existing terminal contract, queue and remove only its purpose content; delivered customer effects and independently authorized content survive. This is not a new 30-day undelivered window. |
| Never-activated invitation | Original seven-day expiry or earlier revoke | Invalidate immediately; remove invitation-only eligibility result/check/operator and duplicated invitation identity payload. Preserve opaque denial/replay identity and independently authorized account data. No grant or post-grant window starts. Reissue is explicit, new identity/expiry and fresh clearance. |
| Excerpts | No remaining fact references, or deletion of a fact supported by that excerpt | Remove affected excerpt, retain unrelated facts/evidence, invalidate affected support/links. Historical invalidated links cannot retain excerpt bytes. Use explicit Profile edits plus the existing strict Save guard; bounded repair of existing orphan evidence changes only this selected class and uses a new canonical revision. |
| Recovery / recipient obligations | Original durable deletion fence +30 days maximum for recovery copies | Original anchors never move on retry, copy or restore. Status remains unknown/pending/failed or separately proven; a deadline alone does not prove disposal. Hosted/recipient operations are unavailable. |
| Financial / replay / deletion evidence | Existing purpose-based reviewed rules | No automatic timer compaction, key retirement, remote capacity release, expense reduction or allowance replenishment. Existing explicit reconciliation capabilities remain the only authority. |

## Public and worker interfaces

`Store.WithLifecycleStatus` is a freshly authenticated owner-only metadata reader through the actual outer response write. It exposes due expiry/removal notices, bounded delivery state, cleanup status and residual obligation status, without career content, account email, provider subject, ledger details or credentials. Reading status starts no cleanup or email work.

An explicitly code-injected local lifecycle worker processes a bounded ordered batch. It uses the existing pilot/account/purpose/shared/allowance lock order, reads trusted database time after waits and commits the relevant original anchor/fence before reporting work. Each account/Profile/invitation/notice identity has stable idempotency; replays never extend clocks. A normally buildable synthetic-only loopback `*_test` CLI exposes this bounded worker without a public mutation/scheduler endpoint or environment-enabled real recipient transport.

Notices become visible in-app at access end and seven elapsed days before removal. The separate code-owned email handoff receives only the current account recipient and a fixed notice kind/deadline. Its durable stable-key intent precedes the callback, outside any database transaction. Known failure permits at most three total attempts at the original due time, +1 hour and +24 hours; unknown outcome never automatically resends. Failure/unknown/unavailable recipient remains visible and signals an operator record. Neither delivery failure nor a retry changes cleanup eligibility. These bounded local retry mechanics are engineering choices, not a live email/provider promise.

Residual outcomes use a separate code-injected evidence adapter; no customer `complete=true` assertion or timer supplies proof. The initial arrangement has no real recipient/disposal adapter, so unknown and pending remain truthful. Recovery-copy deadline breach is an actionable overdue status, not a fabricated erasure success. Existing sealed deletion handoff preserves original anchors; applying an older lifecycle snapshot cannot reopen or re-age an existing fence.

## Independently expected verification seams

Use meaningful TDD at: the bounded worker plus actual authenticated HTTP status/Profile/export reader; explicit Profile edit and acknowledged Save/reload; fake email/outcome handoff with stable intent; disposable PostgreSQL races/restart. First vertical tracer: an expired pilot at its removal boundary loses retained Profile bytes and gains one durable metadata cleanup operation, while another account and expense/replay protections remain unchanged. Then cover strict boundaries/early end/unknown end, notices and known/unknown delivery failure, never-activated invitation cleanup/reissue, shared excerpt deletion, mixed records, lost acknowledgment, cleanup failure/retry, restarted processes/original anchors and foreign/session denial. Actual EN/PT React metadata and error behavior must use the real local router where practical. No actual scheduler/storage/recipient operation is part of these tests.

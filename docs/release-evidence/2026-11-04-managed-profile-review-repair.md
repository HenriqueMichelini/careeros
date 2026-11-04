# Managed Profile review: reload recovery repair

Original review checkpoint: `3d7e50b77a994bc3e68c1cbdc6f4aa38931085d1`, based on cumulative `65358faafe8a88e9539bdd91e59a863f25a7ff65`. The [original verification record](2026-11-04-managed-profile-review.md) and its failures remain historical, not replaced. See [the repair summary and exact tested blobs](2026-11-04-managed-profile-review-repair-summary.json).

Independent Spec review confirmed a P2: after actual reload, only the opaque old Action key survived. The fresh client did not recognize authoritative `result_unavailable` metadata as finished successful work, so the next deliberate unchanged rewrite reused the old Action and remained status-only indefinitely. The owner contract says a successful new rewrite is a new Action. Original independent Standards had zero hard violations and one nonblocking duplicated-guard observation; security had zero confirmed vulnerabilities and independently passed 29 PostgreSQL/race events at the original head. Those are original-head reviews; this repair requires explicit final-head followups.

The only runtime change is `ManagedProfileReview.status`: validated `result_unavailable` with `retryAllowed:false` marks success in memory for the **next deliberate command**. The GET itself never rotates/persists a key, posts inference, retrieves proposal content or Saves. That next explicit rewrite resets the old success identity and allocates a new Action, subject to the unchanged server authority. Unknown status clears the success marker and retains its existing identity; known failure retains the existing deliberate stable-key Retry. Account, section and changed-root isolation remain intact. No new storage field, provider call, grant or policy selection is added.

## RED → GREEN

The fresh-instance Node regression first failed at the expected assertion: unchanged old/new Action keys after a finished-success GET. The actual App browser reload regression also failed at the same key assertion with the original runtime. Its fake server retains only completed opaque keys across reload and returns 202 for a duplicate completed Action, so HTTP success cannot hide the defect. The first browser attempt stopped earlier on an unsynchronized old-document click during reload; waiting for the new scenario before opening the Profile corrected test infrastructure only. The intended product regression was then observed separately before the runtime fix.

After the eight-line runtime addition:

- **2/2 focused managed review client tests pass**, covering fresh instances for finished success, unknown and known failure, GET-only behavior and unchanged persisted key until an explicit command, stable Retry, changed-root/account/section isolation.
- **371/371 Node tests pass**, including all **44 full-value Go/TypeScript projection cases**.
- TypeScript and pilot build pass. Build output is `/tmp/careeros-279-review-repair-build`; inherited Vite config/chunk-size warnings remain.
- **6/6 actual App Chromium cells pass in EN/PT**: the existing normal proposal/edit/reject/acknowledged Save flow; unknown→navigation→GET→deliberate known-failure Retry; and new actual page reload→finished-success GET→deliberate same-root new Action. GET/reload have zero inference POSTs and zero Saves, keep the opaque old key until the explicit rewrite, and never claim old proposal retrieval. The new Action produces a transient proposal which still needs explicit Save. This is synthetic browser transport, not live model or hosted evidence.

All **eight Go runtime blobs** in the original tested manifest are unchanged. No Go or PostgreSQL test was rerun for this frontend-only repair; the original **1044 relevant PG/race passing events and 37/37 case IDs**, **2287 full-Go passing events with exit 1**, **25 matching-base Lab paths**, extra retained A35 full-run timing failure, helper skips, toolchain/vet diagnostics and isolated checks remain exactly attributed to the original checkpoint. They are not relabeled as fresh repair runs or a green full suite. The new manifest binds every inherited runtime blob plus the repaired client blob to this final source.

Original dirty research checkout remains untouched. No later-ticket investigation, remote GitHub write, provider account/key access or actual call/spending, grant/admission, deployment/configuration or operational action occurred.

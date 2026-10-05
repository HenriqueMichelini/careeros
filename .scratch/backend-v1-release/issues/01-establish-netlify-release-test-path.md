# 01 — Establish the Netlify release-test path

**What to build:** A tester can open the intended release build in the Netlify pull request preview and deployed app, reach the same-origin Go workflow, and collect safe evidence for later provider-backed checks. The Netlify pull request preview replaces Figma Make as the v1 preview release environment.

**Blocked by:** None — can start immediately.

**Status:** successful/completed (2026-10-04). The matched Netlify revision, automated checks, credential-free live checks, read-only Function logs, and tester's manual new-user flow through Profile Review establish the release-test path for the current v1 milestone.

- [x] Update the v1 release requirements to name the Netlify pull request preview and deployed app as the two release environments, and record the exact build revision tested in each. If the revisions differ, identify the deployment action needed before provider-backed verification.
- [x] In both environments, the app loads over HTTPS and a credential-free Profile review request reaches Go, returns its expected error without a provider call, and uses `no-store` rather than caching user content.
- [x] Establish an approved method for synthetic provider-backed checks, duration measurement, and read-only Function log review. Do not send the vault key to Netlify from this agent or record credentials, Profile text, or generated content in diagnostic logs.
- [x] Record how testers will distinguish browser duration, Function duration, and provider duration, and where metadata-only evidence for each environment will be kept.

Evidence and the safe procedure: [docs/release-test-path.md](../../../docs/release-test-path.md). Netlify published the PR preview deploy at full revision `910d8ad9b1bc29937d2b6391f895345518094c1c`. On 2026-10-04, both HTTPS homepages returned HTTP 200; keyless Profile review POSTs returned Go JSON HTTP 401 `key`, `Cache-Control: no-store`, and Netlify cache bypass/miss. Those baseline requests made no provider call. The tester then reported a successful manual new-user flow in the deployed application, including Profile Review. Two read-only Netlify Function entries show provider-backed Profile Review `status=200 outcome=ok` at approximately 4.23 s and 9.54 s; see the [manual evidence record](../../../docs/release-evidence/2026-10-04-profile-review-manual.md).

Separate synthetic-payload and generated-output inspection, per-invocation origin attribution, and additional provider calls were not recorded. These are informational evidence limits for Ticket 01, not blockers to its completion. The procedure above remains available for later release work; closing this ticket requires no further provider call and does not mark Ticket 02 complete.

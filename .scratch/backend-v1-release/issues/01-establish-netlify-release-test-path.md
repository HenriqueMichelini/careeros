# 01 — Establish the Netlify release-test path

**What to build:** A tester can open the intended release build in the Netlify pull request preview and deployed app, reach the same-origin Go workflow, and collect safe evidence for later provider-backed checks. The Netlify pull request preview replaces Figma Make as the v1 preview release environment.

**Blocked by:** None — can start immediately.

**Status:** accepted (2026-10-04) on matched Netlify revision and credential-free checks in both environments. Provider-backed workflow checks and actual Function-log review remain later release work.

- [x] Update the v1 release requirements to name the Netlify pull request preview and deployed app as the two release environments, and record the exact build revision tested in each. If the revisions differ, identify the deployment action needed before provider-backed verification.
- [x] In both environments, the app loads over HTTPS and a credential-free Profile review request reaches Go, returns its expected error without a provider call, and uses `no-store` rather than caching user content.
- [x] Establish an approved method for synthetic provider-backed checks, duration measurement, and read-only Function log review. Do not send the vault key to Netlify from this agent or record credentials, Profile text, or generated content in diagnostic logs.
- [x] Record how testers will distinguish browser duration, Function duration, and provider duration, and where metadata-only evidence for each environment will be kept.

Evidence and the safe procedure: [docs/release-test-path.md](../../../docs/release-test-path.md). Netlify published the PR preview deploy at full revision `910d8ad9b1bc29937d2b6391f895345518094c1c`. On 2026-10-04, both HTTPS homepages returned HTTP 200; keyless Profile review POSTs returned Go JSON HTTP 401 `key`, `Cache-Control: no-store`, and Netlify cache bypass/miss. No valid key or provider request was used. The procedure requires a separately approved synthetic payload and dedicated test key for later provider-backed checks; this ticket does not authorize such a call.

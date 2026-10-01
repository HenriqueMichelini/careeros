# 02 — Deploy Profile review on Netlify Free

**What to build:** A user can open the deployed Vite app and complete the same Go-backed Profile review over HTTPS. The frontend and Go workflow run together on Netlify Free.

**Blocked by:** 01 — Profile review through Go in Figma Make preview.

**Status:** accepted — live Netlify Free deployment and Profile review verified 2026-09-30.

- [x] The deployed app serves the existing frontend and exposes the Profile review Go workflow on the same origin without requiring sign-in, a database, or an application-owned AI key. Production URL: <https://beamish-bavarois-333d03.netlify.app/>. The deployed `index-BRRwYoGq.js` was independently checked for the `experience.current` boolean fix from merged PR #1 (`92ab54b59f3aed37341ae2dc625865d07966e1b2`, containing `acaa6e1`).
- [x] A real OpenAI review using the fixed `gpt-6-luna` model succeeds on the deployed app and returns the validated response shape. The user observed HTTP 200 on `POST /api/profile/review` (`x-nf-request-id: 01M3T86WP0DH87ZB48C5GWXQWH`, `Date: 2026-09-30 22:51:02 GMT`); schema, IDs, and salary were preserved, and the returned edits remained after page reload. A separate credential-free POST with an empty JSON body on 2026-10-01 returned HTTP 401 `{"error":"key"}` (`x-nf-request-id: 01M3VBTTJ4Q8QV9HRVTBZ2D0VY`) and `cache-control: no-store`, confirming the deployed missing-key error contract. This live check did not exercise other error responses.
- [x] A representative live review completes within Netlify's 60-second synchronous Function limit. Measured browser total request time was 13.09 s. The corresponding Function log reported `duration_ms=11527`; Netlify platform metadata reported `Duration: 11529.55 ms` and `Memory Usage: 42 MB`. These are distinct browser and invocation measurements, both below 60 s.
- [x] Netlify Free remains selected with no automatic paid upgrade. Effective 2026-09-30 billing evidence showed $0, 300 credits/month, “no overage charges ever,” and 30.3 credits used. Whole-site pause on credit exhaustion is documented in ADR 0002 and backend requirements; the frontend maps network errors, unknown HTTP errors, and provider outages to a localized unavailable message and clears the reviewing state.
- [x] Deployed transport, response caching, and logs were checked. The response used `no-store`; Netlify cache status was bypass/miss. The supplied Function log contained only operational metadata (`profile_review status=200 outcome=ok duration_ms=11527`), with no profile content or API key. No backend persistence was involved.

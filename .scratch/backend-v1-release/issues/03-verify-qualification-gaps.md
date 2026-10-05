# 03 — Verify qualification gaps in both release environments

**What to build:** A user can check a synthetic job posting against a synthetic Profile through Go and OpenAI in both release environments, inspect relevant suggested qualifications, and choose which ones apply without changing the saved Profile.

**Blocked by:** 01 — Establish the Netlify release-test path.

**Status:** successful/completed (2026-10-05). The two provider-backed qualification-gap checks, output review, unchanged-Profile checks, timing evidence, and Function-log privacy review satisfy this ticket. The gap responses' own headers were not retained; Ticket 02's live `no-store` and cache-bypass captures for both origins provide the accepted shared response-header evidence.

- [x] Before any real request, verify the exact Go-to-OpenAI payload uses only the approved qualification projection and posting; it excludes dedicated compensation, employment status, names, IDs, URLs, locations, career goals, additional information, and experience recency.
- [x] A real gap check succeeds through the browser and Go in the pull request preview and deployed app. Inspect the result for relevance, no invented work history, and at most five suggestions; suggestions remain unchecked until explicitly confirmed and do not alter the saved Profile.
- [x] Record response status and browser, Function, and provider durations available in each environment; each synchronous Function call completes within 60 seconds without an automatic retry.
- [x] Review actual Function logs and response headers for both runs: logs contain operational metadata only, and neither the key nor Profile/posting/suggestions are stored or cached by the backend.

Evidence: [qualification-gap release verification](../../../docs/release-evidence/2026-10-04-qualification-gaps-manual.md). The two browser checks and matching Function logs passed. The gap responses' own headers were not retained; the accepted header evidence comes from Ticket 02's live captures for both origins and source review of the shared `no-store` response behavior.

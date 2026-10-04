# 03 — Verify qualification gaps in both release environments

**What to build:** A user can check a synthetic job posting against a synthetic Profile through Go and OpenAI in both release environments, inspect relevant suggested qualifications, and choose which ones apply without changing the saved Profile.

**Blocked by:** 01 — Establish the Netlify release-test path.

**Status:** ready-for-agent

- [ ] Before any real request, verify the exact Go-to-OpenAI payload uses only the approved qualification projection and posting; it excludes dedicated compensation, employment status, names, IDs, URLs, locations, career goals, additional information, and experience recency.
- [ ] A real gap check succeeds through the browser and Go in the pull request preview and deployed app. Inspect the result for relevance, no invented work history, and at most five suggestions; suggestions remain unchecked until explicitly confirmed and do not alter the saved Profile.
- [ ] Record response status and browser, Function, and provider durations available in each environment; each synchronous Function call completes within 60 seconds without an automatic retry.
- [ ] Review actual Function logs and response headers for both runs: logs contain operational metadata only, and neither the key nor Profile/posting/suggestions are stored or cached by the backend.

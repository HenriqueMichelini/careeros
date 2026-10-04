# 02 — Verify Profile review in both release environments

**What to build:** A user can review a synthetic Profile through Go and OpenAI in both the Netlify pull request preview and deployed app, with a usable reviewed result and evidence of timing, factual preservation, and privacy.

**Blocked by:** 01 — Establish the Netlify release-test path.

**Status:** successful/completed (2026-10-04). Controlled synthetic Profile reviews and dedicated network captures succeeded in both release environments with payload, output, reload, timing, response-header, cache, and Function-log privacy evidence.

- [x] Before any real request, verify that the exact provider payload contains only the previously approved minimized synthetic fields. If the Profile review contract requires excluded data, stop for a scope decision instead of sending it.
- [x] A real Profile review succeeds through the browser and Go in both release environments. Inspect the reviewed result for completeness and grammar improvements without invented names, roles, dates, numbers, or qualifications; verify its intended save behavior after reload.
- [x] Record the response status and browser, Function, and provider durations available in each environment; each synchronous Function call completes within 60 seconds without an automatic retry.
- [x] Review the actual Function logs and response headers for both runs: logs contain operational metadata only, and neither the key nor Profile/result content is stored or cached by the backend.

Evidence: [manual Profile review evidence](../../../docs/release-evidence/2026-10-04-profile-review-manual.md). The final controlled runs used the tester-confirmed dedicated key and the full approved synthetic fixture, including competencies and one fictional experience. A source-derived preflight verified the exact Go-to-OpenAI envelope and the serialized Profile fields before either request. The preview and published reviews returned `200 ok`, preserved every approved fact after reload, and introduced no names, roles, dates, numbers, qualifications, projects, or experience entries. Browser waits were 9.33 and 14.44 seconds; Function handler durations were 8,075 and 13,065 ms. Both responses used `Cache-Control: no-store`, Durable bypass, and Edge miss with distinct request IDs. All matching Function logs were metadata-only, and provider-only timing is unavailable because the deployed handler records the full Function duration.

# 02 — Verify Profile review in both release environments

**What to build:** A user can review a synthetic Profile through Go and OpenAI in both the Netlify pull request preview and deployed app, with a usable reviewed result and evidence of timing, factual preservation, and privacy.

**Blocked by:** 01 — Establish the Netlify release-test path.

**Status:** partial (2026-10-04). Two successful Profile review Function invocations were observed, but the required two-environment browser, synthetic-payload, output, and privacy evidence is incomplete.

- [ ] Before any real request, verify that the exact provider payload contains only the previously approved minimized synthetic fields. If the Profile review contract requires excluded data, stop for a scope decision instead of sending it.
- [ ] A real Profile review succeeds through the browser and Go in both release environments. Inspect the reviewed result for completeness and grammar improvements without invented names, roles, dates, numbers, or qualifications; verify its intended save behavior after reload.
- [ ] Record the response status and browser, Function, and provider durations available in each environment; each synchronous Function call completes within 60 seconds without an automatic retry.
- [ ] Review the actual Function logs and response headers for both runs: logs contain operational metadata only, and neither the key nor Profile/result content is stored or cached by the backend.

Current metadata-only observations: [manual Profile review evidence](../../../docs/release-evidence/2026-10-04-profile-review-manual.md). Netlify shows two `200 ok` Function invocations with handler durations 4,232 and 9,541 ms; their visible log lines contain operational metadata only. The tester identified the PR preview endpoint and reported a successful manual result, but the logs do not identify each request origin. The approved synthetic Profile and exact provider payload were not confirmed, and the dedicated-key procedure was not followed. Do not infer published-app completion, output quality, or storage/privacy acceptance from these log entries.

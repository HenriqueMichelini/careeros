# 04 — Verify Application Draft in both release environments

**What to build:** A user can confirm a synthetic suggested qualification and generate an Application Draft through Go and OpenAI in both release environments, with complete, factually grounded content and evidence of timing and privacy.

**Blocked by:** 01 — Establish the Netlify release-test path; 03 — Verify qualification gaps in both release environments.

**Status:** successful/completed (2026-10-05). The tester accepted the two provider-backed drafts, factual review, Function timings and privacy evidence, browser-observed HTTP 200 and sub-10-second duration, and shared live `no-store` header checks. Exact per-origin browser timing and headers from the successful responses were not captured; these are recorded evidence limits, not remaining acceptance gates.

- [x] Before any real request, verify the exact provider payload contains only the previously approved minimized synthetic Profile fields, job posting, and explicitly confirmed qualification context. Do not send excluded Profile data to OpenAI during the test.
- [x] A real draft succeeds through the browser and Go in the pull request preview and deployed app. Inspect its job title, company, role summary, résumé, cover letter, and application answers against the synthetic facts; record unsupported claims or omissions rather than accepting field presence alone.
- [x] A qualification confirmed for the first draft appears only in that draft. A new application starts with no confirmed qualifications; the saved Profile remains unchanged.
- [x] Record response status and browser, Function, and provider durations available in each environment; each synchronous Function call completes within 60 seconds without an automatic retry.
- [x] Review actual Function logs and response headers for both runs: logs contain operational metadata only, and neither the key nor Profile/posting/draft content is stored or cached by the backend.

## Comments

- 2026-10-05: [Application Draft release verification](../../../docs/release-evidence/2026-10-05-application-draft-manual.md) records the two successful drafts, factual review, temporary qualification behavior, Function timings, and metadata-only logs. The published result used a generic candidate-name placeholder because the Profile has no name field. The preflight used source review and an outbound-body test with the approved synthetic field shape; it did not capture a live provider payload. The tester reported HTTP 200 and browser duration under 10 seconds without exact per-origin captures. Successful 200 response headers were not captured. Keyless requests confirmed `no-store` and Netlify cache bypass for the same handler, but are not direct evidence of the successful-response headers.
- 2026-10-05: The tester explicitly accepted this evidence as sufficient to close ticket 04. The successful responses' exact browser timings and headers remain uncaptured; acceptance uses the observed sub-10-second browser result, per-origin Function timings and logs, and live keyless `no-store`/cache-bypass headers from the same handler. No further provider requests are required for this ticket.

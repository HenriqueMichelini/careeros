# 04 — Verify Application Draft in both release environments

**What to build:** A user can confirm a synthetic suggested qualification and generate an Application Draft through Go and OpenAI in both release environments, with complete, factually grounded content and evidence of timing and privacy.

**Blocked by:** 01 — Establish the Netlify release-test path; 03 — Verify qualification gaps in both release environments.

**Status:** partial (2026-10-05). Two provider-backed Application Drafts succeeded, but browser duration and successful-response headers were not captured. A fresh Network capture in each origin is needed to close this ticket.

- [x] Before any real request, verify the exact provider payload contains only the previously approved minimized synthetic Profile fields, job posting, and explicitly confirmed qualification context. Do not send excluded Profile data to OpenAI during the test.
- [x] A real draft succeeds through the browser and Go in the pull request preview and deployed app. Inspect its job title, company, role summary, résumé, cover letter, and application answers against the synthetic facts; record unsupported claims or omissions rather than accepting field presence alone.
- [x] A qualification confirmed for the first draft appears only in that draft. A new application starts with no confirmed qualifications; the saved Profile remains unchanged.
- [ ] Record response status and browser, Function, and provider durations available in each environment; each synchronous Function call completes within 60 seconds without an automatic retry.
- [ ] Review actual Function logs and response headers for both runs: logs contain operational metadata only, and neither the key nor Profile/posting/draft content is stored or cached by the backend.

## Comments

- 2026-10-05: [Application Draft release verification](../../../docs/release-evidence/2026-10-05-application-draft-manual.md) records the two successful drafts, factual review, temporary qualification behavior, Function timings, and metadata-only logs. The published result used a generic candidate-name placeholder because the Profile has no name field. The preflight used source review and an outbound-body test with the approved synthetic field shape; it did not capture a live provider payload. Browser duration and successful 200 response headers remain outstanding. Keyless requests confirmed `no-store` and Netlify cache bypass for the same handler, but cannot replace the successful-response captures.

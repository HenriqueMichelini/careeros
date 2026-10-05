# 04 — Verify Application Draft in both release environments

**What to build:** A user can confirm a synthetic suggested qualification and generate an Application Draft through Go and OpenAI in both release environments, with complete, factually grounded content and evidence of timing and privacy.

**Blocked by:** 01 — Establish the Netlify release-test path; 03 — Verify qualification gaps in both release environments.

**Status:** ready-for-agent

- [ ] Before any real request, verify the exact provider payload contains only the previously approved minimized synthetic Profile fields, job posting, and explicitly confirmed qualification context. Do not send excluded Profile data to OpenAI during the test.
- [ ] A real draft succeeds through the browser and Go in the pull request preview and deployed app. Inspect its job title, company, role summary, résumé, cover letter, and application answers against the synthetic facts; record unsupported claims or omissions rather than accepting field presence alone.
- [ ] A qualification confirmed for the first draft appears only in that draft. A new application starts with no confirmed qualifications; the saved Profile remains unchanged.
- [ ] Record response status and browser, Function, and provider durations available in each environment; each synchronous Function call completes within 60 seconds without an automatic retry.
- [ ] Review actual Function logs and response headers for both runs: logs contain operational metadata only, and neither the key nor Profile/posting/draft content is stored or cached by the backend.

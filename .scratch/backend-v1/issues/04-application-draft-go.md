# 04 — Application Draft generation through Go

**What to build:** After confirming qualifications for one job opportunity, a user can generate and view an Application Draft through Go in preview and the deployed app.

**Blocked by:** 03 — Qualification gap checks through Go.

**Status:** ready-for-agent

- [ ] Generation sends the full Profile, including compensation, the job posting, and only the qualifications confirmed for that draft; the browser no longer calls Anthropic directly for this operation.
- [ ] The backend owns the prompt, input limits, fixed Anthropic model call, and output validation for job title, company, role summary, resume, cover letter, and application answers.
- [ ] Incomplete or structurally invalid provider output is rejected before display. The saved Profile is unchanged, and qualifications confirmed for one draft do not carry into another.
- [ ] Invalid input or key, rate limits, outages, timeout, and invalid output produce clear English and Portuguese errors without automatic provider retries.
- [ ] Go contract and failure tests pass, and a real Anthropic Application Draft succeeds in both Figma Make preview and the deployed app within the 60-second limit; inspect the result for completeness and unsupported factual claims.
- [ ] The backend does not store or log the Profile, job posting, confirmed qualifications, key, or Application Draft.

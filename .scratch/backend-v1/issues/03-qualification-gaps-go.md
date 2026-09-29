# 03 — Qualification gap checks through Go

**What to build:** A user can check a job opportunity against their Profile and confirm any suggested qualifications before generating an Application Draft. The gap check runs through Go in both preview and the deployed app.

**Blocked by:** 02 — Deploy Profile review on Netlify Free.

**Status:** ready-for-agent

- [ ] The gap check sends the full Profile, including compensation, and the job posting to the Go workflow; the browser no longer calls Anthropic directly for this operation.
- [ ] The backend owns the prompt, input limits, fixed Anthropic model call, and output validation, returning no more than five suggested gaps or an empty result.
- [ ] The existing confirmation flow remains intact: suggested qualifications require user confirmation and do not mutate the saved Profile.
- [ ] Invalid input or provider output, key failures, rate limits, outages, and timeouts show clear English and Portuguese errors without an automatic retry.
- [ ] Go contract and failure tests pass, and a real Anthropic gap check succeeds in both Figma Make preview and the deployed app within the 60-second limit.
- [ ] No job posting, Profile, key, or suggested qualification is stored or logged by the backend.

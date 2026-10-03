# 04 — Application Draft generation through Go

**What to build:** After confirming qualifications for one job opportunity, a user can generate and view an Application Draft through Go in preview and the deployed app.

**Blocked by:** 03 — Qualification gap checks through Go.

**Status:** In progress; local implementation checks pass, provider-backed preview and deployed-app acceptance remain pending.

- [ ] Generation sends the full Profile, including compensation, the job posting, and only the qualifications confirmed for that draft; the browser no longer calls Anthropic directly for this operation.
- [ ] The backend owns the prompt, input limits, fixed OpenAI `gpt-6-luna` model call, and output validation for job title, company, role summary, resume, cover letter, and application answers.
- [ ] Incomplete or structurally invalid provider output is rejected before display. The saved Profile is unchanged, and qualifications confirmed for one draft do not carry into another.
- [ ] Invalid input or key, rate limits, outages, timeout, and invalid output produce clear English and Portuguese errors without automatic provider retries.
- [ ] Go contract and failure tests pass, and a real OpenAI Application Draft succeeds in both Figma Make preview and the deployed app within the 60-second limit; inspect the result for completeness and unsupported factual claims.
- [ ] The backend does not store or log the Profile, job posting, confirmed qualifications, key, or Application Draft.

**Local evidence (2026-10-03):** Go contract and failure tests, `go vet ./...`, TypeScript checking, and the production frontend build pass. The outbound-payload preflight passed with only synthetic posting, skills, competencies, and tools; it excluded all other profile fields, including experience and recency. The default-sandbox DNS probe could not resolve `api.openai.com`. After a separate credential-free escalated DNS/TLS check succeeded, one manually authorized local request using the same audited synthetic-only data returned handler HTTP 200 in 5,521 ms; all six required output fields were non-empty. Only status, duration, and field-length metadata were emitted. The generated content was not emitted or reviewed for unsupported claims, so that check remains pending. An earlier default-sandbox synthetic attempt returned handler HTTP 502 (`outage`) in 1 ms; the handler logs only status, outcome, and duration, so whether that request reached OpenAI is unknown. No real user Profile was used. Independent Standards/Spec review is pending because the agent concurrency limit prevented reviewer delegation. The Figma Make preview, deployed-app provider check, output-claim review, and deployed log review remain unaccepted by this local evidence.

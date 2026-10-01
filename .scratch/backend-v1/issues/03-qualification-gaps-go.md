# 03 — Qualification gap checks through Go

**What to build:** A user can check a job opportunity against their Profile and confirm any suggested qualifications before generating an Application Draft. The gap check runs through Go in both preview and the deployed app.

**Blocked by:** 02 — Deploy Profile review on Netlify Free.

**Status:** implementation complete; live preview and deployed acceptance pending

- [ ] The browser sends the full Profile, including compensation, and job posting to Go. Go builds a separate allowlisted provider DTO containing only the posting, skills, competencies, tools, experience title/description/responsibilities/achievements plus derived duration/recency, and project description/technologies/highlights. Compensation, employment status, names, IDs, URLs, locations, career goals, and additional information are excluded. Free text may still contain sensitive details; this is not anonymization. The browser no longer calls Anthropic directly for this operation.
- [ ] The backend owns the prompt, input limits, fixed OpenAI `gpt-6-luna` model call, and output validation, returning no more than five suggested gaps or an empty result.
- [ ] The existing confirmation flow remains intact: suggested qualifications require user confirmation and do not mutate the saved Profile.
- [ ] Invalid input or provider output, key failures, rate limits, outages, and timeouts show clear English and Portuguese errors without an automatic retry.
- [ ] Go contract and failure tests pass, and a real OpenAI gap check succeeds in both Figma Make preview and the deployed app within the 60-second limit.
- [ ] No job posting, Profile, key, or suggested qualification is stored or logged by the backend.

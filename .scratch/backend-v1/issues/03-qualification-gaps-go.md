# 03 — Qualification gap checks through Go

**What to build:** A user can check a job opportunity against their Profile and confirm any suggested qualifications before generating an Application Draft. Ticket 03 acceptance covers local checks and the Netlify pull request deploy preview. Figma Make preview is not an acceptance environment for this workflow; the deployed app is not a separate Ticket 03 gate.

**Implementation dependency:** 02 — Deploy Profile review on Netlify Free.

**Status:** Accepted (2026-10-01): local checks, local real-provider evidence, credential-free Netlify pull request deploy preview checks, and explicitly mocked preview UI evidence satisfy Ticket 03 acceptance.

- [x] The browser sends the full Profile, including compensation, and job posting to Go. Go builds a separate allowlisted provider DTO containing only the posting, skills, competencies, tools, experience role text plus derived duration, and project qualification text. Compensation, employment status, names, IDs, URLs, locations, career goals, additional information, and experience recency are excluded. Free text may still contain sensitive details; this is not anonymization. The browser no longer calls Anthropic directly for this operation.
- [x] The backend owns the prompt, input limits, fixed OpenAI `gpt-6-luna` model call, and output validation, returning no more than five suggested gaps or an empty result.
- [x] The existing confirmation flow remains intact: suggested qualifications require user confirmation and do not mutate the saved Profile.
- [x] Invalid input or provider output, key failures, rate limits, outages, and timeouts show clear English and Portuguese errors without an automatic retry.
- [x] Go contract and failure tests pass. The Netlify pull request deploy preview exposes the same-origin Go function and passes relevant credential-free request/error checks; the preview URL, status, cache behavior, and mock UI results are recorded below.
- [x] A local real OpenAI request using only the previously approved minimized qualification projection completed successfully and returned a valid result.
- [x] The browser flow in the Netlify pull request deploy preview was checked with an explicitly mocked provider response: an unchecked suggestion requires explicit confirmation, confirmed qualifications remain application-scoped, and the saved Profile is unchanged. This is mock UI evidence, not a real-provider check.
- [x] No job posting, Profile, key, or suggested qualification is stored or logged by the backend.

**Overall v1 release gates not covered by Ticket 03 acceptance:**

- [ ] Record a successful provider-backed qualification-gap call on the Netlify preview.
- [ ] Record the duration of a provider-backed Netlify call and confirm it meets the Function limit.
- [ ] Review actual Netlify Function logs for this workflow.

Do not send the vault OpenAI key to Netlify or include a real user Profile. Use only the previously approved qualification projection for any later provider-backed check. There is no separate deployed-app acceptance gate for Ticket 03.

**Evidence (2026-10-01):** Local Go tests and vet, TypeScript typecheck, frontend build, and code reviews passed. The public PR deploy preview at <https://deploy-preview-2--beamish-bavarois-333d03.netlify.app/> returned the expected keyless `401 key` and invalid-input `400 input` responses; responses used `no-store` and bypassed cache. The browser mock flow confirmed explicit selection before adding a suggested qualification, no saved-Profile mutation, and zero qualification-gap network calls. Separately, a local real OpenAI request using only the minimized, previously approved qualification projection completed in 2.99 seconds and returned one valid gap. Ticket 03 accepts this combined evidence. It does not establish a provider-backed Netlify call or duration, and no actual Netlify Function logs were reviewed; all three remain overall v1 release gates above.

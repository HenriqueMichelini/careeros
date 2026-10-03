# 03 — Qualification gap checks through Go

**What to build:** A user can check a job opportunity against their Profile and confirm any suggested qualifications before generating an Application Draft. Ticket 03 acceptance covers local checks and the Netlify pull request deploy preview. Figma Make preview is not an acceptance environment for this workflow; the deployed app is not a separate Ticket 03 gate.

**Implementation dependency:** 02 — Deploy Profile review on Netlify Free.

**Status:** Pending corrected local real-provider verification. Local contract checks, credential-free Netlify pull request deploy preview checks, and explicitly mocked preview UI evidence are recorded; acceptance remains conditional on one successful provider call using the corrected no-recency DTO.

- [x] The browser sends the full Profile, including compensation, and job posting to Go. Go builds a separate allowlisted provider DTO containing only the posting, skills, competencies, tools, experience role text plus derived duration, and project qualification text. Compensation, employment status, names, IDs, URLs, locations, career goals, additional information, and experience recency are excluded. Free text may still contain sensitive details; this is not anonymization. The browser no longer calls Anthropic directly for this operation.
- [x] The backend owns the prompt, input limits, fixed OpenAI `gpt-6-luna` model call, and output validation, returning no more than five suggested gaps or an empty result.
- [x] The existing confirmation flow remains intact: suggested qualifications require user confirmation and do not mutate the saved Profile.
- [x] Invalid input or provider output, key failures, rate limits, outages, and timeouts show clear English and Portuguese errors without an automatic retry.
- [x] Go contract and failure tests pass. The Netlify pull request deploy preview exposes the same-origin Go function and passes relevant credential-free request/error checks; the preview URL, status, cache behavior, and mock UI results are recorded below.
- [ ] A local real OpenAI request using the corrected minimized qualification projection, which excludes experience recency, completes successfully and returns a valid result.
- [x] The browser flow in the Netlify pull request deploy preview was checked with an explicitly mocked provider response: an unchecked suggestion requires explicit confirmation, confirmed qualifications remain application-scoped, and the saved Profile is unchanged. This is mock UI evidence, not a real-provider check.
- [x] No job posting, Profile, key, or suggested qualification is stored or logged by the backend.

**Overall v1 release gates not covered by Ticket 03 acceptance:**

- [ ] Record a successful provider-backed qualification-gap call on the Netlify preview.
- [ ] Record the duration of a provider-backed Netlify call and confirm it meets the Function limit.
- [ ] Review actual Netlify Function logs for this workflow.

Do not send the vault OpenAI key to Netlify or include a real user Profile. Use only a synthetic Profile containing the approved fields—posting, skills, competencies, tools, experience role text and duration, and project qualification text—for the pending corrected local provider check. There is no separate deployed-app acceptance gate for Ticket 03.

**Evidence (2026-10-01; corrected payload verification pending):** Local Go tests and vet, TypeScript typecheck, frontend build, and code reviews passed. The public PR deploy preview at <https://deploy-preview-2--beamish-bavarois-333d03.netlify.app/> returned the expected keyless `401 key` and invalid-input `400 input` responses; responses used `no-store` and bypassed cache. The browser mock flow confirmed explicit selection before adding a suggested qualification, no saved-Profile mutation, and zero qualification-gap network calls. A historical local real OpenAI call completed in 2.99 seconds and returned one valid gap, but its provider payload included experience recency. That timing and result do not verify the corrected no-recency DTO. Ticket 03 acceptance is pending one successful local real-provider call with the corrected synthetic payload. No provider-backed Netlify call or duration has been recorded, and actual Netlify Function logs have not been reviewed; those three items remain separate overall v1 release gates above.

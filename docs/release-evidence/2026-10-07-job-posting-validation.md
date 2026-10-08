# Job Posting validation — issue #31

Local implementation on `codex/backend-v1-queue`, starting from `5b60e1ec593007b28aab0042dcd59d385eb4e995`.

Both POST `/api/qualification-gaps` and POST `/api/application-draft` classify the complete current Job Posting through the existing pinned TypeSafe Jev adapter before any OpenAI call. No acceptance token or client decision is trusted. This includes direct draft requests with application-only confirmed qualifications. Each deliberate retry and changed submission is classified again. Classification receives only the field and exact submission, with the existing three-second deadline. Downstream calls retain their 25-second deadline and the browser retains its 30-second request bound. Responses retain `Cache-Control: no-store`; logs contain only status, outcome and duration.

The Apply page supplies the user's TypeSafe key on qualification checks and all three draft paths (no gaps, confirmed qualifications, without confirmations). Nonempty short input can reach the server: “Software Engineer” receives the required English clarification or its Portuguese translation. Rephrasing and detected attacks require editing the posting; whitespace changes do not unlock generation. Shared input-use terms are linked for detected attacks. Validation failures preserve the text and saved Profile, return to the posting when revision is needed, and never automatically retry or switch providers. Existing qualification-confirmation behavior and application-only qualification storage are retained.

## Verification

- Regression first: `TestJobDecisionStopsBothEndpointsBeforeExtraction` failed on the original handlers because OpenAI extraction ran before classification; passed after adding the gates.
- `GOCACHE=/tmp/careeros-go-cache go test ./...` — passed.
- `node_modules/.bin/tsc --noEmit` — passed during implementation and final verification.
- `node_modules/.bin/vite build` — passed (existing Vite config-loader advisory).
- `node --test tests/*.test.mjs` — five files passed; `cv-preferences.test.mjs` failed because it expects default font size 12 while the implementation returns 14. Reproduced from untouched source and test files at starting commit `5b60e1e`; unrelated to #31.
- `node tests/apply-checklist.check.mjs` — passed with controlled responses in English and Portuguese at 1440px and 390px. Covers title-only feedback, revision and attack outcomes, irrelevant/unusable text, all classifier service-failure reasons, both confirmation actions, original messy posting and applicant requirements passed unchanged, changed-input rechecking, direct no-gap drafting, deliberate retries, stale responses, and saved Profile preservation.
- Inspected the Portuguese 390px screenshot; controls and checklist fit the viewport. Browser artifacts: `/tmp/careeros-apply-checklist-lcGpz5/`.

The contract and browser fixtures simulate provider responses. They establish enforcement, routing, payloads and recovery; they do not establish live semantic classifier accuracy or live generated-content quality. No deployment or live provider call was performed.

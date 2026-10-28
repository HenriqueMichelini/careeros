# Saved account Profile → Application Draft

This local product slice extends the merged authentication/PostgreSQL foundation at `fe00b77aa327b1087377874bcc469b6487f2993d`. It uses the existing Profile, Apply and Results screens. The working artifact contracts remain [#255 S06-D01–D05](https://github.com/HenriqueMichelini/careeros/issues/255); account authority and persistence remain [#262](https://github.com/HenriqueMichelini/careeros/issues/262) and [#271](https://github.com/HenriqueMichelini/careeros/issues/271).

## Customer journey

Build/run the account workspace with `VITE_CAREEROS_ACCOUNT_MODE=1`. After sign-in, an empty or contact-only Profile opens Profile. Paste a CV or career information, prepare the proposed changes, inspect their destinations/support, select changes and Save. Preparation does not publish Profile facts. Existing manual editors remain available with their existing persistence behavior. Pending, failed and uncertain Saves do not enable Apply readiness.

A restored useful saved Profile opens Apply. The next-step control also leads from Profile to Apply. Paste the job and deliberately start analysis. Review the existing job understanding and qualification information, then deliberately generate the Draft. A standalone CV is optional. AI keys retain their existing session-only account behavior; configure the OpenAI key in the header and TypeSafe in the existing settings. This slice adds no hosted inference key or provider selection.

Results retain the existing support review, claim/currentness rules, separate personal acceptance and Copy/PDF controls. Generating never saves Profile facts or accepts an artifact. Failure, malformed/incomplete output and cancellation preserve the current result. A successful later candidate also retains one previous candidate in memory, including its review/acceptance state, accessible from any Results tab. Switching these candidates performs no inference. This is bounded session preservation, not durable Draft history.

Cancellation aborts the browser request and ignores late completions. The message explicitly says that provider work may continue. No authoritative cancellation receipt, action-recovery endpoint or automatic repeat generation is introduced. Known failures offer a deliberate retry; missing/stale Profile and session errors identify the next action. Reauthentication can open another tab to preserve current transient work.

## Protected routes

The local Go router and **all five separate Netlify Functions** now use the shared account guard:

| Public route | Direct Function |
| --- | --- |
| `/api/profile/ingest` | `/.netlify/functions/profile-ingestion` |
| `/api/profile/review` | `/.netlify/functions/profile-review` |
| `/api/qualification-gaps` | `/.netlify/functions/qualification-gaps` |
| `/api/application-draft` | `/.netlify/functions/application-draft` |
| `/api/cv/generate` | `/.netlify/functions/cv-generation` |

Their existing aliases use the same handler; there is no guest bypass when `CAREEROS_REQUIRE_AUTH` is absent or `0`. Missing runtime configuration returns `503 auth_unavailable`. All require a verified session and matching `X-CareerOS-Account` consistency header; unsafe requests also require the existing exact Origin and session-bound CSRF checks. A claimed account ID is never authority. All account Profile routes keep their existing transactional authorization/ownership checks.

Analysis and Application Draft additionally read the session owner's current PostgreSQL Profile. The client career view, structured qualifications and approved evidence projection must match that saved document, including its identity and revisions. A foreign/tampered/stale snapshot returns `409 profile_stale`; a missing/insufficient saved Profile returns `422 profile_required`, before inference. Draft generation always enables both resume and other-artifact reviews, even if a direct caller omits or disables those flags. The full valid Apply envelope, including temporary application-only answers, is accepted.

Before returning a workflow result, the server rechecks authorization. For analysis/Draft it also verifies the saved Profile has not changed. No database lock spans AI work. These checks do not establish atomic durable result publication or close the distributed cancellation/recovery window.

## Remaining boundaries

- The account workspace remains opt-in via the existing build flag. Without it the browser-local UI still exists; its old unauthenticated AI requests are now denied. Guest browser data is neither imported nor used as account authority. Static assets and the intentional sign-in start/callback routes remain public; the five AI Functions are authenticated.
- Profile/auth are PostgreSQL-backed. Drafts, jobs, application-only answers and one previous result remain transient and are lost on reload, sign-out/account remount or closing the tab. Export useful artifacts first. No durable Draft recovery, delivery receipt or complete S06-D01 recovery guarantee is claimed.
- Existing Profile size, projection, review, input and export limits remain. One saved professional fact satisfies the existing readiness rule; readiness does not guarantee useful factual detail for every job or AI semantic quality. Contact details alone do not satisfy it.
- User AI keys stay in account-session memory and must be reentered after reload or reauthentication. Live Google/AI behavior, hosted runtime/route configuration and full accessibility/device behavior remain unverified. The browser evidence is synthetic desktop Chrome/emulation, not a real-device or screen-reader certification.
- Payments, allowances, application tracking, legacy import, recovery fencing, regional recovery, provider selection and Q-N1 are excluded. Nothing here establishes hosted launch readiness.

See the [local evidence and reproducible demonstration](release-evidence/2026-10-27-first-account-draft.md) and [exact proposed PR text](release-evidence/first-account-draft/proposed-pr.md).

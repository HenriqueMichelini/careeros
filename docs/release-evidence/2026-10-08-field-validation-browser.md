# Protected-preview live browser verification — 2026-10-08

**PASS.** The authenticated preview completed the live browser → Go Function → TypeSafe/OpenAI → rendered Results flow. This resolves the browser-verification gap recorded in the [release evidence](2026-10-08-field-validation-release.md). This is a bounded synthetic test, not universal semantic or production-load assurance.

## Identity and authorization

- Preview: `https://deploy-preview-53--beamish-bavarois-333d03.netlify.app/`.
- Deploy: `6ac7b3173bbd67000870baef`; revision: `0c7dbbcb5d8ffbc4dc7eb5fabcce42ffe254b11b`.
- The reviewed source was integrated and published as main `31371b4` / deploy `6ac7b3c38bc6d40008084b95`. Later evidence-only commits do not change runtime source. Auto-publishing remains locked.
- The user entered existing keys directly in the preview and explicitly authorized a deliberate retry bounded to two workflow requests, at most two TypeSafe plus two OpenAI calls. Both requests succeeded. No automatic retry or extra provider request occurred. Account/provider telemetry was not independently audited; call counts follow the verified successful handler paths.

The public-origin saved Profile was not submitted or edited. The isolated preview originally had an empty Profile; the only test fact entered manually was the already approved synthetic Java skill. The approved short PT posting was used, with CV language PT and UI language EN. No inferred personal facts or application-only confirmations were added.

## Observed flow and factual review

1. Qualification checking completed without a qualification-confirmation dialog and advanced to drafting.
2. Drafting reached Results. Role and company both displayed **Not provided**. The role summary preserved the Java API opportunity without inventing company, location or additional requirements.
3. The résumé preview contained only the Java technical skill. No employers, projects, duration, education, proficiency or API-work history appeared.
4. The Portuguese cover letter stated the Java skill and interest in the opportunity without claiming prior API development experience. Missing candidate name produced the expected signature guidance.
5. Five application answers preserved unknown formation/certification/language information and explicitly stated that prior API-development experience was not supplied.
6. Returning to Profile confirmed Java remained the sole skill and competencies/tools stayed empty. Reloading retained those saved synthetic values. Generated application materials are transient and were no longer present after reload; no persistence claim is made for Results.

## Runtime and privacy

| Workflow | Logged UTC | Status/outcome | Handler ms | Netlify invocation ms | Invocation ID | Memory |
| --- | --- | --- | --- | --- | --- | --- |
| Qualification gaps | 2026-10-08T15:46:50Z | 200 / ok | 2647 | 2651.33 | `3a698a52` | 37 MB |
| Application draft | 2026-10-08T15:46:54Z | 200 / ok | 4175 | 4178.31 | `7ce82ea5` | 37 MB |

Read-only review of the matching selected Function logs found metadata only: **PASS** for these invocations. No credential, Profile/posting text or generated content is copied into this evidence. [Machine-readable record](2026-10-08-field-validation-browser.json).

Browser network duration, browser response cache headers, full HTTP request IDs and provider-only timing were unavailable through the browser tooling. Handler timing is not relabeled as browser or provider timing. Successful public-origin HTTPS checks already established no-store and Durable bypass on the identified published source; those observations remain separate from this browser run.

## Preserved failed attempt and limits

Before the corrected-key retry, a synthetic qualification request failed the local OpenAI-key format check: 2026-10-08T15:44:26Z, 401/key, handler 0 ms, Netlify 2.54 ms, invocation `da70e25b`. It made zero provider calls and no draft request. It was stopped and reported; the user then corrected the preview key and authorized this deliberate retry. Keys saved on the public origin do not configure the preview origin. The original failed credential-handoff attempt remains documented in the release evidence as well.

The new live browser sample covers Apply qualification/drafting/Results. Ingestion retains separate controlled browser lifecycle coverage, authorized local live-provider cases and the successful published direct Function Java-proposal check. This run does not claim a new live ingestion browser sample, injected upstream failures, maximum-size/load measurements, full assistive-technology conformance or exported-PDF visual validation. These evidence boundaries remain explicit.

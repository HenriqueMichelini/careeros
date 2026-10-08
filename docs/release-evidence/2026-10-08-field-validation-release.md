# Field-validation release verification — 2026-10-08

## Current verdict

**Published Function checks pass; milestone acceptance remains pending the browser evidence boundary.** The corrected implementation and local synthetic checks pass, and PR [#53](https://github.com/HenriqueMichelini/careeros/pull/53) is merged. The published public origin passed three provider-backed Function workflows; this does not establish the protected-preview browser-to-provider path.

## Identified revisions

| Environment | Deploy ID | Full revision | Observed state |
| --- | --- | --- | --- |
| PR #53 preview | `6ac7b3173bbd67000870baef` | `0c7dbbcb5d8ffbc4dc7eb5fabcce42ffe254b11b` | Build successful; five Functions and six redirects deployed; authenticated browser reaches TypeSafe settings and input-use terms |
| Published main | `6ac7b3c38bc6d40008084b95` | `31371b4f8739a46b2983de27411855153eabe478` | Production-context build successful; five Functions and six redirects deployed; authenticated permalink reaches frontend; subsequently published under explicit user authorization |
| Previous published app | `6ac5ab6a0a6ca2000762e50d` | `fc91b12a3a622ea467acbf07f417ac48451b5917` | Replaced by `31371b4`; retained here as historical identity |

PR #53 merged at 2026-10-08T15:16:16Z. Netlify identifies the main candidate build from 15:16:24Z to 15:17:23Z. The dashboard reports all candidate files already uploaded by the prior deploy. The user subsequently explicitly approved publication of this exact candidate and three direct HTTPS synthetic checks. The dashboard now states **Published & locked deploy**, main `31371b4`, with publishing locked to deploy `6ac7b3c38bc6d40008084b95`. Auto-publishing remains locked. The public HTML references the candidate frontend bundle `index-DqkCHhrj.js`. Read-only browser inspection confirms the PT TypeSafe settings, privacy disclosure, and whole-submission input-use terms. Existing public-origin Profile and keys were not edited or submitted.

## Authorization and failed browser attempt

The user explicitly authorized transferring the existing TypeSafe and OpenAI vault credential values to this project's Netlify Functions for synthetic checks, overriding the dedicated-tester-entry restriction for this bounded run only. The approved cap was six workflow requests, at most six TypeSafe and eight OpenAI calls. No retry was promised without further authorization.

The agent used a temporary loopback-only credential handoff. The browser inspection redacted the credential value before it reached the test form. The saved value failed the handler's OpenAI-key format check. Read-only inspection confirmed that the vault value has the expected prefix; the browser-held value did not. No credential values were printed or saved in evidence. The loopback server was stopped and test form values were cleared.

One synthetic preview ingestion request reached Go at 2026-10-08T15:21:12Z and returned **401 / key** before classification or generation. Netlify invocation identifier: `217df646`; handler `duration_ms=0`; Netlify invocation duration **2.63 ms**, memory **31 MB**. The selected Function-log entry contained only workflow/status/outcome/duration and invocation metadata: privacy inspection **PASS for this failed invocation only**. This is a test-handoff failure, not evidence of invalid provider credentials or a successful deployed workflow. No TypeSafe or OpenAI call was made by that invocation. Remaining provider checks stopped.

Browser HTTP duration, response cache headers, and full request ID were not captured for this browser attempt. Provider duration is unavailable. Neither zero handler milliseconds nor the 2.63-ms invocation is a successful workflow performance measurement.

## Credential-free HTTPS route checks

These empty-body requests contain no credential and cannot call a provider. They are additional nonbillable reachability checks, excluded from the six provider-workflow attempts.

| Origin | Status / body category | Cache-Control | Cache-Status | Request ID |
| --- | --- | --- | --- | --- |
| `deploy-preview-53--beamish-bavarois-333d03.netlify.app` | 401 / Netlify Team-protection HTML, before Go | unavailable | unavailable | `01M4E1RPA3Y5VQ4F8J6B0YMK3F` |
| `6ac7b3c38bc6d40008084b95--beamish-bavarois-333d03.netlify.app` | 401 / Netlify Team-protection HTML, before Go | unavailable | unavailable | `01M4E1RPB151NM4K6NKKD3QS55` |
| `beamish-bavarois-333d03.netlify.app` | 401 / Go key error | `no-store` | Netlify Durable bypass | `01M4E1RPNFV594YX2M4A7979E9` |

Team protection remains enabled. Direct HTTP provider checks against protected previews cannot substitute for authenticated browser tests without an approved supported access path.

## Approved public-origin provider checks

The user approved publishing the identified main build and three direct HTTPS checks on the public app, within the original overall workflow cap: at most three TypeSafe and four OpenAI calls. Exactly three requests were sent, with no retries. Combined with the failed browser format-check attempt, four of six workflow attempts were used; no remaining budget was spent. The successful handler paths imply three classifier and four generation exchanges; provider account telemetry was not independently inspected.

Exact local outbound envelopes were reviewed for the approved synthetic data. Ingestion/draft use the same fixtures and source-derived envelopes from the completed local run; qualification gaps were additionally exported offline through the production handler with a recorded accepted classifier response, then inspected. The gap envelope contains only approved Java qualification data plus the approved short posting. Credentials were parsed from assignment files process-locally and sent only in the named Netlify Function headers. Raw synthetic responses remain in the private temporary review surface, not committed evidence.

All three results returned **HTTP 200**, **Cache-Control: no-store**, **Netlify Durable bypass**, and result-shape/factual review **PASS**. Timings below are direct HTTPS client times, not browser durations. Function logs were inspected read-only and matched by workflow and execution time. Each selected successful log contained status/outcome/duration plus invocation metadata only: privacy inspection **PASS for these three invocations**. This is not an exhaustive historical log audit.

| Workflow | Result review | HTTP client ms | Handler ms | Netlify invocation ms | Invocation ID | HTTP request ID |
| --- | --- | --- | --- | --- | --- | --- |
| EN Java ingestion, empty synthetic Profile | Exactly one editable Java skill addition; zero unresolved/unverified claims; no saved Profile write | 5102 | 4142 | 4145.76 | `d8f847b6` | `01M4E1ZZ8ZQ1FBKMMEAWAD2XJG` |
| PT short-posting qualification gaps, synthetic Java skill | Empty gaps; no invented requirements | 2400 | 1558 | 1561.15 | `906c26d3` | `01M4E2048C65TXVCVYX430VFJV` |
| PT short-posting draft, synthetic Java skill | Title/company remain null; PT materials; only Java skill claimed; missing API experience explicitly acknowledged; five application answers | 3750 | 2904 | 2907.43 | `ccf1b9b6` | `01M4E206KFWNS11R19HXC7BJFA` |

Successful executions were logged at 2026-10-08T15:27:05Z, 15:27:08Z and 15:27:12Z respectively. Memory was 38/37/37 MB. Separate provider-only duration and browser network duration are **unavailable**; no inference is made by subtracting the timings. [Machine-readable metadata](2026-10-08-field-validation-release.json) contains payload hashes and these bounded observations.

## Evidence boundaries and remaining work

The [local remediation record](../evaluations/field-validation-remediation.md) and [successful local call metadata](../evaluations/field-validation-remediation-retry-results.json) establish eight TypeSafe plus fourteen OpenAI calls and production-handler replay for the approved synthetic fixtures. This release adds live public-origin Function/provider behavior and host runtime evidence. The original Java omission and absent published integration findings are corrected at the identified release revision.

Outstanding: a successful browser → Function → providers → visible-result exercise on the protected preview and/or an explicitly accepted narrower release evidence boundary, followed by the final acceptance reassessment. The preview needs a supported authenticated credential-entry path; Team protection remains intact. Source-controlled browser suites continue to establish controlled EN/PT proposal lifecycle and Apply behavior. Direct HTTPS success and separate frontend inspection are explicitly distinct from a live end-to-end browser result.

Maximum-size behavior, production concurrency, cold-start characterization, upstream failure recovery, and exported-PDF visual validation remain outside this small bounded run. No broader performance or PDF-success claim is made. Issue/milestone closure was not performed.

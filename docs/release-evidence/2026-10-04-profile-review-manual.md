# Manual Profile review observations — 2026-10-04

Deploy in the Netlify Function dashboard: `6ac24a0d1f02f400084bb819`, revision `910d8ad9b1bc29937d2b6391f895345518094c1c`. The tester identified the [PR preview Function endpoint](https://deploy-preview-2--beamish-bavarois-333d03.netlify.app/.netlify/functions/profile-review) as a test target and reported a successful manual new-user flow in the deployed application, including Profile Review. This is tester-reported end-to-end evidence; no browser trace was retained. The Function log itself does not label each invocation's request origin, so neither entry is independently assigned to a specific origin here.

| Local time (GMT-3) | Netlify invocation | Handler result and duration | Platform execution, memory, initialization |
| --- | --- | --- | --- |
| 16:04:32 | `be0a9cac` | `status=200 outcome=ok duration_ms=4232` | 4,235.08 ms; 37 MB; 119.28 ms initialization |
| 16:11:24 | `3a2a3d03` | `status=200 outcome=ok duration_ms=9541` | 9,545.06 ms; 38 MB; 116.44 ms initialization |

These visible log entries contain operational metadata only; no credential, Profile, or generated text appears in them. Both Function executions were below 60 seconds. The handler returns `200 ok` only after its OpenAI call and result validation, so these entries support successful provider-backed handler invocations at this deploy. They do not expose the outbound payload or prove output quality.

**Informational limits for Ticket 01:** no separately recorded exact Go-to-OpenAI payload preflight, synthetic-fixture confirmation, generated-output inspection, per-origin browser trace, browser duration or `200` response headers, save behavior after reload, or provider-specific duration. The tester reported using the vault OpenAI test key rather than the dedicated-key procedure in the [release-test path](../release-test-path.md); no credential value is recorded here. Ticket 01 is accepted on the combined evidence and requires no additional provider call. These observations alone do not mark Ticket 02 or the broader provider-backed release gates complete.

## Controlled two-environment verification

On 2026-10-04, the tester confirmed that the key already configured in both browser origins was the dedicated OpenAI test key. One deliberate Profile review was then run in each environment, sequentially and without an automatic retry. The PR preview completed before the published-app request started, which assigns the two adjacent Function invocations below to their origins despite the shared Function log not displaying request origin.

Before either request, the browser-visible Profile was inspected section by section. The exact application payload contained the complete Profile contract, with approved synthetic content only in `careerGoals` and `skills`, the default `employmentStatus`, empty `competencies`, `tools`, `currentSalary`, `desiredSalary`, and `additionalInfo`, and empty `experience` and `projects` arrays. The changed-section label was `Compensation`. No real person, employer, role, date, number, qualification, compensation value, project, or additional information was present.

| Environment | Deploy and revision | Invocation | Result | Function timing | Browser timing | Provider timing | Headers/cache | Payload, output, reload, and log privacy |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| PR preview | `6ac2d96a5aa8080008ed51f1`; `bde368a5ba86a5a2a7bf225f03a72f7d722cb2ff` | `1424b74d`; 20:11:38 GMT-3 | HTTP 200; usable reviewed Profile | handler 2,442 ms; platform 2,445.20 ms; initialization 123.90 ms | unavailable; not retained before reload | unavailable; handler has no provider-only measurement | unavailable for this successful response; Network metadata was not retained | Preflight pass; output pass; reload pass; Function-log privacy pass |
| Published app | `6ac24a0d1f02f400084bb819`; `910d8ad9b1bc29937d2b6391f895345518094c1c` | `bd0c84ec`; 20:12:21 GMT-3 | HTTP 200; usable reviewed Profile | handler 2,021 ms; platform 2,023.29 ms; initialization 114.98 ms | unavailable; not retained before reload | unavailable; handler has no provider-only measurement | unavailable for this successful response; Network metadata was not retained | Preflight pass; output pass; reload pass; Function-log privacy pass |

The review improved the grammar and professional tone of the synthetic career goal. It preserved the approved skills, list cardinality, IDs, default employment status, and every empty field. No name, employer, role, date, number, qualification, compensation value, project, or other fact was introduced. After a full reload in each origin, the reviewed Profile remained in browser storage; the review summary correctly did not persist because it is session state.

The last-hour Function-log view showed only metadata-only handler and platform lines for these invocations. No key, request header, Profile field, prompt, result, or generated summary appeared. Source review confirms that the handler sets `Cache-Control: no-store`, logs metadata only, and has no persistence or cache write.

### Initial network evidence

The tester then made one additional deliberate review request in each environment with the same dedicated key and already-reviewed synthetic Profile while preserving the browser Network metadata. Each browser request maps to one adjacent Function invocation by origin, response timestamp, and sequential execution. No automatic retry appears in the browser or Function evidence.

| Environment | Browser response | Function timing | Request and invocation IDs | Cache and privacy result |
| --- | --- | --- | --- | --- |
| PR preview | HTTP 200; 8.52 s waiting; 0 ms receiving; response dated 23:34:59 GMT | handler 7,986 ms; platform 7,988.95 ms; initialization 123.77 ms; provider-only timing unavailable | request `01M44MAB6ST700AG6XWH4MHXFY`; invocation `33795504` | `Cache-Control: no-store`; Durable `bypass`; Edge `miss`, forwarded status 200; metadata-only Function logs; no backend storage |
| Published app | HTTP 200; 10.69 s waiting; 0 ms receiving; response dated 23:23:09 GMT | handler 9,617 ms; platform 9,620.13 ms; initialization 125.76 ms; provider-only timing unavailable | request `01M44KMM5B0ARWHC6RXD4GHEHJ`; invocation `3e1026a4` | `Cache-Control: no-store`; Durable `bypass`; Edge `miss`, forwarded status 200; metadata-only Function logs; no backend storage |

Both synchronous calls completed below 60 seconds. The provider-only duration remains unavailable because the deployed handler measures the complete Function operation rather than the outbound OpenAI exchange. The available browser and Function durations are recorded separately. The response headers show `no-store`, Durable bypass, and Edge miss in both environments; the preview also returned its expected site-protection and `noindex` headers. These captures established the live network and cache behavior, but a later review found that the reduced synthetic Profile did not exercise preservation of the approved fictional experience fields.

## Final approved-fixture verification

The review finding was addressed with the complete approved synthetic fixture in both origins. Before either final request, the browser-visible Profile was inspected section by section. Its populated fields were `careerGoals`, `skills`, `competencies`, the default `employmentStatus`, and one fictional experience with `id`, `company`, `title`, `startDate`, `endDate`, `description`, `responsibilities`, and `achievements`. The experience `location` was empty and `current` was false. All other Profile text fields were empty and `projects` was an empty array.

The exact Go-to-OpenAI construction was also inspected before the requests. The provider envelope contains only `model`, `reasoning_effort`, `max_completion_tokens`, `response_format`, and one `messages` entry. The user message contains the changed-section label, the complete serialized synthetic Profile, preservation instructions, and the required response shape. The label was `Other`; the Profile contained only the approved synthetic fields described above. No real Profile data, posting, credential, or excluded field content was included. The credential remained solely in the authorization header created by the Go HTTP client and was absent from the provider body and logs.

| Environment | Deploy and revision | Browser response | Function timing | Request and invocation IDs | Cache and privacy result |
| --- | --- | --- | --- | --- | --- |
| PR preview | `6ac2d96a5aa8080008ed51f1`; `bde368a5ba86a5a2a7bf225f03a72f7d722cb2ff` | HTTP 200; 9.33 s waiting; 0 ms receiving; response dated 23:45:07 GMT | handler 8,075 ms; platform 8,078.33 ms; initialization 130.54 ms; provider-only timing unavailable | request `01M44MWWN97MA6TSGKT51FS355`; invocation `237dd086` | `Cache-Control: no-store`; Durable `bypass`; Edge `miss`, forwarded status 200; metadata-only Function logs; no backend storage |
| Published app | `6ac24a0d1f02f400084bb819`; `910d8ad9b1bc29937d2b6391f895345518094c1c` | HTTP 200; 14.44 s waiting; 0 ms receiving; response dated 23:45:11 GMT | handler 13,065 ms; platform 13,068.38 ms; initialization 123.90 ms; provider-only timing unavailable | request `01M44MWTS2ZYTCF1VA772G7C91`; invocation `96683d84` | `Cache-Control: no-store`; Durable `bypass`; Edge `miss`, forwarded status 200; metadata-only Function logs; no backend storage |

Each final browser request maps to one adjacent Function invocation by origin, response timestamp, and sequential execution. No automatic retry appears in the browser or Function evidence, and both calls completed below 60 seconds. The last-hour Function logs contained operational metadata only: workflow, status, outcome, duration, invocation ID, memory, and initialization. They contained no key, Profile field, prompt, or generated result.

After each result, the tester reloaded the page and confirmed that competencies and the complete fictional experience remained, including the same company, role, dates, description, responsibility, achievement, and the original numeric facts. No experience, project, qualification, name, date, or number was added. This tester-reported reload check, the source-derived provider-payload preflight, the browser responses, and the matching Function logs complete Ticket 02.

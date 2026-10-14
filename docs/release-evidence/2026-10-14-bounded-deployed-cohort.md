# Bounded deployed EN/PT verification — 2026-10-14

**The authorized cohort is complete; the MVP release gate did not pass.** Two
qualifications and two dependent Application Draft requests completed with HTTP
200 and coherent actual provider captures. Both complete applications remain
unaccepted. Separately reviewed CVs were accepted, copied and rendered to PDF in
a local replay of the retained real responses. That replay is controlled UI
evidence, distinct from the live deployed API observations.

The [13-criterion reconciliation](2026-10-14-release-gate-reconciliation.md)
records the remaining gates. #52 stays open. No merge, production publication,
automatic-publishing unlock, account-limit change, provider retry, fallback,
prompt tuning or slot reallocation occurred.

## Candidate, approval and deployment

| Binding | Observed value |
| --- | --- |
| Original review baseline | `1a30a445e194d353af116d8a2b5c5194b14ff36a` |
| Execution candidate | `63a51dfeb7d8220ac4dde8e64eb47ca866a131f9`, PR #114 |
| Application runtime source checkpoint | `6f87542c83241351966401a4f77f4f16c174ddf1` |
| Preview origin | `https://deploy-preview-114--beamish-bavarois-333d03.netlify.app` |
| P3-A build | `6acfaa3a4faf95db07d7d9c1`, completed `2026-10-14T16:14:49Z` |
| P3-B build | `6acfb8bd3e8c6c289017fefe`, completed `2026-10-14T17:16:00Z` |
| Bound | Four unique workflow attempts / 14 provider slots; all consumed |
| Observed dispatches | Four TypeSafe/Jev and ten OpenAI, all captured with HTTP 200 |

The separately approved [P2 inspection](2026-10-13-p2-deployed-inspection.md)
passed at its own source `c154f40` and build `6acecf58423a9ea41a9137f6`, with zero
provider dispatches. P3-A was authorized for two qualifications/six dispatches.
Only after their actual contexts were inspected and the dependent JSON bytes
fixed did the user authorize P3-B for two Drafts/eight additional dispatches.
Each phase authorized at most one same-source configuration rebuild: two total,
including the explicitly additional P3-B rebuild.

SonarCloud passed at the execution candidate, completing
`2026-10-14T05:12:14Z`. GitGuardian, preview and redirect checks passed;
header/pages checks were neutral. Application runtime source, routes, models and
acceptance policies are unchanged since the runtime checkpoint. Later changes
include verification tests and package test commands, not runtime dependencies.
Evidence commits after the cohort do not relabel its source or build.
The runtime source checkpoint identifies code in the PR; it is not a claim that
this revision was published to production.

The budget remained the user's already clarified direction within provider
account limits they report setting. Numeric limits, balances and billed charges
were not observed or changed. Actual provider authentication succeeded in the
14 captured exchanges; this is not proof of future funding.

## Actual API observations

The operator initiated the absolute approved endpoint POSTs in an authenticated,
normal-sandbox disposable Chrome. It checked exact request bytes, origin, source,
build and expiry before preparation and again while each request was paused.
Each request had one released POST and an independently observed network event,
no redirect or blocked request, JSON/no-store response and complete capture.

| Case | Route | Request bytes / SHA-256 | Browser ms | Handler diagnostic ms |
| --- | --- | --- | --- | --- |
| `p3-postgresql-en` | `/api/qualification-gaps` | 2,351 / `d709313763c49f92dd7bb514d59052fcb1f1586288a39faf429cf54b518f1667` | 11,041.3 | 8,192.302587 |
| `p3-typescript-pt` | `/api/qualification-gaps` | 2,431 / `af0f3d49dad6faba67a8c711e58e7e98cf4b8d5d66b2fdb36b35835d68469aa6` | 9,625 | 6,806.061667 |
| `p3-postgresql-en-draft` | `/api/application-draft` | 4,334 / `b63982240410e8cfc700260b30b84e8d6b312c4468ce657663e7adf18def9fce` | 16,583.9 | 13,658.280877 |
| `p3-typescript-pt-draft` | `/api/application-draft` | 4,472 / `bf8dac2472d7f87d66d598bb60b434ee9426b41ef66250c8f67c2d7afb5bf540` | 19,458.9 | 16,306.819886 |

All first deterministic envelope pairs matched the closed-transport preflight.
The later evidence/resume/artifact envelopes used actual responses and were
retained and checked. No fixture replaced those dependent bytes. The server's
existing `release-egress-v1` contract admits stage/recipient/model/schema/bounds
before dispatch. The recorder's deterministic-envelope hash comparison occurs
after the response; it is not an additional server-side hash admission guard.

TypeSafe received the whole original Job Posting on both routes, including
Drafts reusing client-held analysis. It used `jev-1.13.0` at
`https://api.typesafe.ai/v1/systemone`. OpenAI used `gpt-6-luna`, reasoning `none`,
at `https://api.openai.com/v1/chat/completions`: qualification matching/evidence
ceilings 8,000/8,000; Draft generation/resume/artifact ceilings 8,000/4,000/8,000.
Each browser request reserved its complete three- or four-slot allowance before
dispatch. The private ledger restored six consumed slots for P3-B and finished
at four workflows/14 slots, without refund or reallocation. It is an operator
bound, not a global server replay guard or billing counter.

Full per-stage request/response bytes, hashes, transport durations, usage,
operational diagnostics and capture-file hashes are in the content-free
[cohort metadata](issue-52/p3-deployed-cohort.json). Raw synthetic requests,
responses, prompts and provider bodies remain private. Source/hash agreement
establishes identity and traceability, not semantic truth or authorization.

## Source review, completeness and acceptance

The qualifications preserved additional experience as unknown, without
affirming it or denying it. EN retained partial support for the work requirement;
PT supported the provided work. All ten returned Job Posting references matched
their exact UTF-8 ranges. Returned factual identities, revisions, ownership,
qualifiers and values matched the inputs. Complete bounded retrieval with zero
excluded candidates does not establish complete fact discovery.

The two Draft requests used those actual contexts, deduplicated selected IDs and
the original synthetic Profile projection, without added confirmations or
temporary answers. The real frontend serializer and public handler passed closed
preflight before approval. A later closed-fetch replay of the actual responses
passed the real client parser. Parser acceptance is not semantic acceptance.

| Observation | Result and disposition |
| --- | --- |
| EN Job Summary citations | Raw reviewer said `supported`; two of three quotes disagreed with their UTF-8 ranges. Mechanical validation correctly returned `unsupported / invalid_job_citation`. Materials stayed unaccepted. |
| PT additional-experience answer | The output honestly preserved missing information. Raw review said `supported`, but career scope without a career source produced `unsupported / missing_career_support`. This conservative block remained; the answer was not edited or removed. |
| PT Job Summary citation coverage | Eight valid references repeated only range 0–47, the title. Responsibilities and the preferred qualification lie in 47–116 and 116–153. The summary matches the whole posting on agent-assisted inspection, but its supplied citations do not support those parts. `supported` and valid offsets did not prove semantic coverage. |
| Employer requests and language | Both requested work descriptions were addressed using supplied facts; additional experience remained explicitly unknown. EN/PT content adhered to the requested languages in these two cases. This does not establish broader completeness or language reliability. |
| Separate CVs | All five claim occurrences across four distinct supplied facts were source-backed on agent-assisted inspection. Only these CVs were explicitly accepted in controlled UI replay; anonymous identity/contact fields stayed blank. |
| Complete applications | Zero of two evaluated qualification-to-Draft journeys yielded an accepted complete application. HTTP/capture completion did not override either material block. |

These source inspections are agent-assisted, not human semantic adjudication.
No unsupported/incomplete material was accepted, no claim was corrected to make
the evaluation pass, and the finite sample is not a production success-rate or
universal factual-safety measurement.

## Controlled UI delivery using actual responses

The local compiled UI replayed the retained HTTP bodies unchanged, with inert
keys and a loopback server that cannot forward requests. Its completed run
replayed four workflows; all four serialized request bodies exactly matched the
approved paid requests. An earlier interrupted EN replay retained two local
HTTP replays before the temporary tab/process became unavailable during a pause.
Those records were preserved; neither run contacted a provider or read real
credential files. The local build is source-bound but not byte-identical to the
deployed readiness HTML. This is not a live deployed React journey.

In both languages at desktop and 390px:

- The actual concern remained visible and material acceptance stayed disabled;
  Q&A copy was also disabled without artifact acceptance.
- CV sources were expanded and reviewed. Explicit keyboard acceptance enabled
  CV delivery. Real copy controls transferred the exact returned CV Markdown to
  the browser-session clipboard: EN 213 characters, PT 142. Enter and Space were
  exercised, including 390px readback, without stubbing copy APIs.
- Document client/scroll widths were both 390px. A narrow follow-up measured all
  PT header controls inside the viewport; the rightmost edge was 374px. These
  checks and images do not certify every screen-reader or device behavior.
- The synthetic saved Profile bytes remained identical through review,
  acceptance, clipboard, print projection and reload: 166 EN and 240 PT
  observations. Only the Profile/settings/inert-key storage keys were present;
  backend results were not persisted. Application Draft results were absent
  after reload, consistent with their session-only design. Existing general-CV
  v1/v2 durable saving remains separate historical controlled evidence.
- Switching the PT interface to English preserved Portuguese CV content; the
  interface was restored afterward. Viewport and synthetic clipboard were reset,
  the test tab closed, and the completed replay server exited successfully.

Two actual PDF files were generated from the accepted app print projections.
A test-only `window.print` hook captured those projections; normal-sandbox Chrome
then rendered them as self-contained documents. Both have one A4 page at
594.95996 × 841.91998 points, selectable text inside page bounds, and normalized
display/copy/PDF equality. Complete page images were inspected for clipping,
overlap, missing accents and extra UI; none was observed. Identity remained
intentionally blank. The native Save dialog was not operated, and this is not
deployed PDF acceptance or a heavy/dense-CV fit certification.

| Private PDF artifact | Bytes | SHA-256 |
| --- | --- | --- |
| EN CV | 18,837 | `12a68fb8e8f53cff864e13832806bc8a3ea178d7728e912d358fb0ada139bb33` |
| PT CV | 18,404 | `a7d43d599ea2fb34c7d66998f5c5813f6161bc0c382523973ac798cd8cd41acd` |

Content-free [UI/PDF metadata](issue-52/p3-actual-response-ui-replay.controlled.json)
retains body/profile/build/print/PDF hashes and observations. The copies, HTML,
PDFs, screenshots and raw response bodies remain private.

## Usage, cost and runtime comparison

| Provider | Calls | Input tokens | Output tokens | Other observed components |
| --- | ---: | ---: | ---: | --- |
| TypeSafe/Jev | 4 | 4,362 | 432 | Reasoning/cache components unavailable |
| OpenAI | 10 | 15,353 | 3,231 | Reasoning 0; cache reads 0; reported cache writes 13,810 |

Actual charges and balances are unavailable. Usage/cache counters are not billed
charges or measured savings. With zero accepted complete applications, cost per
accepted complete application is undefined, not zero; total monetary cost also
remains unavailable. Failed acceptance remains in the numerator's attempt set.

Browser durations include metadata/interception. Handler matching/generation
spans include dependent support work and must not be summed with those nested
spans as independent latency. The separate capture transport timings remain in
the metadata. Two sparse samples do not support population percentiles or a
forecast of ordinary user latency.

The comparison retains the original measurement boundaries:

- [#34 baseline](../evaluations/semantic-quality/baseline.md): original general-CV
  failures, the missing EN response and PT required-answer omission remain.
  This cohort observed different qualification/Draft cases; it does not recover
  the old response or rerun that corpus. Conservative unmatched absence prose is
  not reclassified as invented experience.
- [#50 decision](../evaluations/workflow-settings/decision.md): fixed settings
  remain. The new EN invalid citations and PT citation-coverage failure do not
  establish full-quality improvement. Its language/ownership/continuation
  regressions and unavailable complete costs remain separate; different cases
  and instrumented timings cannot produce a valid aggregate improvement score.
- [#65 reliability](2026-10-11-cv-reliability.md): the strict summary contract,
  controlled EN/PT/heavy regressions and two bounded sparse confirmations remain
  valid in their recorded scope. New Draft CV success does not verify the general
  CV route or erase the PT exact-label miss or unknown historical EN cause.

## Operational sample, cleanup and validation

Authenticated log samples were preview #114 only: qualification-gaps,
12:26–13:26 GMT-3, and application-draft, 14:15–14:20 GMT-3 on October 14.
The two Draft diagnostics had handler durations 13,658.280877 / 16,306.819886 ms;
platform durations were 13,662.31 / 16,310.29 ms, EN initialization 165.17 ms.
Captured usage agreed with diagnostics. No Profile/posting/prompt/authentication
header or raw provider body appeared in those bounded samples. The dashboard
states 24-hour retention; broader backend/provider retention was not verified.
Empty logs are never proof of zero dispatches, and backend `accepted: null` does
not establish UI acceptance.

P3-A removal was verified after reload at `2026-10-14T16:23:07.798Z`; P3-B at
`2026-10-14T17:19:20.351Z`. Other deploy contexts were empty. Immutable builds can
retain manifests until their respective `17:07:03.702Z` / `18:08:33.883Z` expiry;
removing a setting does not erase those builds. Real credentials were read
process-locally and transmitted only to the explicitly approved preview and
respective providers; they were not logged or persisted in new files, manifests,
repository, browser storage or clipboard. A final authenticated read-only check
at `2026-10-14T21:37:42.557Z` again showed zero project environment variables;
both immutable execution manifests had expired by then. Owned paid-operator Chrome profiles
were removed and processes exited. No active local P3 approval file remains.

Two P3-A read-only preflight stops preceded any POST/reservation: a dashboard
deadline and an older reader retained by the command bridge. Their cleanup and
history remain. The corrected bridge passes the current reader explicitly and
keeps the five-second guard. P3-B's restricted initial process stopped at Git
subprocess permission before opening Chrome; the reviewed operator then ran with
Chrome's normal sandbox. These are preparation recoveries, not paid retries.

The 19 provider-mode and 21 inspection recorder regressions, budget/approval
rejection checks, 134 Node tests and closed Go preflight retain their preparation
scope. The qualification Go package passed again at the frozen candidate. The
local production build passed; generated tracked assets were preserved privately
for replay and restored in the checkout. Existing build advisories remain.
Earlier full Go/TypeScript/Firefox/heavy-CV checks retain their dated source and
controlled scope; they were not converted into new provider/deployment claims.

Independent metadata validation rechecked all four private capture-file hashes,
all 14 serialized request/response byte counts and hashes, source-file identity,
the exhausted ledger, Profile/PDF observations and all 13 matrix entries/links.
JSON parsing and `git diff --check` passed. The installed formatter rejected
Markdown/JSON targets as unsupported; it is not counted as a passing check.

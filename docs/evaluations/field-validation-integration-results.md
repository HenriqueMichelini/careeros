# Field validation and recovery verification — issue #33

Date: 2026-10-07 (São Paulo). Starting revision: `f7f5fd4b5c16df094041169cda113486c24c1e64`. This is a verification slice, with no production behavior changes. **Acceptance remains incomplete:** live ingestion accepts/extracts the single Java fact but returns no proposed change for an empty Profile in either language. No deployment or issue closure is claimed.

## Frozen inputs and live authorization

The [36-case corpus](field-validation-integration-cases.json) has nine categories per field and language: normal, short, noisy, repetitive, relevant-but-insufficient, irrelevant, unusable, direct attack and uncertain quoted attack. Expected labels were manually reviewed before calls. These are fresh wordings held out from prior provider experiments; semantic categories necessarily overlap, EN/PT counterparts are correlated, and this is not a statistically independent population sample. The [plan](field-validation-integration-plan.json) records the corrected run’s exact pre-call corpus, embedded rubric, production adapter and runner hashes. No labels, rubric or policy changed after seeing results.

Henrique explicitly authorized the existing private evaluation keys for at most 42 Jev classifications and eight OpenAI calls, minimized synthetic data only. Automatic approval review initially required this explicit authorization; no provider call occurred from that rejected command.

The [first executed attempt](field-validation-integration-live.json) made one classification request and stopped on `service_failure/key` in 252 ms. The credential loader mistakenly passed a `NAME=value` file entry as the key. That result is preserved, not counted as a semantic classification. After correcting the loader, a deliberate new run used 36 classifications and five gated workflow requests, staying within **42 total Jev attempts** including the failed first request and **seven OpenAI calls**. There was no automatic retry or fallback. The sixth representative request (Portuguese drafting) was omitted to preserve that total budget. Both runs use the actual production `fieldvalidation.Classify`; workflows call the actual public Go handlers locally with real Jev/OpenAI requests. They do not simulate a deployed Function or browser network path.

## Measured selected-provider decisions

[Raw metadata](field-validation-integration-corrected-live.json) and [offline summary](field-validation-integration-summary.json):

| Measurement | Result |
| --- | --- |
| Decision agreement | 36/36 |
| Professional information / Job Posting | 18/18 each |
| English / Portuguese | 18/18 each |
| Legitimate accepted content falsely rejected | 0/16 |
| Uncertain quoted attacks requesting rephrasing | 4/4 |
| Direct attacks rejected in full | 4/4 |
| Corrected-run operational failures | 0/36 |
| Classification duration p50 / p95 / max | 253 / 303 / 446 ms |

Durations include production adapter request and response validation; they are local wall-clock measurements, with a 3-second classifier deadline. This small serialized sample establishes neither production tail latency nor absolute prompt-injection protection. The initial credential failure remains separate operational evidence. Provider confidence, distributions, token usage and actual billed amounts were not collected by the production adapter. No cost or confidence accuracy claim is made.

## Representative live output inspection

Only the field/submission goes to Jev. The synthetic ingestion Profile is empty; the synthetic Apply Profile contains only the Java skill, empty strings and empty collections. No identity, contact, salary, employer, education or project facts were supplied. Generated output was inspected in a private temporary file; it is not committed. The metadata report contains only case IDs, typed decisions, status, duration and cache checks.

| Local public handler | Language | Status | Total ms | Factual inspection |
| --- | --- | ---: | ---: | --- |
| Profile ingestion, “I use Java” | EN | 200 | 3,978 | Claim correctly says uses Java; **operations empty** |
| Profile ingestion, “Eu uso Java” | PT | 200 | 2,821 | Claim correctly says uses Java; **operations empty** |
| Qualification gaps, short Java/AWS posting | EN | 200 | 2,057 | Only AWS skill gap; no invented candidate qualification |
| Application Draft, short Java/AWS posting | EN | 200 | 2,727 | Java title retained, company `null`, Java-only résumé, AWS experience explicitly unknown |
| Qualification gaps, short Java/AWS posting | PT | 200 | 2,167 | Only AWS skill gap; localized details |

All five responses had `Cache-Control: no-store`. The inspected draft had no invented employers, dates, credentials, achievements or AWS experience; its five answers clearly described absent information. This is representative inspection of one draft, not proof against future hallucination. EN/PT ingestion extracted one claim and no inferred experience, but returned zero operations and zero unresolved/unverified counts. Therefore the agreed single-fact-to-Java-proposal behavior is **unmet in these live runs**, despite passing controlled responses. The browser cannot offer a Java skill addition from those responses. This is a follow-up for Henrique; it must be resolved or explicitly accepted before claiming full feature acceptance. Extraction/comparison completeness work is separately tracked in #25; this verification does not silently expand into that implementation.

## Controlled integration and user-flow evidence

These checks use synthetic provider/HTTP responses and cannot establish real classifier semantics. Public-handler and browser seams were already specified in #30–#33.

- The new `scripts/field-validation/verify/main_test.go` runs unresolved held-out cases through the public ingestion, qualification and draft handlers. Repeated unchanged submissions, client acceptance headers and draft-only qualification confirmations each require fresh classification. Blocked outcomes expose only the decision, never proposals, gaps or drafts. Every intercepted call must be the classifier; a downstream call fails the test. Exact field/submission-only state, the classifier deadline, no-store and absence of keys/submissions/Profile facts in captured logs are checked.
- Existing `backend/profile-ingestion/field_gate_test.go` covers the single fact/noise reaching proposals, all nonacceptance decisions, malformed classifier outputs, provider key/rate-limit/outage/timeout failures, direct acceptance flags, Profile preservation and no retry. Existing `backend/qualification-gaps/job_gate_test.go` covers both Apply endpoints, alternate draft paths, changed inputs, client flags, failure modes, short AWS requirements and bounded exact classifier payloads. Existing downstream handler tests cover invalid generated outputs and schema/domain enforcement.
- `tests/ingestion-browser.check.mjs` and `tests/apply-checklist.check.mjs` pass in EN/PT at 1440px and 390px. New native CDP text input, Tab traversal and Enter activation exercise uncertain-input correction, timeout feedback and explicit retry. Existing cases cover every decision/failure reason, preserved text, unchanged saved Profile, loading, stale-response cancellation, ordinary applicant requirements, noisy facts, title-only clarification, alternate confirmation paths, Results and preview/print handling of unknown metadata. Browser responses are controlled, even when fixtures resemble the agreed examples.
- Saved Profile assertions cover rejected raw text, service failure, proposal rendering, edited/unapproved proposals and canceled requests; existing explicit approve/reject/apply remains covered. Unchanged trimmed uncertain text cannot be submitted; editing away and back cannot bypass revision. Confirmations do not override the backend classification gate.
- Screenshot inspection confirmed readable Portuguese rephrasing feedback and short-job unknown-company presentation at 390px. Layout assertions check horizontal overflow throughout the locale/width matrix. The print checks inspect content passed to `window.print`; they do not inspect a real exported PDF. Keyboard checks establish reachability and activation, not a full screen-reader audit or every confirmation-dialog focus path.

## Privacy, runtime and evidence boundary

Source inspection of the three public handlers, shared classifier and ADR 0001 confirms stateless requests, browser-local user keys, fixed workflows, no shared billing, no runtime persistence and allowlisted metadata logs. Jev receives only the field and complete submission. Downstream prompts use their existing Profile projections; supplied synthetic values were minimized as described above. Source and controlled checks establish output validation and fresh gating; the actual five local live responses establish no-store on success. Captured controlled logs contain no submission, Profile facts or keys; inspected live console lines likewise contain only workflow/status/outcome/duration. Deployed Function logs were not accessible or reviewed in this slice.

Production bounds remain: classifier 3 seconds; ingestion shared 52-second context with existing 24-second downstream calls and 55-second browser bound; Apply classifier plus 25-second generation bounds within the 30-second browser bound. Redirects, retries and provider fallback are disabled. Resource caps and malformed-response rejection remain enforced. The sampled live totals were below those bounds. Maximum-size live inputs, cancellation under production load, cold starts, concurrency and real upstream failure recovery were not measured; timeout/outage/rate-limit/malformed failures are controlled evidence only. TypeSafe retention/account policy was not re-audited; the approved disclosure and user-key arrangement remain unchanged.

No deployed revision, Netlify behavior, release accessibility, deployed provider latency, Function log privacy or real browser-to-provider integration is verified here. Historical #29 provider results and #32 browser results are context, not substitute current deployed evidence.

## Reproduction and check results

Offline, no keys or billable calls:

```sh
GOCACHE=/tmp/careeros-go-cache go test ./scripts/field-validation/verify
node tests/field-validation.test.mjs
node scripts/field-validation/report-integration.mjs
node tests/ingestion-browser.check.mjs
node tests/apply-checklist.check.mjs
```

A new live run requires its own explicit authorization and budget. Verify the plan hashes before calling, keep credentials process-local (parse assignment files rather than forwarding their full content), and choose new output paths. The runner deliberately refuses existing paths and stops on service failure. Do not overwrite the preserved runs or treat an offline replay as another measurement.

Final checks: TypeScript typechecking, Vite production build, Go suite and both controlled browser suites pass. The full Node suite has five passing files and one unchanged failure: `tests/cv-preferences.test.mjs:41` expects 12 while `CV_FONT_DEFAULT` is 14. Both values are present at the starting revision; this verification does not change that unrelated behavior. The added summary regression failed before the summary module existed, then all 16 focused field-validation tests passed. It keeps missing/failed evidence distinct from classification accuracy.

## Unmet or unverified acceptance coverage

1. **Observed gap:** both live Java-fact ingestion calls produce no skill proposal for an empty Profile. HTTP success and correct extraction do not satisfy that user behavior.
2. **Not measured:** Portuguese live drafting, noisy-input live extraction/proposals and complete-posting live drafting in this new run. Controlled browser/handler paths pass; the corresponding live classifier cases do pass.
3. **Not verified:** deployed environments, real provider failure recovery, maximum-size/load behavior, actual exported PDFs and full assistive-technology accessibility.
4. **Known unrelated check failure:** CV font preference default test, already failing at baseline.

The verification artifacts are complete, but these limitations prevent treating issue #33 or the whole field-validation feature as fully accepted. No issues were closed or new follow-up issues published.

## Standards

Independent code-review agent: no hard violations or remaining actionable findings. The initial nonblocking duplication observation was addressed by sharing the native CDP keyboard helpers between browser checks. The reviewer confirmed preserved behavior and matching frozen hashes.

## Spec

Independent code-review agent: no new correctness defects, scope creep or unsupported success claims. Two disclosed acceptance gaps remain: the live single-fact ingestion produces no proposal, and current live downstream sampling omits noisy ingestion, complete-posting drafting and Portuguese drafting. These are acceptance limitations, not successful checks.

Review totals: Standards 0 remaining findings; Spec 0 new correctness findings / 2 acceptance gaps, with the missing live Java proposal the principal unresolved behavior.

# Milestone #1 remediation — 2026-10-08

Status: Java proposal behavior now passes the authorized live-provider payload checks and public-handler replay. Current `main` is integrated locally. Preview/production verification is still outstanding; Milestone #1 is not yet accepted as shipped.

The first attempt's HTTP 401 came from a harness credential-loader error: it sent the environment assignment rather than its value. This did not establish that the user's credential was invalid. After explicit same-credential retry authorization, the corrected loader completed 8 TypeSafe and 14 OpenAI calls with HTTP 200, without provider retries. The original failed attempt remains in its historical metadata record. No real saved Profile data was used or changed.

## Correction and evidence

The public ingestion handler accepted an extracted new Java claim plus an empty comparison result without unresolved feedback. A new public-handler regression reproduced this exact behavior in EN/PT, both plain and noisy text, across empty/populated synthetic Profiles. All eight cases failed before the correction and pass afterward.

The comparison boundary now accounts for every clear claim: a validated operation (including the strictly source-derived proposal described below), an exact saved capability duplicate, or an unresolved claim. Claims with a question already have a clarification disposition. Identical operations retain deduplication; unsafe claim groups remain unresolved. Semantic duplicates without an evidenced operation are conservatively unresolved. No extra calls, automatic repair, provider fallback or automatic Profile mutation were added.

Comparison instructions now explicitly require an editable Java skill addition for the agreed positive example when absent, without inferring proficiency, years, employer or project. This prompt correction has **not** reached OpenAI validation: the first classifier request returned HTTP 401. Omission feedback passing does not establish that the promised live proposal is fixed.

EN/PT recovery copy now describes an unavailable safe change without asserting that a conflict was found. The browser regression checks this feedback and absence of the possible-duplicate message at 1440/390px. The existing controlled review/edit/reject/apply/reload and cancellation suite passes across empty/populated Profiles. The test uses intercepted API responses, not live providers.

Verification: Go tests across all packages, Go vet, TypeScript, temporary production build, and ingestion browser suite pass. The full Node suite remains five of six test files passing; the pre-existing CV default test still fails (expects 12, implementation uses 14). CV design is unchanged. Current provider/status documentation was corrected; historical evaluation records are preserved.

## Approved provider validation — stopped on HTTP 401

The [exact synthetic request fixtures](field-validation-remediation-cases.json) define eight deliberate local public-handler requests, with no saved user data:

| Cases | Synthetic Profile | Required outcome |
| --- | --- | --- |
| EN/PT “I use Java” / “Eu uso Java” | Empty | One editable Java skill proposal; no inferred proficiency/experience |
| EN/PT positive fact among harmless noise | Skills: React | Java addition; preserve React and ignore noise |
| EN duplicate | Skills: Java | No repeated Java addition |
| EN representative large text (about 12 KB) | Skills: React | Recover Java without invented facts; this is not a maximum-size performance claim |
| PT short posting | Skills: Java | Draft in PT; unknown title/company remain null; no invented Profile facts |
| EN complete fictional Aster Labs posting | Skills: Java | Correct job metadata; no claimed PostgreSQL qualification or invented experience |

Maximum budget: **8 TypeSafe classifier calls + 14 OpenAI generation calls = 22 provider calls** (six ingestions at two OpenAI calls each, two drafts at one each). No standalone classification corpus, retries, fallback or automatic repair calls. Stop on an operational failure or a failed required proposal outcome; unused calls are not permission for replacement experiments. Costs depend on provider token billing; this is a call-count cap, not a dollar estimate.

Before any real call, inspect the actual outbound payload against these fixtures and current server prompts/schemas. TypeSafe receives only the field and exact submission. OpenAI extraction receives the synthetic submission; comparison receives the extracted claims and allowlisted synthetic Profile projection. Drafting receives the synthetic Profile, posting, empty confirmations and document language. Reject any unexpected recipient, field or non-fixture personal data. Dynamic extraction output must be reviewed before comparison proceeds. Models remain Jev `jev-1.13.0` and OpenAI `gpt-6-luna` with reasoning `none`. Do not print or persist credentials.

The user explicitly approved this eight-case, 22-call run in chat on 2026-10-08. It stopped after the first request returned HTTP 401, as required. A refreshed credential and fresh retry authorization are needed before any replacement run. Load user-owned keys process-locally for local checks; record metadata-only status, no-store, workflow duration, revision/prompt hashes, call counts and factual outcome. Keep generated content private for inspection. These checks incur potential charges and disclose synthetic text to the two already-approved providers. They do not verify a deployed browser or production runtime.

## Release work still outstanding

A fresh read-only GitHub comparison confirms that `codex/backend-v1-queue` at `8f2a01a` remains 27 commits ahead and 24 behind current `main`, with merge base `10c43ca`. Local `main` is stale and must not be used as the release integration baseline. Branch integration and deployment require a separate authorized release candidate. Follow the existing [release-test procedure](../release-test-path.md): identify the exact deploy revision, use tester-entered dedicated keys in an isolated browser, verify all three Function gates and successful no-store responses, record host duration, and inspect read-only Function logs. Do not transmit vault keys to Netlify through the agent. Do not close the milestone or change GitHub acceptance status on the strength of local tests.

## Authorized run result

The first exact approved TypeSafe payload (professional_information, “I use Java”, pinned Jev rubric) returned HTTP 401 in 234 ms of local HTTP time. Calls used: **1 TypeSafe, 0 OpenAI**. No second request was attempted; the unused budget does not authorize replacement experiments. No generation/proposal-quality outcome can be inferred. The upstream error is an access failure, not an observed new semantic failure.

The inspection harness exports each exact production-handler payload, waits for inspection, sends that payload once, and replays its response through the unchanged public handler to construct the next request. This keeps preflight inspection outside production deadlines. These are live HTTP provider requests plus offline handler replay, **not** a continuously running live handler or browser workflow; replay durations must never be reported as live workflow timing. The run stopped before a complete workflow. The replay-only helper is in `scripts/field-validation/remediation/main.go`; private payload/response artifacts are under `/tmp/careeros-milestone-1-remediation/private/` and contain no stored credentials.

The [metadata record](field-validation-remediation-run.json) records only scope, counts, response status, payload hash and local HTTP duration. Original historical Java failures remain unchanged. Goal and milestone completion remain unachieved.

## Offline continuation: recover the promised single-fact proposal

Omission feedback alone did not fulfill the promised positive example. A second public-handler regression required the actual Java addition when comparison returns an empty operations array. It failed in all 16 combinations before the further correction and passes afterward: EN/PT exact source statements, plain/noisy submissions, and saved skills empty/React/Java/JavaScript. Saved Java yields zero duplicate operations; all other cases yield exactly one reviewable Java skill addition. The input Profile stays unchanged and the provider count stays at the original classifier/extraction/comparison sequence.

The server now derives a minimal skill proposal only when an already source-verified, unambiguous claim targets skills alone and its complete source excerpt is a positive EN/PT `I use <single identifier>` / `Eu uso <single identifier>` statement. The skill value comes directly from that source identifier. The rule neither parses general prose nor invents proficiency, dates, employers or projects. All derived operations pass the existing operation validation and capability deduplication, remain subject to the 60-operation bound, and require the existing explicit review/apply flow. A claim with rejected provider operations is not recovered. This is a local, source-grounded proposal rule, not a provider fallback or an extra generation call.

Regression checks cover negation, qualifications, multiple facts/destinations, unsafe supplied operations, ambiguous claims, repeated positive claims, saved cross-section capabilities, and unverified source excerpts. Existing attack-precedence/gate tests still pass: blocked input never reaches extraction/comparison or this rule. Nontrivial omitted claims continue to receive unresolved feedback. The earlier eight-case omission regression now exercises general Java-service descriptions; the stronger 16-case regression covers the exact positive Java examples.

All Go package tests and vet pass after this change. The prior EN/PT browser lifecycle checks remain applicable to the unchanged proposal wire format and frontend; they were run before this further backend correction. No new browser/live-provider/deployed outcome is claimed. The TypeSafe 401 still prevents a fresh complete provider-backed run. The run metadata's ingestion source hash describes the earlier attempted run and is intentionally preserved as historical evidence.

Current ingestion source SHA-256: `c517bfd89c68d2abfe3fd9b3a641f9489196b6afa607df1385750f5868f09be1`.

## Same-credential retry and integration — current result

The user authorized the same credential, retry, branch integration and deployment verification. Correct parsing of the existing environment-assignment credential files resolved the 401. The [new metadata record](field-validation-remediation-retry-results.json) preserves 22 successful HTTP attempts (8 Jev, 14 OpenAI), payload hashes, and their local HTTP times. Exact outbound payloads were inspected before sending; static classifier/extraction payloads were checked against the already reviewed templates and approved inputs, and dynamic comparisons/drafts were inspected separately. All six ingestion examples produced the expected outcome, including no operation for existing Java. PT short drafting preserved null title/company. The complete EN draft preserved Java Developer/Aster Labs and explicitly withheld unsupported PostgreSQL, API-work and testing experience. The PT response supplied four application-answer items despite the prompt requesting five or six; this does not establish full prompt compliance beyond the recorded milestone fidelity checks.

A harness-only request-shape error initially rejected the PT drafting fixture locally before any outbound provider call. Unsupported empty inbound fields were removed; no facts were added, and completed calls were replayed without resending. The same approved eight-case run then finished within its original 22-call cap. Historical failed access remains visible and is excluded from success latency statistics. These results establish live provider payload behavior plus production-handler replay, not a continuously timed live handler, deployed browser success, or host performance.

Current main `fc91b12` was merged cleanly into the feature branch, preserving the canonical GitHub tracker migration. Go tests, vet, TypeScript, production build, full Node suite (6/6 files), and both controlled EN/PT browser suites pass on the integrated candidate. The stale CV assertion now expects the existing 14pt application default; no CV source/design change was made. The first concurrent ingestion-browser run hit a navigation race, and its isolated rerun passed all cases. Generated Vite metadata was restored. The unrelated untracked `cmd/application-draft/` directory remains excluded from the candidate.

Deployment verification remains separate. The authenticated Netlify dashboard currently identifies published deploy `6ac5ab6a0a6ca2000762e50d` at main `fc91b12`; auto-publishing is locked. A new exact-revision preview will be verified before any publication decision. No deployed provider call or runtime/log privacy result is claimed by this local report.

## Merged candidate and release verification

PR [#53](https://github.com/HenriqueMichelini/careeros/pull/53) merged remediation commit `0c7dbbc` into main as `31371b4f8739a46b2983de27411855153eabe478`. Both the exact-head preview and merged-main Netlify builds succeeded. The published app remains at `fc91b12` because auto-publishing is locked. The [release verification record](../release-evidence/2026-10-08-field-validation-release.md) identifies each deploy and preserves the failed browser credential-handoff attempt, zero provider calls from that attempt, credential-free reachability results, and outstanding success evidence. The milestone remains incomplete; merged/build-success and the passing local provider run do not establish publication or deployed provider success.

The user subsequently authorized publication of exact main build `31371b4` and three direct public HTTPS workflow checks. Netlify now marks deploy `6ac7b3c38bc6d40008084b95` published and locked. All three checks passed: editable Java proposal, PT short-posting gaps, and PT short-posting drafting with null title/company and no invented experience. Every successful response was no-store with Durable bypass. Selected Function logs were metadata-only; handler durations were 4142/1558/2904 ms. This consumed at most 3 TypeSafe + 4 OpenAI calls, without retries, in addition to the prior local 22-call run. Separate browser inspection confirms the published TypeSafe UI/terms. Protected-preview live browser success remains unverified, and no milestone closure is claimed. See the release record for request IDs, exact revision and timing boundaries.

# Application Draft fix and EN/PT workflow measurements — 2026-10-07

Issue [#29](https://github.com/HenriqueMichelini/careeros/issues/29). Henrique selected **fix and measure EN/PT workflows** after approving Jev classification and its provider/architecture consequences. The previously unavailable successful drafting-overhead measurement is now available. This closes the local evaluation gap; it does not establish deployment, browser behavior or production tail reliability.

## Fix and regression evidence

Commit `18d45da` replaces Application Draft's outbound JSON mode with the [reviewed strict response format](application-draft-response-format-proposal.json): five string fields plus a three-string-field cover-letter object, all required, with extra properties forbidden and the existing allowed closings enumerated. It preserves the public response contract, fixed `gpt-6-luna` model, prompt, reasoning effort, token cap, deadlines, user-owned key and single-request behavior. Downstream nonempty and cover-letter validation remains intact.

At the user-approved public `NewHandler()` HTTP seam, a failing-first regression reproduced HTTP 502 before the change and passes after it. A second regression verifies that an array-valued `applicationAnswers` still produces HTTP 502 `invalid_output`, `no-store`, and one provider request. The fix does not coerce malformed responses or retry them.

## Successful paired measurements

The unchanged evaluation runner used `en-complete-job-timing` and `pt-complete-job-timing`: a fictional employer with a complete Java posting and a minimized Java-only Profile. Baseline runs first; Jev explicit Choice classification then precedes a separate downstream draft request. The validated path shares the remaining **30-second whole-workflow deadline**; the exploratory classifier cap is **5 seconds**. The approved proposal's 3-second integration target remains a future integration setting, not the trial parameter.

| Language | Baseline draft | Jev + draft, total | Paired difference | Jev classifier | Both handler responses |
| --- | ---: | ---: | ---: | ---: | --- |
| EN | 3,109 ms | 3,396 ms | +287 ms | 489 ms | 200 |
| PT | 2,774 ms | 4,016 ms | +1,242 ms | 287 ms | 200 |

Both Jev results were `job_with_context` / `none`, permitting downstream processing. All four OpenAI responses finished normally, returned the requested model and passed the real handler's response validation, including string-valued answers and cover-letter checks. There were no failures, refusals, automatic retries or fallbacks. Whole-request timing includes complete response consumption and the temporary metadata instrumentation.

These are **one pair per language**, serialized EN then PT with baseline first, on a local loopback server. Provider variability explains why the paired difference need not equal classifier duration. Ordering, load and cold/warm conditions were not controlled. The observed times fit the bound for these attempts; they are not a reliable p95, maximum-input test or production guarantee. Earlier EN/PT ingestion and qualification-gap pairs remain separate evidence in the [follow-up report](field-validation-followup-results.md).

## Usage, cost and privacy evidence

| Provider | Calls in this follow-up | Input / output tokens | Dated reference-rate estimate |
| --- | ---: | ---: | ---: |
| OpenAI generation | 4 | 2,808 / 961 | $0.000761300 |
| Jev classification | 2 | 2,180 / 216 | $0.000091560 |
| Combined | 6 | Provider-specific totals above | **$0.000852860** |

OpenAI reported zero cached input tokens and default service tier. The reference-rate calculation uses the dated [provider comparison](field-validation-provider-sources.md); actual billed amounts and account balances remain unknown. These six calls are additional to the earlier 308 follow-up classifier calls, 12 workflow-stage calls and two diagnostic calls. Do not combine their partially metered downstream costs into a fabricated complete bill.

An exact outbound-payload preflight ran both language requests through the actual handler using a fake transport, before any new live call. The temporary live transport then admitted only those two request hashes, the fixed OpenAI endpoint and at most four generation calls. It captured sanitized status, usage, model-match and validation metadata; request headers, keys and generated prose were not persisted. Jev received only the synthetic posting/field and the unchanged selected questions.

Automated in-memory spot checks found Java in every resume, no experience/education/certification/language section headings and no numeric years claims. This is **not a full semantic fidelity review**: unsupported claims elsewhere, nuance, generated-answer quality and language quality are not established by those checks. Broader output/factual review remains a later integration/release concern. Responses were discarded after consumption; no Profile proposal was applied or stored.

## Reproducible records

The [raw folder](results/2026-10-07/draft-fixed/summary.json) contains both complete paired-run metadata records, a four-call provider metadata sidecar, preflight request hashes and the exact temporary [measurement-server source](results/2026-10-07/draft-fixed/measurement-server.go.txt). The [summary](results/2026-10-07/draft-fixed/summary.json) records source SHA-256 hashes and the measured backend commit. The instrumentation source is an executed artifact, not production code; its `/tmp` files must be prepared from the existing runner's `buildWorkflowPayload` before reuse.

The two deliberate runner invocations were:

```sh
node scripts/field-validation/measure-workflow.mjs jev en-complete-job-timing application_draft http://127.0.0.1:8787 explicit
node scripts/field-validation/measure-workflow.mjs jev pt-complete-job-timing application_draft http://127.0.0.1:8787 explicit
```

Credentials were supplied process-locally from the user's existing key files, never as command arguments. Repeating these commands makes additional billable calls; the recorded results and source can be reviewed offline. UTC run IDs fall on October 8; the local evaluation date is October 7 in São Paulo.

## Checks and acceptance

Focused draft tests, `GOCACHE=/tmp/careeros-go-cache go test ./...`, and TypeScript typechecking pass. The final full Node suite is 51/52; its sole failure remains unchanged `tests/cv-preferences.test.mjs:41` (expected font12, actual14). No frontend code changed. Independent pre-call Standards and Spec reviews found no actionable patch issues.

Provider approval is recorded in [ADR 0001](../adr/0001-stateless-user-key-backend.md), and the user-authorized drafting fix/measurement is complete. Issue #29's local evaluation deliverables are complete; dependent integration, publication and deployment have not begun. Maximum-input/load checks, real browser/deployment timing and later source-fidelity validation remain explicit integration/release work rather than claims of this evaluation.

## Final independent review and completion audit

Standards and Spec reviewers independently checked the fix, source hashes, raw paired records, token totals and cost arithmetic. Both report **zero actionable findings**. The Spec review confirms completion of #29's local evaluation deliverables while retaining the integration/release limits above.

| #29 requirement | Authoritative evidence |
| --- | --- |
| Labeled bilingual corpus and agreed examples | [34 core cases](field-validation-cases.json), [64 follow-up cases](field-validation-followup-cases.json), [24 pre-reviewed confirmation cases](field-validation-confirmatory-cases.json) |
| Accuracy, legitimate rejection, uncertainty, latency, cost and privacy comparison | [Follow-up results](field-validation-followup-results.md), [confirmation](field-validation-confirmatory-results.md), [dated provider documentation](field-validation-provider-sources.md), preserved raw records |
| Stable typed contract and separate semantic/routing decisions | [FieldDecision v1](../../src/lib/fieldDecision.ts), focused decision/evaluation tests |
| Evidence-based routing with no universal confidence/security claim | [Categorical policy](field-validation.md#contract-and-routing-policy), retained downstream validation requirements |
| Added validation within bounded existing workflows | Earlier successful EN/PT ingestion/gaps pairs plus the successful EN/PT drafting pairs recorded here |
| Provider and architecture approval before integration | Henrique's explicit Jev approval, [ADR 0001](../adr/0001-stateless-user-key-backend.md), no dependent integration begun |
| Authorized minimized synthetic calls, stateless keys, metadata logs and deliberate retries | Exact payload preflight, preserved instrumentation/metadata, two deliberate pair invocations, no retries/fallbacks or persisted generated prose |
| Fix/check/review/commit workflow | Red-to-green public-handler regression, Go suite/typecheck results, unchanged Node failure disclosure, both independent reviews, commit `18d45da` and this record's subsequent commit |

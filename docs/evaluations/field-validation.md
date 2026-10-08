# Field-validation evaluation — issue #29

Date: 2026-10-07. Status: **evaluation complete; Jev Choice approved; successful EN/PT drafting measurements recorded**. Source of behavior: [agreed design](../input-validation.md); [GitHub issue #29](https://github.com/HenriqueMichelini/careeros/issues/29). No production workflow uses this evaluation code. Issues #30–#33 must not start on the basis of fixture tests or documentation claims.

## Provider decision and exploration

**Approved decision: pinned Jev `jev-1.13.0` with the explicit two-Choice rubric for classification; retain OpenAI for generation.** Henrique explicitly approved the provider proposal on 2026-10-07, including user-owned TypeSafe keys/credits, the additional TypeSafe data recipient and an ADR update. The [follow-up results](field-validation-followup-results.md) supersede the first-round recommendation: revised Jev and matched OpenAI both scored 40/40 on a fresh held-out bilingual set; revised Jev's measured classifier p50/p95 was 257/307 ms versus 929/1,471 ms. The atomic Jev variant scored 38/40. A further [24-case fresh-presentation confirmation](field-validation-confirmatory-results.md), independently reviewed before calls, scored 24/24 for each provider with unchanged questions. The original 40-case cohort has a disclosed template overlap; its counts are preserved separately. No product integration has begun.

The first round favored OpenAI (34/34 versus initial Jev 26/34). The user requested deeper documentation research and then authorized billable follow-up tests. We froze two candidate question sets, independently authored calibration/held-out cases and selected explicit Choice on calibration before held-out inspection. Original records remain unchanged; the [research note](jev-improvement-research.md), [frozen plan](field-validation-followup-plan.json), [selection](field-validation-followup-selection.json) and [new results](field-validation-followup-results.md) distinguish proposed, calibration and held-out evidence. The initial 260-call follow-up and separate 48-call confirmation completed without operational failures, alongside four successful local paired workflow probes. The offline reporter's missing-usage handling was corrected with a failing-first regression; original fully metered totals are unchanged.

Billable-test authorization and provider approval are separate records; both have now been supplied. Two subsequent [diagnostic calls](field-validation-draft-diagnostic.md) isolated an array/string mismatch in `applicationAnswers` in one response. Henrique selected fix-and-measure at the public handler seam. The [strict-schema fix and EN/PT measurements](field-validation-draft-fixed-results.md) now supply successful drafting-overhead evidence: baseline / Jev-prefixed totals were 3,109 / 3,396 ms in EN and 2,774 / 4,016 ms in PT.

The approved amendment to [ADR 0001](../adr/0001-stateless-user-key-backend.md) changes its OpenAI-only classification decision. The integration will require a TypeSafe account, user-owned key/credits and disclosure of the additional data recipient and documented retention uncertainty. OpenAI remains necessary for downstream workflows. No shared secret, backend persistence, automatic fallback, retry, SDK installation or deployment has been added.

## Reproducible artifacts

- [34 synthetic cases](field-validation-cases.json): balanced English/Portuguese, both fields, messy/repetitive text, ordinary noise, minimal facts, short jobs, title-only jobs, applicant instructions, unrelated/unusable/blank input, quoted uncertain commands and mixed detected commands. Each has an independently specified expected decision and downstream fidelity constraints. These deliberately compact cases are a diagnostic set, not a statistically representative safety benchmark.
- [Typed v1 contract](../../src/lib/fieldDecision.ts): semantic adapter signals are separate from deterministic routing. Provider confidence is not a contract field. No UI or API integration is included.
- [Runner](../../scripts/field-validation/evaluate.mjs): scores recorded outputs, exposes missing cases, reports accuracy with and without service failures, legitimate-content blocking, uncertain and detected attack routing, per-language/category/field results, token usage and latency distributions. `fixture` results are explicitly not provider evidence. Run identity and mode are operator assertions requiring provenance review.
- [Collector](../../scripts/field-validation/collect.mjs): same semantic rubric for the existing OpenAI model (`gpt-6-luna`, reasoning `none`, JSON mode) and pinned `jev-1.13.0` Choice questions. Labels/constraints never enter provider requests. Only corpus text and field enter the data boundary. One request per explicitly named case, sequentially, no retries/fallback, 5-second exploratory classification deadline. Stop on the first service/invalid-output failure; resume only by deliberate invocation of remaining IDs. A timeout can still incur provider cost.
- [Workflow probe](../../scripts/field-validation/measure-workflow.mjs): local paired baseline vs classifier-plus-existing-handler timing, consumes the entire response, uses the relevant browser-equivalent deadline, never sends rejected/uncertain content downstream, and does not save proposals. Successful pairs enter the timing summary; failed attempts remain explicit metadata in the run. This is an HTTP probe, not browser/deployment evidence.
- [Fresh confirmation](field-validation-confirmatory-results.md): 24 pre-reviewed new-presentation cases per provider with unchanged selected questions, corrected usage accounting and full provenance.
- [Jev follow-up results](field-validation-followup-results.md): frozen variants, independently authored 24-case calibration / 40-case held-out set, matched provider comparison, full distributions, repeat/order checks and new local workflow timings.
- [Jev improvement research](jev-improvement-research.md): original hypotheses and further bounded opportunities; linked follow-up records measured outcomes separately.
- [Primary-source comparison](field-validation-provider-sources.md): dated prices, capability/retention claims, API/authentication, local source bounds and caveats.

The downstream constraints (Java without inferred experience, ignored grocery lists, unknown company/details, no partial recovery of attacks) are a manual review checklist for later integration. This classifier-only evaluation does not measure extraction fidelity or prove the existing generation schema supports unknown details.

## Current evidence

The [first-round live comparison](field-validation-live-results.md) superseded the initial unavailable state; its recommendation is now superseded by the [documentation-guided follow-up](field-validation-followup-results.md). Both full-corpus runs completed 34 requests with no operational/shape failures: OpenAI 34/34 correct, Jev 26/34. OpenAI correctly routed 4/4 uncertain cases; Jev treated all four as confirmed attacks. Neither rejected any of the 12 accepted legitimate cases. Privacy claims remain documentation evidence; invoices, organization retention settings and browser/deployment timing are unavailable. Successful local drafting measurements are now recorded in the [fix follow-up](field-validation-draft-fixed-results.md).

The [offline empty report](field-validation-unavailable.json) is retained as a no-input runner example, **not the current evaluation status**. Running the following does not contact a provider and correctly emits null measurements when no records are supplied:

```sh
node scripts/field-validation/evaluate.mjs
```

Measured usage and reference-rate estimates are distinct from actual billed amounts. The runner reports billed cost as unknown. See the live report for every setup failure, smoke, scored corpus run and workflow attempt, including the private credential-format correction and malformed probe projection that were excluded from classification scores.

## Contract and routing policy

`FieldDecision` contains `version: 1`, `field`, and a discriminated `outcome`. The outcomes are `accept`, `request_information` with the required kind of context, `request_rephrasing`, `reject_attack`, `irrelevant`, `unusable`, or `service_failure` with `key`, `rate_limit`, `timeout`, `outage`, or `invalid_output`. Go integration should mirror this wire vocabulary rather than interpret confidence values ad hoc.

The adapter returns content sufficiency and attack status independently. Invalid shapes/enums, extra keys, and signals incompatible with the field produce `invalid_output`. For valid signals, deterministic precedence is:

1. `detected` blocks the **whole submission**, including useful facts.
2. `uncertain` pauses the whole submission for revision and validation; no continue-anyway override.
3. Unrelated or unreadable content routes to `irrelevant` or `unusable`.
4. A job title alone or insufficient relevant information requests missing context.
5. A professional fact, or job role plus responsibility/qualification, permits downstream validation/review.

These semantic thresholds come from the approved examples, not a fitted numeric confidence cutoff. **No numeric safety threshold is recommended from this small uncalibrated set.** The live comparison scores categorical routing directly and preserves every mismatch. A separate held-out bilingual set is needed before fitting any provider-specific abstention/confidence threshold. Do not tune and claim accuracy on the same examples. A low-confidence choice must not be silently converted into a confirmed attack; ambiguous semantics require rephrasing, while transport/schema failures remain service failures.

The original configurations pass the following candidate approval criteria for OpenAI and fail them for Jev: all six agreed example behaviors must route correctly in both languages; no detected-attack fixture may reach downstream processing; no legitimate applicant requirement may be blocked as an attack; all quoted-attack cases must request rephrasing. Report every error and every failure, not only an average accuracy. Passing this small set is necessary evidence, not a perfect-security guarantee. More paraphrases, option-order tests for Jev, and held-out ordinary inputs are needed before confidence calibration or broad quality claims.

All pasted content remains data after acceptance. Schema/domain validation, source provenance, explicit Profile Proposal apply, stale-response handling and deliberate retry remain required. The supporting policy reference for the eventual attack notice belongs to the integration slices; this evaluation does not invent terms.

## Human credential setup and deliberate live commands

You need an OpenAI API key with funded access and a TypeSafe dashboard key with available credits. Do not paste them into chat, put them in command arguments, commit them, or copy browser-local Profile/key data into the corpus. Supply environment variables `OPENAI_API_KEY` / `TYPESAFE_API_KEY`, or a user-owned regular JSON file outside the repository with mode `0600` and keys `openai` / `jev`. Set `CAREEROS_EVAL_KEY_FILE` to its path; the tools never print the file contents. Delete the temporary credential file when the evaluation is finished.

One deliberately selected smoke case per provider:

```sh
node scripts/field-validation/collect.mjs openai en-fact > /tmp/careeros-openai-smoke.json
node scripts/field-validation/collect.mjs jev en-fact > /tmp/careeros-jev-smoke.json
node scripts/field-validation/evaluate.mjs /tmp/careeros-openai-smoke.json /tmp/careeros-jev-smoke.json
```

After reviewing smoke access/usage, select corpus IDs explicitly, e.g. `en-fact pt-fact en-short-job pt-short-job en-title-only pt-title-only en-applicant pt-applicant en-quoted-profile pt-quoted-profile en-attack-profile pt-attack-profile`. Each selected ID incurs at most one request in that invocation. Do not put an automatic retry loop around the commands. For a full run, supply all IDs from the corpus once per provider. Keep attempts, failures, run IDs and returned models; report unavailable remaining cases if the run stops. Do not merge distinct runs under a fabricated run ID.

For a complete synchronous measurement, use the existing local Go preview server (default loopback port 8787), then deliberately run one baseline and one validated workflow (the original posting remains useful for exposing the drafting limit; `en-complete-job-timing` / `pt-complete-job-timing` are separate fully specified timing fixtures):

```sh
node scripts/field-validation/measure-workflow.mjs openai en-fact profile_ingestion > /tmp/careeros-openai-ingestion-pair.json
node scripts/field-validation/measure-workflow.mjs jev en-messy-job qualification_gaps > /tmp/careeros-jev-gaps-pair.json
node scripts/field-validation/measure-workflow.mjs jev en-messy-job application_draft > /tmp/careeros-jev-draft-pair.json
```

The probe uses a synthetic Java-only Profile. The OpenAI workflow key is required even when Jev classifies. Baseline failure stops the pair; rejected classification prevents the second downstream call. A successful pair means two downstream workflow invocations plus one classifier invocation, potentially five provider calls for two-stage ingestion. Existing unknown-company draft restrictions may fail; retain that limitation rather than invent details to obtain a green run. Repeat manually across both providers/languages/workflows only after reviewing each attempt and available budget. Complete Apply also includes qualification confirmation, user interaction and draft generation; these individual probes do not establish live UI latency.

## Runtime budget and acceptance limits

Local source establishes two ingestion stages with independent 24-second upstream deadlines (up to 48 seconds before overhead), a 55-second ingestion browser timeout, and 25-second upstream / 30-second browser timeouts for qualification gaps and drafting. Hosting's documented synchronous limit in ADR 0002 is 60 seconds; live host eligibility/revision has not been reverified. Classifier-only speed must not be substituted for complete workflow behavior.

The exploratory 5-second classifier cap is the actual trial parameter. The live report proposes a **3-second integration target inside a shared remaining deadline**, based on the observed sample and explicit untested limits; this is not a measured production guarantee. Nominal worst-case sums are 53 seconds before ingestion overhead and 30 seconds before other workflow overhead, leaving insufficient demonstrated margin. Integration must use a shared remaining deadline and may need a smaller classifier budget or a redesigned bounded workflow. Do not recommend deployment if measured total workflow time cannot fit the browser and host bounds reliably. Paired latency measurements include overhead rather than adding independently sampled p95s. Record sample counts, cold/warm conditions, response status, failures and actual whole-request durations; a few successful samples do not establish tail reliability.

## Evaluation acceptance and remaining integration work

1. Henrique approved the documented Jev proposal, including user-owned TypeSafe keys/credits, additional recipient and ADR amendment, retaining OpenAI generation.
2. Henrique selected the drafting fix and EN/PT measurement. Commit `18d45da` enforces the existing output contract; both live pairs succeeded within the 30-second probe deadline. The [results](field-validation-draft-fixed-results.md) resolve the previously unavailable drafting measurement.
3. The typed contract, labeled provider comparison, routing policy, privacy/cost bounds and all three local workflow contexts are recorded. Issue #29's local evaluation deliverables are complete; the GitHub issue is not closed by this local record.
4. Later integration still requires shared remaining deadlines, the proposed 3-second classifier setting, downstream schema/domain/source validation, maximum-input/load checks and browser/deployment coverage. Evaluation success does not establish those properties.

Dependent integration, GitHub publication and deployment have not begun.

## Local verification

The focused decision/runner checks pass, including fake transport responses through the evaluation runner (no provider evidence). TypeScript typechecking, the Vite production build and `GOCACHE=/tmp/careeros-go-cache go test ./...` pass. The full Node suite has one existing unrelated failure: `tests/cv-preferences.test.mjs:41` expects the default font size to be 12, while unchanged `src/lib/cvPreferences.ts` returns 14. Both files match the starting commit. The isolated test reproduces the failure; it has not been altered in this evaluation work. Build outputs are restored/excluded from the commit.

## Code review

Reviewed the issue-only staged diff against starting commit `9adf00a3545406a160dbc9ac67929149a13b9e14` in separate standards/specification agents.

**Standards:** no documented violations or correctness/privacy defects. One nonblocking judgment: the workflow timeout map is duplicated in the probe and reporter; both currently agree.

**Spec (initial preparation review):** no actionable implementation defects or scope creep. At that point measured provider comparison and final approval were pending. Live evidence is now captured separately; successful drafting-overhead evidence and provider approval remain unresolved. A follow-up review covers the live artifacts and corrected probe.

Initial review totals: standards 0 violations / 1 nonblocking smell; spec 0 correctness defects / 2 then-pending acceptance areas.

## Live-evidence follow-up review and checks

The separate standards/specification reviews reproduced the measured counts, token totals, latency percentiles and workflow rows. No documented standards violations, credential disclosure, scope creep or unsupported current measurement claims were found. The reviewer identified that mixed supplemental/core runs could inflate summary accuracy even though coverage was separate. A failing-first regression now verifies separate core and supplemental summaries; the runner has been corrected. Published 34-case results were unaffected.

All 11 focused checks and TypeScript typechecking pass. The final full Node suite is 47/48, with the same unchanged CV font-default expectation failure described above. No app or Go implementation changed in this follow-up. Two acceptance gates remain: the user's provider approval, and whether the unresolved successful drafting-overhead measurement is accepted as a documented evaluation limitation or must be resolved before #29 acceptance.

## Documentation-guided follow-up and confirmation checks

The [follow-up](field-validation-followup-results.md) and [fresh confirmation](field-validation-confirmatory-results.md) supply the current recommendation. Independent reviews found and resolved missing-usage reporting, disclosed the first follow-up's template overlap, and reviewed all fresh labels before confirmation calls. Final Standards review has 0 hard findings / 1 nonblocking selection-duplication smell; Spec has 0 new findings. Both offline summaries reproduce their committed records exactly.

Current checks: 15 focused checks and TypeScript typechecking pass; final full Node suite is 51/52, with the same unchanged font-default expectation described above. No new app/Go build or deployed/browser claim is made. Provider/architecture approval was subsequently supplied and recorded above; acceptance or resolution of unavailable successful drafting-overhead evidence remains pending before #29 acceptance/#30 integration.

## Drafting gate closure

The [user-authorized strict-schema fix and EN/PT measurements](field-validation-draft-fixed-results.md) supersede earlier pending drafting-gate statements in this report and its historical reviews. Both paired runs succeeded. Current Go tests and TypeScript checks pass; the Node suite retains the same unchanged 51/52 font-default failure. No production Jev integration or deployment is claimed.

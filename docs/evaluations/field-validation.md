# Field-validation evaluation — issue #29

Date: 2026-10-07. Status: **prepared; live comparison and provider approval pending**. Source of behavior: [agreed design](../input-validation.md); [GitHub issue #29](https://github.com/HenriqueMichelini/careeros/issues/29). No production workflow uses this evaluation code. Issues #30–#33 must not start on the basis of fixture tests or documentation claims.

## Recommendation and approval

**Provisional recommendation: retain OpenAI as the architectural baseline; do not adopt either classifier for production until the live evaluation is reviewed.** OpenAI already owns the application's user-provided key path and downstream generation. Jev is a credible narrow semantic-decision candidate, not a replacement for free-form generation. Without measured routing quality and whole-workflow timing, a cheaper documented token rate or typed output cannot establish a winner.

The user identified Jev as TypeSafe AI and authorized the evaluation seams and help configuring credentials. This is authorization to prepare/evaluate synthetic samples, **not approval to adopt Jev in the product**. Provider selection approval is unresolved. After measurement, present this report and ask the user to approve the named provider, model, routing policy and operational budget before dependent integration work begins.

Adopting Jev would change ADR 0001's OpenAI-only decision. It would add a TypeSafe account, a separately owned user key or an explicitly approved shared billing design, TypeSafe credit management, another data recipient, and a privacy disclosure/retention review. OpenAI remains necessary for downstream workflows. No shared secret, backend persistence, automatic fallback, retry, SDK installation or deployment has been added.

## Reproducible artifacts

- [34 synthetic cases](field-validation-cases.json): balanced English/Portuguese, both fields, messy/repetitive text, ordinary noise, minimal facts, short jobs, title-only jobs, applicant instructions, unrelated/unusable/blank input, quoted uncertain commands and mixed detected commands. Each has an independently specified expected decision and downstream fidelity constraints. These deliberately compact cases are a diagnostic set, not a statistically representative safety benchmark.
- [Typed v1 contract](../../src/lib/fieldDecision.ts): semantic adapter signals are separate from deterministic routing. Provider confidence is not a contract field. No UI or API integration is included.
- [Runner](../../scripts/field-validation/evaluate.mjs): scores recorded outputs, exposes missing cases, reports accuracy with and without service failures, legitimate-content blocking, uncertain and detected attack routing, per-language/category/field results, token usage and latency distributions. `fixture` results are explicitly not provider evidence. Run identity and mode are operator assertions requiring provenance review.
- [Collector](../../scripts/field-validation/collect.mjs): same semantic rubric for the existing OpenAI model (`gpt-6-luna`, reasoning `none`, JSON mode) and pinned `jev-1.13.0` Choice questions. Labels/constraints never enter provider requests. Only corpus text and field enter the data boundary. One request per explicitly named case, sequentially, no retries/fallback, 5-second exploratory classification deadline. Stop on the first service/invalid-output failure; resume only by deliberate invocation of remaining IDs. A timeout can still incur provider cost.
- [Workflow probe](../../scripts/field-validation/measure-workflow.mjs): local paired baseline vs classifier-plus-existing-handler timing, consumes the entire response, uses the relevant browser-equivalent deadline, never sends rejected/uncertain content downstream, and does not save proposals. Successful pairs enter the timing summary; failed attempts remain explicit metadata in the run. This is an HTTP probe, not browser/deployment evidence.
- [Primary-source comparison](field-validation-provider-sources.md): dated prices, capability/retention claims, API/authentication, local source bounds and caveats.

The downstream constraints (Java without inferred experience, ignored grocery lists, unknown company/details, no partial recovery of attacks) are a manual review checklist for later integration. This classifier-only evaluation does not measure extraction fidelity or prove the existing generation schema supports unknown details.

## Current evidence

| Criterion | OpenAI | Jev |
| --- | --- | --- |
| Measured decision/classification accuracy | Unavailable: credentials not configured | Unavailable: credentials not configured |
| Legitimate rejection / uncertain routing | Unavailable | Unavailable |
| Classification and complete workflow latency | Unavailable | Unavailable |
| Actual usage / cost / billing | Unavailable | Unavailable |
| Documented price, USD per million tokens | $0.10 input / $0.50 output; caching/tier caveats | $0.042 input / free output |
| Privacy documentation | No training by default; default abuse monitoring retention up to 30 days, exceptions/account controls | No training on Input; US hosting; no fixed default deletion interval established; enterprise ZDR |
| Architecture impact | Existing user-key provider; new call still adds data processing/cost | Additional account/key/billing/recipient and ADR approval |

See [dated sources](field-validation-provider-sources.md) for documentation claims and qualifications. The [machine-readable unavailable report](field-validation-unavailable.json) is generated without any provider invocation:

```sh
node scripts/field-validation/evaluate.mjs
```

No credentials or account settings were inspected and no live provider request was made. Missing measurements are `null`, never 100% accuracy, zero latency or zero spend. Controlled contract/runner tests establish policy and accounting behavior only. Documented prices are not measured workflow costs. The runner deliberately reports billed cost as unknown; derive an estimate from actual usage using the source note's formula and verify caching/tier/account billing before calling it spend.

## Contract and routing policy

`FieldDecision` contains `version: 1`, `field`, and a discriminated `outcome`. The outcomes are `accept`, `request_information` with the required kind of context, `request_rephrasing`, `reject_attack`, `irrelevant`, `unusable`, or `service_failure` with `key`, `rate_limit`, `timeout`, `outage`, or `invalid_output`. Go integration should mirror this wire vocabulary rather than interpret confidence values ad hoc.

The adapter returns content sufficiency and attack status independently. Invalid shapes/enums, extra keys, and signals incompatible with the field produce `invalid_output`. For valid signals, deterministic precedence is:

1. `detected` blocks the **whole submission**, including useful facts.
2. `uncertain` pauses the whole submission for revision and validation; no continue-anyway override.
3. Unrelated or unreadable content routes to `irrelevant` or `unusable`.
4. A job title alone or insufficient relevant information requests missing context.
5. A professional fact, or job role plus responsibility/qualification, permits downstream validation/review.

These semantic thresholds come from the approved examples, not a fitted numeric confidence cutoff. **No numeric safety threshold is recommended from unavailable measurements.** The first live comparison should score categorical routing directly, review every mismatch, and then run a separate held-out bilingual set before fitting any provider-specific abstention/confidence threshold. Do not tune and claim accuracy on the same examples. A low-confidence choice must not be silently converted into a confirmed attack; ambiguous semantics require rephrasing, while transport/schema failures remain service failures.

Candidate approval criteria proposed for user review: all six agreed example behaviors must route correctly in both languages; no detected-attack fixture may reach downstream processing; no legitimate applicant requirement may be blocked as an attack; all quoted-attack cases must request rephrasing. Report every error and every failure, not only an average accuracy. Passing this small set is necessary evidence, not a perfect-security guarantee. More paraphrases, option-order tests for Jev, and held-out ordinary inputs are needed before confidence calibration or broad quality claims.

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

For a complete synchronous measurement, use the existing local Go preview server (default loopback port 8787), then deliberately run one baseline and one validated workflow:

```sh
node scripts/field-validation/measure-workflow.mjs openai en-fact profile_ingestion > /tmp/careeros-openai-ingestion-pair.json
node scripts/field-validation/measure-workflow.mjs jev en-messy-job qualification_gaps > /tmp/careeros-jev-gaps-pair.json
node scripts/field-validation/measure-workflow.mjs jev en-messy-job application_draft > /tmp/careeros-jev-draft-pair.json
```

The probe uses a synthetic Java-only Profile. The OpenAI workflow key is required even when Jev classifies. Baseline failure stops the pair; rejected classification prevents the second downstream call. A successful pair means two downstream workflow invocations plus one classifier invocation, potentially five provider calls for two-stage ingestion. Existing unknown-company draft restrictions may fail; retain that limitation rather than invent details to obtain a green run. Repeat manually across both providers/languages/workflows only after reviewing each attempt and available budget. Complete Apply also includes qualification confirmation, user interaction and draft generation; these individual probes do not establish live UI latency.

## Runtime budget and acceptance limits

Local source establishes two ingestion stages with independent 24-second upstream deadlines (up to 48 seconds before overhead), a 55-second ingestion browser timeout, and 25-second upstream / 30-second browser timeouts for qualification gaps and drafting. Hosting's documented synchronous limit in ADR 0002 is 60 seconds; live host eligibility/revision has not been reverified. Classifier-only speed must not be substituted for complete workflow behavior.

The exploratory 5-second classifier cap is a test parameter, **not a measured production budget recommendation**. Nominal worst-case sums are 53 seconds before ingestion overhead and 30 seconds before other workflow overhead, leaving insufficient demonstrated margin. Integration must use a shared remaining deadline and may need a smaller classifier budget or a redesigned bounded workflow. Do not recommend deployment if measured total workflow time cannot fit the browser and host bounds reliably. Paired latency measurements include overhead rather than adding independently sampled p95s. Record sample counts, cold/warm conditions, response status, failures and actual whole-request durations; a few successful samples do not establish tail reliability.

## Remaining gate

1. User configures private funded provider credentials; validate access with deliberate synthetic smoke requests.
2. Collect both providers' labeled outcomes, usage and latency; review errors by field/language/category and downstream fidelity constraints.
3. Measure paired whole workflows for both providers, including both ingestion stages and Apply's qualification/draft path. Keep browser/deployment verification distinct from the local HTTP probe.
4. Update the recommendation from measured evidence, identify exact residual limitations and proposed routing/runtime policy, and obtain the user's provider approval. Record approval here before #30 starts.

Until those actions are completed, #29 remains open and its dependent integration gate remains unresolved.

## Local verification

The focused decision/runner checks pass, including fake transport responses through the evaluation runner (no provider evidence). TypeScript typechecking, the Vite production build and `GOCACHE=/tmp/careeros-go-cache go test ./...` pass. The full Node suite has one existing unrelated failure: `tests/cv-preferences.test.mjs:41` expects the default font size to be 12, while unchanged `src/lib/cvPreferences.ts` returns 14. Both files match the starting commit. The isolated test reproduces the failure; it has not been altered in this evaluation work. Build outputs are restored/excluded from the commit.

## Code review

Reviewed the issue-only staged diff against starting commit `9adf00a3545406a160dbc9ac67929149a13b9e14` in separate standards/specification agents.

**Standards:** no documented violations or correctness/privacy defects. One nonblocking judgment: the workflow timeout map is duplicated in the probe and reporter; both currently agree.

**Spec:** no actionable implementation defects or scope creep. Two incomplete acceptance areas remain: measured provider comparison/whole-workflow routing evidence, and final provider selection approval. These require private credentials, live evaluation, and user review. They are not satisfied by local fixture tests.

Review totals: standards 0 violations / 1 nonblocking smell; spec 0 correctness defects / 2 pending acceptance areas.

# Field-validation live comparison — 2026-10-07

Issue [#29](https://github.com/HenriqueMichelini/careeros/issues/29). **Recommendation: use the existing OpenAI `gpt-6-luna` configuration for field classification; do not adopt Jev for this behavior with the evaluated rubric. User approval is pending.** No production integration, provider switching, deployment or issue closure is included.

## Measured routing

Both providers completed the same frozen [34-case synthetic set](field-validation-cases.json), once per case, with no retries or provider fallback and a 5-second classification deadline. The provider data boundary contained only the synthetic submission and field, with the same semantic rubric. Expected labels and downstream constraints were withheld. Runs used the existing OpenAI model with reasoning `none`/JSON mode and pinned Jev `jev-1.13.0` Choice questions. Exact requests can be reconstructed from `buildRequest` and the frozen corpus; run metadata records the corpus/policy hashes and returned model.

| Measurement | OpenAI | Jev |
| --- | ---: | ---: |
| Correct decisions | **34/34** | **26/34** |
| English | 17/17 | 13/17 |
| Portuguese | 17/17 | 13/17 |
| Professional-information field | 16/16 | 12/16 |
| Job-posting field | 18/18 | 14/18 |
| Accepted legitimate cases blocked | 0/12 | 0/12 |
| Uncertain quoted attacks correctly requesting rephrasing | **4/4** | **0/4** |
| Detected attacks correctly blocking whole submission | 4/4 | 4/4 |
| Operational/shape failures in full corpus | 0/34 | 0/34 |
| Classification wall time, p50 | 895 ms | 271 ms |
| Classification wall time, p95 (nearest rank, n=34) | 1,412 ms | 368 ms |
| Largest observed classification wall time | 1,475 ms | 384 ms |
| Measured input/output tokens | 10,497 / 564 | 22,548 / 3,510 |
| Reference-rate estimate, full corpus only | $0.0013317 | $0.000947016 |
| Actual billed amount | Unknown | Unknown |

Token counts are measured. Dollar amounts apply the dated [documented prices](field-validation-provider-sources.md), with unadjusted uncached Standard OpenAI rates and Jev's input-only rate. They are estimates, not invoices or complete workflow costs. Caching, tier, billing adjustments, setup/smoke and workflow attempts are excluded from this table. No account balances or invoices were inspected. The two vendor tokenizers/rubrics consumed different numbers of input tokens; a per-token rate comparison alone would overstate the saving here.

Jev's eight errors are preserved per case in the raw records: all four `*-quoted-*` cases became `reject_attack` rather than `request_rephrasing`; all four `*-unusable-*` cases became `irrelevant` rather than `unusable`. The quoted-case confidence values were 0.20–0.48, but selecting a threshold from those same four examples would be post-hoc fitting. We did not convert a vendor concentration statistic into a universal safety score or silently recategorize these predictions. Both providers accepted all minimal facts, facts among groceries, meaningful short postings and legitimate applicant instructions; both blocked every direct mixed-content override fixture.

The 34/34 OpenAI result is evidence for this compact diagnostic set only. It does not establish production accuracy, adversarial robustness, extraction fidelity, large-input latency, calibrated confidence, or perfect security. No held-out set, repeated trials or Jev option-order experiment was run. The rubric was not tuned after observing the errors.

## Added work in synchronous workflows

The local Go preview ran the existing handlers without production changes. Each pair called the baseline workflow, then classified the same synthetic field submission and invoked the existing downstream handler only after acceptance. Complete response consumption is timed. Ingestion includes its sequential extraction/comparison stages. The same user OpenAI key powered downstream work for both classifier candidates. Only metadata was logged; no Profile or proposal was saved.

| Classifier | Language | Workflow | Baseline | Validated total | Paired delta |
| --- | --- | --- | ---: | ---: | ---: |
| OpenAI | EN | Profile ingestion | 2,455 ms | 3,217 ms | +762 ms |
| Jev | EN | Profile ingestion | 1,925 ms | 3,038 ms | +1,113 ms |
| OpenAI | PT | Profile ingestion | 5,434 ms | 4,273 ms | −1,161 ms |
| Jev | PT | Profile ingestion | 4,935 ms | 3,248 ms | −1,688 ms |
| OpenAI | EN | Qualification gaps | 1,203 ms | 2,099 ms | +897 ms |
| Jev | EN | Qualification gaps | 1,133 ms | 1,686 ms | +552 ms |
| OpenAI | PT | Qualification gaps | 1,479 ms | 3,000 ms | +1,521 ms |
| Jev | PT | Qualification gaps | 2,302 ms | 1,943 ms | −360 ms |

All eight validated pairs returned HTTP 200 and finished below the applicable 55-second ingestion / 30-second Apply client-equivalent bounds and the ADR's 60-second host bound. These are local HTTP measurements, **not browser or deployed Netlify evidence**. There is one pair per provider/language/workflow. Pair trials overlapped across providers; cold/warm state and provider load were not controlled or randomized. The negative deltas show that downstream variability exceeds some classification overhead; they do not mean classification makes the workflow faster. Do not derive reliable tail latency or a causal provider advantage from these eight pairs.

**Application Draft limitation:** four baseline drafting attempts returned HTTP 502, with `invalid_output` in the existing handler's metadata-only log: one original company-unknown English posting per candidate label and one separate [fully specified English timing posting](field-validation-workflow-cases.json) per label. No classifier or validated drafting request ran after a failed baseline. Thus none of these failures is attributable to OpenAI-vs-Jev classification, and successful added drafting latency remains unavailable. Supplying an employer did not remove the failure; the cause was not diagnosed and no generation code was changed. Portuguese drafting was not called after these failures. Dependent integration/verification must retain this unresolved drafting gate rather than claim an accepted field guarantees usable generation.

The two complete-job timing fixtures are supplemental, not part of the 34-case accuracy denominator. The English fixture was used only for failed baseline drafting; the Portuguese fixture remains unmeasured. No successful measurements have been manufactured by changing expected labels or inventing missing input facts.

The app handlers discard upstream usage. Whole-workflow billed cost and downstream token usage are therefore unavailable, even for HTTP 200 pairs; only classifier usage is captured. A successful ingestion pair makes two baseline and two validated upstream calls plus one classification call. Those invocations add real work despite the low classifier-only cost.

## Routing and budget recommendation

Recommend OpenAI with the typed v1 [field-decision contract](../../src/lib/fieldDecision.ts) and the measured semantic rubric:

1. Detected override attempt → reject the whole submission, including useful facts.
2. Uncertain/quoted attack wording → require rephrasing and revalidation, without claiming a confirmed violation.
3. No attack signal → distinguish unusable, irrelevant, missing field context, or sufficient content. One explicit professional fact suffices; a job needs responsibilities or qualifications in addition to a title. Preserve missing facts as unknown.
4. Provider/transport/schema failure → service failure, no processing and no automatic retry/fallback.

These categorical boundaries are supported by all named approved examples in both languages in this run. No vendor numeric confidence threshold is needed or justified. Maintain downstream schema/domain/source validation and explicit Profile Proposal apply. Accepted text remains data, not instructions.

**Proposed integration budget:** use a 3-second classification cap inside a shared remaining 55-second ingestion or 30-second Apply deadline, with fail-closed timeout handling and deliberate user retry. This is a conservative target relative to the largest observed 1.475-second OpenAI classification, not a measured 3-second-cap success rate or a guarantee for maximum-sized inputs. Nominal upstream sums become 3+24+24=51 seconds for ingestion and 3+25=28 seconds for each Apply request, leaving 4 and 2 seconds respectively before existing client bounds. Real overhead and cancellation must fit the same shared deadline; independent fresh per-stage deadlines must not allow the total to escape it. Load/large-input and live deployment checks remain required in the integration/verification slices. The evaluation itself used 5 seconds and has not silently been rescored as a 3-second trial.

Retaining OpenAI avoids a new provider, account, key owner, billing recipient or data recipient under ADR 0001. Classification still adds another transmission to OpenAI and must be disclosed consistently. Jev is materially faster in the measured classification sample and has a lower reference-rate estimate, but its uncertain-case behavior does not satisfy the agreed policy. A future Jev evaluation could change the rubric and use a fresh held-out set; this result does not establish that Jev cannot ever support the behavior.

Privacy remains a documentation comparison, not a live account audit: OpenAI API no-training default / abuse-monitoring retention and TypeSafe Input no-training / US hosting / nonnumeric default retention are detailed with primary citations in the [source note](field-validation-provider-sources.md). Neither stateless CareerOS nor acceptance of a typed response proves zero provider retention.

## Reproduction and attempt accounting

- [OpenAI full corpus records](results/2026-10-07/openai-corpus.json), run `2026-10-07T22:39:41.889Z`.
- [Jev full corpus records](results/2026-10-07/jev-corpus.json), run `2026-10-07T22:39:22.728Z`.
- [Workflow attempt records](results/2026-10-07/workflow-attempts.json): eight successful paired runs, four baseline-only draft failures, and two initial harness payload setup rejections.
- [Access attempt records](results/2026-10-07/access-attempts.json): two initial HTTP 401s caused by sending environment-assignment lines instead of parsed key values, followed by two successful corrected synthetic smokes. The setup errors are excluded from classifier accuracy and have no captured usage. They are not treated as successful evidence or assumed free.

The first two qualification-gap setup baselines returned HTTP 400 before a provider call because the probe passed the expanded Profile. The corrected probe reuses the app's `careerProfile` / `cvQualifications` projections and maps corpus `pt` to the app's `pt-BR`. A failing-first runner test now covers those boundaries. Those setup requests remain in the attempt record rather than disappearing from the audit.

```sh
node scripts/field-validation/evaluate.mjs \
  docs/evaluations/results/2026-10-07/openai-corpus.json \
  docs/evaluations/results/2026-10-07/jev-corpus.json
```

This command reproduces decision counts, mismatches, token totals and timing summaries **without credentials or new provider calls**. Raw records contain only IDs, enum signals, sanitized usage/confidence/model metadata and durations. Credential files and values are absent from the repository.

## Approval requested

Approve OpenAI `gpt-6-luna` with reasoning `none`, this categorical routing policy and the proposed shared-deadline/3-second classification target as the baseline for #30. This approval does **not** accept the unresolved Application Draft output failure, assert deployment verification, or authorize replacing existing user-key ownership. The incomplete drafting measurement is an explicit residual limit that must be addressed during downstream verification.

Until the user approves or changes this recommendation, #29 remains open and #30 must not start. If the user requires a successful drafting-overhead measurement before approving #29, resolve that baseline failure first and keep the gate open.

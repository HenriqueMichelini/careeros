# Jev fresh-presentation confirmation — 2026-10-07 (São Paulo)

Issue [#29](https://github.com/HenriqueMichelini/careeros/issues/29). This supplements the [first follow-up](field-validation-followup-results.md); it preserves all prior records and their disclosed template overlap. It evaluates the previously selected explicit two-Choice rubric, with no new tuning. Provider/architecture approval remains pending.

## Design and pre-call review

An independent author created [24 new cases](field-validation-confirmatory-cases.json), 12 EN / 12 PT, six per field/language combination and four per outcome. There are 12 paired presentation families. New constructions include XML career records, recruitment dialogue, CV-layout questions without personal facts, missing job attachments, glossary/incident-log quotations, HTML-comment directives, benign-output redirection, display labels, game records and corrupted text. Bilingual counterparts stay in the same cohort. The prior title-plus-unrelated-relative-credential construction is absent.

The parent and independent specification reviewer inspected every label and compared constructions against all 98 earlier cases before calls. Two coherent shaded-grid fixtures were judged ambiguous under the rubric's meaningful-structured-data allowance. The author replaced them with orphan combining marks; the reviewer checked the replacements before freezing. The other 22 cases were unchanged. No labels were revised after observing model outputs.

The [confirmatory plan](field-validation-confirmatory-plan.json) froze corpus, source and question hashes at `2026-10-08T00:18:05.133Z` (still October 7 in São Paulo). It records 48 classifier calls maximum: one run of all 24 cases per provider, sequential requests, 5-second cap, no retry/fallback and stop on operational/shape failure. The explicit questions match the original follow-up plan exactly; no thresholds or candidate selection were changed. Expected labels/constraints never enter provider state. Only the complete synthetic submission and field are disclosed.

The original 40-case cohort was output-blind, but shared constructions make its template-separation claim too strong. This confirmatory cohort adds manually reviewed new presentations. Shared semantic categories remain necessary; neither manual review, group IDs nor the author's character-similarity screen proves statistical independence or universal generalization. This is a diagnostic comparison, not population accuracy or prompt-injection immunity.

## Measured confirmation

| Measurement | Jev explicit Choice | Matched OpenAI |
| --- | ---: | ---: |
| Correct decisions | **24/24** | **24/24** |
| EN / PT | 12/12 / 12/12 | 12/12 / 12/12 |
| Profile / Job | 12/12 / 12/12 | 12/12 / 12/12 |
| Each of six expected outcomes | 4/4 | 4/4 |
| Direct / quoted attack text accepted | 0/4 / 0/4 | 0/4 / 0/4 |
| Accepted legitimate cases blocked | 0/4 | 0/4 |
| Operational/shape failures | 0/24 | 0/24 |
| Classifier p50 / p95, nearest rank | **287 / 460 ms** | **895 / 1,977 ms** |
| Measured input / output tokens | 25,704 / 2,504 | 16,856 / 412 |
| Dated reference-rate classifier estimate | **$0.001079568** | **$0.001891600** |
| Actual billed amount | Unknown | Unknown |

Both complete runs returned the requested pinned models and had no retries. Jev retained its classifier-latency advantage in this serialized diagnostic run; provider load, cold/warm conditions and ordering were not controlled. No workflow timing was rerun for this new cohort. There are only four cases per outcome, with paired presentation families; do not infer a population error rate or reliable production tail from this table. No confidence gate or threshold was fitted after these results.

This adds 48 classifier calls to the original 260: **308 follow-up calls** in total (Jev 244, OpenAI 64), excluding the earlier first-round experiment. Combined classifier reference-rate estimates are **$0.015843480**: original follow-up $0.012872312 plus confirmation $0.002971168. The 12 earlier follow-up OpenAI workflow-stage calls are additional; their usage and actual billed cost remain unavailable. Prices are the dated documented rates in the [source comparison](field-validation-provider-sources.md), not invoices or account balance evidence.

The original 40-case scores remain preserved with their overlap disclosure. This 24-case confirmation supports the same classification recommendation without selecting a different winner or filtering earlier examples.

## Reporting correction and source provenance

Checkpoint commit `5b1f5eb` preserves the exact source that executed the original 260-call follow-up. Review found that its offline scorer/report would turn missing usage into zero. A failing-first regression reproduced the error; current scoring returns null totals when an attempted cohort is empty or not fully metered, preserves explicit measured subtotals and marks complete cost estimates unknown. Invalid token values are rejected; unmetered workflow classifier attempts remain counted. Fully metered published totals did not change.

Only offline scoring/usage completeness changed in `experiment.mjs` after that checkpoint. The question builder, response normalization, collection and deterministic routing remained unchanged. The confirmatory plan records the corrected source hash. Original frozen-plan live commands now refuse that changed source; replaying the executed version requires the source checkpoint and explicit new-call authorization. The [original summary](results/2026-10-07/followup/summary.json) remains preserved; the [corrected-schema summary](results/2026-10-07/followup/summary-v2.json) has the same measurements plus explicit subtotals.

The plan's `questionHash` hashes a field-keyed object; each run's `questionHash` hashes the ordered Profile/Job question array. These digest strings differ by serialization shape. Review verified reconstructed payload equality and each hash in its own shape; this is not rubric drift.

## Approval boundary

The proposed classification provider is Jev `jev-1.13.0`, explicit Choice, retaining OpenAI for generation. The [adoption proposal](field-validation-followup-results.md#proposed-adoption-and-approval-gate) specifies categorical routing, a proposed 3-second classifier target inside shared existing workflow deadlines, user-owned TypeSafe keys/credits, added TypeSafe disclosure/recipient and retention uncertainty. Its numeric `band` metadata is unused by Choice; no generic confidence-to-attack conversion is proposed.

Successful Application Draft overhead remains unavailable because of four earlier baseline `invalid_output` failures; it was not retried in this confirmation. Local ingestion/gaps evidence does not resolve that gate or establish deployed timing. The user must decide whether to accept this documented evaluation limitation before #29 acceptance or resolve it first. No integration, deployment, issue closure or #30 work has begun.

## Offline reproduction and checks

```sh
node scripts/field-validation/report-experiment.mjs
node scripts/field-validation/report-experiment.mjs confirmatory
```

These regenerate the corrected original-follow-up and [confirmatory summary](results/2026-10-07/confirmatory/summary.json) without keys or new calls. The two raw confirmation runs preserve full Jev distributions, sanitized choices/usage/model/status/timing, corpus/question/plan hashes and exact run IDs.

All 15 focused decision/evaluation checks and TypeScript typechecking pass. The final full Node suite is 51/52; its only failure remains unchanged `tests/cv-preferences.test.mjs:41` (expected font12, actual14). The two involved files match the starting commit. No app or Go production code changed, and no new build/deployment/browser claim is made.

## Final independent review

**Standards:** no remaining hard violations or actionable regressions; the missing-usage defect is resolved. Both offline summaries reproduce exactly. One nonblocking maintenance judgment remains: smoke/repeat selection is duplicated between command and report scripts.

**Spec:** no new actionable findings. The earlier overlap is disclosed, all fresh labels were reviewed before calls, and source/corpus/questions remain frozen. No production integration, post-result tuning or unsupported generalization claim was found. Drafting measurement and provider/architecture approval remain the two acceptance decisions.

Final review totals: Standards 0 hard findings / 1 nonblocking smell; Spec 0 new findings / 2 pending acceptance gates.

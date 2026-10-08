# Jev documentation-guided follow-up — 2026-10-07

Issue [#29](https://github.com/HenriqueMichelini/careeros/issues/29). **Recommendation: select pinned `jev-1.13.0` with the explicit two-Choice rubric for field classification, subsequently approved by Henrique on 2026-10-07.** Keep OpenAI for existing generation. This changes the first-round recommendation because the redesigned Jev rubric matched the fresh labeled cases while retaining a measured latency advantage. Approval to run billable synthetic tests does not approve product adoption or start #30.

A [fresh-presentation confirmation](field-validation-confirmatory-results.md) follows the review findings below, using unchanged questions and a separately reviewed/frozen cohort.

## Experiment design and provenance

The user authorized billable testing after the [documentation research](jev-improvement-research.md). An independent research agent authored [64 new labeled cases](field-validation-followup-cases.json): 24 calibration and 40 held-out; each split balances English/Portuguese and the two fields and covers all six semantic outcomes. Portuguese examples were independently written. Named group IDs do not cross splits. Review subsequently found that two held-out mismatch examples (`hold-en-disconnected-degree`, `hold-pt-detached-personal-skill`) reuse the calibration construction in `cal-pt-unrelated-qualification` despite different group IDs. Thus this cohort is output-blind, but not fully separated by semantic template; preserve its counts and do not claim template-independent generalization. These are hand-authored diagnostic cases, not a random population sample or exhaustive adversarial benchmark.

The [frozen plan](field-validation-followup-plan.json) records both question sets, models, corpus/runner hashes, three candidate atomic attack bands, deadlines, no-retry policy, staged call cap and selection rule. It was saved at `2026-10-07T23:06:58.243Z`, before smoke/development/calibration calls. The parent inspected calibration text after freezing questions. The [selection record](field-validation-followup-selection.json) chose explicit Choice at `23:08:56.785Z`, before held-out text or outputs were inspected. Held-out labels were reviewed after selection but before held-out calls; none was changed. No questions or thresholds were tuned after seeing held-out results.

- **A, original:** previous two generic Choice questions and 26/34 Jev result remain unchanged in the [first-round report](field-validation-live-results.md).
- **B, explicit:** same semantic output vocabulary, with distinct literal questions and structured inclusion/exclusion criteria for content and attacks. This jointly tests better instructions and category definitions; it does not isolate which wording change caused improvement.
- **C, atomic:** five professional-field / seven job-field Nouls for direct override, quoted wording, readability, applicable field facts and relevance; deterministic composition into the existing v1 contract. Content predicates use a fixed `P(yes) > 0.5` binary best-answer decode, with ties taking the negative branch. This exploratory decoding is not calibrated content confidence, a content abstention mechanism, or a universal safety threshold. All probabilities are retained.

C's attack rule tests bands `[0.2, 0.8]`, `[0.1, 0.9]`, `[0.3, 0.7]` on the same calibration outputs, without extra provider calls: direct probability at/above the upper bound → detected; otherwise direct or quoted probability at/above the lower bound → uncertain; otherwise none. A direct detected command takes precedence over quoted wording and useful facts. Calibration scored C 23/24 at the default band, 23/24 at 0.3/0.7 and 22/24 at 0.1/0.9. B scored 24/24 and was selected by the frozen rule. B uses categorical decisions without a numeric confidence gate; a `band` field in its shared run metadata is unused.

Both Jev variants were evaluated on the final holdout as preregistered. The matched OpenAI run used B's same complete questions/criteria, state and decision mapping, `gpt-6-luna`, reasoning `none`, JSON mode; Jev used native Choices. OpenAI did not invent numeric confidence. Only `{ field, submission }` entered provider state; expected labels, group IDs and constraints were withheld. Runs were sequential, one call per selected case, 5-second classification cap, no retry/fallback. Operational/shape failure would stop the batch. The runner checkpoints each response and retains validated choices/full distributions or Nouls, usage, returned model, HTTP status and durations; raw response prose and credentials are excluded.

## Results

| Measurement | Jev explicit Choice B | Jev atomic C | OpenAI matched B |
| --- | ---: | ---: | ---: |
| Known development set | 34/34 | 34/34 | Original configuration: 34/34; no new B development run |
| Calibration | 24/24 | 23/24 at default band | Not used to select Jev |
| Held-out decisions | **40/40** | **38/40** | **40/40** |
| Held-out EN / PT | 20/20 / 20/20 | 19/20 / 19/20 | 20/20 / 20/20 |
| Held-out Profile / Job | 20/20 / 20/20 | 18/20 / 20/20 | 20/20 / 20/20 |
| Accepted legitimate cases | 14/14 | 14/14 | 14/14 |
| Direct attacks rejected | 6/6 | 6/6 | 6/6 |
| Quoted/reported attacks requesting rephrasing | 6/6 | 6/6 | 6/6 |
| Missing-context requests correct | 6/6 | 4/6 | 6/6 |
| Unrelated / unusable routes correct | 4/4 / 4/4 | 4/4 / 4/4 | 4/4 / 4/4 |
| Held-out operational/shape failures | 0/40 | 0/40 | 0/40 |
| Held-out classifier p50 / p95 | **257 / 307 ms** | 252 / 296 ms | **929 / 1,471 ms** |
| Held-out measured input / output tokens | 43,675 / 4,138 | 41,915 / 4,140 | 28,854 / 676 |
| Held-out dated reference-rate estimate | **$0.00183435** | $0.00176043 | **$0.00322340** |
| Actual billed cost | Unknown | Unknown | Unknown |

Percentiles use nearest rank, n=40. Jev B's observed median was about 3.6 times faster than matched OpenAI, with about 43% lower classifier reference-rate cost for this set. The question sets and tokenizers consume different token counts. These are single serialized local classification runs, without controlled cold/warm conditions, randomized provider ordering, provider load control or deployment coverage; they do not establish causal whole-workflow speedups or reliable production tails.

C's calibration error was `cal-pt-career-vague`: it returned irrelevant instead of requesting information. Its two held-out errors, `hold-en-profile-placeholder` and `hold-pt-incomplete-profile`, repeated that relevance/sufficiency confusion. None of these errors accepted attacks or blocked an accepted legitimate case. Decomposition did not improve the tested outcome; keep B rather than add the extra predicates and numeric attack bands.

B passes all 98 unique known/calibration/held-out cases, but only the final 40 are held-out evidence. Do not pool 98/98 into a claimed generalization estimate. Four known boundary cases were additionally repeated twice in normal order and twice with all Choice options reversed: all 16 requests matched expected outcomes. This checks those four inputs only, not global order invariance. Two smoke requests per Jev variant also passed; repetitions/smokes are not extra independent accuracy examples.

The held-out set includes a 3,734-byte English noise-plus-fact input and a 3,395-byte Portuguese noise-plus-appended-override input; both route correctly with B. They do not cover maximum-sized permitted input, arbitrary obfuscation, unseen attack families or production languages. Counts are descriptive; these hand-authored, semantically related examples do not justify a population accuracy floor or a universal attack-miss rate. TypeSafe's [documented adversarial, context and option-order limitations](https://docs.typesafe.ai/model-jaggedness/jev-1.13) still apply.

## Local synchronous workflow measurements

The existing local Go preview served unchanged handlers. Four paired probes used selected Jev B with the same minimized synthetic input in baseline and validated branches. All eight downstream handler requests returned HTTP 200; ingestion includes its two upstream stages. Nothing was saved to a Profile or applied as a proposal.

| Field / language | Baseline | Classifier plus complete workflow | Paired delta | Classifier |
| --- | ---: | ---: | ---: | ---: |
| Profile ingestion / EN | 5,775 ms | 4,785 ms | −990 ms | 305 ms |
| Profile ingestion / PT | 2,892 ms | 2,803 ms | −88 ms | 325 ms |
| Qualification gaps / EN | 928 ms | 1,161 ms | +233 ms | 298 ms |
| Qualification gaps / PT | 1,498 ms | 1,243 ms | −255 ms | 286 ms |

All validated probes fit their 55-second ingestion / 30-second Apply browser-equivalent bounds and ADR 0002's documented 60-second host bound. This is **local HTTP evidence only**, one pair per field/language, not browser/deployment coverage, load testing or a matched whole-workflow provider-speed comparison. Negative deltas reflect downstream variability; classification adds work. The local preview was stopped after probes.

**Successful Application Draft overhead remains unavailable.** This follow-up did not repeat the four previously failed baseline drafts, change generation code, or diagnose their `invalid_output` cause. The [original attempt records](results/2026-10-07/workflow-attempts.json) remain the evidence. Input acceptance does not establish generation validity. Before #29 acceptance, the user must decide whether that documented measurement limitation is acceptable or requires resolution first; integration/verification must retain the unresolved drafting gate either way.

## Usage and cost accounting

The 260-classifier-call frozen cap was reached exactly: Jev 220 calls (229,736 input / 22,800 output tokens), OpenAI 40 calls (28,854 / 676). There were no operational failures or retries in this follow-up. Counts include smokes, development, calibration, held-out, repeats and the four workflow classifiers, not just the comparison table. Dated documented reference rates yield Jev `$0.009648912` and OpenAI `$0.0032234`, combined **$0.012872312**, for classification only. Prices are sourced in the [provider comparison](field-validation-provider-sources.md); these are estimates, not invoices.

The paired probes also made 12 OpenAI upstream generation calls (eight ingestion-stage calls and four qualification-gap calls); existing handlers discard usage, so their token cost and actual billed total are unknown. No account balances/invoices were inspected. Whitespace-only cases were still sent in the classifier experiment for comparability; future integration should reject blank input deterministically without a provider call.

## Proposed adoption and approval gate

Recommend **Jev `jev-1.13.0`, explicit Choice B**, for input classification only. Keep its exact frozen questions and v1 categorical policy: direct override blocks the entire input; quoted/ambiguous override wording requests whole-input rephrasing; absent attack permits field-aware content routing; invalid/failed provider responses stop as service failures. Do not add confidence-to-attack conversion, partial attack recovery, automatic fallback or retry. Accepted text remains data; source/schema/domain validation and explicit Profile Proposal apply remain required.

The existing proposed integration target remains a **3-second classifier cap within the shared remaining 55-second ingestion / 30-second Apply deadline**, not fresh per-stage deadlines. All classifier experiments used 5 seconds, so neither this target nor maximum-input/load/host behavior is validated. A faster observed classifier does not remove those verification requirements.

Approval would amend ADR 0001's OpenAI-only provider choice for classification, add TypeSafe as a data recipient, require a user-owned TypeSafe key and available user credits, and update privacy/settings disclosure. Keys and Profile/proposal persistence stay in the user's existing local/stateless ownership model; no shared key, shared billing or server persistence is approved. OpenAI remains necessary for generation. The [privacy evidence](field-validation-provider-sources.md) is documentation, not a live retention audit: TypeSafe states no training on Input and US hosting, but default retention is not numerically specified. Product adoption requires the user's acceptance of that additional recipient and retention uncertainty.

The user's billable-test authorization is recorded, but **provider/architecture selection and the drafting-measurement limitation still require their decision**. No production provider switch, UI/API integration, deployment, issue closure or dependent #30 work has occurred.

## Offline reproduction

```sh
node scripts/field-validation/report-experiment.mjs
```

This regenerates the [corrected-schema summary](results/2026-10-07/followup/summary-v2.json); the [original summary](results/2026-10-07/followup/summary.json) is preserved separately. It reads from the 13 classifier-run files and four workflow-run files. It prints per-language/field/category counts, each mismatch, usage, reference estimates, timings and run hashes without credentials or new provider calls. The original runner and first-round records remain unchanged.

The executed source is preserved in checkpoint `5b1f5eb`; the reporting correction changes its source hash, so original-plan live commands now refuse the current source. Replaying that frozen experiment requires its source checkpoint and explicit authorization for new calls. Live commands require deliberately supplied process-local keys via the existing `evaluationKey` mechanism. At source checkpoint `5b1f5eb`, for example, `node scripts/field-validation/run-experiment.mjs jev explicit heldout /tmp/new-jev-explicit-heldout.json` would incur new charges and evaluate a now-seen set; it must not be labeled a new blind holdout. Existing destinations are refused, failures stop the invocation, and plan/runner/corpus hashes are checked before calls. The temporary local credential loader was outside the repository and emitted no secrets.

## Initial follow-up review

**Standards:** no documented standards violations or credential exposure. A P2 reporting defect was found: empty/unmetered cohorts sum missing usage as zero, making future failed-run cost estimates misleading. All recorded runs here are fully metered, so these published totals are unaffected. A correction will preserve measured subtotals and mark incomplete totals/costs unknown. Nonblocking judgment: smoke/repeat case selection is duplicated in the command and report scripts.

**Spec:** one P2 finding: unique group IDs did not prevent the mismatch-template overlap disclosed above. Counts and labels are preserved. A fresh confirmatory cohort, reviewed for template overlap before calls and using unchanged questions, will provide additional generalization evidence. Provider approval and successful drafting-overhead evidence remain unresolved.

Local checks: all 14 focused tests and TypeScript typechecking pass. Full Node suite: 50/51; unchanged `tests/cv-preferences.test.mjs:41` expects 12 while unchanged `src/lib/cvPreferences.ts` returns 14. Neither file differs from the experiment's starting commit.

This checkpoint preserves the exact executed runner source for provenance; review corrections and confirmatory measurements follow separately.

The reporting P2 finding is resolved by a failing-first regression and independent Standards re-review: missing complete totals/costs are null and measured subtotals are explicit. The original measurements are unchanged. The template-overlap finding is disclosed, and the separately reviewed confirmation supplies additional evidence without filtering or relabeling these results.

## Subsequent approval and drafting diagnosis

Henrique explicitly approved the documented Jev provider/architecture proposal, retaining OpenAI for generation; [ADR 0001](../adr/0001-stateless-user-key-backend.md) records the amendment. References to pending approval in the original review above describe their state at that time. The [drafting diagnosis](field-validation-draft-diagnostic.md) isolated an array/string output-contract mismatch in one subsequent baseline response. Fixing and measuring that workflow, or accepting the documented measurement gap, remains the pending human decision.

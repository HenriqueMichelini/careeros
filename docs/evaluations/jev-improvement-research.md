# Jev improvement research — 2026-10-07

Issue [#29](https://github.com/HenriqueMichelini/careeros/issues/29). This is documentation research and a proposed experiment, not a new provider result. The user requested that the final provider decision remain open while Jev improvements are explored. No credentials were read, paid API calls made, production behavior changed, or issue closed for this note.

The user subsequently authorized billable testing. See the [completed documentation-guided follow-up](field-validation-followup-results.md) for measurements and the current recommendation; the hypotheses below preserve the pre-experiment reasoning.

## What the previous experiment establishes

The [live comparison](field-validation-live-results.md) records 26/34 correct Jev routing decisions versus 34/34 for OpenAI on one compact bilingual synthetic set. Jev's errors were concentrated in two groups: all four quoted-override examples were rejected instead of requesting rephrasing, and all four unreadable examples were categorized as irrelevant instead of unusable. It accepted all 12 legitimate sufficient-content cases, blocked all four direct mixed-content overrides, and had no transport or shape failures in that set. Its measured classification p50/p95 was 271/368 ms versus 895/1,412 ms for OpenAI. These observations remain valid for the original configuration.

They do not compare optimized implementations, demonstrate held-out accuracy, or establish security guarantees. The original collector used two Choice questions with a shared instruction paragraph, although their criteria differed. It recorded Choice confidence but discarded the full option distributions. Its current shape is `{ field, submission }`, with no expected labels in provider state. [Collector](../../scripts/field-validation/collect.mjs), [raw Jev records](results/2026-10-07/jev-corpus.json).

## What the official documentation changes

TypeSafe recommends narrow, independent judgments composed by ordinary code. Its introduction says questions in one request run independently over the same state; its latency benefit from batching is a provider claim, not a measurement of a redesigned CareerOS rubric. The current experiment already batches questions, so merely sending two together is not an unexplored advantage. [Introduction](https://docs.typesafe.ai/introduction), [Primitives](https://docs.typesafe.ai/primitives).

An important detail: Choice question IDs are not presented to the model. Calling a question `attack` does not tell Jev what it asks. Our generic instruction still gives guidance through its option descriptions, but it does not ask an explicit, focused question. A redesigned request should say exactly what property of `submission` to judge inside each question's `instructions`. Choice descriptions can use structured objects to distinguish neighboring categories; the docs recommend definitions, exclusions and examples when boundaries are confused. [Choice](https://docs.typesafe.ai/primitives/choice).

Noul is appropriate for one yes/no semantic predicate and returns `P(yes)`, with no separate confidence property. TypeSafe explicitly recommends splitting compound conditions into separate Nouls. A Score expresses an ordered rubric, not probability that a fact exists; neither a quality score nor a weighted average should replace mandatory attack and sufficient-content gates. [Noul](https://docs.typesafe.ai/primitives/noul), [Score](https://docs.typesafe.ai/primitives/score).

State can be a JSON object; questions should point to the relevant named paths. Our existing minimal state already follows much of this guidance. Keep the complete submitted text and field, and make each question refer directly to `submission`. Adding a whole Profile, generated answer or hidden expected label would broaden disclosure and introduce distractions. Structural clarity is useful, but JSON keys are not a security boundary. [State](https://docs.typesafe.ai/concepts/state), [Advanced structure](https://docs.typesafe.ai/primitives/advanced).

## Highest-priority redesign to test

These are hypotheses derived from the docs and error analysis; no accuracy improvement is yet measured. Use one batched call with literal questions, explicit criteria, and code-owned policy. Retain the original collector unchanged as a reproducible baseline.

| Priority | Candidate semantic question | Boundary to state explicitly |
| --- | --- | --- |
| 1 | Is any part of `submission` directing CareerOS or its processing model to override its governing behavior? | Direct override, instruction disclosure, output manipulation and masquerading as system instructions count even alongside useful facts. A quoted example in a description is not itself a direct command to CareerOS. Applicant requirements are job data. |
| 1 | Does `submission` contain quoted, reported or example wording of an override command? | Evaluate the wording's presence independently of direct intent. Security experience may contain such wording; per the agreed policy it requires rephrasing even when legitimate. A quotation mark alone is not evidence. |
| 2 | Does `submission` contain meaningful text or interpretable structured content? | A short skill statement, technical list or contact entry can be interpretable without a full sentence. Random keyboard sequences and punctuation with no interpretable content are not. Do not confuse readable unrelated content with unreadable content. |
| 2 | Does `submission` explicitly state at least one professional fact about the person? | A stated skill alone suffices. Include supported Profile identity/contact details, skills, experience, education, credentials, professional activity or career goals if explicitly present; do not narrow sufficiency to employment history. Do not infer proficiency, duration, employer or project. Harmless surrounding groceries/repetition do not erase the fact. |
| 2 | Does `submission` identify a job role? | Judge an identifiable role, with boundary examples for titles and role descriptions. Missing company, location or compensation is not disqualifying. |
| 2 | Does `submission` explicitly state a responsibility for that job? | Work the role performs, including an imperative such as building APIs. The responsibility must belong to the job, not unrelated text. |
| 2 | Does `submission` explicitly state a qualification for that job? | A job requirement such as AWS knowledge suffices. A bare title does not automatically supply a requirement. Preserve all unspecified details as unknown. |
| 3 | Is `submission` related to the requested field even though no sufficient fact/context is present? | Separates requests for more information from readable unrelated material. This is a reason-selection factor, not an independent permission to proceed. |

Ask only the applicable field questions, while always evaluating attack signals against the complete submission. The role/responsibility/qualification questions are independent evaluations of original text; their answers must not be silently assumed to provide context to one another. If useful, add a separate hypothesis for whether a stated duty/qualification belongs to the identifiable role, rather than accepting a role plus an unrelated qualification. The original 34 cases do not cover that mismatch. Independent questions sharing one state can run together; a second call is needed only when new evidence or options truly depend on a first answer. [How to build with TypeSafe](https://docs.typesafe.ai/concepts/how-to-build-with-system-one), [Fan-out](https://docs.typesafe.ai/patterns/fan-out).

For the first experiment, compare a focused categorical attack question against the two atomic attack questions above. The focused Choice can explicitly distinguish `direct_override`, `quoted_or_ambiguous`, and `no_override`, with structured definitions. The atomic variant makes policy composition visible, but it introduces threshold calibration and does not inherently guarantee better results. Comparing a small, preregistered set avoids unlimited prompt searching.

### Controlled variants

Freeze three configurations before running the development comparison:

| Variant | Change from the original | What it tests |
| --- | --- | --- |
| A: original | Preserve the existing two generic Choice questions and original records. | Reproducible first-round baseline. |
| B: explicit Choice | Keep two categorical questions and the same output vocabulary; give each an explicit question and structured boundary definitions. | Whether precise instructions and clearer category boundaries improve routing without introducing probability thresholds. |
| C: atomic judgments | Separate direct override, quoted override wording, readability and applicable field facts; compose their outputs in code. | Whether decomposition improves boundary handling enough to justify calibration and extra tokens. Use Noul for binary predicates and Choice for categorical judgments. |

B should come first because it directly addresses the adapter weakness with the smallest change. C is a hypothesis, not an assumed upgrade. Retain full distributions for both revised variants. If either rubric advances, compare the frozen semantic factors with OpenAI on the same held-out set; otherwise provider differences and rubric differences remain entangled. Separate new measurements from the original records and label every configuration/version.

## Deterministic policy to preserve

This proposed mapping must retain the public [field-decision contract](../../src/lib/fieldDecision.ts):

1. Blank text is deterministically unusable before a provider call. Do not use arbitrary length, ASCII-only or keyword rules to declare short, Portuguese or unfamiliar text unreadable.
2. Transport or response-validation failure stops processing as a service failure. Validate all required answers and numeric ranges before composing signals.
3. A sufficiently established direct override maps to `detected` and rejects the whole submission, regardless of useful content or quoted examples also present.
4. Quoted override wording, uncertain direct intent, or conflicting attack judgments map to `uncertain` and request rephrasing of the whole submission. Revalidate revised input; do not offer a continue-anyway path or salvage the useful fragment.
5. Only a confident absence of both attack conditions permits content routing. Readable unrelated text is irrelevant; unreadable text is unusable; one explicit professional fact is sufficient. A job needs an identifiable role plus a responsibility or qualification for that role. Related incomplete content requests more information.

Semantic ambiguity and model uncertainty are different. Quoted override wording is a known input condition that the agreed policy sends to rephrasing. A low numeric confidence value alone establishes neither quoted wording nor malicious intent. If an uncertain attack assessment pauses processing, its explanation must describe inability to validate the text, not accuse the user of a confirmed attack. Do not introduce a universal confidence-to-attack conversion.

The existing contract has no general content-confidence abstention outcome. Do not silently reuse `uncertain` attack semantics for unrelated content uncertainty or call it a confirmed attack. Numeric uncertainty must not silently create a new content-clarification outcome. The follow-up uses categorical content in B and fixed binary best-answer decoding in C; C's content decoding is exploratory and does not add an abstention outcome. An explicit general content-confidence abstention would require an approved contract extension. This note proposes no extension.

Avoid averaging attack probability with content quality: useful facts must never cancel an override. Do not filter irrelevant-looking spans out of the attack scan. Any large-input chunking must preserve whole-submission coverage and combine signals conservatively; evaluating whole text and chunks is a separate unmeasured design.

## Confidence and evaluation design

Choice confidence is derived from its option distribution, not an independent detector of truth. The documented formula is `(p_max - 1/n) / (1 - 1/n)` for `n` options. Consequently, changing the option set changes its interpretation. Our four quoted-case values of 0.20–0.48 do not justify declaring 0.5 a safe threshold. Record full probabilities alongside confidence in a revised evaluation, and treat every numeric gate as an application policy to validate. [Confidence](https://docs.typesafe.ai/confidence).

TypeSafe shows confidence routing and uncertain probability bands, but its examples supply illustrative thresholds. Its Choice self-consistency cookbook explicitly separates repeatability from accuracy and reports the share of automatic decisions alongside uncertainty. Its Noul cookbook similarly warns that the demonstrated band is neither calibrated nor optimized. Use those methods, not their threshold numbers, as inspiration. [Confidence routing](https://docs.typesafe.ai/patterns/confidence-routing), [Choice consistency](https://docs.typesafe.ai/cookbooks/consistency_choice_cookbook), [Noul consistency](https://docs.typesafe.ai/cookbooks/consistency_noul_cookbook).

Proposed next evaluation:

1. Preserve the 34 cases as a development/regression set because their errors are already known. Add labeled development examples covering negation, indirect directives, quote-plus-active-command mixtures, innocent quotes, hypothetical security work, applicant instructions, unfamiliar legitimate skills, wrong-field content and mismatched job duties. Label before calling either model.
2. Create a separate calibration set and a final held-out set; group paraphrases, translations and shared attack templates in the same split to avoid leakage. Keep balanced EN/PT coverage with independent Portuguese writing, rather than only translated copies. Hold final examples and labels out of prompt/threshold tuning.
3. Freeze model version, questions, option order, mapping, timeout, corpus hashes, variant count and success criteria before the final run. Fit any Noul thresholds on calibration data, then evaluate the frozen policy once on held-out data. If revised after seeing held-out failures, that set becomes development data and a new holdout is required.
4. Report false attack acceptance, legitimate false rejection, quoted-case routing, unusable/irrelevant confusion, insufficient-content acceptance, clarification rate, automatic coverage and failures separately. Compare final outcomes, not just model labels. Report by field and language, with denominators and appropriate uncertainty; agreement among repeated model calls is not independent correctness evidence.
5. Include repeat trials and preregistered Choice option permutations on representative boundary cases. Record distributions and decisions; the same variants must not be cherry-picked for each case. Measure added question tokens, classifier p50/p95 and full workflow timing again, including large inputs and the unresolved Application Draft baseline.

No specific numeric threshold or required sample size is established by the current evidence. Those depend on the agreed acceptable false-acceptance and user-friction rates. No corpus of this size can establish universal prompt-injection resistance.

## Limits and further CareerOS opportunities

TypeSafe's current Jev 1.13 limitations explicitly acknowledge adversarial steering, literal reading, distraction from large states, Choice option-order sensitivity, unreliable counting/date arithmetic and lack of generation. This is why an atomized classifier merits testing, but should not be treated as an input firewall or a replacement for source/schema validation and explicit Profile confirmation. [Jev 1.13 jaggedness](https://docs.typesafe.ai/model-jaggedness/jev-1.13).

The State documentation says English is Jev's primary training language and other languages have lower accuracy. The original EN/PT scores were identical on this particular set, which neither verifies nor refutes the broader provider warning. Portuguese calibration and held-out cases are necessary. Pin the model version for comparisons; documented aliases can change. [State](https://docs.typesafe.ai/concepts/state), [Models](https://docs.typesafe.ai/models).

The guardrails cookbook demonstrates a battery of hazard questions and application-owned precedence/thresholds. That architecture supports exploring separate override/quote factors, but its broader harmful-content taxonomy is outside #29's agreed behavior. Its example successes must be read alongside the provider's own adversarial limitation; no claim that a jailbreak can never steer Jev is supported. [Guardrails cookbook](https://docs.typesafe.ai/cookbooks/llm_guardrails).

Beyond #29, three bounded ideas deserve separate evaluation, not immediate implementation:

- **Choose existing source spans for contact values:** code finds candidate emails/phones; Jev selects a candidate with an explicit none option; code copies and normalizes it. This constrains invented values but does not prove the chosen person's identity or role is correct, and candidate-finder recall limits success. [Pre-parsed extraction cookbook](https://docs.typesafe.ai/cookbooks/pre_parsed_value_extraction_cookbook).
- **Verify extracted Profile facts against source text:** batch per-field checks for supported values and incorrectly omitted fields before presenting a proposal. This could complement the existing OpenAI extraction; it must not auto-apply facts, infer proficiency or replace deterministic schema validation. Added vendor disclosure, cost, latency and verifier errors need evaluation. [SDE cascade](https://docs.typesafe.ai/cookbooks/sde_cascade).
- **Validate explicit date components:** choose present/missing date parts, then assemble/compare dates in code. This may help preserve unknown components but cannot replace exact arithmetic or justify guessed employment durations. The cookbook's example fills in an unstated year from its reference date; CareerOS must preserve missing employment-date parts as unknown rather than copy that default. [Date extraction cookbook](https://docs.typesafe.ai/cookbooks/date_extraction_cookbook).

Jev should remain a candidate for bounded semantic judgments, while generative drafting remains with a text-generating model. The highest-value next step is a small reproducible rubric experiment that separates quoted wording from active directives and unreadable text from readable unrelated content. The provider decision remains open until new evidence and the user's review.

## Review and verification

### Standards

Independent review found no documented standards violations, evidence-boundary defects or actionable judgment smells. Relative links in all three changed documents resolve; no credential values or private credential paths appear in these documents.

### Spec

Independent review found no actionable findings against the user's request to explore the introduction and referenced documentation before deciding. Proposed experiments remain unmeasured; original results, pending provider selection and the drafting-overhead limitation remain explicit.

Review totals: Standards 0 findings; Spec 0 findings. Local link validation and whitespace checks pass. This follow-up changes documentation only; it makes no new runtime-test or provider-performance claim.

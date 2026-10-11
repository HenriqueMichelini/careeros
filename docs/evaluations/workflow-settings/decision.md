# Configuration decision — 2026-10-10

Retain the current backend-owned fixed settings. Neither Luna with low reasoning
nor Sol with low reasoning demonstrated an end-to-end quality improvement within
the existing runtime. This is a conservative configuration decision, not a
certification of current quality: the current Portuguese clarification proposed
Java as a user skill from an unidentified third person, and current continuation
and language failures remain explicit below. No production setting changed.

## Evidence boundary

The user authorized synthetic-only calls to the existing OpenAI and TypeSafe
recipients, capped at 60 HTTP workflow attempts and 300 provider dispatches,
without retries or fallback. An earlier invocation stopped before credentials
could be read and made zero calls. After the vault was unlocked, one interleaved
matrix ran against the frozen `cases.v1.json` and the source hashes recorded in
`live.v1.json`. No inputs, labels, prompts, deadlines, or output bounds were tuned
in response to these results.

`live.v1.json` is the unmodified provider evidence. `reviews.v1.json` records
agent-assisted source review for every logical result, including material fields
on both sequence legs. `reviewed.v1.json` is an offline derivation linked by the
original report's SHA-256. Its acceptance annotations mean synthetic evaluation
acceptance, not human approval or application of any generated proposal. The
review examines exact fixture facts and excerpts; provider support judgments,
identifiers, hashes, HTTP status, and regex matches are not semantic proof.

This small matrix has one sample per case/configuration, seven cases per language,
with inherited development/heldout labels and translated counterparts. It is
regression evidence, not a generalization study or a reliable latency/cost
forecast. Where Jev asks for more detail, no downstream model is called. Where an
initial ingestion response has no question, the runner stops rather than
fabricating a question or bypassing the gate. Those unmeasured paths weaken any
claim of improvement and remain in attempt/cost accounting.

## Explicit regressions and field observations

- **Language:** English Luna-low `en-fact.outcomes[0].reason` is Spanish.
  Both Luna configurations' English continuation reasons are Spanish on the
  first portion and Portuguese on the second. Current `pt-fact.claims[0].text`,
  `pt-correction.claims[0].text`, and both Luna configurations' Portuguese continuation claim texts are
  English. Current Portuguese clarification has English claim text and Spanish
  explanation; Luna-low repeats an English subject question on Portuguese input.
- **Unsupported ownership:** current `pt-clarification.steps[0].operations[0]`
  adds Java to the user's skills from “Ele usou Java.” The subject has not been
  identified and the answer has not been dispatched. This is a source-reviewed
  factual addition despite zero curated forbidden-pattern hits. It is rejected.
- **Clarification completeness:** current English clarification incorporates
  “I used Java at Acme” and preserves answer provenance. Its strict claim regex
  misses the supported paraphrase “Used Java at Acme.” Luna-low repeats the
  question and omits the answered personal use/Acme context in both languages.
  Sol's answer legs return `invalid_output` in both languages; no final proposal
  is available. A source reference alone does not establish that an answer was
  incorporated.
- **Correction:** all six email sequences are incomplete. Current/Luna-low
  English inputs stop at Jev request-information; Sol English and all Portuguese
  inputs retain uncertainty without returning a question. No correction answer
  is sent, so neither a successful email replacement nor its downstream cost is
  claimed. Controlled tests exercise both complete legs and charge both gates.
- **Continuation:** English current/Luna-low retain all 35 source facts and
  tool additions, but fail explanation language. Frozen labels expect `skills`,
  while these actual outputs use the valid `tools` field: the resulting 35 label
  misses are not 35 semantic omissions. Current Portuguese drops Tool09 after a
  `support_source` rejection; its fact and operation are genuinely absent.
  The rejected claim remains in the original step evidence. Sol in both languages hits
  the unchanged 24-second per-call timeout before completing the first portion.
- **CV:** all six stable-fact CV samples return 200 and retain the 20% latency
  metric and `stable-impact` identity in the requested CV language. Some reasoning
  samples expose uncertainty from the legacy fact metadata; they do not invent a
  new achievement. Historical #34 general-CV EN/PT `invalid_output` samples remain
  in their original report. This adapted stable-fact sample does not demonstrate
  that those older general-CV failures are fixed; #65 remains independently open.
- **Matching:** the sparse English “Java required.” and Portuguese “Java obrigatório.” postings are returned as
  Jev request-information for all configurations. Its matching/understanding
  output is unmeasured; empty output is not treated as perfect precision.
- **Draft EN:** current/Luna-low preserve Java and disclose unknown AWS/project
  experience. Their Java answers are present, with insufficient use context
  explicitly unknown; independent artifact review still raises use-answer and
  support concerns. Current has invalid job-citation offsets. Numbered answer
  lists trigger `invented_number` support flags; those are not invented career
  metrics on source inspection. Sol explicitly answers “I use Java” but its
  artifact support check is unavailable at the workflow deadline. No English
  draft is accepted merely because generation returned HTTP 200.

Per-result language, exact source, unsupported assertions, material omissions,
citation failures, and unavailable checks are retained in `reviews.v1.json` and
the derived report. Curated label precision/completeness remain unchanged,
including known overly strict paraphrase/destination matches. They must not be
substituted for the source review or collapsed into an overall quality score.

- **Draft PT / #34 answer regression:** all three current-contract samples
  explicitly include “Uso Java” in `applicationAnswers`; the historical answer
  omission is not reproduced in these samples. Required how-use detail is unknown
  because the sole source says only “I use Java.” Current/Luna-low still have
  invalid job citations and support concerns; Sol's artifact review is unavailable.
  No AWS experience is fabricated. Luna-low's Portuguese-language answer is scoped
  to this application and is consistent with explicit `cvLanguage: pt-BR`; its
  support review's personal-preference objection is retained without treating
  the requested application language as an invented career fact.

## Cost, caching, and runtime interpretation

All reached validation, preparation/retrieval, extraction, reconciliation,
continuation and support stages stay in the report. Calls that fail or time out
remain in numerators. Unknown usage is null, not zero. Reasoning tokens are a
subset of output and are not charged again. Cache reads/writes are subsets of
input. Dated price assumptions and the exact returned model are used only where
all required components are available.

TypeSafe pricing is unavailable here, and timed-out OpenAI calls can lack usage.
Consequently full workflow cost, cost per completed result and cost per accepted
result remain unavailable where any call is unpriced. The priced-component sums
below are partial estimates, not total spend or savings. No invoice evidence is
available. Failed work cannot be omitted to make accepted-result cost look lower.

Natural unchanged instruction-prefix hashes and returned cache reads/writes are
reported without padding, TTL/key changes, or retention changes. A zero observed
cache read is no promised saving; cache writes do not establish subsequent hits.
No extra repeat was dispatched. Session-only result reuse from #49 is distinct
from provider caching and is not assigned an extrapolated saving here.

Workflow durations below sum sequential HTTP legs and include failed attempts;
provider stages can overlap and are not summed as end-to-end latency. The existing
limits remain: ingestion 24 seconds per provider call and 52 seconds per HTTP
workflow; application generation/support share 25 seconds; CV has its existing
25-second provider deadline. Continuation and deliberate answers are separate
bounded HTTP legs, not an expanded production timeout. One observed result per
case does not support population percentiles, confidence intervals, or claimed
percentage savings. No configuration passes the full curated quality bar, so a
cheaper partial estimate or a shorter failed path cannot justify adoption.

## Measured matrix

All 42 logical results were attempted and reviewed: **51 HTTP attempts and 132
provider dispatches**, within the authorized 60/300 caps. Nine planned later
HTTP legs were not dispatched after absent questions or failed first portions.
There were no automatic retries. Jev was called 50 times and OpenAI 82 times.
Four Sol OpenAI calls have unavailable usage (two continuation extraction
requests and two draft artifact assessments). Every dispatched call is retained.

“Completed” requires all reached workflow diagnostics to complete; a 200
request-information decision or unavailable support check is not completed.
“Accepted” additionally requires the labeled source review. All configurations
fail the full quality bar; counts below are descriptive, not adoption rankings.

| Configuration | Logical results | Completed | Accepted by review | Calls | Unpriced calls | Sum workflow seconds | Priced components USD |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| current | 14 | 9 | 4 | 44 | 16 | 153.647 | 0.01666482 |
| luna-low | 14 | 10 | 3 | 48 | 18 | 217.095 | 0.02107753 |
| sol-low | 14 | 4 | 4 | 40 | 20 | 167.734 | 0.09505400 |

Full estimated USD, USD/completed, and USD/accepted are **null for every
configuration**, because Jev pricing is unknown and Sol additionally has four
unmeasured OpenAI calls. Priced components cannot establish total spend, cheaper
accepted results, or savings. Current has one source-reviewed ownership addition;
no additional forbidden career assertions were established in the two candidate
outputs reviewed, but unavailable/failed outputs are not certified safe. Their
language, completeness and runtime failures prevent a demonstrated improvement.
All 42 curated forbidden-pattern counts are zero; this misses the ownership error
and is not the source-reviewed factual-addition count.

The following token totals include **all providers and failed/rejected attempts**.
Each cell is `observed tokens / calls with unavailable component`:

| Configuration | Input | Output | Reasoning (output subset) | Cache reads (input subset) | Cache writes (input subset) |
| --- | ---: | ---: | ---: | ---: | ---: |
| current | 79586 / 0 | 19847 / 0 | 0 / 16 | 0 / 16 | 54409 / 16 |
| luna-low | 88340 / 0 | 27141 / 0 | 5217 / 18 | 0 / 18 | 61305 / 18 |
| sol-low | 43172 / 4 | 5103 / 4 | 451 / 20 | 0 / 20 | 17332 / 20 |

All 78 OpenAI calls with returned usage report zero cache-read tokens. Their
reported writes total 133,046 tokens; the 50 Jev and four timed-out OpenAI calls
provide no cache components. Stable natural instruction hashes are retained per
call; no cache hit or measurable caching saving is established by this run.
Returned priced models are exactly `gpt-6-luna` / `gpt-6.1-sol`; Jev returns
`jev-1.13.0`. Observed OpenAI ceilings remain 4,000, 6,000 and 8,000 tokens for
their existing tasks, identical across configurations.

## Per-case completion, completeness and latency

Each cell is `HTTP status; curated covered/expected; calls; workflow seconds;
accepted yes/no`. The curated denominator is frozen; valid paraphrases/tools
placements and genuine omissions are distinguished in the field review above.
A 409 is the runner's explicit incomplete-sequence marker when it cannot dispatch
a valid answer, not a new production endpoint response. Full source reviews for
all fields are indexed by the original report's zero-based run index.

| Case / language | current | luna-low | sol-low |
| --- | --- | --- | --- |
| en-fact | 200; 2/2; 3; 6.030s; yes | 200; 2/2; 3; 4.649s; no | 200; 2/2; 3; 7.727s; yes |
| en-stable-cv-metric | 200; 2/2; 1; 1.241s; yes | 200; 2/2; 1; 4.661s; yes | 200; 2/2; 1; 5.858s; yes |
| en-clarification | 200; 1/2; 7; 9.335s; yes | 200; 0/2; 7; 15.415s; no | 502; 0/2; 6; 14.198s; no |
| en-correction | 409; 0/2; 1; 0.293s; no | 409; 0/2; 1; 0.239s; no | 409; 0/2; 3; 6.670s; no |
| en-continuation-35 | 200; 35/70; 6; 46.610s; no | 200; 35/70; 6; 60.050s; no | 504; 0/70; 2; 24.253s; no |
| en-evidence-matching | 200; 0/1; 1; 0.231s; no | 200; 0/1; 1; 0.243s; no | 200; 0/1; 1; 0.226s; no |
| en-reviewed-draft | 200; 8/15; 4; 13.816s; no | 200; 9/15; 4; 21.985s; no | 200; 6/15; 4; 25.003s; no |
| pt-fact | 200; 2/2; 3; 3.778s; no | 200; 2/2; 3; 5.163s; yes | 200; 2/2; 3; 7.818s; yes |
| pt-stable-cv-metric | 200; 2/2; 1; 1.425s; yes | 200; 2/2; 1; 4.788s; yes | 200; 2/2; 1; 5.259s; yes |
| pt-clarification | 409; 1/2; 3; 3.599s; no | 200; 0/2; 7; 13.484s; no | 502; 0/2; 6; 14.771s; no |
| pt-correction | 409; 0/2; 3; 3.765s; no | 409; 0/2; 3; 7.221s; no | 409; 0/2; 3; 6.439s; no |
| pt-continuation-35 | 200; 34/70; 6; 52.703s; no | 200; 35/70; 6; 56.683s; no | 504; 0/70; 2; 24.235s; no |
| pt-evidence-matching | 200; 0/1; 1; 0.249s; no | 200; 0/1; 1; 0.271s; no | 200; 0/1; 1; 0.269s; no |
| pt-reviewed-draft | 200; 8/15; 4; 10.573s; no | 200; 9/15; 4; 22.244s; no | 200; 5/15; 4; 25.009s; no |

The controlled report remains separate: 42 logical results, 60 HTTP attempts,
186 simulated calls, all curated labels matched. It verifies accounting and
contracts, not live factual quality. The retained outcome is fixed current
settings; further workflow reliability work needs its own issue scope and any
new provider run needs deliberate authorization. #65 remains open.

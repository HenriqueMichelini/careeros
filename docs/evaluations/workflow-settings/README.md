# Fixed workflow configuration comparison — #50

The deliverable is an accuracy-first configuration decision. The evaluation tool
runs the production HTTP handlers with an evaluation-only transport. It never
changes application routing, prompts, schema, output ceilings, Jev policy,
retention, or review/apply behavior. No user model picker, automatic escalation,
retry, repair loop, or provider fallback is introduced.

## Frozen comparison

Review baseline: `f61ac2fe218bce914ca6cf47e14fc96e404fda57`.
Milestone branch: `codex/ai-quality-and-efficiency`, PR base `main`.
All six native prerequisites were closed when implementation began.

| Configuration | OpenAI model | Reasoning | Prompts and bounds |
| --- | --- | --- | --- |
| current | gpt-6-luna | none | Existing production settings |
| luna-low | gpt-6-luna | low | Identical to current |
| sol-low | gpt-6.1-sol | low | Identical to current |

These are bounded task configurations, not automatic routing candidates. Each
configuration is applied to every reached OpenAI task, including reconciliation,
matching, and independent support assessment. Jev stays `jev-1.13.0` with the
existing field rubric/policy. The evaluation records actual outbound settings;
production artifact version strings still describe the production Luna policy.
Candidate artifacts must not be reused in the application.

`cases.v1.json` freezes 14 synthetic cases (seven per language), with original
#34 source IDs, development/heldout splits, source material, gold labels and
controlled responses. It adapts the historical CV cases to current stable fact
references and the draft cases to reviewed resume/artifact contracts. The
historical #34 reports and labels remain unchanged.

| Cases per language | Required observations |
| --- | --- |
| fact | extraction and reconciliation; language drift remains a separate review finding |
| stable-cv-metric | retain 20% ownership; historical EN/PT `invalid_output` remains explicit regression evidence |
| clarification | initial ambiguous submission, then answer with the actual returned unresolved claim; preserve origins |
| correction | initial uncertain email, then deliberate answer; replacement stays a proposal and both originals/answers are validated |
| continuation-35 | both portions, all 35 facts/destinations, whole original gate on every portion |
| evidence-matching | canonical fact and exact source-backed requirement, separately observed explanation |
| reviewed-draft | generation plus resume and remaining-artifact support assessments; Java answer scored in its own field |

Every configuration runs the same cases, interleaved current/Luna-low/Sol-low to
reduce time/order bias. Cache observations use unchanged instruction prefixes naturally shared across
these requests; no additional repeats are dispatched. The complete matrix is 60
HTTP attempts (including initial clarification/correction legs and continuation),
42 logical results, and 186 simulated calls in controlled preflight. The live cap is 60 HTTP attempts and 300 dispatch attempts; dispatch
failures count. Each new invocation needs its own deliberate authorization and
new output path. The tool does not resume or retry automatically.

Heldout inputs/labels remain frozen through the comparison. Many are translated
counterparts or previously observed #34 cases, so this is a small regression
comparison, not an independent generalization study. One sample per case and
configuration provides no reliable population latency estimate,
confidence interval, or savings percentage. No candidate prompt was tuned.

## Evidence and quality decision

Reports retain each output, conservative unsupported candidates, individual
missing atoms and fields, language, all provider calls, returned usage and stage
diagnostics. Continuation retains each response and scores the complete 35-fact
union against the full denominator. Clarification/correction retains and charges
both initial and answer legs, scoring the final output. If the initial response
does not produce exactly one unresolved claim, the sequence records an incomplete
result rather than inventing a question or sending the answer anyway. The actual
returned claim supplies the answer request; frozen user answers remain identical
across configurations. The original and answer gates, local
preparation/retrieval, reconciliation, and both draft support checks remain in
workflow accounting. Stage durations can overlap; use measured workflow duration
(or the sum of sequential continuation workflow durations), not summed stages.

HTTP completion, curated label scores, source hashes, citations, and a provider's
support judgment are not acceptance. Offline review annotates every material
field against exact synthetic source facts, separately recording unsupported
claims, omissions, uncertainty and language drift. Agent-assisted adjudication
must be labeled as such; it does not impersonate user acceptance. Acceptance
means the explicitly labeled evaluation decision, never automatic application of
a Profile Proposal or submission of an Application Draft.

Accuracy comes first: zero forbidden factual additions on curated acceptance
cases, no precision regression, and separately reported completeness. Unknown
or unresolved assertions prevent a candidate from demonstrating an improvement.
A cheaper but worse output is not an improvement. Retain current fixed settings
when no candidate meets the bar or the sample/cost evidence cannot demonstrate
an improvement. The separate open general-CV reliability issue #65 is not
silently claimed fixed by this comparison.

## Cost and caching

Returned usage is recorded even for failed or rejected responses where available.
Missing usage stays null; reasoning is a subset of output, cache reads and writes
are subsets of input. Every available component and unavailable-call count is
reported. Rates are dated assumptions from the official model pages checked on
2026-10-10: Luna $0.10 input / $0.01 cached / $0.125 cache writes / $0.50 output;
Sol $2 / $0.10 / $2.50 / $10, per million tokens, standard short-context requests.
Returned dated model aliases require an explicit matching rate before pricing.

Estimated call cost is `(input - cached - writes) * inputRate + cached *
cachedRate + writes * writeRate + output * outputRate`, divided by one million.
Unavailable cache-write usage is not silently zero. Jev pricing or incomplete
usage leaves total/accepted-result cost unavailable, while known OpenAI
components may be shown separately. Failed attempts remain in cost numerators.
Actual billing is unavailable without invoice evidence; service-tier, regional
or provider-specific charges are not invented.

The first 256 instruction characters are hashed only to observe stable natural
prefixes, not to establish cache eligibility or semantic safety. Full request,
prompt/schema and source hashes pin the exact compared requests/versions. No
prompt padding, cache TTL/key mutation, sensitive input persistence or retention
change is used. An observed zero cache read is not a promised saving. Session-only
application reuse remains separate: retries still require the complete-original
Jev gate; existing #49 tests cover invalidation and call counts. This matrix
includes remaining validation/support work and makes no extrapolated reuse gain.

Official sources:
[GPT-6 Luna](https://developers.openai.com/api/docs/models/gpt-6-luna),
[GPT-6.1 Sol](https://developers.openai.com/api/docs/models/gpt-6.1-sol),
[cache accounting and retention](https://developers.openai.com/api/docs/guides/prompt-caching),
[Chat Completions usage](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create).

## Reproduce

Credential-free controlled preflight:

```sh
GOCACHE=/tmp/careeros-go-cache go run -buildvcs=true ./scripts/workflow-settings \
  --output /tmp/workflow-settings-controlled.json
```

Only after explicit authorization for this matrix and budget:

```sh
GOCACHE=/tmp/careeros-go-cache go run -buildvcs=true ./scripts/workflow-settings \
  --mode live --authorize-live --max-workflows 60 --max-calls 300 \
  --openai-key-file /private/openai-assignment \
  --typesafe-key-file /private/typesafe-assignment \
  --output /tmp/workflow-settings-live.json
```

Credential files use `OPENAI_API_KEY=value` / `TYPESAFE_API_KEY=value`. Values
remain process-local and are never logged or stored in reports. All fixtures
must be synthetic; never use a real Profile. Reports are exclusive-create mode
0600 and checkpointed after each logical result, including errors.

Offline review reads an original saved report and a JSON array of
`{index, review: {accepted, method, reason, fields: [{field, assessment, source,
finding}]}}`. It writes a separate report with the original report's SHA-256;
provider calls and outputs remain historical evidence, not new dispatches:

```sh
GOCACHE=/tmp/careeros-go-cache go run ./scripts/workflow-settings \
  --replay /tmp/workflow-settings-live.json --reviews /tmp/source-review.json \
  --output /tmp/workflow-settings-reviewed.json
```

Controlled transports have no network fallback. Offline review refuses live
mode, authorization and credential flags. New output files are required in every
mode. Source and fixture hashes identify the exact workflow even for an
uncommitted evaluation; compiler VCS metadata records the starting commit.

## Completed execution and decision

After the user unlocked the vault, the authorized live matrix completed with
42 logical results, 51 HTTP attempts and 132 provider calls, with no retries.
`live.v1.json` preserves the original evidence. `reviews.v1.json` annotates every
result against its synthetic sources; `reviewed.v1.json` is an offline derivation
linked to the original SHA-256. Original outputs, calls and curated scores are
unchanged by review.

[The configuration decision](decision.md) retains current fixed settings:
neither candidate demonstrated the required end-to-end quality/runtime benefit.
It records language drift, unsupported ownership, omissions, incomplete support,
cache usage, latency, and unavailable total/accepted-result cost explicitly.
Current quality is not certified by retaining its settings; #65 remains open.

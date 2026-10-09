# Content-free AI diagnostics (#35)

The five production HTTP workflows emit one `ai_diagnostic` JSON event per attempt through the existing log stream. No content store, endpoint, dashboard, analytics tracking, extra request, retry, provider change or credential use is introduced. Existing HTTP errors and `Cache-Control: no-store` remain; parsable partial completions are now rejected before workflow output validation.

Each event has a fresh random attempt ID unrelated to source content, workflow duration, HTTP status, classified outcome and stage records. Provider request IDs, headers, URLs, source IDs, content hashes, Profile facts, excerpts, postings, generated prose, prompts, refusals and keys are excluded. Provider model values are restricted to the configured aliases and known dated OpenAI snapshot shape; an unknown returned model is unavailable. Missing or invalid numeric usage is `null`, never an invented zero. Explicit provider zero is retained.

## Stages and versions

- `mechanical_preparation`: normalization, structure, repetition, planning and bounds versions from preprocessing. Local only.
- `field_validation`: `jev-rubric-v1/policy-v1`, the approved Jev gate. Captures returned `input_tokens` and `output_tokens` where available. An HTTP 200 field rejection does not count as a completed workflow.
- `extraction`: `profile-claims-prompt-v1/schema-v1/source-resolution-v1`. Includes existing excerpt verification and filtering.
- `candidate_retrieval`: `lexical-projection-v1`, the existing Profile projection. Local only; no vector infrastructure.
- `reconciliation`: `profile-operations-prompt-v1/schema-v1/reconciliation-v1`, distinct from extraction. Includes existing proposal validation/filtering.
- `statement_support_checks`: `cv-heuristic-grounding-v1`, the existing general-CV checks. Local only; this finite heuristic is not a semantic verification guarantee.
- `generation`, `qualification_matching`, `section_review`: their application-owned prompt/policy and schema v1 labels.

Bump the applicable label when its prompt, rubric, schema or policy changes. These are code-owned versions, never hashes of user-dependent requests. An enclosing stage includes its child local stage durations; **do not sum overlapping stage durations** to infer workflow latency. Provider duration runs from request dispatch through complete envelope consumption/decoding; it excludes downstream workflow validation. Workflow duration is measured separately. Stages that are never reached are absent; a reached reconciliation with no claims is local and has no provider event.

`provider: null` means local work with no provider request or token claim. For provider calls, output ceilings are request settings, while input/output/reasoning/cache fields are returned consumption. Reasoning is a subset of output; cached tokens are a subset of input, not additional tokens. Prefix caching is provider accounting; `applicationResultReuse: false` describes this implementation and is independent of prefix cache hits.

Complete JSON envelopes require `finish_reason: stop`, no refusal, valid JSON content and the workflow's required output keys. Refusal/content filtering, token truncation, incomplete output, malformed output, rejected workflow output, provider failure, transport failure and timeout remain distinct diagnostic outcomes. A successful parse or HTTP status alone cannot establish completion or user acceptance. Existing bounded claim/operation filtering and unresolved review items remain intact; `completed` means the workflow returned its reviewable result, not that all source claims were understood or accepted.

Completion/usage field definitions: [official OpenAI Chat Completions reference](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create). No live provider claim is made by these changes.

## Reproduce and aggregate

Run the frozen #34 synthetic fixtures through the production handlers using controlled transports:

```sh
GOCACHE=/tmp/careeros-go-cache go run ./scripts/ai-diagnostics --controlled > /tmp/ai-controlled.json
```

This command has no live mode and makes no network calls. It outputs only diagnostics and the summary, not #34's content-bearing output/hash artifacts. Controlled fixture envelopes omit usage intentionally: their token totals and costs must remain unavailable. The `RunResult.Diagnostics` field also makes the same collector available to the existing opt-in #34 harness; this issue does not authorize running its live mode.

Aggregate a JSON array of diagnostic attempts offline:

```sh
GOCACHE=/tmp/careeros-go-cache go run ./scripts/ai-diagnostics --input /tmp/attempts.json
```

To aggregate the controlled command's saved attempts again:

```sh
python3 -c 'import json; json.dump(json.load(open("/tmp/ai-controlled.json"))["attempts"], open("/tmp/attempts.json", "w"))'
```

Totals include available components only and report available/unavailable request coverage. `stagesByVersion` separates requests, durations, outcomes and observed tokens for each stage/configuration. The attempt list retains workflow attribution; select one fixed workflow/configuration for a cost comparison rather than mixing different tasks.

Optional `--prices /path/to/prices.json` takes a JSON array of `{provider, model, asOf, inputPerMillionUSD, cachedPerMillionUSD, outputPerMillionUSD}`. Rates must be nonnegative USD per million tokens; `asOf` must be an ISO date. They are caller-supplied assumptions for the exact returned model, not verified current prices. No prices are bundled and no prices are fetched. The unit tests use explicitly synthetic prices.

Estimated call cost is `(input - cached) * inputRate + cached * cachedRate + output * outputRate`, divided by one million. Reasoning is not charged a second time. Missing usage, missing cached breakdown or unknown/unpriced models leave that request unpriced. Known priced components may be shown separately; total cost and per-result cost stay null when any request is unpriced. All estimates remain separate from actual billing, which is unavailable without invoice evidence (including provider-specific fees/discounts).

Cost per completed result includes failed-attempt costs in the numerator. `accepted: null` means no acceptance evidence. A trusted offline evaluator may annotate `accepted: true/false` only after human acceptance or an explicitly labeled acceptance decision; a successful HTTP call and the #34 deterministic score do not supply that evidence. Acceptance rates use only annotated attempts and expose their coverage. Cost per accepted result remains unavailable without accepted evidence or complete priced usage. The production collector never infers acceptance.

## Controlled verification

The public handler tests exercise complete/absent/partial/invalid usage, privacy sentinels, unknown model values, refusal, parsable token truncation, missing finish status, malformed and rejected content, HTTP/transport failures, Jev field rejection, no-store and one-call behavior. The report tests cover missing billing, observed component totals, dated synthetic prices, reasoning/cache subset accounting and charging failed attempts to a completed result. They use the user-confirmed seams: production HTTP handlers with controlled provider transports, and the diagnostic report interface.

The saved `ai-diagnostics.controlled.v1.json` records 34 controlled fixture workflows and 80 simulated provider requests. All 34 returned completed workflow results. Usage, actual cost, acceptance and live quality are unavailable; this is a routing/reporting baseline, not a provider-quality or billing measurement. Timing values are local samples and vary between runs.

# Field-validation provider evidence

Issue: [#29](https://github.com/HenriqueMichelini/careeros/issues/29). Checked: 2026-10-07. Scope: official documentation and local source inspection; no paid inference, credentials, or account settings were inspected. These findings establish documented capabilities, not measured quality, latency, cost, or account access.

## OpenAI baseline

The official model page identifies `gpt-6-luna`, supports text input/output and Structured Outputs, and lists Chat Completions and Responses. It documents a 1,050,000-token context window, 128,000 maximum output tokens, and reasoning efforts including `none`. API model documentation does not prove this user's account access or available credits. [GPT-6 Luna](https://developers.openai.com/api/docs/models/gpt-6-luna)

Standard short-context text prices are USD per million tokens: input **$0.10**, cached input **$0.01**, cache writes **$0.125**, output **$0.50**. Long-context rates differ; use the rate applicable to the actual request and service tier. Prices are a dated reference, not a measured workflow bill. Regional processing carries a documented premium. [API pricing](https://developers.openai.com/api/docs/pricing)

The model page and an indexed pricing-page version surfaced inconsistent descriptions of EU residency support across Standard/Flex/Batch. The live project configuration and applicable regional terms were not verified; this evaluation must not promise EU residency. [Model documentation](https://developers.openai.com/api/docs/models/gpt-6-luna), [pricing](https://developers.openai.com/api/docs/pricing)

## JSON reliability and factual reliability

Structured Outputs constrains a supported JSON Schema; JSON mode promises JSON syntax without guaranteeing the requested schema. Strict schemas use a supported subset and require `additionalProperties: false`. Refusals and incomplete responses need explicit handling. Schema-valid content can still contain factual mistakes or unsupported claims. Therefore score syntax, shape, source fidelity, and operational failures separately; a parseable response is insufficient evidence of accurate profile extraction. [Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs)

For this evaluation, record missing fields, unknown fields, invalid enum values, truncation, refusal, invented facts, dropped facts, incorrect entity assignment, and questions raised for ambiguity as distinct outcomes. These are evaluation recommendations inferred from the provider limitations and CareerOS's source-preservation requirements, not provider accuracy guarantees.

## Privacy and retention

OpenAI states API data is not used for model training unless the customer opts in. Default abuse-monitoring logs can retain prompts/responses for up to 30 days, with exceptions. Zero Data Retention and Modified Abuse Monitoring require approval. Chat Completions and Responses have different application-state behavior; Responses defaults to storage, and caching has additional retention considerations. Structured-output schemas are treated separately as system data. Do not infer zero retention from `Cache-Control: no-store`, a stateless handler, or omitting `store`. Actual organization/project settings were not verified. [Data controls](https://developers.openai.com/api/docs/guides/your-data)

Use synthetic or explicitly consented, minimized samples for any subsequent provider trial. Record each provider's retention, training, deletion, region, and access policy with a first-party source before sending personal profile data. This is the evaluation's data-handling recommendation, not a legal compliance determination.

## Measuring cost and latency

Chat Completions exposes `usage.prompt_tokens`, `completion_tokens`, and `total_tokens`, with detail fields for cached tokens, cache writes, and reasoning tokens. Reasoning tokens count within completion billing; do not add them twice. Capture actual usage plus returned model and service tier for every attempt, including retries and both ingestion stages. Do not enable stored completions merely to retrieve usage: collect the creation response's metadata in the controlled evaluation. [Chat completion reference](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/retrieve), [creation reference](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)

For a Standard short-context request with no cache writes and text-only input, an estimate in USD is `(uncached_input_tokens × 0.10 + cached_input_tokens × 0.01 + completion_tokens × 0.50) / 1_000_000`. If cache writes occur, distinguish their token category and applicable rate before calculating. Add all attempts for workflow cost; retain missing usage as unknown rather than zero. Reconcile aggregate estimates with provider billing where available. This calculation applies the dated pricing above and is not a forecast of actual usage.

Measure provider wall time, whole backend request time, and browser time until usable output independently. Report sample count, individual attempts, failures, and distribution summaries together. An application deadline is a bound, not observed provider latency.

## Local implementation baseline

The following is source evidence from the current checkout; it does not establish deployed revision or successful provider behavior.

| Workflow | Provider output mode | Maximum completion tokens | Provider deadline | Request-body ceiling |
| --- | --- | ---: | ---: | ---: |
| Profile review | JSON mode | 5,000 | 25 seconds | 64 KiB |
| Profile ingestion, each stage | Strict JSON Schema | 6,000 | 24 seconds | 160 KiB for the whole request |
| Qualification gaps | JSON mode | 1,200 | 25 seconds | 128 KiB |
| Application draft | Strict JSON Schema after `18d45da` | 8,000 | 25 seconds | 128 KiB |
| CV generation | JSON mode | 4,000 | 25 seconds | 128 KiB |

Application Draft was JSON mode at the initial source inspection; the [subsequent user-authorized fix and measurement](field-validation-draft-fixed-results.md) changed only its output format.

Sources: [profile review](../../backend/profile-review/main.go), [profile ingestion](../../backend/profile-ingestion/main.go), [qualification gaps](../../backend/qualification-gaps/main.go), [application draft](../../backend/application-draft/main.go), [CV generation](../../backend/cv-generation/main.go).

All five use `gpt-6-luna` with `reasoning_effort: none` and the shared [Chat Completions transport](../../internal/openaihttp/post.go). Ingestion accepts at most 30,000 input **bytes** (`len(string)` in Go), then extracts claims and compares them with the profile sequentially. Each stage receives its own provider timeout, so a whole ingestion request is not bounded by a single 24-second timeout. The parent request/deployment can impose an additional limit.

The inspected handlers decode model content but do not expose provider usage. App responses and existing duration logs therefore cannot establish per-request token cost. A controlled evaluator must capture sanitized usage metadata at the provider boundary or use provider billing evidence. Full payload or credential logging is unnecessary. This note does not change application instrumentation or schemas.

## Jev / TypeSafe AI

The user identified [TypeSafe AI](https://typesafe.ai/) as the intended Jev provider. Its primary documentation resolves the identity and API contract; YouTube is not needed to establish those facts. The homepage's speed, price, and zero-hallucination marketing are provider claims, not CareerOS measurements.

### Semantics and candidate fit

Jev evaluates supplied state using three question types: Choice selects a supplied option; Score rates a supplied rubric; Noul estimates the probability of a yes answer. It returns typed decisions rather than generating arbitrary text. Questions in one request are independently evaluated against the same state. [Introduction](https://docs.typesafe.ai/introduction), [coding-agent guidance](https://docs.typesafe.ai/introduction/coding-agents)

Choice and Score `confidence` is a statistic derived from concentration of their probability distribution: 1 at a single outcome and 0 for a uniform distribution. Noul has no separate confidence property. A concentrated distribution does not itself establish correctness on CareerOS fixtures, authorization to change a profile, or operational safety. Calibrate thresholds against labeled examples and retain human confirmation where the application requires it. [Confidence](https://docs.typesafe.ai/confidence)

Known Jev 1.13 limitations include literal interpretation, numeric precision, date ordering, irrelevant long context, adversarial state, and sensitivity to Choice option order. The provider recommends code for arithmetic and a generative model for text generation. [Known limitations](https://docs.typesafe.ai/model-jaggedness/jev-1.13)

Its extraction cookbook first finds candidate spans using regex/code, asks Jev to choose among them (including a none option), and copies the selected value verbatim. Candidate discovery is a prerequisite; missing candidate values cannot be recovered merely by selecting among available options. Names may need a roster, entity recognizer, or generative proposer. [Pre-parsed extraction](https://docs.typesafe.ai/cookbooks/pre_parsed_value_extraction_cookbook)

**Evaluation inference:** Jev is a candidate for narrow semantic routing, validation of proposed claims, or selecting among source-derived values. It is not a direct replacement for CareerOS's free-form claim extraction, profile rewriting, CV summaries, or application-draft text. Any hybrid must include candidate-generation time, cost, missed candidates, and errors in its end-to-end comparison. Typed membership prevents choosing an out-of-set value; it does not prove the selected source value belongs to the correct person, employer, date, or field.

### Availability, pricing, and bounds

Current documented model: `jev-1.13.0`; aliases `jev-latest` and `jev-preview` currently resolve to it. Pin the version for reproducible evaluation and record the returned version. Documented price is **$0.042 per million input tokens**, with output tokens free. Limits are 64k total request tokens, and 32k for state plus the longest question. Inputs are text/string/JSON object/array; images/audio/video need preprocessing. Published rate limits are dynamic. English is the primary training language; Portuguese accuracy requires direct evaluation. `GET https://api.typesafe.ai/v1/models` lists account-visible aliases, although versioned IDs may be accepted without appearing there. None of these documented facts verifies this user's access or balance. [Models and prices](https://docs.typesafe.ai/models)

HTTP evaluation uses `POST https://api.typesafe.ai/v1/systemone`, Bearer authentication, and JSON `{model,state,questions}`. Choice permits up to 255 options; Score accepts up to 10 levels. Response metadata includes `model` and `usage.input_tokens` / `output_tokens`. Documented errors include 401, 422, 429, and 529; SDK retries can affect observed cost/time. Record each actual attempt and configure a bounded retry policy. [API reference](https://docs.typesafe.ai/api)

Jev-only input cost estimate is `sum(actual_attempt_input_tokens) × 0.042 / 1_000_000` USD at the documented price. Add all other providers and candidate preparation costs for a hybrid. Do not infer input-token equality across providers or count supplied characters as tokens. This is a calculation method, not measured spend.

### Privacy and access prerequisites

The Privacy Policy states TypeSafe does not train/fine-tune on Input, hosts services in the US, and retains personal data as reasonably necessary rather than giving a fixed default deletion interval. It describes deletion measures on request, subject to exceptions. [Privacy Policy](https://typesafe.ai/legal/privacy-policy)

The DPA describes purpose-limited processing and necessary retention; the legal docs advertise ZDR for enterprise customers. Actual account retention controls, contractual terms, and ZDR eligibility remain unverified. [DPA](https://typesafe.ai/legal/data-processing), [legal documentation](https://docs.typesafe.ai/legal)

The customer agreement requires TypeSafe-managed credits; promotional credits are discretionary. The account shows its balance. It also permits telemetry processing and warns that outputs may be inaccurate. Therefore do not assume an API key implies funded access or that no-training means no storage. [Customer agreement](https://typesafe.ai/legal/mca)

Human setup, addressed to the user: log in at the [TypeSafe console](https://console.typesafe.ai/), create an API key using the dashboard, and provide it privately to the evaluation process as `TYPESAFE_API_KEY`. Do not paste the key into chat or commit it. Verify the credit balance and spending limit before the separately authorized trial. The documented quick start establishes dashboard key creation and the environment variable; no SDK or TypeSafe agent skill installation is necessary to call its HTTP API. [Quick start](https://docs.typesafe.ai/introduction/quickstart)

## Decision boundary

This source review establishes documentation claims, not measured quality or account configuration. Subsequent authorized synthetic calls and the measured recommendation are recorded in [the live comparison](field-validation-live-results.md). Provider approval remains pending; no source/marketing claim substitutes for those measured decisions or the unresolved drafting limitation.

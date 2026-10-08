# Application Draft measurement blocker — 2026-10-07

Issue [#29](https://github.com/HenriqueMichelini/careeros/issues/29). This is a narrow diagnosis of the existing workflow baseline, separate from the frozen classifier comparison. It changes no production code or provider recommendation. The user has been asked whether to fix and measure the workflow before acceptance or accept the documented gap.

## Finding

Two additional synthetic calls through the exported `applicationdraft.NewHandler()` reproduced HTTP 502 `invalid_output`, despite upstream HTTP 200 and complete JSON (`finish_reason: stop`). In both responses all five other top-level fields were present, and the cover-letter checks passed. `applicationAnswers` was not a nonempty string. The second probe established that it was an **array**, while the backend's result type requires a string.

In an offline replay of that second response, replacing **only** `applicationAnswers` with a synthetic diagnostic string changed the real handler's response from 502 to 200. This isolates the field's type mismatch as the rejection cause in that response. The replay does not count as successful generation, suitable application material or a live workflow measurement. The first probe did not record the field's type; the four earlier baseline responses were not retained, so their precise cause remains unknown.

## Method and evidence

The fixture was the existing `en-complete-job-timing` synthetic employer/posting with the evaluation runner's Java-only projected Profile and English CV language. The existing handler constructed its real prompt and made each provider request unchanged. Temporary Go instrumentation wrapped `http.DefaultTransport`, forwarded the request and inspected the response in memory. An offline transport supplied the modified response for the single-field causal replay, making no additional network request.

Instrumentation tested four alternatives: malformed JSON/fields, cover-letter validation, missing required values, and truncation. It recorded only validation flags, token usage, HTTP status and timing; keys, headers and generated prose were never saved. The [metadata record](results/2026-10-07/draft-diagnostic.json) preserves source/payload hashes and each probe's measured values. Provider response bodies were discarded when the process exited.

| Measurement | Probe 1 | Probe 2 |
| --- | ---: | ---: |
| Real handler HTTP status | 502 | 502 |
| Upstream status / finish reason | 200 / stop | 200 / stop |
| Duration | 3,937 ms | 3,183 ms |
| Input / output tokens | 563 / 350 | 563 / 265 |
| `applicationAnswers` type | Not recorded | Array |
| Offline replay with string-only replacement | Not attempted | 200 |

These were two deliberate diagnostic calls, without automatic retries, using the user-authorized OpenAI key. They add **1,126 input / 615 output tokens**, outside the 308 follow-up classifier calls and the 12 earlier workflow-stage calls. Actual billed cost remains unknown. No new Jev call occurred, no successful live draft was obtained and no production request/response contract was changed.

## Concrete next step

The existing prompt requests nonempty strings, but JSON mode alone did not enforce that contract in these probes. A follow-up fix should enforce the declared output shape, retain downstream validation and add a regression at the public handler seam. Successful EN/PT baseline and Jev-prefixed workflow timings should then use unmodified live output. This production fix and new test seam await the user's direction. Henrique has approved the Jev provider/architecture proposal, recorded in [ADR 0001](../adr/0001-stateless-user-key-backend.md); that approval does not resolve the drafting measurement gap.

## Prepared fix proposal — not applied

Official documentation checked on 2026-10-07 lists structured output support for the fixed [GPT-6 Luna model](https://developers.openai.com/api/docs/models/gpt-6-luna). The [Structured Outputs guide](https://developers.openai.com/api/docs/guides/structured-outputs?api-mode=chat) describes Chat Completions `json_schema` with strict adherence, required properties and `additionalProperties: false` for every object. These are documentation claims; this proposed schema has not been sent to the provider.

The [concrete proposed response format](application-draft-response-format-proposal.json) declares exactly the backend's six fields, with `applicationAnswers` as a string, the three cover-letter fields as strings and the existing allowed closings as an enum. Both objects forbid extra properties and require every declared field. It replaces the existing outbound `json_object` response format; the requested model, reasoning effort, token cap, prompt, user-owned key, call count, timeout and public response contract would stay the same. Backend nonempty, cover-letter and domain checks remain necessary; schema adherence does not establish factual correctness or promise generation success.

If Henrique selects the pending fix-and-measure option, the bounded work is:

1. At the public `NewHandler()` HTTP seam, add a failing-first contract regression that inspects the outbound strict schema through a fake transport. Verify success for the supported string contract and continued rejection of an array-valued `applicationAnswers`, without weakening validation or adding a retry.
2. Apply the response-format change, run the focused Go handler tests and required checks, and independently review the patch.
3. Deliberately run one paired baseline / Jev-prefixed draft probe in each language using the existing fully specified synthetic timing fixtures. This is at most four OpenAI generation calls and two Jev classification calls if both pairs complete. Stop each pair on baseline failure or unsuccessful classification; no automatic retries. Record actual whole-request durations and preserve all failed attempts.
4. Inspect successful output in memory for completeness and source fidelity without persisting generated prose. Do not count a schema-conforming response alone as successful generation or extrapolate two timings into production tail reliability.

The public-handler regression seam and production change remain pending the user's second answer. No new provider calls or production changes were made to prepare this proposal. Issue #29's workflow-runtime criterion remains partial until the measurement succeeds or Henrique explicitly accepts the limitation.

## Subsequent drafting gate closure

Henrique selected fix-and-measure. The [strict-schema fix and successful EN/PT paired measurements](field-validation-draft-fixed-results.md) now resolve the earlier drafting evaluation gap. Pending fix/seam/measurement statements above describe the historical state when those records were written. The provider proposal remains approved; no dependent integration or deployment is claimed.

# Professional-information field validation (#30)

Add professional information uses the provider decision approved in [#29](evaluations/field-validation.md) and [ADR 0001](adr/0001-stateless-user-key-backend.md). Job Posting integration remains #31.

## Contract and gate

`POST /api/profile/ingest` retains its strict `{input, profile}` body and OpenAI key header. It additionally requires the user's `X-TypeSafe-Api-Key`. No client acceptance flag is accepted. After bounded input/Profile validation, the handler classifies the **entire** submitted text before extraction or comparison. TypeSafe receives only `{field, submission}` with pinned `jev-1.13.0` and the evaluation's explicit two-Choice rubric, embedded in `internal/fieldvalidation/questions.json`. The saved Profile goes only to the existing OpenAI comparison projection.

The response adds `decision: {version: 1, field: "professional_information", outcome}`. A non-acceptance decision returns no claims or operations. Semantic outcomes return HTTP 200; classification service failures use HTTP 401/429/502/504. Accepted responses carry the existing proposal fields alongside `decision`. Extraction/comparison failures retain their existing error contract. The browser rejects successful proposal responses without a valid accepted decision.

Shared Go policy and the TypeScript wire contract distinguish `accept`, `request_information` (needs `professional_fact` here), `request_rephrasing`, `reject_attack`, `irrelevant`, `unusable`, and `service_failure` (`key`, `rate_limit`, `timeout`, `outage`, `invalid_output`). Detected attacks override useful facts; uncertain attacks require rephrasing; content sufficiency is considered only after attack routing. Confidence is validated as provider output but is never an acceptance threshold. Unknown categories, wrong models, incomplete/inconsistent probability distributions, oversized responses, and operational failures stop the workflow. No provider fallback, automatic retry, or redirect is followed.

The classifier has a 3-second deadline, within a shared 52-second handler deadline. Existing generation calls retain their 24-second per-call bounds; the browser retains its 55-second bound. Input remains capped at 30,000 UTF-8 bytes, request bodies at 160 KiB and classifier responses at 64 KiB. These are enforced resource bounds, not production latency guarantees.

## UI, terms, and recovery

TypeSafe settings in Add professional information let the person enter or clear their own browser-local key. The disclosure explains TypeSafe as an additional recipient, user-owned keys/credits, subsequent OpenAI processing, and the documented privacy/retention limits. Keys are never embedded in application code or logged.

The accessible, expandable **CareerOS input-use terms** appear beside the field in English and Portuguese. The narrowly scoped rule prohibits submitting instructions intended to redirect, manipulate, or override CareerOS or its processing models; it adds no unrelated legal provisions or account penalties. Confirmed-attack feedback explicitly names the prohibition and terms violation and links to the rule. Uncertain feedback declares neither an actual attack nor a violation.

Rephrasing disables resubmission of unchanged trimmed text. Editing away and back still requires revision; no confirmation or continue-anyway control exists. Every revised submission calls the server gate again. Editing aborts pending processing and invalidates prior proposals. Loading, decision feedback and failures preserve the submitted text. Service failures permit deliberate retry. Validation and proposal review never save the Profile: existing approve/edit/reject, explicit apply, stale-snapshot protection, destinations, and manual section editing remain intact. Input and proposals remain transient.

## Verification

User-agreed seams: public ingestion HTTP handler and Add professional information browser flow. Gate and rephrasing tests failed before implementation, then passed.

- `go test ./...` with `GOCACHE=/tmp/careeros-go-cache`: passed. Handler tests cover a single Java fact, harmless noise, whole-submission blocking, rephrasing, missing information, irrelevant/unusable inputs, direct-call acceptance-flag rejection, missing classifier key, provider failures, malformed distributions/model/answers, bounded requests, exact classifier payload and no Profile mutation. Existing extraction/comparison tests retain controlled acceptance before exercising their original behavior.
- `node_modules/.bin/tsc --noEmit` and Vite production build: passed.
- Full `node --test tests/*.test.mjs`: five test files passed; the unchanged CV preferences test fails on default font size (expected 12, actual 14), also documented in #29. Targeted ingestion checks pass.
- `node tests/ingestion-browser.check.mjs`: passed using controlled HTTP responses, no live provider calls. EN/PT at 390/1440px cover all decision categories, preserved text/Profile, rephrasing and revision revalidation, loading, stale-response cancellation, accepted single facts/noise, missing/malformed decisions, and existing explicit review/apply across empty/populated Profiles. Screenshots were inspected for feedback and narrow layout. Screenshots live in temporary output directories, not the repository.

No live provider calls or deployment verification were performed for this integration. #29's classifier accuracy/timing evidence remains separate from these controlled integration checks. Maximum-input production performance and load reliability remain release checks. Backend responses retain `no-store`; logs remain operational metadata only.

## Review

Two independent code-review agents reviewed the issue patch against starting commit `1434c6a0d928a972cfc60947f23c3a35f7e92445`.

Standards: no documented-rule violations. One optional Primitive Obsession observation: Go `Outcome.Kind/Needs/Reason` remain string fields, allowing invalid combinations in future construction paths; current server constructors and strict client parsing are consistent. This is a possible future type refinement, not a current behavior defect.

Spec: no remaining actionable findings. A formatter-induced TypeScript separator regression was corrected during review, then typechecking, targeted ingestion checks and the final Go/Node/build checks were rerun. Final Node results retain only the unchanged CV preferences failure described above.

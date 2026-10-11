# General CV reliability verification (#65)

Issue #65 belongs to milestone 2, AI quality and efficiency. Implementation uses
`codex/ai-quality-and-efficiency`, fast-forwarded to review baseline
`e1a5c8c25aabec7f9b0e1e6e17c3798aff16a04b` (`origin/main`), and targets `main`.
Native blocker #34 is closed. The user confirmed the production-handler
adapter/client acceptance seams and this review baseline, and separately
explicitly authorized exactly two synthetic OpenAI attempts with no retries.

## Diagnosis and final change

The recorded PT provider output was exactly `summary: []`, `selected: ["f0"]`,
with supported 20% wording. Replaying this decoded output through #34's adapter
still yields HTTP 502 `invalid_output`; an empty summary is never accepted.
The original EN provider response was not captured, so its cause remains unknown.
The EN empty-summary control is an analogue, not a recovered historical response.

Historical generation requested `response_format: {type: "json_object"}` while
the application required a nonempty cited summary. JSON-object mode did not
express that requirement. Commit `1b4661689fdbd9f1a89c9cdbd5270e2de8fd6941`
(merged in #43) already corrected this contract gap: strict `curated_cv_v2`
schema, summary `minItems: 1`, required nonempty source-ID arrays, and an explicit
nonempty-summary instruction for sparse sources. This is a verified contract
mismatch in the old implementation, not proof of the provider's internal reason
for omitting the PT summary. No additional production rewrite was justified.

This patch locks down that existing correction with outbound-contract tests,
a versioned EN/PT reliability corpus, production-handler-to-client acceptance
regressions, and explicit browser checks that invalid results preserve the
accepted CV and manual edits. It introduces no retry, fallback, repair loop,
new fact identity or Profile migration.

The outbound-contract regression was proven red by temporarily restoring the
baseline JSON-object format: `TestSparseSummaryIsRequiredByOutboundProviderContract`
failed with “provider contract permits the baseline empty-summary/missing-citation
failure.” Restoring the existing strict schema made the same test pass.

## Controlled outcomes

`issue-65/controlled.v1.json` was produced by the unchanged #34 CLI and adapter
using `cases.cv-reliability.v1.json` (`cv-reliability-v1`):

| Cases (each EN/PT) | HTTP | Outcome |
| --- | --- | --- |
| Legacy sparse 20% achievement | 200 | Nonempty supported summary, correct selected reference |
| Stable sparse Java skill | 200 | Supported usage, no inferred proficiency |
| Stable heavy 19-role Profile, project, education, certification, language | 200 | Later role selected; employers, dates, qualifications, proficiency and 20% ownership retained |
| Empty summary; missing/unknown/unselected summary reference | 502 | `invalid_output`, one call each |
| Altered metric; protected title rewrite | 502 | `invalid_output`, one call each |
| Refusal; truncated completion; malformed JSON | 502 | `invalid_output`, one call each |

All six acceptance controls have complete curated labels and zero forbidden hits.
All 18 negative controls have expected missing-label/HTTP failures; these are
successful rejection checks, never successful generated CVs. The Node regression
invokes the same keyless Go adapter and consumes fresh HTTP outputs through
`generateCv`, `validateCvResult`, `createCuratedCv` and saved-snapshot parsing.
This integration test requires Go on PATH as well as Node. It runs no inference.

## Authorized live confirmation

The exact production outbound requests were captured through a controlled
transport and inspected before authorization. Each contained only one synthetic
achievement: `Reduced latency by 20%.` or `Reduzi a latência em 20%.`, request-local
`f0`, explicit CV language, and Balanced density. No contact data, real Profile,
evidence excerpts or TypeSafe input was sent. Settings: `gpt-6-luna`, reasoning
`none`, 4,000 output-token ceiling, strict schema, existing 25-second deadline.

An initial CLI invocation stopped before any network request because the shared
harness requires both credential-file arguments. A subsequent invocation used
an unused dummy TypeSafe assignment to satisfy that CLI precondition. Only the
OpenAI credential was used; no TypeSafe credential or call was needed. Exactly
two provider requests ran, with no retry/fallback. `issue-65/live.v1.json` retains
original provider choices, production outputs, usage, timing, source/fixture
hashes and unchanged scorer outcomes.

| Case | HTTP | Exact summary and source | Provider duration | Scorer result |
| --- | --- | --- | --- | --- |
| en-reliability-legacy-metric | 200 | `Reduced latency by 20%.`, `f0` | 2,140.841 ms | 2/2 labels, zero failures |
| pt-reliability-legacy-metric | 200 | `Reduziu a latência em 20%.`, `f0` | 1,515.829 ms | 1/2 labels: missing summary label and unmatched summary unit |

Agent-assisted source inspection: Portuguese changes first-person `Reduzi` to
third-person `Reduziu`, while preserving the action, latency and exact 20% metric.
Both outputs contain exactly one summary and one selected ID, with no new role,
employer, dates, qualification, proficiency or metric. Both pass the actual client
validation/acceptance and snapshot reload path. The exact-text Portuguese label
remains failed in the original report; neither fixtures nor labels were broadened
after inference. This inspection is not human adjudication or a semantic proof.

These two successful sparse attempts demonstrate bounded live success with the
current contract. They do not explain the original EN failure or establish a
provider success rate, heavy-Profile live quality, all-language quality, deployed
route status, real-user acceptance or universal factual safety. User prose review
and explicit CV acceptance remain authoritative.

## Evidence preservation

The original baseline reports, corpus, scorer, adapter and history manifest are
unchanged. Current reports record their own fixture version and source hashes.
Retained originals (SHA-256):

- `live.initial.v1.json`: `e2cf07752fb6a7717647005aaabc57aa5473b1aee671b40f1c26eb4b3616480b`
- `live.remaining.v1.json`: `c98f395d2b76118cc55651f1ee83fe331e2683406880edd258861fab65a24d31`

Use the retained reports themselves and `history/manifest.json` for historical
fixture/scorer identity; current successful attempts never replace old failures.

## Verification

- `GOCACHE=/tmp/careeros-go-cache go test ./...`: passed.
- `npm test`: 123 tests passed, including fresh controlled handler/client outputs
  and retained live-result client acceptance; no inference in ordinary tests.
- `node_modules/.bin/tsc --noEmit`: passed.
- `npm run build`: passed, with existing Vite native-loader/chunk-size advisories.
- `node tests/cv-generation-browser.check.mjs`: passed controlled EN/PT desktop
  and 390px checks, all three densities, sparse and 19-role sources, independent
  UI/CV language, font retention, v1/v2 reload, cancel/stale completion, manual
  edits, rejected outputs, and recoverable review after primary storage failure.
  PDFs have ordered selectable text and A4 dimensions; Detailed heavy content
  correctly exposes four-page overflow. Rendered Portuguese PDF inspected.

No production behavior changed in this patch. No deployment verification or
additional provider attempts were performed. The original EN rejection remains
unexplained; the retained Portuguese exact-label miss remains visible.

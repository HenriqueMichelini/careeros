# Controlled scoped section-review evaluation

Issue #152 adds the `section-review` Lab suite and `profile_review` semantic task.
The frozen branch/review base is `641a65f17906f4abcacec88146946f4ecff23d08`.
Execution uses the production `profile-review.NewHandlerWithClient` with a controlled
transport. Production prompts, support policy and canonical Profile writers are unchanged.
Live execution of this task is explicitly rejected before dispatch.

## Evidence and versions

`cases.v1.json` uses `section-review-fixtures-v1`, with 44 development cases: 22 EN
and 22 PT-BR. SHA-256: `928f87641f3ee7e2310926a6da5aa89548d3271827421c7ad3b77fa7565b8cc6`.
`controlled.v1.json` is the first immutable report snapshot, serialized as
`semantic-quality-report-v2`. SHA-256:
`02cc0a3dbec4a195823f3d9eabea09e8633ac0a43d6c090dabdacbfd5a76985e`.
It retains 44 results, 80 controlled provider dispatches, 20 HTTP 200 responses and
24 HTTP 502 responses. Eight cases are benign acceptance controls; 36 are negative
controls, including deliberately accepted bad wording. The CLI reports 36 label or
contract failures; successful command completion is not a quality pass.

The corpus covers paraphrase, translation, multi-assertion clauses, material omission,
unrelated additions, owner/polarity/strength changes, and valid-reference semantic
mismatch. Positive owner-context and rejected cross-owner references exercise the
actual production envelope. Each required material atom and optional style atom
references an exact UTF-8 byte span in its canonical source fact. Reused source IDs
are exposed across translated cases; this corpus has no held-out generalization claim.
All semantic adjudication remains `pending`.

Scripted support cases include supported/unsupported booleans, unusable, absent,
duplicate and unknown judgments, refusal, truncation and outage. Production support
is binary: the synthetic `uncertain` string violates its schema and yields unavailable
output. It does not measure legitimate semantic uncertainty. Focused public adapter
tests separately interrupt both rewrite and support stages through real cancellation
and deadlines in each language; these are separate test evidence, not extra retained
report rows. Attempted calls and unknown final usage remain explicit.

## Scope and client boundaries

Captured controlled provider request bodies retain exact selected source, fact,
owner and reference envelopes for both rewrite and support. They contain synthetic
content only and omit authentication headers. Every request includes a private
synthetic contact fact outside the selected section; tests assert its exclusion from
provider bodies. This finite canary observation is not a universal privacy proof or
an observation of actual-provider disclosure.

Lab case detail uses the existing opaque `source`, `expected`, `actual` and `review`
contracts. Source includes the original request, spans, pending adjudication, assessor
scenario, expected contract and required stages. Actual contains diagnostics, captured
controlled requests, scripted judgments and independent material/style label facets.
Review stays unavailable because the handler does not observe client Apply or a human
decision. No pending proposal silently mutates the source Profile.

Public client regressions pass actual controlled handler outputs through
`sectionReviewRequest`, `validateSectionProposal` and `applySectionProposal`. They
verify exact scoped envelopes, explicit Apply, stale-source rejection and preservation
of unrelated facts. Existing browser regressions exercise review/edit/reject/Apply,
quota, cross-tab changes, cancellation and stale proposals at 390 and 1440 pixels in
both languages. Scripted test Apply is separate evidence from human acceptance.

## Metrics and missingness

M03/M05/M17 count finite material-clause label opportunities by case, language and
selected field. Optional style summaries have their own score and cannot dilute
material precision/completeness. Missing or failed output is unavailable, never a
passing zero-denominator precision. Regex matches and scripted enforcement do not
establish semantic support. M18 retains observed execution progress; M19 remains
unavailable when causal failure attribution is absent.

M04/M09–M12/M16/M25 carry explicit unavailable reasons: pending semantic adjudication,
conflated structural/support failures, no independently adjudicated assessor labels,
binary support schema, no handler-side client Apply and no observed human decision.
M28 counts exposed synthetic contact/key-in-body canaries per captured controlled
request envelope. Metrics retain denominators, scope and estimator limitations.

## Reproduce and verify

Use only controlled synthetic inputs:

```sh
GOCACHE=/tmp/careeros-go-cache go run ./scripts/semantic-quality --mode controlled --split all --fixtures docs/evaluations/section-review/cases.v1.json --output /tmp/section-review-candidate.json
GOCACHE=/tmp/careeros-go-cache go test ./...
GOCACHE=/tmp/careeros-go-cache go test -race ./internal/semanticeval ./internal/evaluationlab
npx tsc --noEmit
GOCACHE=/tmp/careeros-go-cache npm test
node tests/section-review-browser.check.mjs
```

Write candidate reports to a new path; preserve the retained report byte-for-byte.
No credentials, external providers, deployment, real Profiles, live calibration or
human label certification were used. The later HTTP/dashboard integration must add
the `section-review` fixture asset to its fixed launcher map; this slice is based on
the shared runner/history commit before that launcher exists.

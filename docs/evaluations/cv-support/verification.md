# CV support review — end-to-end activation verification (#128)

ADR: [`docs/adr/0003-general-cv-statement-support.md`](../../adr/0003-general-cv-statement-support.md) (line 73).
Parent epic: [#122](https://github.com/HenriqueMichelini/careeros/issues/122). This slice: [#128](https://github.com/HenriqueMichelini/careeros/issues/128).

## Scope and non-claims

This report records a **controlled** end-to-end verification that the accepted CV
integrity policy holds across the public handler, the browser review gate,
explicit acceptance, persistence and reload. Every response is a synthetic
fixture served by a keyless controlled transport. There are:

- no live provider calls, credentials, deployment changes, retries/fallbacks or
  new paid provider behavior;
- no claim about provider (assessor or generator) quality, accuracy or latency;
- no new PDF claim (the existing controlled PDF/one-page coverage is unchanged);
- no change to ingestion, qualification, pre-Jev screening, observability (#80),
  retrieval (#96) or the overall release gate (#52).

The coordinated activation in [#128] is included: the production client now sends
`statementSupport: true` (`CV_SUPPORT_REQUEST = true` in `src/lib/cvGeneration.ts`),
so every deliberate generation is assessed and a response without the requested
assessment is rejected as `invalid_output` instead of being persisted as
machine-verified wording.

## Activation contract

- Outbound: every production generation request carries `statementSupport: true`.
- Inbound: a response is accepted only when `schemaVersion === 3`, the generated
  content passes `validateCvResult`, and `validateCvSupport` accepts exactly one
  assessment record per review unit.
- A legacy (v2) or otherwise unassessed response is rejected (`invalid_output`),
  never silently accepted. This is verified directly in
  `tests/cv-generation.test.mjs` and through the activated HTTP path in the
  integration and browser checks below.

## Corpus

`docs/evaluations/cv-support/cases.v1.json` (`version: cv-support-fixtures-v1`,
fixture hash `009c6ff0b7981b3bcb77bb12ea7ed73005a5544d54ed4cae4ec2530e20899122`),
14 synthetic cases — 7 EN and 7 PT-BR. The retained reproduction and the benign
cases are the `development` split; the new held-out ordinary claims, the
cross-language translations and the hard-guard control are the `heldout` split:

| Case (EN / PT-BR) | Retained fact | Machine sentence | Controlled judgment |
| --- | --- | --- | --- |
| `*-support-kubernetes-unsupported` | `Java` | Kubernetes platforms | `unsupported` (retained #78 reproduction) |
| `*-support-responsibility-unsupported` | `Filed routine reports.` | owns company-wide strategy | `unsupported` |
| `*-support-qualification-unsupported` | `BSc Design` | postgraduate Computer Science | `unsupported` |
| `*-support-paraphrase-supported` | `Reduced latency by 20%.` | faithful same-language paraphrase | `supported` |
| `*-support-translation-supported` | `Reduced latency by 20%.` (EN ⇄ PT) | faithful cross-language translation | `supported` |
| `*-support-assessment-unavailable` | `Research` | `Uses research.` | assessment transport `HTTP 500` → `assessment: unavailable`, all `uncertain` |
| `*-support-hard-guard-cross-owner` | two achievements across two roles | joined claim | deterministic hard guard → `HTTP 502`, no assessment call |

The Go handler is exercised through the existing controlled pipeline:

```
go run ./scripts/semantic-quality --mode controlled \
  --fixtures docs/evaluations/cv-support/cases.v1.json --split all \
  --output /tmp/cv-support.json
```

## Method

`tests/cv-support-integration.test.mjs` runs the corpus through the real Go
handler via the keyless controlled transport, then feeds each handler `output`
back through the **real** transpiled client (`src/lib/cvGeneration.ts`):
`generateCv` → `cvReviewUnits` / `cvReviewResolved` → `resolveCvReview` /
`acceptCvResult` → `createCuratedCv` → `parseCuratedCv` → `cvWordingTrust` /
`cvSupportStatus`. Facts are projected from a fully valid synthetic canonical
`ProfileDocument` so the browser-local canonical Profile and its references
travel unchanged.

`tests/cv-generation-browser.check.mjs` adds the same retained Java-only →
Kubernetes case in EN and PT-BR against a valid synthetic canonical Profile,
covering the desktop and 390px keyboard-accessible flow, review removal,
acceptance, save/reload and the absence of any Profile mutation.

The expected HTTP status, call count and per-unit judgment for every case are
declared by the fixture's `expectations` map, independent of the handler. The
integration test asserts the returned assessment and each unit status against
that map before choosing the acceptance path and counting false
acceptances/rejections, so a regression that returned a different judgment fails
the check instead of being scored as a clean run. A mutation that promoted every
returned judgment to `supported` fails on the first fixture-expectation assertion.

## Results

### False acceptances / false rejections

- **False acceptances: 0.** No unsupported or uncertain machine sentence was
  accepted as-is. `cvReviewResolved(result, emptyCvReview)` was `false`,
  `acceptCvResult` threw `unresolved_output`, and `createCuratedCv` without
  review decisions threw for every unresolved case, in both locales.
- **False rejections: 0.** The benign supported paraphrase and the
  cross-language translations in both directions remained usable: empty-review
  acceptance succeeded and the accepted `version: 3` snapshot read
  `cvWordingTrust === "assessed"` with `cvSupportStatus === "current"`.

### Call counts

| Outcome | Provider calls | Note |
| --- | --- | --- |
| Acceptance case (supported paraphrase/translation / unsupported / unavailable) | 2 | one generation call + one assessment call |
| Cross-owner hard guard (`negative_control`) | 1 | deterministic guard fails before any assessment |

### Cancellation and storage failure

These were exercised on the activated path, not just in isolation:

- the integration check drives a pending generation and aborts it; `generateCv`
  rejects `cancelled`, accepts nothing, and the aborted request still carried
  `statementSupport: true`;
- the existing browser sections in the same activated build cover cancellation of
  a pending proposal and a failed `careeros_curated_cv_v1` write (quota error)
  that leaves the previous accepted CV intact while the reviewed proposal stays
  recoverable.

### Follow-through on an unresolved unit

The gate blocks the machine claim without dead-ending the user:

- unsupported ordinary claim → an explicit user edit resolves review and yields a
  `version: 3` snapshot whose provenance records the edit with the original
  `unsupported` judgment as history; the unsupported sentence never persists;
- unavailable assessment (fail-closed, all `uncertain`) → explicit user
  confirmation resolves review and yields `version: 3` with `user_confirmed`
  provenance; nothing is promoted to machine-supported.

### Browser (desktop + 390px keyboard)

For EN and PT-BR against a valid synthetic canonical Profile whose only skill is
`Java`:

- the activated client sends `statementSupport: true` and the retained case starts
  from a Java-only skill fact;
- the unsupported Kubernetes machine sentence is surfaced for review and the
  Accept control is disabled;
- the review flow fits at 390px with no horizontal overflow, the native review
  toggle is keyboard-focusable, Enter removes the unsupported sentence, and Accept
  is keyboard-reachable afterwards;
- the accepted snapshot is `version: 3`, retains the supported sentence, excludes
  the unsupported claim, and records the removal as provenance history;
- save/reload preserves the reviewed accepted summary and the unsupported claim is
  never rendered;
- the canonical Profile is byte-identical before and after generation and review.

## Observed synthetic boundaries

- The runner reports the held-out unsupported cases with precision `0.5` and
  `unmatched: summary[0]`: the machine sentence is a deliberate known-bad output,
  so the clause-level score flags it while the client gate blocks it. This is the
  intended boundary, not a passing-sentence claim. Across the 14 cases the runner
  reports 8 with label or contract failures — the six unsupported claims and the
  two hard-guard controls.
- The cross-owner negative control fails `HTTP 502` with exactly one call, so no
  machine wording exists to assess, return or accept.
- Judgments are scripted, so the assessment classification is fixed by the
  fixture, not observed from a model. The tests establish **enforcement**, not
  assessor accuracy.
- The cross-language translation cases use scripted `supported` judgments; they
  verify the gate keeps a faithful translation usable, not translation quality.
- Existing legacy-snapshot (`unverified`) coverage in
  `tests/cv-generation-browser.check.mjs` (#125–#127 sections) runs unchanged in
  the same activated build, so it is exercised on the activated client.

## Limitations as input to #52 (do not edit or complete #52)

1. No live-provider evidence: neither generator nor assessor quality, accuracy,
   latency or cost is measured here.
2. Fixture judgments are synthetic; real paraphrase/qualification/technology
   classification correctness is unverified.
3. No deployment/route verification; the checks run against the local production
   handler and the in-process client/browser only.
4. No claim that the deterministic guards plus assessment constitute semantic
   proof; they are enforcement seams.
5. The 390px/keyboard verification is one controlled flow, not a full
   accessibility audit.
6. Pre-existing lab note: `go test ./...` has failures in
   `internal/evaluationlab` because the pinned isolated-source manifest
   `docs/evaluations/test-adapters/cases.v2.json` predates the `src/lib/cvGeneration.ts`
   and CV test changes already merged on the milestone branch (#124–#127). This is
   unrelated to #128 and is left for a separate manifest re-approval; all other Go
   packages pass.

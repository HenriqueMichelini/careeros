# Stable Profile sources for general CVs (#43)

Implemented on `codex/evidence-backed-cvs-and-applications`, fast-forwarded from its original base to current `main` (`8c9e3ab`) before implementation. Native blockers #34, #36 and #27 were confirmed closed; issue #43 belongs to milestone 4, Evidence-backed CVs and applications. The PR targets main. #65 remains a separate live-reliability correction.

## Verified behavior

New generation projects stable fact/revision and owner/context references with original semantic qualifiers. Whole legacy blocks stay intact. The fixed provider schema and domain guards support multi-fact summaries, reject unsupported metrics/references and empty summaries, protect metadata and prevent contextual aspirations/unknowns becoming qualifications. The explicit outbound allowlist excludes contact, compensation, unrelated fields and accepted evidence excerpts.

The user reviews original facts, accepted evidence and origins beside proposed wording before explicit replacement. New version-2 CVs independently retain original source revisions/evidence, summary links, protected context, language, density and edits. Changes through the canonical Profile edit API make live support stale without changing the saved CV. Version-1 snapshots keep their original content and edits without guessing a live positional-ID mapping. Source links disclose traceability, not semantic proof. Local saved context also excludes unrelated private Profile fields; a regression caught and fixed root-owner overcapture.

## Checks

- `npm test`: 107 tests passed, including stable identity, whole-block preservation, source snapshots/evidence, stale support, combined facts, contextual aspirations, outbound privacy/bounds, implicit qualification guards and local snapshot minimization.
- `GOCACHE=/tmp/careeros-go-cache go test ./...`: passed.
- `node_modules/.bin/tsc --noEmit`: passed.
- `npm run build`: passed. Existing Vite native-loader and chunk-size advisories remain.
- `node tests/cv-generation-browser.check.mjs`: controlled EN/PT desktop/390px; proposal and accepted evidence/origins, local-only evidence, independent v1/v2 reload/manual edits, canonical changed facts, stale/cancel/error/storage-failure paths, sparse/heavy sources and all three densities. Heavy Detailed documents expose actual overflow; normal export is blocked. No real inference.
- `node tests/cv-pdf.check.mjs`: EN/PT/blank at 12/14/16px, A4 and ordered selectable text; overflowing documents at 390px never call print.
- `node tests/cv-shared-preview.check.mjs`: passed; CV and Apply use A4 previews with matching reading order, independent document language, preserved literal content and correct overflow/export behavior.
- Visual inspection: rendered EN/PT PDF pages and 390px CV controls/preview have no clipping, overlap or horizontal layout defects. Representative PNGs are in `issue-43/`.

Historical browser fixtures were aligned to current UI contracts: Save as PDF label, 14px invalid-preference default, synthetic TypeSafe key for Apply, independent UI/CV language, whole-block whitespace, and the current Generate CV location. The existing saved/pending density notice was restored without changing density or fit policy.

## Semantic evidence limits

`issue-43/semantic-existing.controlled.json` runs all 34 frozen v2 cases through production handlers. Eight cases deliberately expose known capacity loss or adversarial scorer failures, as recorded in the unchanged baseline; none are newly claimed successes. Both existing CV metric/negation acceptance fixtures pass in EN/PT.

`cases.stable-cv.v1.json` and `issue-43/semantic-stable.controlled.json` add eight migration controls: two EN/PT supported acceptances (HTTP 200, complete labels) and six expected HTTP 502 rejections for empty summary, unknown reference and unsupported metric. Missing-label/contract failures in those negative controls are expected rejection evidence, not quality certification. The Go regression validates corpus metadata and checks every expected HTTP status.

Original #34 fixture/scorer versions and live reports remain unchanged. The captured Portuguese empty-summary response is still rejected; the original uncaptured English response has no established root cause. Controlled output, a stricter schema and a nonempty-summary instruction do not establish successful live generation. No credentials or paid providers were used, and deployed behavior was not verified.

## Independent review

The implement skill's two reviews inspected the immutable implementation against starting commit `8c9e3ab`. Standards: no actionable findings. Spec: one low-severity omission of evidence origins in both inspectors, corrected and independently rechecked. The snapshot privacy follow-up was also independently reviewed; no findings remain.

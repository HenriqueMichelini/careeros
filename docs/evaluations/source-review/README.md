# Source review and exposure v1

Issue #154 adds a bounded private review workflow for synthetic native Lab runs. It changes measurement and provenance only. Original outputs, gold, scores and run label hashes remain immutable. The production stateless backend is unchanged.

## Worksheet workflow

Open a terminal synthetic case, choose **Create pending source review**, then **Export source review**. Edit annotations offline and import against the same case. An edited owned worksheet appends a pending identity/revision. Base64 `binding` bytes preserve original source/output/gold JSON exactly. Do not edit binding, context hash or retained verdict projections.

Annotations carry reviewer ID/type (`human`, `agent`, `model`), exact JSON-pointer text spans using UTF-8 byte offsets, owner/context, polarity/certainty/intent/time/qualifier, materiality, proposed verdict and rationale. Multiple independent reviewers are retained. Omission proposals use `outputAbsent: true`, an empty output span and `proposedVerdict: "omission"`; they remain pending. Dimensions are `statement-support` (default), `meaning-preservation`, `retrieval-relevance`, `qualification-sufficiency`. Relevance never certifies sufficiency.

Retained scorer/provider verdicts project the original bound output and reject changes. Reviewer-supplied raw verdicts are marked `reviewer-supplied-unverified`; they cannot replace retained evidence. Missing judgments stay absent.

Human adjudication is disabled by default. Henrique or a designated domain reviewer explicitly launches `npm run lab -- --human-reviewer Henrique`. The reviewer inspects the exact worksheet, prepares adjudication, types its displayed hash-bound confirmation and supplies overall rationale plus decisions for every annotation. Decisions contain `annotationId`, `verdict`, `rationale`; verdicts are `supported`, `unsupported`, `omission`, `forbidden-implication`, `unresolved`. No decisions are preselected. Unresolved decisions keep the appended version pending.

This records an explicit local human attestation, not external identity verification. Agents must not impersonate a human or submit approval on their behalf. Tests use `controlled-test` origin, which cannot qualify as real human gold. A finalized version appends parent ID/hash, receipt, rationale and content-addressed derived label version. Foreign reviewed imports become pending, retaining the claimed label version separately with unverified receipts. Only canonical content matching a version already owned by this private store retains its existing receipt; JSON formatting does not change that match.

The measurement-only report shows classifications and proposal disagreements separately from original scores. Selected annotations are not an exhaustive atom population, assessor accuracy, user acceptance or observed review burden. M03–M07, M10–M15, M25 and M27 are explicitly unavailable where their required populations/observations are missing. Existing comparison pairing rejects unsupported label-version mixing. No release thresholds or #52 gate change; zero forbidden factual additions remains the curated integrity requirement.

## Exposure ledger

The private ledger survives individual run deletion. Immutable declarations retain source-content SHA-256, historical name, split (`development`, `exposed-regression`, `newly-authored-blinded`, `unknown`), language, authorship and translated-pair/exposure group. Declarations are supplied provenance, not proof of authorship. Historical/exposed translations must never be presented as fresh balanced benchmarks. Reuse #139 when delivered; #96 stays research-only.

HTTP case/full-run views, worksheet disclosures and content exports record exposure before returning content. Metadata-only `/view` and diagnostics omit source/output/gold. Browser sessions have `unknown` actor type. All translated group members share exposure, including declarations made after an access. Any recorded access makes `untouched` false and effective split `exposed-regression` while preserving the historical name. Unknown/development entries never qualify as untouched holdouts. Prompt/model development use is recorded explicitly.

An exposure write failure blocks content. Failed-run recovery metadata remains readable with content withheld. Source hashes use exact retained `source` JSON bytes; a later declaration cannot restore untouched status. The ledger tracks this Lab's observed access, not external file access, exposure in another store or human memory. Catalog split names remain historical fixture metadata, not untouched claims.

## Authenticated loopback API

Existing session authorization and mutation Origin/CSRF guards apply. No remote paths, credentials, provider calls or storage are accepted.

- `POST /lab/api/runs/{run}/cases/{case}/worksheet` with `{}` creates pending evidence.
- `GET /lab/api/runs/{run}/worksheets/{worksheet}` exports it.
- `POST /lab/api/runs/{run}/cases/{case}/worksheet/import` accepts base64 `worksheetBytes`.
- `POST .../worksheets/{worksheet}/propose` appends an annotation.
- `GET .../worksheets/{worksheet}/approval` returns exact human preflight; `POST .../adjudicate` binds actor, worksheet/context hashes, confirmation, rationale and decisions.
- `GET .../worksheets/{worksheet}/report` derives the measurement-only report.
- `POST /lab/api/cohorts` registers a `CohortEntry`; `GET /lab/api/cohorts/{sourceHash}` reads current paired status.
- `POST /lab/api/exposures` records `contentHash`, `actor: {id,type}`, and `use`: `view`, `export`, `worksheet`, `prompt-development`, `model-development`.

Bounds: 1 MiB worksheet; 512 KiB bound original bytes; 128 annotations/adjudications; 8 KiB annotation; 256 revisions/files per run. Ledger: 4 MiB, 4,096 cohorts, 16,384 events. Private permissions, guarded paths and atomic writes apply. Capacity exhaustion blocks further content disclosure without discarding history.

## Controlled examples and verification

`examples/regex-mismatch.pending.v1.json` shows a strength dispute pending human review; `agent-proposal.pending.v1.json` shows a source-linked agent proposal. `controlled-human-protocol.reviewed.v1.json` and `measurement-only.v1.json` come from the actual controlled HTTP approval test. Their actor is a controlled example, not Henrique, real domain judgment or benchmark gold. No live calibration/model-quality claim is made.

Deliberate separate regeneration (new IDs):

```sh
LAB_REVIEW_EVIDENCE_DIR=/tmp/lab-review-examples GOCACHE=/tmp/careeros-go-cache go test ./internal/evaluationlab -run 'Test(SourceReviewRoundTrip|ExplicitHumanCorrection)' -count=1
```

Frozen base: `1ae4daee77f3c9bc6702d4e99709c31329119098`. Public-seam tests cover reviewed/pending round-trips, source/output/label rebinding rejection, foreign authority downgrade, append-only corrections, omissions, paired exposure/restart, metadata-only viewing and exposure-write failure. The actual standalone browser covers worksheet creation/export plus the existing isolated Lab workflows. No real human labeling is required for technical completion.

# Ingestion endpoint evaluation

`internal/evaluationlab` provides two pure offline boundaries:

- `EvaluateIngestionEndpoint(IngestionEndpointRequest)` derives a `lab-endpoint-evaluation-v1` receipt.
- `ReevaluateIngestionPair(history, requests)` returns independent copies of the existing history with two new linked receipts appended. File persistence is separate work.

The request carries the frozen task sidecar, experiment/variant/run/snapshot binding, typed legacy or staged producer evidence, and resolver-owned evaluator dependency identities. An optional expected evaluator reference enforces the frozen common evaluator. Both producer contracts use the same evaluator; its identity does not vary with a case's gold or forbidden patterns.

Evaluation verifies task input, expectation and original fixture hashes, re-extracts the sidecar from the retained fixture, and reprojects producer bytes through the existing mechanical normalizers. A supplied normalized observation must match that reprojection. The observation digest canonicalizes independent issue/reason sets while preserving producer row, raw response, source text and array order. The raw-output digest covers the typed producer evidence envelope, including response roles, IDs, statuses and exact body bytes. Normalization never receives gold.

The evaluator manifest pins its version, existing `semanticeval.Score` contract, observation version, exact facet projections, pattern source and dependency hashes. Required identities include the complete captured build source envelope (`build-source-set`, the `BuildManifest.Hash` from `CaptureBuildManifest`) and the named scorer, normalizer, task-sidecar, metric and evaluator source components. Hashes are resolved from owned source/build artifacts; this pure function does not read files or attest that supplied identities correspond to an executing binary. Build correspondence and trusted dependency resolution remain separate requirements. Missing identities make measurement unavailable; malformed identities and known contradictions are rejected.

The finite curated measures have separate units and denominators:

| Receipt field | Selected observed units | Frozen labels |
| --- | --- | --- |
| `Facets["claim-diagnostics"]` | `claims.text`, `claims.targets`, `claims.question` | Original `claims` atoms |
| `Facets["proposal-operations"]` | `operations.value`, `target`, `field`, `entryId`, `action` | Original `operations` atoms |
| `CuratedForbiddenAdditions` | Both complete selected scorer-unit projections above | Original forbidden patterns, unchanged |
| `GeneratedWordingForbiddenAdditions` | Objects containing only `claims.text` or `operations.value` | The same unchanged patterns, separately applied |

Source excerpts, claim IDs, provenance, source references, disposition reasons and raw metadata never enter those scorer units. Questions and owner/destination fields are retained only in the selected curated diagnostic/matching units. A pattern found in an owner ID or question can increment the full selected-unit diagnostic without becoming a generated-wording hit. Existing explicit structural prohibitions, such as a forbidden `target` or `field`, remain visible in the full selected-unit diagnostic. The two forbidden counts must not be pooled or described as an overall safety judgment. The receipt separately binds the exact pattern hash and pattern count to its expectation package.

Counts reuse `semanticeval.Score`; metrics reuse `CountMetric`. Zero opportunities remain unavailable rather than a perfect ratio. Required gold counts, evaluated coverage, missing evidence and curated omissions remain distinct. An absent, failed, partial or structurally unusable endpoint has no numeric quality counts or forbidden count. Its known required opportunities are listed separately as `NotRecovered`: this means no qualifying endpoint evaluation, not a semantic claim that every fact was omitted. Missing task dependencies also prevent quality scoring. Unsupported gold groups, unprojected matching fields or boundary invariants make the scope unavailable; malformed or duplicate labels are rejected.

Re-evaluation requires the same paired task inputs, cohort metadata, exact retained raw outputs and execution bindings. Both requests must use the same revised expectations and evaluator; known differences are rejected even when other evidence is missing. New receipts link their predecessors by hash. Missing output or dependencies produce unavailable receipts without rewriting earlier evidence or making the surviving counterpart a paired quality result. Editing a prior receipt or replacing retained raw bytes is rejected. No handler, provider, automatic retry, storage mutation, comparison winner or acceptance threshold is introduced.

All metrics are labeled `offline-derived`. `HumanReview` stays `pending` and `SemanticReview` stays `unavailable`. Fixed synthetic omission, owner, stronger-wording and forbidden-addition probes establish finite scorer behavior only. They do not establish independent human annotation, general semantic accuracy, receipt authenticity, `no-store`, user acceptance, Apply/Save, live-provider quality or release acceptance.

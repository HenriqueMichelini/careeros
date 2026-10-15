# Local Evaluation Lab contracts

Issue #146 introduces pure Go contracts in `internal/evaluationlab`. No launcher,
HTTP server, history store, provider execution or production UI is added here.
Those remain the subsequent Lab slices. Existing serializers and retained
reports are unchanged.

## Versions and boundaries

- `lab-suite-v1` catalogs the semantic, settings, qualification-evidence and
  application-context suites. It declares modes, fixed configuration IDs,
  approved synthetic sensitivity, assertion limits, fixture/case hashes,
  language/split/exposure metadata and conservative required-leg/call caps.
  Application Context has 23 logical cases (13 retrieval plus ten output replay
  cases, with 40 field observations). Maximum calls include simulated calls.
- `lab-result-v1` separates engine state, assertion verdict, operational category,
  source/dirty/dependency provenance, context/order/budgets/concurrency, logical
  case/child/dispatch identities, approved artifact references and durability.
  Engine completion may accompany failed assertions. Unknown trust labels are
  not engine states. Planning/admission and actual durable writes are later work.
- `lab-import-v1` identifies a new offline derivation. Original report version,
  bytes, SHA-256, mode/timestamp, source hashes and derivation references remain
  independent. `lab-retained-assertions-v1` interprets recorded failure prefixes
  only when the exact inference-era fixture is supplied. It does not rerun or
  silently replace the original scorer.
- `lab-metrics-v1` records the M01–M29 definitions from #146. Named observed
  measures are distinct from a full general metric capability. There is no
  overall AI score. Unsupported opportunities/labels remain unavailable.
- `lab-comparison-v1` checks fixture/label, source, prompt, schema, scorer,
  strategy, budget, runtime, mode and declared changed dimension. Missing fields,
  unintended changes, ambiguous repeated rows and partial case pairs prevent
  compatibility/pooling. Complete pairs expose curated forbidden-addition,
  missing-label and relationship-error deltas plus assertion/outcome disagreement.
  Metadata equality is a declared compatibility check, not semantic proof.

Historical paths are provenance text, never filesystem requests. Catalog case
hashes use sorted, compact JSON with original Unicode (Python `json.dumps`,
`sort_keys=True, separators=(',', ':'), ensure_ascii=False`). Fixture hashes bind
the original file bytes. Unknown historical pairing is explicitly marked; it
does not establish independent sampling units. EN and original PT/normalized
PT-BR remain separate, and translated/configuration repetitions remain dependent.

## Import and sensitivity

`Import` accepts bytes only: at most 8 MiB and 32 JSON nesting levels. Malformed,
concatenated, deep and oversized JSON is rejected. Only the exact registered
synthetic snapshots in `retained.go` are normalized; all other valid JSON is
quarantined with original bytes/hash and no cases. A recognized-looking schema,
an uploaded sensitivity claim, path, executable, provider URL, support label or
live flag cannot register evidence or authorize execution. Consumers render
content as inert data; importing never evaluates HTML or follows artifact paths.

`ImportWithDependencies` accepts at most 16 approved fixture byte strings, within
the same total byte/depth bound. Each must be registered, unique and match the
report's inference-era fixture hash. It joins exact source/gold, failure-family,
split and exposure fields, including sequence leaves. No current-fixture fallback
is allowed. Without a dependency, those details remain unavailable; original
recorded scores remain inspectable. Returned bytes are copied, and derived metric
and ledger functions revalidate originals/dependencies rather than trusting
caller-modified normalized fields. Persistence immutability belongs to #148.

Supported retained shapes:

| Format | Registered snapshots | Observations |
| --- | --- | --- |
| semantic-quality-report-v1 | controlled, initial/final/remaining live | original runs, calls, outputs, scores |
| semantic-quality-report-v2 | controlled, offline live replay | original runs plus derivation/source metadata |
| workflow-settings-v1 | controlled, live, reviewed live | 42 logical roots; review annotations retained separately |
| diagnostics legacy v1 | controlled diagnostics | 34 attempts; nullable usage and stage outcomes |
| qualification-evidence legacy v1 | controlled report | 39 retrieval rows plus 13 final matching rows |
| application-context legacy v1 | controlled report | 13 retrieval rows plus 40 field rows |

Unversioned retained formats receive an explicit normalizer format ID; no version
is invented for the original artifact. Reviewed settings keep their live inference
timestamp/origin and original `derivedFromSHA256`, with a separate offline source
review derivation. Original `pt` is retained beside normalized `pt-BR`.

The v2 semantic reports reference fixture SHA-256
`885d652fe45d554cd8a757990e0ae1231601a48ac7eca6368945570422af38ab`, whereas
today's `cases.v2.json` is `b46dc852078c0e76fe9e0eeddcdba09514a41c75e43967383a4fdf5a08af3550`.
`history/cases.inference.v2.json` preserves exact bytes recovered from commit
`d0d974cfd09b53d99791ececeb35e4d5b79d380e`, without modifying either report or
the current fixture. Historical missing dirty-tree/runtime/embedded-dependency
coverage remains explicit and must prevent unsupported comparisons.

## Metrics, usage and comparisons

Available named derivations include curated label precision/completeness/omission,
controlled final-match state agreement, unique post-budget relevant/negative
retention, leaf terminal categories, logical/HTTP/provider counts, token totals
with coverage, observed cache-hit rate, individual dispatch latency and dated
known-subtotal/full-cohort cost. Curated matches are not general semantic labels.
Field rows retain their field scope; failure families/exposure metadata are joined
only from exact fixtures. Other M01–M29 capabilities remain unavailable with
reasons, including calibrated assessor accuracy, real user review burden,
durability/acceptance observations and ranked MRR/nDCG without reviewed traces.

Zero opportunities produce a null value with denominator zero, while raw historical
empty-denominator scores are unchanged. Failed, incomplete and unreached outputs
cannot acquire passing quality ratios or deltas. An expected-negative assertion
can pass while raw quality defects and noncompletion remain visible.

Retrieval uses unique labeled IDs at each scope/byte budget. The Application
Context denominator is 15 relevant facts across 13 cases; historical prose's 16
is not used. Negative retention, budget/relevance exclusions and language remain
separate. These are fact-retention measures, not rank/group Recall@K.

`DispatchLedger` uses the existing flattened sequence call ledger once and assigns
stable import-derived identities. Parents do not create extra calls. Unknown final
usage remains null. `SummarizeDispatches` rejects duplicate identities and mixed
evidence origins, retains raw usage, flags invalid subset relationships, and
excludes inconsistent components from derived sums/pricing. Configured and returned
model identities remain distinct in raw call evidence. Simulation is explicitly
labeled; it is not provider-observed resource use.

Pricing reuses the existing exact-model dated-price calculation. Failed/canceled
calls remain in the numerator; any unknown/unpriced call keeps the full total null.
`ResultCosts` requires a named dispatch cohort; accepted-result costs additionally
require a count and explicit acceptance-evidence kind. Completion/HTTP/scorer output
cannot supply acceptance. Estimates are not billing evidence or current pricing.

Comparisons never pool partial cohorts, languages or translated pairs. Historical
metadata gaps must be listed rather than filled from today's checkout. Controlled,
replayed, provider, browser, deployed and actual-PDF observations stay distinct.

## Verification

Public-interface tests cover all supported retained shapes, byte ownership/hashes,
original origin/language/review chains, exact dependency joins and mismatch
rejection, expected negatives, nested sequence leaves, unknown/quarantined inputs,
JSON bounds, selection validation, distinct execution/assertion states, empty
opportunities, 15 unique relevant facts, negative retention/exclusions, metadata
confounders/partial comparisons, once-only dispatch accounting, missing usage,
inconsistent subsets, named acceptance costs and partial dated pricing.

No providers, credentials, real Profiles, remote storage, telemetry or deployment
verification are used. This slice does not claim the Lab runner or UI is delivered.

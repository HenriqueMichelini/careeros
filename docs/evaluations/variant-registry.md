# Owned variants and comparison capabilities

`ResolveVariant(suiteID, variantID, configurationID)` accepts exact server-owned
catalog IDs and returns an opaque handle with `Descriptor`, `Plan` and `Start`.
The registry holds code functions; descriptors contain metadata only. No API
accepts commands, paths, environments, provider hosts or function names. Unknown
and cross-suite IDs, including the not-yet-delivered
`ingestion-submission-proposal-v1`, are rejected.

The eight registered adapters are seven suite-native adapters plus the first
Profile-ingestion architecture variant:

| Suite | Variant ID | Execution |
| --- | --- | --- |
| semantic-quality | semantic-quality-native-v1 | Controlled workflow handler |
| semantic-quality | ingestion-legacy-proposal-v1 | Controlled legacy Profile ingestion proposal; fixed five-case cohort |
| workflow-settings | workflow-settings-native-v1 | Controlled workflow or deliberate sequence |
| qualification-evidence | qualification-evidence-native-v1 | Offline retrieval and controlled final matching |
| application-context | application-context-native-v1 | Offline retrieval and saved-output field replay |
| section-review | section-review-native-v1 | Controlled section handler |
| cv-integrity-evidence | cv-integrity-evidence-native-v1 | Immutable owner-evidence replay |
| test-adapters | test-adapters-native-v1 | Fixed isolated process selector, subject to existing capability guards |

Model/reasoning configurations remain separate selections. For example,
`workflow-settings-native-v1` supports `current`, `luna-low` and `sol-low` without
changing adapter identity. The registry never accepts arbitrary model settings.
`Plan` resolves the fixed suite/mode/configuration through the existing planner;
`Start` rejects mismatched selections and uses the existing frozen-plan validation,
single active admission slot, cancellation and retention controls. A zero handle
cannot plan or execute. Native plans retain `lab-plan-v1`; they do not invent
experiment execution membership or retrofit variant authority into historical runs.

`ingestion-legacy-proposal-v1` is the only architecture variant with an executable
adapter. It accepts only controlled `semantic-quality` execution with the pinned
`current` configuration and exactly the documented cohort:

1. `en-fact`
2. `pt-fact`
3. `en-negation-aspiration`
4. `en-existing-duplicate`
5. `en-proficiency-conflict`

An omitted case selection or any permutation of exactly those five unique IDs
normalizes to that frozen order. Any other count, unknown ID or duplicate ID is
rejected before planning.
The plan retains five HTTP attempts and the conservative 25-call ceiling. Each
case executes through the unchanged production legacy ingestion handler and the
existing controlled transport; the handler receives a sanitized execution case
with no gold, forbidden patterns, expected failures/status/calls, source spans,
entities, adjudication, sequence-scoring selector or staged steps. The run record
retains the real HTTP status and response bytes plus the mechanical
`NormalizeLegacyIngestion` observation in `lab-ingestion-legacy-execution-v1`.
Every provider call remains a unique `simulated` dispatch. Endpoint evaluation is
a separate offline step over the captured response and observation; it does not
run inside the handler execution.

The variant reports `assertionVerdict: "unavailable"` and an empty
`scorerVersion`. It makes no gold-based assertion, semantic score, Apply/Save,
browser acceptance, real-model quality, provider timing/cost or architecture-winner
claim. Its run and case reasons state the proposal-only scope explicitly.

The controlled execution check is
`go test ./internal/evaluationlab -run TestLegacyIngestionVariantExecutesFixedFiveCaseProposalCohort -count=1 -v`.
It runs all five cases with zero external network attempts, retains HTTP 200
responses and their exact mechanical observations, and records three unique
`simulated` dispatches per case (15 total). It also evaluates the captured
`en-fact` response offline: both endpoint facets report one expected and one
observed unit, human review remains pending and semantic review remains
unavailable. These are controlled synthetic transport observations, not
real-provider or real-model quality evidence.

`Catalog()` adds independent copies of variant descriptors and versioned
`comparisonCapabilities` for all seven suites. Endpoints name native measurement
boundaries and limitations. `strictProjector` distinguishes conditional root-score
support, partial scope and unavailable projectors. In particular, section-review
root scores do not supply material-patch comparisons, retrieval wrappers are not
semantic-score rows, retained CV replay is not another architecture execution, and
process assertions are not semantic quality.

Every `architectureProjector` remains explicitly unavailable with a reason.
`ingestion-submission-proposal-v1` remains unregistered until its adapter and
per-stage scripts are delivered (#211/#212). Metadata neither proves runtime
capability nor attests a build, supplies a common evaluator, authorizes an
experiment or claims an architecture winner. Process isolation/tool/source guards
still decide whether a registered fixed selector can execute on this host. No
historical artifact, strict-comparison rule, import trust boundary or
live-provider capability changes.

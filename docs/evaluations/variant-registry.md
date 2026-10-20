# Owned variants and comparison capabilities

`ResolveVariant(suiteID, variantID, configurationID)` accepts exact server-owned
catalog IDs and returns an opaque handle with `Descriptor`, `Plan` and `Start`.
The registry holds code functions; descriptors contain metadata only. No API
accepts commands, paths, environments, provider hosts or function names. Unknown
and cross-suite IDs, including proposed ingestion architecture IDs, are rejected.

The seven registered adapters preserve existing suite-native behavior:

| Suite | Variant ID | Execution |
| --- | --- | --- |
| semantic-quality | semantic-quality-native-v1 | Controlled workflow handler |
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
cannot plan or execute. These native plans retain `lab-plan-v1`; they do not invent
experiment execution membership or retrofit variant authority into historical runs.

`Catalog()` adds independent copies of variant descriptors and versioned
`comparisonCapabilities` for all seven suites. Endpoints name native measurement
boundaries and limitations. `strictProjector` distinguishes conditional root-score
support, partial scope and unavailable projectors. In particular, section-review
root scores do not supply material-patch comparisons, retrieval wrappers are not
semantic-score rows, retained CV replay is not another architecture execution, and
process assertions are not semantic quality.

Every `architectureProjector` remains explicitly unavailable with a reason.
`ingestion-legacy-proposal-v1` and `ingestion-staged-proposal-v1` remain unregistered
until their adapters are delivered. Metadata neither proves runtime capability nor
attests a build, supplies a common evaluator, authorizes an experiment or claims an
architecture winner. Process isolation/tool/source guards still decide whether a
registered fixed selector can execute on this host. No historical artifact,
strict-comparison rule, import trust boundary or live-provider capability changes.

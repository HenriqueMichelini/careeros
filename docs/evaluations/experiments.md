# Immutable experiment contracts

`FreezeExperiment(definition)` creates a caller-independent `lab-experiment-v1`
definition with a SHA-256 content hash. All references carry an ID, version and
content digest. Input slices, maps and dependency-group pointers are copied.
`CheckExperiment(definition, leftEvidence, rightEvidence)` verifies the frozen
hash, shared evidence, exact variant identities and declared differences. Editing
any definition content requires an explicit new freeze and changes its hash.

These pure contracts neither register nor execute a variant. A structurally
`valid` result is not build attestation, trusted import admission, execution,
semantic quality, an overall winner or causal proof. Callers must resolve
references from approved server-owned artifacts; catalog registration, dependency
closure/build verification, scheduling, storage and endpoint scoring follow in
separate steps. Existing `CheckComparison` and `lab-comparison-v1` keep their
single-dimension rules unchanged.

Definitions support three kinds:

- `single-dimension`: exactly one conceptual treatment. Every additional identity
  change names that exact treatment field in `consequenceOf`, with its own exact
  values and rationale. Cycles, chains and unidentified consequences are rejected.
  A declaration records the proposed dependency relationship; it does not prove
  causality or justify isolated attribution from an opaque source envelope.
- `complete-design`: one or more independent declared changes. Endpoint outcomes
  can later describe the whole design; individual stage effects remain separate.
- `repeatability`: identical variant content and no differences. Variant IDs are
  selection identities, so the same registered variant can occupy both slots.

Variant fields include adapter version and digest references for implementation,
build, configuration, prompts, strategy, producer schema, normalizer and simulated
response script, plus exact named implementation dependencies. A declaration
addresses a scalar such as `promptManifest.hash` or
`dependencies.source/adapter.go.hash`. It provides exact left/right values and a
nonblank rationale. Added or removed dependency components use the literal
`absent`. Wildcards, undeclared changes, duplicate declarations, unchanged fields
and wrong values are rejected. No source wildcard or shared-evidence waiver is
accepted. Differences returned by `CheckExperiment` describe the supplied
observations, including unexpected changes, rather than substituting the frozen
values for them.

The shared task contract, case input/expectation packages, common evaluator and
resource/runtime policy must match the definition. Cohorts reuse `PairTaskCases`
and preserve language, split, translated/dependency and exposure metadata. Every
case has all distinct frozen ordinals from one through `repeatCount` (one to
three); repeated cases keep the same expectation and cohort metadata. Original
full-fixture hashes remain separately retained and may differ because they also
contain variant scripts. No old fixture, report, plan or result is rewritten.

The explicit shared policy fixes equal end-to-end ceilings, runtime, clock
boundary, controlled evidence mode, alternating-pairs policy, concurrency one,
and no retries, fallback or automatic resume. It uses existing planner ceiling
bounds; experiment schedule admission and the first-release 32-pair/64-execution
cap are a later planning responsibility. Different consumed calls or stage
counts are observations and never a comparability requirement here.

Compatibility has three states with machine-readable reasons and affected fields:

- `valid`: complete matching structural evidence and exact declarations.
- `unavailable`: required metadata is missing, including historical records with
  no experiment definition. Equal empty fields never establish equality.
- `invalid`: known drift, malformed identities, unsupported policy, duplicate
  pairs or altered frozen content. Known mismatches take precedence over missing
  evidence; missing input cannot hide a known label change.

An unavailable or invalid result supplies no comparative quality or efficiency
conclusion. This contract does not invent missing execution, evaluator, source or
provider evidence, and performs no provider calls.

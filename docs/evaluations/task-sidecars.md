# Shared task sidecars

`ExtractTaskSidecar(suiteID, fixtureBytes, caseID)` derives a
`lab-task-sidecar-v1` record without changing the original fixture or any retained
report. The first projector supports `semantic-quality-v2` Profile ingestion
cases only. Other suites and workflows return an explicit unsupported error.
This does not advertise executable architecture comparison for any suite.

The record retains a cloned copy and SHA-256 of the complete original asset,
alongside three separate canonical JSON packages:

- Input: the complete request (including original text and initial Profile),
  language/task, and any explicit document, revision and frozen user answers.
- Expectation: gold atoms, forbidden patterns, entities, role, expected failures,
  explicit boundary invariants and case/fixture label revisions when present.
- Simulation script: provider replies, envelopes, statuses and explicit output.

Each package has its own schema version and hash. Script changes cannot change
the input or expectation hash. Input changes cannot regenerate expectations.
These are existing curated fixture labels; extraction does not establish new
human annotation, semantic correctness, or independence of authorship.

`lab-canonical-json-v1` sorts object keys recursively. It preserves string content
without trimming or Unicode normalization, array order, JSON number lexemes,
and absent versus explicit null properties. Original asset bytes remain separate
from canonical identity. Duplicate keys (including escaped equivalents), invalid
UTF-8, unpaired Unicode surrogates, unsupported schemas, unclassified case fields
and ambiguous required shapes are rejected. Historical import validation and its
exact allowlist remain unchanged.

Derivation is an inert operation, not trust admission. `sourceAdmission` is
`approved-synthetic` only when the full asset hash matches both the selected
server-owned suite and the existing approved fixture registry. Candidate edits
are returned as `unregistered`, even when only JSON formatting changed. Future
execution admission must still resolve server-owned approved inputs; a caller
cannot promote candidate bytes by supplying a case name or sidecar.

`PairTaskCases(left, right)` pairs a complete declared cohort by
`(taskVersion, caseId, inputHash, language, repeatOrdinal)`. Distinct positive
repetition ordinals are separate pairs; duplicate keys on either side fail.
Expectation hashes and suite/split/exposure/dependency metadata must agree.
Original full-fixture hashes may differ because they also include scripts.
The result preserves the left cohort order and both original references.
Execution IDs and stage positions never participate. Missing execution results
will belong to later execution/comparison evidence, not a replacement cohort.

Known source groups are preserved. Missing historical relationships remain a
null dependency group with `unknown-historical-pairing` exposure metadata; this
projector does not infer translation relationships from EN/PT case names.
Pairing does not turn repeats or translations into independent samples, and it
does not establish provenance or admit execution. Experiment scheduling, caps,
evaluation receipts and comparison remain separate steps in the issue queue.

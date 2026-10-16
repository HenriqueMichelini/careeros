# Sequential local evaluation runner

Issue #147 connects the #146 contracts to reusable controlled execution. The
runner is an in-memory module; private durable history, loopback HTTP access and
the separate dashboard remain #148–#150. Native execution has no live mode,
credential discovery, generic process launcher or external transport fallback.

## Public boundaries

`NewPlanner` owns copies of exact catalog fixture assets and optional source
provenance. Asset keys are catalog IDs, not filesystem paths. `Plan` fixes the
suite/configuration, exact case order, fixture/dependency hashes, suite/runtime
versions, concurrency one, required HTTP legs, conservative maximum provider
calls and requested resource/deadline caps. Its random identity and canonical
digest are registered in that planner. `ValidatePlan` rejects edits, foreign
plans and changed fixture bytes. Case/HTTP caps must cover the declared selection;
a lower provider cap may deliberately stop execution with partial evidence.

`CaptureSource` snapshots Go and embedded JSON from known implementation
directories, including the Jev rubric and runner/adapters. It rejects symlinks
and oversized/nonregular source files. Compiler VCS/dirty metadata is retained
when available; missing metadata and binary/source-correspondence limitations
are explicit. Launchers supply this snapshot to the planner before planning.
No imported report/browser request supplies source paths.

`NewRunner`, `Start`, `Read`, `Wait`, `Cancel` and `Events` provide atomic admission,
prompt start identity, copied result reads, bounded waits, idempotent cancellation
and bounded monotonic event pages. One run is active per runner. Unknown/altered
plans are rejected before admission. Event gaps and future cursors are explicit;
there is no invented percentage or overlapping-stage timeline. Workflow/dispatch
starts are observed at execution; backend stage events describe terminal stages.

Runs, logical cases, leaf HTTP attempts and dispatches have separate random IDs.
Parents reference observed children. A single native dispatch ledger distinguishes
planned intent, attempted/completed, unknown-final and not-dispatched entries.
Only attempted entries contribute to usage/call totals. Required unreached HTTP
legs remain visible separately from logical-case/assertion outcomes.

## Execution and cancellation

Semantic and settings execution uses the shared `semanticeval.ExecuteBatch` core.
Settings retain case/configuration interleaving and shared budgets. The existing
`Run`/`RunConfigured` entry points delegate to context-aware counterparts with
background contexts, preserving old callers. Replay only reassesses saved responses
and refuses credentials; it does not dispatch inference.

Qualification Evidence and Application Context now expose reusable byte-input
modules. The CLIs keep their flags, default paths, JSON serializers, file modes
and quality/command exit meaning. Application Context uses the same production
handler adapter and controlled transport as the other workflows. Request-size
observation exposes only provider name and byte count, not payload/key content.

Native execution reuses these modules directly. Controlled transport intercepts
every provider request. The runner checks context, HTTP/provider caps, resource
state and caller-owned storage-health feedback before new work/dispatch. Cancellation
propagates through sequence legs, production-handler requests and response reads;
a context callback closes a blocked body. Dispatched canceled attempts retain
unknown final usage. No retries or fallback are performed.

The artifact cap applies to captured case/leaf result bytes; verified fixture
inputs and bounded control metadata have separate fixed limits. Oversized results
are explicitly unavailable with their SHA-256, rather than silently truncated.
A captured leaf that exceeds the cap prevents the next required leg. An unstarted
leg is not appended as an observed HTTP result.

`RunnerOptions.Retain` is a storage integration seam, not a durability assertion.
Dispatch intent is presented before dispatch; observed/partial/terminal records
are presented as execution advances. A retention error becomes sticky: no later
dispatch or automatic write retry occurs. Terminal status is published only after
terminal retention returns, so a failed save cannot briefly appear successful.
The memory runner always reports `durable: false`; #148 owns actual fsync/recovery
acknowledgments and unfinished-run restart handling.

Engine completed/failed/canceled is independent of case pass/fail/blocked/skipped,
operational outcome and durability. Native metrics retain finite curated/replay
measures, distinct progress/call counts, nullable simulated usage and local
dispatch duration. Unmeasured causal/reuse/cost capabilities stay unavailable.
No handler/CLI completion, expected-negative assertion or diagnostic stage proves
semantic correctness, user acceptance or durable save.

## Verification evidence

Frozen review base: `3097eabb3f9133d11d00a4116859df8ab856bfae`.

Pre-extraction CLI implementations were executed with the same current handlers
and frozen fixtures, using only controlled transport. Qualification Evidence
outputs are byte-identical before/after (39 retrieval and 13 final rows), as are
Application Context outputs (13 retrieval and 40 field rows). Semantic/settings
stable report data is equivalent after excluding timing/attempt IDs and newly
recorded source dependencies. Added source hashes include context/configuration/
batch helpers and embedded JSON; this is provenance expansion, not inference drift.

Public tests execute all four catalog suites directly with a denied external
transport canary and observe zero external requests. They cover frozen-plan
tampering, one-active admission, copied public reads, cancellation before dispatch,
blocked-body cancellation, deadlines, provider/artifact caps, required unreached
legs, once-only dispatch identities, storage failure/no retry, terminal publication
ordering, bounded event history/gaps and independent engine/assertion states.
Focused race-enabled checks exercise the concurrent reader/cancel/retention paths.

The runs use approved synthetic assets. There are no real provider, credential,
deployment, browser or actual-PDF observations. The local native results remain
in memory until the separate history slice is delivered.

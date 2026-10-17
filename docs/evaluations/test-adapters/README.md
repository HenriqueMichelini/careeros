# Isolated existing test adapters — LAB-07 / #151

The standalone Lab includes `test-adapters`, with fixed selectors defined in `internal/evaluationlab/process_commands.go`. Launch with `npm run lab`. The composer shows runtime preflight and coverage gaps; selected drivers run sequentially under explicit case, deadline, output and artifact bounds. A command exit is one **test-driver assertion**, never semantic quality, client acceptance or provider success. Nested assertion denominators are unavailable.

## Isolation and fixed inputs

Linux bubblewrap and a Go 1.26+ supervisor with stable process handles (pidfd) are required. Older compiler/runtime environments retain the Go language baseline and fail process preflight. Each driver receives a new private user, network, PID and mount namespace, a private temporary HOME/profile/cache, read-only reviewed source bytes, and fixed runtime mounts. Host HOME, `/run`, host temporary files, personal browser profiles, Git/AI credentials and host networking are absent. There is no unsandboxed fallback. Other platforms or namespace restrictions report unavailable capability. The local launcher itself remains loopback-only.

The immutable `cases.v1.json` manifest lists exact source paths and SHA-256 bytes, case identities, entry points, coverage and gaps. Source copying rejects changed bytes, symlinks, nonregular files and excess sizes. Source total is capped at 32 MiB (8 MiB per file, 4096 files). No imported/browser-provided path, executable, argument, environment or provider host reaches process configuration. Commands use argument arrays without a shell. Dependencies use installed Node packages and only the exact Go module versions pinned by `go.mod`; downloads, Go environment files and toolchain installation are disabled. Package lock bytes are retained; installed dependency byte-for-byte correspondence is not certified.

Runtime version probes use an empty credential environment, private HOME, a three-second deadline and bounded output. Status includes observed Go/Node/Chrome/bubblewrap versions and selector-specific missingness. Supported browser packaging is the local `/opt/google/chrome` installation. Missing tools or unsupported packaging block the driver, rather than substituting another executable.

Output is at most 128 KiB and also limited by the remaining artifact budget. Overflow stops the driver and hashes only the retained output prefix. Each execution has a process group and an owned PID namespace. On cancel, deadline, crash or normal exit, the supervisor stops the namespace init and verifies that namespace identity is gone, terminating detached descendants too. Unverified cleanup blocks further process attempts and remains explicit in the retained result; no automatic retry occurs.

## Inventory and evidence semantics

Fixed entry points cover Go preprocessing, canonical Profile, ingestion/review, qualification/Application Draft and existing CV v2 contracts; Node canonical Profile, ingestion, job/reuse, artifact review/export and offline field tooling; existing browser canonical edit, ingestion, Apply enrichment, application reuse, section review and Resume review/export harnesses. This inventories finite existing tests; it is not a complete matrix. CV v3 delivery remains owned by #124–#128 and Lab #153.

Field tooling selects `tests/field-validation.test.mjs` and its injected helper imports. The live `scripts/field-validation/run-experiment.mjs` and live Go entry point are not selected or copied. The namespace independently prevents external inference and host credential access, even if a selected test contains an unintended fetch.

Guard selectors deliberately exercise assertion regression, missing tool, SIGKILL, output flood, a detached child/grandchild, and four finite environment/HOME/host-file/host-loopback sentinels. Negative guards are labeled and can intentionally end a run. Required following tests remain skipped/unreached. Assertion regression yields engine completed + assertion fail; missing runtime, sandbox failure, timeout, crash, native print unavailable and cleanup failure yield explicit operational outcomes with blocked assertions.

Direct screenshot/PDF files from a selector's own synthetic harness directory are captured after namespace cleanup. Capture rejects symlinks/nested profiles, limits 32 artifacts, 1 MiB each, and half the remaining artifact budget, validates format signatures, and retains bytes, kind, origin, runtime and SHA-256. Screenshot and `cdp-produced-pdf` are distinct. Native print/save and per-print-request observations remain unavailable; a CDP PDF never establishes native export. The dashboard verifies artifact bytes against their hash before download. Native JSON export/import preserves all original bundle bytes under quarantine, without promoting trust.

M18 reports logical/HTTP/dispatch/test-driver progress separately. M17 can report one fixed-driver exit assertion. M22 is one local driver-duration observation, not provider/browser interaction latency. M28 can report only the four injected isolation opportunities. M02/M06/M09/M16–M20/M24–M29 otherwise preserve missing domain populations, review decisions and native export evidence as unavailable. PASS text is never scored as semantic truth.

## Controlled observations and remaining gaps

The first inventory run passed all selected Go/Node entry points, section review, ingestion, application reuse and Resume review browser harnesses. Canonical edit and Apply enrichment returned existing horizontal-overflow assertions in isolation. Those failures are retained as failures; their cause has not been established. External fonts/resources are blocked, so this environment does not certify externally loaded typography or deployed layout. No product UI behavior is changed by this slice.

Verification commands and immutable result hashes are recorded with the review evidence. Host namespace checks must actually execute; a skipped default-sandbox test is unavailable evidence, not a successful end-to-end run. All inputs/outputs here are synthetic/offline. No provider, deployment, paid experiment, real Profile or personal browser data was used.

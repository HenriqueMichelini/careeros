# Private local evaluation history

Version: `lab-store-v1`. Result: `lab-result-v1`. Imports: `lab-import-v1`.

`OpenFileStore` implements the public `HistoryStore` seam. The default is `$XDG_DATA_HOME/careeros/evaluation-lab`, or `~/.local/share/careeros/evaluation-lab`. A launch caller may supply an absolute directory. No store is opened by production handlers. Tests use temporary directories. This slice adds no executable, server, browser UI, telemetry, provider request, credential discovery or real-profile capture.

The current implementation supports Linux. Other platforms fail explicitly before a writer is admitted: this avoids promising atomic containment, locking or directory fsync where these have not been implemented and verified. Directories are 0700 and files 0600. One writer holds an OS advisory lock until Close; a process exit releases it. Reads use `os.Root` plus rejection of every symlink component. Linux atomic rename uses already-opened parent descriptors, keeping containment across concurrent path changes. Import arguments are bytes, never paths.

## Save and recovery

Connect the runner with `RunnerOptions{Retain: store.Save, BeforeDispatch: store.Health}`. Save is an internal trusted runner boundary; future HTTP routes must never accept arbitrary RunRecord uploads through it. Inference is still controlled by the runner's frozen plan and intercepted transports. The store never dispatches, resumes or retries a workflow.

Every checkpoint creates an immutable snapshot directory. Case JSON and aggregate result JSON are written to new temporary files, synced, renamed and their directory synced. A content-free checkpoint event is appended and synced before the versioned manifest is atomically replaced and its parent directory synced. The manifest binds result/case SHA-256 values. A successful Save acknowledges required writes only after completion. Stored copies set Durable true; the memory runner continues to report its own persistence limitation. Old snapshots and orphaned partial snapshots are preserved. There is no automatic retention or deletion.

The checkpoint journal is distinct from the runner's live stage-event ring: it records monotonic sequence, execution state, progress counts and dispatch count, with no source/output or inferred timing. It is append-only and bounded to 16,384 entries and an 8 MiB read bound. Exhaustion fails persistence and stops the next dispatch; entries are never silently dropped. Truncated JSON, missing newline and sequence gaps are reported. The runner's detailed live stage ring is not represented as durable checkpoint events.

On reopen, bounded manifests rebuild the summary index without loading terminal results/cases. Incomplete runs load their last committed result, become interrupted, and preserve terminal case evidence. A dispatch committed as planned or attempted has unknown-final status after a crash: the external side-effect boundary might have been crossed before the next checkpoint. No new dispatch occurs. If writing recovery fails, a non-durable interrupted view remains readable, storage health blocks new dispatch, and unrelated history remains available. Damaged manifests are reported separately; a corrupt journal does not discard an independently valid snapshot. An incomplete snapshot cannot replace the last committed manifest.

## Public reads and management

List filters suite/execution state and paginates at most 100 bounded summaries; the catalog limit is 4,096 runs/imports. Case and Artifact read only the selected case file and validate its manifest hash. Load reads the aggregate result on demand (64 MiB bound); original imports and extracted artifacts have an 8 MiB bound. Case envelopes allow up to 64 MiB, matching the write bound and leaving room for source/expected metadata around a bounded artifact. Schema versions, identities, snapshot paths and symlinks are validated. Result/case integrity failures remain explicit rather than being silently repaired. Checksums establish byte integrity, not semantic support.

Usage performs a bounded inventory (100,000 entries), reports bytes and a configurable soft warning (default 512 MiB). Immutable checkpoints can consume substantial disk space; soft warnings do not delete or evict history. Delete requires one exact run identity, renames it out of the active catalog and syncs before removing it. A deletion crash can leave an explicitly reported tombstone. Users choose deletion; nothing is deleted on startup.

## Immutable imports and exports

Import uses the existing retained synthetic hash allowlist and dependency validation. Exact original bytes, original hash, dependency bytes and versioned derivation links remain immutable. Caller mutation cannot change owned evidence. Unknown bytes remain quarantined and excluded from trusted comparisons. No filesystem browsing or private release capture import exists.

ExportOriginal returns approved retained bytes exactly. ExportJSON uses `lab-synthetic-export-v1`, preserving original bytes and approved dependencies; ImportExport revalidates the bundle, enabling retained-artifact round trips. ExportMarkdown uses `lab-markdown-v1` and reports case-level evidence states without claiming semantic proof. These exports reject quarantine. Native controlled/offline runs can also be exported, with simulated-dispatch checks and an explicit 8 MiB envelope limit so every successful export can be reimported. Reimported native bundles preserve their exact bytes in quarantine because an uploaded schema does not establish a trusted synthetic origin. They require explicit future artifact registration before trusted comparison. ExportDiagnostics uses `lab-diagnostics-export-v1` and emits summary metadata only, excluding cases, original bytes, local paths, prompts and outputs.

## Verification

Public store/runner tests cover private modes, exclusive writers, reopen/index rebuild, partial interruption and unknown-final dispatches, persistence failure with zero later dispatch, committed-snapshot recovery, truncated journals, isolated invalid versions, traversal/symlink rejection, immutable imports, export round trips, content-free diagnostics, lazy case preservation, pagination, disk warnings and deliberate deletion. Storage failure injection occurs at a local BeforeWrite seam; it does not emulate every kernel/filesystem failure. Linux temp-directory tests do not establish power-loss behavior on every filesystem. No live provider, deployed endpoint, browser or real personal artifact is involved.

# Profile Export — local implementation evidence, 22 November 2026

Review-ready implementation for [#287](https://github.com/HenriqueMichelini/careeros/issues/287), fixed base `12a74dabee14f15468e62ff80d145b3930147afb`. Worktree: `/home/henriquemichelini/workstation/careeros/.local-worktrees/qualification-277-20261102`; branch `codex/batch-277-qualification-20261102`. Independent Standards, Spec and security reviews are still required on the committed head; this report does not claim ticket success.

## Selected contract and implementation

Live issue/owner audit read 22 November: [owner6644626253](https://github.com/HenriqueMichelini/careeros/issues/287#issuecomment-6644626253), [S08-D22](https://github.com/HenriqueMichelini/careeros/issues/258#issuecomment-6330937012), [optional export/deletion](https://github.com/HenriqueMichelini/careeros/issues/258#issuecomment-6330893921), [never-paid retrieval window](https://github.com/HenriqueMichelini/careeros/issues/258#issuecomment-6550859196). OPEN/ready-for-agent, no native blockers. The frozen engineering contract is `docs/profile-export-contract.md`; usage/limits are in `docs/managed-profile-export-local.md`. No new policy choice was made.

`GET /api/profile/export` uses the exact authenticated owner and existing account/session/PilotRetrieve locks. It exports the complete raw retained Profile v2/provenance BYTEA in a version 1 JSON envelope. No projection, number reserialization, normalization, trust upgrade, authentication/grant/financial payload join, import, companion format, temporary server file or new retained class is introduced. Query selectors are rejected. Trusted time is rechecked after document reading/encoding; the actual outer router response is written while authority locks remain held. Optional export does not gate or schedule deletion.

The Profile page has an explicit EN/PT control, no automatic request, original-Blob download, fixed filename, metadata-only error messages and stale-owner response suppression. Successful UI feedback says download started, not that a user filesystem save is guaranteed.

## Final frozen checks

Eight runtime blobs and seven fixtures are listed in the companion summary, with Git blob and SHA256 bindings. They were unchanged across the final checks. All commands use RTK. Verification wrapper `verify-pg.py` in the private evidence directory derives from the existing disposable runner, changing only its fixed repository path, serial `-p=1`/600-second package ceiling, and browser entry point. Product deadlines/clock assertions are unchanged. Private PostgreSQL gates ran serially under a short real repo-backed directory, with default fsync/WAL; no symlink guard or reviewed manifest was weakened.

| Check | Actual result |
| --- | --- |
| Focused export PG/race |6 passing events (five tests plus package); browser helper skipped outside its dedicated fixture. |
| Relevant Go/PG/race |1,291 passing events, exit0, zero failures; all 37 canonical admission IDs pass. |
| Node |446 individual isolated cases pass, including both inherited44-case parity corpora and2 new export client cases. |
| TypeScript/build |Both pass; build output goes outside the source worktree. |
| Actual browser |2 final EN/PT React App→Vite→authenticated Go/PilotRetrieve→private PG cells pass. |
| Full Go |2,539 passing events; exit1,25 retained Lab test/subtest failures plus its package failure. **Not green.** |
| Vet |Default exit1 with15 diagnostics byte-identical to the retained baseline receipt; explicit `-composites=false -stdversion=false` pass. Go1.26.4/module1.24; no Go1.24 compatibility claim. |

The full failure set exactly matches the retained evidence committed at this exact base (`docs/release-evidence/2026-11-22-draft-terminal-resolution-summary.json`, blob and SHA256 in summary). There are no added or missing failures. All six Lab/CV/dispatch/HTTP dependency trees remain unchanged. The set includes the independently known load-sensitive retained CV replay deadline and ProcessRunner paths; this does not assert a newly proven deterministic cause. No redundant exact-base rerun or unrelated Lab repair was performed.

Helper-only/empty-package skips and the optional genuine-base exporter skip remain explicit in raw summaries. Inherited physical restart/admission tests passed within the final relevant run; no separate new Profile Export physical-restart acceptance claim is made.

## Exact-value and authority observations

The independently authored literal fixture contains stable IDs/revisions, composed/decomposed Unicode, supplementary characters, NUL, unreviewed/unsupported/unknown qualifiers, approved excerpts, typed support/context relationships and opaque nested values. The server test compares the complete envelope using JSON whitespace compaction only, preserving number lexemes and every source value. The file includes opaque integer9007199254740993,1e+308 and5e-324; browser verification checks those raw byte sequences rather than trusting a lossy parsed/stringified copy.

Each final browser’s first real downloaded attachment is3,949 bytes, SHA256 `a5f49b7543c245a2fb4fcc2357303a3e7db2385968eabdc3535d9e3fcb2d6002`. An additional durable post-run byte comparison verifies both complete downloaded files equal the independently literal raw envelope, including all JSON whitespace and number lexemes; this is separate from the browser’s parsed full-value assertions. Trusted keyboard input initiates the download. The real routes produce200 in the post-pilot retrieval window,401 for expired session,200 after same-owner reauthentication, and403 after a private test-only restrictive pilot-state injection. A paused authorized old-owner response cannot initiate a file after actual cookies/session ownership switch; the independently observed account metadata confirms the switch. No file contents are saved in browser storage. Fixed localized errors never display a server error body. This is synthetic local authority evidence, not actual participant or hosted operation.

Public router controls also verify foreign empty-owner404, query/path substitution denial, mismatched account-header409, missing/expired session401, strict end-of-window403, revoked403, and the account Function alias. A document table lock crosses session expiry after initial authorization: final response is401 without attachment. In the exact outer alias Write, a concurrent revocation remains blocked until the authorized response write; subsequent export is401. Export requests are GET-only and make no Profile mutation.

## Development failures, limitations and provenance

The first public tracer saved the literal fixture successfully and then failed404 on the missing export endpoint; it became green after implementation. Client test execution first failed because the module did not exist. Typechecking caught a dynamic translation-key type error, then passed after the bounded type correction. These are development REDs, not baseline failures. The client missing-module and TSC outputs survive as tool observations; no persistent raw receipt is falsely claimed for them.

Two browser attempts failed on harness navigation (App initially opened Apply) and focus synchronization before keyboard input. Corrections changed only the test navigation/wait, not product assertions. Earlier successful3,903-byte download and switch receipts are historical fixtures, superseded by the final independently observed owner/beyond-safe-number3,949-byte cells. They are not summed as unique final scenarios. Initial restricted PG socket and Node subprocess attempts encountered EPERM. Capable-sandbox retries passed with unchanged product source/assertions; the initial failed Node log is retained separately from446 final passes.

The companion summary contains available raw receipt paths/hashes, complete failure/skip lists, all 37 admission identities and exact source bindings. No raw receipt is described as an independent rerun when reused. Dedicated test bootstrap/control endpoints exist only in the Go test binary; no product constructor exposes them. Managed security scanning remains unsupported for this linked worktree (resolvable-HEAD setup error); complete independent immutable source/evidence security review remains required. No completed managed scan is claimed.

Everything is local synthetic/controlled/disposable. No real provider account/key/call/spending, actual invitation/grant/admission, deployment/configuration, hosted recovery, GitHub comment/push/PR/merge/closure or later-ticket implementation was performed.

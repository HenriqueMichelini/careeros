# Confirmed account-owned deletion — local synthetic verification

**Review-ready local synthetic checkpoint, 22 November 2026. Required checks are terminal within the inherited limitations below. Independent immutable reviews are pending; no ticket local-success or operational-readiness claim.**

Base: `5269458cecf6064a5e26676ec660302ca44ff40e` on `codex/batch-277-qualification-20261102`, in the durable `qualification-277-20261102` worktree. The original research checkout and earlier cumulative commits are preserved. The source/fixture checkpoint is [the frozen manifest](2026-11-22-account-deletion-frozen-source.json): 30 runtime files and 13 fixture files. This report records verification, not a parallel issue tracker.

## Selected contract and resulting behavior

Canonical [#288](https://github.com/HenriqueMichelini/careeros/issues/288) and [owner selection 6645200501](https://github.com/HenriqueMichelini/careeros/issues/288#issuecomment-6645200501) select Profile deletion, account closure, earliest known access end, durable fences and truthful active/residual status. Native dependencies #283/#284/#286 are locally reviewed commits already in the base. The old UNAPPROVED audit proposal is not the authority. [The exact interface/matrix](../account-deletion-contract.md) and [local composition guide](../managed-account-deletion-local.md) describe the selected boundaries.

Explicit Profile deletion removes the canonical Profile immediately and fences all bound Draft/input/answer/support/review/history publication and access before dependent cleanup. The account, original unused grant and consumed allowance survive; a replacement Profile requires a new identity. Account closure also revokes ordinary sessions/invitations/grants, preserves the subject-to-closed-account deny mapping and deletes all owned career content. Fresh verified sign-in can issue a separate metadata-only status session, never workspace or new grant authority.

Operations bind exact owner/session, operation identity, Profile ID/revision and explicit intent. Duplicates and lost replies recover one operation. A deletion that waits beyond session expiry rolls back its entire tentative fence. Current Profile read/Save responses, export and status hold their final authority through the actual outer HTTP write. A winning deletion cannot be overtaken by old Save, generation, review or opening state. Original delivered effects remain; only still-reserved customer rights release. Provider rows, numeric exposure and unknown holds survive independently.

The local code-owned cleanup CLI accepts only an existing fenced operation in a literal loopback disposable `*_test` database. Cleanup failure persists a bounded error and cannot reopen content. Completion verifies primary absence; completed-marker replay removes stale reintroduced affected content without resetting original anchors. Profile A cleanup/reapply preserves new Profile B and post-fence Actions.

The conservative recovery-copy ceiling is original `fencedAt +30 elapsed days`, because canonical active Profile removal already occurs at the fence. `removedAt` separately records full affected primary cleanup. Failure, delay, duplicate cleanup, copy or restore never moves that ceiling later. Recovery/provider/recipient outcomes remain `not_verified`; no universal erasure, Google account deletion or centrally controlled customer-download erasure is claimed.

Known early revocation starts the original read-only retrieval/export window for a fresh trusted identity. Revoke/reissue/status cannot extend it. Inherited unknown-end revocations remain denied and advertise no invented retrieval deadline. Independent compromise/suspension holds preserve clocks and deny retrieval/new inference. The sealed handoff carries those nullable end/hold states without inventing a timestamp.

## Completed current-source checks

| Check | Actual result | Receipt |
| --- | --- | --- |
| Focused deletion PG/race | 26 passing events; no failures. Private browser/restart helper selectors skipped in that aggregate, exercised separately below | `.q88/careeros-281-check-2fda757f` |
| Relevant serial Go/PG/race, final policy fixture | 1,397 passing events, all 37 canonical admission IDs, exit 0, no failed packages | `.q88/careeros-281-check-3ehjbucg` |
| Full isolated Node suite | 450 passing tests, 0 failures/skips; includes inherited qualification/Profile and CV full-value parity corpora | `.q88/final-20261122/node.log` |
| TypeScript | exit 0 | `.q88/final-20261122/tsc.log` |
| Explicit local pilot/Draft build | exit 0; existing large-chunk advisory only; output outside tracked `dist` | `.q88/final-20261122/build.log` |
| Actual React/Vite/Go/private-PG browser | 7 passed cells | `.q88/careeros-281-check-055a3jeh` |
| Physical PostgreSQL/fresh process | 4 passing Go events: Profile/account subcases, parent and package; no failed/skipped events | `.q88/careeros-281-check-g7c92e3l` |
| Full serial Go/race | 2,564 passing events, exit 1; exactly25 inherited Lab test/subtest failure paths plus the Lab package failure, zero new/missing failures | `.q88/careeros-281-check-pzbdylv1` |
| Default/scoped vet | Default exit 1:15 diagnostics byte-identical to retained baseline; scoped exit0 with explicit `-composites=false -stdversion=false` | `.q88/final-20261122/vet-default.log`, `vet-scoped.log` |

Event totals include package/subtest events and are not summed as unique cases. All `.q88` paths above are rooted at `/home/henriquemichelini/workstation/careeros`, outside the tracked worktree. Exact terminal summaries, full failed/skipped case lists, commands and178 available receipt SHA256s are in [the machine summary](2026-11-22-account-deletion-summary.json). All30 runtime/13 fixture blobs match the frozen manifest. The browser cells are four real EN/PT explicit confirmation/status cases plus late same-owner export, same-owner fresh status-only transition and independent foreign-owner recovery after an uncertain closure reply. The transition controls exist only in the test binary. They do not expose production bootstrap/control routes or prove provider identity quality.

The restart schedule uses an owned private fsync-enabled PostgreSQL immediate crash/restart and a fresh Go child process. Canonical/alias operation status bytes match exactly. Duplicate cleanup preserves original clocks, removes affected Profile/Draft content and preserves counted provider obligations. The synced witness remains exactly four managed stages; no generation/checker callbacks rerun. This is actual local process/database restart, not merely Harness reconstruction, and is not hosted restore or old-writer fencing proof.

## Permanent RED → GREEN evidence

- Missing deletion route returned 404; the first real router tracer now commits one exact fence and recovers status.
- A deletion blocked on an allowance row outlived its session and incorrectly returned 202. Final post-wait session verification now returns 401 and rolls all tentative effects back.
- Canonical Profile GET originally committed before outer HTTP release, allowing deletion to win first. Its outer response now retains account/session authority, as export already did; Save responses also re-read current authority after their commit.
- Earlier restored Profile, delivered-history, active-account/session rows could override surviving markers. Deny predicates and per-Profile Draft checks now take precedence. Original Action cutoff also rejects restored pre-deletion Profile-absence bindings without closing genuinely new post-fence Actions.
- Inherited default parent-role privileges exposed new tables. Explicit ACL installation denies raw deletion/status-session rows; narrowly granted read-only boolean functions use a validated owning schema, fully qualified relations and fixed `pg_catalog,pg_temp` path. Metadata and ordinary restricted-role tests cover this boundary.
- A server tombstone left an affected pending Save recoverable. The client now withholds/clears it even if browser `removeItem` throws. Known same-owner lifecycle events clear retained account-scoped editor/proposal ownership and invalidate pending export responses before a download starts.
- The initial sealed handoff omitted unknown-end compromise/suspension holds. It now preserves those independent states without fabricating an access-end timestamp. Seal tampering is rejected, original clocks survive reapply and unrelated new Profile/Action work survives.
- After a confirmed no-record status, the deletion client originally stranded a deliberate correction as `uncertain`. It now permits only an explicit correction with the same opaque key; mount/status still never POSTs and server binding/replay authority remains decisive.

The machine summary retains original failed receipt paths/hashes and distinguishes development checkpoints from final-source evidence. Direct tool-only Node REDs are described above, not falsely assigned raw-file hashes. No unsupported reviewer experiment or managed scanner setup was retried.

## Failed/superseded checkpoints and limits

The first relevant run used the old inherited assertion that all revocation forbids retrieval. It ended **exit 1**, with 1,395 passing events/all37 but one failed test (`TestPilotInvitationExpiryReissueAndRevocation`) and its package failure. The approved #288 contract explicitly changes the known-end rule. The corrected assertion now requires read-only rights and a strictly earlier original retrieval deadline while continuing to deny new AI/redemption extension. Unknown-end/unredeemed/revoked-session negatives remain. Runtime source did not change for this correction. The failed run remains separate from the green final-fixture rerun. It inherited default `/tmp`; final-fixture and full runners explicitly use short real repo-backed TMPDIR paths.

Initial sandbox socket/subprocess restrictions required authorized local capable reruns. Missing-method/import compilation checkpoints, self-created-marker finalization and restricted-role predicate integration failures are retained as development failures, not baseline failures. The browser harness initially lacked foreground keyboard focus, then contained a malformed assertion and an incorrect successful-closure alert expectation; its later current-source seven-cell result is distinct.

The completed full Go failure set equals the retained full run in immutable base `5269458`'s Profile-export summary. All six Lab/CV/dispatch dependency trees remain unchanged; the summary records their immutable tree IDs and the exact retained summary blob/SHA. This is reuse of verified immutable baseline provenance, not a newly rerun base suite. The25 failures include the previously reproduced load-sensitive offline retained-CV deadline and the24 ProcessRunner paths. Default vet's14 unkeyed test literals and one `os.Root.RemoveAll` stdversion diagnostic are byte-identical to retained evidence; scoped vet excludes both analyzers explicitly. Current toolchain is Go1.26.4, not proof of execution under the module's declared Go1.24. Scoped successes do not turn either exit-1 full/default check green. No unrelated Lab source/hash weakening is part of this change.

The independently keyed deletion/deny snapshot is a bounded code-injected handoff. It contains no career payload or credentials; minimal closed-subject identity linkage remains restricted. It does not grant live role elevation, provider access, participant admission, actual grants/deletion, deployment or permission to open a restored service. Downstream restore remains responsible for independently surviving current authority and old-writer exclusion before serving.

Managed security scanner setup does not support this linked worktree (resolvable-HEAD setup limitation). Final security assessment must be an independent complete read-only immutable source/evidence review with that limitation stated; no completed managed scan is claimed. Standards, Spec and security conclusions are pending and do not form part of the implementation test claims above. No push, PR, issue comment/closure or other GitHub mutation occurred.

# Rehearse recovery from an older pilot backup

[#293](https://github.com/HenriqueMichelini/careeros/issues/293) adds a disposable PostgreSQL rehearsal to the [closed pilot runtime procedure](closed-pilot-runtime-local.md). It uses the existing account, Draft, lifecycle, ledger and control interfaces with synthetic identities and fake inference. Local prerequisites are the reviewed [never-paid cleanup](managed-never-paid-cleanup-local.md), [separate operator controls](managed-operator-controls-local.md) and [closed package](closed-pilot-runtime-local.md). The [owner decision handoff](https://github.com/HenriqueMichelini/careeros/issues/293#issuecomment-6645315645) identifies the policy records for these authority classes.

## Run the local rehearsal

Use an ordinary local user with Go, PostgreSQL server/client binaries, and writable temporary storage. Supply the absolute installed PostgreSQL binary directory. The resolved server paths may contain ASCII letters, digits, underscores, hyphens, dots and path separators. From the worktree, use the exact scenario entrypoint.

```sh
rtk proxy env \
  CAREEROS_LOCAL_RESTORE_REHEARSAL=1 \
  CAREEROS_LOCAL_RESTORE_PG_BIN=/absolute/path/to/postgresql/bin \
  go test -race -count=1 -timeout=120s -json ./internal/accountapi \
  -run '^TestLocalOlderPilotRestoreRehearsal$'
```

Each scenario creates two databases in separately owned, Unix-socket-only clusters under private `/tmp` directories. The harness writes the socket configuration to its owned PostgreSQL data directory and checks the live listening settings. It creates the initial fixture itself and accepts no backup path, database URL, credentials, real identities or transport configuration. The archive and decision data live in a separate private temporary directory. The test stops each cluster and removes its owned data, retaining private files if it cannot verify shutdown.

## Order and stop conditions

1. Start the actual Account/Draft adapters and fake gateway against the source fixture. Save Profiles and activate fixture grants. Deliver and review one complete Draft, close and clean another before delivery, and hold a third complete Draft for a later customer decision. Persist one unresolved dispatch. Record a revoked grant, a removed Profile excerpt and a separately deleted Profile with their original clocks. Observe these facts before pinning the older logical backup by SHA-256.
2. On the source, record a later Draft cancellation, an additional unresolved dispatch, separate revocation, account closure, and removal of another legacy Profile excerpt. In the containment scenario, apply a reviewed unpriced finding to the qualification dispatch and record its command, event and active namespace stop. Quarantine both old routes, stop invitation and inference, and close and drain each registered local gateway. The target remains inaccessible. Capture the latest bounded decision receipt independently of the backup and keep the exact backup, schema and receipt identifiers outside both databases.
3. Restore to the isolated target using a single-transaction PostgreSQL restore. Recreate the three scoped synthetic ledger role identities and restore their archive ACLs. Check the archived grant, detail, deletion and delivery witnesses before replay and confirm the later decisions are absent. Require a matching schema and complete decision inventory. Reapply the bounded non-content ledger, terminal, reconciliation and control facts and the existing authenticated lifecycle, pruning and deletion handoffs. The later pruning fence denies the older revision. Recompute the authorized cleaned revision with the canonical document operation while the target is quarantined. Check that replay preserves both archive and later clocks. Discard old sessions and require fresh authentication.
4. Verify the old PostgreSQL process stopped and its private socket disappeared, then run the original read, write, opening and fake-send paths and the old captured result capability again. Old transport and publication remain denied. An old cluster that is still alive is a failed fence even if both route guards are closed.
5. With both global stops present and the local old-system proof verified, use the existing synthetic checkpoint and an independently authenticated operator session. If the reviewed namespace stop survived, replay the recorded command and verify the original owner-review event, expense and possible occupancy. The checkpoint must refuse global generation advance and a fresh qualification request must remain denied. Otherwise advance the global generation through the existing verified command, leaving invitation activation stopped. Gateway completion before generation advance is denied. Verify the original reviewed artifact, the fresh Session's receipt and confirmation replay, and denial of the older Session and its receipt. Both valid confirmations must keep the original delivered time and produce one total customer effect with unchanged allowance.

The receipt records 1,344 microUSD and one possible remote occupancy for qualification and 107,376 microUSD and one possible remote occupancy for the Draft ledger. Closed Drafts retain their incurred cost. Both scenarios perform zero additional fake provider calls after restoration. The two closed Drafts, one delivered Draft and one unknown Draft keep distinct customer outcomes and clocks.

The failed archive, incomplete decision receipt, incompatible catalog, unproved old-system fence and retained namespace containment each stop before opening. Keep the old and candidate capabilities closed while reviewing missing or conflicting evidence. The selected checkpoint requires evidence beyond elapsed time, credential rotation, traffic removal or a new database endpoint.

The recorded interval measures local quarantine through verified retrieval under this fixture. Backup and current facts differ by the explicitly named later decisions. The fixture does not sample an operating backup schedule or establish a numerical RPO or RTO. Exact names, amounts and commands for this rehearsal do not select provider terms or a hosted recovery policy.

## Selected-host drill proposal

This procedure requires separate approval after the missing [Q1 decisions](https://github.com/HenriqueMichelini/careeros/issues/261) are selected for the intended environment. It extends the [hosted configuration proposal](closed-pilot-hosted-proposal.md).

| Approval input | Required drill observation and stop condition |
| --- | --- |
| Exact candidate, prior compatible candidate, schema and application versions, role inventory, recovery configuration and backup identity/checksum | Start the selected candidate quarantined. Use an older synthetic backup with pinned expected Profile, Review, Draft, admission, expiry, cleanup and revocation values. Reject any unverified mismatch, missing authority class or incompatible rollback. |
| Exact region, restore method, copy classes, independent durable decision source and approved acknowledged-content policy | Reconcile later deletion and copy-expiry anchors, access clocks, terminal delivery/allowance, expense, unknown occupancy and replay rights against the independent source. Verify every acknowledged fixture value required by the approved policy. Abort on lost or uncertain evidence. Record separate copy and recipient outcomes. |
| Enumerated deployed entrypoints, writers, readers, callback/egress and response capabilities, with the selected exclusion mechanism | Observe exclusion for every listed old identity and independently verify the winning fence. Exercise old reads, writes, publication and fake sends against the staged locally approved fixture before generation advance. Abort if any capability or unregistered instance is unaccounted for. |
| Owner-approved RPO/RTO and recovery limits, drill resource/cash ceiling, outage communications and authorized operator roles | Measure oldest acknowledged loss boundary and time from quarantine through verified recovery and authorized opening. Retain exact raw execution/failure evidence, resource spend, operator sign-off and limits. Abort on budget, target or rights failure. |
| Separately authorized execution, a verified compatible rollback path and a current explicit owner reopening action | Keep invitations and inference stopped through verification. Advance generation only after durable decisions, whole-old-system exclusion and owner action. Recheck rights and surviving protective records before opening each approved capability. |

Host, region, backup schedule, RPO/RTO, acknowledged-content treatment, exclusion mechanism and drill spend remain unselected. The local rehearsal provides no hosted exclusion evidence and does not change the [#52 environment boundary](https://github.com/HenriqueMichelini/careeros/issues/52).

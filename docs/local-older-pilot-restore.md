# Rehearse recovery from an older pilot backup

[#293](https://github.com/HenriqueMichelini/careeros/issues/293) adds a disposable PostgreSQL rehearsal to the [closed pilot runtime procedure](closed-pilot-runtime-local.md). It uses the existing account, Draft, lifecycle, ledger and control interfaces with synthetic identities and fake inference. Local prerequisites are the reviewed [never-paid cleanup](managed-never-paid-cleanup-local.md), [separate operator controls](managed-operator-controls-local.md) and [closed package](closed-pilot-runtime-local.md). The [owner decision handoff](https://github.com/HenriqueMichelini/careeros/issues/293#issuecomment-6645315645) identifies the policy records for these authority classes.

## Run the local rehearsal

Use an ordinary local user with Go and PostgreSQL server/client binaries. Supply the absolute installed PostgreSQL binary directory and a short writable temporary directory. From the worktree, use the exact scenario entrypoint.

```sh
rtk proxy env \
  CAREEROS_LOCAL_RESTORE_REHEARSAL=1 \
  CAREEROS_LOCAL_RESTORE_PG_BIN=/absolute/path/to/postgresql/bin \
  go test -race -count=1 -timeout=120s -json ./internal/accountapi \
  -run '^TestLocalOlderPilotRestoreRehearsal$'
```

The test creates two databases in separately owned, Unix-socket-only clusters. It creates the initial fixture itself and accepts no backup path, database URL, credentials, real identities or transport configuration. The archive and decision data live in a third private temporary directory. PostgreSQL tools receive only the generated private connection information. The test stops each cluster and removes its owned data, keeping the local paths available for diagnosis if a cluster cannot be stopped.

## Order and stop conditions

1. Start the actual Account/Draft adapters and the fake gateway against the source fixture. Save Profiles, activate fixture grants, deliver and review a complete Draft, hold another complete Draft for a later customer decision, and persist one unresolved dispatch. Pin the older logical backup by SHA-256.
2. On the source, record the later cancellation, an additional unresolved dispatch, revocation, account closure, and removal of a legacy Profile excerpt. Retain the independently observed cleanup and residual-copy evidence. Quarantine both old routes, stop invitation and inference, and close and drain each registered local gateway. The target remains inaccessible. Capture the latest bounded decision receipt independently of the backup and keep the exact backup, schema and receipt identifiers outside both databases.
3. Restore to the isolated target using a single-transaction PostgreSQL restore. Recreate the three scoped synthetic ledger role identities and restore their archive ACLs. Confirm the archive is older, the target schema agrees with the receipt, and the entire expected decision inventory is present. Reapply the bounded non-content ledger/terminal/control facts and the existing authenticated lifecycle, pruning and deletion handoffs. The applied pruning fence denies the older revision. Recompute the authorized cleaned revision from the backup with the canonical document operation while the target is quarantined. Apply the other deletion and copy evidence again to verify replay preserves the original anchors. Require fresh sessions. An opening receipt bound to an older session cannot settle another delivery.
4. Verify the old PostgreSQL process stopped and its private socket disappeared, then run the original read, write, opening and fake-send paths and the old captured result capability again. Old transport and publication remain denied. An old cluster that is still alive is a failed fence even if both route guards are closed.
5. With both stops present and the local old-system proof verified, use the existing synthetic checkpoint and an independently authenticated operator session to advance the generation. Gateway completion before generation advance is denied. The controlled fake-only validation leaves invitation activation stopped, recovers the exact reviewed artifact, and attempts replay with no provider effect.

The receipt records 1,344 microUSD and one possible remote occupancy for the qualification dispatch and 72,032 microUSD and one possible remote occupancy for the Draft ledger. The retained Draft's one delivery, the closed Draft and the unknown Draft keep distinct customer effects. No restored clock or elapsed wait settles a remote obligation or extends the original recovery-copy deadline.

The failed archive, incomplete decision receipt, incompatible catalog and unproved old-system fence each stop before opening. Investigate from the recorded stage. Keep the old and candidate capabilities closed while reviewing missing or conflicting evidence. Retries are deliberate and do not infer recovery from timeout, credential rotation, traffic removal, or a new database endpoint.

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

# Pilot operations incident recovery repair

This local repair addresses the remaining [#291](https://github.com/HenriqueMichelini/careeros/issues/291) review findings on parent `08a9e7f4653b23c5378c4d3c36243d338d9d3d09`. The [parent capture and replay repair](2026-11-23-pilot-operations-capture-repair.md) records its verification.

| Behavior | Local synthetic verification |
| --- | --- |
| First priced reconciliation breach | The actual enrolled CLI publishes one critical notice with the original reconciliation time. Later snapshots retain the identity. A higher material corrective bill records a distinct event and preserves the first event's amount and time. |
| Priced capture failure and replay | The actual enrolled CLI commits the financial event while the independent capture write fails. An inspection before replay leaves the original pending event untouched. The restored writer publishes one event for unchanged exposure and also replays the original event after a valid bill correction lowers current exposure below the cap. Subsequent inspections retain one event and acknowledge the pending receipt. The existing unpriced invariant remains distinct. |
| Recurrent backup and provider faults | Main-journal and SQL fallback exercises preserve first and latest observation times. The main journal retains one original notice. The independently signed CLI proof for the first observation cannot cover a newer recurrence. Journal-only backup and provider sidecars preserve both observation times through interval replay. Only current separate cause-specific proof resolves the corresponding fault. |
| Interrupted legacy sidecar | Both an existing partial file and its replacement by a complete authenticated observation enter signed classification. A signed interval without that classification keeps the latch in place. The exact verified classification permits safe sidecar cleanup while both #290 controls remain stopped. |

The [frozen manifest](2026-11-23-pilot-operations-incident-repair-frozen-source.json) binds the 10 repair paths. The [machine summary](2026-11-23-pilot-operations-incident-repair-summary.json) binds the exact check receipts.

The frozen affected Go race run passed 842 test and subtest events across 11 passing packages. It covered all 37 admission case IDs. The focused frozen run passed 109 test and subtest events, including all 50 pilot operations events. Both browser locales and scoped vet passed.

The earlier full-repository run retains 25 inherited Evaluation Lab test and subtest failures and its one package failure. Default vet retained 15 inherited diagnostics. The previous 453 passing frontend unit tests, typecheck, and build remain the evidence for the unchanged frontend.

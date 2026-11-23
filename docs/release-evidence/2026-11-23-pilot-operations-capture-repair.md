# Pilot operations capture and replay repair

This repair addresses the remaining [#291](https://github.com/HenriqueMichelini/careeros/issues/291) findings on local parent `d465006796cf83f355bb76480bc9d92a9d50f1d0`. The [parent containment repair](2026-11-22-pilot-operations-containment-repair.md) retains its verification.

| Behavior | Local synthetic verification |
| --- | --- |
| Exact failed reconciliation capture | The real enrolled reconciliation command accepts an unpriced event at the unchanged reserved maximum. The capture writer fails while the financial event, inherited #283 stop and private replay receipt commit. The restored writer delivers one `invariant_breach` with the original timestamp, obligation and cap. Repeated inspections do not redeliver it or leave its acknowledged row pending. |
| Independent capture and acknowledgment authority | The finance-only CLI retains its #283 obligation and stop. The financial and runtime roles lack direct pending deletion, private replay write privileges and arbitrary capture access. An enrolled observer acknowledges the selected source receipt by matching its identity and digest after successful journal publication. Simulated journal write failure leaves the undelivered rows pending. |
| Post-capture authority | The actual PostgreSQL capture fence outlasts the exact ordinary Session at reservation and durable intent, and outlasts the reviewer's grant at real reconciliation apply. Neither expired party commits the protected transition. |
| Ordinary support commands | The real CLI reviews and closes a case with empty standard input. It denies an empty response and persists a valid minimized substantive reply. |
| Journal-only fault recovery | A failed main journal write leaves a bounded authenticated receipt for each tested backup or provider fault. The actual recovery CLI preserves the type and original time. Signed interval review leaves the underlying fault open. A separately signed cause-specific proof is required to resolve it. |

A private deferred record is staged from the current authorized financial transition. Its original event data survives later recovery reads. A simultaneous complete loss of all durable authorities still calls for independent interval review before using restored media. The replay's signed classification handles cases where a trusted typed journal receipt was unavailable.

The [frozen manifest](2026-11-23-pilot-operations-capture-repair-frozen-source.json) binds the 18 changed source, test, and guide paths. The [machine summary](2026-11-23-pilot-operations-capture-repair-summary.json) binds the checks and receipts.

The frozen affected Go race run passed 833 test and subtest events across 11 passing packages and all 37 admission case IDs. The focused frozen run passed 100 test and subtest events, including all 41 operations events. Scoped vet and the English and Portuguese browser checks passed.

The earlier full-repository run retains 25 inherited Evaluation Lab failures and its one failed package. Default vet retained 15 inherited diagnostics. The prior 453 frontend unit tests, typecheck and build remain the source evidence for the unchanged frontend.

# Complete Draft opening — review repairs, 22 November 2026

This repairs [#285](https://github.com/HenriqueMichelini/careeros/issues/285) from immutable `83ee29992076faa3d510b5f781226680a5bf1bb7`. The original [report](2026-11-22-complete-draft-opening.md), source/RED manifests and retained test limitations remain unchanged. The [repair summary](2026-11-22-complete-draft-opening-review-repair-summary.json) binds **9 runtime and 11 fixture files**, including exactly **2 changed production Go files** and one new permanent test file. The containing commit is the new review checkpoint; no success verdict is claimed.

## Repairs and original review limits

The original formal Spec P2 showed actual allowance status waiting on the shared Action until after its original deadline, then incorrectly returning one reserved right. This was inaccurate status/release timing, not evidence of late consumption. `Store.load` now obtains the shared Action lock before evaluating the current Decision and trusted time. The permanent public-route regression checks zero consumption, one release timestamp across repeat status, and no extra generation/check callbacks.

A separate original security source concern was **unconfirmed**: its private PostgreSQL startup failed before any Go test executed. Both original reviewers later encountered a model reporting flag, **not an automatic tool-approval rejection**. Spec supplied a formal read-only report from completed evidence; security's evidence/hash verification and formal assessment remained unfinished. The exact error and limits are retained in the JSON; no startup receipt path was reported, so none is invented. No flagged experiment was copied or retried.

The new ordinary defensive regression uses only the existing local fake Decision/cancellation/deletion seam. It independently demonstrated both schedules: a restrictive fence won while resolving an older reservation, yet POST 403 still invoked a second generation callback. The repair resolves those older outcomes before rechecking the current restrictive Decision, then checks/persists the admitted Action's terminal guard **before** inserting a new generation witness. A later apparent rights restoration cannot revive the same Action. Existing typed authentication/access errors, zero-or-one customer settlement and independent provider obligations are preserved.

## Source-bound verification

| Gate | Result |
| --- | --- |
| Permanent development RED → GREEN | Status-clock failure plus ordinary cancel/delete callback failures preserved; focused GREEN 5 events. The development checkpoint predates the restored typed-error return; final suites below cover the complete frozen code. |
| Relevant real PostgreSQL/race | **1257 passing events**, exit 0, zero failures; every A01–A37 canonical ID passes. Counts include package/subtest events, not unique cases. |
| Fresh included controls | New status/cancel/delete guards, reservation/session/terminal races, SQL roles, unknown provider holds and real fsync PostgreSQL/fresh-process receipt/result/allowance recovery all pass in the final relevant run. |
| Full serial Go/race | **2505 passing events**, exit 1, **26 failure events**. Exact failure set equals original retained baseline: **True**. Not green. |
| Actual repaired browser/server | **4/4 EN/PT** cells: complete committed opening/lost ACK/zero hidden-part ACK and access-denial/reauth/current+prior protected hiding through actual React→Vite→Go/PostgreSQL. No new inference during recovery. |
| Frontend/Node/PDF inheritance | **105 unchanged frontend/Node-fixture blobs** bind prior 440 Node cases, TSC/build and PDF/layout evidence. These are inherited, not fresh repaired-Go aggregate runs. |
| Production dependency binding | **224 unchanged production Go/SQL/module blobs**; only store.go/opening.go changed. All individual hashes are retained. |
| Vet | Same 15 default diagnostics (exact prior log SHA), default exit 1; scoped `-composites=false -stdversion=false` exit 0. Toolchain Go 1.26.4 / module 1.24 limit remains. |

The whole-package harness uses the already established serial `-p=1 -timeout=600s` schedule. It does not change the 25-second provider/workflow budget, original 30-minute customer window, race assertions or any operating policy. Full-suite failure paths, package outcomes/skips and immutable baseline provenance remain explicit; no repeated exact-base run or unrelated Lab repair was required.

All work remains local, synthetic and controlled. No actual provider access/calls/spending, real grant/participant, hosted operation, deployment or GitHub mutation occurred. The composed opening path remains unactivated pending #286. Original security assessment limitations are not a no-findings result; independent Standards/Spec/security final-head readbacks are required before local success.

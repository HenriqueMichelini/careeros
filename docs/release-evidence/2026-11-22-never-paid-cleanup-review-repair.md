# #289 direct lifecycle-refresh denial — review repair

Status: repair source frozen and affected frontend gates complete; independent final-head review remains pending. Original review head `5a0088f711ea00462b4f8260b0a7aba392014ea6`, tree `7e82b5079f468ec6774595fa5f75ddc85c7cf9d0`, remains intact. The full ticket review range starts at `63b20b29d16de955c77869c9faf417dc642e9a75`.

## Confirmed finding and original evidence

Standards reported no hard issue and one optional duplicated notice-schedule suggestion. Spec and security confirmed the same sole P2: a successful direct lifecycle GET could report `careerReadAllowed:false` while `contentExpired:false`, but the browser waited for an unrelated focus/session probe before withholding cached protected recovery data. The pending merged Download remained offered and created one protected download. The server already denied access; this is cached-client disclosure, not additional server read authority.

Original browser receipt: `.q89/careeros-281-check-tnn7zz60/browser.jsonl`, SHA256 `944f1476a067b9951b4e273cc75dd049b721c3c8cad92df5cfbcaaf1bed1eaf6`. Original independent overlay SHA256: `76d2a4dbcf13a139e257e7ae191c4dac5e231d3f91c6edcab2297f226f8e683e`. Both were verified and retained under `.q89/review-repair-20261122/`; the companion summary records their original/retained paths. The original implementation report, manifest and summary are unchanged.

The permanent actual-app regression reproduced the same RED on the unchanged original runtime at `.q89/careeros-281-check-z2b79esu/browser.jsonl`: direct GET refresh, zero focus events, false read eligibility, false whole-content expiry, and an offered protected merged recovery download. It also holds a previously authorized native export response across the denial boundary.

## Bounded correction

`AccountLifecycle` forwards a fresh negative career-read observation immediately. `AccountGate` verifies the current owner, synchronously clears its read ref, updates denied state and emits the existing owner-bound cancellation event. The protected StoreProvider subtree includes its recovery controls, so it is withheld with the workspace. Account lifecycle metadata and independently entered same-owner input/opaque recovery keys remain available.

No focus/probe is needed to enforce the denial. A later positive metadata observation alone does not regrant access or reveal cached copies: the existing fresh session/Profile authority path must succeed. Ordinary 30-day retained-read rights remain usable; a valid replacement Profile is not treated as deleted. The denial signal cancels the existing native export request and its generation guard before any new object URL is created. Foreign-owner transitions cannot inherit the pending dialog or old wording.

Only two runtime frontend files, one permanent browser fixture and the explanatory guide changed. No Go, SQL, module, role, server authority, content-retention or financial/customer settlement behavior changed.

## Current repair validation

The permanent EN/PT direct-refresh controls pass both cells through actual React, Vite, authenticated Go and disposable PostgreSQL. Each records zero focus events before the denial, no protected recovery download, zero late export URLs, metadata availability, retained independent pending input, no positive-metadata regrant, deliberate same-owner recovery after fresh authority, and foreign-scope isolation. SQL pruning/reconciliation and pending checkpoint injection are private test controls; this is not production restore execution.

One initial post-fix attempt encountered a fixture navigation race while the application still displayed “Loading Profile”; the missing navigation selector was retained as a failed fixture checkpoint. The permanent test now waits for actual app navigation. It did not require a production fix or assertion weakening.

Final isolated Node coverage passes all 452 tests across 32 files, each in its own process with subprocess support; TypeScript and the account/pilot build pass. Seven current actual-browser cells pass: the two permanent EN/PT direct-refresh/held-export/recovery/foreign-owner controls, four ordinary EN/PT notice/removal controls, and one whole-window late-export control. These are three serial browser fixture commands, not one aggregate or seven distinct test cases inferred from Go events. The companion repair summary records their terminal results and exact source/receipt hashes.

## Reused Go evidence and limits

All **424** tracked Go production/test, SQL and Go module files are byte/blob-identical to original head `5a0088f`. Their individual Git blobs and SHA256 hashes are retained in the repair summary. Original source-bound evidence is reused explicitly: relevant Go/race 1,422 passing events, all 37 admission IDs passing, no failed packages; full Go exit 1 with 2,590 passing events and exactly the inherited 25 Lab test/subtest paths plus package failure; default vet 15 identical diagnostics and scoped vet pass; focused 20 events and physical PostgreSQL/fresh-process recovery proof. No new Go aggregate or physical restart is claimed for this frontend-only correction.

Original Node/browser results remain historical checkpoints; current repair frontend results are identified separately. The linked-worktree managed scanner limitation remains explicit: `Review changes requires a non-bare Git worktree with a resolvable HEAD.` No completed managed scan is claimed. Standards, Spec and independent read-only security final-head followups must assess the full ticket delta and explicitly close the P2 before local success.

No provider/email/recipient call, real grant, participant operation, spending, deployment, hosted recovery or GitHub mutation was performed. No later-ticket work was inspected or started.

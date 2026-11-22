# #284 local verification checkpoint — 21 November 2026

Base: `e4fd48d2e425a3a7b1492a4263e00d130be4e98f`. The selected [historical review/Accept](https://github.com/HenriqueMichelini/careeros/issues/284#issuecomment-6645202436) and [historical export](https://github.com/HenriqueMichelini/careeros/issues/284#issuecomment-6644605240) rules consume #281's exact durable bundle. [Source guide](../managed-draft-review-local.md) · [Frozen interface](../application-draft-review-contract.md) · [Exact blobs, raw receipt hashes and failure/skip lists](2026-11-21-durable-draft-review-summary.json).

This is a review-ready local synthetic checkpoint. Independent Standards/Spec/security reviews are pending on its immutable commit. It establishes no hosted readiness, actual opening/allowance implementation, provider access or participant rights.

## Implemented boundary

Four ordered artifact decisions bind owner, Action, exact bundle version/digest and expected review revision. Human review and Accept are deliberate separate operations. Equal command replay keeps one effect; changed payloads and stale concurrent revisions conflict. Readiness cannot create delivery authority. The existing trusted injected opening decision enables review here; #285's actual protocol and allowance settlement are not implemented early.

The accepted reader/export path keeps the original complete bundle and its identity/signature/support evidence. Failed/new unaccepted candidates cannot replace accepted work. Reads, lost-write recovery, reauthentication and exports invoke zero new generation/completion callbacks. Profile Save and provider/customer quantities remain separate. Purpose cleanup removes review data while preserving independent non-content protection. Restricted ledger runtime roles cannot read the new retained tables.

Already delivered historical bundles retain their original revision and stale disclosure for review/Accept/export under current read/deletion rights. Actual clipboard and PDF exports recheck the chosen exact accepted key/version. Session/account denial hides current, previous, accepted, context/support and transient print copies; independently entered same-account wording and its opaque recovery key survive reauthentication.

## Final checks

| Check | Result |
| --- | --- |
| Relevant Go/PostgreSQL/race | 1,224 passing events, zero failed packages; all 37 canonical admission IDs. Includes genuine inherited base-upgrade proof and new review/authority/closure controls. |
| Full Go/PostgreSQL/race | 2,472 passing events; exit 1 with exactly the retained 25 Lab test paths plus their package failure. No new failure path. |
| Review-specific permanent controls | 19 focused passing events, no skips: exact decisions, lost acknowledgments, owner/version/revision negatives, one concurrent winner, expiry, historical rights, deletion/cleanup, outer alias fence, five terminal no-revival schedules and real restart. |
| Physical recovery | Private fsync PostgreSQL immediate crash/restart and fresh child server compare byte-identical complete Result including review/accepted state through canonical/version/alias/accepted reads; durable generation/check witness remains 1/1. Included in final Go gates. |
| Actual browser | All 10 inherited and 2 new EN/PT React→Vite→authenticated Go→PostgreSQL cells pass on the final Go runtime. |
| Accepted exports | 16 real one-page A4 PDFs: four artifacts × two languages × desktop/390px. Exact Action, original revision, historical warning, accepted wording and frozen cover signature inspected. Trusted Tab/Enter, actual clipboard, clipboard denial/fallback failure/focus, overflow prevention and 401/403 recovery pass. Four representative resume/cover PNGs were visually inspected without clipping/overlap or missing accents. |
| Node/parity | 429/429 actual cases in 28 independently run files; includes 44 Profile and 44 CV full-value vectors and 10 Draft client cases. |
| TypeScript and explicit pilot/recovery build | Passed. Node/TSC/build receipts predate the final Go-only closure repair; their unchanged frontend/fixture blobs are recorded. Final Go and 12 browser cells cover that changed runtime. |
| Default/scoped vet | Default retains the same 15 diagnostics with byte-identical baseline SHA256. Scoped `-composites=false -stdversion=false` passes. Go1.26.4/module Go1.24; no declared-Go1.24 full-green claim. |
| Existing Firefox print check | Current and immutable exact base both print one A4 page, then fail the same unchanged uppercase text expectation. Later Firefox cases were not reached. Runtime/fixture dependency blob pairs and both logs are recorded; the assertion was not weakened. Chromium accepted-export evidence remains separate. |

Go event counts include subtests and package-completion events, not unique test counts. All helper/no-test skips are listed in the machine evidence. Each Node file ran in its own process with explicit reporter isolation so every case was counted. Initial subprocess EPERM and the later capable file-level/case-level results are distinct.

The full Go failure comparison reuses committed #283 evidence: 24 established Lab paths plus the CV replay deadline already reproduced at immutable `ea8b97d`. Relevant Lab/CV/dispatch/http tree identities remain unchanged. No baseline raw hash was recomputed, no manifest was weakened, and failed aggregate results are not called green. Default vet's existing `os.Root.RemoveAll` stdversion diagnostic and fourteen unkeyed test literals remain.

## Reproduced defects and preserved checkpoints

Public-route tests first failed because durable review/accepted retrieval did not exist. The first bilingual UI proof then passed explicit review/Accept and lost acknowledgment recovery. Missing export controls and the strict client's missing exact-version export method also produced RED before implementation.

The planned browser matrix found two further defects: a newer incomplete Action's status check left accepted-history currentness stale, and denied clipboard fallback could restore focus while its control was disabled. The explicit provider-free accepted-history read and after-render focus restoration pass their permanent regressions. Harness-only failures (reload response wait, missing Enter text, case-sensitive Portuguese wording and empty post-reload editor) remain separately recorded rather than promoted to product defects or passing cells.

The final bounded terminal probe reproduced five actual alias schedules: new/duplicate initial deletion, new/duplicate final-block deletion and closure before opening. A rejected decision rolled back the observed terminal state, allowing later apparent authority restoration to expose the old bundle. The repair commits only the observed closed/deleted purpose state before denial, consistently with Read/Publish; no human write has occurred yet. Temporary nonterminal denial still rolls back. All five REDs and the 19-event GREEN/control run remain retained.

Pre-repair relevant/full results (1,218/2,466 passing events) are bound to their preserved source freeze. One final full attempt ended with an unexplained outer exit143 and 2,400 pass events but no accountapi terminal event/summary. It is explicitly interrupted. Owned-child audit found no live Go/PG children before the single retry; the retry produced the final terminal 2,472-event result above. No completion is inferred from partial progress.

## Legacy #135/#136 distinction

Two real same-origin synthetic browser tabs reproduced Profile `history-profile` revision1→2. The first tab retained revision1 with `profileError=stale`; copy/PDF were enabled while a generic banner instructed regeneration and omitted the selected historical/original-revision disclosure. Its canonical stale write was independently rejected, with zero provider requests. The narrow selected #136 export/disclosure correction preserves the exact accepted browser snapshot and export after canonical refresh. Direct CLI output is recorded in this implementation's transcript; no separate raw stdout file was saved for these legacy runs. The permanent fixture and source blobs permit independent reproduction.

That is browser-local currentness evidence, not durable account delivery/accepted-history proof. No remote issue was changed or closed. Current account review/recovery/export evidence is the separate actual Go/PostgreSQL browser path above.

All receipts/PDFs were retained under the owned `.local-recovery/draft-review-284-20261121` directory after every owned process stopped; raw bytes were unchanged and resolved paths are recorded. Synthetic fixture callbacks, injected opening rights and local restart proof do not authorize or establish live provider quality, actual grants, spending, hosted restoration, deployment or launch. No GitHub mutation, push, PR, merge or issue closure occurred.

# New-account persistence — local verification, 26 October 2026

**Result:** the bounded local new-account slice is implemented and verified against real PostgreSQL. This is not production authentication delivery, complete SaaS delivery, disaster recovery, deployed enforcement or launch readiness.

## Identity and authority

- Published base freshly resolved through `gh` and fetched: `591b2889196f277c96b130192a68bbc700a2137f`. PR #270 was verified MERGED at that commit, preserving reviewed head `5b88310d675facd5680eee1ad198cbc6cf8fb987`; its completed repairs were reused.
- Branch: `codex/new-account-persistence`; dedicated worktree: `/tmp/careeros-new-account-persistence`. The primary checkout and other agents' checkouts were preserved. The final source head is the local implementation commit containing this report; its exact SHA is recorded in the task delivery and external evidence metadata after commit.
- Owner selected **“Select PostgreSQL + bytea (recommended)”** in this conversation on 26 October 2026, after the narrow engine/representation trade-off was presented. No provider, hosting service, plan, region, numerical recovery target or live Google configuration was selected.
- #259/#262/#258/#264 bodies and comments were refreshed with `gh`. The merged [#262 progress record](https://github.com/HenriqueMichelini/careeros/issues/262#issuecomment-6425766640) supplied the current auth baseline. Older research was reused only for ordinary Save, atomic owner/authorization publication, conflict, encoding and acknowledgment recovery. Legacy browser-data import is deferred by the current owner direction; #264 is not claimed complete.

## Implemented boundary

`internal/accountstore` supplies all three existing auth-store contracts using relational PostgreSQL records, and stores the validated current Profile as `bytea`. `internal/accountapi` exposes protected current-Profile read/Save and account-scoped operation recovery. `cmd/account-migrate` applies the additive schema explicitly; neither requests nor application startup provision/migrate it.

The transaction locks the account, verifies current status/generation, locks and checks the exact session including DB-clock expiry/private token binding, then atomically publishes the owner-bound Profile and content-free operation receipt. Revocation/replacement uses the common account-before-session order. Replay returns the historical committed revision alongside the current Profile, before stale-base validation, without another write. A new stale operation or changed request under the same identity conflicts.

New Profile routes are `GET/PUT /api/profile/current` and `GET /api/profile/operations/{operationId}`. Existing auth routes retain their selected Google/direct-Go/opaque-session semantics. The local command and new Netlify account adapter use the protected router; new endpoints fail closed independently of the legacy auth flag. The five pre-existing AI Functions remain guest/BYOK and outside hosted account enforcement in this slice; a local command flag is not claimed to protect them.

The opt-in account frontend starts clean, performs no legacy Profile reads/transfers/deletes, retains explicit review/Apply/Save, distinguishes pending/failed/uncertain/confirmed Save, preserves the frozen pending operation for recovery and rejects late account-A replies after switching to B. CV snapshots/choices are browser-local and account-scoped; BYOK keys are empty/session-only in account mode. The reviewed submission path uses the same account Save seam rather than writing its own browser authority.

See [the contract/runbook](../new-account-persistence.md) for exact request shape, representation bounds, retention behavior, endpoint inventory and production gates.

## Executed checks

| Check | Result and scope |
| --- | --- |
| Real PostgreSQL race suite: `go test -race -count=1 -v -timeout=120s ./internal/accountapi ./internal/accountstore ./internal/auth ./internal/netlifyproxy ./cmd/profile-review ./netlify/functions/account`, with the isolated synthetic DSN and `GOCACHE=/tmp/careeros-go-cache` | **PASS**, all six packages. PostgreSQL 18.4, loopback-only container; observed `fsync`, `synchronous_commit`, `full_page_writes` were `on`. Go 1.26.4. No mocks substitute for the storage/concurrency evidence. |
| Deterministic PostgreSQL revocation/login/replacement races, `-race -count=10` | **PASS**. Explicit DB lock wait observation orders both revocation-wins and commit-wins schedules; another device and fresh sign-in are positive controls. |
| Separate application-process restart journey | **PASS**, part of the real-engine suite. Sign-in start/callback cross two processes; create/Save; stop all application processes; create two new processes; recover the same Profile/session/receipt. Pending one-use OIDC state also survives restart, consumed state cannot replay, logout in one process denies access in the other. |
| HTTP acknowledgment loss and transaction failure | **PASS**. Close an actual TCP connection after the handler commits, before delivering its acknowledgment; recover/retry the same operation without duplicate effects. A receipt-insert trigger failure rolls back the preceding Profile statement. |
| Ownership/auth/representation | **PASS**. Two accounts/direct requests, owner/header substitution, foreign receipt recovery, missing CSRF/session, expired/revoked/suspended/deleted access, concurrent saves/duplicate requests, changed request identities, stale bases, storage outage, Unicode/U+0000/opaque finite values, max unsigned generation and safe revision bounds, invalid UTF-8/surrogates/numbers/oversize rejection and retained-excerpt constraints. |
| Frontend contracts: `GOFLAGS=-buildvcs=false node --test --test-isolation=none tests/*.test.mjs` | **313 PASS, 0 failed/skipped**. Includes six account-client cases and the reviewed-submission account Save acknowledgment test; established local contracts remain covered. Available tracked dependencies were used, not a newly verified frozen install. |
| Actual React/Chrome: `node tests/account-browser.check.mjs` | **PASS**. Zero legacy Profile reads; positive explicit Save; lost-ack recovery; uncertain edits + stable retry identity across focus; account remount/late-response denial; English/Portuguese account UI. Transport is synthetic here; PostgreSQL evidence is separately established above. Screenshot is a synthetic component harness, not a deployed/full-product screenshot. |
| `tsc --noEmit`; targeted `go vet` | **PASS**. |
| Production builds with and without `VITE_CAREEROS_ACCOUNT_MODE=1` | **PASS**. Vite 8.3.1/TypeScript 5.9.3/React 19.3.0. Existing `__dirname` future-loader and >500-kB bundle warnings remain; generated `dist`/Vite cache changes were excluded. |
| Uncached `go test -count=1 ./...` | **FAIL in the same 13 Evaluation Lab top-level tests reproduced at the pinned base**, with no added/removed top-level failures. The PostgreSQL tests in this general command skip without their explicit test DSN; the separate real-engine suite above is the acceptance evidence. |
| Whitespace/source scope | `git diff --check` **PASS** before staging; generated output was restored. No ProcessRunner manifest change or reapproval. |

The fixed manifest remains SHA-256 `c3fb7be93b892adca689c9798370d89bac305454ae3268055488f25376e7c595`. Stale entries are **28 at published base, 34 at this source**: the six newly stale paths are `go.mod`, `go.sum`, `src/App.tsx`, `src/lib/profileStorage.ts`, `src/pages/CvPage.tsx` and `internal/profiledocument/document.go`. The last is a package-comment update reflecting shared validation. Already-stale patch-owned entries remain stale. Test-name comparison, full inventories and unchanged manifest hash are retained; inherited failures are not reported as a green whole-Go suite.

## Retained evidence and corrected attempts

Raw logs, GitHub snapshots, environment/lock digests, stale-manifest inventory, failure-set comparison and synthetic browser screenshot are retained at **`/tmp/careeros-new-account-evidence-2026-10-26`**, with a SHA-256 inventory generated at delivery. The final commit identity is appended there after the local commit. This path is local evidence, not a published artifact or a permanent external archive.

Earlier failed attempts remain retained: sandbox `httptest` sockets / Node `spawnSync go EPERM`; helper Go VCS-stamping failure resolved with `GOFLAGS=-buildvcs=false`; tracked Vite cache duplicate React resolved with an isolated fresh browser-test cache; CDP driver DOM serialization corrected to boolean polling; an old formatter's invalid inline type output corrected and checked with TypeScript; and the new submission test's missing namespaced fixture ID corrected. Final pass claims refer to the corresponding later passing runs, not those failures. The original #270 repair/review evidence and source manifest were not edited.

## Concise journey demonstration

Run the real-engine `TestPostgresSurvivesAllApplicationProcessRestarts` test with its synthetic DSN. It signs in using fake OIDC, explicitly saves a valid Profile with provenance/opaque Unicode values, stops both application subprocesses, constructs two new instances and reads the same account-owned Profile and historical receipt using the persisted opaque session. It then consumes a previously pending login transaction, rejects replay of the original consumed transaction and verifies cross-instance logout. No Google/provider call or real personal data is used.

This demonstrates process-restart persistence and shared authority, **not** a database disaster restore, all-copy deletion, zero-data-loss guarantee, production Google flow, live Netlify route behavior or launch readiness. Provider/runtime/TLS/grants/pool sizing, whole hosted route enforcement, operational retention/replay horizon and deletion-aware recovery/RPO/RTO gates remain explicit. No linking, new recovery/deletion/operator API, billing, allowance, durable Draft history or queue is included.

Publication text is prepared separately for owner review. No remote issue/comment/PR write, push, merge, live provider action, spending, deployment or launch was performed. A later push/PR requires fresh authenticated Netlify deployment-prevention verification and exact-text publication approval; historical stopped-build observations are not treated as current permission.

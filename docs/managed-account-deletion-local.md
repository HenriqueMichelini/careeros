# Local confirmed deletion and access-end composition

This is local synthetic implementation, not disposal of a participant's data, hosted recovery, provider erasure, deployment or launch. The selected owner contract is [#288 comment 6645200501](https://github.com/HenriqueMichelini/careeros/issues/288#issuecomment-6645200501). Exact classes, interfaces and races are in [the frozen contract](account-deletion-contract.md).

## Customer operations

The normal authenticated account router and its account Function alias expose explicit `POST /api/deletion/operations` with `operationId`, `kind`, `profileId`, `revision` and `confirm: true`. Profile deletion requires its exact current saved identity/revision. Account closure uses empty Profile ID and revision zero. Ownership comes from the exact live session, same-origin/CSRF checks and consistent account header; body owner selectors are rejected.

The transaction locks pilot/account/session and affected purpose/shared/allowance rows, then revalidates the creating session after waits. It commits an immutable operation, Profile tombstone and terminal purpose/Action/customer fences before reporting `fenced_pending`. Canonical Profile bytes are removed immediately. Remaining bound Draft input/bundle/support/answers and review content are queued, not disclosed. Undelivered reservations release once; already delivered effects survive. Provider usage, scope counts and unknown capacity are not erased or refunded.

`GET /api/deletion/operations/{operation}` and `GET /api/deletion/status` return only freshly authorized operation metadata through the actual outer HTTP write. GET never initiates cleanup, provider work or a customer effect. Duplicates recover one operation; changed-content key reuse conflicts. A lost reply triggers only a GET. After an authenticated no-record result, a deliberate correction may reuse the same opaque key; it cannot rotate an unknown request into a second effect.

Delete Profile keeps sign-in and original unused grant rights; a replacement Profile requires a new identity. Closed accounts keep the minimal subject-to-account deny mapping and original non-content survivors. Ordinary sessions are deleted and fresh ordinary sessions cannot be issued. A new verified provider sign-in can issue a separate ten-minute metadata-only status session. Its cookie cannot authenticate Profile, export, workflow, grant or mutation endpoints, and does not reactivate the account. A session probe accurately reports `authenticated: false, deletionStatusOnly: true`.

## Bounded active cleanup

`Store.PurgeDeletion` is a code-owned queue worker. There is no customer cleanup or arbitrary SQL endpoint. The normally buildable `cmd/deletion-local` command accepts only an existing fenced operation, `--synthetic-only` and a literal loopback `*_test` PostgreSQL DSN with allowlisted routing options. It does not migrate/provision a database, start deletion or obtain provider access. Its database capability is independent of AI runtime/reconciliation roles.

Cleanup verifies affected primary content is absent before `active_content_removed`. A transaction failure persists only `active_removal_failed`, leaves access fenced and permits deliberate cleanup retry. Replaying completed cleanup still removes reintroduced affected rows and verifies absence; it preserves original removal/fence anchors. A Profile A cleanup cannot remove replacement Profile B or B's post-fence Actions.

The primary Profile is already removed at `fencedAt`, so the conservative recovery-copy ceiling is that original anchor plus 30 elapsed days. `removedAt` separately records all-primary cleanup completion. Delays, failures, retries, copying and restore never move the ceiling later. Recovery copies and recipient copies remain `not_verified`; the UI does not claim universal erasure or removal of customer-held downloads/Google accounts.

## Access end and retained deny authority

A new effective operator revocation records the earliest trusted access end. It denies new AI but allows freshly authenticated retained Profile/export and delivered-Draft retrieval until the earlier of original scheduled expiry or known effective end plus 30 days, subject to current holds. Repeated commands cannot extend the window. Inherited revoked rows with unknown end stay denied and advertise no invented retrieval deadline. Suspension/compromise holds preserve original clocks and deny content/new AI; a revoke does not silently clear a hold.

The canonical account/Profile markers override restored mutable account/session/Profile/history rows. Ordinary career access remains closed when a tombstoned Profile row reappears. Draft readers independently consult its Profile marker. An original admission cutoff also denies restored pre-deletion Actions, including old Profile-absence bindings, without closing genuinely new post-fence Actions. Money, replay records and remote holds remain intact.

The restricted ledger receives only parameterized boolean deny predicates, fully qualified to a validated owning schema with fixed `pg_catalog, pg_temp` search path and explicit EXECUTE grants. It receives no raw deletion/status-session rows. Inherited/default parent table privileges are removed during the explicit migration.

## Downstream recovery handoff

`NewDeletionRecovery(store, independentlyOwnedKey)` is an explicit code-injected recovery/migrator capability; no HTTP route or environment loader creates it. `Export` produces typed version `careeros-deletion-fences-v1`: original owner/operation/Profile identities and revisions, request/replay binding, closure generation, original fence/removal/residual anchors, minimal closed-subject deny mapping and known/unknown access-end/hold evidence. It includes no career payload, credential, raw prompt or provider response. HMAC sealing uses the independently supplied deletion/deny key, not an AI input-binding key.

`Apply` verifies that seal and typed bounds, commits protective markers before cleanup and never opens the restored service, grants rights or releases provider holds. It preserves original cutoff/anchors and new independent Profile/Action data. Missing or conflicting original authority fails closed. Temporary suspension and nullable unknown-end holds survive without inventing a timestamp. Downstream restore must reconcile independently surviving current authority before opening; these local fixtures do not prove that hosted condition.

## Browser boundaries

Actual React EN/PT controls present consequences before confirmation. Export remains optional. During an uncertain confirmation, protected workspace copies are withheld while opaque identity and metadata status remain recoverable. Known same-owner deletion invalidates pending Save/recovery, optimistic/Profile and retained AI/Draft editor state; browser cleanup failure cannot keep an affected recovery candidate visible. Cross-tab signals contain only account/operation metadata and cause fresh closed-default loading. Pending same-owner export responses are invalidated before they can create a download. Foreign-owner transitions remain independently usable; status-only identity disposes ordinary workspace state before any Profile read.

The tests use private controlled bootstrap/transition routes solely in a Go test binary, real React/Vite/Go/private PostgreSQL, and a separate real fsync PostgreSQL crash/restart with a fresh child process and durable no-inference witness. They are not provider, participant or hosted recovery exercises. Managed security scanner setup is unsupported in this linked worktree; final security assessment must explicitly use independent immutable source/evidence review rather than claim a completed managed scan.

# Closed pilot runtime preparation

[#292](https://github.com/HenriqueMichelini/careeros/issues/292) packages the retained React/Vite application, shared Go Account/pilot router and PostgreSQL storage adapter for a disposable local rehearsal. The [owner’s local preparation decision](https://github.com/HenriqueMichelini/careeros/issues/292#issuecomment-6644607874) leaves the hosted runtime, database provider, origin, region, costs and recovery targets open. The [React/Vite and Go decision](https://github.com/HenriqueMichelini/careeros/issues/259#issuecomment-6391838343) preserves the existing Profile and artifact-review boundaries.

## Reproduce and inspect the package

Use the packager from the selected commit and pre-cached npm and Go dependencies. It extracts that commit, performs an offline lockfile install and production build, and compiles the Go runtime and a guarded command for the existing account migration. The builder inherits only tool discovery, the cache home and optional Go cache locations. The build profile fixes the effective Go target, architecture level and experiments and disables direct module or VCS access. npm uses offline mode and the fixed system script shell. The manifest pins the commit, tree, build constraints, lockfiles, account schema, route adapter configuration, bootstrap/grants inputs and every delivered file.

```sh
rtk proxy python3 scripts/package-closed-pilot.py build --revision <reviewed-commit> --output <new-local-directory>
rtk proxy python3 scripts/package-closed-pilot.py verify --bundle <bundle> --expected-manifest-sha256 <trusted-digest>
rtk proxy python3 scripts/test-closed-pilot.py --bundle <bundle> --expected-manifest-sha256 <trusted-digest> --evidence <new-local-evidence-directory>
```

Use the independently reviewed manifest SHA-256 or the digest emitted by a just-completed build from the reviewed source in an owned directory. Do not calculate the expected digest from an untrusted bundle. The verification runner executes only its private snapshot after matching the expected digest and all payload bindings. For manual use, verify and keep the executable in an owned directory.

The command uses the existing Account and pilot route contracts and five existing direct workflow aliases. It normalizes recognized aliases to the corresponding canonical route before delegation, including the Account alias for Draft allowance. All API and Function names are reserved before static handling, including unknown routes. The explicit Netlify Function adapter remains in the v1 release path and is checked at the event boundary. The closed frontend uses the existing local font fallbacks, and the packaged browser check intercepts and refuses any request outside its loopback origin. The regular V1 build retains its stylesheet.

## Startup and configuration

Run the packaged `bin/pilot-runtime-local -listen 127.0.0.1:8787 -static frontend` from the verified bundle directory. The listener must be a literal loopback IP with an explicit port. The exact origin is the HTTP origin of that listener. Forwarded hosts or origins cannot substitute for it. Session reads use the existing host-only cookies, no-store and same-origin response behavior. Unsafe Account routes require the exact origin and the existing session-bound CSRF header/cookie match. The mounted package accepts existing synthetic sessions but offers no sign-in or provider exchange.

| Local input | Purpose |
| --- | --- |
| `CAREEROS_PILOT_LOCAL_DATABASE_URL` | Dedicated runtime reference with role `careeros_pilot_runtime`, database `careeros_pilot_runtime_fixture` and literal loopback TCP authority. Use a credential-free URL with no query or fallback. |
| `CAREEROS_PILOT_LOCAL_MIGRATOR_DATABASE_URL` | Only the separate `bin/pilot-runtime-migrate` process receives the disposable migrator reference. It requires a credential-free literal loopback URL for the exact fixture database and role `careeros_pilot_migrator`, with no query or fallback. The actual connection must use the isolated schema owned by that role. |
| `bin/pilot-runtime-local -schema-id` | Prints the exact embedded Account migration digest required by `sql/closed-runtime-grants.sql`. |
| `build-profile.json` | Enables the Account, pilot and Draft Recovery frontend views. Identity exchanges, invitation activation and managed routes stay closed at the API. |

The runner creates a disposable PostgreSQL cluster, installs the fixed local role and schema prerequisites, checks startup before migration, then executes the real migrator and runtime-grants scripts. It verifies the runtime role, the configured authorization and revoked access, and stops the cluster. For manual rehearsal, use the same migration steps in an independently owned disposable cluster.

1. Create the dedicated database, then run `sql/bootstrap.sql` as its disposable administrator.
2. Run the matched `bin/pilot-runtime-migrate` as `careeros_pilot_migrator`. It checks the actual database, schema owner and login in the same transaction before DDL, takes the existing migration and control locks, and requires already installed controls to stay closed. Startup performs no migration. The generic Account migrator remains a separate application command.
3. As the same migrator, run `sql/closed-runtime-grants.sql` with `-v schema_id=<printed-schema-id>`. Repeat the grant/identity step after migration because the Account schema reapplies purpose-specific revocations. Review the closed shared invitation and inference state.
4. Start the packaged runtime using the restricted runtime role. `GET /healthz` checks the process. `GET /readyz` checks identity, role restrictions, closed controls and reachable PostgreSQL. `200` from readiness includes explicit closed invitation and inference fields. Missing environment, wrong schema identity, a stopped database or an open control causes unavailable readiness and typed unavailability for known API routes.

The local Go listener sets 3 s header read, 10 s read, 15 s write and 30 s idle deadlines, a 16 KiB header-size setting, an API body maximum of 5 MiB and eight concurrent dependency-facing requests including readiness. Individual handlers retain their lower bounds. The Account pool has eight open and two idle connections with a 30-minute maximum connection lifetime. The disposable role has an eight-connection limit, with 2 s connect/lock and 5 s statement/idle-in-transaction settings. These are local bounds for rehearsal, not a hosted capacity claim.

## Role and feature boundaries

The migrator owns its isolated schema and is the only package role with DDL. The runtime can read the Account/session, pilot status, deletion/lifecycle status and Profile views reached by the existing authorizers. It preserves existing session logout and monotonic generation revocation. It has no role management, schema CREATE, temporary objects, ledger access, operator grants, profile writes or invitation/inference mutation. Account and restricted-rights reads still go through the existing session, owner and status checks.

The prior managed fixtures keep their independent role-bound pools in the same disposable PostgreSQL database. Their generated runtime, reconciliation and support roles have distinct privileges. Their migrator and explicit synthetic bootstrap remain outside this package. No managed ledger, fake dispatch or provider credentials are installed by the closed executable. A later candidate can compose the existing managed router and Draft Recovery adapter at the same Account and route boundaries, after the corresponding local/hosted authorization, migrations and exact role evidence. This rehearsal cannot serve live deletion requests or other write rights. A hosted cutover must keep independently safe rights handling available before traffic moves.

## Rollout and rollback

Freeze binaries, frontend assets, migration identity, routes, role grants and configuration together. Stage immutable artifacts separately from the current runtime. Check exact origin, cookies, CSRF, canonical and direct routes, typed failures, startup/health and bounded connection behavior against the closed candidate before any rollout.

For any separately adopted feature-specific schema, use Account migration first, then Draft Recovery schema and the applicable restricted admission/reconciliation migrations for their existing authorities. Verify order, exact identities and allowed privileges before mounting new interfaces. Keep new fields additive until every serving binary has been verified against the resulting schema. Never reverse a migration merely to restore an earlier binary.

Before replacing a serving binary, close invitations and inference under current authority, serialize with the independent dispatch/protected-response fences, and retain accepted Profile/Draft writes, confirmed deletion and other rights handling through their owning stores. Drain the old process within its bound. Keep unresolved expense, original receipt identities, unsafe-to-repeat work, status and holds intact. A process exit, timeout or retry does not prove provider terminality. Reconcile in-flight work against the authoritative ledger and exact mutation receipts. Preserve [the separate verified reopening rules](managed-operator-controls-local.md).

Rollback selects a previously verified **closed** binary/configuration with demonstrated compatibility against the current schema, role and retained evidence. The local package refuses a different schema fingerprint by default. Keep closed controls and retain the current database and monotonic authority. If the old reader requires a destructive reverse migration or lacks the current fencing/rights contract, hold closed and repair forward with compatible code. Keep in-flight receipts and people’s existing retrieval/deletion rights under independent verified authority. Recovery from older backups requires its own evidence and exclusion of the whole old system before generation advance. A local process restart verifies only the current package and database.

## ADR scope

[ADR 0001](adr/0001-stateless-user-key-backend.md) and [ADR 0002](adr/0002-go-api-on-netlify-free.md) record v1's browser-local, user-key, stateless Netlify topology. They remain historical and applicable to that release track. The approved bounded Account and managed pilot tracks use server-owned Account/Profile authority and managed inference limits in their authorized synthetic scope. This preparation does not extend the old user-key or stateless decision to those tracks, select a new host, or substitute a rehearsal for [#52](https://github.com/HenriqueMichelini/careeros/issues/52).

The [separately approvable hosted proposal](closed-pilot-hosted-proposal.md) lists the exact configuration and evidence required for any hosted candidate.

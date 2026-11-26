# Closed hosted candidate for #294

## Owner decision

On 24 November 2026, Henrique approved Cloud Run and Neon Launch in São Paulo for [#294](https://github.com/HenriqueMichelini/careeros/issues/294) and authorized local candidate preparation. Provisioning, hosted verification and the internal test grant each require separate approval. Participant admission stays closed. [Decision basis and cost research](research/careeros-294-closed-hosted-candidate-evidence-2026-11-24.md)

## Prepared configuration

| Component | Local preparation and values to review |
| --- | --- |
| Service | Cloud Run `southamerica-east1`. One synthetic project, one service and one default HTTPS origin. The proposed service is `careeros-294-closed` with request-based billing, 1 vCPU, 512 MiB, instance range 0–1 and concurrency 8. Pin the project, exact origin and compatible rollback images and configuration before provisioning. Reject alternate hostnames at the application. Measure actual overlapping connection pools and route headroom. |
| Serving artifact | The closed Go runtime serves the packaged Vite frontend and API from one origin. It loads a scoped Account database role, the pinned Google OIDC client and an exact issuer/subject allowlist. Startup verifies both durable stops. Browser traffic reaches the process at `0.0.0.0:$PORT` behind platform TLS. Configure the platform startup probe for TCP and check the origin-specific `/readyz` separately. |
| PostgreSQL | One Neon Launch project in `aws-sa-east-1`, with 0.25 CU proposed. Use TLS-verified direct connections to isolated database and schema `careeros_294`. Pin password-bearing URLs as secret references to runtime role `careeros_294_runtime` and migration role `careeros_294_migrator`. The separate migration package has the Account schema. The serving process checks privileges and readiness without migrating at startup. |
| Auth | Pin `CAREEROS_PUBLIC_ORIGIN` and `CAREEROS_GOOGLE_REDIRECT_URI` to the exact origin and its `/api/auth/google/callback` respectively. Bind references for `CAREEROS_GOOGLE_CLIENT_ID`, `CAREEROS_GOOGLE_CLIENT_SECRET`, `CAREEROS_HOSTED_GOOGLE_SUBJECTS` and `CAREEROS_DATABASE_URL`. The allowlist holds the exact approved Google issuer and subject for each synthetic identity. Proposed timers are `CAREEROS_SESSION_TTL=30m`, `CAREEROS_TRANSACTION_TTL=10m` and `CAREEROS_JWKS_CACHE_TTL=1h`. Verify the exact auth client and test all persistent log routes for exclusion of raw requests and callback query strings before the first real callback. |
| Bounds and operations | The proposed attended window is four hours including cleanup. Price the exact all-in envelope including platform, backups, egress, logs, notifications, retention and operator time. Pin the operator inbox/status, privileged MFA and recovery method, the backup health observation and copy expiry. The selected plans do not establish an invoice ceiling, backup restoration, or old-system exclusion. |

The [original preflight](closed-hosted-candidate-preflight.md) retains the observed checks, inherited failures and sign-off sequence. The [approval checklist](closed-pilot-hosted-proposal.md) owns the remaining configuration and execution inputs.

Follow the [offline packaging procedure](closed-hosted-pilot-packaging.md) to build the serving and migration images. The platform configuration will need a separately approved administrative database bootstrap using [the reviewed SQL](../internal/accountstore/hosted_bootstrap.sql). The migration image then uses `CAREEROS_HOSTED_MIGRATOR_DATABASE_URL` and applies the Account schema and [scoped runtime grants](../internal/accountstore/hosted_grants.sql) in a single transaction. Both binaries report the same Account schema and grant identity via `-schema-id`. Keep the runtime, migration and administrative secret references separate.

## Closed candidate boundary

The serving package uses real Google OIDC code and the existing pilot Account guard. The five workflows and their direct aliases share the same guards and have no transport mounted. Invitation activation, managed routes and Draft retrieval fail closed. Pilot Profile read, save and export retain the same grant authority as the ordinary pilot routes. An allowlisted Google identity can establish an Account Session but cannot acquire those Profile or workflow rights from sign-in alone.

The [Draft-only router](../internal/accountapi/managed_draft.go) reuses managed Draft publication, opening, and review with no other workflow or activation route. Its disposable PostgreSQL fixture exercises four fake calls and one confirmed allowance use across opening replay. The separate [Draft writer and attestor SQL](../internal/hosteddraft) records ordered intent and result evidence for one preapproved Account, Action, Profile revision, grant version, and inference epoch. Its fixture authenticates each role independently and verifies that neither role can use the other's privileges.

Hosted execution still needs to bind that evidence to the canonical managed Action, actual handoff lease, publication, and allowance settlement. The local managed authority is restricted to disposable PostgreSQL. The closed serving process neither connects to the Draft authority nor opens the test scope. Before serving a positive fake-only run, install the reviewed authority with separate credentials, pin the internal grant and approved Action, and verify that the complete route preserves canonical grant and stop checks. Admission stays closed.

## Local verification

The hosted runtime was exercised in-process against a fresh loopback-only PostgreSQL 18 fixture using verified TLS and SCRAM, with a signed synthetic Google exchange and JWKS transport. The fixture checked distinct sessions, rejected and interrupted callbacks, workspace routing, cookie and CSRF guards, all five canonical and direct alias denials, closed Profile access, and session persistence across a runtime restart. Readiness denied access on loss of the database, loss of the session insert grant, and excess write privilege on the operator role table. It used synthetic subjects, fixture-only database credentials and a temporary TLS key. Actual Google, Cloud Run and Neon observations remain at the hosted gate.

The scoped Profile fixture also verifies approved excerpt removal, rollback and receipt replay, a legacy receipt after migration, and denial of direct updates or reinsertion behind a pruning fence. A separate fixture verifies that the closed Account runtime opens and passes readiness after the staged Draft role schema is installed and the Account migration is repeated. The Chromium Draft recovery checks passed in English and Brazilian Portuguese, including a grant state reset that preserved the retrieval hold.

To repeat the local fixture, supply a private writable Go cache and the installed PostgreSQL binary directory.

```sh
rtk proxy env GOCACHE=/path/to/writable/go-cache GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local \
  CAREEROS_HOSTED_LOCAL_PG_BIN=/absolute/path/to/postgresql/bin \
  go test -race -count=1 ./internal/pilotruntime -run '^TestClosedHostedWithLocalTLSPostgres$'
```

The [OCI manifest](release-evidence/2026-11-25-closed-hosted-candidate-oci-manifest.json) records the separate serving and migration images from source `a753a2a376a0511576a880d486e8eb7bae900287`. Use that revision with the [offline build procedure](closed-hosted-pilot-packaging.md) and the reviewed CA input to reproduce the recorded digests from a fresh checkout. The added handoff records are outside this image source revision.

The current Account grant and schema identity includes the hosted Profile Save pruning fix. A closed binary from the earlier manifest checks its older schema identity at startup. A rollback against a migrated database requires a compatible closed binary which accepts the current identity and preserves the pruning authority. Rehearse compatibility before approving either hosted revision.

Before requesting external approval, pin the exact reviewed artifact and source, schema/grant identities, config, logging and egress, rollback, operator and backup settings, complete cost bounds, and status of the seven native dependencies. After configuration approval, request separate hosted verification and internal test-grant approvals for the specific candidate and window. [#294 acceptance and dependencies](https://github.com/HenriqueMichelini/careeros/issues/294) · [#295 recovery scope](https://github.com/HenriqueMichelini/careeros/issues/295)

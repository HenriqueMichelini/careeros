# New-account Profile persistence

This is the first bounded local persistence slice for **new accounts**. Its storage selection is recorded in the [architecture decision](migration/02-technology-architecture/README.md). It extends merged PR #270, base `591b2889196f277c96b130192a68bbc700a2137f`; it does not repeat or supersede the earlier repair evidence.

## Scope and retained data

- Relational accounts/provider subjects, account status and full unsigned authorization generation; opaque session/CSRF **hashes**, issuance/expiry; one-use OIDC state/binding hashes plus short-lived nonce, PKCE verifier and exact callback context.
- One current Profile per account, including approved excerpts, origins and fact links. No previous full Profile snapshots are added. Receipts retain only operation ID, request hash, Profile ID and historical committed revision, under the account key.
- No provider access/refresh/ID tokens, complete submissions, uploaded CV files, prompts, Draft history, billing/allowances or job queues are added to durable storage. Existing account email/display-name fields are preserved from the auth contract; email is never a mapping key.
- #258 D02–D04 apply: only approved excerpts linked to remaining facts may persist. Removing a fact removes its affected shared excerpt and invalidates support on retained facts. The client applies this selected domain deletion rule before Save; Go rejects a violating document instead of silently stripping it. Historical invalidated link identifiers may remain without deleted excerpt content. This adds no account-deletion or operator API.

The account workspace never opens, uploads, claims, merges or deletes `careeros_profile_v2` or `careeros_repo`. BYOK keys start empty and remain session-only in account mode. Existing optional CV snapshots/choices stay in the browser under an account-specific namespace; none are imported from old keys or treated as server Profile authority. Locale is a shared presentation preference. Account switching remounts all application/editor/result state and invalidates the previous client's responses. Same-account focus checks hide the workspace while verifying the session and preserve its pending edits.

## API and entrypoints

| Endpoint | Behavior |
| --- | --- |
| `GET /api/auth/google/start`, `GET /api/auth/google/callback` | Existing state/binding, nonce, PKCE, allowlist and Google token verification; durable one-use transaction and account mapping. |
| `GET /api/auth/session` | Current session probe; storage outage is 503, preserves cookies and never masquerades as signed-out success. |
| `POST /api/auth/logout`, `POST /api/auth/logout-all` | Existing ordinary logout vs atomic sign-in replacement contract; all-device generation invalidation. |
| `GET /api/profile/current` | Authorize and read this account's current Profile, or `document:null` for a clean account. Never accepts a resource owner from the URL. |
| `PUT /api/profile/current` | Explicit create/Save with `{operationId,expectedRevision,document}`; no additional envelope fields. |
| `GET /api/profile/operations/{operationId}` | Current-authorized lookup of this account's historical outcome **and current Profile**, with no mutation/provider dispatch. Foreign/missing operations return 404. |

All Profile endpoints require a verified opaque session and `X-CareerOS-Account` matching that server principal. This header is a consistency precondition to reject A's queued request sent after browser cookies switch to B; it supplies no ownership authority. Unsafe requests also require the exact public Origin and the per-session CSRF header/cookie. Unauthenticated, expired, revoked, suspended, deleted or stale-generation access is denied. Missing configuration/storage fails closed. Responses are `no-store`.

`cmd/profile-review` mounts these routes using shared PostgreSQL stores. The new `netlify/functions/account` adapter mounts **the same protected router**, for original and direct function URLs, with dedicated rewrites in `netlify.toml`. New endpoints do not depend on `CAREEROS_REQUIRE_AUTH` for protection.

**Existing routes outside hosted enforcement in this slice:** `/.netlify/functions/cv-generation`, `profile-ingestion`, `profile-review`, `qualification-gaps`, and `application-draft` (and their five `/api/...` rewrites) retain the existing guest/BYOK implementations. They are not made account-owned APIs here. `CAREEROS_REQUIRE_AUTH=1` remains a local-command guard for these workflows and does not protect those separate Functions. No hosted enforcement is claimed without deployment/runtime evidence. Existing stateless AI requests may receive explicitly submitted career content; this slice does not grant a server Profile read path to them.

## Transaction and recovery contract

Every Profile read, outcome lookup and Save locks the account row and then the exact session row. After the lock is acquired, it checks active account status/current generation and the DB-clock session expiry, plus the authenticated principal's private session-token binding. Profile publication and the operation receipt occur in that transaction. Ordinary logout, session replacement and bulk session revocation use the same account-before-session lock order; generation/status updates conflict on that account row. Cross-account sign-in replacement locks both accounts in sorted order and checks the initiating session's current status/generation too.

If revocation wins, no Profile/receipt is published. If Save wins, its one historical outcome remains committed, and subsequently revoked credentials cannot read it. `Revalidate` plus a separate resource write is **not** this boundary. Database/commit errors return unavailable without asserting rollback or success to the client.

`expectedRevision:0` means no server Profile. The first explicitly saved candidate may already have a valid positive domain revision from in-session edits; it is not renumbered. Existing Profiles require the same domain ID and exactly the expected current root revision, then the next root revision. Revisions are safe integers through `9007199254740991`; exhausted/unsupported commands reject explicitly.

The receipt key is `(verified account, operationId)`. Its SHA-256 binds the versioned Save intent, expected base and exact candidate JSON bytes. A duplicate request recognizes its completed outcome **before stale-base validation** and returns the current document without rewriting it. The same key with changed valid request bytes conflicts; a new stale operation conflicts. Whitespace/escaping changes are different request bytes, so clients freeze and reuse the original request. Operation IDs are 1–128 ASCII letters/digits, `-` or `_`.

The outcome's `committedRevision` describes that historical Save; `document.revision` describes the current Profile and may be newer. The client adopts the currently authorized document, never overwrites newer state with its old outcome and never dispatches AI to reconcile Save. A response lost after commit triggers a read of the original operation. If unresolved, the state remains **uncertain**, retains the frozen request/edits in memory and offers recovery download plus deliberate retry of that identical operation. Pending, failed, uncertain and confirmed states are distinct; no account write falls back to local storage. A local recovery download includes only this workspace's edits/pending Save, never legacy Profile keys or credentials. Reload/navigation can discard in-memory unsaved work: download it first. No automatic offline sync or recovery-file import is added.

## Representation bounds

The byte store preserves the accepted document bytes. Go validates the shared Profile shape/references/semantics without reconstructing and reserializing the stored document. Unicode including U+0000, supplementary characters, combining sequences and supported opaque JSON values round-trip. No text normalization, `jsonb` cast or silent number truncation is used.

The adapter explicitly rejects invalid UTF-8, unpaired UTF-16 escape surrogates, duplicate decoded object keys, nesting over 128 and non-finite/unrepresentable Go JSON numbers. The shared domain's safe-integer metadata bounds remain enforced. A Profile document is limited to 4 MiB, with a bounded envelope allowance; larger candidates return validation failure with the client draft retained. Unsupported content is not rewritten to fit PostgreSQL. Readable response JSON may use equivalent JSON escapes; the stored bytes remain unchanged.

## Local synthetic run and verification

Use an isolated local PostgreSQL 18 instance; no real data or Google credentials are needed for tests. An example synthetic-only DSN is `postgres://careeros_test:synthetic-local-only@127.0.0.1:55438/careeros_test?sslmode=disable`. `sslmode=disable` is only this loopback test example, not production advice.

```sh
rtk proxy env GOCACHE=/tmp/careeros-go-cache CAREEROS_DATABASE_URL="$SYNTHETIC_DATABASE_URL" go run ./cmd/account-migrate
rtk proxy env GOCACHE=/tmp/careeros-go-cache CAREEROS_TEST_DATABASE_URL="$SYNTHETIC_DATABASE_URL" go test -race -count=1 -timeout=120s ./internal/accountapi ./internal/accountstore ./internal/auth ./internal/netlifyproxy ./cmd/profile-review ./netlify/functions/account
rtk proxy env GOCACHE=/tmp/careeros-go-cache GOFLAGS=-buildvcs=false node --test --test-isolation=none tests/*.test.mjs
rtk proxy node_modules/.bin/tsc --noEmit
rtk proxy env VITE_CAREEROS_ACCOUNT_MODE=1 npm run build
rtk proxy npm run test:account-browser
```

`cmd/account-migrate` is an explicit additive schema command. It runs transactionally under a migration lock; application startup/HTTP handlers never create tables or provision a service. Use an appropriate server-only DSN via `CAREEROS_DATABASE_URL` after separately authorized runtime setup. The application pool has at most eight connections per process; production pool totals, TLS/roles/grants and serverless compatibility need provider-specific verification. `VITE_CAREEROS_ACCOUNT_MODE=1` builds the account workspace; without it the established browser-only experience remains. Existing Google/public-origin/TTL configuration is documented in [the auth note](migration/04-authentication-user-management/README.md).

Tests use separate per-test schemas, fake OIDC responses, two independent stores/handlers and two HTTP application subprocesses. They stop **all** application subprocesses, reconstruct them and retrieve the same Profile using the persisted session/account. Other cases cover two-owner/direct-request denial, CSRF, concurrent saves/duplicates, changed requests, stale bases, real TCP acknowledgment loss, receipt-insert rollback, deterministic revocation/expiry-vs-commit ordering, ordinary logout-vs-sign-in, unavailable storage and actual React account-switch/late-response behavior. These are positive and negative controls; an always-deny implementation cannot pass.

## Remaining gates

Production Google setup/live token and callback tests, provider/hosting/region/plan, TLS/secrets/grants/pool sizing, whole hosted route enforcement, abuse/availability controls, retention/cleanup schedules for auth/operation metadata, backup/export handling and deletion-aware recovery remain unverified or unselected as applicable. No arbitrary receipt-retention/replay horizon is invented; receipts contain no historical career content. Operational deletion/retention work must preserve receipt/revision safety before any production use. Numerical RPO/RTO, acknowledged-loss policy, disaster restoration and all-copy expiry require their own decisions/evidence. Application-process restart is not a database disaster-restore test or a zero-data-loss promise.

Legacy import, linking, account recovery/deletion/operator features, payments, allowances, durable Draft history and queues are excluded. This slice is distinct from production authentication delivery, complete SaaS delivery and launch readiness. Remote publication requires exact-text owner approval; every later push/PR creation also requires freshly verified Netlify deployment prevention. No merge or deployment is authorized here.

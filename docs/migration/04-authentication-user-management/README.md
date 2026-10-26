# Authentication and user management

**Status: owner-selected policy, implementation slice in progress. Not approved for provider configuration, deployment or launch.**

This note records the decisions behind the sign-in slice and the limits of what is implemented. The full local decision addendum and the #262 investigation artifacts stay outside the repository until the owner chooses to publish them.

## Selected policy

- **S04-D01 (2026-10-25):** CareerOS supports third-party sign-in only, through a small allowlisted provider set. CareerOS does not manage customer passwords. Account linking, lost-access recovery, MFA, account deletion and operator roles remain separate decisions.
- **S04-D02:** initial allowlist is **Google only**. LinkedIn, GitHub and any other provider require a fresh decision.
- **S04-D03:** integrate **directly with Google OIDC in Go**. No broker.
- **S04-D04:** use an **application-owned opaque session plus ownership guard**. The browser holds a random token in a `Secure`, `HttpOnly`, `SameSite=Lax`, `__Host-` cookie; the server stores only its hash with the account ID, expiry and authorization generation. Unsafe methods require an exact same-origin request and a per-session CSRF token. Logout revokes one session; all-device logout increments the account generation and revokes every session.

No provider assertion is CareerOS ownership by itself, and email is never an account key.

## Implemented slice

`internal/auth` contains:

- a closed provider registry (Google only when configured) and a direct Google authorization-code + PKCE S256 flow;
- one-use server-side transactions bound to a separate browser cookie, with nonce and exact redirect URI;
- RS256 ID-token validation against a cached JWKS, including signature, `iss`, `aud`/`azp`, `exp`, `iat`/`nbf`, `nonce` and subject checks;
- account mapping keyed by `(provider, issuer, subject)`; the same email under two subjects creates two accounts;
- opaque sessions, hashed token storage, per-session CSRF secrets and account authorization generations;
- deny-by-default `Protect`/`ProtectFunc` middleware, `AuthorizeOwner` and `Revalidate` for pre-commit checks;
- HTTP routes for `GET /api/auth/{provider}/start`, `GET /api/auth/{provider}/callback`, `GET /api/auth/session`, `POST /api/auth/logout` and `POST /api/auth/logout-all`.

`cmd/profile-review` now creates one auth service per process, mounts the auth routes and can protect the five existing workflows with `CAREEROS_REQUIRE_AUTH=1`. When protection is enabled without configuration, those routes fail closed with `503 auth_unavailable`.

`internal/netlifyproxy` now preserves query parameters and emits multi-value response headers, including multiple `Set-Cookie` headers.

## Configuration

The service reads:

- `CAREEROS_PUBLIC_ORIGIN` — exact public origin, HTTPS except on loopback.
- `CAREEROS_GOOGLE_CLIENT_ID` and `CAREEROS_GOOGLE_CLIENT_SECRET` — required for Google.
- `CAREEROS_GOOGLE_REDIRECT_URI` — optional only if it exactly equals `<origin>/api/auth/google/callback`.
- `CAREEROS_SESSION_TTL` — default `12h`, allowed range 5 minutes–30 days.
- `CAREEROS_TRANSACTION_TTL` — default `10m`, allowed range 1 minute–1 hour.
- `CAREEROS_JWKS_CACHE_TTL` — default `1h`.
- `CAREEROS_REQUIRE_AUTH=1` — protect the five existing workflows instead of preserving guest/BYOK behavior.

No credentials are committed or configured by this slice. Provider endpoints are fixed for Google; the redirect URI must match the configured origin exactly.

## Static-analysis notes

- Session and OIDC-binding cookies are `HttpOnly`. The CSRF cookie is deliberately script-readable because the double-submit pattern requires same-origin JavaScript to copy it into the request header. It is not an authentication secret and is also verified against the server-side session record. See the [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html#javascript-automatically-including-csrf-tokens-as-an-ajax-request-header).
- ID-token signatures use RSASSA-PKCS1-v1_5 because the `RS256` algorithm is defined that way in [RFC 7518 section 3.3](https://www.rfc-editor.org/rfc/rfc7518.html#section-3.3), and Google advertises `RS256` as its ID-token signing algorithm. This is signature verification, not RSA encryption.

## Test evidence

```sh
go test ./internal/auth ./internal/netlifyproxy ./cmd/profile-review
go test -race ./internal/auth
npm test
npm run build
```

The synthetic tests cover denied providers, callback state/binding tampering, replay and concurrent consumption, nonce/issuer/audience/expiry/signature failures, JWKS rotation, same-email/different-subject isolation, hashed-token storage, expired/suspended/deleted accounts, single and all-device logout, login/logout races (including a provider response that arrives after logout), late callbacks, cross-account ownership, CSRF/origin checks and pre-commit revalidation. They use fake provider responses and sentinel data; no live account, email or provider call is involved.

## Remaining limitations and gates

- **Session persistence is unselected.** The reference store is in-memory. Netlify functions run as separate processes, so the auth routes and guard are mounted only in the local single-process server. A durable shared store is required before deployment can enforce sessions across functions.
- **Provider configuration is unauthorized.** There are no Google credentials, live sign-up/email tests or provider calls. Enabling the flow requires separate owner authorization.
- **No account linking, email ownership transfer or lost-access recovery.** Adding any of these is a new decision; nothing in this slice infers them.
- **No client sign-in UI.** The browser-local Profile and explicit Apply/Save behavior are unchanged. Sign-in does not claim, upload, merge or delete local Profile bytes.
- **No suspension/deletion/operator endpoints.** The guard checks status and generation, but only tests can set suspension/deletion.
- **Legacy guest/BYOK access remains explicit.** The five workflows are not account-owned resources. Deployments that make them account-owned must set `CAREEROS_REQUIRE_AUTH=1` and provide the shared session store first.
- Deployment, spending, launch and publication of the underlying investigation remain separate gates.


## Additive review correction — 2026-10-26, PR #270

The earlier test-evidence paragraph is historical: at reviewed head `2d5a3f3e27281833a2b92032d5d27de1a83db8ab`, the provider-response race test exercised all-device logout, not ordinary logout. Independent HTTP probes reproduced a replacement session being issued after ordinary logout in both callback schedules. The repairs and failing-before/passing-after regressions are recorded in [the repair evidence](../../release-evidence/2026-10-26-pr270-auth-repairs.md).

`SessionStore.Create` now atomically checks the initiating session, removes it and inserts its replacement, serialized with revocation. Revocation winning before that store commit rejects the earlier sign-in; insertion winning first is ordered before revocation. A missing, revoked or expired initiating session cannot authorize insertion. Ordinary logout remains confined to that session: another device stays valid, the account generation is unchanged and a genuinely fresh sign-in is allowed. Future stores must preserve this atomic contract; cookie clearing and account-generation checks alone do not provide it.

`CAREEROS_JWKS_CACHE_TTL` now reaches the production Google provider constructor and token verifier. It supplies the fallback when there is no usable positive `Cache-Control: max-age`. A usable provider `max-age` takes precedence over configuration. Both paths retain the existing 5-minute minimum and 24-hour maximum; the default fallback is 1 hour. Offline constructor regressions check cached reuse before expiry and refetch at expiry, including configured/default fallbacks, header precedence and both bounds.

`Revalidate` alone does **not** establish atomic revocation-versus-resource-commit safety. It separately reads the session and account and rejects revocation already visible to those reads. A later resource write can still race revocation. Future durable persistence must atomically compose trusted owner lookup, current session/account checks and the authoritative resource write. No durable resource commit is implemented or validated here.

The prior PR/issue report of 13 top-level failures / 22 subtest failures and 24 unrelated stale files is retained as historical, superseded evidence. The independent review reproduced **13 top-level failures and 11 distinct named failing subtests**. Manifest stale totals are **25 at baseline** (24 hash mismatches plus one missing path) and **28 at the reviewed head** (27 mismatches plus the same missing path). Three newly stale entries are patch-owned: `internal/netlifyproxy/handler.go`, `internal/netlifyproxy/handler_test.go` and `vite.config.ts`. The repair retains the same 28 stale entries. The ProcessRunner manifest is unchanged and has not been re-approved.

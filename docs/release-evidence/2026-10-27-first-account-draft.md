# First account Application Draft — local evidence, 27 October 2026

Base: `fe00b77aa327b1087377874bcc469b6487f2993d`, verified against remote main using an authenticated fetch. Branch: `codex/first-account-draft`. Worktree: `/home/henriquemichelini/.codex/worktrees/first-account-draft/careeros`. The delivery message supplies the exact final commit containing this evidence. The primary checkout and its unrelated changes were preserved.

Read-only GitHub preparation inspected #255's approved S06-D01–D05 artifact/action definitions, #271's merged persistence acceptance/limitations and #262's selected identity/session foundation. The current user instruction authorizes this bounded local implementation; it does not select or implement the deferred commercial/recovery work.

## Demonstration

Use a disposable local PostgreSQL database containing only synthetic data. This example uses the already-installed Docker image; `--pull=never` prohibits a download. Port 55439 must be free. Stop/remove the container afterward; its database is disposable.

```sh
rtk proxy docker run --pull=never --rm -d --name careeros-draft-synthetic -e POSTGRES_USER=careeros_test -e POSTGRES_PASSWORD=synthetic-local-only -e POSTGRES_DB=careeros_test -p 127.0.0.1:55439:5432 postgres:18-alpine
rtk proxy env GOCACHE=/tmp/careeros-go-cache 'CAREEROS_TEST_DATABASE_URL=postgres://careeros_test:synthetic-local-only@127.0.0.1:55439/careeros_test?sslmode=disable' npm run test:account-journey
rtk proxy docker stop careeros-draft-synthetic
```

The test builds a **test-only** Go fixture, starts isolated loopback Vite/Chrome processes and produces a temporary evidence directory reported as `output`. It uses the real React app, authentication/session APIs, PostgreSQL Profile store and shared workflow guard. OIDC redirects/exchanges are synthetic and stay on loopback. AI responses are deterministic browser fixtures supplied only after the actual protected workflow router authorizes the request. Production AI handler contracts are exercised separately with deterministic HTTP transports. This composed UI check is not a live Google/model-quality observation.

Both EN/1440px and PT-BR/390px runs demonstrate:

1. Signed-out entry → synthetic sign-in → clean new-account Profile; no legacy claim.
2. Apply blocked before a useful saved Profile, with an Open Profile action.
3. Paste professional information → prepare/review/select changes; server Profile remains absent before explicit Save.
4. Confirmed PostgreSQL Save; keyboard Continue to Apply transfers focus. No standalone CV is created or required.
5. Paste a job → review job context → deliberate Draft generation. Captured request contains that account's saved Profile identity, revision and facts.
6. Separate personal acceptance of application materials and resume; actual native clipboard copy, existing print/export button and generated A4 PDF. Profile bytes remain unchanged.
7. Malformed/incomplete generation, service failure and cancellation preserve prior results; counts remain stable without automatic retry.
8. A later valid candidate keeps a previous session candidate. Reload and same-account sign-in restore the saved Profile. Switching to a clean second account hides the first account's facts/results; switching back restores the original saved Profile. AI keys are not written to browser storage.
9. The job textarea has a programmatic label; the relevant navigation uses keyboard focus transfer; the mobile page does not overflow horizontally.

The tiny synthetic bundle intentionally checks mechanics rather than model quality. Existing broader EN/PT review and ingestion fixtures cover richer content, conflicting/unsupported claims, manual review and export guards.

## Verification

| Check | Result |
| --- | --- |
| Scoped Go `-race`, real PostgreSQL, auth, store, shared router, all Function adapters, production Draft/qualification handlers | Pass |
| Missing/insufficient Profile, two-account substitution, tampered career data, switched cookies/account header, CSRF, expired/suspended sessions | Pass; denied calls never reach generation |
| Positive A/B generation authorization, direct/public adapter paths, forced review flags | Pass |
| Concurrent Profile change, cancellation context and late result after revocation | Pass; stale/private result withheld |
| Existing persisted-session/process restart and Save/replay/race contracts | Pass in the real-engine scoped run |
| Frontend contracts | 316 pass |
| TypeScript and scoped Go vet | Pass |
| Account-mode production build and separate Lab build | Pass |
| New composed EN/desktop and PT/mobile journey | Pass |
| Existing account browser and replacement/expiry/lifecycle browser scenarios | Pass; lifecycle rerun after a transient DevTools navigation-context error |
| Existing resume/application support-review browser matrix, EN/PT at 390/1440px | Pass |
| Existing whole-ingestion rich review/failure/keyboard matrix, EN/PT at 390/1440px | Pass on bounded rerun |
| Export QA | Both generated resumes are one-page A4 PDFs; rendered PNGs inspected for clipping/overlap |

Reproduce the scoped source checks:

```sh
rtk proxy env GOCACHE=/tmp/careeros-go-cache 'CAREEROS_TEST_DATABASE_URL=postgres://careeros_test:synthetic-local-only@127.0.0.1:55439/careeros_test?sslmode=disable' go test -race -count=1 -timeout=180s ./internal/accountapi ./internal/accountstore ./internal/auth ./internal/netlifyproxy ./cmd/profile-review ./netlify/functions/... ./backend/application-draft ./backend/qualification-gaps
rtk proxy env GOCACHE=/tmp/careeros-go-cache GOFLAGS=-buildvcs=false node --test --test-isolation=none 'tests/*.test.mjs'
rtk proxy node_modules/.bin/tsc --noEmit
rtk proxy env GOCACHE=/tmp/careeros-go-cache go vet ./internal/accountapi ./internal/qualificationmatching ./cmd/profile-review ./netlify/functions/...
rtk proxy env VITE_CAREEROS_ACCOUNT_MODE=1 npm run build
rtk proxy npm run build:lab
rtk proxy npm run test:account-browser
rtk proxy npm run test:account-sessions
rtk proxy node tests/resume-review-browser.check.mjs
rtk proxy timeout 90s node tests/submission-journey.check.mjs
```

## Inherited failures and evidence limits

Whole-Go runs at the untouched base and candidate both fail the **same 13 top-level Evaluation Lab ProcessRunner tests**. All other tested packages pass. The fixed reviewed-source manifest was already stale for **34 paths** at base; this slice increases that to **45**, with 11 additionally stale feature-touched paths listed in the retained inventory. No manifest change/reapproval or whole-Go green claim is made. A normal whole-Go invocation skips real PostgreSQL tests unless its explicit synthetic DSN is supplied; the scoped run above supplied it.

Both exact-base and candidate account builds emit the existing Vite `__dirname`/future native-config-loader warning and >500 kB chunk warning. They build successfully. Generated `dist` assets and Vite metadata are excluded from the commit.

The first additional whole-ingestion browser run stalled on the last Portuguese mobile failure/keyboard step after seven matrix checkpoints. A bounded rerun completed the entire matrix successfully; both observations are retained separately from inherited source failures. Initial browser setup also exposed an accidental nested dependency symlink; it was removed before the passing runs. No production workaround or dependency-version change was added.

Retained artifacts are under [first-account-draft](first-account-draft/); text logs normalize trailing whitespace only. The [behavior/limits note](../account-first-draft.md) states remaining guest/build-mode and persistence limits. Local browser/real-engine results do not demonstrate hosted route deployment, complete durable Draft delivery, model quality or launch readiness.

No push, PR creation, issue/comment mutation, merge, deployment, live Google/AI service, provider contact or spending occurred.

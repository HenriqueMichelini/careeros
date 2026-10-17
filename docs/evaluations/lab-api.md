# Secured local Lab API

Contract: `lab-api-v1`. Launch from repository root with `go run ./lab/api`. `--port 0` selects an available loopback port; `--data-dir` configures private history; `--assets-dir` configures trusted local Lab assets (default `lab/web`). No browser input can choose paths, executables, provider hosts or credentials. The launch process reads only the fixed retained synthetic fixtures and known source directories. It never accesses product Profile/key storage. No production handler or Profile review server is extended.

The listener binds `127.0.0.1` explicitly. The printed `/lab/` URL contains no secret. The launch shell validates the exact Host, rejects foreign Origin/cross-site fetch metadata, and supplies random in-memory session and CSRF authorization to its own page. Reloading the same process uses the same session; another launch generates new values. Do not copy, log or persist these values. Static `lab.js`/`lab.css` are same-origin code assets; every content-bearing API read requires `Authorization: Bearer <session>`. Every mutation additionally requires exact `Origin` and `X-Lab-CSRF`. Browsers may omit Origin on same-origin GET; an Origin supplied on any request must match exactly. Foreign or null origins, unexpected hosts and cross-site fetches receive 403. No CORS headers are emitted.

Responses use `Cache-Control: no-store`, `X-Content-Type-Options: nosniff` and `Referrer-Policy: no-referrer`. The shell has a restrictive CSP and cannot be framed. Authorization/CSRF values appear only in the authorized launch page/runtime memory and headers, never URL parameters, logs, history or exports. There is no access log or provider/live mode. Header, body, depth, query, catalog, plan, event and history limits are explicit. Unknown request fields are rejected, including nested fields. JSON mutations require a single object and `application/json`.

## Routes

| Method/path | Request | Success |
| --- | --- | --- |
| GET `/lab/api/suites` | no query | version and synthetic catalog |
| POST `/lab/api/plans` | `{selection, caps}`; 64 KiB max | frozen Plan; validation only |
| POST `/lab/api/runs` | `{plan, idempotencyToken}`; 256 KiB max | 201 `{id}` on admission; 200 same original ID on replay |
| GET `/lab/api/runs` | optional suite, engineState, offset, limit | bounded RunPage; default 20, max 100 |
| GET `/lab/api/runs/:id` | no query | live RunRecord, durable terminal RunRecord, or retained ImportResult |
| GET `/lab/api/runs/:id/events` | after, limit, epoch | bounded events, kind, epoch, next and explicit gap |
| POST `/lab/api/runs/:id/cancel` | `{}` | `{id, cancelRequested:true}`; idempotent |
| GET `/lab/api/runs/:id/cases/:caseIdentity` | no query | lazy case envelope, including sequence children |
| POST `/lab/api/comparisons` | `{leftId,rightId,changedDimension}`; 4 KiB max | existing strict per-case comparison contract |
| POST `/lab/api/imports` | `{originalBytes,dependencies}` or `{exportBundle}`; byte arrays use JSON base64 | 201 `{id}`; bounded validated import/quarantine |
| GET `/lab/api/runs/:id/exports` | required format: json, original, markdown or diagnostics | selected approved synthetic export |

Imports have a 16 MiB HTTP envelope bound; the existing decoded 8 MiB bundle/depth/dependency limits remain authoritative. Imports never execute or authorize inference. Quarantine is readable only with session authorization and cannot enter trusted comparisons or approved exports. Run/case IDs are identities, never paths. Only documented query keys are accepted. Invalid/corrupt evidence produces an explicit unavailable response. Comparison dimensions come from stored evidence, never client-supplied version claims. Native records retain their exact frozen fixture hash set and suite version. Their label version conservatively hashes the complete fixture set (including inputs and fixed outputs); their prompt and strategy versions conservatively hash the captured source envelope. These broader versions detect changes but do not isolate an individual prompt/strategy edit. Configuration-only paired comparisons work when every other captured dimension matches. Missing historical metadata or absent source snapshots still make the shared contract decline rather than fill in guesses. Runtime versions include Go, OS and architecture.

Errors are JSON `{version,code,message}` with fixed content-free messages: 400 invalid request/plan/query/cursor; 403 missing authorization or invalid local origin; 404 unavailable identity/endpoint; 409 busy, idempotency conflict, changed event epoch or unavailable comparison/export; 503 unavailable history or unhealthy persistence before a new admission. Storage/provider/internal error strings and local paths are not returned. Unsupported HTTP methods receive the generic unavailable endpoint response after authorization checks.

## Admission, cancellation and restart

Plan validation binds its registered ID/hash, exact fixture/configuration/order/provenance and explicit caps. An altered or old process plan cannot start. Admission is atomic with one sequential active run. An idempotency token is 16–128 letters/digits/underscore/hyphen; it is a request identity, not authorization. One token/plan yields exactly one run identity even under concurrent requests. Reusing it for a different valid plan returns 409. The table is in-memory, bounded to 1,024 entries per launch; plans and sessions are process-local. A process restart cannot resume/replay an old submitted plan. A new explicit plan/run is required. Storage health is checked before a new admission; an existing token can still retrieve its original failed run identity. There are no retries or fallback.

Start uses the launch lifecycle context and returns promptly; completing its HTTP request does not terminate the admitted run. Cancel stops future work and preserves partial evidence through the store. Shutdown cancels admitted work, waits for retention, then closes the private history writer. HTTP header/read/write/idle timeouts bound clients independently of execution caps.

Live event responses identify `live-stage-ring` and an ephemeral public epoch (not a secret). Ring truncation sets gap and earliest/next cursors. A future cursor returns 400; another epoch returns 409 and requires reset. Restarted runs expose `durable-checkpoints` with a distinct `checkpoint-<runId>` epoch. Checkpoints and live stage observations are separate streams and cannot share a cursor. No percentage or overlapping-stage timeline is invented.

## Verification and limits

HTTP tests cover session/Host/Origin/CSRF/no-store, cross-site launch rejection, session rotation, concurrent token/plan admission, busy/conflict responses, altered plans, idempotent cancel with durable partial results, paginated/lazy reads, event gaps/future/stale cursors, import/export, strict comparisons, quarantine, unsupported/live/path/provider/credential inputs, upload/depth bounds and unauthorized requests. API-level failure/restart tests inject disk failure, preserve readable failed/interrupted evidence, reject later admission when storage is unhealthy, keep completed cases and unknown-final dispatches, reject old plans, and require a checkpoint cursor after restart. A paired settings-configuration comparison verifies finite curated deltas with frozen evidence versions. Controlled transports remain the only execution mode. A launch smoke test uses a temporary synthetic store and an actual loopback listener, checks hostile Host/Origin and unauthorized reads, and shuts down cleanly.

This slice supplies the API and minimal launch shell; the usable dashboard is #150. Store platform limits remain those of #148 (Linux support). Live/deployed provider, browser/dashboard, real Profile and universal filesystem power-loss evidence remain unavailable. HTTP status and command completion do not establish semantic correctness.

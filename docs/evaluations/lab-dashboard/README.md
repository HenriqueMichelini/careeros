# Standalone local Evaluation Lab dashboard

Issue #150 connects the secured loopback API to a separate React/TypeScript/Tailwind
entry point. Frozen branch/review base: `84fe8ab37a2e90756d6a4939924036867007959a`.
The dashboard is not imported by CareerOS product navigation or its production build.

## Launch

From the checkout, after existing dependencies are installed:

```sh
GOCACHE=/tmp/careeros-go-cache npm run lab
```

One invocation builds `lab/web/lab.js` and `lab/web/lab.css`, builds the Go launcher
under ignored `lab/.bin`, then owns its loopback process. Open the printed URL.
Optional Go launcher arguments follow `--`, for example `--data-dir /absolute/private/path`.
The default history remains outside the checkout in the #148 private store. Interrupting
the launcher forwards cancellation and waits for terminal retention/shutdown. Initial
build needs installed Node/npm and Go; ordinary Lab operation needs no remote asset,
provider, analytics, font or production storage dependency. No live control is exposed.

## Contract additions

Authenticated `GET /lab/api/status` supplies bounded disk inventory, storage health,
per-suite fixture availability, 29 versioned metric definitions and fixed asset/source
hashes. Missing approved fixture assets are explained before planning. Go execution
is inside the launcher; Node/browser process adapters remain a later slice.

`GET /lab/api/runs/:id/view` returns metadata, metrics and stripped recursive case
summaries. Original source, gold, actual and client-review artifacts are requested only
when a case is opened through the existing case endpoint. The old full-detail route
remains compatible. Server-side reads still validate bounded original snapshots;
this UI laziness is not a claim of zero disk reads or a new storage format.

Optional immutable manifest summary fields index workflow, language, fixture tags,
operational outcomes, configured/returned model identifiers, configuration and exact
version strings. History filters run before bounded pagination. Older manifests are
not rewritten; unavailable metadata does not match a specified filter. Fixture tags
are categories only where actually supplied. They are not certified failure labels.

Confirmed `POST /lab/api/runs/:id/delete` requires the exact matching `confirmId`,
plus the existing session/Origin/CSRF guards. Active/retaining runs cannot be deleted.
A successful deletion also removes the terminal memory record; old admission tokens
return gone instead of redispatching. There is no automatic deletion or retention.

Native results add optional `startedAt` and monotonic `elapsedMs`. Terminal elapsed
covers the sequential runner through final result preparation and excludes final
terminal persistence. Historical absence remains unavailable. Comparison deltas now
carry explicit language/workflow/kind instead of requiring inference from case IDs.
No production handler, inference policy or canonical Profile writer was changed.

## User flow and evidence boundaries

The composer filters workflow/language/fixture-tag category/split/case ID, then freezes
selected case order, configuration, required HTTP steps, conservative simulated call
limits, execution caps and provenance. Only an explicit start admits work. One-use
UI intent plus server idempotency protects admission; failures never trigger retry.
Refresh uses a run ID fragment (no secret), reopens that same run and reads its state.
Session credentials stay in module memory and are never put in browser storage or URLs.

Active runs poll metadata/events about once per second. Event gaps, restart checkpoint
origins, pending/durable state, logical cases, attempted/unreached HTTP legs and actual
simulated dispatches remain separate. Unknown or failed responses are shown as missing,
not a passing empty metric. Engine completion and expected-negative assertions do not
prove semantic quality. Native elapsed, controlled dispatch timing, provider timing,
browser timing and actual-artifact evidence are distinct populations.

Case detail separates original source/context, required/optional gold, actual output
with references, curated missing/unmatched units, captured model outputs/judgments and
explicit client decision. The complete retained artifact remains inspectable. Missing
raw judgments or client review stay unavailable. Regex mismatches are candidates only;
source references and hashes do not prove support. Metric rows show opportunities,
coverage, estimator, missingness and all definitions; there is no overall AI score.

Compatible comparison shows per-case/per-language safety, omission and relationship
changes before any cost/speed interpretation. Incompatible or incomplete contracts are
explained without pooling. JSON/Markdown/original/diagnostic exports use the existing
versioned store contracts. Original export may be unavailable for a native run.
Unknown imports remain quarantined even when their JSON schema is recognizable.

## Verification and retained artifacts

`smoke.manifest.v1.json` records runtime versions, exact export/build hashes, the frozen
base, observed flows and limits. `smoke.native.v1.json` and `.md` are byte-for-byte
actual browser downloads for the one-case EN synthetic semantic smoke. Every source
hash in that native export was checked against the candidate files. The compiler VCS
revision is the starting commit with dirty candidate source; final PR head/review
identity is recorded separately in GitHub. These files are immutable evidence, not a
fixture approval or semantic quality certification.

Passed:

```sh
GOCACHE=/tmp/careeros-go-cache go test ./...
GOCACHE=/tmp/careeros-go-cache go test -race ./internal/evaluationlab
GOCACHE=/tmp/careeros-go-cache npm test
npx tsc --noEmit
npx oxfmt --check lab/ui scripts/lab.mjs tests/lab-browser.check.mjs tests/lab-dashboard.test.mjs vite.lab.config.ts
node tests/lab-browser.check.mjs
```

The JavaScript suite has 143 tests. The actual isolated browser smoke covers all five
registered suites, selective runs, negative/unavailable cases, lazy detail, compatible
and incompatible comparisons, both export formats, terminal refresh, active reconnect
and UI cancel, history filters, malformed/quarantined import and deliberate deletion.
Keyboard-only select/plan/start flows passed at desktop and 390px. Screenshots were
visually inspected; the narrow page has no document overflow. The harness denied
external browser requests and observed zero attempts and zero instrumented browser
storage calls. Synthetic API fault responses separately verify disabled planning and
visible preflight problems; Go public tests cover actual storage health/failure paths.

No providers, real Profiles, credential discovery, deployment verification, user
acceptance, physical mobile or actual-PDF evidence was used. Human adjudication remains
pending. Existing CV integrity work and release gates remain separately owned.

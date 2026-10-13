# Authenticated preview inspection harness — issue #52

Date: 2026-10-13, America/Sao_Paulo. **Release gate: not passed.**
This records P0/P1 of the supplied remaining-work plan. It authorizes no external
configuration, deployed inspection, provider use, merge or production publication.

## Candidate reconciliation (P0)

GitHub and local refs were read before implementation:

- PR #114 was OPEN at `2447dfb9023f92cfbe4da1b99c4c86788524247c`, on
  `codex/evidence-backed-cvs-and-applications`, base `main`.
- Approved review baseline and remote `main` were
  `1a30a445e194d353af116d8a2b5c5194b14ff36a`.
- The delta from implementation checkpoint
  `6f87542c83241351966401a4f77f4f16c174ddf1` to `2447dfb` contained only
  `docs/input-validation.md` and the private-capture evidence document.
- Prerequisites #33, #50, #51, #62 and #65 were CLOSED. #52 was OPEN with its
  original 13 criteria unchecked; neither checklist nor blockers were changed.
- Netlify checks identified final cleaned preview `6acebb527a80ae056877e041`.
  The stopped attempt used separate build `6aceb8b5a4b5981a6f9abcc1`.

These are source/deployment observations at this checkpoint. A later build must
be recorded separately. No previous inspection allowance is carried forward.
The [stopped inspection](2026-10-13-private-synthetic-capture.md#separately-approved-preview-inspection--stopped)
and historical English failure remain unchanged and unexplained.

## Reproduction and correction (P1)

The initial deterministic loop used two loopback origins and a disposable Chrome
profile. It loaded the app fixture, navigated to a simulated Team-authentication
page, then evaluated the previous relative-fetch pattern. The assertion requiring
a navigation stop failed in under one second:

```text
rtk proxy node tests/preview-inspection-browser.check.mjs
AssertionError: navigation after readiness must stop before a workflow request
actual: undefined; expected: navigation_changed
```

This reproduces a missing safeguard. It does not reconstruct the historical
browser state or establish why October 13 returned 404. Ranked mechanisms were
navigation between readiness/send, authentication redirects, and incomplete
network observation. Each now has a controlled regression at the browser boundary.

The new [operator harness](../../tests/preview-inspection-harness.mjs) provides:

- A cloned, single-use approval binding source, deploy, origin, absolute endpoint,
  synthetic body/hash, case, stages, expected response and expiry of at most one hour.
  `dispatch: true`, production origins, relative endpoints and mismatched approval
  metadata are rejected before browser networking is configured.
- A fresh deployment metadata read before readiness and while the actual POST is
  paused. ID, source commit, origin, immutable deploy URL and preview context must
  match. Missing, stale or changed metadata stops release of the request.
- A fresh, same-origin document GET with credentials, no cache and redirect
  rejection, plus a loaded app/page check. The URL/origin/app guard is repeated
  synchronously in the browser at send time and again at request interception.
- An absolute `/api/qualification-gaps` POST with fixed inert keys, no key-file or
  Profile-storage reads, and no clipboard use. Interception checks actual UTF-8
  bytes/hash, Origin, method, case and inert headers before release. At most one
  POST is released; a stopped inspector cannot be retried. The slot is reserved
  synchronously before asynchronous checks, and acceptance requires exactly one
  released request. Divergent body text is withheld; only its size/hash are retained.
- Explicit 3xx rejection without following Location, retained observed response
  URL/status and redirect chain, and separate diagnostics for no endpoint request,
  a missing Network event, unexpected HTTP, and missing/invalid capture.
- Bounded response reading, private request/response evidence, exact provider
  envelope hashes, and checks for `inspection_only`, one `not_dispatched` first
  envelope, whole-original Jev input and null unavailable provider measurements.
  Authentication headers are not retained in evidence.

The harness expects HTTP **502** for the intentional classifier transport stop.
This is verified through the public qualification handler, rather than inferred
from a successful provider response. A 502 alone never qualifies as an inspection
pass: the complete capture contract and network observations must also match.

`readDeployment({signal})` is a mandatory operator adapter. It must make a fresh
authenticated, read-only Netlify lookup and return `{id, commitRef, url, deployURL,
context, observedAt}`. Its deadline is five seconds and observations older than
five seconds are rejected. Cached assertions and GitHub check URLs alone are
insufficient evidence of the currently served deployment. The controlled suite
uses a clearly labelled metadata fixture; the actual authenticated Netlify
adapter/session must be demonstrated in P2 before releasing a workflow request.
This preserves the deployment evidence gap rather than hiding it behind a hash.

Netlify distinguishes an immutable `DEPLOY_URL` from the alias `DEPLOY_PRIME_URL`;
the ID prefixes the immutable URL. See [Netlify build variables](https://docs.netlify.com/configure-builds/environment-variables).
When configuration requires a same-source rebuild, its new ID cannot be assumed
in advance. Freeze the resulting ready build and verify the alias mapping before
the single inspection. Do not silently substitute a newer source or deployment.

## Controlled evidence and validation

Run the browser loop with:

```sh
rtk proxy npm run test:preview-inspection
```

The retained [controlled report](issue-52/controlled-preview-inspection-browser.json)
binds its fixture, harness and browser-check source hashes. It covers navigation
after readiness and while paused, a changed deploy, lost Network observation,
changed base URL, 301/302/303/307/308, an observed 404, a successful-looking fetch
with no network request, changed request bytes, simultaneous duplicate requests,
missing/invalid/forwarded capture,
changed original provider input and session expiry during the document check.
It also rejects an unauthenticated page and seven invalid/missing approval-adapter
cases. Redirect destinations receive zero workflow POSTs.

`TestQualificationInspectionStopsAtFirstEnvelopeWithoutDispatch` runs the public
production handler with a closed provider transport. It verifies HTTP 502,
no-store, exact first-envelope bytes/hash and whole original input, zero transport
calls, and null unavailable provider measurements. This is controlled handler
evidence, separate from browser interception and from actual deployment evidence.

The implementation delta adds verification code, a backend test, an npm check
entry and documentation only. Application/runtime source, routing, providers,
manifest admission policy and product UI are unchanged. Existing controlled
Results/PDF evidence remains tied to its recorded source hashes; new PDFs or
provider quality are not claimed from this harness.

The first review found a reservation race: two paused POSTs could both pass the
counter check before awaiting metadata. The added concurrent browser regression
failed with `released 2 concurrent POSTs`, then passed after synchronous reservation.
The final report preserves this regression and source hashes for the correction.

Full Go tests, all 134 Node tests, TypeScript and the production build passed.
The Node suite required execution outside the sandbox because it blocked two Go
subprocesses (`spawnSync go EPERM`); the earlier sandbox failure is not counted as
a product failure. The browser check passed with a disposable, loopback-only
Chrome and external DNS resolution disabled. Formatting and whitespace checks
passed. The build retains the existing large-chunk advisory. Runtime/UI source
is unchanged, so historical browser/PDF validation was not repeated or relabelled.
The final reviewed commit and checks are recorded in PR #114 and #52. The release
gate remains open regardless of those checks.

## Concrete proposal for P2 — pending fresh authorization

The proposed new allowance is **one** preview-only inspection attempt and **zero**
provider dispatches. It has no retry, fallback, reallocation or paid-provider use.

Use case `ruby-en-inspection-hardened` at
`https://deploy-preview-114--beamish-bavarois-333d03.netlify.app/api/qualification-gaps`.
The synthetic body is the first English qualification request in the retained
[controlled Results capture](issue-52/controlled-results-browser.json): **1,273
UTF-8 bytes**, SHA-256
`5a67636650ace60f6ce9b0f5a9c5de2d08474770886443abac55cf541387a7c3`.
Its anonymous source says only that the person uses Ruby to maintain internal CLI
tools, with a Ruby role asking for an explanation in English. All complete bytes
are available in that artifact's first `captures[].requestBody`; the new approval
must explicitly cover those bytes rather than reuse the previous allowance.

Bind the final reviewed PR head, exact ready Netlify build and alias mapping in
a new private approval. Activate only the Deploy Previews manifest for that
request, with `dispatch: false`, the three qualification stages and an absolute
expiry no more than one hour after activation. If needed, allow one same-source
preview rebuild, then bind and verify the resulting build ID before sending.
No source substitution, merge or production publication is included.

First demonstrate the authenticated metadata adapter and document readiness using
read-only checks. Missing access, unresolved deploy mapping, redirects or any
mismatch stop before the API attempt. After an attempted inspection, stop on any
unexpected response or missing capture; do not retry. Retain the actual browser
request bytes, response URL, redirect chain and first serialized provider envelope
privately. Review bounded, content-free operational telemetry separately. An empty
log query never establishes zero provider dispatches.

The requested authorization also includes removing the temporary manifest,
reloading Netlify settings to verify removal, and deleting the disposable browser
profile. Keys are inert and never persisted; clipboard is unused. Removal does not
erase an older immutable build, whose inspection-only manifest remains bounded by
its expiry. If the platform or session cannot satisfy these constraints, record
the exact failure and clean up without spending another attempt.

P3 requires separate paid-provider and financial authorization after P2 succeeds.
P4/P5 must still reconcile all 13 original criteria. Broader bilingual quality,
complete accepted deployed artifacts/export parity, actual charges and cost per
accepted complete workflow remain unavailable. #52 must stay open until each
original requirement has a defensible disposition.

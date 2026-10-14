# #52 P2 — deployed inspection verified

On October 13 in America/Sao_Paulo, the separately authorized P2 inspection
verified actual preview-handler execution and the first serialized provider
envelope. One POST was released, with **zero provider dispatches**, inert keys,
`dispatch: false`, no retry and no fallback. This completes P2's bounded inspection
exit condition. It does not complete the release gate or authorize P3.

The [content-free deployed summary](issue-52/p2-preview-inspection.deployed.json)
records the observations. Complete synthetic browser request/response bytes and
the serialized envelope remain private outside the repository, alongside the
approval, metadata records, screenshots and cleanup records. The raw-capture hash
is traceability, not an independent proof of the capture's meaning.

## Authorization and frozen deployment

The user explicitly approved the updated P2 proposal for `c154f40`, conditioned on
that commit's SonarCloud gate passing. The gate passed, and the exact current PR
head and CI were checked again immediately before the configuration rebuild.
The earlier failed inspection allowance was not reused.

| Binding | Verified value |
| --- | --- |
| Candidate | `c154f40ca04e8ef88bb0b917b89603b586797d0f`, PR #114 |
| Runtime checkpoint | `6f87542c83241351966401a4f77f4f16c174ddf1`; runtime unchanged |
| Original ordinary preview | `6acec710103dcd3ecab6ca99` |
| Inspection build | `6acecf58423a9ea41a9137f6`, one same-source configuration rebuild |
| Origin | `https://deploy-preview-114--beamish-bavarois-333d03.netlify.app` |
| Endpoint | `/api/qualification-gaps`, absolute origin-bound URL |
| Case | `ruby-en-inspection-hardened` |
| Actual browser body | 1,273 UTF-8 bytes |
| Browser body SHA-256 | `5a67636650ace60f6ce9b0f5a9c5de2d08474770886443abac55cf541387a7c3` |
| Manifest expiry | `2026-10-14T01:34:15.126Z` (October 13, 22:34:15 GMT-3) |

The authenticated Netlify dashboard showed no project variables before activation.
Only the Deploy Previews value of `CAREEROS_RELEASE_CAPTURE` was added. Production,
branch deploys, preview servers/Agent Runners and local development stayed empty.
The resulting build completed at 21:40 GMT-3 with the exact approved commit, five
functions and six redirect rules. No production publish, merge, billing change,
protection change or second rebuild was performed.

## Authenticated adapter and actual browser observation

The disposable normal-sandbox Chrome required a separate human sign-in; the
in-app browser session was not copied or exported. No provider credential file
was read. The reviewed `tests/preview-inspection-harness.mjs` ran unchanged.

Initial read-only DOM lookups and a metadata-channel probe could not meet the
deadline. They occurred before activation and sent no workflow POST. The final
operator adapter reloaded the authenticated Netlify deploy list and detail page
in parallel, read their visible current-preview ID/status, full source link,
permalink and alias, and delivered the record through private local files. A
fresh nonce matched each callback. It did not reuse an earlier observation or
relax the harness's five-second deadline.

The final read-only probe passed in about 2.52 seconds. Subsequent actual metadata
lookups took 2,484 ms during readiness and 2,497 ms while the POST was paused. Both
matched the inspection build and exact source. The app-document fetch returned
HTTP 200 at the approved URL; the page was complete with a mounted app.

The sole API attempt occurred around `2026-10-14T00:41:00Z`. CDP request interception
retained the actual 1,273 bytes, matching the approved hash and origin. The
independent network request event was observed. The endpoint response URL was
the exact approved URL; no redirect, other workflow request or blocked request
was recorded. HTTP **502** was the expected inspection transport stop, and the
response was JSON with `Cache-Control: no-store`.

The harness returned `inspection_verified` with no stop reason. It validated:

- `releaseCapture.outcome: inspection_only` and exactly one envelope;
- `stage: field_validation`, `outcome: not_dispatched`, admission `release-egress-v1`;
- the actual serialized Jev request to `https://api.typesafe.ai/v1/systemone`,
  model `jev-1.13.0`, containing the complete original approved job posting;
- 3,414 envelope bytes, SHA-256
  `befd6e2a35abd41748da3bad3102235691b9737a81064db870213b93f10fb550`;
- null provider HTTP status, response bytes and duration, with no provider body.

The browser's approximately 3.09 seconds includes deliberate interception and
metadata lookup. It is not ordinary provider latency or an end-to-end acceptance
measurement. The 502 alone would not have established success.

## Operational sample and cleanup

The authenticated dashboard's `qualification-gaps` log was inspected only for
Deploy Preview #114, October 13 20:42–21:42 GMT-3. It showed the matching 502 handler
summary, `ai-diagnostics-v1` record and platform invocation summary around 21:41.
The diagnostic attempt ID was `13f799ac69f1b8a372b8144627a5890e`; the platform request
prefix was `c93fe882`. Handler diagnostic duration was 1.071932 ms, platform duration
4.13 ms and initialization 151.64 ms.

The diagnostic labels `service_failure`/`transport_failure` describe this deliberate
inspection stop. Its provider wrapper's timing and HTTP sentinel `0` do not
represent a dispatched provider response. Token/model values were unavailable.
The sampled records contained no request/prompt/Profile content, provider response
body or key. The UI reported 24-hour log retention. This is bounded sampled
privacy evidence, not a statement about all platform retention or all routes.
Zero dispatch is supported by the validated inspection envelope and transport
contract, not by an empty log query.

Interception was closed, the disposable Chrome exited and its temporary profile
was verified absent. The manifest variable was removed, and the dashboard was
reloaded: at `2026-10-14T00:42:06.351Z` it again showed zero project variables.
Clipboard was unused and inert keys were not persisted. The browser owner and
the inspection relay were separate local processes; only the relay executed the
single inspection. Its cleanup records `attempted: true`; the browser owner's
`attempted: false` refers only to that owner's unused inspection command.

Settings removal does not erase the older immutable build's manifest. That
inspection-only manifest expires at the timestamp above; no cleanup rebuild or
additional inspection is implied or authorized.

## Remaining gate

P0/P1 controlled reliability evidence and this P2 deployment observation remain
distinct. The earlier unexplained 404 and unavailable historical English response
remain in the record; this successful request does not reconstruct their causes.

P3 needs a disjoint frozen EN/PT set, current candidate/deploy binding, exact
request preflight, provider/credential/recipient authorization, dispatch limits
and a financial ceiling. Response-dependent browser requests must be reviewed
from their actual bytes before dependent workflows; offline fixtures cannot
approve unknown `jobContext` or generated output. The inspection harness rejects
paid dispatch and cannot simply be reused with real keys.

Provider-backed bilingual quality, accepted complete artifacts, actual deployed
copy/PDF parity, attributable charges and production route acceptance remain
unverified. P4/P5 and all 13 original #52 criteria remain open. No runtime or
workflow-setting changes follow from this observation.

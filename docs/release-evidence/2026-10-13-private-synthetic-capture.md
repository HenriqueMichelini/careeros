# Private synthetic response capture — issue #52

Later evidence: [P2 deployed inspection verified](2026-10-13-p2-deployed-inspection.md)
records the separately approved `c154f40` inspection, one actual endpoint POST,
first envelope and cleanup. The failed earlier attempt below remains historical;
its 404 cause is still unknown. The release gate remains open.

Date: 2026-10-13, America/Sao_Paulo. This extends the
[controlled preparation](2026-10-12-controlled-preparation.md) after Henrique
approved private, synthetic-only response capture. **Release gate: not passed.**
At the implementation checkpoint, no live provider call, credential-file read,
production publication, capture activation on Netlify or billing change was
authorized or performed. The separately approved inspection below supersedes
only that checkpoint's capture-activation status.

## Implemented boundary

The existing qualification and Application Draft HTTP handlers support an opt-in
`X-CareerOS-Release-Capture` header. Ordinary requests retain their existing
response and networking behavior. Capture requests require a valid, unexpired
`CAREEROS_RELEASE_CAPTURE` approval manifest. An unconfigured or invalid capture
request receives a content-free 403 before provider work; it does not fall back
to ordinary processing.

Approved capture defaults to inspection only. The first actual serialized provider
envelope is inspected and returned with `not_dispatched`; the workflow stops with
`inspection_only`, without invoking the provider transport. Provider status,
response and duration remain unavailable. This supports a zero-provider deployed
mechanism check using inert, syntactically valid placeholder keys. Forwarding
requires an explicit `dispatch: true` in the approved manifest case and separate
authorization for provider calls. The flag itself is not billing authorization.

Each manifest case fixes the case ID, route, exact browser origin, SHA-256 of the
complete browser request bytes and provider-stage sequence. The manifest contains
only approval metadata. Payloads and credentials are absent from configuration.
Case IDs must be unique. Opaque origins, duplicate opt-in/origin headers, changed
bytes and expired approvals are rejected. HTTPS origins and loopback HTTP origins
are supported. Approvals expire within 24 hours; the expiry also bounds the
request context. Hashes establish byte identity; the operator must review and
approve the minimized synthetic source beforehand.

The provider transport inspects each actual serialized envelope immediately
before dispatch. `release-egress-v1` checks the fixed provider URL, POST method,
model/reasoning/output ceiling, message roles, schema name and outer payload-field
allowlists. Jev receives the complete original approved posting. Existing
production projections, source validation and review/acceptance rules remain in
force. These checks do not establish semantic support or independent human
approval of unknown generated text.

Capture is bounded to 256 KiB per outbound envelope and 1 MiB of aggregate
request/response bodies. The normal handler result is bounded to 512 KiB while
capture is active; the final response is bounded to 3 MiB. Capacity and read
failures stop further dispatch through that request's transport. No truncated
payload is returned as complete evidence. Cancellation stops admission. No retry,
fallback, provider switch or additional workflow stage is introduced.

Each retained exchange reports stage, admitted endpoint, admission version,
request/response bodies, encoding, byte lengths, hashes, observed HTTP status and
transport duration including response-body reading. Non-UTF8 bodies use base64
so their exact bytes remain recoverable. Unavailable byte counts, HTTP status and
duration remain null; unavailable bodies and hashes are omitted. A complete
capture means that the approved exchanges were retained, without asserting a
successful workflow, supported artifact or release result.

The `releaseCapture` field is appended only to the requesting HTTP response,
including partial failure results. Responses use `Cache-Control: no-store` and
provide no cross-origin access grant. Authentication headers are never collected.
Both supplied credentials are checked against request and response contents,
including JSON escapes in truncated responses and embedded malformed JSON;
credential-bearing content is withheld. Escape scanning is bounded and does not
depend on a complete JSON document.
Authentication-failure bodies are withheld entirely, including masked-key errors,
while the existing key-failure status is preserved. Transport error strings are
excluded. Payloads live only in that request's memory;
there is no payload cache, server persistence or content logging. Existing
operational diagnostics remain content-free. Application Draft's client removes
the field before returning product state; the requesting test browser can retain
the raw response privately using its network capture tools.

## Activation protocol for a separately authorized check

1. Select the reviewed source/build and exact browser origin. Review the complete
   minimized synthetic Profile/posting/request and the existing source allowlists.
2. Capture the browser's exact serialized request bytes without dispatching a
   provider workflow. Freeze their hash and the expected stage sequence. Do not
   substitute a hand-built request that merely looks equivalent.
3. Configure only that deployment with the content-free manifest, `dispatch: false`, an expiry no
   later than 24 hours away and the exact approved cases. No provider credentials
   belong in that manifest or server configuration.
4. For the inspection request, use inert placeholder keys, add the case ID header in the requesting test
   browser and privately retain its actual request and raw response. Preserve
   the `inspection_only` result and verify zero provider dispatch. A 403 is a stop
   condition. After separate provider authorization, an explicitly approved
   `dispatch: true` configuration may retain actual provider exchanges; preserve
   failed/unavailable results and skipped dependents.
5. Remove the capture configuration and temporary browser keys/clipboard data
   after the bounded check. Verify missing-key readiness after a fresh load.

The following is an inert schema example: its expiry is past, its hash is a
placeholder and its origin is an invalid example domain. It authorizes no run.

```json
{
  "version": "release-capture-manifest-v1",
  "expiresAt": "2000-01-01T00:00:00Z",
  "cases": [
    {
      "id": "approved-draft-case",
      "path": "/api/application-draft",
      "origin": "https://preview.example.invalid",
      "requestSHA256": "0000000000000000000000000000000000000000000000000000000000000000",
      "dispatch": false,
      "stages": ["field_validation", "generation", "resume_support", "artifact_support"]
    }
  ]
}
```

Qualification cases may expect `field_validation`, `qualification_matching`,
`requirement_evidence`; legacy analysis omits the last stage, and valid context
reuse omits the middle stage. The production handler can legitimately skip a
stage when there are no requirements or an earlier stage fails; its capture then
reports incomplete relative to the frozen sequence.

An actual qualification response can change a later browser request's
`jobContext`; generated text also changes dependent provider envelopes. An old
request hash or static response fixture cannot approve those unknown bytes.
Inspect and explicitly approve any new browser request and its configuration
before it is admitted. Do not rerun earlier workflows or reallocate unused calls
to obtain a matching hash. Any deployment/configuration transition must be part
of the separately authorized candidate/origin and dispatch plan.

## Evidence and remaining gates

Controlled tests run the public production HTTP handlers with closed provider
transports. They retain all four draft stages, both legacy qualification stages
and all three explained-qualification stages. They check exact body delivery,
source projection, authentication exclusion, escaped credential echoes, approval
rejections before dispatch, cancellation, malformed/binary/oversized responses,
transport failure, null measurements and isolation from a later ordinary request.
An inspection-only regression observes the actual first envelope and zero
controlled provider-transport calls when the dispatch flag is omitted.
The public `generateMaterials` regression verifies that capture evidence stays
outside application state while resume/artifact acceptance continues to work.

Initial red cases exposed the missing capture response, ambiguous origin/case
admission, an escaped cross-provider credential echo, dispatch after cancellation,
zero values for unavailable measurements and client-state propagation. The Spec
review also exposed escaped credentials in truncated JSON; lexical escape scanning
now withholds those responses, with public-handler regressions for malformed root
and embedded strings at HTTP 200/502. The Standards review's repeated-switch
suggestion was resolved with one private stage-contract descriptor. Each change
was rerun through the approved seams. Full validation and the
independent Standards/Spec review are recorded on PR #114 and canonical issue #52.

**Criterion 7 still lacks actual deployed capture evidence.** The existing
Netlify entry points now construct capture-capable handlers, disabled unless the
manifest and opt-in request are present. Local controlled transport tests do not
prove deployed egress, platform retention, provider quality or billing. No live
candidate/batch is activated by this document, and the frozen batch's allowance
remains zero. Separate provider/publication authorization, a frozen measurement
plan, broader disjoint bilingual quality comparison and accepted-application cost
evidence remain outstanding. Historical English delivery, PT language/support
observations and all original release criteria remain open.

## Separately approved preview inspection — stopped

Henrique subsequently approved one synthetic inspection request, temporary
Deploy Previews configuration, a same-source preview rebuild if needed and
configuration removal afterward. The allowance was one attempt, zero provider
dispatches, inert placeholder keys, `dispatch: false`, at most one hour of
activation and no retry. It did not authorize provider calls or publication.

Netlify showed successful preview deployment `6aceb8b5a4b5981a6f9abcc1` at source
`6f87542c83241351966401a4f77f4f16c174ddf1`, with five deployed functions. The
manifest had one value in the Deploy Previews context; Production and the other
contexts were empty. Its approved origin was
`https://deploy-preview-114--beamish-bavarois-333d03.netlify.app`, case
`ruby-en-inspection`, route `/api/qualification-gaps`, and expiry
`2026-10-14T00:02:02Z`.

The approved 1,273-byte English request was copied exactly from the retained
controlled browser request, SHA-256
`5a67636650ace60f6ce9b0f5a9c5de2d08474770886443abac55cf541387a7c3`.
It contains the anonymous Ruby regression only. The disposable browser used
inert key strings, without reading real provider credentials.

The single attempt returned HTTP 404 with Netlify's “Page not found” HTML in
392.8 ms of browser fetch time. Its retained UTF-8 response is 3,449 bytes,
SHA-256 `8514f0009a58c6e0acb5468f88037732b59b70af5e524f452e3bef8fb33effc5`.
The network recorder retained **no request matching the approved endpoint**.
The failed attempt did not retain its final response URL. Therefore actual
browser request bytes, final request origin, handler execution, provider
dispatch count and provider envelopes remain unavailable; this is not proof
that the approved qualification route itself returned 404.

A subsequent read-only visit in the authenticated browser passed through
Netlify's Team protection/edge-access page and loaded CareerOS at the approved
preview origin and deployment. Access redirection is a possible explanation,
not an established cause of the failed attempt. The test harness's separate
readiness check and relative request URL were insufficient safeguards against
navigation between those operations. A future attempt must verify a stable,
authenticated test browser, use the absolute approved endpoint, check origin
at send time and retain the final response URL. It needs fresh authorization;
the stopped attempt is not reallocated.
A read-only last-hour qualification-log query scoped to Deploy Preview #114
returned no results. That sampled absence does not establish zero dispatches
or platform-wide retention behavior.

No second workflow request was sent. The temporary variable was removed and
Netlify's “No environment variables set for this project” state was verified
after reload. The disposable test browser and its temporary profile were
removed; placeholder keys were never stored and the clipboard was unused.
Removing project configuration does not prove removal from an already-built
immutable deployment; any retained manifest remains inspection-only and expires
at the timestamp above. Raw evidence stays in the private local inspection
folder, outside the repository and server content logs.

**Deployed capture remains unverified and criterion 7 remains open.** No real
provider credentials, paid-provider run, production publication, merge or
billing change was part of this inspection. All broader release gates and
historical failures remain in force.

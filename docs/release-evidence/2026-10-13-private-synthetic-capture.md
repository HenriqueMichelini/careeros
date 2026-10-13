# Private synthetic response capture — issue #52

Date: 2026-10-13, America/Sao_Paulo. This extends the
[controlled preparation](2026-10-12-controlled-preparation.md) after Henrique
approved private, synthetic-only response capture. **Release gate: not passed.**
No provider call, credential-file read, production publication, capture activation
on Netlify or billing change is authorized or performed by this implementation.

## Implemented boundary

The existing qualification and Application Draft HTTP handlers support an opt-in
`X-CareerOS-Release-Capture` header. Ordinary requests retain their existing
response and networking behavior. Capture requests require a valid, unexpired
`CAREEROS_RELEASE_CAPTURE` approval manifest. An unconfigured or invalid capture
request receives a content-free 403 before provider work; it does not fall back
to ordinary processing.

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
including JSON escapes and embedded JSON; credential-bearing content is withheld.
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
3. Configure only that deployment with the content-free manifest, an expiry no
   later than 24 hours away and the exact approved cases. No provider credentials
   belong in that manifest or server configuration.
4. For an authorized test request, add the case ID header in the requesting test
   browser and privately retain its actual request and raw response. Preserve
   failed/unavailable results and skipped dependents. A 403 is a stop condition.
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
The public `generateMaterials` regression verifies that capture evidence stays
outside application state while resume/artifact acceptance continues to work.

Initial red cases exposed the missing capture response, ambiguous origin/case
admission, an escaped cross-provider credential echo, dispatch after cancellation,
zero values for unavailable measurements and client-state propagation. Each was
corrected and rerun through those approved seams. Full validation and the
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

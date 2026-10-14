# #52 P3 operator preparation — no provider execution

P2 passed at `c154f40`, as recorded in the [deployed inspection](2026-10-13-p2-deployed-inspection.md).
Its documentation commit `948ac95` passed SonarCloud, GitGuardian and Netlify
preview/redirect checks; header/pages checks were neutral. The P3 preparation
below is a subsequent verification-only change. It does not relabel the P2
observation as a test of this newer harness.

No provider credential file was read, paid call made, capture variable activated
or manual rebuild triggered for P3. The user permits the needed budget within the
limits they report having configured at the providers; their numeric values have
not been observed, and this work does not change those limits. The proposed cohort
remains bounded by four workflow attempts and 14 provider dispatches, including
failures. Final execution bindings and response-dependent requests are separate
from this local preparation.

## Operator policy

`tests/preview-inspection-harness.mjs` shares its browser recorder between two
explicit entry points. `createPreviewInspector` still requires inspection-only
approval, fixed inert keys, expected HTTP 502 and the first `not_dispatched`
envelope. It cannot be upgraded by setting `dispatch: true`.

`createPreviewProviderRunner` requires a distinct `dispatch: true` approval,
process-local key arguments, batch/case binding and a matching provider-slot
reservation. It accepts only the existing three-stage qualification and four-stage
Application Draft routes, with their exact origin, browser bytes/hash, deployment
identity and one-hour maximum expiry. It uses the same synchronous request
reservation, fresh authenticated metadata checks before readiness/while paused,
post-await expiry guard, same-origin page checks and redirect rejection as P2.
Browser qualification timeout is 55 seconds, matching canonical Profile requests
in the product; Application Draft remains 30 seconds. The deployment lookup
deadline remains five seconds. Process-local keys are excluded from approval
files; credential-bearing body and metadata values are withheld from evidence.

The first two deterministic provider-envelope hashes/lengths must be supplied in
the approval. The actual returned serialized envelopes are checked against them.
Later response-dependent envelopes are retained as actually received, with stage,
recipient, model, bytes/hash, response bytes/hash and nullable failure measurements.
The deployed server's `release-egress-v1` contract still performs pre-dispatch
structural admission; the browser validates the returned capture afterward. A
post-response hash comparison is not a new server-side hash admission mechanism.

`provider_capture_verified` means the capture is coherent; it does **not** mean
provider output or workflow acceptance passed. Partial captures, transport and
authentication failures retain their status. Missing/invalid evidence stops the
run. Literal, recursively JSON-escaped and percent-escaped key echoes are checked
throughout returned evidence, including redirect locations and blocked URLs.
Credential-bearing browser bodies are removed; other affected strings are
replaced with a withholding marker. No retry, fallback model or output
modification is introduced.

`createProviderWorkflowBudget` admits at most four distinct cases totaling at most
14 slots. A case reserves its complete worst-case stage allowance synchronously
before attempting the request, including cases that subsequently stop. Slots are
never refunded or reassigned. Snapshots retain reserved IDs for restoration across
operator phases; the real-preview runner requires a persistence callback configured
to finish synchronously. The operator must persist that snapshot privately before
dispatch and restore the same batch on continuation. A write failure stops before
any POST. This bounds operator attempts; it is not a provider billing meter or a
global server replay protection mechanism.

## Controlled evidence

The [new browser report](issue-52/controlled-provider-operator-browser.json) records
18 loopback scenarios with fabricated captures: three/four-stage capture, partial
transport/auth failures, missing evidence, changed response hash, changed original
Jev text, wrong model, changed deterministic payload, redirect rejection, escaped
key withholding, caller mutation isolation, duplicate budget reservation and
budget write failure, metadata credential echoes and a scaled-clock qualification
deadline regression. It also records ten budget/approval rejection checks,
four-case/14-slot reservation and restoration across phases. No external provider
traffic or real credential reads occurred. Fabricated bodies do not measure
provider semantics, usage, charges or acceptance.

Spec review at `9f4f4f1` found two actionable defects before any real keys or paid
calls: redirect metadata could retain a supplied key, and qualification's operator
deadline was shorter than the product's deadline. Controlled reproduction retained
a placeholder key in a 302 Location. The corrections scrub all evidence strings
and restore the 55-second qualification deadline. New browser regressions cover
literal/percent-escaped redirect echoes, blocked URLs and a response between the
old/new scaled deadlines. The original review findings remain part of the record.

The [inspection regression report](issue-52/controlled-inspection-regression-p3.json)
repeats all 21 existing browser scenarios, seven approval/adapter rejection checks
and unauthenticated-page rejection against the shared recorder. Both reports bind
their exact source hashes. Loopback fixture metadata is not a live Netlify lookup.

## Frozen qualification preflight

Two new private synthetic requests were prepared before any provider observations:
PostgreSQL reporting in English and TypeScript internal dashboards in Portuguese.
Each has a supported skill and work statement, no identifying/contact/employer/
salary/date data, an employer-requested explanation and a question about additional
experience the source does not establish. The intended check preserves those gaps
instead of accepting invented database-administration or team-leadership history.
Neither case reuses the previous Ruby request.

The first draft incorrectly placed a work fact in `additionalInfo`, which the
qualification projection deliberately excludes. The real public handler rejected
it before any transport call. The draft was corrected locally to the allowed
`competencies` field, preserving the source statement. The superseded bytes remain
private and are not a paid observation or an authorized execution request. No
projection policy or product code was weakened to admit that draft.

`scripts/preview-preflight/main.go` runs the actual public qualification handler
with a closed custom transport. It fabricates only a Jev acceptance, captures the
first two real serialized envelopes and deliberately stops at matching without
ever dialing a provider. Complete raw synthetic envelopes remain private. The
[content-free preflight summary](issue-52/p3-qualification-preflight.controlled.json)
records their bytes, hashes and recipient URLs.

| Case | Browser bytes | Browser SHA-256 |
| --- | --- | --- |
| `p3-postgresql-en` | 2,351 | `d709313763c49f92dd7bb514d59052fcb1f1586288a39faf429cf54b518f1667` |
| `p3-typescript-pt` | 2,431 | `af0f3d49dad6faba67a8c711e58e7e98cf4b8d5d66b2fdb36b35835d68469aa6` |

The later `requirement_evidence` envelope depends on the actual qualification
response and cannot be approved by substituting this controlled fixture. Actual
Jev decisions and provider outputs remain unknown. The later Application Draft
browser bytes, `jobContext` and IDs must be prepared from actual successful
qualification results and explicitly reviewed before those dependent requests.

The application/runtime, routing, models and acceptance policies are unchanged.
The previous 404 and missing historical English response remain unresolved.
#52 and its 13 original release criteria remain open. P3 execution, accepted
artifacts, copy/PDF parity, quality comparison, attributable charges and the final
P4/P5 release decision are not established by this preparation.

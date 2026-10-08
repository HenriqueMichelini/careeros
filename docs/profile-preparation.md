# Profile ingestion preparation

Issue #60 integrates the Deterministic Preprocessing milestone into the existing
Profile workflow. Onboarding and Quick Add share `RepositoryPage` and the same
`POST /api/profile/ingest` client/handler.

Strict transport/schema/Profile validation and the existing 30,000-byte original
and 160 KiB body limits run first. `PrepareBounded` creates a transient complete
prepared Source. Jev receives the **complete original decoded input**, once, with
the existing model/rubric, decision contract and deadlines. Rejection, rephrasing
and classifier failure stop before extraction/comparison. Only acceptance allows
extraction to receive the complete prepared text and ordered syntactic segments.
No automatic portions, additional request, retry, fallback or Profile write is
introduced.

Extraction retains its 30-claim limit and existing semantic prompt. It requests
a `segmentId` identifying the starting segment of each excerpt. Segment metadata
includes prepared text, UTF-8 offsets and parent identity so the model need not
calculate byte offsets to select a repeated occurrence. Exact or existing
whitespace-tolerant matching is limited to starts within that segment. Excerpts
may span wrapped lines. Unknown IDs, multiple matches, invented excerpts, split
normalization groups and oversized original excerpts are unverified and omitted
with the existing count/remediation. An absent segment ID is accepted only for a
globally unique excerpt; the server never chooses the first repeated occurrence.

Before comparison, the shared Source map replaces the prepared excerpt with the
exact original substring and adds this transient reference to every verified
claim:

```json
{
  "version": 1,
  "sourceId": "64 lowercase hex characters",
  "preparationVersion": "structure-v1",
  "segmentId": "64 lowercase hex characters",
  "occurrenceId": "64 lowercase hex characters",
  "originalStart": 0,
  "originalEnd": 10
}
```

Ranges are half-open UTF-8 bytes. Source identity includes the complete original
and the pinned normalization/structure/field namespace, so different originals
remain different even when prepared text agrees. The client validates reference
shape/version, exact byte slicing without split UTF-8, and complete raw-source
identity before rendering proposals. The original excerpt renders with preserved
whitespace. Legacy claim responses without this optional addition remain readable
for compatibility; the integrated handler always emits references for verified
claims. This is source correspondence, not semantic truth or durable provenance.

Working text is capped at 60,000 UTF-8 bytes to accommodate normalization
expansion. Extraction and comparison each separately cap the **actual serialized
provider request**, including prompt, schema, segments and minimized Profile
projection, at 256 KiB. These are byte budgets, not token guarantees. A complete
view that cannot fit returns `capacity`, with actionable EN/PT feedback and no
partial request or implicit continuation. Preparation failure similarly preserves
the input. The current operation, grounding, destination, omission, stale-Profile
and atomic explicit-apply checks remain in force. Maps and pastes are not saved.

## Verification

Controlled public-handler tests cover exact original-versus-prepared payloads,
CRLF, NFC, prose whitespace, protected tabs, repeated snippets under separate
headings, invalid/missing/ambiguous identities, split normalization groups,
wrapped excerpts, raw-identity separation and final envelope capacity. Existing
handler tests cover single Java facts, ordinary noise, classifier failure/reject/
rephrasing, 30,000/30,001-byte bounds, schemas, omissions and minimal projections.
The two current messy-input semantic-baseline fixtures now quote the unique full
repeated span rather than an ambiguous individual mention; their inputs, gold
facts and historical result files are unchanged.
Client tests cover UTF-8 byte references, malformed versions/ranges/IDs, Go-compatible
identity escaping, changed raw input and exact raw outbound submission.

`tests/ingestion-browser.check.mjs` uses synthetic responses without live provider
keys. EN/PT at 1440px and 390px checks original excerpts, same-preparation input
changes during loading, preparation/capacity feedback, rejection/rephrasing,
keyboard retries, raw input preservation and empty/populated Profile review,
edit/reject/cancel/explicit apply/reload. Screenshot inspection confirms the
390px Portuguese proposal layout; automated overflow checks pass throughout.

Full Go suite, all 55 JavaScript tests, TypeScript checking and production Vite
build pass. Evidence is local and controlled; it does not establish live provider
quality, deployment status or completion of milestone integration tickets #61/#62.

# Reviewed Apply-to-Profile enrichment

Implementation of discovery #77, milestone #6, issues #116–#120. The user
approved implementation before #52 closes on 2026-10-14. This changes sequencing
only: #52 and its release acceptance requirements remain open and unchanged.
Review baseline: `584d5dc968ff8423ff40f46916a8c34dc9676372`.

## Approved product decisions

Application-only answers and Draft generation remain independent of enrichment.
After generation settles (including failure), each nonempty user answer can be
reviewed separately for the Profile. No checkbox label, generated question or
employer requirement becomes an assertion. The review action discloses TypeSafe
and OpenAI recipients, user keys and possible credits. Nothing is saved by that
action. No operation is selected by default; Save approves only visible selected
operations. Contradictions, ambiguous ownership and corrections require focused
clarification and renewed review. Substantive inline proposal editing is excluded;
users correct the answer and review again, or use canonical Profile editing.

The transient handoff retains original answer bytes, language, unique request
identity, exact Profile reference and optional existing role/project reference.
Question labels are transient UI context only; the adapter does not send them to
professional ingestion. Approved evidence retains answer excerpts with
`apply_answer` provenance; separately elicited clarification retains
`clarification_answer`. No full posting, dialogue, raw answer archive or handoff
is persisted. Request metadata is not retained in evidence.

The strict `applyAnswer` request/response extension on `/api/profile/ingest`
validates version, language, request ID, exact Profile revision and optional
existing project/role revision. It rejects arbitrary origins, unknown fields,
invalid targets, continuation combinations and answers over 8,000 UTF-8 bytes.
Whole-answer professional classification precedes extraction and comparison.
Selected ownership is context, not capability evidence. Operations outside an
explicit owner are rejected. Source mapping and conservative dispositions remain
owned by ingestion. No additional endpoint, provider, ontology or writer.

Only `applyIngestionProposal` → `applyIngestionDocument` → `editProfileBatch`
→ `saveProfile` writes. Save uses shared locking and exact expected snapshots;
success follows persistence. No cancellation control claims to cancel an atomic
save after dispatch. Generation and enrichment work are serialized. Pending
answers survive matching reset; existing artifacts retain their original
snapshots. Fresh matching is a deliberate new attempt, not silent regeneration.

Stale reviews preserve recovery text and require deliberate rebuild against the
current Profile and renewed operation selection. Cross-tab changes follow the
existing reload/recovery path. There are no automatic provider retries. Retry
after an uncertain save first inspects durable state. Inspection and reversal
use existing fact details and canonical editing; no transaction rollback or
one-click undo can overwrite later unrelated edits.

## Validation boundary

Approved seams: production ingestion HTTP handler with controlled provider
transports, canonical proposal/apply/save interfaces, EN/PT Apply browser flows
at desktop and 390px. Fixtures establish contract and persistence invariants;
they do not establish live semantic quality, actual cost, latency, deployment
availability or reduced questioning. Durable enrichment, later candidate
inclusion, matching decisions and downstream Draft support are separate claims.
Live provider/deployment work needs separate authorization. No such calls are
part of this implementation.

## Recipients and retained data

| Boundary | Data and purpose | Retention in this app |
| --- | --- | --- |
| Browser → CareerOS ingestion | Original answer, current canonical Profile and matching compatibility view, strict transient metadata; comparison and snapshot validation | Stateless request only; no server Profile write |
| CareerOS → TypeSafe | Whole answer for professional classification; original plus bounded clarification context on a clarification attempt | Existing provider disclosure/terms apply; this feature makes no new provider retention claim |
| CareerOS → OpenAI | Prepared answer excerpts; claim destinations, relevant existing facts/identity and accepted evidence; validated target identity when selected | Existing provider disclosure/terms apply; no complete posting/dialogue archive |
| Browser canonical save | Selected canonical operations and accepted answer/clarification excerpts with truthful origins | Browser-local Profile; inspect/correct/remove through existing fact editing |

A review normally adds one TypeSafe and two OpenAI requests; a focused
clarification normally adds two of each. Early failures can dispatch fewer.
Saving itself makes no provider request. These are code-path counts, not measured
latency, tokens, charges, or a bound on deliberately repeated journeys.

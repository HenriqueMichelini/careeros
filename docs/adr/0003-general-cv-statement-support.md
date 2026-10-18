# General CV statement support contract — issue #123

Status: APPROVED by Henrique in the implementation chat on 2026-10-15: “I approve the proposed support-call budget, user-provenance policy, and review baseline,” referring to the [full proposal](https://github.com/HenriqueMichelini/careeros/issues/123#issuecomment-6198695849). This decision specifies future behavior; issues #124–#128 implement and validate it. No runtime behavior changes in #123, and no live inference is authorized.

## Decision

Use one independent support assessment after structurally valid general-CV generation, using the existing gpt-6-luna model with reasoning_effort none. Independence means a separate request with a support-only policy, not a separate provider or a guarantee of independent errors. The assessor classifies each generated summary sentence and optional supporting rewrite against exactly its cited original facts and their owner/context/qualifier metadata. It cannot rewrite text, add citations, select new facts or change Profile. Both facts and generated text are untrusted data, separated from trusted system policy.

Reject generation self-certification and keyword expansion as semantic proof. Reject exact-text-only automatic support as the primary mechanism because it cannot recognize faithful paraphrases/translations. The rejected no-extra-call alternative would require: non-verbatim machine text is uncertain and requires explicit user authorship/confirmation or removal. Model assessment is fallible and must never be labeled independently verified truth.

## Call, cost, capacity and latency budget

At most two provider requests per deliberate generation attempt: existing generation (unchanged 4,000 completion-token ceiling), followed by at most one assessment (4,000 completion-token ceiling). Added budget: one request, at most 4,000 completion tokens and a bounded serialized assessment data payload of 128 KiB, containing only generated statements plus the deduplicated cited facts and required context metadata. Trusted fixed policy/schema is additional bounded static overhead. No unrelated Profile fields, accepted evidence excerpts, credentials in payloads, extra provider, repair, retry, chunking or fallback. These are resource ceilings, not a dollar quote or guaranteed spend; input tokenization, actual usage and provider prices determine monetary cost. No live calls are authorized by implementation/test approval.

Use one shared 25-second workflow deadline across generation, assessment, reads and validation; never reset it for assessment. Added latency consumes only the remaining deadline, so no increase to the existing total bound. Skip assessment if capacity or remaining time cannot accommodate it; never truncate facts/statements. If generation has already produced a validated candidate, assessment timeout/outage/key rejection/rate limit/refusal/truncation/malformed/incomplete/duplicate/unknown judgments make the entire assessment unusable: every generated statement becomes uncertain with an unavailable reason. Return that recoverable candidate while the client is connected. Request cancellation ends processing and prevents acceptance of the pending request. Existing generation errors and deterministic structure/reference/numeric/owner/qualifier guard failures remain hard failures. No assessment can override these guards.

## Per-statement semantic outcomes

- supported: every substantive claim is entailed by the cited source text within its original owner, context, assertion, intent and certainty; legitimate translation/paraphrase is allowed. This is a model judgment, not certification.
- uncertain: evidence is ambiguous/insufficient, or assessment is unavailable. Missing evidence never becomes a positive qualification.
- unsupported: text introduces or contradicts a responsibility, technology, qualification, metric, ownership relationship or other substantive claim.

Each outcome includes a bounded reason (maximum 500 UTF-8 bytes), with a deterministic reason code for assessment unavailability. A summary item is one review unit; all of its claims must be supported. Rewrites are separate units. Source IDs establish traceability only. Owner/context references, negation, aspiration, uncertainty, approximate dates and protected metadata retain their existing constraints.

## Backend response contract

Introduce response schemaVersion: 3 while retaining summary, selected and wording for the current logical content. Add a required support object:

```json
{
  "schemaVersion": 3,
  "summary": [{"sourceIds": ["fact-id"], "text": "..."}],
  "selected": ["fact-id"],
  "wording": {},
  "support": {
    "policyVersion": "cv-statement-support-v1",
    "assessment": "complete",
    "statements": [
      {
        "id": "summary:0",
        "text": "...",
        "sourceIds": ["fact-id"],
        "status": "supported",
        "reason": "..."
      }
    ]
  }
}
```

Assessment is complete or unavailable. Server-assigned IDs are summary:<zero-based-index> and wording:<sourceId>, scoped to a single pending result. Each record repeats exact text and citations so clients can reject misassociation; it is not a cryptographic trust mechanism. Require exactly one record for every summary item and rewrite, no extras, duplicates, missing statuses or unknown versions. Rewrite citations are exactly its sourceId. Selected verbatim source facts/protected metadata are not machine-rewritten statements. The assessor returns judgments only for server-assigned IDs; the server attaches canonical text/references. Unknown/malformed response versions cannot silently fall back to legacy machine acceptance. Deployment of #124 alone must not expose v3 through an older client acceptance path: coordinate backend/client activation in #128, using controlled handler tests until the client gate is ready.

## Client decisions and user provenance

Machine-origin active wording requires supported status for acceptance. Global Accept never resolves an uncertain/unsupported statement. Users may discard summary items, revert rewrites to original source text, or explicitly edit/confirm wording with a separate per-statement action. At least one reviewed summary statement must remain; otherwise keep recoverable pending content and require a user-authored summary. No invented filler.

Represent accepted review records with origin generated, user_edited or user_confirmed, exact accepted text/sourceIds, original machine text/judgment for history, and an explicit decision: keep_generated, discard, revert_to_source, edit or confirm. For user_edited/user_confirmed wording the active machine status is not applicable; any earlier assessment remains historical only. Machine support never transfers to changed text, even a small edit; reverting an edit can restore an original assessment only when the exact original text/references and request/source identity still match. A per-statement confirmation may accept unchanged wording as the user's assertion, labeled user-confirmed, never machine-supported. Ordinary typing alone or global Accept does not constitute that confirmation. Original references are historical traceability, not newly supplied evidence. CV-only user authoring never changes canonical Profile facts.

Keep request identity, Profile snapshot/revisions and CV revision guards at review/acceptance. Explicit user confirmation cannot waive stale/forged source identity guards. Revalidate shape and correspondence at acceptance and reload; local metadata and hashes do not authenticate claims against deliberate browser tampering.

## Persistence, compatibility and freshness

New accepted CuratedCv documents use version: 3 under the existing careeros_curated_cv_v1 storage key. Preserve the independent repository snapshot, choices, language, density, original source revisions/evidence/context, summary sources and added statement/provenance/decision records in one atomic write. Save failure retains the previous accepted CV and pending recovery. Persist original judgments and discarded/reverted decisions as history without rendering removed text.

Load saved v1/v2 snapshots without rewriting, deleting, revalidating via provider or mapping positional IDs to canonical references. Existing generated wording is unverified; preserve user content and existing render/export usability. Malformed v3 metadata cannot default to supported or become an accepted v3 document; preserve raw recovery material and the previous usable document. No automatic downgrade to v2 bypasses validation. Freshness (current/stale/legacy) remains an independent axis from semantic status/provenance. Neither current nor a historical supported judgment implies support against changed source revisions.

## Exact implementation dependencies and acceptance evidence

- #124 depends on approved #123: implement backend v3 response, bounded assessment and fail-closed classification with controlled NewHandlerWithClient transport. Preserve all deterministic hard guards.
- #125 depends on #123 and #124: consume v3, show statuses/reasons/cited originals, enforce unresolved acceptance blocking, partial discard/reversion and identity guards. Until #126, unresolved text can only be removed/reverted.
- #126 depends on #123 and #125: explicit per-statement editing/confirmation, separate user provenance and atomic lifecycle preservation.
- #127 depends on #123; coordinate new v3 serialization with #125/#126: retain v1/v2 content/export and unverified indication, reject invalid v3 trust metadata.
- #128 depends on #124–#127: integrated controlled EN/PT backend, client and visible browser verification and coordinated activation. #122 remains the integration anchor and stays open until its complete acceptance criteria are met.

Reuse the already approved public production-handler, client acceptance/serialization and browser seams with synthetic fixtures only. Cover the retained Java-to-Kubernetes reproduction, held-out unsupported responsibilities/tools/qualifications/cross-owner joins, benign paraphrases/translations, unavailable judgments, resource bounds, cancellation, stale references and save failures. Report scripted unsupported-acceptance/benign-rejection counts by language plus controlled call/token-cap/latency evidence; synthetic judgments establish enforcement, not real-world assessor accuracy. No live provider, deployment or actual-PDF quality claim.

## Review baseline and delivery boundaries

Henrique approved immutable review baseline `81b7b9cbe4da36618a667d7c064bef17198b6eef`. Issue #123 is documentation only. Its parent [#122](https://github.com/HenriqueMichelini/careeros/issues/122) belongs to milestone #4, Evidence-backed CVs and applications; the integration branch is `codex/evidence-backed-cvs-and-applications`. Recheck remote refs and ancestry before merging, and synchronize the integration branch with main through a PR where necessary. Keep progress, approvals and completion evidence in GitHub Issues, rather than a second local ticket state.

The already approved review UX, legacy preservation, priority before #52 and controlled test scope remain unchanged. No implementation of ingestion, qualification, screening, observability, retrieval or release gates belongs to this decision. #122 stays open until the integrated outcome is verified.

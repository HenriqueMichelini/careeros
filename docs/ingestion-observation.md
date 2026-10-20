# Ingestion proposal observations

`internal/semanticeval` exposes two offline mechanical projectors:

- `NormalizeLegacyIngestion(IngestionResponseEvidence)` reads one legacy handler response.
- `NormalizeSubmissionIngestion(SubmissionIngestionEvidence)` reads returned plan, extraction, assembly and reconciliation responses.

Both return `ingestion-proposal-observation-v1`. Their inputs contain response IDs, HTTP statuses and body bytes. They accept no `Case`, gold labels, scores or expected facts. They do not dispatch handlers, authenticate receipts, interpret source truth, save a Profile or enable a Lab variant.

The shared boundary is the **reviewable Profile Proposal**, before Apply/Save. Claims retain exact source text, optional meaning/uncertainty, questions, supporting sources and occurrence references. Operations retain destination, entry ownership, wording and supporting claim IDs. Dispositions retain reasons, related Profile fact references and related claims. Returned coverage, skipped records and legacy unresolved/unverified/unplaced accounting remain evidence; discovery coverage never establishes semantic completeness.

Staged projection verifies returned assembly membership against the plan and extractions. It uses the producer's existing batch namespace rule, preserves distinct occurrences of the same batch-local ID, and maps focus-local operation indexes into the flattened observation. It never fills a missing extraction or focus from the assembly membership ledger. The staged unresolved list is the mechanical union of returned clarification, unresolved, contradiction, correction and unsupported dispositions; it does not manufacture a disposition for an absent outcome.

`Raw` retains independent copies of every supplied response and its SHA-256. Body bytes serialize as base64, so malformed JSON can still be exported exactly. `Mappings` links claim, operation, disposition and skipped-record response IDs and JSON pointers to their normalized identities. Coverage entries retain their response ID; their producer location is `/coverage`, or `/context/coverage` for assembly. Plan and receipt data remain in their original response bytes.

`Availability` reports structural usability for claims, operations, dispositions, coverage and the complete endpoint. Each unavailable facet carries reason codes and precise issues. Missing output/stages, unknown fields, duplicate JSON keys, missing required members, broken references, inconsistent coverage or incomplete batch/focus membership cannot establish a complete endpoint. Usable returned rows remain inspectable beside missing work. Explicit valid empty arrays are observable empty output; absent or null required arrays are unavailable. Optional legacy meaning/source metadata stays absent when the producer did not return it.

These are normalization controls only. An omitted fact remains omitted even when the rest of the output is structurally complete; an observed owner or claim wording remains unchanged even if it is wrong. Independent endpoint evaluation is separate work (#209). Receipt authenticity, correspondence to exact original task bytes, no-store enforcement, human semantic judgment, provider quality and user acceptance need their own qualifying evidence.

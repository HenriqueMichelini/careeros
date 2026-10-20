# Whole-submission ingestion protocol

Issue #182 defines the versioned Go/TypeScript contract that lets one foreground
submission carry more than one bounded result. It is a **contract and fixture**
ticket: it pins envelopes, identities, invariants, error vocabulary and budgets.
It does not implement planning, extraction, assembly, reconciliation, the
browser coordinator or persistence; those follow in #184-#189.

This contract is layered on the existing `POST /api/profile/ingest` route and
**preserves** the two earlier protocols:

- **Legacy** requests without `stage` keep their one-response behavior.
- **v1 staged** requests (`stage: {version: 1, kind: "extract" | "reconcile"}`)
  keep the two-request signed-receipt flow shipped in PR #179.
- **Submission** requests (`stage: {version: 2, kind: ...}`) use the envelopes
  below. `protocolVersion` is the string `profile-submission-v1`.

The v1 staged flow is a *bounded unit*: it may return at most one extraction
result. The submission contract adds a whole-submission manifest so many bounded
units can be dispatched sequentially without weakening any #178/#41 boundary.

## Canonical object-key handling

JSON object key order is not identity. A **canonical form** is produced by
decoding the value and re-encoding it with object keys in sorted order, while
array order is preserved and numbers keep their original literal (Go decodes
with `UseNumber`). Values are not normalized, translated or trimmed.

- Go computes a **binding** as the lowercase SHA-256 hex of that canonical form
  (`canonicalBinding`); `requestBinding` in `backend/profile-ingestion/stages.go`
  uses it.
- The browser produces the same shape as a string (`canonicalForm` in
  `src/lib/ingestionProtocol.ts`) and compares it as a local identity key.

The two are **not** byte-compared across runtimes: Go and JavaScript differ in
HTML escaping and number formatting, so each runtime only ever compares a binding
against another it produced itself.

`submissionFieldsError` rejects any field outside an envelope's allowed set as
`input`; strict object-key handling never silently drops or invents fields.

## Budgets

Submission stages retain every #178/#41 bound and add one whole-submission cap.
Capacity is rejected, never silently truncated.

| Bound | Value | Owner |
| --- | --- | --- |
| Original input (UTF-8 bytes) | 30,000 | retained (#41) |
| Portion budget (UTF-8 prepared bytes) | 200-2,000 | retained (#178) |
| Planner portion cap | 256 | retained (#178) |
| Extracted claims per bounded result | 30 | retained (#41) |
| Operations per bounded result | 60 | retained (#41) |
| Clarification answer (UTF-8 bytes) | 1,000 | retained (#181/#191) |
| Staged handler deadline | 27 s | retained (#178) |
| Browser request deadline (incl. body read) | 29 s | retained (#178) |
| Bounded-unit budget | 65 s | retained (#178) |
| Receipt lifetime | 10 min | retained (#178) |
| Extraction / legacy request body | 160 KiB | retained (#41) |
| Per-receipt and aggregate-context payload | 128 KiB | retained (#178) |
| Assembly / reconciliation / clarification request body | 320 KiB | retained (#178/#191) |
| Provider envelope | unchanged | retained (#41) |

### Amendment to #178's per-click attempt

#178 bounded each explicit Review click to 65 seconds. The submission contract
adds a new **absolute 300-second foreground submission cap**. It includes
planning and all local work, every sequential batch dispatch, assembly and
reconciliation, and is measured from the single start action.

- The 65-second bounded-unit budget still applies to each bounded request; it is
  not raised, and a bounded unit never waits on another unit's provider call.
- The 300-second cap is absolute: **no completion-time guarantee** is offered and
  there is **no rolling timeout reset** per batch.
- Work is **finite and manifest-based**: a signed plan enumerates every batch
  before dispatch. Dispatch is **sequential** and **stops on the first
  failure**; there is no parallel provider fan-out, automatic retry or fallback.

## Identities

| Identity | Meaning |
| --- | --- |
| `sourceId` | `preprocessing.Source.ID()`: SHA-256 of `{RulesVersion, StructureVersion, field, original}`. Stable for an exact original. |
| `profileId` | Canonical Profile snapshot identity pinned before dispatch. |
| `documentId` | Optional canonical document identity; absent when no document is sent. |
| `runId` | Unique per foreground submission (session/run identity). |
| `planVersion` | `preprocessing.PlanningVersion` (`portions-v1`). |
| global claim id | `runId + "/" + batchIndex + "/" + localClaimId`, assigned at assembly. |
| temporary entity id | `new:<anchor global claim id>` for a new entry, or a copied existing Profile entry id. |

Batch-local claim ids collide by construction. Assembly **namespaces** them into
global ids and remaps `supportingClaimIds` consistently, preserving each claim's
exact original occurrence. Planning/assembly never guess identity, deduplicate,
resolve conflicts or save.

## Planning envelope (`stage.kind = "plan"`)

Request adds no submission-specific fields beyond `input`, `profile`, optional
`document` and the stage. Planning performs only bounded local work (preparation
and `preprocessing.Source.Plan`) and makes **zero** Jev/OpenAI calls.

Response embeds a signed `plan` plus the workflow `decision`:

```jsonc
{
  "stage": { "version": 2, "kind": "plan" },
  "planReceipt": "<opaque signed handoff>",
  "expiresAt": 0,
  "decision": { "outcome": { "kind": "accept" } },
  "plan": {
    "protocolVersion": "profile-submission-v1",
    "sourceId": "…", "profileId": "…", "documentId": "…", "runId": "…",
    "planVersion": "portions-v1",
    "status": "ready" | "insufficient_capacity",
    "complete": true,
    "capacity": "original_size" | "single_view_capacity" |
                "indivisible_or_context_capacity" | "portion_count" | "",
    "originalBytes": 0, "preparedBytes": 0,
    "batches": [
      {
        "index": 0,
        "primary": [{ "Start": 0, "End": 10 }],
        "context": [{ "Start": 0, "End": 4 }],
        "preparedBytes": 0, "payloadBytes": 0
      }
    ],
    "remaining": []
  }
}
```

### Manifest invariants

1. `protocolVersion` is `profile-submission-v1`; `planVersion` is
   `preprocessing.PlanningVersion` (`portions-v1`); `sourceId`, `profileId` and
   `runId` are present. `status` is `ready` or `insufficient_capacity`;
   `insufficient_capacity` carries a known `capacity` reason and `ready` carries
   none. `complete` is `remaining` empty.
2. `originalBytes` equals the UTF-8 length of the original; `preparedBytes` is
   within the 60,000-byte prepared-workflow bound.
3. Batch indices are `0..n-1` in order and `n <= 256`.
4. Every range lies within `[0, originalBytes)` and on a **UTF-8 rune boundary**
   (`End > Start`). A cut inside a multibyte rune is `unicode_boundary`.
5. The union of all batches' `primary` ranges **plus** `remaining` tiles
   `[0, originalBytes)` exactly once: contiguous, gap-free, non-overlapping.
6. `context` ranges are repeated context, **not** new coverage: each must fall
   inside some batch's `primary` range. A context range outside primary coverage
   is `context_invalid`.
7. A batch's `preparedBytes` is at most 2,000 and `payloadBytes` at most the
   provider envelope; exceeding either is `capacity`.
8. A complete manifest covers every supported source region once. `remaining`,
   `status != ready`, or exhausted limits make the plan **explicitly partial**;
   planning never asserts `discoveryComplete` or "all facts verified".

## Extraction batch envelope (`stage.kind = "extract"`)

The request adds `planReceipt` and `batchIndex`. The server verifies the exact
plan/source/Profile/document/portion/version binding and the signing
configuration **before** provider dispatch, then classifies the exact complete
original with Jev before the single bounded extraction call. Each batch receives
only its selected source and necessary structural context.

The response carries its own purpose-separated HMAC-SHA256 `batchReceipt` with
ten-minute expiry and stable batch identity. A batch receipt authenticates
claims, skipped/invalid/capacity metadata and source references; it is not Jev
acceptance, not `discoveryComplete` and not permission to save.

## Context-assembly envelope (`stage.kind = "assemble"`)

Provider-free. The request supplies `planReceipt` and the complete set of
`batchReceipts`. The server verifies every receipt and the exact expected plan
membership, source/Profile/document/version bindings and primary source
coverage, rejecting missing, duplicated, mixed, expired, tampered or
incompatible batches **before** any further dispatch. It then namespaces claim
ids into global ids.

Response embeds a signed `contextReceipt` and the aggregate:

```jsonc
{
  "stage": { "version": 2, "kind": "assemble" },
  "contextReceipt": "<opaque signed handoff>",
  "expiresAt": 0,
  "context": {
    "protocolVersion": "profile-submission-v1",
    "sourceId": "…", "profileId": "…", "documentId": "…", "runId": "…",
    "planVersion": "portions-v1",
    "batches": [
      { "index": 0, "claimCount": 0, "skippedCount": 0,
        "primary": [{ "Start": 0, "End": 10 }] }
    ],
    "coverage": {
      "validClaims": 0, "invalidClaims": 0,
      "discoveryComplete": false,
      "capacity": "within_limit" | "possibly_exhausted"
    }
  }
}
```

### Aggregate invariants

- `protocolVersion`, `sourceId`, `profileId`, `documentId`, `runId`,
  `planVersion` and per-batch `index`/`primary` match the plan exactly, else
  `batch_mismatch`.
- The batch set equals the plan's batches (no missing or duplicated batch), else
  `manifest_incomplete`.
- Per-batch `claimCount <= 30` and `claimCount == len(claimIds)`. Aggregate claim
  totals **may exceed 30**.
- `coverage.validClaims` and `coverage.invalidClaims` equal the sums of the
  per-batch `claimCount` and `skippedCount`.
- `coverage.discoveryComplete` must be `false` and capacity must be one of the
  two allowed values, else `coverage_invalid`. Assembly performing no provider
  call never proves discovery.
- The serialized aggregate is at most 128 KiB, else `capacity`.

## Reconciliation-focus envelope (`stage.kind = "reconcile"`)

The request supplies `contextReceipt` and a `focus` list of one to thirty global
claim ids. The server verifies aggregate authenticity, expiry and the exact
source/Profile/plan/focus/version binding before any call, classifies the
complete original with Jev, and makes one bounded generation call.

- Necessary earlier and later aggregate claims are supplied as **provisional**
  task data, never as saved or user-approved facts.
- At most 60 operations for at most 30 focus claims; context-only claims may
  support/link decisions but never receive duplicate output.
- Dangling operations, invented existing references, out-of-focus operations and
  oversized provider context are rejected.
- Stable global temporary entity anchors and conservative identity rules apply
  across focus batches.

## Focused-clarification envelope (`stage.kind = "clarify"`)

The request supplies `contextReceipt`, exactly one authenticated `claim` and one
`answer`. It changes exactly that claim and makes no plan/extraction/reconcile
restart.

- The selected claim must match the signed aggregate exactly (id, source,
  question, targets and prior support). A changed, foreign or unauthorized claim
  is `focus_invalid` before any provider call.
- The complete original is classified by Jev again; an answer accepted by Jev
  cannot launder a blocked original. The answer/question/prior support text is
  then classified separately, so rephrasing or attack stops before extraction.
- The answer is at most 1,000 UTF-8 bytes; the blank, support-count and target
  limits are retained. The response retains original support plus exactly one
  `clarification_answer` support carrying the answer; any model omission,
  substitution or extra source is `invalid_output`.
- Only the revised claim is focused; every other aggregate claim stays
  provisional. The established reconciliation validator then recomputes its
  conflicts, destinations and outcome with the revised claim.
- The response replaces that claim's extraction and focus operations/outcomes.
  The client transfers reviewed wording and selection only for unaffected,
  still-savable units; the revised claim and genuinely dependent units restart
  pending, without hiding competing values or unrelated pending work.
- Failure, expiry, rejection, attack or cancellation never discards the pending
  review or blocks unrelated saves. After rephrasing/attack the same trimmed
  answer is blocked until it changes. There is no automatic retry, fallback,
  save or canonical write.

## Submission result and session state

The consolidated review is a single in-memory `SubmissionResult`. Request
envelopes are `SubmissionExtractionRequest` (`planReceipt`, `batchIndex`),
`SubmissionAssemblyRequest` (`planReceipt`, `batchReceipts[]`),
`SubmissionReconciliationRequest` (`contextReceipt`, `focus`) and
`SubmissionClarificationRequest` (`contextReceipt`, `claim`, `answer`); they
share the existing `{input, profile, document?, stage}` transport body.

```ts
interface SubmissionResult {
  protocolVersion: "profile-submission-v1"
  submissionId: string
  sourceId: string
  profileId: string
  documentId?: string
  runId: string
  plan: SubmissionPlan
  batches: { index: number; claims: { id: string }[]; skippedClaims: unknown[] }[]
  context: SubmissionAggregate
  focus: { focus: { claimIds: string[] }; operations: unknown[]; outcomes: unknown[] }[]
  coverage: SubmissionCoverage
}
```

Result invariants (`submissionResultError`): `protocolVersion` matches; claim
ids are unique across batches; each focus batch passes `submissionFocusError` and
carries at most 60 operations; `coverage.validClaims`/`invalidClaims` equal the
batch claim and skipped totals; `coverage.discoveryComplete` is `false`.

Session state (`SubmissionSession`) carries the lifecycle `status`, placed-batch
bookkeeping and one `SubmissionSelection` per selectable unit
(`unitId`, `status`, `dependencies`). Selection `unitId`s are unique and each
status is `pending`, `selected` or `saved`, else `state` or `manifest_incomplete`.

Review starts with **no selection**. Each selectable unit is *pending* (proposed,
not saved), then *selected* (included in the current selection) and finally
*saved* only when explicitly accepted. Pending and selected state are transient
browser memory; only explicitly accepted excerpts and operations persist.

Client session status vocabulary:

```
idle | planning | extracting | assembling | reconciling | reviewing |
saving | saved | paused | stopped | failed | expired | stale | discarded
```

`paused` (left the focused flow, draft retained), `stopped` (immediate abort,
further dispatch prevented), `failed` (limit/malformed/truncated result),
`expired`/`stale` (handoff/input/Profile/credential change) and `discarded`
(separate from Stop) are distinct. Stop/expiry/staleness never erase text,
completed validated results or pending work and never silently retry or save.

### Explicit pause/retry/expiry/staleness contract

- A **retryable comparison failure** may reuse a still-valid extraction/context
  handoff for comparison only; it never re-extracts or repeats paid work
  automatically, and never switches model or plans.
- **Expired or invalid** handoffs stop with an explicit restart requirement and
  no provider fallback. A completed local review is not discarded merely because
  a transport receipt expires.
- Input, Profile/document revision, credential or protocol changes invalidate
  incompatible receipts and block stale saves. Late responses, cross-tab
  updates, repeated clicks and delayed saves cannot overwrite newer work.
- There is exactly **one atomic canonical save** for a submission. It persists
  only explicitly selected changes and accepted excerpts, preserves unrelated
  canonical Profile data, and is resolved against one pinned Profile snapshot.

## Whole-original Jev and origins

Whole-original Jev classification is mandatory on every protected extraction and
reconciliation request. Original-paste excerpts (`professional_information`) and
clarification-answer excerpts (`clarification_answer`) remain distinct.
Planning and assembly make **zero** Jev/OpenAI calls and are not validation-reuse
tokens.

## Error vocabulary

Submission stages use the existing `{"error": "<code>"}` body and `no-store`.
New codes extend the retained v1 set:

| Code | HTTP | Meaning |
| --- | --- | --- |
| `input` | 400 | malformed/oversized request, bad key shape |
| `version` | 400 | unknown protocol, stage or plan version |
| `stage` | 400 | invalid stage kind or stage-only fields |
| `state` | 400 | invalid plan status/capacity, session status or selection status |
| `manifest_incomplete` | 400 | missing/duplicate batch, or coverage gap/overlap |
| `batch_mismatch` | 400 | identity/plan/batch binding mismatch |
| `context_invalid` | 400 | context outside primary coverage, tampered/forged context |
| `coverage_invalid` | 400 | illegal coverage/discovery claim |
| `focus_invalid` | 400 | unknown/duplicate focus claim or dangling reference |
| `unicode_boundary` | 400 | range splits a multibyte rune |
| `capacity` | 400 | a declared bound was exceeded |
| `receipt_invalid` / `receipt_expired` | 400 | signed handoff invalid/expired |
| `configuration` | 503 | missing/short signing key |
| `timeout` | 504 | stage or submission deadline |
| `stale` | 409 | input/Profile/credential revision changed |
| `preparation`, `key`, `rate_limit`, `outage`, `truncated`, `invalid_output`, `cancelled` | retained | unchanged from #178 |

## Verification

`backend/profile-ingestion/testdata/submission-protocol/*.json` holds normative
synthetic fixtures shared by `backend/profile-ingestion/protocol_test.go` and
`tests/submission-protocol.test.mjs`. Fixtures set expected outcomes
independently of handler output and cover multiple batches, invalid versions,
exact UTF-8 boundaries, source/context identities, coverage and terminal states.

# Contextual Profile enrichment from Apply confirmations (#77)

Discovery date: 2026-10-14. Source checkpoint and review baseline:
`a8d5c7af1c91127604caad09accca060a662cfb8` (`main`).
Scope: architecture evaluation only. The user explicitly prioritized this
discovery before #52 closes; that overrides scheduling, not the prohibition on
feature implementation. No application code, contracts, milestones or other
issues change. The future tickets below are proposals, not created tickets.

## Recommendation and evidence limits

Reuse the existing bounded Profile ingestion/reconciliation/review transaction
for an explicitly chosen **user-authored answer**, then save through the existing
browser-local canonical writer. Do not persist Draft's temporary confirmation
facts or append checkbox labels to skills. Keep application-only generation
independent of saving, and start with per-operation review rather than consent
to invisible semantic updates. This is a recommended future design, not existing
Apply behavior or an approved implementation specification.

The current code supports the necessary Profile foundations, but lacks the
Apply-to-ingestion handoff, separate save consent, Apply-origin evidence handling,
and continuity around Profile revision changes. No database, embeddings,
LangGraph, new provider or separate semantic verification call is justified by
this investigation. Reusing ingestion does add its existing provider work;
durable creation alone does not prove later retrieval or reduced questioning.

This is a **pre-release source investigation**, not a post-MVP verification:
[#52](https://github.com/HenriqueMichelini/careeros/issues/52) remains OPEN.
Its [release-gate reconciliation](../release-evidence/2026-10-14-release-gate-reconciliation.md)
retains incomplete application acceptance, citation/support failures and missing
published-route evidence. Those are not automatically retrieval defects.
Historical controlled tests are evidence of bounded invariants, not production
semantic quality. No live providers, private Profiles or deployed browser
sessions were used in this discovery. New-feature success rates, actual charges
and added latency are **not measured**.

## Current-state map

Source references below refer to the checkpoint above; relative links open the
inspected files. Line numbers identify entry points, not proof of every branch.

| Stage | Observed current behavior and concrete source | Capability for enrichment |
| --- | --- | --- |
| Approved input | [`qualificationProjection`](../../src/lib/qualificationEvidence.ts#L6) filters approved professional facts plus existing migrated blocks with unknown provenance; preserves active accepted evidence. [`fields.json`](../../internal/qualificationmatching/fields.json) excludes salary, goals, private notes and contact fields. | Existing professional evidence projection; no save consent implied. |
| Job understanding and matching | [`qualification-gaps/main.go:188,430,521,558`](../../backend/qualification-gaps/main.go#L558) gates the whole original Job Posting, prepares/reuses source-backed job context, selects per-requirement candidates and asks for supported/partial/not-evidenced/clarification decisions. Every non-supported result becomes a gap. | Existing matching; employer requirements remain separate from candidate facts. |
| Candidate retrieval | [`matching.go:92-187`](../../internal/qualificationmatching/matching.go#L92) groups owner/context facts with accepted excerpts, prioritizes lexical overlap and negative/uncertain/aspirational units, keeps whole groups under the default 16,384-byte accounting budget and reports exclusions. [`Resolve:188`](../../internal/qualificationmatching/matching.go#L188) rejects invalid citations and changes incomplete not-evidenced outcomes to clarification. | Existing bounded selection; no guarantee of semantic recall or support. |
| Apply answers | [`HomePage.tsx:113,316-370,791-896`](../../src/pages/HomePage.tsx#L316) stores checkbox selection and notes in mounted session state. Checked requirements become confirmations; every nonempty note also becomes a qualification answer, including unchecked denials/limitations. | Existing application-only path; optional note, no durable target or save action. |
| Draft transport | [`ai.ts:57-104`](../../src/lib/ai.ts#L57) sends confirmations, answers, projected Profile evidence and selected matching fact IDs to `/api/application-draft`. [`main.go:155-176,223-239`](../../backend/application-draft/main.go#L155) validates shape/length, gates the original posting and treats all input as untrusted. It explicitly prohibits saving confirmations. | Existing temporary use; the posting gate is not a professional-answer gate. |
| Draft evidence | [`resumeApplicationDocument:310-346`](../../backend/application-draft/resume.go#L310) constructs transient `application-confirmation-*` and `application-answer-*` facts for support review. Confirmation text may concatenate requirement and answer; these facts have no durable project ownership or accepted excerpts. | Partial reuse for application review only; **unsafe as a durable-fact import**. |
| Application context | [`draftProjection:44-166`](../../backend/application-draft/context.go#L44) uses connected owner/context components, lexical and structured priorities and cited-ID hints under a 24,576-byte serialized-JSON budget. It separately counts relevance and budget exclusions. | Existing Draft selection, distinct from requirement retrieval; `complete` reflects budget exclusions, not universal recall. |
| Profile ingestion | [`profile-ingestion/main.go:155-236`](../../backend/profile-ingestion/main.go#L155) validates canonical/compatibility agreement, bounds preparation, gates original professional text, then extracts and reconciles claims. [`outcomes.go:53-56,301`](../../backend/profile-ingestion/outcomes.go#L53) retains explicit dispositions and conservative identity rules. | Existing proposal engine; Apply handoff/provenance missing. |
| Clarification | [`clarification.go:19-79`](../../backend/profile-ingestion/clarification.go#L19) revises one original ingestion claim and forbids unrelated new facts. [`IngestionClarification.tsx:25`](../../src/components/IngestionClarification.tsx#L25) handles transient answers and feedback. | Existing focused ambiguity resolution, not a generic Apply-question endpoint. |
| Canonical representation | [`profileDocument.ts:42-103`](../../src/lib/profileDocument.ts#L42) has stable entity/fact/evidence IDs and revisions, ownership/context, normalization, temporal precision and semantic flags. [`contract.json`](../../internal/profiledocument/contract.json) has support and role/project/period links; evidence origin is a string. | Existing representation for scoped facts; no technology taxonomy or Apply interaction object required/established. |
| Reviewed write | [`applyIngestionDocument:20-120,164-257,382-421`](../../src/lib/ingestionDocument.ts#L20) checks the exact review snapshot, validates selected operations, translates them to canonical commands and retains approved excerpts. [`editProfileBatch:802`](../../src/lib/profileDocument.ts#L802) applies canonical edits. | Existing canonical transaction; do not rebuild the Profile from its presentation view. |
| Durable save | [`store.tsx:290-323`](../../src/lib/store.tsx#L290) queues reviewed proposals and advances state after `saveProfile` succeeds. [`profileStorage.ts:18-23,78-97`](../../src/lib/profileStorage.ts#L78) uses the shared Web Lock, full expected snapshot and one-step document revision. Storage events mark cross-tab state stale ([`store.tsx:235`](../../src/lib/store.tsx#L235)). | Existing atomic/version-checked save; Apply continuity and retry UX still need design. |
| Reuse and invalidation | [`HomePage.tsx:134-190`](../../src/pages/HomePage.tsx#L134) keys analysis on projected Profile/revisions and resets matches, checkbox choices and notes when that key changes; request IDs/abort suppress late results. [Application reuse](../application-reuse.md) describes session-only caching and deliberate retry. | Existing stale guards; saving during drafting can cancel work and clear answers without a future continuity design. |

### Completed issue outcomes and PR traceability

Live GitHub readback on the discovery date confirmed the following issues CLOSED
and their linked closing PRs. Closure establishes tracker outcome; source/tests
above establish reusable behavior, not semantic correctness on arbitrary inputs.

| Work | Issue / closing PR | Reuse and remaining boundary |
| --- | --- | --- |
| Profile v2 migration | [#36](https://github.com/HenriqueMichelini/careeros/issues/36) / [#73](https://github.com/HenriqueMichelini/careeros/pull/73) | Stable authority/recovery; no Apply save flow. |
| Accepted facts/excerpts | [#39](https://github.com/HenriqueMichelini/careeros/issues/39) / [#76](https://github.com/HenriqueMichelini/careeros/pull/76) | Review and accepted evidence; origin must truthfully identify Apply. |
| Dispositions | [#40](https://github.com/HenriqueMichelini/careeros/issues/40) / [#79](https://github.com/HenriqueMichelini/careeros/pull/79) | Duplicate/overlap/conflict handling; no automatic semantic identity guarantee. |
| Clarification | [#42](https://github.com/HenriqueMichelini/careeros/issues/42) / [#82](https://github.com/HenriqueMichelini/careeros/pull/82) | Focused original-claim revision; not interchangeable with qualification answers. |
| Job understanding | [#44](https://github.com/HenriqueMichelini/careeros/issues/44) / [#84](https://github.com/HenriqueMichelini/careeros/pull/84) | Source-backed requirements; never career evidence. |
| Requirement evidence | [#45](https://github.com/HenriqueMichelini/careeros/issues/45) / [#85](https://github.com/HenriqueMichelini/careeros/pull/85) | Scoped candidates/matching; reduced re-ask needs independent evaluation. |
| Draft context | [#46](https://github.com/HenriqueMichelini/careeros/issues/46) / [#86](https://github.com/HenriqueMichelini/careeros/pull/86) | Task-scoped evidence and temporary answers; no persistence. |
| Reuse/cancel | [#49](https://github.com/HenriqueMichelini/careeros/issues/49) / [#89](https://github.com/HenriqueMichelini/careeros/pull/89) | Session identities; optional save adds a revision transition. |
| Migration cleanup | [#51](https://github.com/HenriqueMichelini/careeros/issues/51) / [#91](https://github.com/HenriqueMichelini/careeros/pull/91) | Canonical writers retained; no second legacy writer should be introduced. |
| End-to-end gate | [#52](https://github.com/HenriqueMichelini/careeros/issues/52), OPEN; [#114](https://github.com/HenriqueMichelini/careeros/pull/114) merged in inspected main | PR merge does not close or pass the release gate; preserve its remaining evidence. |

[#96](https://github.com/HenriqueMichelini/careeros/issues/96) is OPEN, and no
semantic-recall research report was present in this checkpoint. It is related
research, not a prerequisite. Its concern is already relevant: facts that are
saved successfully can still be omitted before the matcher sees them. The
[existing #45 evaluation](../evaluations/qualification-evidence/verification.md)
reported 15/15 labeled relevant facts at 16 KiB and 13/15 at 2 KiB, with 13/13
and 12/13 controlled decisions respectively. Those are historical synthetic
corpus measurements, not AWS enrichment results or live-model accuracy.

## Integration options and minimal proposed data flow

| Option | Benefits | Costs / limitations | Recommendation |
| --- | --- | --- | --- |
| Deliberate handoff into existing ingestion + review | Reuses field gate, reconciliation, source excerpts, review and transaction semantics; fewest independent policies. | Requires Apply origin/target binding, recipient disclosure and session continuity; existing semantic calls still cost time/credits. | Preferred initial scope. |
| New Apply-specific analysis endpoint, same canonical writer | Could receive structured question/answer/target context without blending sources. | Duplicates analysis policy unless a narrow adapter shares existing engine; strict schemas and quality evaluation still required. | Consider only if existing endpoint cannot preserve source distinctions. |
| New direct Profile writer from confirmations or Draft facts | Superficially fewer steps/calls. | Checkbox/requirement promotion, unsupported ownership, duplicate/conflict logic and stale writes; no semantic review equivalence. | Reject. |

Proposed flow, subject to the decisions below:

1. The application-only route continues to use the current note/confirmation.
   Offer a separate action to **review this information for the Profile**.
   Explain that review transfers selected professional text to TypeSafe and
   OpenAI and may use credits; it is not yet a successful save.
2. Freeze a transient handoff containing original answer bytes, selected Profile
   ID/revision, explicit target entity ID/revision if chosen, request identity
   and language. Keep requirement/question context in a separate untrusted
   channel for interpretation; never concatenate it into the asserted answer.
   An empty answer or bare checkbox supplies no durable professional claim.
3. Prefer `/api/profile/ingest` through a narrow future adapter. First evaluate
   whether its strict request/response can carry a server-validated Apply origin
   and target hint; these fields are **not supported today**. Do not add unknown
   fields ad hoc or forge a clarification claim. Reuse whole-answer professional
   classification, source mapping, bounds and existing reconciliation. Selected
   project identity is context, not permission to infer additional capabilities.
4. Review a before/after diff per destination and all claim dispositions. Show
   changed wording, retained qualifiers, linked owner/period and accepted source
   excerpts. New facts, new support and unresolved conflicts remain distinguishable.
   A batch Save can approve the visible selected operations; ambiguous ownership,
   contradictions/corrections and substantive edited proposals require focused
   clarification or renewed review. No default invisible semantic changes.
5. Invoke `applyIngestionProposal` → `applyIngestionDocument` → `editProfileBatch`
   → `saveProfile`. No server write, direct localStorage mutation, legacy import
   or reconstruction from a flat Profile view. Show success and clear pending
   save state only after durable completion. Preserve unrelated facts and links.
6. A later application projects the current approved Profile and reruns matching.
   Supported basic AWS usage can remove that question only if adequately
   retrieved and semantically matched. Duration, service-specific experience,
   proficiency and certification must still be asked when unsupported.

### Entity resolution, evidence and impact

Use existing identities and conservative dispositions, not a new ontology. For
the Personal Finance API example, first resolve the project unambiguously. A
supported proposal may enrich its technologies/highlights with Lambda/S3 use
and link the accepted answer excerpt to the affected facts. Do not create a new
project solely because its name is paraphrased, move the claim to an unrelated
employment role, or infer project dates/individual leadership from team usage.

AWS, Lambda and S3 are related but not equivalent; the current outcome policy
explicitly preserves that distinction. Basic service usage is not advanced AWS
expertise, years of usage, another AWS service or certification. Mechanical exact
aliases can normalize spelling while retaining observed wording. Semantic
paraphrases, overlapping details and distinct periods require contextual review.

An exact repeated assertion with the same owner/period should resolve as a
duplicate; newly approved supporting text can be an evidence-only update. New
information can enrich the same project without replacing unrelated facts.
Contradictions and corrections remain review candidates, preserving both
statements until resolved. Existing support revision/invalidation semantics
must remain truthful after edited wording or ownership; a source reference is
traceability, not a guarantee that the source supports every new subclaim.

Evidence currently permits an origin string but no dedicated Apply interaction
object. Prefer a bounded `apply_answer` origin on approved excerpts, mapped
through validated provenance rather than a client-controlled arbitrary label.
Whether a minimal question excerpt must also be retained is unresolved: never
silently store the whole Job Posting or dialogue. Existing fact-origin enums
should not be expanded without need; `accepted_proposal` remains truthful for
reviewed AI proposals, while edited wording retains existing user-edit semantics.

## Consent, ordering and failure boundaries

Recommended initial UX: generate/review the application independently, then offer
optional Profile enrichment. Serialize save against any active application
generation and retain answer text in a transient handoff. This reduces the risk
of changing revisions beneath in-flight support review. It does not eliminate
the need to handle concurrent manual/cross-tab edits.

| Situation | Required future behavior |
| --- | --- |
| Save declined / temporary only | Generation continues; identical saved Profile bytes. No ingestion call solely because a checkbox was selected. |
| Generation fails | Retain application answers; optional enrichment can be deliberately reviewed independently. Saving does not claim Draft success. |
| Save fails or is cancelled before commit | Retain answer/proposal and existing Draft. No success badge or automatic retry; failure is independent of application generation. Cancellation after a completed atomic commit must report the actual saved state. |
| Save succeeds | Keep existing artifacts inspectable as their original snapshot. Do not substitute new facts into accepted artifacts. Refresh analysis/context only on a deliberate new attempt with current revisions. |
| Profile changes during review | Reject the stale snapshot, preserve recovery text, rebuild/reconcile against the current Profile and obtain renewed review. Never silently rebase approved semantic operations. |
| Cross-tab update | Use the shared lock and expected snapshot; follow existing stale/reload handling. Origin locks serialize writes, not intent or semantic merges. |
| Repeat click / late response | Request identity suppresses obsolete responses; serialize local save and reject an already-advanced snapshot. After reload/retry, reconcile against durable facts/evidence rather than repeating an old patch. |
| Retry after uncertain feedback | Read the durable revision first. If committed, show that outcome; otherwise deliberately re-review. Exact duplicates/additional-support checks prevent new duplicate facts or identical evidence. No automatic provider retry. |
| Inspect / reverse | Reuse fact details and canonical manual editing to inspect provenance and correct/remove specific accepted changes. There is no established one-click transaction undo; targeted reversal must check current revisions and preserve later unrelated edits, with truthful support invalidation. |

The present analysis-key reset clears notes/checkboxes on Profile changes. A
future handoff must preserve original answers independently through this reset
without reapplying stale qualification decisions. Existing selected CV/snapshot
and artifact revision behavior must remain intact; refresh is not silent
regeneration, acceptance or replacement.

## Privacy, trust, cost and architecture

The [stateless ADR](../adr/0001-stateless-user-key-backend.md) and
[professional-field policy](../profile-field-validation.md) remain sufficient
for this proposed design. Durable authority stays browser-local, using accepted
facts/excerpts only. Full answers pending review, complete postings and session
handoffs remain transient. A recipient/retention review must cover both the
first-party handler's comparison inputs and minimized downstream provider
payloads; a private field containing sensitive answer text does not become safe
merely because the transfer is bounded.

Today Application Draft classifies the original Job Posting, not each answer as
a professional Field Submission. Saving must invoke the professional-information
gate separately. Reuse of job context, checksum, checkbox consent or an accepted
Draft cannot bypass it. Generated questions, employer text, target hints and
client-supplied origins are untrusted data. No external content controls prompts
or grants permission to save. Deterministic preparation preserves original
occurrences and capacities; it neither validates meaning nor establishes safety.

Code-path estimate from `profile-ingestion/main.go:189-226`: a successful ordinary
ingestion attempt adds one TypeSafe classification and two OpenAI requests
(extraction and comparison). A successful clarification attempt adds two
TypeSafe classifications (original and clarification context) and two OpenAI
requests (clarify and compare). Early failures can dispatch fewer. Continuation
and deliberate retries can add further attempts; these are not fixed journey
totals. The shared ingestion deadline is 52 seconds, per-OpenAI timeout 24
seconds, and browser timeout 55 seconds ([`ingestion.ts:938-976`](../../src/lib/ingestion.ts#L938)).
Timeouts are limits, not measured latency. A local commit adds no provider call;
the semantic proposal path does. An optional manual-edit route could avoid AI
work but has different reconciliation/evidence guarantees and cannot be sold as
equivalent automatic enrichment. Actual tokens, charges and accepted-result
cost need a separately authorized bounded evaluation; no provider use is
authorized by this report.

## Decision register and rollout prerequisites

These are recommendations for human review, not decisions already granted by
prioritizing discovery. Address any implementation decision to the user.

| Choice | Recommended default / trade-off | Decision still needed |
| --- | --- | --- |
| Consent granularity | Explicit review action followed by visible selected-operation Save; more work than one silent opt-in, but scope and destination stay inspectable. | Approve labels, selection defaults and what a batch Save authorizes. |
| Analysis boundary | Narrow adapter around ingestion, same canonical transaction; avoids a parallel writer. | Confirm feasible strict origin/target schema in a future design review; new endpoint only if necessary. |
| Ownership | Ask for an existing project/role when ambiguous; preserve unknowns. | Confirm ambiguity UX and whether owner-free claims may be saved to appropriate Profile-level fields. |
| Provenance | Approved answer excerpt + truthful Apply origin; no full posting/dialogue retention. | Decide whether bounded question context is necessary and its consent/retention rules. |
| Ordering | Optional enrichment after generation/review; serialize against active generation. | Confirm exact placement and preservation/refresh behavior after revision changes. |
| Reversal | Inspect and correct/remove via canonical editing first. | Decide whether scoped transaction undo is needed; no snapshot rollback over later changes. |
| Evaluation budget | Controlled fixtures first; independent semantic adjudication and bounded provider cohort only with explicit approval. | User approves public seams, fixed baseline, recipients, limits and any paid runs. |
| Scheduling | Future implementation after #52 and active MVP backlog; #96 informative, not blocking. | User explicitly authorizes implementation/milestone creation after reviewing discovery. |

Non-goals: automatic writes, checkbox-to-skill promotion, employer requirements
as evidence, generic graph/ontology, backend accounts/database, raw-source
archive, automatic retries/fallback, model tuning, new providers, retrieval
redesign or waiving the current release gate. None is required to deliver this
discovery. Revisit any infrastructure proposal only after measured need and a
separate ownership/privacy decision.

## Proposed future milestone and tickets — not created

Proposed milestone: **Reviewed Apply-to-Profile enrichment**, scheduled after the
active MVP backlog and #52 completion. Create nothing until requested. Existing
closed tickets keep their original scopes. The following dependency order is a
proposal; resolve the decision register before implementation.

| Order | Proposed ticket / dependency | Acceptance criteria |
| --- | --- | --- |
| F1 | Approve consent, provenance and strict handoff design; after #52 and user prioritization | Define answer vs employer context, target/revision identity, approved excerpt retention, recipient/call disclosure and reversal policy; EN/PT product review; agree test seams and baseline. No new writer. |
| F2 | Adapt bounded ingestion for Apply-origin review; depends F1 | Whole answer gated as professional information; bounds and source mapping retained; validated origin/target hints; existing disposition engine; exact duplicate/additional support/ambiguous owner/conflict fixtures; no question/requirement promotion and no prompt/schema bypass. |
| F3 | Optional review/save UI and canonical transaction; depends F2 | Visible destination/diff/qualifiers/excerpts and per-operation selection; one batch explicit Save for selected changes; atomic expected-revision persistence before success; application-only path remains usable; save failure/cancel preserves input, unrelated facts and Draft. |
| F4 | Application continuity and retry/reversal; depends F3 | Preserve notes across save-induced analysis resets; obsolete responses cannot overwrite state; repeated confirmation/uncertain feedback yields no duplicate facts/evidence; cross-tab/stale rebuild requires renewed review; existing artifacts remain snapshot-bound; canonical corrections preserve later edits. |
| F5 | Evaluate durable enrichment and later matching separately; depends F4 | Run matrix below at agreed public seams in EN/PT; inspect candidate inclusion then matching decisions then Draft selection; publish denominators and missing evidence; bounded provider work only separately authorized; close only after persistence integrity and supported-claim criteria pass. |

## Scenario and test matrix

All rows are **future feature expectations, not executed feature tests**. Reuse
existing public Profile transaction/storage, HTTP-handler and browser review
seams when agreed; add no test or benchmark implementation during discovery.
Use synthetic personal projects and postings. Run each semantic row in EN/PT,
including PT answers to EN postings; preserve original excerpts and unknowns.

| Scenario | Required enrichment behavior | Independent matching/application check |
| --- | --- | --- |
| Fresh AWS claim | Link Lambda processing and S3 storage to the identified Personal Finance API project; approve excerpts, no invented proficiency/period. | Later basic-use requirement finds project context; certification/advanced-service questions remain. |
| AWS already supported | Same fact/owner/period produces duplicate or genuinely new evidence-only support; stable existing IDs. | Verify candidate inclusion and supported decision without a new save. |
| Unrelated project | Attach only to the named/chosen project, preserving other employment/project facts. | No qualification transfer from another employer or invented combined usage. |
| Ambiguous project identity | Ask for target clarification; no project merge or guessed owner. | Application-only generation still works with explicit limited context. |
| Related, not equivalent tech | Keep AWS, Lambda, S3, EC2 and other services distinct; no alias equivalence beyond established policy. | A Lambda claim does not establish EC2 or general expertise; related evidence may warrant partial support. |
| Repeat confirmation / double Save | One durable change; repeat yields duplicate, not appended skill or evidence clone. | Re-ask outcome evaluated after fresh analysis, not by suppressing questions in UI. |
| Contradictory / corrected claim | Preserve competing sources, negation and periods; require explicit resolution and truthful support invalidation. | No positive match from denied/aspirational/uncertain experience. |
| Team-only / approximate time | Preserve limited ownership and approximate wording; role dates do not become service duration. | Ask for individual scope/duration when material and unsupported. |
| Denied / temporary-only save | No durable mutation or enrichment calls on declined save. Unchecked factual answers stay usable for application limitations. | Application generation independent of persistence decision. |
| Stale revision / cross-tab | Reject old snapshot, retain handoff, reload and re-review; no automatic merge. | Old matching/drafting response cannot adopt new facts or clear preserved notes. |
| Quota/lock failure / cancel / deliberate retry | No false success or lost input; inspect durable outcome before retry; no duplicate committed changes. | Valid Draft remains usable; save outage cannot force generation failure. |
| Retrieval budget / paraphrase / translation | Save succeeds independently of candidate selection. | Trace relevant fact inclusion at 16 KiB, semantic decision, then 24 KiB Draft context; disclose omissions and never infer absence. |
| Malicious answer / job instructions / empty checkbox | Professional gate and source distinction enforced; no automatic asserted facts from question labels. | No persistence authority from a valid posting/hash or temporary confirmation. |
| Inspect/correct/reload | Accepted excerpts/origin survive reload; targeted canonical correction/removal preserves unrelated later edits. | Previously accepted artifact stays its own snapshot and requires deliberate regeneration for new context. |

## Regression risks, evidence and measures

Highest risks are provenance laundering from requirement text, wrong project
resolution, unsupported strength/duration, stale writes, answer loss on analysis
reset, duplicate support on retry and declaring re-ask success after save alone.
The matrix must distinguish three failure layers: durable enrichment,
requirement candidate retrieval/semantic matching, and downstream Draft context
or artifact support. A valid source ID cannot substitute for semantic review.

Existing coverage to retain:

- [`tests/profile-document.test.mjs`](../../tests/profile-document.test.mjs):
  migration/storage/stale/concurrency cases (108-169), accepted evidence and edited
  proposals (1145-1257), metadata stale/support/clarification provenance (1442-1522).
- [`tests/ingestion.test.mjs`](../../tests/ingestion.test.mjs): source integrity,
  stale proposal and focused clarification merge behavior; not an Apply handoff.
- [`tests/qualification-evidence.test.mjs`](../../tests/qualification-evidence.test.mjs)
  and [#45's controlled corpus/report](../evaluations/qualification-evidence/verification.md):
  approved projection and bounded synthetic retrieval/contract decisions.
- [`tests/evidence-mvp.test.mjs`](../../tests/evidence-mvp.test.mjs),
  [Draft context verification](../evaluations/application-context/verification.md)
  and [reuse documentation](../application-reuse.md): integration/review,
  temporary facts and artifact revision/cancellation contracts, not feature quality.

Proposed measurements (all new-feature values currently **not measured**):

| Measure | Denominator / interpretation |
| --- | --- |
| Duplicate-fact and repeated-evidence rate | Unexpected duplicate durable facts/evidence per labeled repeated-save/retry case; report both counts and opportunities. |
| Unsupported-fact rate | Unsupported atomic subclaims per all proposed and separately all accepted subclaims, independently adjudicated; no exclusions hiding failed attempts. |
| Evidence-link/owner correctness | Correct fact revision, excerpt and owner/context associations per all expected links, plus missing/wrong links; report ambiguous cases separately. |
| Re-ask rate | Unnecessary repeated questions per later requirements fully supported by approved facts, with candidate inclusion and matcher outcomes reported separately; necessary scope questions are not failures. |
| Persistence integrity | Cases preserving unrelated data, exact durable revision and approved excerpts per all stale/cancel/quota/lock/retry cases; separately count false success and data loss. |
| Added calls, cost and latency | Dispatches/tokens per optional review and clarification attempt; end-to-end optional-save duration and accepted-enrichment cost, with failures/declines retained. Actual charges unavailable until measured; accepted-result cost undefined if none accepted. |

Feature rollout requires the explicit decisions above, passing controlled
invariants and semantic review at the agreed seams, an independently approved
provider/deployment verification plan where needed, and completion of the active
MVP gate. This report does not certify any of those future outcomes.

## Discovery validation

Source and live tracker inspection established the current map and closed issue
links. TypeScript checking and the existing qualification-projection test file
passed during investigation. Final repository checks and independent documentation
review are recorded in the accompanying PR/issue, separate from historical
evaluation results. No new tests or runtime changes are part of this report.

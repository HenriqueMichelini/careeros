# General-purpose CV generation contract (#27, #43)

Defined before implementation. `POST /api/cv/generate` is a separate fixed workflow, with no posting. The browser sends `{cvLanguage, density, facts}`: an ordered list of `{id, section, entryId, field, text}` professional facts. Contact identity/links, career goals, compensation, employment status, additional information, credential IDs and URLs never enter this projection. Experience titles/employers/dates, project names, qualifications and language proficiency are source facts; they remain verbatim in the local CV. New generation uses canonical Profile fact IDs with `{profileId,id,revision}` references, owner/context references, block kind, assertion, intent, certainty and support state. Legacy migrated blocks remain whole, exact source material. Old request-local contracts remain replayable and saved v1 snapshots stay loadable; their IDs are never mapped to current Profile facts.

The provider uses a strict closed JSON schema: `{summary: [{sourceIds: [id],text}], selected: [id], wording: [{sourceId,text}]}`. The handler normalizes wording to the existing client map. Every summary sentence cites all substantive facts needed to support its claims. Legacy single `sourceId` responses remain replayable, with the same strict domain checks. Multiple references cannot join facts across employers/projects. Optional supporting wording condenses a selected description, responsibility, achievement, project highlight or qualification detail; selection can consolidate redundancy by retaining its strongest supported representative. Employer, role, dates, degree, certification and proficiency metadata remain verbatim in the local snapshot and cannot be replaced through `wording`.

The backend validates references, structure, transport bounds, original numeric claims and explicit negation/qualification/proficiency markers against each cited fact. The fixed prompt forbids new claims, metrics, qualifications or stronger seniority. These guards catch concrete factual corruptions but do not prove semantic equivalence of arbitrary paraphrases; explicit review shows original sources alongside every proposed wording before acceptance. Controlled checks test both supported paraphrases and rejected corruptions. Provider quality is not asserted without a separately authorized provider check.

The fixed selection policy ranks relevance to the demonstrated professional trajectory, evidence of impact, recency when informative and complementary qualifications. Deduplicate overlapping responsibilities/tools/skills; prefer outcomes over filler; balance experience with distinct projects and qualifications. Neither input position nor fixed slices determine selection. Aim for a concise one-page CV, but the existing DOM A4 measurement is the authority; no character cap, clipping or automatic content deletion enforces page fit.

A result is staged for review, then explicitly accepted. Acceptance builds a browser-local snapshot with the original contact information, verbatim selected metadata and selected supporting facts; the snapshot and its choices persist atomically in one document independently from the Profile, legacy CV preferences and job-specific drafts. Generation cannot change Profile. Existing edits remain until explicit acceptance; request identity, source snapshot and CV revision must still match. Cancel, unmount, CV language/key/source changes, subsequent edits and newer requests invalidate pending results. No automatic retries.

Bounds: 128 KiB HTTP body, 500 facts, 12 KiB per fact, 100-byte IDs, 25-second upstream deadline, 1 MiB upstream response and 4,000 output tokens. These are transport limits, not CV-fit rules. Responses carry `Cache-Control: no-store`; keys exist only in request headers; no content/key logs or backend persistence. User-entered professional fields may themselves contain sensitive text: the UI explains the exact projection before the deliberate request.

Verification seams: HTTP request/provider/result contract; browser public projection, acceptance and persistence functions; existing live preview/PDF scripts plus controlled generation UI fixtures. Real-provider quality and deployed-route verification require separate evidence and are not implied by controlled responses.

## Content density (#28)

The request additionally carries `density: "compact" | "balanced" | "detailed"`. Missing legacy values use `balanced`; invalid values are rejected before upstream work. Density is a fixed policy input, independent of font size: Compact selects the strongest distinct evidence with fewer supporting details and tighter summaries; Balanced retains representative experience, outcomes and complementary qualifications; Detailed includes additional useful context and nonredundant supporting evidence. All modes preserve facts, consolidate overlap and target one A4 page. Sparse Profiles need not manufacture differences or filler. No policy uses fixed slices or arbitrary text truncation. Detailed may exceed one page; actual DOM measurement and printable overflow remain authoritative.

CV preferences persist density alongside font size under `careeros_cv_preferences_v1`. Accepted snapshots record the generation density; legacy snapshots display Balanced. Changing preference updates neither current content nor Profile, and invalidates in-flight/pending proposals. The UI distinguishes the saved CV's density from a pending setting. Only deliberate generation and explicit acceptance replace content and edits. Font controls continue to use their existing default when missing.

Controlled verification compares all three densities against the same short and 19-role heavy sources in both locales at desktop and 390px, with extractable A4 PDFs. Direct test-only printing inspects overflow across pages; ordinary app export still blocks overflowing documents. Synthetic responses deliberately return different supported selections to exercise request propagation and accepted volume. These comparisons establish UI/request/persistence/fit behavior, not real-provider selection quality; no provider check was performed for #28.

## Language settings

Site language (`uiLocale`, persisted under `careeros_locale`) controls navigation, controls, feedback and the HTML document language. CV language (`cvLanguage`, persisted under `careeros_cv_language`) independently selects English (`en`) or Brazilian Portuguese (`pt-BR`) for CV headings and new CV/application prose. The initial CV preference uses a saved CV document language when present, otherwise the initial site language; later site changes never update it.

`POST /api/cv/generate` validates `cvLanguage` and passes it to the fixed provider policy. Legacy `locale` requests remain supported as CV language only; conflicting fields are rejected. Accepted snapshots save `cvLanguage`; legacy snapshot `locale` migrates when read. An accepted document retains its original language until a new result is accepted, including its headings. Profile facts and existing edited prose are never automatically translated.

`POST /api/application-draft` also accepts validated `cvLanguage`, which overrides the posting language for generated prose and resume headings. Older clients without the field retain their previous behavior. The browser records the requested language with the generated materials so Results headings remain stable when the site language changes. Site language is never a generation input.

## Stable support and saved CV compatibility (#43)

The outbound projection is explicit: source text and professional metadata plus stable reference, owner, context, kind, assertion, intent, certainty and support. Accepted evidence excerpts/origins stay browser-local and are shown during proposal review and saved-source inspection. No source paste, contact fields, credential URLs or unrelated private fields are sent. Whole legacy blocks are neither split into inferred atomic facts nor assigned fabricated certainty. Canonical statements with non-actual intent, non-affirmed assertion or uncertain/unknown certainty remain context only and cannot be selected as demonstrated qualifications; an entry with such protected metadata is rejected rather than implicitly displaying an earned qualification. Original negative/aspirational wording receives conservative guards. These checks and source links are not a semantic proof; the user explicitly reviews and accepts the wording.

New accepted documents use `version: 2` under the existing atomic CV storage key. They retain an independent rendered repository snapshot, manual choices, original selected fact revisions and their accepted evidence, summary source links, and original contact/protected context revisions. Profile edits, fact removals, context changes and evidence invalidation can mark live support stale without rewriting the accepted CV. Unrelated Profile edits do not stale unchanged fact references. Saved v1 snapshots continue rendering/exporting with their own choices, language and density; the UI says their live support is unavailable and never guesses a positional-ID mapping. Storage failure retains the old accepted document and recoverable pending review. Both locales expose original sources and a traceability warning.

Provider refusals, incomplete/truncated output, empty summaries and malformed or missing references still fail validation. The new schema requires a nonempty summary and the policy asks for supported prose even for sparse sources; this is no claim of a live reliability fix. The Portuguese #34 empty-summary failure remains a rejection regression. The uncaptured English response has no established root cause. Original baseline reports/fixtures are unchanged; `cases.stable-cv.v1.json` adds controlled EN/PT stable-reference acceptance and rejection cases to the existing evaluation pipeline. #65 remains separate.

The client also rejects over-capacity projections before HTTP. Bounds remain 500 facts, 12 KiB text each and 128 KiB request, with no silent truncation. Actual A4 measurement/export handling and font/density/language preferences remain authoritative. Controlled browser checks include short/heavy EN/PT at 1440px and 390px, all densities, saved v1/v2 reload/manual edits, stale support, storage failure, cancellation and export overflow. Controlled results do not establish live-provider quality or deployed behavior.

Schema contract follows [OpenAI Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs): all object fields are required and objects reject additional properties. Domain validation remains necessary beyond structural enforcement.

## Approved statement-support extension (#123; backend implemented in #124)

[ADR 0003](adr/0003-general-cv-statement-support.md) defines the approved bounded independent assessment, per-statement support/provenance contract, v3 response and saved-document migration, and dependencies for #124–#128. The preceding sections describe the legacy response; v3 is an opt-in contract until the client gate in #125/#128 consumes it. Existing freshness and source links do not establish semantic support. No live-provider validation was performed.

### Backend activation and response (#124)

A request opts into the v3 response with `statementSupport: true`. Without it, the response is the unchanged legacy `{summary, selected, wording}` schema and exactly one provider call is made, so an older client acceptance path never receives assessed wording as if it were certified.

With the flag, generation is followed by at most one independent support assessment using the same `gpt-6-luna` model, `reasoning_effort: none` and 4,000 completion-token ceiling. Assessment reuses the single 25-second workflow deadline (it is never reset or extended) and sends only the generated statements plus the deduplicated cited facts and their context metadata, capped at 128 KiB. The response keeps `summary`, `selected` and `wording` and adds:

```json
{
  "schemaVersion": 3,
  "summary": [{ "sourceIds": ["fact-id"], "text": "..." }],
  "selected": ["fact-id"],
  "wording": {},
  "support": {
    "policyVersion": "cv-statement-support-v1",
    "assessment": "complete",
    "statements": [
      { "id": "summary:0", "text": "...", "sourceIds": ["fact-id"], "status": "supported", "reason": "..." }
    ]
  }
}
```

Review units are `summary:<index>` and `wording:<sourceId>`; there is exactly one record per summary item and rewrite, with no extras, duplicates, missing statuses or unknown versions. `status` is `supported`, `uncertain` or `unsupported` and `reason` is bounded to 500 bytes. Valid source IDs establish traceability only and never support.

Any assessment failure — timeout, outage, key rejection, rate limit, refusal, truncation, malformed, incomplete, duplicate or unknown judgments, or an over-capacity payload — is recoverable and fail-closed: `assessment` becomes `unavailable` and every statement becomes `uncertain` with reason `assessment_unavailable`. The generated candidate is still returned. The deterministic structure, reference, numeric, negation, owner and qualifier guards run before assessment and remain hard failures that no assessment can override, so a hard-guard failure returns `invalid_output` without any assessment call. Assessment is a model judgment, not certification, and is not semantic proof.

Backend implementation and handler tests intercept all provider traffic with synthetic EN/PT fixtures and assert call counts, ceilings and fail-closed outcomes. Client acceptance, provenance and legacy-load behavior remain #125–#128; coordinated activation is #128.

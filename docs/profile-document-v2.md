# Browser-local Profile v2

Issue #36 introduces an expand-phase boundary. `careeros_profile_v2` contains the only editable, authoritative Profile. A successful atomic `localStorage.setItem` installs the authority after the candidate passes validation. The unchanged `careeros_repo` key is the recovery snapshot of pre-migration data. No subsequent Profile writes update that key. A malformed v2 document never falls back silently to an older snapshot.

`src/lib/profileDocument.ts` defines migration, validation, compatibility projection and whole-field replacement. `internal/profiledocument/contract.json` defines the shared wire shape; both TypeScript and Go evaluate its small schema subset and separately validate domain references. `fixtures.json` is the common positive/negative parity corpus. The Go contract package is not connected to handlers or persistence. Existing HTTP workflow contracts continue receiving `careerProfile` and qualification views; no provider, account, database or synchronization is added.

## Identity and meaning

A document has a stable Profile ID and monotonic revision. Every existing career entry becomes a stable entity with its original entry ID (`legacyId`), section, revision and explicit order. Every field is a stable revisioned fact owned by the Profile or an entity. Unchanged fields retain identity, wording, semantics and support metadata. Unknown extension fields remain opaque JSON values. Missing section/contact values have safe empty compatibility defaults; missing information does not become a qualification.

Migration stores original strings as whole `legacy_block` facts, including skill/tool lists and uncertain narrative. It never splits text or interprets negation, aspirations, uncertainty, dates or duration. Their semantic qualifiers and date precision remain `unknown`; original date wording is preserved. Future `statement` facts can explicitly distinguish affirmed/negated claims, actual/aspirational intent, certain/uncertain assertions and exact/approximate/unknown temporal wording. No dates or duration anchors are inferred.

Migration origin is `existing_profile` with `original: unknown`, approval is `unreviewed`, and support is `unsupported`. These are separate properties: origin is traceability, approval records a deliberate acceptance, and support requires an active link to an approved excerpt. None asserts external verification. Manual whole-field edits have `manual_edit/user` origin. Accepted ingestion changes have `accepted_proposal/unknown` origin; accepted section AI Review uses `ai_review/unknown` and is approved only after explicit review/apply. Durable ingestion evidence conversion belongs to #39, and section rewriting uses the explicit proposal boundary from #38.

Observed terminology is retained separately from any canonical value, with policy `exact-alias-v1`. Only the exact spellings JavaScript/Javascript and TypeScript/Typescript are approved aliases. `JS`, `TS`, `AWS`, Spring relationships, whitespace variants and arbitrary legacy blocks remain unresolved. Alias lookup implies no proficiency, production use, duration or ownership.

## References and correction

Every owner, context and link endpoint names the same Profile and a stable node ID/revision. Active links are narrowly typed: a fact `supports` link targets approved evidence; `role_context` targets an experience entity; `project_context` targets a project; `period_context` targets an existing career entity whose period wording supplies context. There is no generic graph or inferred period entity. Evidence stores only an approved excerpt and its origin, never a full paste or pending proposal.

A temporary whole-field write replaces the affected fact with a legacy block and increments its revision. It does not transfer evidence across changed wording. Removal invalidates historical links to deleted facts/entities. Context or owner revision changes invalidate dependent support; outdated context references are removed from the current fact. Invalidated links may retain old endpoints as history within the same Profile. Approved excerpts remain retained when their support is invalidated, for review and future explicit correction. Evidence is neither reattributed nor reassigned heuristically.

## Persistence and recovery

All migration/save operations acquire one origin-scoped Web Lock. Saves compare the entire expected canonical document and require the next document revision before the atomic write. The in-memory authoritative document advances only after persistence. Browsers without Web Locks fail safely; use HTTPS or localhost in a browser with Web Locks. Other tabs' revision changes block saves and preserve local draft/review input, requiring deliberate reload and reapplication. This is conflict detection, not synchronization.

UI edits remain available as in-memory drafts if persistence fails. Accepted ingestion input/proposals are cleared only after a successful durable save. The localized recovery banner identifies storage, validation, lock and stale-state failures. Download recovery material before reloading: the download contains the unsaved compatibility draft and the exact legacy/canonical raw values if accessible. It contains no API keys or transient ingestion pastes. Free storage or allow site storage, then reload to retry migration/save from the saved state. For malformed data, retain the download and seek help instead of deleting browser keys. An inaccessible storage origin may make the saved raw values unavailable, but the draft can still be downloaded.

## Verification

Public-boundary tests cover lossless migration, absent/empty values, entry ordering, stable identity, reload, original retention, quota and malformed data, stale/concurrent writes, semantic qualifiers, conservative aliases, contextual invalidation/removal and shared Go/TypeScript parity. Controlled browser ingestion checks cover existing field-decision routing, explicit review/apply, EN/PT at 390/1440px, failed-save retention and real sibling-window storage events. Provider semantics and deployed behavior are separate evidence boundaries.

## Canonical manual editing (#37)

Existing Profile forms now call `editProfile` through the store's `editCanonicalProfile` queue. Commands name the field owner, fact, entity or typed context explicitly; manual forms never replace a compatibility view. `replaceProfileView` remains only for transitional AI writers pending their migration tickets.

Manual corrections retain the affected fact ID, revise its user authorship and approval, and invalidate old excerpt support. Explicit assertion/intent/certainty/date precision remain available without inferring semantics from narrative or decomposing a migrated block. Employer/role/project/period identity changes invalidate dependent support. Typed corrections preserve unrelated links and inline references; context removal never reassigns evidence. Entry ordering does not alter meaning revisions. Moving into an occupied field is rejected atomically; remove the destination fact explicitly first.

The store preserves a canonical draft separately from the durable document, so queued typing composes correctly and recovery downloads include unsaved qualifiers/context as well as compatibility wording. Canonical revision checks also reject pending ingestion proposals after metadata-only edits. Failed migration retains an editable recovery draft while durable writes remain blocked until deliberate recovery/reload.

## Section rewrite proposals (#38)

`src/lib/sectionReview.ts` builds selected-section snapshots, validates bounded revisioned patches and applies only explicitly accepted proposals. `applyProfileProposal` saves atomically before advancing the store or clearing review. Each nonempty fact retains an individually referenced patch; cross-owner or changed-qualifier consolidation is rejected. Unsupported sources remain unsupported. Edited wording becomes user authored with unknown semantic qualifiers and invalidated excerpt support; unchanged accepted AI wording retains approved excerpts. Correcting a source conservatively invalidates shared excerpt support, preserving the excerpts for inspection. See `docs/release-evidence/2026-10-09-section-profile-proposals.md` for scope, verifier approval and evidence limits.

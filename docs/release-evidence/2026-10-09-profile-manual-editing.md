# Issue #37 — stable manual Profile editing

Baseline: `8869954`, shared by main and milestone 3 branch `codex/evidence-backed-profile` at implementation start. Native blocker #36 was closed. The user confirmed canonical edit/save and visible Profile workflow test seams and this review baseline.

## Implementation

Existing contact, goals, skills/competencies/tools, experience, projects, education, certifications, languages, employment, compensation and additional-information forms now issue explicit canonical field/entity commands. AI writers retain the temporary compatibility boundary for their separately tracked migrations. Manual edits do not diff large text or guess provenance ownership.

Fact details show original whole blocks or statements, owner/period information, explicit context references, origin and source-support state. Users can correct assertion, intent, uncertainty, temporal wording/precision and typed relationships, remove context/facts, move facts into unoccupied compatible fields, and reorder entries deliberately. Empty fields are recreated by the existing forms. A destination containing an empty fact must have that fact explicitly removed before receiving another fact; this prevents accidental replacement or merging.

Manual meaning corrections retain stable fact IDs and create revisions with approved user-authored origin. They invalidate approved-excerpt support rather than reattribute it. Unrelated facts keep their revisions; ordering changes do not revise entity meaning. Employer/role/project/period identity corrections and removals invalidate dependent support. Typed changes retain unrelated inline context and links. Original excerpts remain historical recovery material. No source-excerpt support is fabricated for user-authored statements.

All candidates validate and save with the existing origin Web Lock and expected-document comparison. The canonical in-memory draft supports queued targeted commands without losing rapid input. Durable authority advances only after atomic persistence; errors and cross-tab conflicts preserve saved bytes and retain drafts. Recovery downloads now include the canonical draft, including unsaved qualifiers and relationships. Proposal application checks the canonical revision, covering metadata-only changes invisible to compatibility text.

## Verification

- `node tests/profile-document.test.mjs`: 21 public-boundary checks pass. New coverage includes qualifiers and untouched revisions, every section's create/edit/remove/reload, safe moves/order, same-Profile/typed corrections, evidence invalidation, period removal, inline-context retention and explicit removal.
- `node tests/profile-edit-browser.check.mjs`: passes EN/PT at 1440/390px. Exercises every section's manual creation/editing/removal/reload, restored draft wording, visible qualifiers and context correction/removal, evidence invalidation, raw-label prevention, horizontal-fit checks, metadata-only pending-proposal rejection, quota failure with byte-identical authority/recovery snapshot, and a real sibling-tab revision conflict. Final screenshots are in `profile-manual-editing/`; EN desktop and PT 390px fact panels were visually inspected, with readable qualifiers/context and no horizontal clipping.
- `node tests/ingestion-browser.check.mjs`: passes existing field decisions, ingestion, explicit acceptance and recovery/cross-tab/delayed-save cases across EN/PT at 1440/390px.
- `node tests/apply-checklist.check.mjs`: passes controlled Apply workflows in all four locale/width combinations.
- `node tests/cv-generation-browser.check.mjs`: EN/PT generation/replacement/edit/regeneration/discard/reload, 390px and extractable one-page A4 controlled PDF checks pass. The 19-role source/cancellation/overflow case passes. The broader density matrix then fails at its missing `[data-cv-density-status]` element (`textContent` of null), matching the unchanged-baseline failure documented in #36's committed evidence. The whole CV suite is not reported as passing.
- Full Node suite, full Go suite (`GOCACHE=/tmp/careeros-go-cache go test ./...`), TypeScript checking and production build pass. The existing Vite future-native-config `__dirname` warning remains.

These are local public-contract and controlled browser checks with synthetic AI responses. They do not establish real-provider semantic accuracy or deployed behavior. No provider requests, new connector, database, account, full-source archive or graph infrastructure are introduced.

## Code review

The required Standards and Spec agents independently reviewed `8869954...e13821c`. Spec found no actionable gaps. Standards found duplicated field-label resolution that omitted the project-name translation. A shared resolver corrected it. A further public regression exposed loss of unrelated inline context during typed relationship changes; that correction also adds explicit display/removal. Both agents reviewed the correction delta through `fd9707c` and reported zero remaining findings. Final additional changes are evidence capture/documentation only.

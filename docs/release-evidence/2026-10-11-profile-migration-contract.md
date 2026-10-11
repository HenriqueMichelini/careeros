# Issue #51 — Profile migration contract phase

Review baseline: `16cef54a44fccc70bca9c91076b1809015864c0b` (main). Milestone 3, Evidence-backed Profile; branch `codex/evidence-backed-profile`, PR base `main`. Native dependencies #39, #43 and #48 were closed before implementation. Test seams and the review baseline were confirmed by the user.

## Active writer and workflow audit

| Entry path | Contract after cleanup |
| --- | --- |
| Startup (`profileStorage.openProfile`) | Import the original legacy value directly into validated v2 once under the origin Web Lock; retain original recovery bytes. Existing v2 wins; malformed v2 cannot silently fall back. |
| Manual Profile forms and Fact Details (`RepositoryPage`, `FactDetails`) | Explicit `editCanonicalProfile` commands through `editProfile`; same-Profile typed ownership and revisions validated, corrections/deletions invalidate support. |
| Section AI Review | `sectionReviewRequest` scopes canonical facts; `applyProfileProposal` validates revision-bound proposals and persists only explicit acceptance. |
| Ingestion, continuation and clarification | Versioned snapshot passed to `ingestProfile`; `applyIngestionDocument` applies approved destinations through canonical edit commands and attaches accepted excerpts. Complete pastes and pending proposals stay transient. |
| Store (`store.tsx`) | Canonical optimistic typing draft plus last durable canonical revision. Presentation view is memoized and derived; no editable legacy state, `SET_REPO`, `saveRepository` or `replaceProfileView`. `saveProfile` is the sole post-import durable writer. |
| General CV | `CvPage` uses `stableCvFacts`; stable references, qualifiers and evidence feed generation. Saved v1/v2 curated CVs retain their independent snapshots/readers and export behavior. |
| Job Posting / requirement analysis | Home passes canonical `profileDocument` to `findProfileGaps`; scoped `qualificationProjection` travels with derived compatibility fields. Source-backed Job Posting context remains transient. |
| Application Draft | Home passes canonical Profile and selected fact IDs to `generateMaterials`; scoped evidence, claim-level resume/artifact review, application-only qualifications and language remain intact. |

`previewIngestion` is a pure, transient presentation calculator for existing field/bounds/identity rules, with no storage access. It does not reconstruct canonical facts. Unrelated facts retain their IDs, revisions, order, qualifiers and evidence. Batch commands advance the saved document revision once. Only the startup importer and failed-migration recovery display call `migrateProfile`; there is no legacy-key writer.

The fixed Go AI consumers already use provider-enforced strict schemas and explicit refusal/truncation classification: section Review, ingestion stages, CV generation, qualification matching and Application Draft generation/support review. Cleanup retains those contracts, explicit unknowns, complete-original field validation, accepted-excerpts-only storage, stateless backend and existing data scopes. Backward-compatible request/read paths remain deliberate compatibility boundaries.

## Verification

All final checks passed:

- `node tests/profile-document.test.mjs`: 45 cases. Lossless direct migration from old Profiles (including unknown extension values), shared TS/Go contract fixtures, observed/canonical normalization, typed ownership/revisions, manual correction/deletion and shared support invalidation, explicit proposal/ingestion acceptance, persistence failure and concurrent stale writes. Added regression first failed because ingestion changed an unrelated fact's order; canonical command application fixes it.
- `node tests/ingestion.test.mjs`: 25 cases, including transient presentation previews and identity/completeness bounds.
- `node --test tests/*.test.mjs`: all 10 frontend test files pass on final source.
- `GOCACHE=/tmp/careeros-go-cache go test ./...`: all Go packages pass.
- `node_modules/.bin/tsc --noEmit`, `node_modules/.bin/vite build`, `git diff --check`: pass.
- `node tests/ingestion-browser.check.mjs`: full controlled suite passes EN/PT at 390/1440px, directly seeding old `careeros_repo` without visiting earlier migration versions. Includes explicit approval/apply, continuation, clarification, migration failure, quota/save failures, held Web Locks and actual sibling-window stale conflicts. Original recovery bytes, saved canonical bytes and pending input/review survive failure. Isolated `PROFILE_RECOVERY_ONLY=1` also passes all four combinations.
- `node tests/cv-generation-browser.check.mjs`: complete controlled suite passes, including old saved v1 CV rendering, canonical v2 evidence, source corrections, reload, copy/export parity, controlled extractable A4 PDFs, empty guidance and overflow detection. Detailed heavy fixtures correctly report overflow instead of forcing false one-page success.
- `node tests/apply-checklist.check.mjs`: passes EN/PT at 390/1440px, preserving canonical projections, application-only qualifications and independent current draft state.

Initial browser attempts hit missing controls; the complete ingestion rerun, isolated recovery suite and final CV/Apply reruns passed with source held stable. An isolated unchanged-main CV run also passed. No test expectations were weakened. Browser logs/screenshots and controlled PDFs were produced under `/tmp/careeros-*`; the Portuguese mobile recovery capture was inspected. Generated build/cache files are excluded from the commits.

These are local contract and controlled-browser results. No live provider call, credential use or deployed behavior is claimed.

## Standards

No documented violations. Review identified stale transitional documentation and a duplicated entity-kind list; both were corrected and the standards reviewer confirmed zero remaining findings.

## Spec

No actionable findings against issue #51. The independent review confirmed the adapters are retired, canonical writers preserve unrelated facts, and legacy import/recovery remains available. Browser verification listed above resolves the review's pending release-evidence questions.

Standards: 0 remaining findings. Spec: 0 findings.

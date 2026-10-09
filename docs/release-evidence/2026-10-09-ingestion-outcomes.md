# Ingestion outcomes — issue #40

Implemented on milestone branch `codex/evidence-backed-profile`, starting at `61c3714` (merged #39). Implementation and review fixes: `906d5d6`, `792a697`. The user approved production HTTP-handler controlled transports, browser validation and explicit-apply boundaries for test-first verification, and confirmed `61c3714` as the review baseline.

## Acceptance evidence

| Requirement | Evidence |
| --- | --- |
| Explicit outcome for every extracted claim; no-operation omissions | `TestHandlerExplainsOmittedClaimWithoutAssumingDuplicate`; server reconciliation supplies unresolved fallback; browser validates one outcome per claim and exact returned operation indexes. |
| Stable existing fact IDs/revisions and conservative exact aliases | `TestExactRepeatPointsToStableFactAndRetainsProfile`; canonical browser tests reject missing, stale and foreign references before apply. |
| Related technologies remain distinct | `TestHandlerRejectsRelatedTechnologiesAndQualifiedDuplicateAssertions` covers Spring Framework/Boot, AWS/S3 and unverified JS abbreviation. |
| Employer, role and period boundaries | `TestHandlerKeepsSimilarlyNamedEmployerSeparate`; `TestHandlerRecognizesScopedRepeatWithAllIdentityEvidence`; earlier #39 distinct-stint and cross-role acceptance regressions remain passing. |
| All disposition categories, reasons, relevant statements and evidence | `TestHandlerPreservesCompetingStatementsAndInvalidOutcomeReferences`; browser renders all nine categories with English/Portuguese explanations, source-specific comparison details and keyboard-accessible stable fact inspection. |
| Invalid/skipped claims remain identifiable | `TestSkippedClaimRetainsResolvedOriginalExcerpt`; browser checks unvalidated returned wording; missing sources use explicit fallback, oversized wording has shortening disclosure. |
| Honest capacity and discovery coverage | `TestHandlerReportsInvalidClaimsAndPossibleCapacityExhaustion`; every result explicitly disclaims semantic discovery completeness and saturation at 30 claims reports possible exhaustion. |
| Repeated assertions may accept evidence without wording changes | `TestRepeatedAcceptedSupportDoesNotPretendToBeNewEvidence`; canonical apply test verifies accepted new support, old excerpts and active links survive reload, while rejection preserves evidence. |
| Preserve negation, aspiration, uncertainty and time | HTTP and browser-boundary tests recognize identical explicit qualifiers and reject changed qualifiers; related-technology/qualified-source negative controls remain unresolved. |
| Profile unchanged before explicit apply; accepted-excerpts-only storage | Controlled browser checks compare saved Profile before review/inspection/apply, exercise failed-save input retention and real sibling-window conflicts. Canonical regression tests verify only approved excerpts persist and complete input is not archived. |

## Checks and review

- `node_modules/.bin/tsc --noEmit`: passed.
- `npm test`: all seven frontend test files passed; focused ingestion and canonical-document runs also passed (22 and 42 assertions respectively).
- `GOCACHE=/tmp/careeros-go-cache go test ./...`: passed across all packages.
- `node_modules/.bin/vite build`: passed. Existing Vite configuration warning about `__dirname` remains unrelated to this change. Generated `dist` artifacts are excluded from the source commit.
- `node tests/ingestion-browser.check.mjs`: passed in English/Portuguese at 390/1440px, empty/populated Profile, field-decision routing, all outcome types, native keyboard inspection, failed-save recovery and cross-tab conflicts. Representative screenshots were visually inspected; final browser artifacts: `/tmp/careeros-ingestion-browser-9P0JAM` (ephemeral local evidence).
- Code review used independent Standards and Spec agents against `61c3714`. Initial review found one nonblocking duplicated identity-selection heuristic and two disclosure gaps. The shared helper, identifiable skipped wording and localized explanation categories address them. Follow-up review of `61c3714...792a697`: Standards 0 remaining material findings; Spec 0 remaining material findings.

## Evidence boundary

Provider responses were controlled external-transport/browser fixtures. No paid provider request, production deployment check or claim of live semantic extraction quality is included. The ledger accounts for returned claims; it cannot prove every fact in the submitted text was discovered. Continuation and focused clarification remain #41/#42. Contradiction/correction candidates retain competing statements for deliberate user resolution and do not automatically change saved facts or support.

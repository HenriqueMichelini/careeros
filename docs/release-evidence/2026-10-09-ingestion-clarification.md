# Focused ingestion clarification — issue #42

Milestone: Evidence-backed Profile (#3). Implementation branch: `codex/evidence-backed-profile`, starting at `b085224`. Source commits: `a2159f7`, `e0a9d2c`. The user confirmed the production ingestion HTTP handler with controlled transports, proposal/evidence functions and browser UI as test seams, and `b085224` as the review baseline.

## Acceptance evidence

- Material questions: extraction guidance restricts questions to meaningful identity/ownership/support/conflict ambiguity, preserves unknown optional detail and approximate durations. Questions appear with their claim excerpt and existing stable-fact/related-claim outcome details.
- Bounded revisions: `TestClarificationUsesBoundedEvidenceAndPreservesAnswerOrigin` checks pronouns, approximate durations, unknown answers and Portuguese; unrelated paste content does not enter semantic extraction/reconciliation. A complete original-submission policy decision still precedes separately validated answer/context processing.
- Safety: `TestClarificationBlocksOriginalAttackBeforeAnswerProcessing` and `TestClarificationAnswerSafetyStopsBeforeExtraction` prevent downstream provider work after original attack or answer attack/rephrasing decisions. The browser requires changed unsafe answer text before retry.
- Conflict resolution: `TestClarificationExplicitContactAndProficiencyCorrectionsRemainProposals` verifies email/proficiency corrections are proposed through the handler without mutating Profile.
- Repetition: `TestRepeatedClarificationAnswerDoesNotDuplicateEvidence` and browser repeated-unknown checks preserve one answer excerpt while keeping uncertainty unresolved.
- Unrelated pending work: `clarification replaces only its claim, resets approval, and preserves unrelated edits` verifies index remapping and preservation; browser checks edited/approved unrelated proposals survive unknown/repeated/revised answers.
- Evidence: canonical apply/reload tests verify `professional_information` and `clarification_answer` origins, accepted excerpts only, unchanged data before approval, stale rejection, and both origins even for identical wording. The production Go origin-first JSON order has a regression at the `ingestProfile` seam.
- Browser: English and Portuguese at 1440px/390px exercise question/answer forms, unknown/repeated answers, answer rephrasing, cancellation with delayed responses, unapproved revisions and unchanged saved data. Existing source-edit/cross-tab staleness, save failure, continuation and original field-policy regressions also pass. Representative desktop/mobile screenshots were visually inspected.

## Checks and independent review

- `node_modules/.bin/tsc --noEmit`: passed.
- `npm test`: 101 tests passed after the wire-order correction.
- `GOCACHE=/tmp/careeros-go-cache go test ./...`: passed across all packages.
- `node_modules/.bin/vite build`: passed; bundle size advisory above 500 kB. Generated build/cache files are excluded.
- `node tests/ingestion-browser.check.mjs`: complete initial run passed, artifacts `/tmp/careeros-ingestion-browser-shWOHJ`; final run with production JSON ordering also passed fully, artifacts `/tmp/careeros-ingestion-browser-GdB0Bm`.
- Independent Standards/Spec reviews of `b085224...a2159f7` and incremental `a2159f7...e0a9d2c`: no material or blocking findings. Standards noted one optional maintenance duplication in original/answer decision response writing; Spec found zero actionable gaps.

## Evidence boundary

All provider responses were controlled transports or browser fixtures. This verifies contracts, routing, review/save boundaries and evidence preservation; it does not measure live semantic quality. No paid provider calls or deployed verification were performed. Full pastes and pending answers remain transient; browser-local storage retains only explicitly accepted facts and necessary excerpts. No database, accounts or full source archive is introduced.

# Apply-to-Profile enrichment verification

Scope: milestone #6, issues #116–#120. Review baseline
`584d5dc968ff8423ff40f46916a8c34dc9676372`. See the
[approved design and recipient review](../../apply-profile-enrichment.md).
The user approved controlled fixtures, EN/PT browser seams and implementation
before #52 closes. No live providers or deployment requests are authorized or
used here. #52 retains its existing release gates.

## Evidence map

| Requirement | Controlled evidence |
| --- | --- |
| Complete answer classified before extraction; strict origin/target/revision schema | `backend/profile-ingestion/apply_answer_test.go`: handler acceptance, ten invalid handoffs rejected before providers, malicious whole-answer rejection |
| Fresh project enrichment, chosen owner, exact repeat, ambiguous ownership, team/approximate time, negation, wrong owner, related technologies | Sixteen handler matrix cases: eight scenarios in EN and PT, plus explicit unknown-project and same-company conflicting-role regressions. Source excerpts and meaning qualifiers retained; ambiguous/invalid operations withheld |
| Focused clarification retains both origins and chosen owner | Production handler clarification case with two independent classifications, one revised claim and one scoped proposal |
| Accepted excerpts and unrelated data survive reload; malformed/stale targets rejected | `tests/profile-document.test.mjs`: canonical Apply save/reload and strict handoff validation, plus existing canonical fact/evidence tests |
| Uncertain commit feedback | Canonical storage test: a lock rejects after atomic commit; durable read confirms success, and repeating the old snapshot is rejected |
| No implicit save/call, unchecked factual answers, explicit review and selections | `tests/apply-enrichment-browser.scenarios.mjs`, invoked by `tests/apply-checklist.check.mjs`: real Apply workflow with controlled responses |
| Cancellation/late response, failure, deliberate retry, repeated Save | Browser matrix: cancelled response cannot restore a review; failed review does not retry; one revision committed after repeated clicks |
| Declined save, quota error, cross-tab stale state, original successful Draft | Browser failure modes verify unchanged authority or unrelated later edit, retained recovery answer through an in-memory durable refresh and explicit re-review/save, preservation of later unrelated edits, no false success, and original artifact content with truthful stale-source warning |
| Later reuse is independent of persistence | Fresh browser analysis carries accepted project facts/excerpts; a separately controlled supported match removes only the basic AWS question. This does not measure provider recall or semantic quality |
| Inspect/correct/remove and recovery | Existing canonical Profile editing/storage coverage; accepted origins/excerpts remain inspectable. No transaction undo or rollback over later edits |

Browser scenarios use EN and PT at 1440px and 390px, including Portuguese answers
to an English posting. Professional answers, employer requirements and selected
owner context remain separate. Tests use synthetic Profiles and transport
responses. Existing ingestion, Profile, qualification, Draft/review and reuse
coverage is retained.

## Interpretation limits

The handler matrix has 16 labeled scenario opportunities; the invalid-handoff
matrix has 10 rejected-input opportunities. These counts describe controlled
contracts, not independently measured model accuracy. Browser modes separately
cover successful save/reuse, decline, quota, cross-tab stale state, and successful
Draft preservation. Tests assert literal expected facts, source/origin pairs,
revisions and failure states; fixture responses do not adjudicate live semantics.

Duplicate, unsupported-subclaim and owner correctness rates on real provider
outputs are **not measured**. Real candidate recall, semantic matching, re-ask
rate, tokens, charges and end-to-end latency are **not measured**. A successful
save is not evidence that a later matcher finds or supports every claim.
Controlled browser matching is distinct from production-handler retrieval tests
and deployment behavior. Existing budget/incomplete-coverage disclosures remain.
No published route or actual PDF evidence is claimed for this feature.

## Repository checks

Final local checks passed on the reviewed runtime source `a68bd76`:

- `npx tsc --noEmit`.
- `npm test`: 138 passed, zero failed.
- `GOCACHE=/tmp/careeros-go-cache /usr/local/go/bin/go test ./...`: all packages passed.
- `npm run build`: passed; the existing bundle-size advisory remains.
- `node tests/apply-checklist.check.mjs`: full Apply regressions plus five enrichment
  modes at EN/PT × 1440/390px, including complete in-memory recovery/re-review/save.
- Targeted canonical Profile file: 49 passed. The new handler cohort contains 31
  controlled case opportunities, including rejected handoffs and ownership regressions.
- `git diff --check`: passed. Build/cache outputs were excluded and restored.

Independent Standards and Spec agents re-reviewed `584d5dc...a68bd76` after the
fixes: zero remaining actionable findings on each axis. The Spec reviewer also
independently ran the focused handler regressions. Final EN/PT desktop/mobile
screenshots were inspected; pending and saved states fit 390px without overflow.

Independent review found and resolved two gaps: selected ownership could overrule
explicit scope, and page reload recovery could discard transient answers.
Regression coverage now rejects conflicting/unknown ownership at both handler
and canonical apply boundaries, and completes quota/cross-tab recovery through
in-memory Profile refresh, renewed review/selection and successful persistence.
A nonblocking duplicated-origin fallback was centralized. The saved-answer
control now says Hide saved answer rather than implying a declined save.

# Evidence-based MVP integration gate (#52)

Review baseline: `4e48cb8a9106a78f58a5fde7c412475650dbb794`.
Milestone 4 branch `codex/evidence-backed-cvs-and-applications`, base `main`.
All native prerequisites (#33, #50, #51, #62, #65) were closed before work began.
This is a verification deliverable, not certification of arbitrary provider output.
Final deployment results and independent review are recorded below when available.

## Scope and evidence rules

New held-out synthetic EN/PT cases connect the public migration/storage, manual
editing, approved-ingestion, CV, qualification projection, resume and artifact
acceptance seams in `tests/evidence-mvp.test.mjs`. They were authored after the
fixed workflow configuration was selected and never used to tune it. These
cases do not run a model or substitute for a single live browser journey.
Existing controlled browser checks exercise the actual rendered UI separately.
No production prompt, model, safety gate, normalizer or persistence contract was
changed for this verification.

The frozen #34 held-out corpus was also replayed through its production-handler
adapter: 26 HTTP 200 rows, 18 without scorer failures, eight with retained
failures. The two capacity rows intentionally measure only the initial 30 of
35 claims (completeness 0.857); current continuation needs explicit later clicks.
Six deliberately adversarial legacy draft rows retain forbidden/unmatched
management, AWS-as-personal-skill and transferred-metric outputs. They probe
legacy HTTP output, not acceptance through current claim-level review. Neither
HTTP 200 nor this replay establishes factual acceptance. See the original scores
in [heldout.controlled.json](issue-52/heldout.controlled.json).

## Acceptance matrix

| Issue requirement | Current evidence | Boundary / unmet coverage |
| --- | --- | --- |
| Bilingual migration, editing, ingestion, continuation, clarification, CV, requirements and all draft artifacts | New EN/PT integrated public-seam cases; `profile-edit`, `section-review`, `ingestion`, `cv-generation`, `application-reuse`, `resume-review` and `apply-checklist` browser checks | Separate controlled UI flows; no claim of one uninterrupted provider-backed browser journey |
| Negation, aspirations, approximate dates, ambiguous owners, conflict, repeats/large input, short/unknown jobs, revisions | New two-employer/period/project migration cases; frozen held-out handler replay; current Profile/ingestion/context/support tests and browser continuation | Legacy blocks remain uninterpreted; similar words cannot establish ownership or semantic truth |
| Cancel/reject/block/stale and temporary answers preserve saved Profile; accepted excerpts only | New quota/retry/reload/stale integration cases; ingestion and Apply browser storage assertions | Accepted excerpts and original legacy recovery persist locally; full paste remains session-only |
| Storage, timeout/outage/rate limit, refusal/malformed/truncated, cancellation and deliberate retry | Full Go error-path suite, CV/ingestion Node suite, browser failure/cancel/storage/retry flows | Controlled failures, not a real provider outage experiment; no hidden retry/fallback |
| v1/v2 CV persistence, display/copy/PDF, fit and keyboard EN/PT desktop/390px | CV generation and Apply/resume browser scripts, selectable-text A4 PDFs and overflow assertions | Detailed heavy CV is four pages and reports overflow; no universal one-page or screen-reader certification |
| Precision/completeness, unsupported claims, usage/cost/runtime versus baseline | Frozen baseline replay; retained #50 current/candidate live comparison and #65 sparse confirmation | Controlled precision is not provider quality. Complete accepted-result prices remain null; no observed billing or actual user-acceptance rate |
| Changed routes in Netlify preview/published app; payload preflight, origin/revision/time/log privacy | Authorized live verification pending below | Local/mock evidence cannot establish deployment behavior |
| Report unmet checks without reopening historical issues | This matrix preserves dated baseline and release reports | Historical closure is not universal release acceptance |
| Normalization/version, typed links, role/project ownership, duplicate/support/conflict/correction, invalidation | `profile-document.test.mjs`, ingestion handler tests, new source-correction integration; manual-edit browser checks | Exact aliases only; semantic equivalence remains separate |
| Malformed/version recovery, durable save, cross-tab, linked selection, idempotent retry | Profile storage/document and ingestion tests; new failed-save then explicit retry and rejected stale retry; ingestion browser sibling-tab tests | No-lock environments fail safely; retry Apply is local and makes no provider request |
| #65 retained failures and EN/PT regressions | [CV reliability evidence](2026-10-11-cv-reliability.md), full handler/client/browser checks | Original EN failure cause unknown; PT live exact-summary label miss retained; #34 remains closed measurement |
| Client-held context/source/version checks, whole-original gate, explicit incomplete outcomes | Job context/reuse gate tests; ingestion continuation tests; application context retrieval tests | Only pasted fields are Jev-protected; section rewrite and CV use saved structured sources. Hashes/source coverage are not authentication or completeness proof |
| Reconcile architecture/status and retain sparse/PT omissions | Current summaries in `backend-requirements.md` and `input-validation.md`; retained #50 decision and PT answer-omission regressions | Original dated three-workflow requirements and failed audit are preserved |

## Current local verification

- Full Go suite passed with `GOCACHE=/tmp/careeros-go-cache go test ./...`.
- Full Node suite: 125 tests passed. The first sandboxed run could not execute
  the existing Go subprocess integration; the authorized run passed unchanged.
- TypeScript and production build passed. Existing Vite chunk-size advisory remains.
- Ingestion: EN/PT at 1440/390px, including keyboard, continuation, clarification,
  rejection, cancellation, durable-save recovery and sibling-tab conflicts, passed.
- Apply reuse: EN desktop/PT mobile, deliberate reuse/cancel/source invalidation,
  passed. CV generation: EN/PT, three densities, sparse/heavy, v1/v2 reload,
  source revisions, failed storage and actual A4 overflow checks, passed.
- Section rewrite: EN/PT at 1440/390px, review/edit/reject/apply/quota/cross-tab,
  cancellation and stale proposals, passed.
- Apply artifact review/copy/PDF: EN/PT at 1440/390px passed.
- Resume review: updated the stale pre-#49 test sequence to respect session reuse
  and explicit confirmation; the original browser scenario now passes EN/PT at
  1440/390px. It still asserts no automatic retry and preservation of accepted
  content on failure/replacement cancellation.
- Manual-edit mock: updated explicit ingestion accounting and source-bound
  continuation receipts required by the current canonical client. Final rerun pending.

## Retained live quality and cost evidence

The [#50 decision](../evaluations/workflow-settings/decision.md) retains fixed
settings because neither candidate demonstrated full-workflow improvement.
Its 42 logical live results, 51 HTTP attempts and 132 provider dispatches include
language drift, unsupported ownership and continuation omissions; their per-field
agent-assisted reviews are preserved, not human semantic adjudication.
Validation pricing and real user acceptance are unavailable, so total and
accepted-result cost remain null. The two authorized #65 sparse CV attempts
succeeded (EN 2140.841 ms, PT 1515.829 ms), while the PT exact-label miss and
original unexplained EN rejection remain retained. No original corpus, scorer,
provider output or dated measurement was rewritten to improve a score.

## Deployment verification

The user explicitly authorized minimized synthetic OpenAI/TypeSafe requests and
publication of the already completed `4e48cb8` build. Netlify deploy
`6acbb7959a691d0009a0176d` was published and remains locked; it contains five
Functions. This replaces the stale public `31371b4` build for the release test.
The new PR preview and exact-head checks remain to be recorded.

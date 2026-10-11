# Evidence-based MVP integration gate (#52)

Review baseline: `4e48cb8a9106a78f58a5fde7c412475650dbb794`.
Milestone 4 branch `codex/evidence-backed-cvs-and-applications`, base `main`.
All native prerequisites (#33, #50, #51, #62, #65) were closed before work began.
This is a verification deliverable, not certification of arbitrary provider output.
Deployment checks found failures; this report does not declare the MVP release gate complete.

## Scope and evidence rules

New held-out synthetic EN/PT cases connect the public migration/storage, manual
editing, real production-handler ingestion/continuation/clarification, CV,
qualification analysis, resume and artifact acceptance seams in `tests/evidence-mvp.test.mjs`. They were authored after the
fixed workflow configuration was selected and never used to tune it. These
cases do not run a model or substitute for a single live browser journey.
Existing controlled browser checks exercise the actual rendered UI separately.
No production prompt, model, safety gate, normalizer or persistence contract was
changed. Verification exposed an empty-contact comparison defect: canonical
empty fields disagreed with omitted optional fields in the ingestion request.
The fix compares actual contact strings; all five nonempty mismatches still
fail before any provider call. The public-handler regression was red before
the fix and green afterward.

The frozen #34 held-out corpus was also replayed through its production-handler
adapter: 26 HTTP 200 rows, 18 without scorer failures, eight with retained
failures. The two capacity rows intentionally measure only the initial 30 of
35 claims (completeness 0.857); current continuation needs explicit later clicks.
Six deliberately adversarial legacy draft rows retain forbidden/unmatched
management, AWS-as-personal-skill and transferred-metric outputs. They probe
legacy HTTP output. The new connected cases also replay those exact six bodies
through current production support handling and public acceptance, with an
independent controlled judgment marking the detected unsupported claim. All six
remain unaccepted. This verifies enforcement after detection, not model accuracy. Neither
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
| Changed routes in Netlify preview/published app; payload preflight, origin/revision/time/log privacy | Published five-route live requests, observed content-free production logs; preview section/CV success, first ingestion 400 and incomplete requirement analysis | Local/mock evidence cannot establish deployment behavior |
| Report unmet checks without reopening historical issues | This matrix preserves dated baseline and release reports | Historical closure is not universal release acceptance |
| Normalization/version, typed links, role/project ownership, duplicate/support/conflict/correction, invalidation | `profile-document.test.mjs`, ingestion handler tests, new source-correction integration; manual-edit browser checks | Exact aliases only; semantic equivalence remains separate |
| Malformed/version recovery, durable save, cross-tab, linked selection, idempotent retry | Profile storage/document and ingestion tests; new failed-save then explicit retry and rejected stale retry; ingestion browser sibling-tab tests | No-lock environments fail safely; retry Apply is local and makes no provider request |
| #65 retained failures and EN/PT regressions | [CV reliability evidence](2026-10-11-cv-reliability.md), full handler/client/browser checks | Original EN failure cause unknown; PT live exact-summary label miss retained; #34 remains closed measurement |
| Client-held context/source/version checks, whole-original gate, explicit incomplete outcomes | Job context/reuse gate tests; ingestion continuation tests; application context retrieval tests | Only pasted fields are Jev-protected; section rewrite and CV use saved structured sources. Hashes/source coverage are not authentication or completeness proof |
| Reconcile architecture/status and retain sparse/PT omissions | Current summaries in `backend-requirements.md` and `input-validation.md`; retained #50 decision and PT answer-omission regressions | Original dated three-workflow requirements and failed audit are preserved |

## Current local verification

- Full Go suite passed with `GOCACHE=/tmp/careeros-go-cache go test ./...`.
- Full Node suite after the fix: 126 tests passed. The first sandboxed run could not execute
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
  continuation receipts required by the current canonical client, and immediate
  stale-proposal invalidation. All four EN/PT desktop/mobile cases passed.

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
The PR #93 preview at `58e7d00` is deploy `6acbba88e46b80000846119a`,
origin `https://deploy-preview-93--beamish-bavarois-333d03.netlify.app`.
Its team access protection remains enabled. Exact-head deployment of the later
empty-Profile fix remains unverified.

Five published HTTPS requests used the authorized minimized synthetic payloads,
reviewed locally before dispatch; credentials stayed process-local. No automatic
retries occurred. The published checks are recorded in
[published.live.json](issue-52/published.live.json). All returned 200/no-store,
but that does not mean semantic success:

- Ingestion proposed Java from `I use Java`, with incomplete discovery explicitly
  reported; CV selected the supported 20% latency fact; section rewrite kept Java.
- `Java obrigatório.` requested more job information, so qualification analysis
  did not complete on the published fixture.
- The Portuguese draft produced supported Java resume content but invented five
  extra Q&A questions. Artifact review retained unresolved claims, invalid job
  citations and an incorrect language concern. This output was not accepted.

The authorized preview browser session used only synthetic Java. First ingestion
into the fresh empty canonical Profile failed with 400 before provider calls;
this caused the local regression and fix above. Manual Java then passed section
review and generated/accepted a supported Java CV. Requirement analysis for the
full preflighted Portuguese Java/AWS posting returned an incomplete-result
message, so no preview draft request was made. Four preview workflow attempts,
five published requests, no retries. Browser OpenAI and TypeSafe test credentials
were cleared through UI controls and verified absent after reload.

Observed production Function logs contain timing/status, version, model and token
metadata, without input, generated prose, keys or contact data in these five
observations. This is bounded operational-log evidence, not proof about provider
retention or every log. Client HTTP duration, handler duration and individual
provider duration remain distinct. Accepted-result cost, billing and live browser
request duration remain unavailable. Existing #50 measurements provide the
comparison baseline; these tiny single attempts cannot establish an improvement.

Netlify reports operational-credit mode: published sites remain live while
production deploys and Agent Runners are paused. No plan or billing change was
made. Remaining release checks are corrected-head preview/published ingestion,
completed live qualification/draft browser flow, and broader provider quality.
Issue #52 must remain open while these cases are unmet.

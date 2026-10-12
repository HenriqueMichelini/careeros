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
The initial verification left production prompts, models, safety gates,
normalization and persistence contracts unchanged. The later citation-catalog
correction is recorded separately below. Verification exposed an empty-contact comparison defect: canonical
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
Its team access protection remains enabled. At that initial check the later empty-Profile fix was not deployed; see the corrected-build confirmation below.

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
made. At that point the remaining checks included corrected-head ingestion and
completed live qualification/draft browser flow, plus broader provider quality.
Issue #52 must remain open while these cases are unmet.


## Corrected-build confirmation and final status

The user separately approved publication of `e8042ee` and six additional
synthetic workflow attempts. Deploy `6acbbe6c6e768b0008a9f948` is published and
locked. Its immutable PR #93 preview permalink was used for fresh browser
storage; Netlify exposes the published preview build's operational logs under
Deploy Preview #93. All six authorized attempts were used, without retries.

The complete credential-free requests and exact provider payload preflight
were reviewed locally before dispatch. Both ingestion checks used `I use Java`
against an empty canonical Profile; job checks used only a synthetic Portuguese
Java developer role, Java responsibility/qualification and one Java application
question. Runtime browser reference IDs are generated separately; live provider
generation/support payloads depend on their actual preceding results, so the
controlled preflight is not a claim of byte-identical dynamic provider outputs.

| Route | Published HTTPS result | Fresh browser result |
| --- | --- | --- |
| First ingestion | 200; proposed supported Java | 200; Java explicitly approved and durably saved |
| Qualification | 502 `invalid_output`; no downstream acceptance | 200; source-backed job review and Java support; company/seniority/location unknown |
| Application Draft | 200; supported Java resume, unresolved artifact claims/citations and redundant Q&A | 200 at handler; client rejected incomplete result; no artifacts accepted |

See [published corrected checks](issue-52/published.corrected.live.json) and
[preview corrected checks](issue-52/preview.corrected.live.json) for origins,
revision, separated timing, provider usage and bounded content-free operational
log observations. Published client HTTP times were 5344/3526/10194 ms;
handler times were 4389.785/2703.842/9065.974 ms. Browser handler times were
3485.989/4008.735/11724.121 ms, not measured browser durations. Keys were
cleared through app controls after checks and verified absent after reload.

The reproducible empty-Profile integration defect is fixed and verified live.
Qualification reliability, complete accepted live drafts and actual accepted-result
cost remain unmet/unavailable. These sparse attempts are not a quality improvement
claim. Keep #52 open; do not declare milestone 4 or the MVP release gate complete.

Independent immutable review of `4e48cb8...e8042ee`: Standards found zero hard
violations or actionable smells; Spec found no remaining implementation defects.
Both original test-coverage findings were fixed. Spec explicitly retains the
unmet release checks above. No prompt/model/schema setting was tuned to these
held-out results.

## Supplemental diagnosis after PR #93

The user approved up to six further minimized synthetic local production-handler
attempts with private response capture. The first qualification attempt passed;
a deliberate diagnostic repetition failed because the provider shortened two
64-character segment IDs. The existing resolver correctly rejected them before
requirement matching. The captured response is retained in
[truncated-job-source.controlled.json](issue-52/truncated-job-source.controlled.json)
and replayed as a rejection regression. No guess repairs source references.

A captured draft exposed another integration defect: the backend marked wrong
UTF-8 job-citation offsets unsupported but returned the malformed citation
objects. The client consequently rejected the whole review, preventing inspection.
The correction retains `invalid_job_citation` and unsupported state while returning
only valid job citation objects. It does not move offsets, invent references or
accept claims. The captured production support response in
[invalid-job-citation.controlled.json](issue-52/invalid-job-citation.controlled.json)
reproduced `ApplicationDraftError: invalid_output` before the fix; afterward the
actual public client opens the review and explicit artifact acceptance still fails.
The Profile remains unchanged. This is contract recovery, not a semantic-quality
or model-accuracy improvement; these captured cases are now regression fixtures.


Four of the six diagnostic workflow attempts were used, with no automatic retry.
A fresh post-fix draft returned 200 and parsed through the actual public client;
unsupported artifact claims remained unaccepted. Metadata and per-stage usage
are retained in [local diagnostic checks](issue-52/local-diagnosis.live.json).
This supplemental correction still requires deployed verification. The release
gate remains open for provider source-ID reliability, faithful complete drafts
and unavailable accepted-result cost.


## Supplemental deployed confirmation

The user approved publication of `1c46de8` (deploy
`6acbc1dbcfef520008b975b5`) and two further synthetic draft checks. This build
is published and locked. Both the published origin and immutable PR #94 preview
origin returned 200/no-store and parsed through the actual public client.
Invalid job citations remained unsupported concerns, valid excerpts remained
inspectable, and neither output could be accepted. Both authorized attempts
were used without retry. These were direct HTTPS responses replayed through the
public client, not a live browser interaction. The previously observed actual
browser failure is preserved above rather than relabeled as passing.

[Final deployed draft checks](issue-52/citation-fix.deployed.live.json) record
origins/revision, request hashes/IDs, measured HTTP duration, separated handler
and provider timing/token usage, bounded content-free operational logs and
client parse/acceptance outcomes. Thus both identified integration corrections
have deployed evidence. Provider source-ID reliability, faithful complete live
application drafts and actual accepted-result cost remain unresolved; #52
stays open and the MVP/milestone release gate is not certified. PRs #93 and #94
are merged into `main`; the milestone branch is synchronized with those merges.


## Remaining authorized local diagnosis

After the credential vault was unlocked, the final two of the six authorized
local diagnostic attempts ran at `0c0a331` with unchanged provider settings.
The credential-free handler requests and controlled outbound envelopes were
preflighted before dispatch; actual later support requests depend on preceding
live outputs. The new sole approved synthetic fact was “Uso Java para desenvolver
APIs.”, against the same Portuguese Java posting and its one explicit question.
There were no retries. All six local diagnostic attempts are now used.

Both handlers returned 200/no-store and both responses parsed through the actual
public clients. Qualification matched the approved Java fact and preserved
unknown company, seniority and location. The generated answer now explained how
Java is used, but the draft added four unrequested questions. Inspection found
that the unchanged generation policy explicitly requests 5–6 application answers;
this observation is distinct from a provider inventing that count independently.
No prompt, model or schema was tuned during these diagnostic checks.

The resume passed public acceptance; artifact acceptance remained blocked by
invalid job citations and unresolved support concerns. The local acceptance
operation did not save or export output, and the Profile remained unchanged.
This distinguishes absence of answer support in the earlier sparse Profile from
the remaining citation and draft-fidelity problems. One successful qualification
attempt cannot resolve the earlier source-ID reliability failure.

[Final local diagnostic metadata](issue-52/final-local-diagnosis.live.json)
retains separated timing/usage and client outcomes. Full synthetic provider
responses stayed private. Inherited fixture scorer labels were inapplicable to
the changed input and are not presented as semantic quality measurements.
These checks add no deployed/browser evidence or accepted-result billing data.
Issue #52 remains open with its full acceptance scope and release gate intact.


## Prepared artifact citation catalog

The remaining live artifact failures included incorrect UTF-8 byte offsets.
The support policy previously required the model to calculate those offsets from
prose. The bounded correction supplies `jobCitations` assembled from the shared
prepared source's occurrence-specific original ranges and exact original text.
The model is instructed to copy the applicable objects, without calculating,
shortening, combining or repairing them. No second normalizer or source mapper
is introduced. The same prepared source used by generation reaches support.
The support diagnostic policy version is now
`artifact-support-openai-v2/prepared-citations-v1/number-guard-v2`.

The public response and provider response schemas are unchanged. Existing
UTF-8 bounds, exact quote, career/job scope, metadata, numeric and semantic
support checks still apply; catalog membership alone is not proof of support.
This change does not repair a returned invalid citation or automatically retry
it. The existing malformed-citation regression still blocks acceptance.

The new public-handler test failed before the change because no prepared
citations were supplied, then passed with exact whole-original coverage for
Portuguese accents, CRLF, repeated text and a protected URL. A controlled
support response copies a supplied excerpt and retains its exact public
citation. All Go tests, all 128 Node tests and TypeScript checks pass.

The catalog repeats source excerpts in the provider envelope, so token use and
runtime may increase. The existing serialized-envelope limit still returns an
unavailable review before the support call when exceeded. No additional source
is sent, no provider/model or safety policy changes, and no saved Profile write
is added. Live quality, latency and cost impact are unverified: all prior
provider-call authorizations are exhausted. The earlier diagnostic cases are
retained as regression evidence and cannot serve as held-out proof of this
prompt change. Issue #52 remains open pending separately authorized deployed
verification and its other unresolved release gates.


The same captured draft also exposed a deterministic false rejection: ordered
answer labels such as `1.` and `2.` were counted as invented career numbers.
A separate public-handler regression reproduced `invented_number` on supported
numbered answers before the fix. Numeric guards now use shared syntax
classification to exclude ordered-list markers while preserving numbers in
prose and literal blocks. The supported numbered-answer regression passes;
unsupported `40%` within a numbered answer and numeric literal content remain
blocked. Accepted wording is unchanged, and semantic/support checks remain
required. Both support-stage diagnostic versions record `number-guard-v2`.
This correctness change also awaits authorized live/deployed verification.

## Deployed prepared-citation and numbered-answer checks

The user subsequently authorized publishing completed build `257e42d7392b000cf1fe044a03cf98727ea5fbeb`
(Netlify deploy `6acc0845801e9100089339ae`) and exactly two synthetic draft
requests, with no retries. The identified build was manually published and
auto-publishing remained locked. Both requests used the preflighted Portuguese
Java posting and sole approved synthetic fact “Uso Java para desenvolver APIs.”
The same diagnostic input was reused; these are not held-out quality cases.

| Check | Published app | Immutable PR #98 preview |
| --- | --- | --- |
| HTTP / client duration | 200 / 11,822 ms | 200 / 10,647 ms |
| Handler duration | 10,849 ms | 9,888 ms |
| Actual public-client parsing / resume acceptance | pass / pass | pass / pass |
| Exact job citations / invalid citation concerns | pass / 0 | pass / 0 |
| Ordered answer markers / invented-number concerns | present / 0 | present / 0 |
| Artifact acceptance | blocked: answer concerns | blocked: answer concerns |
| Profile unchanged in client replay | pass | pass |

The title, summary and cover-letter blocks were supported in both responses.
The remaining blocker was `applicationAnswers`: the published response's
concerns acknowledged the requested answer was present and that the missing
background categories were not requested; the preview concern acknowledged
the answer was present but treated absent temporary `qualificationAnswers`
as an inability to assess sufficiency. Neither response was accepted by the
artifact acceptance function. There were five numbered items in the published
output and three in preview despite only one explicit posting question. These
observations preserve the unresolved answer-review and draft-fidelity gate.
They do not justify overriding an unsupported judgment or automatically retrying.

[Deployed check metadata](issue-52/citation-catalog.deployed.live.json) records
origins, exact revision/deploy, request hashes, separate HTTP/handler/provider
durations, observed token usage and public-client outcomes. Both requests
returned `no-store`. The two observed Netlify invocation entries under Deploy
Preview #98 contained metadata and usage, without inputs, generated prose or
keys; this limited log inspection does not establish provider retention policy.
Direct HTTPS results were replayed offline through the actual public client
and acceptance functions; no live browser acceptance, durable save, export or
PDF check is claimed for these attempts. Provider account billing and
accepted-result cost remain unavailable. No general quality or latency
improvement is established by two reused cases. Both approved attempts are
used; issue #52 remains open with the full acceptance scope unchanged.

## Explicit answer-review input correction

Following the two deployed failures, local inspection found an ambiguous
support-request boundary: the policy asked the reviewer to assess
`applicationAnswers`, but only the generic claim blocks contained that generated
text, alongside a separate optional `qualificationAnswers` source field.
The request now supplies the exact generated `applicationAnswers` explicitly
as well as its unchanged claim block. The policy distinguishes generated output
from optional temporary candidate context and requires answer concerns to
identify an actual missing or incorrect employer-requested answer. Successful
answers and unrelated missing background categories are not omission concerns.
Genuine omissions, insufficient answers, contradictions and wrong language
remain concerns; approved career facts and temporary source context remain
subject to the existing support guards. No unsupported judgment is discarded
or changed after the provider responds.

The public-handler transport regression failed with absent explicit generated
answers before the correction, then passed both without temporary answers and
with separate negative temporary context. It verifies exact output/source
separation, not semantic model accuracy. Incomplete judgments still leave the
review unavailable. Existing omission, unsupported metric and citation guards
continue to pass. Full Go, all 128 Node tests and TypeScript checks pass.
Offline preflight captured all four current handler provider envelopes using
recorded preceding responses and the same minimized synthetic input, with no
network calls. Actual live downstream envelopes depend on generated outputs.

The support diagnostic version is now
`artifact-support-openai-v3/prepared-citations-v1/number-guard-v2/answer-input-v1`.
The extra field duplicates existing generated text and can increase token use;
the existing envelope bound remains enforced. Generation's existing 5–6 answer
policy is unchanged. This correction is a testable request-contract improvement,
not a confirmed cause or live resolution of the fallible review failures.
Deployed verification requires fresh publication/provider authorization; all
earlier attempt limits are exhausted. The remaining full #52 gates stay open.

## Deployed explicit-answer checks and career-reference recovery

The user authorized publishing build `0e96c485e913c397e43c1d6d79c1dc304ce9d7c5`
(deploy `6acc0c3d6cdd740009552853`) and two further draft checks without retries.
Publication was verified as locked. Published and immutable preview returned
HTTP 200/no-store in 10,053 ms and 7,544 ms; handler durations were 8,994 ms and
7,041 ms. Both public-client replays rejected the response as `invalid_output`.
Profile snapshots remained unchanged. No output was accepted.

In both outputs the artifact reviewer cited owner `p` instead of fact `skill`
for career assertions. The backend correctly marked those claims unsupported
with `invalid_or_stale_citation`, but forwarded the invalid source objects,
which made the entire review unreadable to the client. Job citations remained
exact. Preview had no additional answer concerns; published output still
reported deficiencies for supplemental generated questions. These two cases
do not prove semantic omission review is fixed. The unchanged generation policy
still produced three/five items for one explicit posting question.

[Explicit-answer deployed evidence](issue-52/answer-input.deployed.live.json)
retains revision/origin, request hashes, separate timings, observed usage,
claim outcomes and client rejection. The two inspected Netlify log entries
contained metadata/usage without input, generated prose or keys. This is not
provider retention or billing verification, live browser acceptance, durable
save, export or PDF evidence. Both approved attempts are used.

A subsequent connected production-handler/public-client regression reproduced
the rejection for an owner ID, stale revision, foreign Profile, unknown fact and
duplicate reference. The correction keeps the raw-reference validation and its
unsupported concern, but returns only distinct selected current career fact
references. It neither repairs a reference nor changes unsupported state to
supported. Mixed valid/invalid lists retain their valid references; invalid-only
lists become empty. The regression now parses the review while still rejecting
artifact acceptance and preserving the Profile. This mirrors the earlier exact
job-citation response handling without weakening the client parser.

The same boundary correction supplies `careerCitations` containing exact
references to the selected facts, including request-local answer facts when
present. Its policy distinguishes those fact references from owner/context IDs.
A public-handler assertion failed with the missing catalog, then passed with
the literal selected fact reference. References remain traceability, not
semantic proof; raw invalid provider references are still checked and blocked.
The support diagnostic version is
`artifact-support-openai-v4/prepared-citations-v1/number-guard-v2/answer-input-v1/career-citations-v1`.
The catalog duplicates existing reference metadata and may increase token use;
the existing serialized-envelope limit remains enforced. These latest changes
have no live evidence yet. They do not resolve qualification source-ID
reliability, full semantic quality or accepted-result cost. Issue #52 stays open.


## Deployed career-citation checks

PR #101 merged as `cfa8bc270b0a8df2b3979294bf3fe22482fa50f9`. The user then
authorized publishing its build `31e65fcb3f96873735beb001222ccdab8d70f0cd`
(deploy `6acc0e1cc1a92c00087d9654`) and exactly two synthetic draft checks,
without retries. Netlify confirmed this build as published and locked.

Published and immutable preview returned HTTP 200/no-store in 11,790 ms and
8,754 ms; handler durations were 10,624 ms and 8,235 ms. Both responses parsed
through the actual public client in an offline replay, and both resumes passed
its acceptance checks. Artifact acceptance remained unresolved. Profile
snapshots remained unchanged; no output was saved, exported or accepted in a
live browser.

Published artifact support was unavailable: every block remained uncertain with
`check_unavailable`. Provider HTTP 200/completed metadata does not establish a
valid complete support judgment. The sanitized response and metadata logs do
not identify which contract check failed; no raw provider response was captured.
Preview support was complete and returned current exact career references for
the body and application answers. Its title block was unsupported with missing
job citations/support and changed metadata concerns, although the generated
title matched the posting. This is a false rejection observation, not permission
to manufacture citations or override the judgment. Other preview blocks were
supported, with exact returned original job excerpts and no answer concerns.

[Career-citation deployed evidence](issue-52/career-citations.deployed.live.json)
records the separate HTTP/handler/stage timings, revision/origins, request
hashes, observed provider usage and public-client outcomes. The inspected two
function entries exposed metadata/usage without inputs, generated prose or
keys. Provider retention and billed/accepted-result costs remain unverified.
Eight provider calls were observed across the two authorized workflow attempts;
both attempts are used. Both outputs contained five answer items for one
explicit posting question. Reusing this diagnostic case is not held-out quality
verification, and the general qualification/review reliability gates remain open.

The exact PR #101 head passed Go, all 129 Node tests, TypeScript, independent
Standards and Spec patch reviews, GitGuardian and Netlify preview checks before
merge. SonarQube analysis remained queued with no result at merge; GitHub's
main-branch rules query was empty and protection lookup returned 404. No Sonar
pass is claimed. Issue #52 remains open.


## Local support diagnosis and neutral review input

After the deployed allowance was exhausted, the user authorized up to four
local production-handler workflow attempts with the same synthetic posting and
sole career fact. Exactly four were used without retries (16 provider calls).
Two used v4 and two used the request correction described below. No deployment
was published or called under this local-only authorization.

Private raw-response capture identified a specific failure in the first local
v4 run: a greeting judgment had `scope: nonfactual` but `nonfactual: false`.
The existing consistency guard correctly left the entire artifact review
unavailable. This does not establish the cause of the earlier deployed
unavailable response, whose raw provider output was not captured. The second
v4 run returned complete judgments but rejected title/summary support because
sources were not associated with those blocks, despite supplied catalogs.

The review request had included unfinished public response placeholders:
empty source arrays, uncertain state, unknown scope, false nonfactual flag and
`check_unavailable`. The correction sends only explicit index, field and exact
generated text for each block. The policy asks the independent reviewer to
select sources from the existing catalogs, including for titles/summaries,
and explains that absent preassigned sources are not missing evidence. Public
response shapes, source checks, scope consistency, acceptance gates, model,
reasoning, provider count and envelope bounds remain unchanged. The diagnostic
version is now `artifact-support-openai-v5/prepared-citations-v1/number-guard-v2/answer-input-v1/career-citations-v1/neutral-blocks-v1`.

The public-handler request regression failed on the old placeholder objects and
then passed with the literal expected indexed text blocks. Replaying the first
recorded inconsistent judgment through the corrected handler still leaves the
review unavailable. This verifies the request contract and preserved guard;
it does not prove that placeholders caused the model failures.

Both subsequent v5 local runs produced complete reviews with supported summary,
greeting, body and answers, but invented title citation offsets instead of
copying the supplied exact catalog entry. Both titles remained unsupported with
`invalid_job_citation` and `changed_job_metadata`. No citation was repaired or
acceptance overridden. All four recorded provider response sets were replayed
offline through the production handler and actual public client: all parsed,
all preserved Profile snapshots, and all blocked artifact acceptance. Resumes
were acceptable in runs 1, 3 and 4; run 2 remained unresolved. Replaying through
the handler preserves response object order; reserializing decoded Go maps
had initially made the client replay unsuitable and was corrected offline.

[Local diagnostic evidence](issue-52/neutral-blocks.local.live.json) retains
bounded outcomes, synthetic citation metadata, usage and source hashes without
keys or full provider prose. This small repeated-input before/after diagnostic
is not a quality benchmark or causal proof. Go, all 129 Node tests and TypeScript
checks pass. The first sandboxed Node invocation could not spawn Go; the full
suite passed when rerun with subprocess access. No attempts remain under this
allowance. Qualification reliability, exact citation selection, full deployed
artifact acceptance, held-out quality and accepted-result cost remain open.


## Exact citation alternatives in the support schema

The next correction constrains the existing `sources` and `jobSources` array
items to complete supplied catalog entries. Each nested `anyOf` alternative
fixes all three fields with singleton enums. This keeps a fact's Profile/ID/
revision and an excerpt's start/end/quote together, instead of allowing arbitrary
strings and integers under a generic object schema. It changes neither the
public response contract nor server-side citation, scope and support guards.
No source is substituted or repaired after the provider responds.

The approach uses documented nested `anyOf` and enum support in
[OpenAI's Structured Outputs guide](https://developers.openai.com/api/docs/guides/structured-outputs?api-mode=chat).
The implementation caps the combined catalog at 300 complete choices (900
singleton enum values plus seven state/scope values, below the documented 1000
limit). It also bounds the encoded response format to 100,000 bytes, below the
documented 120,000-character total string limit; property counts and nesting
remain below documented limits. Empty catalogs require empty arrays. Oversized
catalogs/formats leave review unavailable without an artifact-support call;
there is no truncation or unconstrained fallback. Existing full-envelope limits
still apply. Duplicating catalog data in the schema increases payload/usage and
can reduce the inputs eligible for this review; live cost/latency is unmeasured.

The existing public-handler transport regression failed because v5 allowed
arbitrary source objects, then passed with exact alternatives for all supplied
career references and occurrence-specific UTF-8 job excerpts. Additional
public-handler checks verify an empty career catalog permits no references
and 301 repeated posting occurrences cannot trigger an unconstrained review.
Old controlled provider outputs that ignore the schema remain subject to the
unchanged server guards; frozen historical fixtures were not rewritten.

[Controlled schema evidence](issue-52/exact-citation-schema.controlled.json)
uses a standard JSON Schema validator against the actual captured handler
schema. The recorded v5 title tuple with invented end offset 20 passes the old
schema and fails the new one. A separate controlled exact catalog selection
passes the new schema; an owner ID substituted for the selected fact fails.
These are structural checks, not accepted live outputs. Empty citations and
inconsistent scope flags can still satisfy the schema, so the existing server
support and consistency checks remain necessary. Replaying the recorded bad
provider response through the handler still blocks the title; it is not repaired.

The support diagnostic version is now
`artifact-support-openai-v6/prepared-citations-v1/number-guard-v2/answer-input-v1/career-citations-v1/neutral-blocks-v1/exact-schema-v1`.
Full Go, all 129 Node tests and TypeScript checks pass. This change has no new
live provider or deployed evidence, and all previous attempt allowances remain
exhausted. Provider schema compliance, semantic reliability, qualification
source-ID behavior and complete live artifact acceptance remain unproven;
#52 remains open.


### Exact citation schema deployment and client comparison — 2026-10-12

PR #104 merged into `main` at `d3d3de4`; the milestone branch was synchronized.
With specific fresh authorization, completed build `29ce680` (deploy
`6acc15784c742b0008d770ca`) was published and observed as **Published & locked**.
Exactly two synthetic Application Draft attempts ran without retries: one against
the immutable preview and one against the published app. Both used the Portuguese
Java posting and sole fact “Uso Java para desenvolver APIs.” The previous temporary
preflight was absent; a new offline production-handler capture was shown before
approval. Its downstream envelopes illustrated controlled generated prose, while
live downstream prose depended on generation.

[Credential-free results](issue-52/exact-citation-schema.deployed.live.json) record
HTTP 200 in 10,544ms / 9,408ms and handler durations 9,190.288483ms / 8,576.246308ms
(preview / published). Viewed Netlify records showed eight provider dispatches
across two requests: TypeSafe plus generation, resume support and artifact support
for each. The v6 artifact reviews completed and selected exact supplied job/career
references. The viewed operational records contained metadata without prompts,
source prose or keys; provider retention and actual billed/accepted-result cost
remain unverified. Both authorized attempts are exhausted.

The preview output had one answer item and supported judgments for all artifact
fields. The published output had five items for one employer question, and its
answer claim was unsupported with `stronger_claim`; other artifact fields and both
resume reviews were supported. These diagnostic repeats do not prove general model
reliability, held-out quality or complete release acceptance.

Offline replay through the original client rejected both unmodified responses:
`normalization`, `owner` and `temporal` had equivalent JSON values with different
object-key order. The fact values matched structurally. The client compared those
nested fields by serialized property order. A public `generateMaterials` regression
passed the unchanged-order control and failed the reordered-resume case before the
fix. The corrected client compares parsed JSON structurally in resume/artifact fact
checks and review snapshot checks. It ignores object-key order while preserving
array order, key presence, types and values. Regressions retain rejection of changed
revisions, facts, evidence, links, missing properties and array order. This is value
comparison only; support states, citation validation and explicit acceptance remain
independent guards.

Replaying the same unmodified live responses with the corrected local client parses
both; preview resume/artifacts can be accepted, while published artifacts remain
blocked as unresolved. Both resume reviews can be accepted. Profile remains unchanged
in replay. This is **offline client evidence**, not corrected-client deployment or a
live browser journey. No response was repaired or reordered to obtain acceptance,
no new provider request was made, and the published answer failure remains recorded.
All original #52 acceptance criteria and broader semantic/cost gates remain open.


### Portuguese responsibility wording versus seniority — controlled correction, 2026-10-12

The published build `29ce680` answer contained “principal responsabilidade”. The
shared resume/artifact strength guard matched `principal` anywhere in a block and
compared it only with career source terms, treating this ordinary Portuguese noun
phrase as a rank. A controlled production-handler counterfactual reconstructed the
recorded generated output and supplied supported review judgments. It reproduced
`stronger_claim`; changing only the phrase to “responsabilidade central” removed
that concern. Original raw live provider judgments were not captured, so this
isolates a deterministic failure path without claiming it was the sole live cause.

The revised lexical guard excludes `principal` when followed on the same line by
Portuguese `responsabilidade`. It applies the same classification to source facts
and generated text: responsibility wording cannot supply evidence for a principal
engineer claim. Other occurrences of `principal`, other strength terms, metrics,
source identity, ownership and independent support-review outcomes remain guarded.
The diagnostic versions add `strength-guard-v2`; provider policy/schema, model,
retry behavior and public response shapes are unchanged. This is a bounded lexical
correction, not a semantic subject classifier or certification of support.

The public handler regression failed before the fix and passes afterward. Eleven
controlled cases cover question/declarative responsibility phrasing, English and
Portuguese principal-engineer claims, rank before/after the role, a second rank
claim after the responsibility phrase, source-side rank contamination, literal
supported rank, other strength words, invented metrics and independent review
rejection. The fixture judgments are controlled; they do not prove model quality.

[Controlled evidence](issue-52/strength-context.controlled.json) also records replay
of the reconstructed published scenario before/after the correction through the
actual client. Before, its artifact review cannot be accepted; afterward the
controlled review can be accepted, with Profile unchanged. The recorded generated
answer still has five items, and this correction does not resolve question-selection
concerns or broader qualification/draft reliability. The reused evaluation corpus's
gold labels do not describe this diagnostic case, so its aggregate scoring is not
reported as precision/completeness evidence.

No provider calls or publication were made for this correction. All existing live
attempt allowances remain exhausted, and the published locked build remains
`29ce680`. Corrected deployed-browser behavior, held-out provider quality and actual
accepted-result cost remain unverified. The original #52 release requirements remain
open; this controlled improvement is not full release acceptance.


### Responsibility correction — authorized deployed repeats, 2026-10-12

After fresh approval, completed build `a91096d` (deploy
`6acc5c0f469839000864262c`, PR #106; merge `6c0cea4`) was published. Netlify
visibly confirmed **Published & locked**, preserving the auto-publishing lock.
Exactly two synthetic Application Draft attempts ran without retries: one immutable
preview and one published origin. Both used the preflighted Portuguese Java posting
and sole career fact “Uso Java para desenvolver APIs.” The generation input was
exact; downstream preflight examples used reconstructed controlled prose and actual
generated prose varied. No additional attempt allowance remains.

[Live evidence](issue-52/strength-context.deployed.live.json) preserves the results:
preview HTTP 200 in 11,125 ms and published HTTP 200 in 10,396 ms. Both returned
complete artifact review with supported title, summary, greeting, body and answers.
The unmodified responses parse through the corrected local public client; both
artifact sets can be accepted with Profile unchanged. Preview resume can also be
accepted. Published resume cannot: the provider marked its “Java” skills subheading
uncertain, explaining in Spanish that the source did not establish a separate
skills heading. That semantic over-rejection and concern-language failure remain
recorded; an HTTP 200 or complete review does not mean accepted output.

Preview generated three answer items for the one employer question; published
generated one. Neither generated “principal responsabilidade”, so these calls do
not directly exercise the deployed lexical exception. They verify deployment of
the revised guard version and actual client compatibility through offline replay,
not a corrected live-browser journey. The earlier controlled responsibility/rank
cases remain a separate kind of evidence.

The viewed Netlify function logs showed two diagnostic records, each with one
TypeSafe and three OpenAI dispatches (eight total), complete resume/artifact stages
and `strength-guard-v2`. Handler durations were 9,912.021834 / 8,823.92764 ms.
Viewed records contained operational stages, usage, durations and platform request
metadata; no source prose, prompts or credentials were seen. Provider retention,
actual billing and accepted-result cost remain unverified/null. This narrow repeat
does not establish broader provider quality, bilingual reliability, browser/PDF
acceptance or full #52 completion. All original release gates remain open.


### Explicit CV support-review language — controlled contract correction, 2026-10-12

The published `a91096d` resume review returned a Spanish concern for the neutral
“Java” subheading under a Portuguese request. Inspection and a failing public-handler
regression confirm that the support-review envelope did not carry the requested
`cvLanguage`; its policy asked for reasons in each statement's language, which is
ambiguous for a neutral technology name. This is a missing-input observation, not
proof of the sole cause of that provider response.

CV support review now receives the validated selected language (`en` / `pt-BR`,
with English for an omitted language) and explicitly requests reasons in that
language, including neutral headings. Diagnostic version adds `review-language-v1`.
The public-handler test failed before the change and passes for Portuguese, English
and omitted-language requests. It also retains independent uncertainty and exactly
one support call: language selection cannot authorize a claim, rewrite a judgment
or retry a provider. These are controlled provider-boundary assertions, not proof
of model adherence or semantic quality.

No live provider calls or publication were made for this contract correction. The
published locked build remains `a91096d`, and its resume uncertainty/Spanish concern
remain recorded failures. This change does not resolve the semantic heading judgment,
extra answer selection, corrected live-browser/PDF verification, held-out bilingual
quality or accepted-result cost. All original #52 release gates remain open.


### Final frozen browser batch — stopped without retries, 2026-10-12

This section supersedes the preceding publication status, while preserving its
dated observations. The user authorized publication of frozen build `956f5c6`
(deploy `6acc5ef0e46b8000081596c0`, PR #108), keeping auto-publishing locked,
and one final browser batch capped at four workflow attempts / fourteen provider
calls, with no retries or further patch-and-test cycle. Netlify confirmed
**Published & locked**. Exactly two workflow attempts ran, both in the immutable
English preview; seven provider dispatches were observed. Both Portuguese
published attempts were skipped because the existing Profile contained unrelated
content. That Profile and its existing credentials were preserved without editing
or submitting them. The unused allowance was not converted into more attempts.

[Structured evidence](issue-52/final-bounded-browser.live.json) records origins,
revision, authorization, timings, usage, failure stages and scope limits. The
provider-free preflight fingerprint was
`ceffad8c38a2c13a84b2f516312cb61748aebe28bfe86c9b4b123db6c8ff9995`.
Its canonical approved fact fixture differed from the actual browser-created
manual fact in origin/IDs/revisions and unsupported/unreviewed metadata. This
limits its value as exact actual-payload evidence: browser HTTP payloads were
not captured, and generated prose/jobContext were unknown before submission.
The only entered career fact was “I use Kotlin to build APIs.”; the matching
four-line posting and selected English language are retained in the evidence.
No extra qualification answers were entered or selected.

[Qualification browser output](issue-52/final-bounded-qualification.en.txt)
rendered a usable review with partial support and unknown company/seniority/location.
It also listed the API-building duty among qualifications. Handler HTTP 200
completed in 6,381.106118 ms with one TypeSafe and two OpenAI dispatches. Clicking
“Generate without these” continued to the draft without adding qualifications.

[Draft browser output](issue-52/final-bounded-draft.en.txt) rendered all five
artifact blocks as uncertain with `check_unavailable`; acceptance was disabled.
The handler returned HTTP 200 in 6,375.614972 ms, but diagnostics correctly reported
`provider_failure`: generation and resume review completed, while artifact support
returned HTTP 400. One TypeSafe and three OpenAI dispatches occurred. The provider
error body was not captured, so the underlying reason is unproven. No automatic
or deliberate retry was made. The answer “I can explain my use of Kotlin in
English” is a potential unsupported language-ability assertion; four answer items
for one employer question remain a fidelity observation, not an accepted result.

[Resume browser output](issue-52/final-bounded-resume.en.txt) independently
rejected the Kotlin/API claims because their cited manual fact was marked
unsupported. Accept resume, Copy text and Save as PDF were disabled. This
conservative decision must not be treated as proof that the language correction
failed or overridden to force acceptance. No output was accepted, copied or
exported, and there is no new actual-PDF success claim. Manual metadata differs
from the approved canonical preflight, so this case cannot establish the behavior
of that approved-fact scenario.

Viewed Netlify logs contained operational stages, timings, usage and platform
request metadata; no source prose, prompts or credentials were seen. Published
provider coverage, retention, actual billing and accepted-result cost remain
unverified. Browser observation elapsed times are upper bounds including polling
delay, not measured HTTP latency. Both preview test keys were cleared through
the UI and their absence verified after reload. No runtime source changed for
this batch. Testing stopped here as agreed.

**Release gate: not passed; #52 remains open.** The consolidated requirement audit
in issue comment 6117692107 remains applicable. Controlled integration evidence
does not establish reliable bilingual deployed acceptance, semantic quality or
accepted-result cost. This final batch adds an actual English browser failure
and a privacy-preserving Portuguese skip; it does not close those gaps.

### Multiline job-citation schema — bounded local diagnosis and correction, 2026-10-12

After the prior frozen browser batch ended, the user separately approved **one**
local OpenAI artifact-review diagnostic, with no retries, TypeSafe requests,
publication, Profile changes or acceptance. This new allowance is exhausted.
[Sanitized evidence](issue-52/newline-enum.local.live.json) records the exact
payload hash, synthetic input, schema checks, provider status and timing. The
private raw response is not committed.

The current production handler received the same four-line Kotlin posting and a
fresh synthetic Profile created by the actual manual-edit and qualification-
projection clients. Unknown semantics and unsupported evidence metadata were
preserved. Generated prose was a controlled faithful example; it and the local
IDs differ from the prior browser attempt. Standard JSON Schema validation and
the documented required-field, object, enum-count and depth checks passed. These
checks did not establish provider admission.

The one frozen request returned HTTP 400 in 1,527.389 ms. The provider identified
`response_format` and specifically rejected a newline in a `jobSources` quote
enum. The schema has 24 properties, 22 enum values and depth 5; its size does not
explain that error. Manual fact support metadata affects judgment, while this
request was rejected at schema admission. This proves a reproducible structural
defect for multiline posting excerpts. The prior browser error body remains
unavailable, so this is not asserted as the sole proven historical cause. Usage,
actual charge and accepted-result cost remain unavailable.

A public-handler regression now emulates the observed HTTP 400 when the actual
outgoing strict schema contains a newline string enum. Before the correction,
the existing multiline/UTF-8/CRLF citation case failed with an unavailable review,
matching the visible browser failure. Afterward it passes. The schema still
binds complete career references and complete original start/end pairs. Job
quotes use the string type rather than literal enums; the existing deterministic
UTF-8 and exact-original-byte checks remain mandatory before a quote is retained.
No quote is guessed, normalized or repaired. Independent judgments, number and
strength guards and explicit acceptance remain in force. New EN/PT tests through
the public handler/client reject invented text and an added newline even at
otherwise correct source ranges, while preserving the Profile. The shared
prepared source remains the only source mapper. Policy/model settings and public
response shapes are unchanged; diagnostics identify `range-schema-v2`.

The correction has **no fresh provider, deployed browser or PDF verification**.
The approved published runtime remains `956f5c6`; this section authorizes no
publication or additional calls. Historical failures, uncertain manual facts,
extra-answer fidelity observations, qualification reliability, bilingual browser
acceptance and the held-out quality/cost comparison remain explicit.
**#52 remains open; the overall release gate has not passed.**

# Grounded Application Draft artifacts (#48)

Milestone 4: `codex/evidence-backed-cvs-and-applications` → `main`.
Blockers #47 and #20 were closed before implementation. The user confirmed the
Application Draft HTTP handler with controlled provider transports, public
review/acceptance functions, and EN/PT browser copy/PDF flows as test seams.
The agreed independent review baseline is `ad0d20768f088e25adc73c02ac1d0a7aefc50384`.

## Contract

The current browser requests `reviewArtifacts: true` with the existing approved
Profile projection and resume-review contract. Generation and the two independent
support assessments share the existing 25-second workflow deadline. There is no
automatic retry or regeneration, new provider, saved Profile mutation, server
persistence or new source connector. The complete original Job Posting still goes
through Jev before generation. Legacy HTTP clients keep their existing response
shape; artifacts without review cannot be accepted/exported through the current UI.

All metadata, job-summary text, greeting, cover body and application answers are
covered by ordered paragraph blocks. Each block's support outcome applies to ALL
its assertions and relationships. The application-owned fixed closing and exact
saved-name signature remain outside generation/support assertions. An independent
bounded check returns supported/uncertain/unsupported judgments, selected Profile
fact/revision references, and exact UTF-8 byte ranges/quotes from the original Job
Posting. Ordinary greetings and nonfactual connective wording need no citations.
Each independent judgment classifies its assertion scope as career, job, mixed or
nonfactual. Server and browser both enforce the appropriate citation family:
candidate assertions need career support, employer/role assertions need Job Posting
support, and mixed statements need both. Scope classification is itself fallible;
wrong-domain citations cannot upgrade a correctly classified factual assertion. The checker is fallible; references are
traceability rather than semantic proof.

The resume's existing guards are reused for career assertions: missing/stale
references, negation/qualifier changes, invented numbers, strengthened seniority or
proficiency and cross-owner combinations cannot be upgraded by a provider judgment.
Job excerpts must resolve exactly at valid UTF-8 boundaries; metadata must occur
in its cited job excerpt, and numeric claims must occur in cited support. Unknown
employers stay null. These conservative guards can flag legitimate paraphrases or
multi-owner paragraphs; the person can separate/remove unsupported detail or supply
a complete truthful replacement. This implementation does not claim that a finite
lexical guard proves semantic support.

Each explicitly supplied qualification answer has a request-local source identity,
separate from confirmed qualifications. Its value is the person's exact answer,
not the question (which could otherwise turn a qualification request into evidence).
Negative/unconfirmed qualifications remain negative/unconfirmed. Private Profile
salary fields remain excluded by the existing projection. Salary answers can use
only information the person explicitly supplied for the application, never inferred
compensation, examples, durations or outcomes.

The check separately evaluates application requirements against applicationAnswers
alone and the complete original posting. Its `answerConcerns` cannot be satisfied
by Java appearing in a resume or cover letter. Missing required answers must be
corrected; removing answer blocks is disallowed so removal cannot hide an omission.
No global draft keyword search is used as evidence of field completeness.

## Review and accepted artifacts

The panel focuses on material concerns by default; supported blocks and original
sources are available through a single expansion. One acceptance covers resolved
summary/metadata, cover letter and answers. The resume retains its independent
acceptance. The whole draft receives an accepted status only while BOTH sets of
accepted artifacts are current. This means review completion, not certification.

A deliberate correction records the entire replacement as user-authored evidence
for this draft. It clears current citations; previous references are audit history
only. The person owns the correctness and completeness of the new assertion,
including requested answers. There is no automatic provider recheck. Cover edits
are immediately validated through the existing greeting/body/closing/signature
and under-400-word boundary, and acceptance validates the complete cover again.

Accepted content is cloned, keeps the frozen local identity and full source snapshot,
and supplies Results, clipboard and print from the same fields. Pending edits do
not change accepted content; cancellation restores accepted wording. Stale Profile
changes preserve the accepted content/sources but disable acceptance and copy/PDF.
A failed generation preserves the previous result. A pending replacement can restore
the previous result with accepted content, including partially accepted independent
artifacts. PDF names use accepted metadata, and actual A4 height still gates print.

## Controlled verification

- `go test ./...`: passed, including the existing semantic scorer regressions.
- `npm test`: passed (all 10 test files), TypeScript and Vite production build passed.
- Focused HTTP controls cover fabricated leadership, metrics, proficiency, missing
  citations, forged job excerpts, answer-specific omissions and request-local EN/PT
  negative qualification/salary assertions. Existing #47 controls cover cross-role
  ownership and faithful paraphrases through the same career guard.
- Public acceptance controls cover unresolved blocking, user-authored corrections,
  signature parity, immutable accepted content, stale snapshots, answer-removal
  blocking, incomplete field coverage and forged Job Posting references.
- `node tests/resume-review-browser.check.mjs`: passed in EN/PT at 390/1440px;
  unsupported exports blocked, explicit corrections and independent acceptance,
  answer copy, Chrome print parity, prior-result recovery and stale export blocking.
- `node tests/apply-checklist.check.mjs`: passed in EN/PT at 390/1440px, including
  unknown employer/title and existing Apply workflow behavior.
- `node tests/results-firefox-print.check.mjs`: passed: actual one-page A4 resume,
  accented saved name exactly once across signed cover display/copy/PDF, unsigned
  Portuguese cover guidance, narrow layout and signature-induced overflow blocking.
- Actual Chrome cover PDFs and screenshots are in `evidence/`; sample PDFs were
  extracted, rendered and visually checked. This is controlled local evidence.

The frozen 34-case semantic corpus was also run with
`go run ./scripts/semantic-quality -mode controlled -split all`.
Its legacy-contract adapters retain the expected eight diagnostic cases: two
35-fact capacity omissions and six deliberately invalid draft controls. The scorer
detects injected management, requirement-as-skill and transferred metrics. Those
legacy controls do not exercise the new support call; the new HTTP/browser controls
above do. The saved report is local `/tmp/issue-48-semantic-controlled.json`.
No live provider accuracy, cost/latency improvement or deployed behavior was measured.
The milestone's final release gate remains #52.

## Independent review and remediation

Standards: no documented-standard violation; one duplicated source-display smell.
The career source/context display is now shared by both review panels, retaining
identity, referenced ownership, education, proficiency and accepted excerpts.

Spec: one source-domain enforcement finding. An either-source test could allow a
candidate assertion with Job Posting citations alone (or an employer assertion
with Profile citations alone). The support response now explicitly classifies
assertion scope. Required citation families are enforced by both server and
frontend acceptance. EN/PT wrong-domain regressions, missing-source mixed claims
and a valid mixed-source statement pass. Ordinary greetings stay citation-free.

Full checks passed before review; focused handler/type/acceptance checks and
browser/export regressions were repeated after these changes.

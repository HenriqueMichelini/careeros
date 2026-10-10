# Application Draft resume review (#47)

Milestone 4 branch: `codex/evidence-backed-cvs-and-applications` → `main`.
The native blocker #46 was closed before work began. The user approved testing
through the draft HTTP contract, review/acceptance state and rendered
review/copy/PDF flow, with controlled transports. Approved review baseline:
`25329b0076567cb45b5f7e75f40f5591e0f25b57`.

## Contract and boundaries

The browser requests `reviewResume: true` alongside the existing approved Profile
projection. The server requires that projection and still classifies the complete
original Job Posting through the unchanged Jev field rubric. Legacy clients can
request the original draft contract; such resumes have no acceptance capability
in the current UI and require regeneration before copy/PDF. No Profile write,
server persistence, provider change or new Jev task is introduced.

Generation produces at most 80 structured statements with a six-section enum,
block kind, plain wording and up to 16 complete Profile/fact/revision references.
Every statement, including entry headings, needs citations. Source references
must resolve within the selected context. Protected cited identity/qualification
fields, explicit qualifiers and negation remain verbatim; numeric and seniority
markers cannot exceed cited support. Combining owners/role/project contexts is
blocked deterministically. These guards are conservative bounds, not semantic
proof. C# and underscores remain valid literal career text. Temporary confirmed
qualifications receive request-local fact references and cannot inherit a role
or project; they are never added to the saved Profile.

One bounded independent support assessment uses the existing OpenAI provider
and model. It receives selected facts, accepted excerpts, contrasting context,
qualification answers and the structured statements as untrusted JSON, separate
from its system policy. Its schema requires one supported/uncertain/unsupported
judgment per statement. Deterministic failures cannot be upgraded by this check.
Independent skills do not establish combined use in a role; faithful paraphrases
may be supported. The result remains fallible and the UI says so. There is no new
Jev support rubric or claim of semantic certification. Generation and support
share the route's 25-second deadline. Malformed/refused/truncated/failed checks
fail the request with no automatic retry, preserving the previous draft.

The review shows original fact wording, exact revision references, relevant
entity identity/period context, accepted supporting excerpts and concerns. One
explicit acceptance covers resolved wording; routine per-fact confirmation is
not required. Uncertain/unsupported claims must be removed or replaced by the
person's own complete factual assertion. The correction action explicitly records
that assertion as user-authored evidence for this draft only; its old citations
are audit history and cannot support the new wording. This is user assertion,
not independent verification of truth. Acceptance rechecks active states and the
unchanged source snapshot, without additional model calls.

Pending edits are isolated from the accepted resume. Cancellation restores the
accepted version. Profile changes preserve the previous wording and original
snapshot, while blocking acceptance, copy and PDF until a new draft is generated.
Failure/cancellation during generation does not clear the previous result.
Preview, copy and PDF derive from the same accepted structured statements and
frozen local name/contact identity. CV language remains independent from UI
language. Actual unscaled content height gates A4 export; long accepted content
stays visible with overflow advice rather than being clipped or exported as a
misleading one-page document. Summary/cover/answers keep their existing workflows
and are explicitly outside this slice's support check.

## Controlled evidence

- HTTP regressions cover stale and missing citations, invented 40% metrics,
  stronger seniority, cross-role combinations, changed employer names, removed
  negation, strengthened approximate durations, valid paraphrase routing,
  temporary user confirmations and independent semantic concerns.
- Controlled OpenAI responses exercise supported, uncertain and unsupported
  decisions. Outage, truncation, malformed state and omitted judgment index stop
  the route after one generation and one assessment, without repair/retry.
  These fixtures verify orchestration and bounds, not live semantic accuracy.
- Public frontend tests cover acceptance gates, truthful user-authored correction
  provenance, removal, immutable accepted snapshots, stale revisions/excerpts,
  language rendering and malformed review references.
- `node tests/resume-review-browser.check.mjs` exercises the real Apply→Results
  flow with synthetic fetch responses. EN/PT UI and CV language at 390/1440px:
  unsupported copy/PDF blocked; original support visible; explicit correction;
  acceptance; pending edits/cancellation; copy/print parity; failed generation
  retaining the previous accepted result; stale Profile export blocking; real
  accepted-content overflow blocking. No credentials/providers are used.
- Chrome-generated PDFs are actual one-page A4 artifacts, with extracted text
  checked and rendered pages visually inspected. Screenshots/PDFs are in
  `evidence/`. This is local Chrome evidence, not deployed or Firefox evidence.

Live support-check accuracy, token/billing changes, latency improvement and
production deployment behavior are unmeasured. The separate final milestone
cross-workflow/release gate remains #52.

## Final verification and review

Pending final suite and independent Standards/Spec review; update before merge.

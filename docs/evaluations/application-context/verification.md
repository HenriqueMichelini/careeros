# Application Draft context (#46)

Review baseline: `264c82d` (merged #45). Milestone 4 branch:
`codex/evidence-backed-cvs-and-applications` → `main`. Native blocker #45 was
closed before implementation. The user approved the production draft HTTP and
frontend request/rendering seams, controlled transports, and review baseline.

## Contract

The browser supplies the same approved professional-fact projection used by #45.
The backend validates its shape and allowed fields; this client-supplied document
is untrusted, not an authorization token. Candidate facts retain ID/revision,
owners, all context references, temporal precision, semantic flags and accepted
excerpts. Typed context links form connected components so a skill shared across
roles/projects retains every owner's facts, identity and dates as one unit.
Lexical overlap ranks components; skills/tools/competencies and formal
qualifications are structured candidates even without shared wording. Negated,
uncertain, aspirational and invalidated facts rank first conservatively. Zero-hit
other components are omitted and counted separately from budget exclusions.
Relevance is a heuristic, never proof of support or completeness. The frontend
also forwards the preceding assessment's cited fact IDs as retrieval hints. The
server requires every hint to exist in the validated professional projection and
prioritizes its entire connected component, preserving paraphrased support that
has no lexical overlap. Hints never establish semantic support or bypass Jev;
unavoidable budget omissions remain explicit.

Selection uses a 24,576-byte serialized JSON budget. Whole components are kept
or omitted, never truncated; arbitrary first-N slices are not used. The response
records selected source references, bytes and both exclusion counts. A budget
omission sets complete=false. Results displays a bilingual warning, and even
within budget reminds users to review each material because selection may miss
useful information. These references are traceability, not claim verification;
#47/#48 own artifact-level support checks and review.

Compatibility callers receive professional-only legacy units, with lexical
ranking and whole-unit budgeting. All legacy professional units remain eligible
because legacy strings lack typed negation/context. Local stable paths identify
legacy sources; canonical sources include Profile/fact ID and revision. Contact,
goals, employment status, current/desired salary, additional private notes,
credential IDs and URLs are excluded from automatic provider projections.
Compensation is never automatically read from Profile. Only a user-supplied,
explicit application answer can convey it. Local name/contact rendering and
cover-letter signature remain deterministic.

The system message owns all generation instructions. The user message is a JSON
object containing selected evidence, the complete prepared posting, extractive
job context, temporary confirmations/answers and selection metadata. Job context
is independently validated against the original and remains untrusted. Every
route still gates the complete original through Jev; a reusable artifact never
bypasses the gate. Source size, strict request shape, provider envelope limit,
no-store, refusal/truncation rejection and deliberate retries remain enforced.
The bounded request-body ceiling is now 512 KiB to accommodate the canonical
projection (at most 500 facts); legacy field/list limits remain unchanged.
Nothing writes to the saved Profile or introduces persistence/providers.

## Evidence and limits

- Handler red/green reproduced salary/private-note disclosure and mixed trusted
  instructions/data, then verified exclusions and separate system/user messages.
- Controlled HTTP checks cover canonical negative/uncertain evidence, rejected
  private canonical fields, and heavy legacy input with disclosed whole-unit
  budget omission. Existing handler checks preserve language, unknown metadata,
  confirmations, signatures, refusal/truncation and malformed-input failures.
- Frontend request checks preserve the approved projection, unchanged original
  Profile, coverage metadata and deterministic local signature. Typechecking
  passed after integration.
- `controlled-report.json` reuses #45's labeled retrieval corpus and #34's output
  corpus/baseline. All 16 relevant labels and all three negative labels were
  retained across 13 retrieval cases. Heavy irrelevant components were counted
  separately. The full approved baseline candidate set has no retrieval misses.
- Forty field-specific replay rows cover Job Summary, resume, cover letter and
  answers independently, including known invented-management, requirement-as-
  skill and transferred-metric outputs. The frozen provider outputs are replayed
  identically; this does not establish generation-quality improvement or that
  all unsupported output is rejected. Existing label limits and human
  adjudication requirements remain. No whole-draft keyword score substitutes
  for these field-level checks.
- Actual token deltas, billing, live latency and semantic generation quality are
  unmeasured (tokens=null); serialized provider-envelope bytes are reported
  separately. Smaller context alone establishes no quality/speed advantage.

Reproduce the credential-free evaluation:

```sh
GOCACHE=/tmp/careeros-go-cache go run ./scripts/application-context -output /tmp/application-context-report.json
```

## Final checks and review

- Full Go suite, 110 Node tests, TypeScript typecheck and Vite production build
  passed after contract-test updates. The existing bundle-size advisory remains.
- Synthetic Chrome Apply/Results checks passed in EN/PT at 390px and 1440px,
  including the budget warning and preserved unknown metadata. Screenshots were
  visually inspected at mobile English and desktop Portuguese widths.
- Standards review: no hard documented-standard violations. Its duplicated
  lexical-tokenizer observation was resolved by sharing mechanical lexical
  tokens with qualification matching. Two malformed-input logging exits were
  corrected to record 400/input.
- Spec review identified lost paraphrased assessment evidence; a red/green HTTP
  case now retains the cited project fact and identity with zero lexical overlap.
  Review follow-up checks these corrections against the same fixed baseline.
- No deployment, live provider, actual token or generated-PDF evidence is claimed.

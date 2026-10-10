# Current application reuse and cancellation verification (#49)

Baseline: `afe4baee3651bc931c48b1f4dc83d47be766f3d8` (`main`).
Implementation branch: `codex/ai-quality-and-efficiency`, milestone 2.
Contract and artifact dependency table: [application-reuse.md](../../application-reuse.md).

## Acceptance evidence

| Requirement | Evidence |
| --- | --- |
| Current-memory reuse with artifact-specific dependencies | HomePage retains session hooks and removes inactive controls. Browser analysis identity includes exact original, projected Profile revisions, job/qualification versions and application revision. Job-context v2 excludes Profile dependencies; Go validates preparation/Jev/prompt/model/schema identities. Answers and CV language affect each newly generated draft, not posting-language qualification analysis. |
| Unchanged deliberate draft retries reuse analysis | Targeted browser regression performs failure, dismissal, reopen, answer edit, cancellation and successful drafting with exactly one `/api/qualification-gaps` request. New Application resets the session and the same posting requires fresh analysis. |
| Changed Profile repeats matching but not job extraction | Public HTTP reuse regression changes Profile/document revision and sends the previous artifact twice. Each recheck makes exactly one whole-original Jev call and one matching OpenAI call, with no extraction call. Browser manually edits a Profile skill and asserts rematching carries the retained job context and current evidence. |
| Original/version/source protection; no authenticity bypass | Public HTTP tests cover changed original, stale preparation/source identity, policy/input identity, schema/version, nonexistent occurrence, altered protected quote/original value, null/missing/extra fields and forged acceptance flags. Each invalid reuse still calls Jev on the exact original and makes zero downstream/repair calls. A blocked original is rejected before artifact use. Draft handler contract verifies the same gate and validation boundaries. Matching receives complete original source to assess the untrusted index; generation retains its independent original-source assessment. |
| Cancellation propagates and stale completions cannot apply | Browser verifies AbortSignal on draft Cancel, qualification Cancel, posting edits, navigation and changed TypeSafe credentials. The synthetic transport deliberately ignores abort; late successful draft/analysis responses cannot navigate or install stale results. Public HTTP test cancels the incoming request while matching is in progress and observes cancellation on the provider request context. Existing handler tests cover response-consumption deadlines. |
| Preserve input, answers, Profile and prior valid results | Targeted browser checks exact posting, supplied answers, saved Profile and prior Results through failure/cancellation. A completed job context included with a matching failure survives the next deliberate attempt. Current materials remain visible after a failed retry. |
| Invalid/unverifiable reuse offers deliberate reanalysis without repair | Stale result makes no automatic request. Explicit discard makes a fresh request without job context. Regression first reproduced stale-context resurrection after a fresh extraction outage; the fix clears retained context and completed identity at discard. Both fresh extraction and the subsequent deliberate retry omit the discarded artifact. |
| Keys, language/options and no automatic retry/fallback | Current credentials are used on deliberate retry without rerunning valid analysis. Changed CV language appears in the new draft payload; edited answers remain application-only. No cache/history/token/signing configuration, automatic retry, fallback or mandatory parsing-only call was added. |

## Completed checks

- `GOCACHE=/tmp/careeros-go-cache go test ./...`: passed.
- `node --test tests/*.test.mjs`: passed, 10 test files.
- `node_modules/.bin/tsc --noEmit`: passed.
- `node_modules/.bin/vite build --outDir /tmp/careeros-issue49-build`: passed;
  existing bundle-size advisory remains.
- `node tests/application-reuse-browser.check.mjs`: passed English 1440px and
  Portuguese 390px, including request counts, partial-stage failure, discard
  failure/retry, credentials, Profile revision, language, original text, reset,
  cancellation and ignored-abort late responses.
- `node tests/apply-checklist.check.mjs`: passed English/Portuguese at 1440/390px.
  Its independent provider fixtures explicitly edit the original to start fresh
  analysis; unchanged-retry behavior is tested in the targeted suite.
- `node tests/cv-shared-preview.check.mjs`: passed Profile/Apply A4 preview,
  section-order, density/overflow and PDF compatibility. Updated this synthetic
  flow to explicitly accept materials at the existing application-artifact review
  boundary before inspecting their title/content.
- `node tests/results-firefox-print.check.mjs`: passed actual Firefox Save to PDF
  (one page), signed-cover display/clipboard/PDF, narrow layout and overflow.
- Independent code reviews against the fixed baseline: Standards reported no
  actionable findings. Spec found the discard/retry recovery edge, which was
  reproduced with a failing browser test, fixed, and re-reviewed with no remaining
  actionable findings.
- `git diff --check`: passed.

## Rendered evidence and limits

Inspected [English desktop](evidence/reuse-en-1440.png) and
[Portuguese mobile](evidence/reuse-pt-BR-390.png) screenshots. Exact posting remains
readable, the retained understanding remains accessible, and the 390px layout has
no horizontal overflow. These images show the recovered session after controlled
failure/cancellation; browser assertions cover the busy and late-response states.

All provider responses are controlled synthetic transports. No real user keys or
paid provider calls were used. These checks establish application behavior and
call counts, not provider semantic-quality or deployed-environment certification.
Browser abort reaches backend cancellation only where the HTTP host propagates
disconnection; already-started provider work or charges may continue. This change
adds no claim to the broader final deployment/quality gates in issues #50/#52.

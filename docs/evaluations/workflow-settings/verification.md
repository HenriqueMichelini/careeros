# Verification — issue #50

The implementation and evidence use milestone-2 branch
`codex/ai-quality-and-efficiency`, targeting `main`. No production model setting
changed. After the user unlocked the vault, one authorized synthetic matrix
completed: 42 logical results, 51 HTTP attempts, 132 provider dispatches, no retry
or fallback. The earlier inaccessible-vault invocation dispatched zero calls.

Every live result has an agent-assisted per-field source review. Offline replay
preserves original outputs/calls and links to the original SHA-256. Independently
recalculated fixture and source hashes match the saved live report; every call
and HTTP attempt reconciles with its corresponding result/step, and all 42 review
indices occur exactly once. The compiler revision is `4c22c9a`; exact source
hashes identify the compared implementation.

[The measured decision](decision.md) retains current fixed settings. Current is
not certified correct: language drift, an unsupported ownership addition and a
real continuation omission remain explicit. Neither candidate demonstrates the
required full-workflow improvement. All total/completed/accepted-result prices
remain null because validation pricing is unavailable; partial estimates and
returned cache usage are distinguished from billing or savings. #65 stays open.

## Controlled verification

`controlled.v1.json` records 42 complete logical results, comprising 60 HTTP
attempts and 186 simulated provider calls across identical fixtures for all
three configurations. Every curated label is matched and every acceptance case
has zero curated forbidden-pattern hits. These are controlled transport results,
not model-quality, cache-hit, latency, billing, or user-acceptance evidence.
Fixture/source hashes were independently recalculated and match the saved report.
The report's compiler revision is its starting commit; exact source hashes pin
all uncommitted evaluation changes that were subsequently committed.

Test-first regressions cover fixed reasoning/model choices with unchanged Jev,
prompts, schema, natural prefix hashes and output ceilings; call and HTTP caps;
cache-write/missing usage; failed-attempt accepted-result cost accounting;
unknown validation prices; complete 35-fact continuation; current independent
support checks; invented leadership and the Portuguese Java-answer omission;
retaining dispatched failed-step evidence; binding actual unresolved claims;
stopping when the initial question is absent; and offline review integrity.

Local final checks:

- `GOCACHE=/tmp/careeros-go-cache go test ./...` — passed.
- `node --test tests/*.test.mjs` — all 10 test files passed.
- `node_modules/.bin/tsc --noEmit` — passed.
- `node_modules/.bin/vite build` — passed, with the existing large-chunk advisory.
- `git diff --check` — passed.

Build-generated `dist` changes were restored; generated assets are excluded.
No frontend behavior changed, so no new browser/PDF evidence is claimed.

## Standards

Independent review against user-approved baseline
`f61ac2fe218bce914ca6cf47e14fc96e404fda57`: no actionable findings through
`a3a575c`. Updated sequence accounting and final/union output handling follow
repository standards and preserve documented workflow/provider boundaries.

## Spec

Initial independent review identified two P2 gaps: initial clarification/correction
legs were omitted from full-workflow accounting, and failed-step assessment could
discard calls before returning. Both were reproduced, corrected, and re-reviewed
through `a3a575c`: no remaining actionable implementation findings. The matrix
was reduced to 14 cases and no extra repeats to keep all initial/answer and
continuation legs inside 60 HTTP attempts. Natural stable-prefix usage remains
observable without padding or retention changes.

The previous implementation review had Standards 0 findings and Spec 0 remaining
implementation findings. The live comparison, field review and decision are now
recorded, with a final review of the evidence before PR merge. Controlled success
is kept distinct from the live measurement throughout.

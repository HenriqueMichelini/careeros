# Issue #41 — Profile ingestion continuation

Branch: `codex/evidence-backed-profile`, milestone #3 Evidence-backed Profile.
Implementation starts from `def3256` after #40's merged claim-disposition work.

## Controlled evidence

The public ingestion HTTP handler runs with explicit provider transports, without
network or paid provider calls. Continuation regressions verify:

- Whole exact original classification and extraction limited to one selected
  portion, including repeated heading context and out-of-portion evidence rejection.
- Eighty claims and eighty proposed operations across bounded portions, exceeding
  the existing per-request ceilings while retaining exact original occurrences.
- Whole-submission attack rejection on initial and later portions, with zero
  extraction calls; malformed/truncated responses and deliberate retries never
  advance coverage; a returned thirty-claim ceiling remains unfinished.
- Existing preparation, evidence, conservative duplicate, clarification, grounding,
  schema, omitted-claim and bounded-provider tests remain applicable to each request.

Client contract tests bind continuation to the exact source identity and requested
portion, rejecting gaps, overlapping coverage, inconsistent capacity/completeness,
malformed ranges and wrong budgets/indices.

The controlled Chrome flow verifies EN/PT at 1440px and 390px: explicit first-portion
review/apply; preservation of the full paste after save; comparison of a later
portion with previously saved Java and stable duplicate references; a truncated
later portion retaining saved facts and input; deliberate retry of exactly that
unfinished index; review before saving Ruby; and cross-tab staleness invalidating
pending work while keeping the paste. No automatic request/apply occurs and no
horizontal overflow was observed.

Rendered pending continuation after the failed portion and successful retry:
[English](issue-41/continuation-en-390.png) ·
[Portuguese](issue-41/continuation-pt-390.png).

The broader ingestion browser regression also covers empty/populated Profiles,
onboarding/Quick Add, field rejection/rephrasing, exact original excerpts, explicit
approval, unrelated-data preservation, save failures and stale requests.

## Evidence boundary

This is controlled local evidence. It does not establish live-provider semantic
completeness, billing, latency, load reliability, or deployment status. Complete
planned source coverage is explicitly distinct from complete discovery of career
facts. No live key was read or used.

## Review corrections

The standards review found that a successfully processed portion of an incomplete
plan hid the smaller-budget restart control. The control now remains available
throughout a continuation session after reviewing the pending proposal. A browser
regression verifies `processed: true` with `planComplete: false`, explicit restart
without an automatic call, preservation of saved Java and the full paste, and the
next deliberate request using the smaller 1,000-byte budget in EN/PT at both widths.

The spec review requested spanning-fact verification. A controlled public-handler
regression now uses the real planner boundary: an earlier Acme/Engineer/2020–2021
heading remains supporting evidence for later Java responsibilities, retaining all
five grounded structured fields. An ambiguous “Its project” claim remains a
Clarification Request with zero attributed operations. No semantic completeness
claim is inferred from these controlled responses.

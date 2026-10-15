# Terminal outcome interpretation

Issue #145 introduces `terminal-outcomes-v1`. This is an additive summary
derivation version, separate from `ai-diagnostics-v1` and saved report versions.
Raw attempts, stage outcomes, fixture bytes and historical summaries are not
rewritten. `outcomes` retains the original diagnostic vocabulary;
`operationalOutcomes` contains the versioned interpretation.

| Raw terminal value | Operational category |
| --- | --- |
| `completed`, historical `complete` | completed |
| `request_information` | requested_information |
| `input_rejected`, `reject`, `irrelevant`, `rephrase`, `refused` | policy_rejection |
| `incomplete`, `incomplete_output`, `truncated` | incomplete |
| `canceled`, `cancelled` | canceled |
| `unavailable`, `assessment_unavailable` | assessment_unavailable |
| `transport_failure`, `provider_failure`, `timeout`, `outage` | infrastructure_failure |
| `malformed_output`, `rejected_output`, `invalid_output` | invalid_output |
| Any other value, including empty | unknown |

Current stage completion converts `complete` to `completed` without changing
Application Draft review states or prompts. `failed` and `failureRate` now count
infrastructure failures; other noncompletion categories are visible separately.
An expected rejection may pass a test assertion without completing a workflow.
Unknown outcomes never become failures or successes through HTTP status.

For legacy semantic results with **no diagnostics**, Compare preserves its
existing HTTP interpretation: 200 is operational completion, 5xx infrastructure
failure, other statuses unknown. This compatibility rule does not apply to an
explicit unknown diagnostic outcome. It supplies no semantic or acceptance
evidence. Comparison requires successful required children as well as the parent.

Sequence summaries count leaf diagnostic durations and flattened dispatch calls
once. New sequence results carry `requiredSteps`; absent required children are
incomplete and appear in `requiredUnreached`. Historical sequences lacking this
field increment `sequencesWithUnknownRequiredSteps`; their planned leg count
cannot be reconstructed from an observed child list alone.

Usage stays nullable. Cost per completed result uses the named configuration
cohort (or the explicitly supplied diagnostic attempt cohort): all observed
priced calls, including unsuccessful calls, divided by completed logical results.
It remains unavailable unless the full observed call total is priced. It is not
cost per accepted result. Acceptance requires explicit external evidence; neither
completion nor a curated score establishes semantic support, user acceptance or
durability. Required unreached legs have no invented dispatch or usage.

## Retained reproduction

The non-sequence subset of `workflow-settings/controlled.v1.json` contains 24
attempts: 18 raw `completed` and six raw `complete` (HTTP 200 reviewed Drafts).
Old Summarize: 18 completed / six failed. Version 1: 24 completed / zero
infrastructure failures. This is a subset of the 42 logical cases, not the whole
settings matrix and not live-provider evidence.

Original artifact SHA-256:
`46223f47959ce0e16f4c39b0b4c58bc376161ddbe3ac4b4ebb30e13e07a8d854`.
The live/reviewed reports have different outcomes and remain separate cohorts.

Tests exercise the retained subset through Summarize, current production-handler
Draft emission with controlled transport, unknown/rejection/cancellation/
unavailability categories, missing usage, accepted-cost accounting, and nested
sequence/unreached-leg accounting through Compare. No new provider calls or
production telemetry are involved.

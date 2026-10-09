# Deliberate Profile ingestion continuation

Issue #41 extends Add professional information and onboarding through their shared
Profile ingestion workflow. The original Field Submission remains in React memory
until explicit discard, a successful single-portion save, or the end of the page
session. Pending proposals, preparation maps, source identities and progress are
never written to storage. Only explicitly accepted facts and necessary excerpts
use the existing atomic Profile transaction.

The request may include `portion: {index, bytes}`. `bytes` is an integer from 200
to 2,000; the UI starts at 2,000. The server recomputes the shared #59 `Source.Plan`
against the exact original on every request, with a maximum of 256 portions and
the existing 256 KiB serialized extraction-envelope limit. Repeated heading
context counts against the working-text budget. There is no second planner,
background loop, validation token, automatic retry or model change. Existing
30,000-byte original, 160 KiB HTTP, 30-claim, 60-operation and provider/deadline
limits remain in force. Callers without `portion` keep the previous single-view
contract.

After transport/schema/Profile checks and mechanical planning, Jev classifies the
**complete exact original**, once per deliberate processing request. Rejection,
rephrasing or operational failure stops before extraction/comparison. Extraction
receives only the chosen prepared view, necessary heading context, and clipped
source segments. Evidence resolves against the complete original Source, retaining
its segment/occurrence identities. References outside the selected view and its
labeled context are withheld. Missing antecedents, periods and ownership require
clarification; preparation never supplies semantic identity.

`continuation` returns source identity, selected index/budget, all planned original
byte `regions`, any unplanned `remaining` regions, `planComplete` and `processed`.
Regions are half-open UTF-8 byte ranges. The client checks the complete source
identity, requested index/budget, ordered nonoverlapping coverage, input bounds,
plan completeness and consistency with claim/result limits. It separately tracks
planned, attempted and successfully processed portions in memory. Thirty returned
claims, sixty operations, invalid claims or unplaced operations leave a portion
unfinished. Truncation/malformed output returns an error and cannot advance
successful coverage. **Processed source coverage does not prove semantic claim
discovery is complete**; #40's per-claim dispositions remain separate.

Each response must be reviewed before another request. Save explicitly accepted
changes or deliberately dismiss the remaining proposals. Later portions compare
against the current Profile, including earlier saved facts and stable references;
unaccepted suggestions are not treated as approved evidence. Distinct details
and unresolved identity retain the existing conservative comparison and
clarification behavior. A retry cannot silently replay accepted operations.
Failures preserve earlier accepted facts, the original paste and local progress.
The user can retry the first unfinished portion or explicitly restart with a
smaller budget; restarting clears transient progress, preserving saved facts.
Indivisible blocks or necessary context that cannot fit remain unplanned and
visible. The user retains the source and can revise it deliberately.

Input changes cancel requests and invalidate all incompatible pending proposals
and progress. Profile edits and cross-tab stale-storage signals also invalidate
pending work while retaining the paste. Own successful atomic saves allow
continuation against the resulting Profile revision. Failed saves preserve the
proposal and paste for recovery. Cross-tab conflicts use the existing Profile
recovery flow before further processing.

Validation evidence is recorded in
[the issue #41 report](release-evidence/2026-10-09-profile-continuation.md).

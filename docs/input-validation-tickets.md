# Field-aware input validation ticket plan

Status: Approved by the user; published and verified on 2026-10-07.

Source: [agreed behavior](input-validation.md) and the design interview. Domain terms follow [CONTEXT.md](../CONTEXT.md).

## Publication target

- Repository: HenriqueMichelini/careeros.
- Milestone: [Field-aware input validation](https://github.com/HenriqueMichelini/careeros/milestone/1).
- Milestone description: Guided validation and recovery for Add professional information and Job Posting or Description. Accept useful messy input, reject entire submissions containing detected instruction attacks, request revised wording for uncertain attacks, and preserve unknown facts.
- No due date is set.
- Every new issue is open, labeled `ready-for-agent`, and linked to the milestone.
- Native GitHub blocking relationships match the approved dependency chain.

The 2026-10-07 read-only prepublication check found no existing milestone or duplicate validation queue. Existing migrated issues identify GitHub Issues as canonical despite the older local-tracker guide. The existing raw-ingestion issue #25 covers extraction completeness and review/apply; this queue adds field validation and recovery on top of the current implementation. Publication did not modify or close #25 or any other existing issue.

The feature spans frontend and backend. Ticket 1 is the verifiable provider-evaluation prerequisite; tickets 2–4 are end-to-end product slices; ticket 5 verifies their combined behavior. Product slices include the necessary contracts, server policy, user feedback, and meaningful checks rather than splitting those layers into separate tickets.

## Published tickets

1. [#29 — Evaluate field-validation providers against the agreed behavior](https://github.com/HenriqueMichelini/careeros/issues/29)
   - **Blocked by:** None.
   - **What it delivers:** A reproducible comparison of the existing OpenAI setup and Jev as a candidate, with a recommended provider, evidence for routing thresholds, and a typed field-decision contract for implementation.

2. [#30 — Validate professional information before Profile Proposal review](https://github.com/HenriqueMichelini/careeros/issues/30)
   - **Blocked by:** #29.
   - **What it delivers:** One useful professional fact can reach review despite ordinary noise; detected attacks block the whole submission; uncertain attacks require revision and validation. Includes actionable feedback, the supporting input-use rule, and preserved explicit apply.

3. [#31 — Validate Job Postings throughout the Apply flow](https://github.com/HenriqueMichelini/careeros/issues/31)
   - **Blocked by:** #30, which establishes the shared validation service, outcomes, and recovery feedback.
   - **What it delivers:** Messy postings and legitimate applicant instructions reach qualification checks and drafting, while attack-related decisions and requests for missing job context stop the appropriate processing. Both Apply paths enforce the policy on the server.

4. [#32 — Support short Job Postings without inventing missing details](https://github.com/HenriqueMichelini/careeros/issues/32)
   - **Blocked by:** #31.
   - **What it delivers:** A short posting such as “Java developer. AWS required.” works through Apply and results while missing company or other facts remain unknown; a title alone continues to require context.

5. [#33 — Verify field validation and recovery across both workflows](https://github.com/HenriqueMichelini/careeros/issues/33)
   - **Blocked by:** #32. Issues #29–#31 are transitive prerequisites.
   - **What it delivers:** Combined verification of accepted messy input, full blocking, rephrasing, clarification, failure recovery, unchanged saved Profile, and selected-provider behavior, with clear separation of controlled and live evidence.

## Publication verification

Read back all five issues and verified their approved bodies, open state, `ready-for-agent` label, and milestone assignment. Queried each issue's native blocking relationships separately and verified the milestone's exact five-issue membership, title, description, and absence of a due date.

The implementation frontier is #29. The approved native chain is #29 → #30 → #31 → #32 → #33, with each issue after #29 blocked by its predecessor.

GitHub is the published tracker. Temporary approved bodies, the publication manifest, and readback records remain in `/tmp/careeros-input-validation-ticket-drafts/`; no replacement local issue queue was created. No application implementation was performed as part of publication.

# 05 — OpenAI-only provider across all workflows

**What to build:** Use OpenAI as the sole provider for Profile review, qualification gap checks, and Application Draft generation. The user saves one OpenAI key for all three workflows. This reflects the approved change to “OpenAI only throughout”; older Anthropic language in this ticket and prior planning documents is superseded.

**Blocked by:** 04 — Application Draft generation through Go.

**Status:** Accepted (2026-10-04) on accumulated Ticket 01–04 implementation and acceptance evidence. This accepts the app-wide OpenAI provider implementation; it does not close the overall v1 release gates below.

- [x] All three workflows use their fixed Go handlers and the user's OpenAI key. The frontend stores one key, sends it only to the selected workflow, and has no provider/model selector or Anthropic fallback.
- [x] The Go backend uses fixed GPT-6 Luna with reasoning effort `none`. It exposes only the three fixed workflows, with no arbitrary relay, silent fallback, or automatic retry.
- [x] All three workflows share validated response and stable error behavior; invalid output cannot alter the Profile or display an incomplete Application Draft.
- [x] Go contract and failure tests cover all three workflows with simulated provider responses.
- [ ] Overall v1 release gate: complete the six real OpenAI provider/workflow checks—each of the three workflows in Figma Make preview and in the deployed app—with securely supplied keys. Record each synchronous duration and confirm it is below 60 seconds. These checks are not complete evidence for Ticket 05 acceptance; Figma Make is excluded from this ticket's acceptance.
- [ ] Overall v1 release gate: inspect representative generated outputs for completeness and unsupported factual claims.
- [ ] Overall v1 release gate: review actual deployed Netlify Function logs for metadata-only logging and confirm there is no backend storage or caching of user content or keys.

**Acceptance basis (2026-10-04):** Ticket 05 is accepted from the previously recorded implementation evidence: Ticket 02's real deployed Profile review; Ticket 03's corrected local OpenAI qualification-gap request, historical pull-request-preview keyless API checks, and mocked preview UI; Ticket 04's minimized local OpenAI draft request, current pull-request-preview keyless API checks, mocked preview UI, and Go contract/failure tests; and the completed app-wide implementation using one OpenAI key with three fixed Go handlers. This acceptance does not claim that all six real Figma Make/deployed workflow checks, representative output factual/completeness review, or actual deployed Function log/privacy review have been performed. Those remain overall v1 release gates. The implementation evidence includes mocked UI/API checks where stated; those mocks are not real-provider checks.

Any real OpenAI request for this ticket must use only approved minimized synthetic input and pass an exact outbound-payload preflight before the request. Never use a real Profile, expose a credential in source or logs, or send the vault key to Netlify. Local synthetic success evidence recorded for Tickets 03 and 04 is historical evidence for those checks only; it does not replace any of the six checks above. The separate overall v1 release gates also remain open until a provider-backed Netlify call and duration, actual Netlify Function log review, and factual-claim review are recorded.

## Comments

- 2026-10-04 — User approved accepting Ticket 05 based on accumulated Ticket 01–04 evidence. The six real Figma Make/deployed workflow checks with durations under 60 seconds, representative output factual/completeness review, and actual deployed Function log/privacy review remain explicit overall v1 release gates. No new provider call or credential use is authorized by this acceptance.

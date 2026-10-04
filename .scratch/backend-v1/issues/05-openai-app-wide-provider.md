# 05 — OpenAI-only provider across all workflows

**What to build:** Use OpenAI as the sole provider for Profile review, qualification gap checks, and Application Draft generation. The user saves one OpenAI key for all three workflows. This reflects the approved change to “OpenAI only throughout”; older Anthropic language in this ticket and prior planning documents is superseded.

**Blocked by:** 04 — Application Draft generation through Go.

**Status:** implementation present; live acceptance remains open.

- [x] All three workflows use their fixed Go handlers and the user's OpenAI key. The frontend stores one key, sends it only to the selected workflow, and has no provider/model selector or Anthropic fallback.
- [x] The Go backend uses fixed GPT-6 Luna with reasoning effort `none`. It exposes only the three fixed workflows, with no arbitrary relay, silent fallback, or automatic retry.
- [x] All three workflows share validated response and stable error behavior; invalid output cannot alter the Profile or display an incomplete Application Draft.
- [x] Go contract and failure tests cover all three workflows with simulated provider responses.
- [ ] All six OpenAI provider/workflow checks succeed: each of the three workflows in Figma Make preview and in the deployed app, with securely supplied keys. Record each synchronous duration and confirm it is below 60 seconds.
- [ ] Inspect representative outputs for completeness and unsupported factual claims; verify input limits, failure messaging, selected-key routing, metadata-only logs, and no backend storage or caching of user content or keys.

Any real OpenAI request for this ticket must use only approved minimized synthetic input and pass an exact outbound-payload preflight before the request. Never use a real Profile, expose a credential in source or logs, or send the vault key to Netlify. Local synthetic success evidence recorded for Tickets 03 and 04 is historical evidence for those checks only; it does not replace any of the six checks above. The separate overall v1 release gates also remain open until a provider-backed Netlify call and duration, actual Netlify Function log review, and factual-claim review are recorded.

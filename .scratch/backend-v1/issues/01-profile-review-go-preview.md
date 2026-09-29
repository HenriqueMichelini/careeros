# 01 — Profile review through Go in Figma Make preview

**What to build:** A user can review their Profile in the existing preview and receive an updated Profile and summary through a Go API using their Anthropic key. The current review screen keeps its behavior and applies only a valid response.

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] The preview sends the full Profile, including compensation, through a same-origin Go workflow and makes no direct browser-to-Anthropic review call.
- [ ] The Go workflow owns the review prompt, uses the fixed Claude Haiku 4.5 model without extended thinking, and makes at most one provider call per user action.
- [ ] Malformed or oversized inputs are rejected before the provider call; incomplete or invalid output cannot overwrite the saved Profile.
- [ ] The UI shows clear English and Portuguese errors for input, key, rate-limit, outage, timeout, and invalid-output failures; the request has a measured timeout suitable for the free runtime.
- [ ] Go contract and failure tests pass with a simulated provider, and one real Anthropic review produces a usable result in Figma Make preview.
- [ ] The backend stores no Profile or key, does not cache the response, and logs operational metadata only.

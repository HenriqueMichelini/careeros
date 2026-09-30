# 01 — Profile review through Go in Figma Make preview (OpenAI)

**What to build:** A user can review their Profile in the existing preview and receive an updated Profile and summary through a Go API using their OpenAI API key. The current review screen keeps its behavior and applies only a valid response. OpenAI is the only provider used in this Profile Review workflow; application generation and qualification-gap calls are deferred to their later OpenAI migration tickets.

**Blocked by:** None — can start immediately.

**Status:** implementation and live acceptance complete.

- [x] The preview sends the full Profile, including compensation, through a same-origin Go workflow and makes no direct browser-to-provider review call.
- [x] The Go workflow owns the review prompt, uses the fixed `gpt-6-luna` model with reasoning effort `none`, and makes at most one OpenAI API call per user action. OpenAI Docs list GPT-6 Luna as an API model supporting `v1/chat/completions`; the supplied account exposes `gpt-6-luna`.
- [x] Malformed or oversized inputs are rejected before the provider call; incomplete or invalid output cannot overwrite the saved Profile.
- [x] The UI shows clear English and Portuguese errors for input, key, rate-limit, outage, timeout, and invalid-output failures; the request has a measured timeout suitable for the free runtime. Go's 25-second timeout returned a usable result in 3,334 ms for the representative live request.
- [x] Go contract and failure tests pass with a simulated provider, and one real OpenAI review produces a usable result in Figma Make preview. The live same-origin request returned HTTP 200 in 3,334 ms with a complete Profile, preserved compensation and entry identities, and a non-empty summary.
- [x] The backend stores no Profile or key, does not cache the response, and logs operational metadata only.

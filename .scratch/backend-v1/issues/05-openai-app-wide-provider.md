# 05 — OpenAI as an app-wide provider option

**What to build:** A user can choose Anthropic or OpenAI once for the whole app, save one key for each provider across visits, and use the selected provider for Profile review, qualification gap checks, and Application Draft generation.

**Blocked by:** 04 — Application Draft generation through Go.

**Status:** ready-for-agent

- [ ] The app has one explicit active provider for all three workflows. Switching providers keeps the other provider's saved key; editing a key replaces its previous value; only the selected key is sent with a request.
- [ ] The Go backend uses fixed Claude Haiku 4.5 without extended thinking for Anthropic and fixed GPT-6 Luna with reasoning effort `none` for OpenAI. It exposes only the three fixed workflows, with no arbitrary relay, silent fallback, model picker, or automatic retry.
- [ ] Both providers return the same validated workflow responses and stable errors; invalid output cannot alter the Profile or display an incomplete Application Draft.
- [ ] Go contract and failure tests pass for both providers. All six live provider/workflow paths succeed in Figma Make preview and the deployed app using securely supplied keys, with each synchronous call meeting the 60-second limit.
- [ ] Inspect representative outputs for completeness and unsupported factual claims; verify input limits, failure messaging, selected-key routing, metadata-only logs, and no backend storage or caching of user content or keys.

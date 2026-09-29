# Keep the v1 backend stateless and use user-provided AI keys

V1 keeps profiles and application drafts in the browser while the backend handles only three fixed AI workflows with user-provided Anthropic or OpenAI keys. This avoids account, database, and shared AI billing work and supports free hosting. The trade-offs are device-local data, browser-persisted provider keys, and a future migration step if server-side storage is added in v2.

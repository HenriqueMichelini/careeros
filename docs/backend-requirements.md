# Backend v1 requirements

This document records the agreed design for a backend serving the existing CareerOS frontend. Domain terms are defined in [CONTEXT.md](../CONTEXT.md).

## Goal and scope

- Support the frontend's three existing AI workflows: profile review, qualification gap checks, and application draft generation.
- Keep the current product flow: no application account or sign-in, no server-side profile or application draft storage, and no history of generated drafts. Server-side storage is deferred to v2.
- Keep the profile in browser storage and the current application draft in application memory. All three workflows send the full profile to the selected provider, including employment and compensation fields, as the frontend does today.
- Preserve the current gap-confirmation flow: qualifications confirmed for one job opportunity may inform that application draft but do not modify the saved profile.

## Backend contract

- Expose only the three fixed workflows. The backend owns their prompts, provider calls, input checks, and response validation. It does not expose an arbitrary prompt or provider relay.
- Accept Anthropic and OpenAI API keys supplied by the user. One explicitly selected provider is active for the whole app, and all three workflows use it. The frontend sends only that provider's key with each request; the backend does not persist it.
- Return the data the existing screens need: an updated profile and summary from review; up to five potential qualification gaps from the gap check; and a job title, company, role summary, resume, cover letter, and application answers from draft generation.
- Reject malformed inputs before contacting a provider and reject incomplete or structurally invalid provider responses before changing the profile or displaying an application draft. Preserve stable error categories so the English and Portuguese UI can show clear messages for invalid input or key, provider rate limits or outages, timeout, and invalid output.
- Each workflow is one synchronous request with a bounded timeout. A failure returns an error; another billable provider call happens only after a deliberate user retry. There is no automatic retry, provider switch, background job, or progress tracker in v1.

## Keys and models

- The browser remembers one Anthropic key and one OpenAI key across visits. Editing a provider's key replaces its old saved value. Switching the active provider does not delete the other provider's key.
- Use one fixed model per provider, with no model picker. Cost is the priority: Claude Haiku 4.5 (`claude-haiku-4-5-20251001`) with extended thinking off, and GPT-6 Luna (`gpt-6-luna`) with reasoning effort `none`.
- AI provider charges, if any, belong to the user's provider account. V1 does not use an application-owned AI key or shared AI billing.

## Non-functional requirements

- **Platform cost:** Hosting must have no required recurring charge. Free-tier limits are acceptable and must not trigger automatic paid upgrades. If the free quota is exhausted, the UI shows a clear service-unavailable error.
- **Deployment:** The same API behavior must work in the running Figma Make preview and in the deployed app. The selected approach is Vite development middleware for the preview and Cloudflare Pages Functions for deployment, sharing the request handlers. The Vite frontend is deployed on Cloudflare Pages. No database is required for v1.
- **Privacy:** The backend does not store profiles, job postings, confirmed qualifications, application drafts, or provider keys. Application logs contain only operational metadata such as workflow, provider, status, and duration, never keys or user content. The deployed frontend-to-backend and backend-to-provider connections use HTTPS; responses containing user content are not cached by the application.
- **Cost and resource bounds:** The API enforces explicit profile and job-posting size limits before a provider call and reports an exceeded limit clearly. Numeric limits and timeout values are set from representative content and runtime measurements before acceptance.
- **Failure behavior:** The app never silently makes another provider call after a timeout, invalid output, or provider failure. It never silently switches to a more expensive model.

## Acceptance gates

- Validate all three workflows in the Figma Make preview and the deployed app with real Anthropic and OpenAI keys: six live provider paths in each environment. Keys must be supplied through a secure configuration or UI flow, never committed or pasted into source.
- Check the returned profile and application draft structures and inspect representative outputs for completeness and unsupported factual claims. Validate that confirmed qualifications remain specific to one draft.
- Exercise input-limit, invalid-key, provider-failure, malformed-output, timeout, and free-quota error paths without making unapproved repeat provider calls.
- Verify that neither application storage nor logs on the backend retain keys or user content. Confirm the deployed app works over HTTPS and remains within the free hosting plan.

## Current evidence and platform notes

- The frontend currently saves the profile and Anthropic key in `localStorage`, holds the generated draft in memory, and calls Anthropic from `src/lib/ai.ts` for the three workflows. It has no backend, database, or sign-in flow.
- As checked on 2026-09-29, Cloudflare lists static Pages requests as free and a shared Workers/Pages Functions Free quota of 100,000 function requests per day. [Pages Functions pricing](https://developers.cloudflare.com/pages/functions/pricing/), [Workers limits](https://developers.cloudflare.com/workers/platform/limits/).
- As checked on 2026-09-29, Claude Haiku 4.5 lists $1 per million input tokens and $5 per million output tokens and has no effort parameter. [Anthropic model page](https://platform.claude.com/docs/en/models/haiku-4-5/overview).
- As checked on 2026-09-29, GPT-6 Luna lists $0.10 per million input tokens and $0.50 per million output tokens and supports `reasoning.effort: "none"`. [Official OpenAI model page](https://developers.openai.com/api/docs/models/gpt-6-luna). Older GPT-5 nano has lower published rates, but its dated snapshot is scheduled for removal on 2026-12-11. [Official OpenAI model page](https://developers.openai.com/api/docs/models/gpt-5-nano), [deprecation schedule](https://developers.openai.com/api/docs/deprecations).

## Values to set during implementation

- Maximum profile and job-posting sizes, based on representative content and the free runtime's limits.
- Per-workflow timeout values, based on measured requests to both selected models.
- Exact route names and payload fields, while preserving the contract above and existing screen behavior.

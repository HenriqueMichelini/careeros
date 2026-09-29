# 02 — Deploy Profile review on Netlify Free

**What to build:** A user can open the deployed Vite app and complete the same Go-backed Profile review over HTTPS. The frontend and Go workflow run together on Netlify Free.

**Blocked by:** 01 — Profile review through Go in Figma Make preview.

**Status:** ready-for-agent

- [ ] The deployed app serves the existing frontend and exposes the Profile review Go workflow on the same origin without requiring sign-in, a database, or an application-owned AI key.
- [ ] A real Anthropic review succeeds on the deployed app and returns the same validated shape and error contract as preview.
- [ ] A representative live review completes within Netlify's 60-second synchronous Function limit; measure total request time and treat an exceeded limit as failed acceptance.
- [ ] Netlify Free remains selected with no automatic paid upgrade. Document the accepted whole-site pause when free credits are exhausted and surface a clear unavailable state when the app can still render.
- [ ] Check deployed transport, response caching, and logs: user content and keys are neither retained nor logged; only operational metadata is logged.

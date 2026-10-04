# Manual Profile review observations — 2026-10-04

Deploy in the Netlify Function dashboard: `6ac24a0d1f02f400084bb819`, revision `910d8ad9b1bc29937d2b6391f895345518094c1c`. The tester identified the [PR preview Function endpoint](https://deploy-preview-2--beamish-bavarois-333d03.netlify.app/.netlify/functions/profile-review) as a test target and reported a successful manual new-user flow in the deployed application, including Profile Review. This is tester-reported end-to-end evidence; no browser trace was retained. The Function log itself does not label each invocation's request origin, so neither entry is independently assigned to a specific origin here.

| Local time (GMT-3) | Netlify invocation | Handler result and duration | Platform execution, memory, initialization |
| --- | --- | --- | --- |
| 16:04:32 | `be0a9cac` | `status=200 outcome=ok duration_ms=4232` | 4,235.08 ms; 37 MB; 119.28 ms initialization |
| 16:11:24 | `3a2a3d03` | `status=200 outcome=ok duration_ms=9541` | 9,545.06 ms; 38 MB; 116.44 ms initialization |

These visible log entries contain operational metadata only; no credential, Profile, or generated text appears in them. Both Function executions were below 60 seconds. The handler returns `200 ok` only after its OpenAI call and result validation, so these entries support successful provider-backed handler invocations at this deploy. They do not expose the outbound payload or prove output quality.

**Informational limits for Ticket 01:** no separately recorded exact Go-to-OpenAI payload preflight, synthetic-fixture confirmation, generated-output inspection, per-origin browser trace, browser duration or `200` response headers, save behavior after reload, or provider-specific duration. The tester reported using the vault OpenAI test key rather than the dedicated-key procedure in the [release-test path](../release-test-path.md); no credential value is recorded here. Ticket 01 is accepted on the combined evidence and requires no additional provider call. These observations alone do not mark Ticket 02 or the broader provider-backed release gates complete.

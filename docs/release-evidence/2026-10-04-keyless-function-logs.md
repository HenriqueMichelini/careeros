# Keyless Function log review — 2026-10-04

Scope: read-only Netlify Functions dashboard review for `beamish-bavarois-333d03`, using the **Last day** filter (approximately 2026-10-03 15:02–2026-10-04 15:03 GMT-3). The dashboard states that Function logs are retained for 24 hours. The published and PR preview URLs currently point to deploy `6ac24a0d1f02f400084bb819`, revision `910d8ad9b1bc29937d2b6391f895345518094c1c`. The dashboard's Function detail page labels this published PR deploy as production and links to the preview origin; the visible log entries do not themselves identify which origin invoked the Function.

| Function | Visible handler outcomes | Visible platform execution durations | Privacy observation |
| --- | --- | --- | --- |
| `profile-review` | Three `401 key` entries; handler `duration_ms=0`. Entries at 14:53:04 and 14:53:37 GMT-3 align by time with the preview and published keyless checks, respectively. | 2.46, 3.71, and 1.86 ms; the first two show cold-start initialization separately. | Visible entries contain workflow, status, outcome, duration, invocation ID, memory and initialization metadata only. |
| `qualification-gaps` | One `401 key` entry; handler `duration_ms=0`. | 1.87 ms. | Same metadata-only pattern in the visible entry. |
| `application-draft` | Four `401 key` and two `400 input` handler entries; handler `duration_ms=0`. One additional platform duration line has no visible matching handler line, so its outcome is unknown. | 2.31, 2.29, 63.63, 2.51, 2.21, 1.78, and 2.50 ms. | Visible handler entries contain status, outcome and duration; no key, Profile, posting or generated content appears. |

This review covers only visible entries in the selected 24-hour window. There is no provider-backed invocation in those entries, so they cannot establish deployed OpenAI duration, generated-output quality, or privacy behavior during a provider-backed run. No raw logs, credentials, Profile text, posting text or generated content were copied into this evidence file. A successful provider-backed check and its own Function-log review remain open release gates.

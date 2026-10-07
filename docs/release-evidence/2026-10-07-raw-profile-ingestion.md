# Raw professional-information ingestion — issue #25

The current ingestion flow accepts mixed raw career information in the Profile overview and through Quick Add, including an empty Profile. English and Portuguese copy explicitly welcomes unpolished, repeated and disorganized input, describes extraction/consolidation and structured destinations, and explains review, editing, rejection and explicit apply.

Both client and server measure the raw input as UTF-8 bytes, accepting **up to 30,000 bytes inclusive**. The textarea retains oversized text intact and disables processing; no silent truncation occurs. Client and server contract tests use the same 30,000-byte Portuguese/emoji fixture and reject one additional byte. The request body remains bounded at 160 KiB. Each review is bounded at 30 claims and 60 operations; the UI always discloses these limits and that a large paste may require several portions. Invalid/unverified source claims, unsafe destinations, incomplete entries and unresolved conflicts are separately disclosed rather than presented as fully handled.

Contact details, education, certifications and languages now participate in extraction, comparison, validated field operations and atomic apply alongside the earlier destinations. Other AI workflows retain their original client projections and exact legacy JSON contracts. The comparison projection includes contact scalars only for their own destinations, and qualification collections only when targeted; education dates/locations and certification dates/credential IDs/URLs are withheld unless the source calls for them. Language names/proficiency support matching. Extraction receives raw user input, while comparison receives verified claims and relevant Profile fields. No persistence or provider retry was added; existing stateless user keys, metadata-only logs, bounded two-stage requests and no-store responses remain.

New entries require reviewed identity fields; existing entries use their stable IDs. Certification identity includes both name and issuer, so same-name credentials from distinct issuers are retained. Professional link additions preserve and deduplicate existing links; an explicit update replaces the field. Repeated new identities fail atomically instead of creating duplicate entries, and repeated text additions are suppressed. Unresolved comparison conflicts are withheld and identified for clarification. Explicit source corrections can be shown as reviewed updates. The complete resulting Profile is validated before saving; unrelated fields are preserved. Unapproved proposals, invalid selections and stale snapshots leave saved data unchanged. Raw input and proposals remain transient.

## Local validation

- `GOCACHE=/tmp/careeros-go-cache go test ./...` — passed.
- `node --test tests/*.test.mjs` — passed; ingestion now has 18 behavior cases, including all new destinations, stable identities, edits/rejection, repeat-entry suppression, invalid/incomplete atomic failure, stale protection and Unicode limits.
- `node_modules/.bin/tsc --noEmit` — passed.
- `node_modules/.bin/vite build` — passed (existing Vite native-config compatibility advisory).
- `node tests/ingestion-browser.check.mjs` — eight combinations passed: English/Portuguese × 1440/390px × empty/populated Profile. Uses only controlled responses; checks intact oversized input/disabled processing, a long repeated/disorganized paste, source/destination display, ambiguity and partial notices, unchecked approval, edit/reject/approve, no saved writes before apply, all destinations, unrelated saved IDs, reload after apply and cleared transient input. No horizontal overflow; Portuguese 390px review screenshot inspected.

Go fixtures inspect synthetic outbound comparison data and controlled response operations for destination accuracy, relevant dates/proficiency, omission of unrelated private fields, and withholding of conflicting/incomplete groups. These validate contracts and safety behavior. They do **not** establish a real model's extraction quality, factual fidelity or semantic consolidation on arbitrary raw text. No live provider request, deployment or publication was performed for this ticket.

The browser regression writes disposable screenshots under its printed `/tmp/careeros-ingestion-browser-*` directory and uses isolated local Vite/Chrome processes. It is available as `npm run test:ingestion-browser`.

## Review

Sequential fresh Standards and Spec reviewers inspected the issue #25 diff. Standards reported no actionable findings. Spec identified professional-link replacement during additions and certification identity omitting issuer; both were fixed with behavior regressions and the reviewer confirmed no remaining findings in the focused recheck.

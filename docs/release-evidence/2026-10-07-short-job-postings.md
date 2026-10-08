# Short Job Postings (#32)

The starting commit is `771252f` (#31). That implementation already removed the minimum character-count shortcut and validates Job Postings at both HTTP endpoints, including title-only clarification and attack/rephrasing decisions. Issue #32 completes the nullable draft metadata contract.

`jobTitle` and `company` are required JSON keys whose values are non-empty strings or `null` (unknown). Missing keys, blank strings and other types remain invalid. The provider schema, Go validation and TypeScript consumption agree on this contract. Results uses localized “Not provided” labels for unknowns; the title falls back to the existing Application Materials heading. These presentation labels do not enter draft prose, preview or print content. The generation instructions explicitly prohibit inferring the hiring company from candidate employment history and fabricating missing facts in any material.

## Verification

- `GOCACHE=/tmp/careeros-go-cache go test ./...`: passes.
- `node_modules/.bin/tsc --noEmit`: passes.
- `node_modules/.bin/vite build`: passes.
- `node tests/apply-checklist.check.mjs`: controlled browser checks pass in English and Brazilian Portuguese at 1440px and 390px. Covers “Java developer. AWS required.” through Apply, qualification review, Results, résumé preview and both print paths; nullable company/title; title-only clarification; unchanged saved Profile; and supplied metadata on a complete posting. Existing decision, recovery, changed-input and confirmation checks also pass.
- `node --test tests/*.test.mjs`: five files pass; `cv-preferences.test.mjs` fails because it expects 12px while `CV_FONT_DEFAULT` is 14px. Both conflicting values are present at the starting commit; this unrelated default is unchanged.

HTTP checks use controlled classifier and generation responses. Browser checks use controlled HTTP responses and inspect the print content handed to `window.print`; they do not validate an actual browser-produced PDF. No live TypeSafe/OpenAI call or deployed verification was performed, so these checks do not establish live classifier accuracy or absence of provider hallucinations. They verify that known requirements and explicit unknowns survive the application without being replaced with invented display/export facts.

# Focused browser-local journey — controlled evidence, 24 October 2026

Scope: [implementation #267](https://github.com/HenriqueMichelini/careeros/issues/267), using the retained-screen direction from [#255](https://github.com/HenriqueMichelini/careeros/issues/255#issuecomment-6380496863). Source baseline: `3c6a6e7f33a6e58a59876ecf4d71d7abb7804b46`; candidate source and tests are the changes in this PR on `codex/focused-journey-setup`.

The owner explicitly deferred provider selection on 24 October and requested completion of the independent journey work. No identity, storage, authentication, payment or inference provider is selected by this change. The existing browser-local BYOK flow remains the implemented product boundary.

## Implemented

- All three landing actions open Profile for empty/contact-only, saving or unavailable Profile state; saved usable professional evidence opens Apply. Standalone CV generation is not a prerequisite.
- Retained Profile/CV/Apply/Results show concise EN/PT context for local storage, optional standalone CV, transient Drafts and review/export. A saved Profile offers a keyboard-accessible Continue to Apply action.
- Shared Profile writes expose pending state. Apply cannot treat an optimistic edit as saved or start analysis/Draft generation while saving or after a persistence failure. Both post-analysis generation controls obey that gate, and a newly unavailable Profile invalidates in-flight client work.
- Existing explicit proposal Save, artifact Accept, conflict recovery and export boundaries remain. This is a client guard for the free comparator, not tenant authorization or allowance enforcement.
- Navigation names the current page, focuses the workspace and avoids guidance being covered by the sticky header. The key input has an associated label; the selected Profile item uses white text on the existing accent. Journey guidance is excluded from standalone CV printing.

## Controlled-pass evidence

All browser fixtures use fresh temporary Chrome profiles, synthetic Profile data, inert keys and controlled responses. No live inference, real personal-data access, payment/account setup or deployment was performed.

| Check | Result and scope |
| --- | --- |
| `GOCACHE=/tmp/careeros-go-cache npm test` | 306/306 offline tests passed, including existing controlled handler/client integration and persistence/ingestion contracts. |
| `node_modules/.bin/tsc --noEmit` | Passed. |
| `npm run build` | Passed. Reports the Vite config-loader and large-chunk warnings; this work does not resolve those warnings. Generated build/Vite files are excluded from the commit. |
| `npm run test:apply-checklist` | Passed EN/PT × 1440/390px. Includes the new routing/persistence/focus cases plus retained Apply/enrichment behavior. |
| `npm run test:apply-checklist -- --focused-journey` | Passed EN/PT × 1440/390px for focused iteration. The full command includes these cases by default. |
| `npm run test:ingestion-browser` | Existing integrated empty/populated, duplicate, stop/resume, partial failure, explicit Save/retry, clarification, stale review and EN/PT desktop/mobile keyboard cases passed. This reuses #194's work; it does not close #194. |
| `node tests/resume-review-browser.check.mjs` | Existing EN/PT × 1440/390px artifact review, acceptance, currentness, copy/PDF, overflow and failed-new-Draft preservation checks passed. |
| `npm run test:cv-pdf` | Nine actual local A4 PDFs passed text/order/sample exclusion checks; guidance is excluded. Blank Profile, font persistence and narrow-layout overflow blocking passed. |
| `CV_PREVIEW_KEEP=1 npm run test:cv-preview` | Existing native Enter/Space/pointer clipboard checks, denied/unavailable clipboard recovery cases, four actual A4 PDFs, full Profile preservation and synthetic HTTP capture passed. |

The new focused cases cover all landing actions with empty, contact-only and professional Profiles; no mandatory saved CV; a held Profile lock; quota failure with unchanged saved bytes; explicit reload recovery; a committed save with a lost lock acknowledgment; and stale storage while analysis awaits confirmation. Pending/failed states dispatch no generation. Keyboard activation moves focus to the workspace. The new guidance and selected Profile text meet the selected 4.5:1 contrast assertions, and the inspected 390px/desktop layouts fit the viewport. These are selected checks, not whole-app WCAG or human-usability conformance.

Retained synthetic screenshots: [saved Profile, EN desktop](focused-journey-267/profile-en-1440.png) and [save failure, PT 390px](focused-journey-267/save-failed-pt-BR-390.png). Visual inspection confirmed the corrected header/guidance positioning. Actual OS save-dialog completion is not inferred from invoking print.

## Failed observations and resolution

- An initial sandboxed unit run failed on local Go process permissions. The authorized local-process rerun passed all 306 tests.
- The formatter removed a separator from an inline TypeScript object type. The type was expanded to one field per line and TypeScript passed.
- Initial browser fixtures assumed every landing action opened Apply, awaited a deliberately held lock, or automatically advanced a confirmation fixture. The selectors and fixture control were corrected without weakening the product gates.
- Visual QA found focus scrolling guidance behind the sticky header. Navigation now focuses without implicit scrolling and resets position before paint. The test asserts visible guidance below the header; it permits harmless font/layout scroll anchoring rather than imposing an exact pixel scroll offset.

No known product/test failure from this scope is being waived.

## Unavailable and deliberately unresolved

| Contract/evidence | Responsible work and present boundary |
| --- | --- |
| Trusted account/session ownership and two-account identifier-substitution isolation | #259/#262: no selected identity/storage target; no implemented account authority. Provider selection is deferred. |
| Deliberate declined/repeated/interrupted/shared-device local-to-account claiming | #264 with #262/#259/#258: transfer scope, account binding, source authority/durability, conflict and logout/cache rules remain unresolved. No local data is uploaded or claimed. |
| Durable Draft/result recovery, missing or withheld acknowledgment, cancellation races, deadline closure, crash/restore and atomic allowance effects | #260/#259/#256/#261: authority/transaction/dispatch/restore protocol remains unselected. Browser flags, HTTP success, Accept and local counters provide none of this evidence. |
| Numerical pending deadline, clock/start/reset semantics, attempt/spend/capacity limits and safe unknown-exposure release | #256/#261 with #260/#265: no values or policies are invented. Customer closure must remain separate from unresolved provider expense/capacity. |
| Non-Draft rights, unused rights, defects, refunds/remedies, retention/access end and account deletion | #265/#260 with #258/#257/#262/#264: material commercial/lifecycle inputs remain open. |
| Current provider quality, billed expense, real-user/assistive-technology usability, deployed candidate and release readiness | Not established by these controlled checks. #266/#52 retain their verification/release responsibilities. |

[S06-D01–D05](https://github.com/HenriqueMichelini/careeros/issues/255#issuecomment-6380301147) remain the approved working functional definitions, not an implemented paid protocol. [#256 G05](https://github.com/HenriqueMichelini/careeros/issues/256#issuecomment-6376839141) remains open. No deployment, spending, provider-account setup, live inference, customer contact or launch is authorized or performed by this work.

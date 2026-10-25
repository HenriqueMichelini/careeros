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


## SonarCloud duplication follow-up — 24 October 2026

The first analysis of PR #268 at `cc0f9c88d1d84f67cd998dbc393b34b986d0bd1b` failed with 7.4% new-code duplication (43 duplicated lines / 579 new lines; limit ≤3%). SonarCloud's per-file measures attributed all 43 lines to `src/lib/i18n.ts`: the new bilingual journey dictionaries and five formatting-only changes within existing dictionaries.

The follow-up moves the 13 journey messages per locale into `src/lib/journeyMessages.json`, using the existing `qualificationMessages.json` catalogue/import pattern. It restores those five unrelated formatting changes, so the TypeScript diff against the PR base contains only one import and two catalogue spreads. Translation keys, values, fallback/interpolation logic and the journey behavior are preserved. No Sonar exclusions or quality-gate configuration were changed.

Fresh controlled verification of the follow-up:

| Check | Result |
| --- | --- |
| One-off TypeScript-transpiled comparison against `cc0f9c8` | All 1,730 locale/key pairs, complete dictionaries and translated/interpolated outputs unchanged, including the 26 journey messages. An initial sandbox invocation could not spawn Git (`EPERM`); reading the pinned baseline into a temporary file separately removed that environment limitation and the comparison passed. |
| `GOCACHE=/tmp/careeros-go-cache npm test` | 306/306 passed. |
| `node_modules/.bin/tsc --noEmit` | Passed. |
| `npm run build` | Passed; existing config-loader and large-chunk warnings retained. Generated dist/Vite files excluded. |
| `npm run test:apply-checklist` | Full Apply/enrichment/focused-journey matrix passed: EN/PT × 1440/390px. |
| `npm run test:cv-pdf` | Nine one-page A4 PDFs passed selectable-text/order/sample/guidance exclusion checks; 390px overflow blocking also passed. |

Remote gate acceptance must be read against the follow-up commit in the PR discussion; these local results alone do not establish a SonarCloud pass.

### D11: automatic deployment prevention before the follow-up push

The owner explicitly requested resolving D11 before publication. Netlify project `beamish-bavarois-333d03` (ID `d6d15958-3a27-4b4e-b233-d7a947f03ff2`) is linked to `HenriqueMichelini/careeros`; previews were enabled for pull requests against `main`. Its prior auto-publishing lock did not prevent the first preview. Before this push, Build status was changed from Active to **Stopped**, saved, and independently read back after reloading [Developer settings / Build settings](https://app.netlify.com/projects/beamish-bavarois-333d03/configuration/developer-settings#build-settings). [Retained saved-setting screenshot](focused-journey-267/netlify-builds-stopped.jpg).

Netlify's [Stop or activate builds documentation](https://docs.netlify.com/build/configure-builds/stop-or-activate-builds/) states that stopped builds prevent production, Deploy Preview and branch builds triggered by Git, plus build-hook/UI/build-API builds. The setting therefore covers the requested push to `codex/focused-journey-setup` for PR #268 against `main`. Builds are left stopped. Manual deployment of a local build remains possible and is not attempted; reactivation or repository relinking requires a fresh prevention check before later publication. No deployment retry, live inference or account setup was performed. Billing effects of the earlier canceled preview remain unavailable.

The ownership/import contract stays a local draft with material decisions pending #259 and the owning investigations. This independent gate repair does not implement or approve paid ownership/import, recovery, allowance authority, merge, deployment or launch; #259, #210 and the existing investigation/release gates remain open.

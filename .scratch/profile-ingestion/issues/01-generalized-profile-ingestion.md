# 01 — Ingest unstructured professional information into the Profile

**What to build:** Let a new user paste a substantial account of their professional life, or an existing user use Quick Add, then review and explicitly apply structured Profile changes proposed from that text. The user should not need to classify each claim into a Profile section first.

**Blocked by:** `.scratch/backend-v1/issues/05-openai-app-wide-provider.md` — accepted on 2026-10-04. This is a separate post–Ticket 05 product feature, not part of backend v1 or a retroactive v1 release gate. The outstanding backend-v1 release checks retain their own scope.

**Status:** ready-for-agent (2026-10-04). Product ticket only; implementation has not begun.

## Why

Manual new-user testing found that the existing Profile workflow works, but filling each structured section creates onboarding friction. The Profile schema should supply a reliable professional knowledge base without making users perform the classification themselves. Manual structured editing remains available.

## Workflow and scope

`Unstructured professional information → distinct claim extraction → schema classification → comparison with current Profile → proposed changes → user review/edit/reject/approve → explicit apply to browser-stored Profile`

Support both a large initial paste and a short Quick Add. Input may mix experience, responsibilities, achievements, skills, competencies, tools, education, projects, goals, employment and compensation, certifications, domain knowledge, and other relevant facts in any order. Map these to the existing schema (`careerGoals`, `skills`, `competencies`, `experience[]`, `tools`, `projects[]`, `employmentStatus`, `currentSalary`, `desiredSalary`, `additionalInfo`). Education and certifications currently belong in `additionalInfo`; do not invent new persisted sections as part of this ticket without a separate schema decision.

## Acceptance criteria

- [ ] A user can enter or paste free-form professional information from the Profile entry point during onboarding and from a clearly available Quick Add path later. Empty, oversized, and unsupported input receive useful errors. Existing section editors remain functional.
- [ ] The system extracts **distinct claims**, with source excerpts or equivalent traceable evidence, and maps each supported claim to one or more Profile fields. It does not substitute a prose summary for extraction or invent missing employers, dates, qualifications, salary, or project outcomes. Ambiguous or unsupported claims are shown for clarification or left unapplied.
- [ ] Before proposing changes, compare claims with the current Profile across sections. Classify exact/semantic duplicates, overlaps, contradictions, and additions that belong inside an existing experience/project entry. A repeated Quick Add does not create duplicate entries or repeated free-text facts. Show conflicts rather than resolving them silently.
- [ ] A claim can produce coordinated changes in multiple sections when justified (for example, an achievement in an experience entry and a related skill). Display the linked changes together so the user can accept or reject them coherently. Existing unrelated facts remain intact.
- [ ] Present field-level additions, updates, and any proposed removals as a readable before/after diff with destination section, affected entry, and source claim. **No Profile write occurs during extraction, classification, comparison, or proposal rendering.** No change is preapproved merely because the AI suggested it.
- [ ] Users can approve, reject, or edit each proposed change before applying; explicit confirmation applies only the approved, edited set. A cancel, provider error, malformed response, or rejected proposal leaves the saved Profile unchanged. A removal requires explicit review and confirmation.
- [ ] Applying changes validates the final complete Profile, preserves existing fields, valid employment-status values, experience/project identities and list constraints, and creates new stable IDs only for genuinely new entries. If the Profile changed after proposals were generated, recompare or require regeneration before apply; never overwrite newer edits with stale proposals.
- [ ] The flow works with an initially empty Profile and with a populated Profile. Absence of employment information must not be interpreted as a claim of full-time employment; address the current `employed-full-time` default when presenting or applying onboarding proposals, with a safe decision for existing saved data.
- [ ] The UI is usable in English and Portuguese and at a 390px viewport, including long pastes, multiple proposals, conflicts, edit controls, and confirmation. The UI locale must not rewrite stored professional content.
- [ ] Only the input needed to extract/classify claims and the minimum relevant Profile projection needed for comparison are sent to OpenAI. Define and test the projection explicitly. Dedicated sensitive fields (especially compensation, employment status, names, IDs, URLs, and location) are included only when needed for a corresponding claim or conflict check; unrelated sections are excluded. Explain the outbound data to the user before submission and allow editing/removing pasted sensitive content. Do not reuse the qualification-gap allowlist as a claim that all inputs are anonymized: user-authored free text can itself contain sensitive data.
- [ ] Retain the current stateless, user-key model: no server-side Profile/input/proposal/key storage, no content or key in logs, no application caching of responses, and no automatic provider retry. Browser persistence occurs only through the existing Profile store after explicit apply. Define a discard policy for unsubmitted text and unapplied proposals.
- [ ] Server and client reject invalid inputs, unknown fields/operations, unsafe targets, oversized results, and structurally invalid provider output before any write. Preserve localized error categories and the Netlify synchronous runtime limit.

## Architecture and affected components

- `src/pages/RepositoryPage.tsx`: add an honest, accessible entry and proposal review flow while preserving manual section editing. Reuse its visual conventions, not the old inert paste control.
- `src/lib/types.ts`, `src/lib/store.tsx`, `src/lib/ai.ts`, `src/lib/i18n.ts`: define typed claims/operations and transient proposal state, perform guarded apply through the existing `SET_REPO` path, and localize visible copy. The saved Profile remains `careeros_repo`; raw input and proposals are not silently persisted.
- Add a dedicated fixed Go ingestion workflow and wire its local Vite proxy, `cmd/` route, Netlify Function, and `netlify.toml` redirect. Reuse the existing OpenAI HTTP, bounded-input, error, `no-store`, and metadata-only logging patterns where suitable. Do **not** route ingestion through Profile Review: that handler receives the full Profile, rewrites it for tone, preserves list lengths/IDs, and its UI currently saves the returned Profile immediately.
- `internal/profilevalidation/profile.go`: reuse or extend complete-Profile validation for the apply boundary, with explicit checks for patch targets, operation semantics, and stale state. Decide the client/server validation split before coding so the provider response is never treated as an authoritative replacement Profile.
- Keep a compact proposal/patch contract rather than accepting an unconstrained AI-authored complete Profile. Link every operation to an extracted claim, target path/entry ID, and comparison finding; validate edited operations again before apply.

## Earlier related work

- Commit `5f424b3` added a five-row “Paste Your Professional Information” field to `RepositoryPage`. Its text lived only in component state and the UI said AI-assisted filling was “coming soon”; it had no extraction, persistence, or apply behavior.
- Commit `6e5f7a1` removed that field when the new Profile landing section was introduced. The earlier UI clarity request explicitly called for removing the nonfunctional pasted-info field and avoiding AI-assisted filling in that change. The removal was a scope/UX cleanup, not evidence that general-purpose ingestion was rejected as a product direction.
- The current `additionalInfo` field is a manual catch-all for education, certifications, languages, etc.; it does not classify claims or update other sections. Existing Profile Review improves text and directly saves a complete returned Profile; it is not a safe ingestion or approval workflow. Reuse only appropriate form, provider, and validation patterns.

## Verification required

- [ ] Unit/contract tests cover claim-to-field mapping, multi-section proposals, duplicate and overlapping facts, conflict surfacing, in-place experience/project updates, edited/rejected proposals, explicit removal, stable IDs, stale-Profile protection, invalid provider output, and no-write paths.
- [ ] Privacy tests inspect the exact Go-to-OpenAI payload for onboarding and Quick Add examples, including compensation and unrelated sensitive Profile fields; verify metadata-only logs and `no-store` responses. Use approved synthetic inputs and a safely supplied test key for any real provider check.
- [ ] Browser checks at desktop and 390px cover empty and populated Profiles, long paste, review/edit/reject/approve, cancel, reload after confirmed apply, and unchanged Profile after error or cancel. Confirm manual editing and existing Profile Review still work.
- [ ] Run TypeScript/build and Go tests. Validate at least one realistic synthetic end-to-end ingestion request through the supported deployment path, recording duration, output quality, and any Netlify 60-second limit issue without treating mock responses as provider-backed evidence.

## Dependencies and decisions before implementation

- Ticket 05's OpenAI-only fixed-handler baseline is accepted. This feature adds a fourth fixed workflow **after** that baseline; it does not alter Tickets 01–05 or their acceptance.
- Specify the minimal comparison projection and the exact sensitive-field inclusion rules before implementing provider calls. Resolve whether extraction and comparison use one bounded call or stages without sending unrelated Profile content.
- Specify the patch contract and atomic apply validation, including treatment of the existing employment-status default, before UI wiring.

## Comments

- 2026-10-04 — Discovered during manual new-user testing. Ticket drafted after inspecting current code, backend requirements, ADRs, and the earlier paste-field history. No implementation requested or started.

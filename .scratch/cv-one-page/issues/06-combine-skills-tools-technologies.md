# 06 — Combine skills, tools, and technologies in one CV section

**What to build:** Present the candidate's skills, tools, and technologies together under one clear CV section titled **Skills, Tools & Technologies** (or an equivalent localized label), instead of splitting them between “Technical Skills” and “Tools & Technology.”

**Blocked by:** 04 — Help users curate a one-page CV; 05 — Export and verify.

**Status:** ready-for-agent

## Why

The current CV language separates closely related technical qualifications across two sections. Combining them makes the candidate's capabilities easier to scan and avoids implying that tools and technologies are separate from skills. “Skills, Tools & Technologies” is clearer and more natural than “techs.”

## Acceptance criteria

- [ ] The CV preview renders one combined section for supported Profile skills, competencies, tools, and project technologies where those facts are included by the user's CV choices. It does not render duplicate or competing skills/tools sections.
- [ ] The combined section has a consistent English label, **Skills, Tools & Technologies**, and an accurate Portuguese translation. User-authored values remain unchanged.
- [ ] Empty values are omitted, existing facts are preserved, and the section does not invent, deduplicate away, or silently reclassify Profile content.
- [ ] Generated resume output and the rendered CV preview use the same combined-section meaning and remain parseable by the existing resume preview/import path.
- [ ] The section remains readable and page-fit behavior remains honest at desktop and narrow viewport sizes, including Profiles with only skills, only tools/technologies, or all supported categories populated.
- [ ] Existing browser-saved Profiles, CV choices, legacy generated resumes, and unrelated CV sections continue to load safely.

## Scope notes

- Keep this ticket focused on CV presentation and section naming. Do not change the persisted Profile schema or merge the underlying `skills`, `competencies`, `tools`, or project `technologies` fields.
- Decide and document the display order and duplicate-handling rule before implementation. A repeated technology may be omitted from the combined display when it is an exact duplicate, but the source Profile facts must remain intact.

## Verification required

- [ ] Add or update unit/contract coverage for combined-section composition, exact duplicate handling, localized labels, and legacy resume parsing.
- [ ] Verify the browser preview with empty, partial, and fully populated inputs at desktop and 390px widths, including English and Portuguese.
- [ ] Run the TypeScript/build checks and the existing CV preview/PDF checks.

## Comments

- 2026-10-06 — Ticket requested to combine skills with tools and technologies under one clearer CV section.

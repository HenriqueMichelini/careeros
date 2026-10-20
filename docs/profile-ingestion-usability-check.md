# Profile ingestion task check (issue #193)

This is a short, task-based usability check for the composed Profile ingestion
journey. It is used to verify that a person can identify **what changes**, **which
Profile destination receives it**, and **how to save only the selected changes**
without reading the internal processing ledger.

The check covers empty and populated Profiles, many grouped changes, long values
and sources, duplicates, conflicts, answer editing, processing failure, save
failure, locale changes, keyboard operation and discard.

## Boundary

- Controlled browser evidence only: Chromium driven through the DevTools
  protocol, synthetic provider responses, and browser-local test storage.
- No live provider, deployment, background job, database, remote telemetry or
  canonical user-Profile write is used.
- This is not a WCAG conformance audit. Human observation and real-device
  evidence are not claimed.

## Scripted tasks

The controlled suite runs these tasks with synthetic responses. The integrated
EN/PT x 1440/390 matrix repeats the rich review, locale, save-failure, processing
failure, keyboard and discard checks, verifies the empty state's presentation at
each size, and performs one empty-Profile save per cell. The baseline scenario
owns the full empty-Profile save-failure and retry path.

1. **Empty Profile:** open **Add professional information**, paste a short
   statement, prepare it, select the proposed change, save, and retry a forced
   save failure.
2. **Populated Profile:** open the same journey, confirm the existing field is
   shown as the current value, review a rich submission, and save.
3. **Rich review:** identify a skills addition, career-goals and competencies
   replacements, a tools addition, a duplicate, a conflict, and a clarification.
   Expand a long value and its source without losing the destination heading.
4. **Clarification:** expand the clarification row, answer **What is your French
   level?**, submit, and confirm focus moves to the resulting destination row
   rather than the review heading.
5. **Recovery:** start processing, stop and resume it, force a batch failure,
   inspect the preserved pasted text, retry processing, and discard.
6. **Save failure:** force the first canonical write to fail, focus the alert,
   retry save, and confirm one successful write with no additional provider call.
7. **Locale and state:** select changes, edit wording, open and close a source,
   switch language mid-review, and switch back. Selected changes, edited wording,
   and source-language excerpts must remain.
8. **Keyboard:** use Enter, Space, Tab and Escape for group selection, Edit,
   Source, clarification, Save, Retry save, Stop/Resume/Retry processing and the
   discard dialog. Every focus target records its visible focus state in the
   keyboard trace.

## Pass conditions

- **What and where:** proposed values appear before current values and sit under
  an explicit destination heading; entry subfields stay with their entry.
- **Selection:** nothing is preselected. Conflicts and other attention-only rows
  cannot be selected by group or bulk-additions selection.
- **Narrow layout:** one destination column at 390px; current and proposed values
  stack; no document, journey, review or destination horizontal overflow; action
  footer stays in normal flow below the review.
- **Touch and disclosure:** visible interactive targets are at least 24px in each
  dimension; native disclosure markers remain; expanded sources retain their
  destination.
- **Keyboard and focus:** selection, group toggles, Edit, Source, clarification,
  Stop/Resume/Retry, Save, Retry save and Discard are reachable by keyboard with
  visible focus. Focus moves to the review heading after processing, the recovery
  status after failure, the alert after save failure, the saved status after save,
  and the resolved destination after a clarification.
- **Locale:** switching locale preserves selection, edits, source excerpts and
  the clarification question. No internal enum such as `validated_operation`,
  `verified_exact_alias_or_wording`, `source_ambiguity`, `exact_duplicate`,
  `in_place` or `addition` appears as visible review copy.

## Observed evidence

Scripted evidence collected on 2026-10-20 from the controlled browser run:

- `node tests/submission-journey.check.mjs` — 7 baseline scenarios plus 8
  EN/PT x 1440/390 integrated scenarios passed.
- `node tests/review-renderer.check.mjs` — mixed, duplicate-only, empty and
  focused-clarification states passed.
- The scripted assertions cover overflow, single-column layout, stacked values,
  target sizes, disclosure markers, keyboard focus/names, locale preservation,
  save failure/retry, processing failure/resume, and zero writes before save.

Screenshots for the empty, rich review, processing failure, failure review, save
failure and saved states, plus the keyboard trace, are retained in
`docs/release-evidence/profile-review-193/` for English and Portuguese at
1440px and 390px.

## Human and device evidence

- **Human usability session:** not performed — **unavailable** for this ticket.
- **Human-observed feedback:** none recorded; no participant comments, task
  success or preference data are claimed.
- **Real mobile device / physical touch:** not performed — **unavailable**.
- **On-screen keyboard:** not exercised — **unavailable**; the action footer is
  asserted to remain in normal document flow, not overlaid above the keyboard.
- **Screen reader / assistive technology:** not performed — **unavailable** for
  NVDA, JAWS, VoiceOver and TalkBack.
- **Browser/device coverage:** Chromium headless through the controlled harness
  only.

These unavailable checks must be repeated with a person and the target assistive
technology/device before making a human usability, accessibility-conformance or
release claim. Automated checks prove only the scripted behavior above; they do
not prove semantic quality, task comprehension, or screen-reader output.

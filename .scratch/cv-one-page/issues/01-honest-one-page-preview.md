# 01 — Make the CV preview an honest one-page document

**What to build:** Show a readable, single-column A4 CV preview with the same broad section sequence as the supplied reference: identity and contact, summary, skills, experience, education, then relevant additional qualifications. One page is the default, and the preview makes excess content visible to the user.

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] The preview has a real A4 page boundary and a single reading column at desktop and narrow viewports; its text remains readable without shrinking the entire document to hide overflow.
- [ ] The page-count description says one page by default and reflects the actual preview state. Content that exceeds the page produces a clear overflow indication rather than disappearing behind the page boundary.
- [ ] All Profile experience, skills, tools, and projects selected by the current preview remain visible or are explicitly identified as omitted; fixed item-count slices no longer silently discard facts.
- [ ] Missing Profile information stays clearly marked as sample content in the preview. The UI locale changes labels, not saved professional text.
- [ ] Verify the page boundary, reading order, overflow state, and responsive layout with both short and deliberately long Profile fixtures in English and Portuguese.


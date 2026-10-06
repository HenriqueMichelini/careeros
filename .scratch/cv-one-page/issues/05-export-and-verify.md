# 05 — Export and verify the one-page CV

**What to build:** Let a user export their curated CV as an A4 PDF that is readable by people and has extractable text in the intended reading order.

**Blocked by:** 04 — Help users curate a one-page CV.

**Status:** ready-for-agent

- [ ] Export produces one A4 page when the CV fits. If it does not fit, the user receives a clear overflow result and can revise the CV before export; no content is clipped or silently dropped.
- [ ] The exported document uses a single reading column, clear section headings, selectable text, and functional professional links where supplied.
- [ ] Preview-only sample identity, jobs, education, and qualifications never appear as the user's facts in an export.
- [ ] Automated checks verify PDF page count and extracted text order using representative English and Portuguese CVs; visual review confirms readable typography, margins, and no overlap or clipping.
- [ ] The UI describes the format as designed for common résumé parsers without claiming guaranteed compatibility with every applicant tracking system.

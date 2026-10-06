# 02 — Put real identity and contact details in the CV

**What to build:** Let a user maintain their name and contact details as Profile facts and see those details in the CV header, replacing the sample identity and contact line.

**Blocked by:** 01 — Make the CV preview an honest one-page document.

**Status:** ready-for-agent

- [ ] A user can manually enter and edit their name, email, phone, location, and professional links in the Profile; the CV preview updates from those facts.
- [ ] Empty fields do not become invented contact details in the user's CV. Sample values, if shown in the preview, are unmistakably labeled as samples.
- [ ] Existing browser-saved Profiles load without losing any previous facts, and newly entered details remain available after navigation and reload.
- [ ] Existing Profile Review, qualification-gap, and Application Draft paths continue to accept old and new Profile records. New contact facts are sent to an AI workflow only when that workflow needs them; verify the actual request projection.
- [ ] The editing and preview flows work in English and Portuguese and on a narrow viewport; changing UI language does not translate the user's contact information.


# CV density controls (#28)

Local controlled verification on 2026-10-07. No deployment, publication or real-provider call.

- Accessible English/Portuguese Compact, Balanced (default), Detailed radio controls explain evidence selection and compression, independent of typography. Saved density and pending setting are distinguishable.
- Dedicated generation sends density with professional facts and locale. Backend rejects invalid density and defaults missing legacy density to Balanced, then applies the matching fixed selection/compression policy.
- Accepted density metadata persists with the CV snapshot; changing density preserves existing content/edits and Profile, makes no AI request, and invalidates stale proposals. Only deliberate generation plus explicit acceptance replaces content.
- Browser matrix exercises the same short and heavy synthetic source across modes, both locales, desktop/390px; font=15 survives density changes, and density/manual edits survive navigation and reload. Controlled rate-limit responses require deliberate retry.

Controlled responses demonstrate distinct accepted content volumes; they do not prove provider judgment. Supporting source facts and identities stay intact. Direct test-only CDP printing bypasses the ordinary export gate to inspect heavy Detailed content across three A4 pages and confirm measured overflow. Ordinary app export remains blocked for overflowing content; short outputs fit one A4 page. No clipping or content slicing enforces fit.

Validation: frontend suite, Go suite, TypeScript, production Vite build, controlled generation browser/PDF regression, existing shared CV preview/PDF regression. The Vite build reports its pre-existing native config-loader warning about `__dirname`.

Observed preview text lengths and inspected A4 page counts from controlled selections:

| Locale / source | Compact | Balanced | Detailed |
| --- | --- | --- | --- |
| English / short | 192 chars / 1 page | 252 / 1 | 290 / 1 |
| Portuguese / short | 223 / 1 | 294 / 1 | 352 / 1 |
| English / heavy | 187 / 1 | 256 / 1 | 2146 / 3 (overflow) |
| Portuguese / heavy | 187 / 1 | 260 / 1 | 2149 / 3 (overflow) |

Counts are observations, not selection rules. Controlled response selection drives these differences; real-provider density effectiveness remains unverified. Artifacts from the completed run: `/tmp/careeros-cv-generation-qAwkvF/` (desktop/390px screenshots and PDFs for each matrix cell). Visual inspection sampled English mobile Detailed controls/overflow and Portuguese desktop Balanced preview.

Review: Standards found no hard violations and one naming heuristic (density ownership hidden behind the font-size hook); renamed it to `useCvPreferences`. The separate Spec review found no actionable findings and confirmed the ordinary export overflow gate. Existing PDF regression now searches all accessible status messages for overflow, since density adds its own status.

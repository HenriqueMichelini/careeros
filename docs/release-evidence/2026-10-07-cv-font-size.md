# CV font-size verification — issue #26

Verified locally on 2026-10-07 with synthetic Profile data, Chrome 154, and Poppler. No AI/provider calls or deployment checks were made.

The shared CV preference controls the CV Studio and generated Application Draft résumé. Body text uses 12–16 px (9–12 pt), default 12 px; section and entry headings use body +1 px, contact details body −1 px, and the name body ×2.6. Line height grows with text (1.5 in CV Studio, the existing compact 1.4 in generated résumés). Arial and normal tracking apply in screen, export measurement, and print. Preview zoom remains separate.

`npm run test:cv-pdf` verified:

- Native keyboard slider operation at 12, 14, and 16 px, explicit current value, and reset to 12 px in English and Portuguese.
- Immediate font, wrapping/document-height changes, including nested experience/education text. Print media uses the same selected font size.
- Nine PDFs (English, Portuguese, empty Profile × three sizes), each one A4 page. Extracted selectable text stays in reading order; professional link annotations remain active; sample content stays out of empty exports.
- The preference survives navigation and reload. Invalid JSON, out-of-range/non-numeric values, and older empty preferences safely use the default. Existing CV choices and Profile storage remain unchanged; size changes make no AI request.
- Short CVs and controls fit a 390 px viewport in both languages. Deliberately overflowing content at 12 and 16 px remains visible, reports overflow, and blocks export at 390 px.

`npm run test:cv-preview` verified generated résumés with 16 px typography preserved in the print clone and its A4 PDF, plus the representative dense default-size résumé, shared headings/order, responsive preview, and the existing overflow export guard. Its synthetic cover-letter fixture now follows issue #23's structured response contract.

Rendered English minimum and Portuguese maximum PDFs were visually inspected: readable hierarchy, single column, even margins, and no overlap/clipping. English/Portuguese 390 px control screenshots were inspected. `npm test`, `tsc --noEmit`, and `npm run build` passed. Build retains the existing Vite native-config warning.

These are local Chrome checks. Browser print-dialog overrides and other browsers/deployed revisions are not verified by this ticket.

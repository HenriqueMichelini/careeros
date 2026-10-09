# Apply preprocessing integration

Issue #61, Deterministic Preprocessing milestone 5. Verified locally on 2026-10-08, against baseline `8f180baf65423402a258237e6f6edd7101b116cd`.

Both protected public handlers retain strict transport/schema/raw-size validation and Jev classification of the complete original decoded Job Posting. After acceptance, they call `preprocessing.PrepareBounded` and pass its complete `Source.Text()` to their existing AI workflow. `Source` retains original byte mappings, syntax boundaries and rules/source identity for #44. No competing job parser, client acceptance artifact, extra provider call, automatic retry, fan-out or storage is added. Employer instructions, navigation and repetitions remain present; semantic interpretation belongs to downstream AI.

The 30,720-byte original and 128 KiB HTTP body limits are unchanged. Shared workflow limits are 60,000 prepared bytes and 256 KiB for the fully serialized AI envelope, matching Profile ingestion. A capacity error stops downstream AI and offers EN/PT correction feedback. Textarea content and saved Profile remain intact. Exact original input changes invalidate pending work, including formatting changes that would prepare identically. Native textarea line endings are LF; HTTP tests separately cover exact CRLF originals.

## Verification

- `GOCACHE=/tmp/careeros-go-cache go test ./...`: passed.
- `npm test`: all six Node test files passed.
- `node_modules/.bin/tsc --noEmit`, `npm run build`, `git diff --check`: passed.
- Controlled public-handler transports verify exact original classifier input, identical complete prepared text in both AI payloads, Unicode/NFC, protected table line endings, lists, repeated text and applicant PDF/portfolio instructions. Existing handler regressions cover short Java/AWS postings, title-only feedback, uncertain/detected attacks, no bypass, service failures, retries, strict output validation, EN/PT and null metadata. New boundary cases verify 30,720 admitted bytes, 30,721 rejected bytes, a multibyte crossing and normalization expansion capacity with one classifier call and zero downstream calls. Client prepared artifacts/source maps remain rejected by the strict schema.
- `node tests/apply-checklist.check.mjs`: passed EN/PT at 1440px and 390px. Includes qualification confirmation and skip paths, draft gating feedback, deliberate retries, raw source transmission, format-equivalent changed-source invalidation, capacity recovery, cancellation, unchanged saved Profile, short/complete/noisy postings, rendered Results and print-content guards.
- Results screenshots retained below. Visual inspection sampled English desktop and Portuguese mobile; the browser overflow assertions passed for every locale/viewport.

| Locale | Desktop | Mobile |
| --- | --- | --- |
| English | [1440px](evidence/apply-preprocessing/short-results-en-1440.png) | [390px](evidence/apply-preprocessing/short-results-en-390.png) |
| Portuguese | [1440px](evidence/apply-preprocessing/short-results-pt-BR-1440.png) | [390px](evidence/apply-preprocessing/short-results-pt-BR-390.png) |

Evidence is local and controlled: synthetic classifier/provider responses exercise contracts and UI behavior, not live semantic quality or deployed acceptance. No live keys were used. Browser print-content guards are not an actual exported-PDF verification. #62 owns milestone-wide preservation verification; #44 owns source-backed semantic job understanding.

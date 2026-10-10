# Reusable Job Posting context (#44)

## Contract

Apply requests `understandJob: true` in its existing qualification call. The same
OpenAI response contains gaps and a strict extractive `job` schema: nullable role,
company, seniority and location; responsibilities, qualifications and Application
Requirements with required/preferred/unspecified importance. Legacy callers can
still request gaps alone, but receive no reusable artifact. No parsing-only
provider call, backend cache, signing key or acceptance token is introduced.

Each reference specifies a shared preparation segment, literal prepared quote and
zero-based occurrence within that segment. The server resolves it through #61's
source mapping into exact original UTF-8 byte ranges and original excerpts. It
rejects missing fields, unknown fields, invalid categories/importance, duplicate
occurrences within a category, nonexistent quotes and occurrences, invalid mapping
boundaries, refusal and truncation. Protected values cannot be paraphrased inside
a reference. Unknown metadata is explicit null; absent lists are explicit empty
arrays. Mechanical validation cannot establish semantic classification or support.

The artifact's source ID keys the exact original and shared preparation versions.
Its inputs ID includes the Profile and structured qualifications supplied to the
joint call, job prompt/schema/model/reasoning policy, preparation/portion/repetition
versions and bounds policy, and Jev model. The contract version includes the field
rubric/policy. Updating any relevant policy requires a version change. Confirmations
are application-only and do not change the job understanding input. Language only
affects draft generation; job excerpts retain the original posting language.

Every protected route classifies the complete original posting through Jev before
using a context. Draft reuse reconstructs all evidence and checks server-owned
identities/versions. Invalid, stale or unverifiable artifacts yield
`job_context_stale` and no generation call. Apply clears the artifact, preserves
the posting/Profile and asks for deliberate reanalysis. Changed UI inputs invalidate
pending work and the session artifact, even when normalized postings would match.
No artifact is persisted in localStorage or sent to diagnostic logs.

Hashes do not authenticate client artifacts. Even a mechanically valid context is
untrusted: the existing generation prompt independently assesses its metadata,
classification, importance, omissions and semantic support against the complete
posting. References are an inspection index, not an authority that can override
the posting, inject candidate facts or bypass acceptance.

Apply exposes the context and original excerpts before drafting, including when
there are no qualification gaps. Users correct it by returning to the original
textarea and explicitly reanalyzing. Draft retries preserve the artifact and whole
original gate; obsolete artifacts never trigger an automatic repair/retry.

## Verification

- `GOCACHE=/tmp/careeros-go-cache go test ./...`: passed.
- `node --test tests/*.test.mjs`: passed (7 test files).
- `node_modules/.bin/tsc --noEmit`: passed.
- `node_modules/.bin/vite build`: passed; existing bundle-size advisory remains.
- Public production HTTP handlers with controlled provider transports: full/noisy
  posting metadata and duties, short English posting, Portuguese Unicode and
  repeated occurrences, explicit required/preferred/unspecified, unknown metadata,
  salary/portfolio/PDF requests, invented/protected quotes, missing fields,
  nonexistent occurrences, malformed/refused/truncated output, valid draft reuse,
  changed original/Profile/versions, tampered evidence, missing artifact fields,
  rejection before reuse and `Cache-Control: no-store`.
- `node tests/apply-checklist.check.mjs`: passed in English and Portuguese at
  1440px and 390px. Inspects context/original excerpts, confirms exact draft payload,
  recovers deliberately from stale context, invalidates changed originals, preserves
  Profile and source across field decisions/cancellation/failure, and reaches
  rendered Results/controlled print paths.
- `node tests/cv-shared-preview.check.mjs`: passed shared Profile/Apply A4 preview,
  section order, fit and overflow behavior with synthetic API responses.
- `node tests/results-firefox-print.check.mjs`: passed actual Firefox Save to PDF
  (one page), signed cover display/copy/PDF, narrow layout and signature overflow.
  Its preexisting fixture needed a synthetic TypeSafe key to reach Apply's gate.
- Code review against `54d6d3b`: independent Standards and Spec reviewers reported
  zero actionable findings; controlled evidence limits were preserved.
- Visual inspection: Portuguese 390px review contains readable metadata, source
  excerpts, qualifications, Application Requirements and no horizontal overflow.

These are controlled transport/browser results, not real-provider semantic-quality
or deployment certification. No paid provider calls or user credentials were used.
Finite extraction fixtures do not prove universal completeness or factuality.
Broader matching/support and final deployment/PDF gates remain #45–#48/#52 work.

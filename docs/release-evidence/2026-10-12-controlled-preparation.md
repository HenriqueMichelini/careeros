# Issue #52 — controlled preparation after the remaining-gates audit

Started: 2026-10-12. Controlled validation refreshed: 2026-10-13,
America/Sao_Paulo. Review baseline:
`1a30a445e194d353af116d8a2b5c5194b14ff36a`.

**Release gate: not passed; #52 remains open.** This follow-up implements the
provider-free preparation requested after the audit. No credentials were read,
provider requests made, production runtime published or billing settings changed. Original
release observations and closed prerequisite tickets remain intact.

## Resulting behavior

Results waits for a successful Clipboard API write or a `true` legacy-copy
acknowledgment before showing “Copied!” / “Copiado!”. Missing, throwing and
rejecting APIs with failed fallbacks show localized failure feedback. Pending
writes disable repeat activation. Switching artifacts, changing accepted content
or changing the Profile invalidates old completion feedback and prevents a late legacy fallback write. Fallback cleanup
preserves keyboard focus, including after asynchronous permission rejection.

The user chose to answer explicit employer requests first and label additional
suggestions separately. Generation policy v3 removes the old five-to-six-answer
quota. English sections are `Employer-requested answers` and `Additional
suggestions`; Portuguese sections are `Respostas solicitadas pela empresa` and
`Sugestões adicionais`. Empty sections are omitted. A posting with no requested
answers uses only suggestions. An individually requested response language is
preserved, missing candidate information remains explicit, and suggestions cannot
replace requested answers. Artifact-support policy v8 receives `cvLanguage` and
can flag mislabeled suggestions as well as missing requested answers. Returned
concerns still block acceptance through the existing contract.

These are prompt and enforcement contracts, not evidence that an actual model
reliably follows the policy. Models, reasoning settings, schemas, dispatch counts,
source validation, whole-original Jev gating and explicit acceptance remain in
place. No semantic parser was relaxed to recover the uncaptured English result.

## Evidence boundaries

The retained Portuguese copied Markdown is used verbatim as a rendering input:

```text
## Competências Técnicas
- Ruby: manutenção de ferramentas CLI internas.
```

The English counterpart and sparse Profile are controlled reconstructions.
Browser responses are synthetic review fixtures, not historical raw responses or
new provider judgments. The Ruby source and posting are known regression inputs,
not a disjoint final held-out set.

The extended `test:cv-preview` uses an empty temporary Chrome profile. It checks
native Enter, Space and pointer activation for accepted resume, cover letter and
Q&A text, with 24 actual Clipboard API transfers. Resume checks run in English
and Portuguese at 1280px and 390px. It separately exercises controlled missing,
throwing and rejecting clipboard APIs with acknowledged and failed fallbacks.
The native checks grant clipboard permission in the isolated browser; they do
not establish behavior under every browser's permission policy.

Four actual A4 resume PDFs are saved from the Results print clone, one per
language/viewport combination. Page count, selectable claim occurrence and
accepted heading wording are checked with Poppler. Complete rendered pages are
inspected for clipping, overlap and extra UI. Capitalization comes from styling;
the accepted heading wording is preserved. The older frozen-runtime PDF remains
unchanged and is not used to verify this correction. Existing dense/overflow,
cover-letter/signature and Firefox checks remain separate tests.

Full parsed saved Profile objects are compared before and after the controlled
draft, acceptance, clipboard and PDF operations. This uses deep value equality,
not serialized property order. Temporary synthetic keys and clipboard contents
are cleared. The clipboard write is awaited and empty contents are read back
before and after reload. The fresh-load UI shows pending keys with a ready
Profile/posting and disabled generation; both stored keys and the rendered
TypeSafe key field are empty. These local synthetic
checks do not replace the missing full-Profile comparison in the frozen live
batch.

The reconstructed Ruby handler/client regression gives a multi-subclaim summary
four distinct role/duty/requirement/request excerpts. A supplied uncertain verdict
with title-only support remains unaccepted; a supplied missing-required-answer
concern also blocks acceptance. A separately supported CV can still be accepted.
This verifies transport, source preservation and concern enforcement. It does
not demonstrate that a real reviewer will recognize every unsupported subclaim.
The historical PT ingestion-language and qualification/request-separation
observations, and the uncaptured English delivery cause, remain unresolved.

## Capture capability and stopping point

The browser check captures actual local browser request bodies, response status,
allowlisted content/cache headers, response bodies and Network request duration
with CDP. Requests are intercepted and fulfilled with synthetic responses;
**there is no deployed handler or provider dispatch in this browser check**.
Authentication headers are never written to its capture artifact. Request and
response hashes/byte lengths and the checked source-file hashes are retained in
[the controlled browser record](issue-52/controlled-results-browser.json).

Separately, `TestControlledReleaseCapturesEachProviderEnvelopeBeforeAdmission`
invokes the production `NewHandlerWithClient` seam with a closed in-memory
transport. It inspects all four serialized envelopes before responding: complete
original Jev input, generation, resume support and artifact support. It checks
outbound field allowlists, fixed settings, output ceilings and absence of the
synthetic credentials from bodies. Changing a generated result changes both
dependent support envelopes despite an identical initial generation envelope.
Denying the artifact-support envelope leaves that review unavailable with no
retry. The test transport has no network delegate; no live command or credential
file is involved.

**Criterion 7 remains unavailable at the deployed egress boundary.** The ordinary
Netlify wrapper still constructs the ordinary handler. This patch adds no
production content logging or deployed interception mechanism. Local CDP capture
and in-memory transport inspection cannot establish actual deployed outbound
bytes. Stop before selecting a further paid batch until safe deployed inspection
is demonstrated inside a separately approved privacy boundary. A static fixture
cannot preapprove unknown response-dependent envelopes.

No new live candidate, exact deployment/origin pair, broader held-out inventory,
dispatch budget or pricing/billing evidence is frozen by this patch. Those belong
to the subsequent gated measurement work. A finite smoke batch could narrow
delivery/export gaps but cannot supply the missing broader quality and
accepted-result-cost comparison. Actual charge and accepted complete-application
cost remain unavailable; a zero accepted-application denominator stays undefined.

## Validation

The implementation uses the user-confirmed Results/copy/PDF,
`generateMaterials` and production HTTP-handler seams. Required final checks and
the two-axis review are recorded on the implementation PR and canonical
[issue #52](https://github.com/HenriqueMichelini/careeros/issues/52), together with
its exact commit. The commands are:

```bash
rtk proxy env GOCACHE=/tmp/careeros-go-cache go test ./...
rtk proxy env GOCACHE=/tmp/careeros-go-cache npm test
rtk proxy node_modules/.bin/tsc --noEmit
rtk proxy npm run build -- --outDir /tmp/careeros-issue52-controlled-build --emptyOutDir
rtk proxy node tests/resume-review-browser.check.mjs
rtk proxy npm run test:cv-preview
rtk proxy npm run test:results-firefox-pdf
```

Refreshed on 2026-10-13: all Go packages passed; all 133 Node tests passed;
TypeScript and the production build passed. The build retained its existing
large-chunk advisory. The extended Chrome check passed, including the 24 native
transfers, stale-copy rejection and four A4 exports. Resume/application review
passed in EN/PT at 390/1440px. Firefox saved one-page PDFs and passed signed-cover
display/copy/export, narrow-layout and signature-overflow checks.

The initial clipboard regression failed because a rejected write and false
fallback still displayed “Copied!”. A later red case demonstrated that an
obsolete rejected copy could start a legacy write; the current guard prevents
that write and preserves focus on the current artifact. Test-harness failures
also exposed reload and fixed-delay dialog-click races. The harness now waits
for the replacement document and rendered dialog controls. These controlled
failures are distinct from the historical uncaptured English delivery failure.

The initial PR security check flagged PATH lookup for the three Poppler tools in
the new browser harness. Those invocations now use the verified system binaries
under `/usr/bin`. Controlled fixture setup and assertions were split into smaller
helpers after the complexity review; browser actions remain sequential because
they share the document and clipboard. The full Go and Node suites, TypeScript,
production build and extended Chrome/PDF check passed again after these changes.
The retained browser record and PDF artifacts were regenerated from that source.

Node/Go adapter runs also receive process-local `GIT_DIR` and `GIT_WORK_TREE`
pointing at the actual checkout. No VCS stamping is disabled. The production build
uses a temporary output directory; generated assets are excluded from the source
commit. An initial restricted-sandbox `spawnSync go EPERM` was an environment
failure; the authorized local rerun used the same controlled adapter.

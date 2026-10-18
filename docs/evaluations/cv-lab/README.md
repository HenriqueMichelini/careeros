# Controlled CV integrity evidence in the Lab (#153)

The `cv-integrity-evidence` suite replays immutable owner evidence **offline**.
Select `owner-retained-v1` in the standalone Lab (`npm run lab`). It makes zero
new handler, HTTP, generation or assessment calls. CV v3 policy and client
behavior remain owned by #124–#128. PR target is the delivered
`codex/evidence-backed-cvs-and-applications` integration branch; frozen review
base: `db6b3446acb2c5df464a846a2a95a9a158f94775`.

`evidence.v1.json` retains original handler-report and fixture bytes (base64),
with SHA-256 identities. Exact registration permits trusted import; changing
any receipt byte quarantines it. The mapper never opens receipt-supplied paths.
The original owner corpus and historical Lab reports remain unchanged.

## Cases and joins

- Fourteen #128 handler/client cases, seven EN and seven PT-BR: Java→Kubernetes,
  unsupported responsibilities and qualifications, paraphrases/translations,
  unavailable assessment, and cross-owner hard rejection.
- Eight existing browser observations: discard/revert, all-summary removal,
  cancellation, user edit/confirmation, stale source references, quota failure
  preserving the previous CV and recoverable proposal, legacy-v2 unverified
  state, and v3 save/reload. The Kubernetes flow retains desktop/390px keyboard
  provenance in both languages.
- Two original owner contract outputs (TAP and Go JSON), with source hashes:
  v1/v2/malformed-v3 loading, provenance validation, bounded assessment,
  cancellation, unavailable/deadline paths and call envelopes. Command success
  remains test evidence, not semantic or user acceptance.

Case names alone cannot join receipts. Handler/client joins require exact
language, fact text/field/section, canonical owner/reference/revision, and
statement text/source IDs/status. `stableCvFacts` assigns positional legacy
entry aliases; the join resolves them through the canonical document. Missing
observations remain missing; duplicates and mismatches are rejected.

Browser mocks have different source/output identities from the handler corpus.
They remain separate cases and cannot duplicate dispatches or invent a
handler→browser join. In-process serialization/reparse is separate from actual
localStorage durability. Snapshot provenance and original judgment history
remain intact. User edits/confirmations and legacy wording never inherit
machine-supported status; source freshness remains separate from wording trust.

## Measures and limits

The handler cohort retains **26 simulated dispatches**: two per ordinary case,
one per hard-guard control. Replay records **zero new dispatches**. The six
unsupported outputs and two hard rejections retain their **eight scorer
failures**. Successful enforcement is not rewritten as successful content.

The explicitly named scripted acceptance measures are **0/6 unsupported
statements** and **0/6 unsafe cases** accepted as active machine wording. Eight
explicit user edits/confirmations form a separate provenance cohort. M15's
independently adjudicated denominator is **0**, with six pending scripted labels;
its value is unavailable. No human reviewer or adjudicated population is
invented. Missing acceptance cannot produce a zero false-acceptance result.

M09–M12, M16, M19, M23–M25 retain explicit missingness where owner evidence does
not establish complete causal denominators, independent semantic judgments,
human acceptance/review burden, real cost, or handler-cohort durable storage.
Separate browser receipts retain actual storage observations.

ADR 0003 remains authoritative: one added 4,000-token / 128-KiB assessment under
the shared 25-second workflow, without retry/repair/chunk/fallback. Original Go
owner tests retain these boundary observations; replay does not redefine them.
No live quality, billing, deployment, native PDF, #122 closure-policy change or
#52 release acceptance is claimed. Comparison still requires the shared complete
pairing contract; unavailable metadata and unscored cases decline comparison.

## Reproduction

Owner harnesses have optional observation-only output hooks. Require successful
commands and review receipts before registering a new hash:

```sh
rtk proxy env GOCACHE=/tmp/careeros-go-cache CV_LAB_EVIDENCE_OUTPUT=/tmp/cv-owner.json node --test tests/cv-support-integration.test.mjs
rtk proxy env CV_LAB_BROWSER_OUTPUT=/tmp/cv-browser.json node tests/cv-generation-browser.check.mjs
rtk proxy sh -c 'GOCACHE=/tmp/careeros-go-cache node --test --test-reporter=tap tests/cv-generation.test.mjs > /tmp/cv-client.tap'
rtk proxy sh -c 'GOCACHE=/tmp/careeros-go-cache go test -json -count=1 ./backend/cv-generation > /tmp/cv-backend.jsonl'
rtk proxy node scripts/cv-lab-evidence.mjs /tmp/cv-owner.json /tmp/cv-browser.json /tmp/cv-client.tap /tmp/cv-backend.jsonl /tmp/new-cv-receipt.json
```

New receipts remain candidates, quarantined until reviewed and registered.
Exclusive creation prevents replacement. Receipt source hashes identify the
actual captured owner source; later formatting does not change historical bytes.

Confirmed public test seams: Lab import/catalog/result/metrics, including exact
joins, M15 denominators, missingness and duplicate handling. Focused tests retain
original failure counts, 26 unique historical calls, zero replay calls, exact
provenance, pending labels and private export/reload. Foreign Profile references
and changed HTTP outcomes failed before their respective fixes.

The first owner browser capture passed; a later capture timed out before the
first proposal appeared. After adding timeout page-state diagnostics, the next
run passed. The cause remains unestablished; no flake-free claim is made.

The previous isolated-test manifest predates CV deliveries. Immutable
`test-adapters/cases.v3.json` updates eight changed source hashes and adds the
already-used `tests/cv-support-fixtures.mjs` dependency. It adds no executable
and does not widen isolation. v1/v2 manifests and results remain unchanged.

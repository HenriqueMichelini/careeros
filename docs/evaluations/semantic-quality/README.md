# Semantic quality baseline — issue #34

This local harness evaluates extraction/Profile Proposals, qualification gaps,
general CV generation and Application Drafts through the production Go HTTP
handlers. It does not implement another inference pipeline or change prompts,
models, routing, source filters or Profile apply behavior.

## Frozen labels and evidence boundaries

`cases.v1.json` contains 26 synthetic EN/PT cases with stable IDs, task labels,
development/heldout splits, exact application requests, independently authored
controlled provider responses, gold expectations and forbidden additions.
`sourceId` links reused cases to issue #29's field-validation corpus. Its
acceptance decisions and approved Jev routing remain owned by #29. Here the
controlled classifier accepts usable synthetic input to isolate downstream
contracts; it is not evidence of classifier accuracy or provider quality.

The held-out partition is reserved from prompt/model/settings development.
Running its deterministic controls verifies plumbing, not inference quality.
No production prompt or settings were tuned in this ticket. Some held-out PT
cases are translated counterparts; this small bilingual set is not an
independent population sample. Add independent unseen cases before drawing
generalization conclusions. Increment the fixture version when changing labels,
source facts or splits. Keep original run artifacts when correcting a label.

Gold atoms explicitly specify facts and structured destinations, action/finding
constraints where applicable, source IDs and entity IDs. Negation/aspirations,
approximate dates/pronouns, multiple roles/projects, metric ownership,
proficiency conflicts, exact/repeated duplicates, messy/short postings and
sparse Profiles are represented. The 35-fact inputs deliberately exceed the
production 30-claim limit: all 35 facts and destinations remain in the denominator.
Each omitted atom is reported separately, including on failed HTTP responses.

## Scoring contract

`internal/semanticeval.Score` performs deterministic, one-to-one matches against
curated output atoms. It reports label precision (`matched / observed`) and
completeness (`matched / expected`) separately, with individual missing and
unmatched units, duplicates, relationship errors and forbidden-pattern hits.
Empty denominators conventionally score 1; HTTP/field rejection still records
failures and missing expectations, so an error is never a passing case.

Extraction claims and proposal operations are separate atoms; gap atoms measure
requirement coverage; CV selection and cited summaries measure source coverage.
Application Draft uses one conjunctive material-level label over metadata,
Job Summary, resume, cover letter and answers. Its label precision is therefore
not sentence-level factual precision. Audit each generated factual sentence
against the source Profile, confirmed qualifications and Job Posting using the
review worksheet below. Preserve unsupported claims and relationship errors
individually, with their output location and source evidence.

An unmatched unit is an **unsupported-claim candidate**, not an established
hallucination: a valid paraphrase can miss a narrow label. Relationship errors
mean a content match failed another labeled constraint (such as owner,
destination or source ID); inspect the output before assigning a semantic cause.
Regexes also cannot enumerate every possible fabrication. A zero result means
zero curated forbidden-pattern hits on that finite run, never universal factual
safety. The acceptance target is zero forbidden factual additions, including
ones found by source-grounded review. Findings do not authorize a prompt fix.

No semantic model judge is used. Introducing one requires human-labeled
calibration and reversed-order/verbosity tests before its scores count as
evidence. These boundaries follow the issue's linked
[OpenAI evaluation guidance](https://developers.openai.com/api/docs/guides/evaluation-best-practices).

## Reproduce

From the repository root (Go 1.24):

```sh
GOCACHE=/tmp/careeros-go-cache go test ./internal/semanticeval
GOCACHE=/tmp/careeros-go-cache go run ./scripts/semantic-quality \
  --split all --output /tmp/semantic-controlled.json
```

All ordinary tests use controlled transport. It never falls through to the
network, even if real credentials exist in the environment. Each output path
must be new; reports are written with mode 0600 and checkpointed after every
case. Reports contain only synthetic source-derived outputs, never keys or
authorization headers. Keep real Profile data out of this corpus.

Only after explicit authorization, run a chosen split or ID list:

```sh
GOCACHE=/tmp/careeros-go-cache go run ./scripts/semantic-quality \
  --mode live --authorize-live --limit 8 \
  --ids en-fact,pt-fact,en-short-job,pt-short-job,en-cv-metric,pt-cv-metric,en-draft-sparse,pt-draft-sparse \
  --openai-key-file /private/path/openai-assignment \
  --typesafe-key-file /private/path/typesafe-assignment \
  --output /tmp/semantic-live.json
```

Credential files contain `OPENAI_API_KEY=value` / `TYPESAFE_API_KEY=value`.
Values are parsed process-locally. No environment key triggers a run implicitly.
Eight **workflow** attempts can mean up to 18 provider calls (ingestion needs
classification, extraction and comparison). There are no automatic retries or
fallbacks. A non-200 live workflow stops the run and preserves partial evidence;
unattempted IDs remain pending. A further invocation is a deliberate run within
the user's authorized budget, never an invisible retry. Success exit status
means the baseline was saved, not that semantic acceptance passed. Inspect each
run's `score.failures`, `status` and `humanReview` fields.

Every report includes timestamp, mode, git base commit, exact fixture hash and
version, workflow/scorer/adapter source hashes, and each outbound model,
reasoning setting, output-token bound, request/prompt/schema hash. Prompt hashes
include synthetic dynamic content; source hashes identify the actual production
prompt/schema templates even in an uncommitted evaluation. Successful OpenAI
responses also record returned model and generated choices for diagnosis.
Provider failures may lack a returned model. Local handler evidence does not
establish deployed behavior, rendering, PDF layout, latency/cost comparisons or
service reliability; those remain separate work.

## Source-grounded review worksheet

For every prose-bearing live result, record report hash and case ID, then:

| Dimension | Labels to record |
| --- | --- |
| Atomic support | Every factual sentence/excerpt; supported, unsupported, or uncertain; exact source |
| Requirements | Explicit Job Posting qualifications present/omitted; no invented qualification |
| Relations | Employer/role/project/date/metric/proficiency ownership retained or transferred |
| Proposals | Correct destination; duplicate suppressed; conflict withheld; no silent apply |
| Unknowns | Unspecified company/title/dates/proficiency remain unknown |
| Failures | Every unsupported addition, omitted atom and rejected/unfinished workflow |

Agent-assisted inspection is recorded as such; it does not impersonate human
adjudication. Human review remains explicitly pending when needed. Issue #35
owns reusable usage/outcome measurements and #50 owns later configuration
comparisons; this baseline makes neither a provider-selection nor tuning claim.

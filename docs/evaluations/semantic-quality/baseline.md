# Baseline recorded on 2026-10-08

Issue #34 establishes measurement before behavior changes. No workflow prompt,
model or provider-selection setting changed. The user authorized up to eight
synthetic workflow attempts using their local OpenAI and TypeSafe credentials.
Exactly eight distinct cases were attempted; failed cases were not retried.
The first continuation was delayed by automatic approval-review timeout before
execution; the allowed retry started the remaining cases. No provider request
occurred during that timeout.

## Controlled evidence

`controlled.v1.json`: 26 production-handler cases, all HTTP 200 with no-store.
Zero curated forbidden-pattern hits. Twenty-four cases match every label.
`en-capacity-35` and `pt-capacity-35` each retain only 30 of 35 explicit facts and
destinations: label precision 1, completeness 60/70 = 0.8571. The report names
the ten missing labels per case. These responses intentionally model the
documented extraction capacity. They demonstrate detectable loss, not measured
real-provider extraction quality. Duplicate provider operations are suppressed;
proficiency-conflict operations are withheld through production filtering.

The scorer's public-interface regression also confirms that a 20% achievement
rewritten as 90% is rejected by the real CV handler. Ordinary tests remain
keyless and have no network fallback.

## Real-provider evidence

Reports remain separate from controlled evidence and retain their original
fixture/source hashes. Only eight representative cases ran; adversarial,
capacity and the rest of the held-out inference corpus remain **pending**.

| Case | Report | Production result | Individual finding |
| --- | --- | --- | --- |
| en-fact | live.initial.v1.json | 200; Java proposal | Original gold accepted only `Java`; `Uses Java.` is a valid paraphrase but scored 0.5/0.5. Development label corrected to list this bounded alias; original result retained. |
| en-short-job | live.initial.v1.json | 200; AWS gap | Precision/completeness 1; Java correctly not a gap. |
| en-cv-metric | live.initial.v1.json | 502 invalid_output | Both CV labels missing; provider response was not captured by this initial adapter version. Cause beyond production rejection remains unknown. |
| en-draft-sparse | live.remaining.v1.json | 200 | Material label passes; company null, Java supported, AWS only a job requirement. |
| pt-fact | live.remaining.v1.json | 200; Java proposal | Correct fact/destination; English claim prose `Uses Java.` despite Portuguese source is a separate language-preservation concern. |
| pt-short-job | live.remaining.v1.json | 200; AWS gap | Requirement coverage passes; English detail prose on Portuguese input is recorded without treating it as an invented fact. |
| pt-cv-metric | live.remaining.v1.json | 502 invalid_output | Captured provider output contains `summary: []`, `selected: ["f0"]`, and wording with the supported 20% metric; production rejects the empty summary. Both expected output labels are missing. |
| pt-draft-sparse | live.final.v1.json | 200 | Material label passes; company null, Java supported, AWS only a requirement. Answers include unknown experience/education/certification rather than fabricated histories. |

No curated forbidden-pattern hit occurred on these eight attempts. This is a
finite-set observation. Two CV attempts failed, so it does not demonstrate
successful CV quality. The sparse draft material labels do not measure
sentence-level precision; their source-grounded inspection is below. The
recorded label failure is not erased or combined into a single success score.

## Agent-assisted source inspection (not human adjudication)

For EN/PT ingestion, `I use Java` / `Eu uso Java` support the extracted usage
claim and exactly one `skills=Java` addition. No employer, dates, seniority,
duration or proficiency is added. For the gaps, the posting explicitly requires
AWS and the Profile contains only Java; one AWS memory prompt is supported.

The EN draft's resume contains only Java. Its cover letter says Java is listed
among the candidate's skills and AWS is required, then explicitly declines to
claim AWS experience, projects or history. Its six answers preserve absent AWS,
projects, education, certifications and languages as unknown. Company is null.
Its summary's AWS requirement is supported by the Job Posting.

The PT draft's resume contains only Java. Its cover letter's Java knowledge is
supported by the Profile's Java skill; AWS remains a requirement, with no claim
of mastery. Five answers describe the Java vacancy, AWS requirement, and absent
professional history, education and certifications. Job Summary accurately
states that company/location/additional requirements are not given. Company is
null. Neither draft transfers employer/project/metric facts or invents factual
candidate achievements. No unsupported factual addition was identified in this
agent-assisted inspection; human prose adjudication remains explicitly pending.

This is a baseline with observable defects, not a prompt-tuning or provider
comparison result. The broader live suite, human adjudication, browser/PDF and
deployed evidence are not claimed. The zero-additions acceptance **target** is
retained; this finite inspection is not a guarantee outside the reviewed cases.

## Implementation verification

`go test ./...`, all six `npm test` files, `tsc --noEmit`, and `vite build` pass.
The targeted scorer/production-adapter tests were run red then green at the two
user-agreed seams. Build artifacts were restored after verification. The
required independent Standards and Spec reviews are recorded in the PR/issue.

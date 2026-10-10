# Issue #45: explained requirement support

Review baseline: 68e2abad9fd15280371d5324c13fb1af94b134b2.
Target: milestone 4, codex/evidence-backed-cvs-and-applications → main.
Native blockers #36 and #44 were closed before implementation. The user approved
public qualification HTTP, Profile projection and rendered Apply seams, and the
fixed review baseline.

## Behavior and boundaries

Apply sends a transient qualification-only Profile subdocument. Approved facts
and accepted excerpts retain stable IDs/revisions, owners, linked context, dates,
negation, uncertainty and approximate wording. Existing migrated whole blocks
remain usable with their original unreviewed/unknown provenance; they are never
promoted to approved or decomposed. Contact, goals, compensation, employment
status, other notes, credential identifiers and links are excluded from the
matching projection. Manually entered facts need no fabricated source excerpt.

The whole original Job Posting still passes Jev first. The existing structured
job-understanding call runs once, followed by one strict matching call when
qualifications exist. No retry or fallback occurs. Each extracted explicit
qualification gets a result; there is no five-gap cap in this path. Legacy
qualification callers keep their original contract. Responsibilities and
Application Requirements remain separate in job understanding.

Candidate selection groups role/project facts with identity and dates, includes
linked context, prioritizes negative/uncertain units and lexical matches, and
keeps whole canonical facts. Its default per-requirement serialized fact budget
is 16,384 bytes. Excluded facts are counted and coverage is incomplete whenever
any projected fact is omitted, even if it appears irrelevant. A not_evidenced
provider result under incomplete coverage becomes needs_clarification. A
complete serialized provider envelope exceeding the shared payload limit stops
with a capacity error. Nothing is truncated. The workflow has a 52-second shared
deadline, 25-second provider stages and a 55-second client deadline.

The server verifies exactly one result per extracted qualification, state enums,
required keys, bounded strings, candidate fact references and positive-support
semantic flags. Explicit negation, aspiration, uncertainty or invalidated support
cannot establish positive support. The prompt independently requires semantic
assessment, preserves conflicting evidence, and prohibits inferring technology
usage duration from role dates or merging employers/projects into invented
experience. References and lexical matches do not prove semantic truth.

The bilingual Apply review displays all four states, original job importance,
explanations, focused questions, incomplete-coverage warnings, selected facts,
identity/period context and accepted excerpts. Facts are grouped by their owner,
and profile-level facts display their linked role/project identities explicitly. A checkbox confirms only a
qualification. Users can supply an answer without checking it, including a
negative answer. Answers and confirmations stay transient and application-only;
the draft prompt distinguishes answers from positive confirmations. Saving facts
requires separate Profile review/apply. The contextual draft contract permits up
to 100 confirmations of at most 4,000 bytes, matching job qualification limits;
legacy draft callers retain their previous limits. Answers have independent
bounded strict contracts and never alter the saved Profile.

## Verification

- TypeScript tsc --noEmit and Vite production build passed.
- Full Go suite passed, including HTTP checks for exact references, duplicate or
  missing requirements, private projection fields, malformed output, explicit
  negative/aspirational false support, and incomplete coverage.
- All eight Node test files passed. Projection checks cover sparse Projects,
  Education, Certifications, Languages, tools, competencies, skills and Experience.
- Controlled Chrome Apply flow passed in EN/PT at 390/1440px. It covers every
  state, focused questions and incomplete warnings, factual-context inspection,
  unconfirmed negative answers, unchanged saved Profile, deliberate retries,
  field gate failures, and ordinary application/Results transitions. At 390px in
  both languages each sparse section independently starts Apply, while posting,
  key and in-progress guards remain active. A two-employer case verifies that a
  profile-level skill linked to Harbor retains that relationship and remains
  distinct from Summit, including titles and dates. A rerun on unchanged product code
  resolved an intermittent pre-existing native keyboard-navigation failure.
- Reproduce labeled evaluation with:
  GOCACHE=/tmp/careeros-go-cache go run ./scripts/qualification-evidence -output /tmp/qualification-evidence-report.json.

The frozen 13-case EN/PT corpus labels support, partial/missing evidence,
aspiration, negation, approximate duration, related technologies, Java versus
JavaScript, contrasting employers/projects, and incomplete coverage. Candidate
recall and controlled final matching are separate metrics:

| Per-requirement budget | Labeled relevant facts retrieved | Negative misses | Controlled match labels preserved |
| --- | --- | --- | --- |
| 2,048 bytes | 13 / 15 | 0 | 12 / 13 |
| 8,192 bytes | 15 / 15 | 0 | 13 / 13 |
| 16,384 bytes | 15 / 15 | 0 | 13 / 13 |

At 2 KB, the contrasting-employer case misses b-company and b-aws; the
provider fixture citing excluded facts is rejected, rather than accepted as
support. This failure remains visible in controlled-report.json. Default-budget
production HTTP replays preserve 13/13 expected decisions, including converting
an absence claim to clarification under incomplete coverage.

## Evidence limits

Final matching replays synthetic provider outputs through production HTTP
handlers. It measures contract preservation, not live model accuracy. Candidate
recall is independently measured on labeled synthetic facts, not a population
estimate. Real-provider semantic accuracy, maximum-input latency and deployed
provider operation are unmeasured here; no credentials or paid calls were used.
Source references alone cannot detect every semantically incorrect explanation
or unsupported relationship. Explicit user inspection remains required before
continuing. The broader scoped Application Draft projection belongs to #46.

## Independent review

The first Standards review found no hard violations and one nonblocking field-
policy duplication concern. The first Spec review identified a P2: the flat
inspector did not make fact-to-role/project relationships visible. The correction
groups facts by owner, renders linked context identities, adds a bilingual
two-employer browser case and shares the allowlist through fields.json.

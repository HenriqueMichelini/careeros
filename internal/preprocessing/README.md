# Transient source normalization (issue #56)

`Normalize(decodedOriginal, fieldvalidation.Field)` returns an immutable `Source`
with `Original()`, `Text()`, `ID()`, `Version()` and a defensive-copy `Mappings()`.
The original string is retained exactly. This package has no I/O, logs, clock,
randomness, acceptance decision or handler integration. Transport/schema/raw-size
checks and whole-original Jev validation remain the callers' responsibilities.

| Rule (`normalization-v1`) | Working view | Original / mapping |
| --- | --- | --- |
| Canonical Unicode NFC in prose | `Joa\u0303o` → `João` | Entire original combining group retained |
| Prose CRLF and lone CR | `a\r\nb\rc` → `a\nb\nc` | CRLF maps as one replacement group |
| Recognizable URL/email value | Exact bytes | One mapping per unchanged UTF-8 character |
| Inline/fenced code | Exact bytes, including line endings | One mapping per unchanged UTF-8 character |
| Everything else | Preserved (spacing, paragraphs, case, punctuation, diacritics, emoji, compatibility characters) | Ordered complete source coverage |

Tests contain literal input/output fixtures. NFC does not add stream-safety CGJ
characters to long combining sequences; original CGJ characters remain intact.
There is no NFKC, case folding, aliasing, content filtering or whitespace cleanup.

## Minimal lexical literal guards

- URLs begin with case-insensitive `http://`, `https://`, `www.` or `mailto:`
  and extend until whitespace or `<`, `>`, `"`, or a backtick. Trailing punctuation
  can be protected conservatively. These guards do not validate URLs.
- Email values use Unicode letters, combining marks, numbers and common ASCII
  local-part characters, an `@`, and dot-separated domain labels of letters,
  marks, numbers, underscores or hyphens. These are guards, not email validation.
- Inline code uses matching backtick runs of equal length, including across
  lines. Fences use at least three backticks or tildes after at most three leading
  spaces; a closing line uses the same marker, at least the opening run length,
  and whitespace only afterward. Original closing line endings are protected.
- Unclosed backtick spans/fences conservatively protect the remaining source.
  This is a small lexical guard, not a complete Markdown parser. Rule changes
  require a new version and preservation fixtures.

## Coordinates and identity

All `Range` values are half-open UTF-8 **byte** offsets. Original and normalized
coordinates are separate. JavaScript callers must convert from UTF-16 explicitly.
`Resolve(normalizedRange)` returns exact original ranges, coalescing adjacency.
An unchanged character is indivisible. A changed NFC group or CRLF is also
indivisible: partial transformed groups, split characters, empty/reversed or
out-of-bounds ranges return explicit errors. No search-based offset guessing is
used. A whole nonempty working view always resolves to the entire original.
Empty sources have empty mappings; they have no nonempty resolvable range.

`ID()` hashes a framed JSON tuple of rules version, field and complete original
with SHA-256. The field is an identity namespace, not a field policy decision.
There are no rule options in v1. `RangeID()` hashes that source identity plus
normalized coordinates, so identical repeated excerpts have distinct occurrence
IDs. Re-running the same input/field/version gives identical values; canonically
equivalent but byte-distinct originals intentionally have different source IDs.
Hashes and source objects are transient and must not be logged or persisted by
future integrations merely because they are available.

Run focused tests with `GOCACHE=/tmp/careeros-go-cache go test ./internal/preprocessing`.
Golden, invariant and fuzz-seed tests cover Unicode, literal protection, exact
excerpts, repeat identity, invalid ranges, immutability, and current 30,000 and
30,720 byte limits. This package does not impose a replacement input limit.

## Structural preparation (issue #57)

`Prepare(decodedOriginal, field)` returns the same immutable `Source` contract
plus defensive-copy `Segments()`, under `structure-v1`. Its identity includes
both normalization and structure versions, the field, and the complete original.
`Normalize` retains its independent `normalization-v1` behavior.

Segments cover each physical line, including its line ending, in order. Explicit
blank lines delimit paragraphs; adjacent text lines are retained individually,
without sentence splitting. Every byte belongs to one segment, including blank
lines, separators and unknown text. Each segment has a source-scoped occurrence
ID, syntactic kind, normalized range, exact original ranges, indentation columns,
ATX heading level and an optional earlier parent ID. Headings attach by explicit
level, list items by marker indentation, and indented continuations by list
content columns. Blank lines retain list context; dedented content ends it.
These references describe syntax, never a career category or semantic ownership.

Recognized syntax: ATX headings (`#` through `######` followed by whitespace or
end of line), unordered markers (`-`, `+`, `*`), up-to-nine-digit numbered markers
with `.` or `)`, quotations beginning with `>`, repeated `-`, `*` or `_` separators,
backtick/tilde fences and indented literal blocks. Leading tabs advance to the
next four-column stop. Pipe/tab alignment and two or more internal multi-space
gaps conservatively protect table-like material. Capitalization, section names,
Setext-looking lines and unfamiliar syntax remain text. This is deliberately a
small lexical representation, not a complete Markdown parser.

The cleanup allowlist contains only internal horizontal separator runs in
prose/list text: ordinary spaces and NBSP can become one ordinary space outside
literal/token guards. Leading indentation, marker spacing, trailing whitespace
(including Markdown hard breaks), protected values, quotation/literal/table
alignment and uncertain cases remain unchanged. Literal, quotation and aligned
blocks are protected before NFC/line-ending normalization as well. No characters
are dropped: collapsed runs map as indivisible groups to their entire original
range. All original regions remain recoverable, including every repeated line.
Preparation is deterministic and normalized text is idempotent.

Golden cases and mapping/segment coverage invariants exercise nested/mixed lists,
repeated headings, wrapping, CRLF, tabs/NBSP, inline/fenced/indented literals,
alignment, bilingual prose and one-line Java/AWS inputs. `FuzzPreparedCoverage`
checks determinism, idempotence, UTF-8 boundaries and complete original coverage.
No handler integration, provider call, semantic classification, acceptance,
filtering, persisted source or Profile mutation is introduced by this API.

## Repetition annotations (issue #58)

`Source.Repetitions()` returns transient, defensive-copy groups over `Prepare`'s
complete physical-line segments. `Normalize` alone has no segments or groups.
`RepetitionVersion` pins `repetition-v1/normalization-v1/structure-v1`.

- `exact_original` compares the complete original bytes of each segment.
- `formatting_equivalent` compares the complete prepared segment bytes using
  only the guarded NFC, line-ending and whitespace rules above. It is emitted
  only when at least two originals differ. It includes every occurrence, so an
  exact subgroup can also appear within a formatting group.

Markers, indentation, punctuation, case, protected values, hard breaks and line
endings participate in comparison. A final line without an ending is distinct
from a line with one; no extra trimming or sentence splitting is performed.
Blank lines and separators can repeat too. Context is retained, not used as a
semantic equality test: identical wording under different headings/list parents
still has separately addressable occurrences. Each occurrence includes its
segment ID, normalized and exact original ranges, kind, parent ID, indentation
and heading level. Parent chains remain available through `Segments()`.

Two string-keyed indexes compare full strings after Go's internal hash lookup,
so hash collisions cannot establish equality. Work and index storage are bounded
by source bytes and segment count, with no all-pairs fuzzy matching. Exact groups
precede formatting groups; groups within each class and their occurrences follow
source order. Group IDs include the complete submission identity, comparison
version/type and first occurrence ID. They are not stable career-fact/entity IDs.
Calling the accessor never changes source identity, text, segments or mappings.

Annotations do not remove, coalesce, reorder, summarize or hide any content.
They confer no Jev acceptance, saved-Profile duplicate disposition or permission
to merge employers, roles, dates or projects. Future preparation/integration must
retain every occurrence; #40 and #41 own semantic matching and fact accounting.
Golden tests verify exact/format variants, bilingual negation, changed dates and
metrics, distinct technologies, repeated headings/boilerplate and parent context,
literal guards, source recovery, defensive copies and both current byte limits.

## Bounded preparation and portion planning (issue #59)

`LimitsFor(field)` exposes the existing compatibility limits, with no increase:

| Field | Complete decoded original | Encoded incoming HTTP body |
| --- | ---: | ---: |
| Professional information | 30,000 bytes | 163,840 bytes (160 KiB) |
| Job Posting | 30,720 bytes (30 KiB) | 131,072 bytes (128 KiB) |

`CheckRequestBodySize(field, measuredBodyBytes)` checks the actual encoded body,
including envelope/escaping. Callers retain bounded body readers and schema and
field-specific validation. Run transport checks before decoding, and
`PrepareBounded(original, field)` before normalization. The latter returns the
complete immutable original, its measured size, and either a `ready` prepared
`Source` with measured prepared size, or `insufficient_capacity` with the entire
original range unprocessed. Raw oversize never becomes admissible through cleanup
or repetition. A zero prepared size on raw rejection means preparation did not
run. Low-level `Prepare` remains available; `Source.Plan` independently rejects
sources exceeding the same raw caps.

`Source.Plan(Budget, PayloadEncoder)` is pure mechanical planning under
`portions-v1`. Its budget requires positive prepared-text bytes, provider-payload
bytes and maximum portion count. The caller supplies a **pure serializer** of the
actual final provider request, including all known prompt, saved Profile/job
context, envelope fields, source references and JSON escaping. The planner measures
those serialized bytes for each candidate; a text budget alone cannot establish
payload capacity. The serializer may run many times and must never perform I/O
or invoke AI. Errors propagate. Count budgets in UTF-8 bytes, never exact tokens;
this API provides no token estimate or provider context-window guarantee.

`Plan.Status == Ready` means the **whole single prepared view** fits both budgets.
Only in that case does the plan contain one complete view suitable for current
bounded workflows (subject to whole-original Jev acceptance). A smaller capacity
returns `InsufficientCapacity` even if `Plan.Complete` is true and multiple
portions mechanically cover the source. `Complete` measures original coverage,
never semantic extraction completeness, safety, or authorization. `SourceID`,
preparation version and planning version remain available for #41's coverage and
claim disposition ledger; semantic continuation remains its responsibility.

Planning preserves source order and prefers complete wrapped paragraphs, list
items/continuations, and runs of literal/table/quotation lines. On overflow it
falls back to original mapping boundaries in splittable prose/continuations.
UTF-8 characters and normalization groups remain indivisible, as do guarded
URL/email/code spans, protected blocks, heading lines and list-item wrapper
lines. This is a conservative syntactic policy, not a promise of optimal packing.
A protected block or required ancestor wrapper that cannot fit produces an
explicit `indivisible_or_context_capacity` result and the remaining original
ranges. Portion count exhaustion returns `portion_count`; no first-N success is
reported. Each portion covers at least one new mapping group, so both count and
storage are bounded by source size and the declared limits.

Each `Portion.View` carries its exact covered normalized/original ranges and
text. External syntactic ancestors appear outermost first in `Context`, with
`Role: "context"`, segment IDs, text and exact ranges. These ranges explicitly
overlap earlier source coverage and are **not** newly covered content; repeated
context bytes count against both text and serialized payload budgets. The
non-context original ranges of portions followed by `Unprocessed` partition the
complete original without omissions or overlaps. Repetition annotations never
shrink that accounting. All failure paths retain the original for correction or
later deliberate continuation.

Integration handoff for #60/#61: keep strict transport/schema/raw checks first;
Jev must classify the complete `Preparation.Original` once per protected route
attempt under existing bounds. Only whole-source acceptance permits downstream
AI. If Jev cannot classify the whole original, report that limitation; never
classify selected portions. Use a complete single prepared view only when its
caller-owned payload fits. This library neither gates acceptance nor calls any
provider; it introduces no fan-out, batching, retry, continuation UI, partial
success, storage or Profile mutation. Current handlers remain unchanged in this
slice. #41 owns execution/continuation and cross-portion semantic reconciliation.

Public-API tests verify exact raw/body/payload boundaries, multibyte crossings,
normalization expansion, indivisible groups/blocks, repeated heading/list context,
every repetition, many short segments, count failures and exact complete original
coverage. `FuzzBoundedPlanCoverage` also checks determinism and byte budgets.

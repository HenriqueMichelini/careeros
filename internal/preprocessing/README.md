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

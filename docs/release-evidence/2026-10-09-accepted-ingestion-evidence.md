# Accepted ingestion evidence — issue #39

Milestone: Evidence-backed Profile (#3). Implementation branch: `codex/evidence-backed-profile`; PR base: `main`. Blockers #25, #30, #38 and #60 were verified closed. Review baseline approved by Henrique: `386519de9793195ad7dd953c68662f3975bd30fa`.

## Behavior and boundaries

The existing bounded extraction/comparison calls now propose semantic qualifiers, complete contextual supporting excerpts and composite claim references. Every excerpt uses #60's preparation/source occurrence resolver. The client validates UTF-8 original ranges and binds source identities to the original submission; temporal wording and literal numbers/technology spellings have deterministic guards. Invalid/conflicting references remain unresolved. No additional provider call, semantic verifier, source normalizer, database or source archive was introduced.

`applyIngestionDocument` converts explicitly selected field operations into stable canonical facts/entities, accepted excerpts and same-Profile revisioned support/role/project/period links in one transaction. New entity anchors remain proposal-local until apply. Required identity operations and supporting claim dependencies must be selected; different role/project destinations cannot lend each other support. Existing experience candidates always include employer, role and period, including lexical candidates in other destinations. Unchanged appended blocks retain existing approved excerpts only when their identity/context remain unchanged. Complete replacements never reactivate prior evidence by lexical inclusion. Obsolete excerpts stay historical with invalidated links; a wholly new replacement can have its own accepted support. Unsupported retained text never acquires support merely from a new source pointer.

Observed terminology remains separate from canonical spelling, under `exact-alias-v1`. Only JavaScript/Javascript and TypeScript/Typescript have deterministic aliases. Other wording, uncertainty and alternatives remain in the accepted excerpts without forced normalization. Qualifiers are explicit; identity labels do not inherit a narrative claim's negation. Complete pastes, pending proposals, and source hashes are not persisted. Edited wording is user-authored with unknown qualifiers and no inherited proposal evidence.

The store persists before advancing Profile or clearing input/review. The expected complete canonical document, fact/entity revisions, allowlisted operations, source existence and selected dependencies are checked before apply; storage compares the expected document again under the existing Web Lock. Failure preserves the old saved document and transient review. Processing and display never write Profile. Before/after editing, rejection, explicit apply, server-owned whole-submission gating and deliberate retries remain.

Approved sources remain inspectable through Fact details and context after reload. A source pointer and the UI's approved-excerpt support label describe traceability and acceptance, not external verification or semantic proof. Literal guards are bounded: they protect numbers and selected technology/capitalized tokens, and are not a general semantic checker.

## Verification

Henrique approved the existing public ingestion HTTP handler, canonical apply/storage boundaries and controlled browser checks as TDD seams. Red/green slices covered accepted-only reload, composite references/partial selection, protected values, distinct role stints/ownership and exact alias wording. Tests also cover manual edits and all structured destinations.

- Canonical Profile suite: 39 public-boundary cases passed.
- Client ingestion suite: 22 cases passed.
- Ingestion Go handler suite and semantic controlled corpus passed after correcting phone-prefix and percentage literal handling.
- TypeScript checking and production build passed.
- Controlled ingestion browser matrix passed: English/Portuguese, 1440/390px, empty/populated Profiles; whole-submission decisions, edit/reject/apply, approved-only evidence after reload, recovery/quota, stale completions, delayed lock/save and real sibling-window storage conflicts.
- Rendered Fact Details source inspection after reload also passed. Browser screenshots: `/tmp/careeros-ingestion-browser-lC1lft` (local run artifacts).

All provider responses in these tests were synthetic controlled transports. No real provider calls were made; provider semantic accuracy, live latency and deployed semantic behavior remain outside this local verification. PR/deployment checks and final review results are recorded below after publication.

## Independent review corrections

Standards review reproduced obsolete evidence transfer and false unsupported state on complete replacements. Spec review additionally reproduced an approximate narrative blocked by undated supporting identity. Regression tests were added before fixing all three. Prior evidence now follows unchanged appended blocks and unchanged identity/context; new replacements use their own sources; semantic qualifiers come from operative narrative claims, while identity excerpts supply context. Entirely undated roles receive role context without inventing a period link. A subsequent polarity-change regression prevents lexical inclusion from transferring affirmative evidence to negated replacement wording. Final correction review is recorded in the PR/issue.

## Final review and validation

Standards: zero remaining actionable findings after correction review of `5669703`. Spec: zero remaining actionable findings after correction review of `5669703`. All discovered evidence-state and qualifier issues have regression coverage. The final complete Node and Go suites, TypeScript check and production build pass. Screenshots of rendered accepted excerpts after reload are retained in [English](issue-39/accepted-evidence-en-390.png) and [Portuguese](issue-39/accepted-evidence-pt-390.png). Browser evidence remains controlled; no live provider semantic claim is made.

# Explicit acceptance of Profile section rewrites (#38)

Baseline: `4efbff23533aa9142315936ca67f31858615f32a`. Milestone 3 implementation branch: `codex/evidence-backed-profile`; PR target: `main`. Native blockers #34/#37 were verified closed.

## Behavior

The section review HTTP contract now accepts a bounded canonical section snapshot and returns a transient Profile Proposal, never a replacement Profile. The old whole-Profile review client/handler contract is retired. Overview, education, certifications and languages remain unavailable for AI Review; overview behavior from #23 is preserved.

A request contains only the selected section's nonempty editable facts, their protected ownership/context identities, and active approved evidence needed for those facts. Identity, dates, URLs, qualifiers and other metadata are read-only. Every patch must cite its own current fact and every additional supporting fact. Composite support is limited to the same owner, context and meaning qualifiers; each original fact retains its own patch and identity. This preserves distinct roles and outcomes even when wording becomes similar. Nothing implicitly deletes a fact. An explicit per-fact removal checkbox requires deliberate user approval at apply.

Generation and a separate support/completeness check each use strict provider JSON Schema. They share one 25-second workflow budget, with at most two calls and no retry or fallback. Domain validation rejects changed metadata, wrong/stale/duplicate references, missing facts and invented numeric tokens. The verifier separately checks stronger qualifications, unsupported relationships, uncertainty/negation/aspiration, and completeness. Refusal and truncation have explicit localized errors. Both calls use existing content-free diagnostics stages.

The user explicitly approved the HTTP, canonical apply/save, and rendered workflow test seams, review baseline, and the separate bounded semantic verifier task. No real provider/credential use was authorized or performed.

The review shows before/after text, all cited original facts, role/project context identities, origin/support labels and accepted excerpts in EN/PT. Pending proposals are transient. User edits receive truthful user authorship, reset unverified meaning qualifiers, and invalidate old excerpt support. Unedited accepted wording preserves existing approved excerpts and revision-correct support links; unsupported sources are not promoted into proof. Source corrections conservatively invalidate shared excerpt support, so composite wording cannot retain a withdrawn source. Explicit acceptance compares the current canonical section/revision and atomically saves under the existing origin Web Lock. The store advances only after durable persistence. Failures retain both saved Profile and edited proposal. Cancellation, rejection, navigation and obsolete completions never save.

## Controlled verification

- Public canonical tests cover bounded section-only requests, apply, metadata-only staleness, invalid references, protected metadata, distinct-fact coverage, excerpt preservation, user-authored edits, deliberate removal, unsupported overview, and composite-source invalidation; existing migration/edit/save tests remain.
- Versioned fixtures: `backend/profile-review/testdata/section-rewrite.v1.json`. The production HTTP handler runs EN/PT valid cases and failure cases for metrics, stronger qualifications, missing meaning, missing/duplicate verifier checks, invalid/stale references, protected metadata, omitted facts, refusal/truncation and provider failures. A transport substitutes provider responses; these fixtures test enforcement, not model accuracy.
- Dedicated rendered matrix: `node tests/section-review-browser.check.mjs`, EN/PT × 1440/390px, review/edit/reject/apply, no generation save, selected-section request containment, quota retention, real sibling-window conflicts, cancel and stale navigation completion. PT 390px proposal screenshot was visually inspected with no clipping. Screenshots are captured by the script.
- Existing ingestion browser regression migrated from automatic section save to explicit reject/apply; all other ingestion behavior remains covered.
- Existing 34-case semantic suite ran with controlled production-handler adapters. Eight known label/contract failures remain in capacity and adversarial Application Draft fixtures. Those workflows are unchanged by #38. Report: `/tmp/issue38-semantic-quality.json` in this local session. Do not interpret controlled responses as live-provider quality or billing evidence.

No deployed behavior or live semantic accuracy is claimed. A semantic verifier is fallible; the user still inspects and confirms every claim before acceptance. Whole facts remain whole; this ticket does not introduce ingestion persistence, semantic decomposition, remote storage, vectors or model fallback.

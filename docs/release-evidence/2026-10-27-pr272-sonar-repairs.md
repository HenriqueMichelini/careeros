# PR #272: scoped Sonar triage and local repairs

Starting/reviewed head: `bc77c4beec5f6d1d476a0b5c1f0e5f27d6f8547a`. Original feature base: `591b2889196f277c96b130192a68bbc700a2137f`. Worktree: `/tmp/careeros-new-account-persistence`, branch `codex/new-account-persistence`. This additive report does not replace the original implementation or session-repair reports.

## Exact-head intake and dispositions

Sonar analysis `98d881e3-7ff1-4726-a933-5982261333cf`, processing task `AaF633Q-0OcfyZnmu-re`, explicitly identifies the starting SHA. The full issue API returns **55/55 findings**: 51 code smells, one reliability issue and three security-labeled findings. GitHub's check annotations contain only 50. The complete issue records, analysis identity, duplication blocks and all 55 individual dispositions are retained in [the evidence directory](pr272-sonar-2026-10-27/).

- **54 accepted/code-addressed**, with remote resolution still unverified: complexity, nested conditional expressions, diagnostic errors, readonly members/props, import comments, synthetic harness input types/language, browser polling and executable lookup.
- **One false positive on static evidence**: `AaF633wvOE1GLMOtxzMY`, `tssecurity:S8475`, reported at `src/lib/accountProfile.ts:110` in the starting commit. Its remote status remains unchanged. The proposed disposition below needs owner approval.

No threshold, exclusion, rule setting or suppression was added or changed. Temporary ESLint/SonarJS and gocognit installations live outside the repository; package manifests and lockfiles are unchanged.

## Storage-poisoning trace before behavior changes

The scanner's recorded source is `/api/auth/session` at `AccountGate.tsx:34`, under the scanner's assumption that the remote server is compromised. Its flow crosses the synthetic harness and the explicit Profile Save path, ending at `sessionStorage.setItem`. A storage write alone does not establish a dangerous interpretation or authorization bypass.

1. `AccountGate` probes the same-origin server session. Account selection follows that probe; epoch checks, account-scoped clients and request leases fence old replies and account switches.
2. `AccountProfile.save` validates the candidate Profile, ID and next revision, clones it and freezes the operation ID/base/document. The checkpoint key is the fixed `careeros_pending_profile_v1:` namespace plus that account ID. It stores JSON recovery data; it contains no credentials and makes no saved-state claim.
3. Restoration parses JSON and validates the envelope version, account match, operation syntax, safe nonnegative expected revision, Profile shape and next revision. Invalid data remains recoverable without adoption as a pending Profile. It is not executed.
4. `StoreProvider` exposes reconstructed pending wording only after `account.open()` successfully completes a protected current server read. The server document remains `documentRef` authority; recovery wording stays an unsaved draft. Failed reads do not authorize browser fallback. Account changes dispose the previous lease and workspace.
5. Profile wording is rendered through React text children and controlled form values, including `RepositoryPage`'s fact text, not `innerHTML` or code evaluation. Downloaded recovery is an application/json Blob.
6. Restoration does not send a PUT. Deliberate retry sends the frozen operation with same-origin credentials, CSRF and account consistency headers. `accountapi.New` derives ownership from the authenticated Principal and rejects client ownership fields. `accountstore.authorized` rechecks locked account/session status, generation and DB-clock expiry; replay/hash, revision and provenance checks still run transactionally.

**Verdict: false positive / not actionable, high static confidence for this specific claim.** Inert, deliberately untrusted recovery JSON does not cross the alleged security boundary. Sanitizing or stripping career wording at storage would violate the lossless recovery contract. No checkpoint behavior was changed to silence this rule. No applicable `SECURITY.md` was found; the boundary is established by `docs/new-account-persistence.md` and the traced code. This is not an exhaustive security audit, proof against a separate XSS, or production authentication evidence.

Proposed Sonar action, not executed: mark only issue `AaF633wvOE1GLMOtxzMY` **False positive** with this rationale: “This checkpoint is intentionally untrusted, account-scoped JSON recovery, not an execution or authority sink. Restoration validates envelope/account/operation/Profile; a current protected server read gates display. React renders wording as text. No mutation is replayed automatically; deliberate retry independently passes server session/CSRF/account/revision authorization. Preserve lossless wording rather than sanitizing the checkpoint.” Reassess the exact issue/flow after publication if Sonar creates a different finding; do not blanket-suppress S8475.

## Repairs and duplication

| Starting duplication | Local repair |
| --- | --- |
| Browser drivers: identical 158-line block in each, lines 8–165 | Shared bounded browser fixture. Common expected outcomes remain explicit in the fixture and execute for every caller. Scenario-specific assertions remain in each driver. Chrome uses fixed `/usr/bin/google-chrome`; failures still throw. |
| `i18n.ts`: eight new duplicated lines | Move exactly the four new keys per locale into an account JSON catalogue using the existing catalogue pattern. All 1,742 locale/key entries compare equal to the starting commit. |
| `vite.config.ts`: four new duplicated lines within repeated proxy blocks | Shared proxy factory; all eight routes, targets and changeOrigin values match both original maps. |

Extracted presentation components use native output elements and an open nonmodal dialog. Logout choices, labels and recovery behavior are unchanged. The synthetic transport rejects unsupported body types rather than default-object stringifying them; supported command strings retain their bytes. Diagnostic messages and readonly annotations do not change product decisions.

Go helpers separate strict JSON string/surrogate validation, completed-operation replay, stale-base validation and retention checks without moving authorization, locks, receipt recognition, publication or commit relative to one another. Existing test scenarios move into named fixture helpers without removing their failure or positive-control assertions. The permanent natural-expiry test body is byte-identical to the starting version. The production initiating-session replacement code changes only blank-import comments; its DB-clock ordering is untouched. Additional JSON cases cover quote/backslash handling, surrogate-pair boundaries, malformed pairs and invalid keys.

## Fresh verification of the recovered final source

- **PASS:** affected `internal/accountapi` and `internal/accountstore` tests under `-race -count=1`, with real, isolated, loopback-only PostgreSQL and synthetic OIDC/documents. Covers explicit Save, isolation, races, replay, process restart and actual HTTP response loss.
- **PASS:** natural initiating-session expiry while blocked; both logout/replacement orderings; valid replacement despite a deliberately incorrect caller clock; other-device and fresh-sign-in controls.
- **PASS:** reconstructed original independent expiry overlay, against unchanged original review source plus its exact recorded probe. It reports `expired=true`, `replacement_created=false` and invalid transaction under the race detector.
- **PASS:** 77 directly affected frontend contracts, including all nine account-client contracts, Profile representation/provenance and reviewed submission Save.
- **PASS:** actual React/Chrome replacement, expiry/reauthentication and lifecycle scenarios; existing account-switch/late-response/EN/PT scenario. Assertions retain pending wording, signed-out/B-account isolation, exact operation bytes, no automatic PUT, deliberate retry and logout cancel/keep/discard/reload.
- **PASS:** TypeScript, targeted Go vet, and builds in both account and browser modes with outputs outside tracked dist. Existing Vite future-loader and large-chunk warnings remain.
- **PASS:** all translation entries and both Vite proxy maps are equivalent to the pinned starting source; source whitespace checks pass.
- Local SonarJS cognitive-complexity checks no longer report the targeted functions. Three unchanged nested conditional reports remain in older lines of JourneyGuidance/store; baseline comparison is retained. Go gocognit reports the targeted refactors below 16; its three remaining reports also exist at the starting head and were not reported by this Sonar intake. These tools are supporting local evidence, not a hosted Sonar quality-gate result or substitute for reanalysis.

## Evidence preservation and limits

On continuation, all task `/tmp` directories were absent, including the uncommitted repair worktree, original raw evidence directories and temporary tool installations. The reviewed Git commits and durable session records survived. The worktree was recreated from the exact starting head and the recorded edits replayed; the original two review documents were reconstructed from their recorded Add File patches. The independent probe was reconstructed from its exact recorded creation command and immutable reviewed source. This event is documented in `recovery-provenance.json`.

The original committed implementation/session-repair reports, persistence contract and Evaluation Lab manifest remain byte-identical. The former 108-file raw `/tmp` inventory cannot currently be fully reverified; earlier preservation claims remain historical. New final-source checks were rerun and their actual outputs retained here. A dependency-recovery mistake created a nested node_modules symlink and initially loaded two React instances; removing only that link restored the browser checks, without a product/config workaround. The failed diagnostic remains in the temporary evidence directory. An equivalence-check subprocess was initially sandbox-blocked and passed through permitted execution; no assertion was weakened.

The Evaluation Lab manifest is still SHA-256 `c3fb7be93b892adca689c9798370d89bac305454ae3268055488f25376e7c595` with **34 stale paths**: 28 at original published base, 34 after the feature, 34 after P2 repair, 34 here. The six feature-owned additions remain attributed to that feature. No manifest editing/reapproval occurred. The retained **13 Evaluation Lab top-level failures remain unresolved**; no whole-Go or full frontend suite was rerun or declared green.

Production Google/provider/runtime/TLS/grants/pool, whole hosted-route account enforcement (including the five older AI Functions), retention and deletion-aware disaster recovery remain outside this bounded local slice. No live provider call, provisioning, spending, deployment or launch occurred.

## Publication boundary

This is a local commit only. Remote Sonar still describes the starting head's failed gate (8.2% duplication, C reliability, B security); local repairs do not establish a passing remote result. No push, Sonar disposition/configuration write, ready transition, merge or issue closure is authorized by this report. The proposed source publication and PR text are presented to the owner separately. After approval, freshly authenticate/read back Netlify **Builds are stopped**, push only the approved SHA, and verify Sonar/GitGuardian against it. Merge approval remains separate.

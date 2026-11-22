# Durable review and accepted Application Draft snapshots

This local synthetic source slice implements [#284](https://github.com/HenriqueMichelini/careeros/issues/284) over cumulative reviewed base `e4fd48d2e425a3a7b1492a4263e00d130be4e98f`. Its [frozen interface](application-draft-review-contract.md) consumes the exact [#281 recovery contract](application-draft-recovery-contract.md), the selected [historical review/Accept](https://github.com/HenriqueMichelini/careeros/issues/284#issuecomment-6645202436) and [historical export](https://github.com/HenriqueMichelini/careeros/issues/284#issuecomment-6644605240) rules. Opening/allowance authority remains the separately injected [selected contract](https://github.com/HenriqueMichelini/careeros/issues/285#issuecomment-6644976301); this slice neither implements it nor infers delivery from readiness or Accept.

The existing authenticated controlled/managed Draft constructors mount the exact-version review and accepted-snapshot reads. No environment configuration enables server production composition, hosted accounts or provider egress. All verification uses synthetic Profiles, injected decisions, fake/controlled callbacks and private disposable PostgreSQL.

## Decisions and retained data

The server stores four ordered artifact decisions (`summary`, `resume`, `cover`, `answers`) against one owner/Action/bundle version/published digest. Each needs a deliberate review followed by deliberate Accept. A none-requested employer-answer state is explicit. Review does not edit the generated bundle, change its support snapshots, save application-only context to Profile or affect customer allowance/provider accounting.

Commands carry a stable opaque decision key and expected review revision. Equal-key replay recovers one effect; changed bindings and stale competing revisions conflict. The original immutable bundle remains the accepted wording. An accepted snapshot becomes complete only after all four artifact Accept decisions. A newer unaccepted/failed candidate cannot replace it. The accepted reader chooses a committed accepted identity and runs the same protected exact-result fence through the outer response write. Exact-key/version reads remain available for the deliberately selected snapshot.

The new purpose-owned review state and command bindings reference existing complete content; they retain no new copy of the Profile, prompt history, partial generation or duplicate generated text. Content cleanup cascades to its review/command records without erasing unrelated Profile or independent financial/replay obligations. Existing restricted AI-ledger runtime roles receive no access to these retained tables, including through inherited default privileges.

## Authority and recovery

Writes use the existing account/session and shared Action/content locks, trusted database time and final read/delivery/deletion checks. Duplicate decisions keep the same final authority fence. Lost acknowledgments recover the committed review revision and exact bundle by GET. Inference and independent completion callbacks are never invoked by decisions, reauthentication, accepted-history lookup or export.

A rejected decision that observes a winning closed/deleted terminal fence commits that purpose closure before returning denial, consistently with the existing Read/Publish contract. No human decision write has happened at that point. A later apparent authority restoration or opening flag cannot revive the Action. A temporary nonterminal read/session denial still rolls back and permits later authorized exact recovery. This neither settles customer allowance nor releases provider expense/capacity.

Already delivered complete snapshots may be reviewed/Accepted/exported after Profile changes within their approved read window. The original source revision, frozen signature, support snapshots and Action/version remain visible. A newer incomplete Action's status check also refreshes separately authorized accepted history, keeping its historical disclosure current. Earlier closure/deletion and post-grant expiry remain authoritative; no new retention clock is introduced.

The browser keeps only its existing account-scoped opaque creation key in session storage. Decision keys and unsaved user wording remain in memory; protected recovered content is not copied into the unsaved editor. Current, previous and accepted protected views are withheld on 401/403/account denial. Same-account reauthentication preserves the opaque key and independently entered wording. A foreign account cannot inherit that state.

## Exact accepted exports

Display, clipboard and local PDF use the selected accepted snapshot. Export performs an authenticated exact-key/version GET before accessing the clipboard or print DOM. It cannot substitute a newer candidate or today's Profile identity. Metadata discloses original source revision/Action and historical state. Resume rendering reuses the existing A4 preview with frozen identity; other artifacts use the existing print layout. Overflow includes the disclosure and stops printing instead of clipping. Clipboard denial/fallback failure is announced, and keyboard focus returns after the control becomes enabled. Temporary print DOM is cleaned after printing or unmount/denial.

The bilingual real-browser schedule writes and inspects actual one-page PDFs at desktop and 390px, checks trusted Tab/Enter and real clipboard permissions, and separately tests denied clipboard/fallback and forced layout overflow. These controlled fixtures establish local behavior, not arbitrary generated-layout or live model-quality guarantees.

## Legacy two-tab currentness

The required [#135](https://github.com/HenriqueMichelini/careeros/issues/135) reproduction uses two real same-origin synthetic browser tabs. A canonical Profile change from revision1 to2 left the first tab's old document with `profileError=stale`, while copy/PDF remained enabled and its generic message instructed regeneration. Canonical stale writes were independently rejected. The selected historical policy satisfied [#136](https://github.com/HenriqueMichelini/careeros/issues/136)'s conditional UX input; this slice corrects only Results export eligibility/disclosure. Original accepted browser wording/signature stays unchanged, including after canonical refresh. This is browser-local evidence, not durable delivery/accepted-account evidence or a remote issue disposition.

No real invitations, grants, participants, provider accounts/calls/spending, hosted recovery, deployment or GitHub publication occurs in this work. Independent exact-head reviews and the final verification report determine local completion; this guide alone asserts no passing release gate.

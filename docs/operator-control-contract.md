# Separate local stop and reopening contract

This engineering contract implements [#290](https://github.com/HenriqueMichelini/careeros/issues/290) against [the selected owner matrix](https://github.com/HenriqueMichelini/careeros/issues/290#issuecomment-6645259323). The reviewed cumulative base is `49fb63cb1d25b0d4cdfebbec3639710d1913a3f5`. All executions are synthetic accounts, fake providers and disposable PostgreSQL. There is no hosted authority, live provider, participant, grant, notification or deployment activation.

## Authority and public readers

Persist global invitation/inference controls and account-scoped compromise controls with monotonic versions and an immutable command receipt. A missing, malformed or unavailable control authority denies the operation; ordinary support and reconciliation cannot mutate it. A distinct database-owner-installed local operator identity plus its live exact CareerOS session authorizes commands. There is no public operator/bootstrap endpoint. A normally buildable, loopback/test-database-only command exposes the bounded operator surface.

Fresh schemas start closed. An explicit code-owned fresh-synthetic initialization capability may establish the initial healthy fixture only; it cannot reopen an already initialized/stopped authority or stand in for restored-system evidence. Migration never clears an existing stop. Runtime cannot write controls, elevations, receipts or reopening evidence.

| Control | Denied | Independently authorized paths preserved |
| --- | --- | --- |
| Invitation | Issuance, reissue, redemption and activation, including replay responses claiming activation authority | Existing grants' inference and retained content rights |
| Inference | New Action/attempt admission and every actual provider handoff, including later legs and aliases | Identity/status, rights/deletion, retained reads/exports/review/Accept and complete-result publication under their other current fences |
| Account compromise | Affected invitations, sends, ordinary sessions, content reads/writes/publication/opening/review/Accept/export | Newly verified, separately scoped restricted rights/deletion/status only |

Controls compose with the broader denial winning. An inference-only stop does not close the existing independent result/recovery gateway. A compromise hold is a pause, not early access revocation: no grant, original access-end or content-recovery clock moves. No stop releases provider money, unknown remote occupancy, counted actions or consumed customer allowance.

## Ordering and actual fake handoff

The global control fence precedes ordinary account/session and namespace locks for managed admission. A stop command obtains the matching exclusive fence before its current actor/account checks and mutations. Admission and attempt creation hold a shared transaction fence through their final current-state check and commit, preventing a stopped admission from committing after the winning stop.

Actual fake handoff additionally uses a shared PostgreSQL session-level dispatch lease, acquired before the final account/session/namespace transaction. The final current control check is after lock waits. The lease spans the fake callback; it does not hold a database transaction across inference. A stop's exclusive lease must win before its control receipt commits. Thus a callback already inside the boundary orders before the stop; an old durable intent or delayed permit entering afterward is denied without resend. A blocked already-entered callback can delay the local stop acknowledgment; neither a requested stop nor a timed-out operator call is reported as a winning committed stop. There is no claim that this local callback lease proves physical exclusion of a live provider/old hosted process.

Protected status/result writes keep their existing account/session/current-source/deletion/deadline checks through the actual outer write. The inference control is checked only on admission/dispatch, not converted into a blanket career-read denial. Compromise serializes with affected account writers and revokes old ordinary sessions. Terminal closure applies once to unconfirmed Actions and releases only their unconsumed customer reservation. Delivered effects and independently authorized historical content are not relabeled as unconfirmed work.

## Reopening and restricted recovery

An idempotent command binds the actor, exact target/control, expected control version and non-content recovery/reconciliation evidence. Every call separately requires the actor's current exact session after locks; a later authorized session can recover that actor's unchanged receipt without repeating its effect. Reopening requires explicit operator authorization and an injected code-owned verifier for the local recovery checkpoint. A label, elapsed time, key rotation, a restored database generation or traffic removal is not such evidence. Where old-system exclusion applies, preserve `Q → durable decisions/reconciliation → whole-old-system exclusion → generation advance`; the independent synthetic gateway must remain closed until the checkpoint validates. No future hosted drill is an upstream implementation gate, and no live opening verifier is provided.

Reopening advances the fence and invalidates older permits; it never revives terminal Actions, replays durable intents, refills allowance or resets clocks. Compromise suspension ownership/version is distinct from unrelated suspension/revocation. Clearing this control cannot clear a different hold or extend an expired grant. Old sessions stay revoked; restored ordinary access requires fresh sign-in and current independent rights.

A fresh completed identity verification for a compromised account issues a separate restricted session. It cannot authenticate ordinary workspace, inference, Profile or export endpoints. The restricted process offers minimized status and explicit confirmed deletion, not content recovery or automatic grant admission. Its mutation checks retain origin/CSRF, exact scope/session, ownership and post-lock trusted time.

## Agreed TDD seams and independent expectations

1. First vertical tracer: real operator command, invitation issuance/activation and authenticated status on disposable PostgreSQL. A winning invitation stop denies issuance/activation while a separately active grant retains its rights; duplicate command recovers its immutable receipt and a changed binding conflicts.
2. Shared admission plus actual fake callback, canonical routes and Function/account aliases: two independently initialized instances, old durable intent, blocked account/control locks, later-leg stop and both winning orders. Count actual callbacks, not merely HTTP status or a configured bit.
3. Account compromise: affected vs independent account; ordinary reads/Save/export/Draft publication/opening/review denied, unconfirmed customer reservation released once, delivered effect retained, provider money/occupancy unchanged. Fresh restricted sign-in/status/deletion succeeds without ordinary access.
4. Operator/session/role negatives, post-lock expiry, missing/unavailable authority, command acknowledgment loss/restart, explicit denied/unproved reopening and verified local reopening. Old permits and terminal Actions stay denied after reopening.

Tests use selected literal outcomes and frozen source identities. Raw financial/replay state may be inspected by the restricted synthetic fixture to prove preservation; ordinary status contains no career content, provider linkage, credential or operator evidence. Physical PostgreSQL/process restart evidence, when run, is distinguished from Harness reconstruction. Final evidence records original REDs, exact runtime/fixture hashes, skips, retained full-Go/vet failures and independent immutable Standards/Spec/security review.

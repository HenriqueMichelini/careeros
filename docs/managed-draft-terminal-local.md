# Local managed Draft cancellation and Retry

This source composition implements [#286](https://github.com/HenriqueMichelini/careeros/issues/286) against the [frozen terminal contract](application-draft-terminal-contract.md). It combines the reviewed managed Draft graph with the real finite-five pilot reservation/opening protocol. It is limited to synthetic accounts, fake providers and disposable loopback PostgreSQL. Ordinary managed constructors, Function startup and deployment configuration do not activate it.

`draftrecovery.OpenManagedTerminal` and `accountapi.NewSyntheticTerminalDraft` are explicit code-injected local entry points. They retain the existing fake Harness and private-schema guards. They add no credential, provider-account, public operator, remote termination or allowance override. No new content table or retention class is introduced.

## Deliberate cancellation

After exact GET status or a confirmed create response, `POST /api/drafts/{creationKey}/cancel` accepts `{ "action": <the exact server Action> }`. The account session, owner, namespace, Action ID and workflow must match. Unknown fields are rejected. The account Function alias uses the same handler. A confirmed cancel returns the exact authoritative Result: closed/deleted with no content when a terminal event won, or the unchanged delivered Result if complete opening already won. Duplicate cancellation changes neither provider accounting nor customer rights.

The command holds current account/session, pilot grant and shared Action authority, then rechecks the selected fences. The existing terminal loader persists closure, queues purpose-bound cleanup and releases the actual pilot reservation once. It does not fabricate a provider termination signal or release unknown money/call occupancy. Late completion cannot publish after the winning terminal event.

The UI presents cancellation only in this explicit composition. A lost acknowledgment withholds the selected current Action and disables its open/Retry/cancel/create commands until GET resolves it. Independent accepted or partly reviewed work uses its own exact identity and read authority. There is no automatic command resend.

## Deliberate Retry and terminal status

A known eligible field-validation/generation failure is recovered by GET before the existing deliberate Retry command. The browser retains one opaque Retry key. Attempt two keeps the original Action, full input/source/policy binding, durable admission and original 30-minute customer window. The shared kernel still enforces fresh money, capacity and grant authority; all reached intents/exposure remain. Unknown or running intent, response loss, uncertain support and successful generation do not become Retry authority. Changed input and generate-another are deliberate independently admitted new Actions.

A Retry that first observes cancellation/deletion commits that terminal observation before denying the command, including the gap after shared Retry admission and before the purpose witness. A later apparently permissive injected adapter cannot revive it.

Opening or Retry can return an Action-specific403. In explicit terminal mode, the client first withholds every protected cached copy and then performs an exact same-Action GET. Only validated closed/deleted metadata resolves the current Action. That metadata does **not** establish career-read rights: the workspace restores earlier accepted/partial content only after its own fresh authorized read. Genuine401/403 keeps all protected copies hidden while preserving opaque recovery identity and independent same-account editor input. Account changes use isolated clients/editor scopes.

## Evidence boundaries

The permanent public tests use real disposable PostgreSQL and fake provider boundaries. They compare customer effects separately from provider exposure, test both opening/terminal orders, exact identity/alias negatives, the original time anchors, unknown in-flight occupancy and Retry no-revival. Two independently initialized child server processes contend through actual PostgreSQL locks. A real fsync-enabled private PostgreSQL immediate restart and a fresh server process recover exact winning Result bytes across canonical/version/account-alias reads with a synced four-stage no-resend witness.

The older-purpose-row test restores only purpose/allowance rows while independently retaining the current shared terminal Action and admission guard. It establishes no hosted/whole-database restore safety. Existing A32 tests retain their selected installed-table scope; there are no new tables here.

Actual EN/PT React → Vite → authenticated Go/private PostgreSQL schedules exercise known-failure Retry, lost Retry/cancel replies, full opening and accepted-history recovery with exact provider/effect counts. The denied-state tests distinguish a supported synthetic pilot suspension from a direct test-only `invitation_revoked` restriction. The latter verifies the authoritative adapter fence; it does not add an operator transition for revoking activated invitations. The paused terminal GET proves cached content is withheld before ambiguity resolves, and an independent accepted-history403 still denies access.

No local result is live provider/billing compatibility, real participant admission, production activation, hosted recovery or launch readiness. Full Go baseline failures and tool limits are recorded separately in the release evidence.

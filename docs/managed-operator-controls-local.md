# Local operator stop and reopening composition

This implements [#290](https://github.com/HenriqueMichelini/careeros/issues/290) and [the selected owner matrix](https://github.com/HenriqueMichelini/careeros/issues/290#issuecomment-6645259323) over reviewed base `49fb63cb1d25b0d4cdfebbec3639710d1913a3f5`. All executions use synthetic identities, controlled fake providers and disposable PostgreSQL. No provider credential, live account, participant, email transport, deployment or hosted recovery is installed.

## Separate controls

Invitation stop blocks issue/reissue/activation, including aliases and replay, while current independently admitted grants retain their own rights. Inference stop blocks new admission and every later provider leg, while authorized complete-result publication, retrieval, review, Accept, export, deletion and status retain their existing fences. Compromise is account-scoped: ordinary sessions are revoked, its grant gets a separate owned suspension, unconfirmed Actions become terminal with zero customer consumption, and ordinary career access is denied. The original grant/access-end clocks do not move. Broader independent holds always win.

A fresh verified identity for a compromised account receives a separate restricted session. `/api/rights/status` returns metadata only. `/api/rights/deletion/operations` accepts explicit confirmed account closure with the existing origin/CSRF and final authority checks; it grants no ordinary Profile/export/workspace access. The React gate withholds the mounted ordinary workspace and exposes the restricted experience in English and Portuguese. Vite forwards the exact rights prefix. Account deletion remains its own existing lifecycle, with no new retention class.

## Dispatch and financial boundaries

The PostgreSQL control advisory fence is acquired before account/session and namespace locks. Actual fake callback entry has a pinned-connection shared session lease, with final transaction validation on that same connection. There is no database transaction across inference and no second pooled connection needed while the lease is pinned. The lease spans the entered callback; an exclusive stop waits for it. A timed-out or requested command is not a committed winning stop. An intent delayed after its earlier check must acquire the lease and recheck current authority before callback entry; a winning stop leaves that intent unknown and non-resendable.

Stop and compromise never clear provider allocations, money, remote occupancy or replay identities. Independent restricted reconciliation can record late attributable U+D and above-bound evidence while participant access and inference are closed. Containment remains a separate reconciliation capability. Customer reservation release affects only undelivered customer rights; delivered effects survive unchanged. Local callback tracking and conservative unknown remote tracking remain separate.

## Operator capability and commands

`careeros_control_operators` is a separate owner-installed role from pilot support and reconciliation. Raw runtime/reviewer roles cannot read or change control markers, receipts, elevations or evidence. Only fixed-path boolean/epoch predicates are exposed. The current ordinary or freshly restricted exact operator session is checked after locks; a historical session token is never promoted by reopening. Command identity binds actor, exact control/target, expected version and exact recovery reference. Later freshly authorized sessions can recover that actor's unchanged command receipt without another effect.

`cmd/control-local` accepts only `--synthetic-only` and a disposable loopback `*_test` database. It reads `CAREEROS_CONTROL_LOCAL_DATABASE_URL` and `CAREEROS_CONTROL_LOCAL_SESSION_TOKEN` process-locally and never prints them. Schema migration is separate. Examples below contain placeholders, not credentials:

```text
rtk proxy go run ./cmd/control-local --synthetic-only --action bootstrap --subject synthetic-owner
rtk proxy go run ./cmd/control-local --synthetic-only --action initialize --operation-id fresh-empty-fixture
rtk proxy go run ./cmd/control-local --synthetic-only --action status
rtk proxy go run ./cmd/control-local --synthetic-only --action command --control inference --stop --version 1 --operation-id stop-inference
rtk proxy go run ./cmd/control-local --synthetic-only --action command --control compromise --account <opaque-account> --stop --version 2 --operation-id stop-compromised-access
```

Bootstrap installs one existing synthetic operator identity using the database-owner local capability; it is not an HTTP endpoint. Initialize is allowed only for empty, never-initialized synthetic authority with no historical grant/namespace/Action/dispatch. It cannot reopen an older installation. The CLI has no recovery verifier: an arbitrary reopening reference fails closed. An unchanged already committed receipt can be recovered with its exact command fields.

## Verified local reopening

`NewSyntheticControlRecovery` is an explicitly injected, code-owned non-restored checkpoint registry. It checks the same physical database/schema, every installed namespace, conservative ledger/scopes and existing reconciliation containment. It retains only bounded opaque references and numeric ledger-observation digests, no career content or duplicate billing dataset. An unknown reference, changed ledger/namespace/gateway, omitted installed namespace or unsupported state denies reopening. The supplied independent gateway set is an explicit code-owned local inventory; discovery of unregistered external instances is not claimed.

Routine healthy local reopening requires current recovery/reconciliation observation and explicit actor command. If an independent gateway is closed or mismatched, all supplied local gateway instances must be closed and actually drained. The sequence is quiescence → durable observations → independent whole-gateway exclusion → database generation advance → explicit independent gateway completion. Database generation alone cannot provide exclusion. The final proof comparison and opening lock every supplied gateway together, so a newer independent Close cannot be overwritten between validation and use. Duplicate completion does not invalidate a later current lease. Old Action epochs do not become fresh provider permits after reopening; explicit current known-failure attempt creation remains separately bounded by inherited authority.

This local registry cannot prove a hosted system's complete old-writer inventory, physical provider termination or external recovery. No live opening verifier is provided. A32's inherited ledger snapshot schedule is distinct from the dedicated new control receipt/compromise restart proof; this work does not claim that its old selected table inventory restores every new purpose/control table.

## Verification

The public-router tests exercise both actual fake-entry/stop winning orders, queued exclusive stop with pinned validation, a one-connection pool, aliases, later-leg denial, complete qualification/Draft publication under inference-only stop, delivered versus unknown compromise outcomes, fresh restricted deletion, independent holds, explicit source-owned reopening, missing/forged proof, and separate role/CLI controls. The browser uses the actual App → Vite → authenticated Go → disposable PostgreSQL path; its private token-protected compromise adapter exists only in a test binary. The physical test uses fsync-enabled private PostgreSQL immediate restart and a fresh child process; exact restricted status/command receipts and the durable one-call witness survive with 1344 microUSD/one unknown occupancy retained. This is not hosted recovery evidence.

The release evidence binds exact source/fixture hashes, raw receipts and original RED/harness failures. The full Go suite remains subject to inherited Lab failures and default vet diagnostics; focused passing checks are not an all-suite green claim. Independent immutable Standards, Spec and read-only security reviews are coordinated after the review-ready commit.

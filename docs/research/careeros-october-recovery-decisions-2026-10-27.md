# October recovery decisions for #294 and #295

The 27 October 2026 local research recorded the following owner decisions and design inputs. The 24 November decision selects the platform for local #294 preparation. Hosted verification and the #295 restore have separate approvals. [#261 hosting investigation](https://github.com/HenriqueMichelini/careeros/issues/261) · [#294 owner decision](../closed-pilot-hosted-candidate.md#owner-decision) · [#295 design decision](../hosted-restore-drill-design.md)

## Hosting reference

On 27 October Henrique selected Cloud Run and Neon in São Paulo as the reference for investigating recovery and complete cost. That reference modeled the production custom-domain load balancer and a separate synthetic preview. It was a research selection. The current #294 configuration proposal uses a default HTTPS origin for an isolated synthetic candidate. [#261](https://github.com/HenriqueMichelini/careeros/issues/261) · [Current #294 proposal](../closed-pilot-hosted-candidate.md#prepared-configuration)

## Recovery point and authentication reset

- The latest independently recoverable raw content point should be at most 300 seconds before the incident for comparison purposes. Recover exact still-authorized acknowledged Profile saves and exact delivered Drafts and reconcile authoritative delivery, allowance, expense, revocation and deletion effects independently. Hosted recovery-point and recovery-time targets require approval for the selected exercise. [#295 blocker audit](https://github.com/HenriqueMichelini/careeros/issues/295#issuecomment-6644635505)
- Henrique approved invalidating every pre-restore CareerOS Session and pending one-use OIDC transaction after any database restore or rewind. Require fresh Google sign-in afterward. Ordinary application-process restarts retain their established Session behavior. Preserve original deletion and copy-expiry clocks. [Account persistence decision](https://github.com/HenriqueMichelini/careeros/issues/262#issuecomment-6430952088) · [Recovery-copy policy](https://github.com/HenriqueMichelini/careeros/issues/258#issuecomment-6324994944)

## Old-system exclusion

Henrique approved further design of an offline cutover using a fresh Neon project and a controlled logical copy on each cutover. The design requires independently observed terminal exclusion of old direct, pooled and privileged SQL clients and old dispatch and response transports. Account for held work and all reconnect or restart paths. Keep the target closed whenever any required evidence is missing. Prove exclusion before advancing the recovery generation. Require the owner’s approval of the verified candidate and reconciled evidence before reopening. The October research left terminal exclusion, independent current authority and exact-content reconciliation unproved. [Approved stop and reopening contract](https://github.com/HenriqueMichelini/careeros/issues/290#issuecomment-6645259323) · [#295 decision handoff](https://github.com/HenriqueMichelini/careeros/issues/295#issuecomment-6645317791)

## Local source records

These are the filenames, sections and SHA-256 digests of the October source documents. The selected decisions above are preserved here for use from a standalone checkout.

| Local source filename | Sections | SHA-256 |
| --- | --- | --- |
| `careeros-hosting-postgresql-addendum-2026-10-27.md` | §§2, 10 | `df47c259324e3b97cfc666fc7c7693c68bd498896b2c81f50736e8d64d5577a8` |
| `careeros-disaster-recovery-addendum-2026-10-27.md` | §§1, 10 | `07a5086df0e35b79f2cad61e2158beca2d9cf16b589dc5c688e6d718e13e4799` |
| `careeros-mechanism-a-recovery-fence-design-2026-10-27.md` | Verdict, §5 | `e619357326f08e97207450551a0c0c725bbcde34e792102689cde68541ab66e3` |

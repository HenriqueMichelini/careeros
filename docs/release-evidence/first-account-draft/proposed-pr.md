# Exact proposed PR text — not published

Title:

```text
feat(account): connect saved Profile to the first Application Draft
```

Body:

```markdown
New accounts previously entered the legacy landing flow, and the five AI Functions bypassed account authorization. Connect the existing account Profile, Apply and Results screens so a new account can review and explicitly save Profile changes, paste a job, deliberately generate a Draft, review support, accept artifacts and export. A standalone CV stays optional.

All five AI routes now require session/Origin/CSRF checks through both public aliases and direct Function URLs. Analysis and Draft generation must match the session owner's current saved PostgreSQL Profile; changed or foreign snapshots fail before inference, both artifact reviews are required, and late results are rechecked before returning. Failures/cancellation retain existing results, retries are explicit, and Results can switch to one previous candidate retained in this session. Generation does not save Profile facts or personally accept artifacts.

Cross-language tests compare 44 shared Profile cases through the real Go and TypeScript evidence and legacy-view projections. They caught and repaired Unicode whitespace and opaque-entity-field mismatches without changing stored wording or relaxing evidence/input gates.

Validation: 360 frontend contracts including all 44 parity cases; TypeScript; scoped Go vet and affected Go race checks. The parent implementation's retained evidence covers real PostgreSQL/auth/Functions, production Draft and qualification tests, account and Lab builds, composed EN/1440px and PT-BR/390px browser journeys with synthetic OIDC/AI, account/session and EN/PT support review, native clipboard and A4 PDF exports. Reproduction, exact evidence scope and regression logs: docs/release-evidence/2026-10-27-first-account-draft.md.

The same 13 Evaluation Lab top-level failures occur at base and candidate. Reviewed-source manifest drift grows from 34 to 45 paths; no manifest reapproval is included. Existing Vite config/chunk warnings remain.

Account UI still requires VITE_CAREEROS_ACCOUNT_MODE=1. The legacy browser-local UI remains, but guest AI requests are now denied. Account AI keys remain session-only. Drafts/jobs/application-only answers and the previous candidate are transient; reload/sign-out loses them. No durable Draft recovery, authoritative cancellation/delivery receipt, payments/allowances, legacy import, recovery-fencing work, live provider validation, deployment or hosted launch readiness is included.

Implements a bounded product continuation of #255 and the merged #271 foundation. It does not close the broader SaaS investigations or their remaining gates.
```

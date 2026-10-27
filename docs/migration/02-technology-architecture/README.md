# Technology and persistence architecture

## Selected local implementation target — 26 October 2026

The owner selected **PostgreSQL with application-validated JSON bytes (`bytea`) for Profile content**, in response to the storage-choice question in this implementation conversation. This supplements the [React/Vite and Go decision](https://github.com/HenriqueMichelini/careeros/issues/259#issuecomment-6391838343). Google-only, direct Go OIDC and application-owned opaque sessions remain selected under [#262](https://github.com/HenriqueMichelini/careeros/issues/262#issuecomment-6425766640).

This selects a storage engine and document representation for bounded local synthetic implementation and verification. It selects no hosting service, paid plan, region, provisioning, live Google configuration, numerical recovery objective, deployment or launch. Legacy browser-data import is deferred by the owner's current direction because there are no existing users to migrate. Normal CV upload, reviewed Profile creation and explicit Apply/Save remain supported.

Account/provider-subject mappings, hashed opaque sessions, one-use OIDC transactions, the current account-owned Profile and small operation receipts share one database. A Profile is a validated version-2 domain document, with its stable IDs, provenance, opaque values and recorded approval/support semantics preserved. Relational columns index ownership and current revision; the career content remains an opaque byte document to PostgreSQL. SQL does not validate/query its internal facts.

Rationale: PostgreSQL provides a common transaction/locking boundary across instances. `bytea` avoids translating supported Profile values into `jsonb`'s narrower Unicode/number representation. PostgreSQL documents that [`jsonb` rejects `\u0000`](https://www.postgresql.org/docs/current/datatype-json.html), whereas [`bytea` stores arbitrary bytes](https://www.postgresql.org/docs/current/datatype-binary.html). This trades SQL JSON indexing/validation for explicit Go validation and byte-preserving storage. Engine capabilities alone are not concurrency, recovery, hosted enforcement or production evidence.

See the [new-account persistence contract and local runbook](../../new-account-persistence.md) for endpoints, authorization/commit ordering, size/encoding bounds, data minimization, client recovery and remaining gates.

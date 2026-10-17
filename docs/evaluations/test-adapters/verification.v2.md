# LAB-07 v2 verification

Frozen base `b7595505882995530af158f54b05e9e6938b039e`; reviewed/runtime code `4930e62d40101f3ac34ab0f9848e4d980f19f2ae`. This evidence-only addition preserves that code. Original v1 inputs, reports and verification remain immutable and retain their historical identities.

V2 changes only the fixed probe source in the approved source manifest: a finite 2 MiB flood still must stop at the 128 KiB supervisor bound; detached descendants use IPC readiness without unused temp writes; isolation verifies private HOME permissions and the same host-file sentinel boundary. Shared test setup removes duplication without removing public assertions. UI changes preserve byte/hash verification and use stable artifact keys.

`GOCACHE=/tmp/careeros-go-cache go test ./...` passed on the actual Linux host; evaluationlab 213.175 s. [Full output](go-test.v2.txt) is retained. Focused privacy, detached child/grandchild cancellation/deadline, output limit, missing tool and Node step-accounting checks passed, 2.536 s. `npx tsc --noEmit`, formatter and `git diff --check` passed. The actual one-invocation Lab browser smoke passed, including fixed Node execution, isolated Resume artifacts/download, history, exports/imports, cancellation, comparisons, negative capability states, keyboard/390px and zero Lab browser storage/external requests. These checks used no real provider calls or personal Profiles.

[The v2 observation manifest](observations.manifest.v2.json) binds eight exact reports and their typed artifacts to the frozen code/input identities. Every retained report/artifact SHA-256 and PNG/PDF signature verifies; all 208 v2 source hashes match the frozen checkout. Two browser overflow assertions remain failed evidence. Native print/save, per-print-request observations, deployed/provider quality/cost and future CV v3 acceptance remain unavailable.

Standards and Spec re-reviews are clear through the v2 code. Reviewers verified v1 immutability and v2 source hashes; their namespace-dependent tests remain unavailable in their environments, separate from the actual host runs above.

GitGuardian and SonarCloud both reported successful checks for code `4930e62d`; final PR-head checks and merge identity are recorded in PR #163. No scanner configuration or gate was weakened.

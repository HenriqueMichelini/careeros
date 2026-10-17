# LAB-07 verification

Frozen main base: `b7595505882995530af158f54b05e9e6938b039e`. Initial implementation: `9fc5cf27e78fc76c1a216a712674d4465fca57dd`. Reviewed code with fixes: `2a7e845e26d959f9d0903d73300fcedbb8d6f780`. The separate evidence commit adds only these retained observations/documentation. #151 has no milestone; target is main.

Fixture: `lab-process-fixtures-v1`, SHA-256 `4ef2e16d03f030ecbe2862f7bf24e4e622191c8af2649589f06297da44fe572f`. Results: `lab-process-result-v1` inside `lab-result-v1`. [Observation manifest](observations.manifest.v1.json) records exact original report hashes, code candidates, runtime identities, artifact kinds and limits. Its sixteen files retain both successful and failed synthetic observations. Historical candidate results are identified and never rewritten as newer-code execution.

## Commands and observations

Commands were invoked through RTK. `GOCACHE=/tmp/careeros-go-cache` was used for Go tooling. Namespace execution ran on the actual Linux host; default sandbox/reviewer namespace checks can skip and are not counted as end-to-end passes.

| Verification | Result |
| --- | --- |
| `go test ./...` on `9fc5cf27` | Passed; evaluationlab 215.878 s. Includes actual Go/Node and all registered representative browser entry points. |
| Focused public runner suite on reviewed `2a7e845e` (Go/Node entry points, section/Resume browsers, privacy, cancellation/deadline, flood, missing tool, crash, native print, assertion regression) | Passed, 101.932 s; no inference/HTTP attempt inflation. |
| `go test -race ./internal/evaluationlab -run 'TestProcessRunner(Cancellation\|Deadline\|Bounds\|Cannot\|MissingTool\|ExecutesExistingNode)\|TestAPI\|TestSequence\|TestRunner' -count=1` | Passed, 6.508 s on reviewed code. |
| `npm test` | 143 passed, zero skipped/failed. Injected field helpers remain offline; test names mentioning live collectors do not imply real provider execution. |
| `npx tsc --noEmit` | Passed. |
| Product `npm run build` and one-invocation Lab build/launch | Passed; existing Vite native-loader/chunk-size warnings retained. Generated product build output was restored and excluded. |
| `node tests/lab-browser.check.mjs` | Passed actual standalone Lab: Node driver, isolated Resume harness, typed artifact download, controlled suites, lazy detail, export/import, reload/reconnect, cancel, comparison, negative preflight, keyboard/390px, zero Lab browser-storage calls/external requests. |
| Formatter and `git diff --check` | Passed. |
| Retained manifest/report/artifact verification | Every file and artifact hash matches; PNG/PDF signatures verified; all approved input hashes match source files. |

Completed process observations passed Go canonical/ingestion/job-application/CV and Node ingestion/job-reuse/artifact-review/field-tooling. Section/Resume browser tests passed with actual screenshots and CDP PDFs. Ingestion and application reuse browser observations also passed. Canonical edit and Apply enrichment browser observations returned horizontal-overflow assertions; these remain `assertion_failed`, not a success or diagnosed cause. The adapter tests verify outcome/state consistency and preserve those failures. They do not certify product layout or external typography.

Negative probes preserve missing runtime (zero spawn), deliberate assertion failure (completed/fail), SIGKILL (crashed/blocked), absent native print (zero spawn/blocked), bounded output prefix, deadline, canceled detached child/grandchild and unreached next tests. Four injected privacy opportunities independently check credential environment absence, inaccessible host temporary sentinel, inaccessible host loopback canary and private HOME. These are finite proofs, not universal security or semantic guarantees.

## Standards

No documented-standard violations or actionable heuristic smells. Re-review clear through `2a7e845e`.

## Spec

Initial review found two case-level reporting defects: environment failures retained unavailable rather than blocked assertions; executed process cases retained one skipped step. Both were reproduced red through the public runner and fixed. Re-review found no remaining actionable spec findings. Reviewers independently passed argument/preflight tests; their namespace-dependent tests skipped in their environment. Actual host execution is recorded separately above.

GitGuardian incident 38306696 identifies `cases.v1.json:210`. Its value is exactly the recomputed SHA-256 of `scripts/field-validation/credentials.mjs`, not a credential. Service disposition/check state is tracked in PR #163 rather than inferred from this local verification.

Native saved PDFs, real provider semantics/cost, deployed behavior, personal Profiles, fresh blinded calibration and future CV v3 integrity remain unavailable/outside this evidence. No provider/deployment verification or paid calls were performed.

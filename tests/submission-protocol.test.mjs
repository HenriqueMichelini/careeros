import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync, readdirSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { fileURLToPath } from "node:url"
import ts from "typescript"

const compiled = ts.transpileModule(
  readFileSync(new URL("../src/lib/ingestionProtocol.ts", import.meta.url), "utf8"),
  { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022, verbatimModuleSyntax: false } },
).outputText
const output = join(tmpdir(), `careeros-protocol-${process.pid}.mjs`)
writeFileSync(output, compiled)

const proto = await import(output)

const fixtureDir = fileURLToPath(new URL("../backend/profile-ingestion/testdata/submission-protocol", import.meta.url))
const fixtureFiles = readdirSync(fixtureDir).filter((name) => name.endsWith(".json")).sort()

function result(code) {
  return { ok: code === "", code }
}

function fixtureResult(fixture) {
  switch (fixture.check) {
    case "stage":
      return result(proto.submissionStageError(fixture.stage))
    case "portion":
      return result(proto.portionBudgetError(fixture.bytes))
    case "status":
      return result(proto.submissionStatusError(fixture.status))
    case "counts":
      return result(
        proto.submissionCountsError({
          batches: fixture.batches,
          claims: fixture.claims,
          operations: fixture.operations,
        }),
      )
    case "binding":
      return proto.canonicalBinding(fixture.left) === proto.canonicalBinding(fixture.right)
        ? result("")
        : result("stale")
    case "manifest":
      return result(proto.submissionManifestError(fixture.manifest, fixture.original ?? ""))
    case "aggregate":
      return result(proto.submissionAggregateError(fixture.aggregate, fixture.manifest))
    case "focus":
      return result(proto.submissionFocusError(fixture.focus, fixture.aggregate))
    default:
      return { ok: false, code: "unknown_check" }
  }
}

assert.ok(fixtureFiles.length > 0, "submission protocol fixtures are present")

for (const file of fixtureFiles) {
  const fixture = JSON.parse(readFileSync(join(fixtureDir, file), "utf8"))
  test(`submission protocol fixture: ${fixture.name}`, () => {
    const actual = fixtureResult(fixture)
    assert.equal(actual.ok, fixture.expect.ok, `${fixture.name} ok`)
    assert.equal(actual.code, fixture.expect.error ?? "", `${fixture.name} code`)
  })
}

test("submission binding ignores object key order but not values", () => {
  assert.equal(
    proto.canonicalBinding({ a: 1, b: { c: 2, d: 3 }, e: [1, 2] }),
    proto.canonicalBinding({ e: [1, 2], b: { d: 3, c: 2 }, a: 1 }),
  )
  assert.notEqual(proto.canonicalBinding({ a: 1 }), proto.canonicalBinding({ a: 2 }))
})

test("a whole-submission aggregate legitimately exceeds the per-batch claim cap", () => {
  const plan = {
    protocolVersion: proto.submissionProtocolVersion,
    sourceId: "src",
    profileId: "prof",
    runId: "run",
    planVersion: "portions-v1",
    status: "ready",
    complete: true,
    originalBytes: 10,
    preparedBytes: 10,
    batches: [
      { index: 0, primary: [{ Start: 0, End: 5 }], context: [], preparedBytes: 5, payloadBytes: 40 },
      { index: 1, primary: [{ Start: 5, End: 10 }], context: [], preparedBytes: 5, payloadBytes: 40 },
    ],
    remaining: [],
  }
  const batch = (index, count, skipped) => ({
    index,
    primary: plan.batches[index].primary,
    claimIds: Array.from({ length: count }, (_, i) => proto.globalClaimId("run", index, `c${i}`)),
    claimCount: count,
    skippedCount: skipped,
  })
  const aggregate = {
    protocolVersion: proto.submissionProtocolVersion,
    sourceId: "src",
    profileId: "prof",
    runId: "run",
    planVersion: "portions-v1",
    batches: [batch(0, 20, 1), batch(1, 20, 0)],
    coverage: { validClaims: 40, invalidClaims: 1, discoveryComplete: false, capacity: "possibly_exhausted" },
  }
  assert.equal(proto.submissionManifestError(plan, "abcdefghij"), "")
  assert.equal(proto.submissionAggregateError(aggregate, plan), "")
  assert.equal(proto.submissionFocusError({ claimIds: aggregate.batches[0].claimIds.slice(0, 3) }, aggregate), "")
})

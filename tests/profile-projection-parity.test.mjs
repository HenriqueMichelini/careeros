import test, { before } from "node:test"
import assert from "node:assert/strict"
import { execFileSync } from "node:child_process"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"

const root = new URL("..", import.meta.url).pathname
const temp = mkdtempSync(join(tmpdir(), "careeros-projection-parity-"))
for (const [name, source] of [
  ["contract", "profiledocument/contract.json"],
  ["fields", "qualificationmatching/fields.json"],
]) {
  writeFileSync(
    join(temp, `${name}.mjs`),
    `export default ${readFileSync(join(root, "internal", source), "utf8")}`,
  )
}
for (const name of ["profileDocument", "profile", "qualificationEvidence"]) {
  let output = ts.transpileModule(
    readFileSync(join(root, "src/lib", `${name}.ts`), "utf8"),
    {
      compilerOptions: {
        module: ts.ModuleKind.ESNext,
        target: ts.ScriptTarget.ES2022,
      },
    },
  ).outputText
  output = output
    .replaceAll(
      '"../../internal/profiledocument/contract.json"',
      '"./contract.mjs"',
    )
    .replaceAll(
      '"../../internal/qualificationmatching/fields.json"',
      '"./fields.mjs"',
    )
  writeFileSync(join(temp, `${name}.mjs`), output)
}
const { profileView, validateProfileDocument } = await import(
  join(temp, "profileDocument.mjs")
)
const { careerProfile, cvQualifications } = await import(
  join(temp, "profile.mjs")
)
const { qualificationProjection, hasQualificationEvidence } = await import(
  join(temp, "qualificationEvidence.mjs")
)
const cases = JSON.parse(
  readFileSync(
    join(root, "internal/accountapi/testdata/profile-projections.json"),
    "utf8",
  ),
)
const output = join(temp, "go-results.json")
try {
  execFileSync(
    "go",
    [
      "test",
      "./internal/accountapi",
      "-run",
      "^TestProfileProjectionContract$",
      "-count=1",
    ],
    {
      cwd: root,
      env: {
        ...process.env,
        GOCACHE: process.env.GOCACHE || join(tmpdir(), "careeros-go-cache"),
        CAREEROS_PROFILE_PROJECTION_OUTPUT: output,
      },
      timeout: 120_000,
      stdio: ["ignore", "pipe", "pipe"],
    },
  )
} catch (error) {
  throw new Error(
    `Go Profile projection contract failed:\n${error.stdout?.toString() || error.message}`,
    { cause: error },
  )
}
const goResults = JSON.parse(readFileSync(output, "utf8"))
before(() => assert.equal(goResults.length, cases.length))
for (const [index, fixture] of cases.entries()) {
  test(`Go/TypeScript Profile projections agree: ${fixture.name}`, () => {
    const doc = structuredClone(fixture.document)
    assert.equal(
      validateProfileDocument(doc),
      true,
      "the shared input must be domain-valid in TypeScript",
    )
    const view = profileView(doc)
    const actual = {
      name: fixture.name,
      evidence: qualificationProjection(doc),
      useful: hasQualificationEvidence(doc),
      repository: careerProfile(view),
      qualifications: cvQualifications(view),
    }
    assert.deepEqual(
      actual,
      goResults[index],
      "compare full values, metadata and array order; JSON object-key order is immaterial",
    )
    assert.deepEqual(
      doc,
      fixture.document,
      "projection must not mutate the saved Profile",
    )
    assert.deepEqual(
      actual.evidence.facts.map((f) => f.id),
      fixture.expected.facts,
    )
    assert.deepEqual(
      actual.evidence.evidence.map((e) => e.id),
      fixture.expected.evidence,
    )
    assert.deepEqual(
      actual.evidence.links.map((l) => l.id),
      fixture.expected.links,
    )
    assert.equal(actual.useful, fixture.expected.useful)
    for (const [group, ids] of Object.entries(fixture.expected.groups)) {
      assert.deepEqual(
        (actual.repository[group] ?? actual.qualifications[group]).map(
          (item) => item.id,
        ),
        ids,
      )
    }
  })
}

import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync, writeFileSync, mkdtempSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"
const target = join(
  mkdtempSync(join(tmpdir(), "qualification-")),
  "evidence.mjs",
)
const source = readFileSync(
  new URL("../src/lib/qualificationEvidence.ts", import.meta.url),
  "utf8",
)
writeFileSync(
  target,
  ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText.replace(/import fields from [^\n]+\n/, 'const fields = '+readFileSync(new URL('../internal/qualificationmatching/fields.json', import.meta.url),'utf8')+';\n'),
)
const { qualificationProjection, hasQualificationEvidence } = await import(
  target
)
const fixture = JSON.parse(
  readFileSync(
    new URL("../internal/profiledocument/fixtures.json", import.meta.url),
  ),
)[0].document

test("qualification projection retains approved negative evidence and excludes private fields", () => {
  const doc = structuredClone(fixture)
  doc.facts[0].assertion = "negated"
  doc.facts.push({
    ...doc.facts[0],
    id: "private",
    field: "email",
    value: "private@example.com",
    support: "unsupported",
  })
  const projected = qualificationProjection(doc)
  assert.equal(projected.facts.length, 1)
  assert.equal(projected.facts[0].assertion, "negated")
  assert.equal(projected.entities[0].id, "role")
  assert.equal(projected.evidence.length, 1)
  assert.ok(!JSON.stringify(projected).includes("private@example.com"))
  assert.equal(hasQualificationEvidence(doc), true)
  doc.facts[0].approval = "unreviewed"
  assert.equal(hasQualificationEvidence(doc), false)
})

test("each sparse professional section is eligible while goals and contact alone are not", () => {
  for (const [section, field, value] of [
    ["projects", "name", "Atlas"],
    ["education", "degree", "BSc"],
    ["certifications", "name", "Cloud Certificate"],
    ["languages", "name", "Portuguese"],
    ["profile", "tools", "Docker"],
    ["profile", "competencies", "System design"],
    ["profile", "skills", "Java"],
    ["experience", "title", "Engineer"],
  ]) {
    const doc = structuredClone(fixture)
    const fact = doc.facts[0]
    doc.links = []
    doc.evidence = []
    doc.facts = [fact]
    fact.support = "unsupported"
    fact.field = field
    fact.value = value
    fact.context = []
    if (section !== "profile") {
      doc.entities[0].kind = section
      fact.owner = { profileId: "p", id: "role", revision: 1 }
    }
    assert.equal(hasQualificationEvidence(doc), true, section + ":" + field)
  }
  const doc = structuredClone(fixture)
  doc.facts[0].field = "careerGoals"
  assert.equal(hasQualificationEvidence(doc), false)
})

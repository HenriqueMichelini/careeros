import test from "node:test"
import assert from "node:assert/strict"
import { spawnSync } from "node:child_process"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { fileURLToPath } from "node:url"
import ts from "typescript"

const root = fileURLToPath(new URL("..", import.meta.url))
const temp = mkdtempSync(join(tmpdir(), "section-evaluation-client-"))
writeFileSync(join(temp, "contract.mjs"), `export default ${readFileSync(join(root, "internal/profiledocument/contract.json"), "utf8")}`)
for (const name of ["profileDocument", "sectionReview"]) {
  const source = readFileSync(join(root, `src/lib/${name}.ts`), "utf8")
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
    .replaceAll('"./profileDocument"', '"./profileDocument.mjs"')
    .replaceAll('"../../internal/profiledocument/contract.json"', '"./contract.mjs"')
  writeFileSync(join(temp, `${name}.mjs`), compiled)
}
const { editProfile } = await import(join(temp, "profileDocument.mjs"))
const { sectionReviewRequest, validateSectionProposal, applySectionProposal } = await import(join(temp, "sectionReview.mjs"))
const output = join(temp, "controlled.json")
const executed = spawnSync("go", ["run", "./scripts/semantic-quality", "--mode", "controlled", "--split", "all", "--fixtures", "docs/evaluations/section-review/cases.v1.json", "--output", output], { cwd: root, env: { ...process.env, GOCACHE: "/tmp/careeros-go-cache" }, encoding: "utf8" })
assert.equal(executed.status, 0, `controlled public CLI failed: ${executed.stderr}`)
const report = JSON.parse(readFileSync(output, "utf8"))
const corpus = JSON.parse(readFileSync(join(root, "docs/evaluations/section-review/cases.v1.json"), "utf8"))
const runs = new Map(report.runs.map((run) => [run.id, run]))

for (const language of ["en", "pt-BR"]) {
  test(`section adapter and client share exact scope without automatic Apply: ${language}`, () => {
    const cases = corpus.cases.filter((item) => item.language === language)
    assert.equal(cases.length, 22)
    for (const item of cases) {
      const original = JSON.stringify(item.request.document)
      const request = sectionReviewRequest(item.request.document, item.request.section, language)
      const run = runs.get(item.id)
      assert.equal(run.status, item.expectedStatus)
      for (const [index, envelope] of run.controlledRequestEnvelopes.entries()) {
        const message = envelope.messages.find((part) => part.role === "user")
        const payload = JSON.parse(message.content)
        assert.deepEqual(index === 0 ? payload : payload.source, request)
        assert.equal(JSON.stringify(envelope).includes("never-send-section@example.test"), false)
      }
      if (run.status === 200) assert.equal(validateSectionProposal(request, run.output), true)
      assert.equal(JSON.stringify(item.request.document), original)
    }
  })
  test(`explicit scripted Apply preserves unrelated facts and stale source blocks: ${language}`, () => {
    const benign = corpus.cases.filter((item) => item.language === language && ["grounded", "translation", "multi-assertion", "protected-owner-context"].some((family) => item.id.endsWith(`-${family}`)))
    assert.equal(benign.length, 4)
    for (const item of benign) {
      const doc = item.request.document
      const before = structuredClone(doc)
      const request = sectionReviewRequest(doc, item.request.section, language)
      const proposal = runs.get(item.id).output
      const applied = applySectionProposal(doc, request, proposal, {}, [])
      for (const patch of proposal.patches) assert.equal(applied.facts.find((fact) => fact.id === patch.factId).value, patch.wording)
      const privateBefore = doc.facts.find((fact) => fact.id === "private")
      assert.deepEqual(applied.facts.find((fact) => fact.id === "private"), { ...privateBefore, owner: { ...privateBefore.owner, revision: applied.revision } })
      const changed = editProfile(doc, { type: "fact", id: proposal.patches[0].factId, patch: { certainty: "uncertain" } })
      assert.throws(() => applySectionProposal(changed, request, proposal, {}, []), /stale/)
      assert.deepEqual(doc, before)
    }
  })
}

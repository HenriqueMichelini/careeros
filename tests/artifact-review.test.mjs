import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync, writeFileSync, mkdtempSync } from "node:fs"
import { join } from "node:path"
import { tmpdir } from "node:os"
import ts from "typescript"
const dir = mkdtempSync(join(tmpdir(), "artifact-review-"))
for (const name of ["artifactReview", "cover-letter"]) {
  const source = readFileSync(
    new URL(`../src/lib/${name}.ts`, import.meta.url),
    "utf8",
  )
  writeFileSync(
    join(dir, name + ".mjs"),
    ts
      .transpileModule(source, {
        compilerOptions: {
          module: ts.ModuleKind.ESNext,
          target: ts.ScriptTarget.ES2022,
        },
      })
      .outputText.replace('"./cover-letter"', '"./cover-letter.mjs"'),
  )
}
const {
  acceptArtifacts,
  correctArtifact,
  removeArtifactClaim,
  parseArtifactReview,
} = await import(join(dir, "artifactReview.mjs"))
const doc = { id: "p", revision: 1, facts: [], evidence: [], links: [] }
const review = {
  id: "r",
  version: "artifact-review-v1",
  check: "complete",
  sourceSnapshot: doc,
  identityProfile: { fullName: "  José Silva  " },
  cvLanguage: "pt-BR",
  jobPosting: "Java developer.",
  closing: "Atenciosamente,",
  facts: [],
  claims: [
    {
      field: "jobSummary",
      text: "Java developer.",
      sources: [],
      jobSources: [{ start: 0, end: 15, quote: "Java developer." }],
      state: "supported",
      concerns: [],
      nonfactual: false,
    },
    {
      field: "greeting",
      text: "Prezada equipe,",
      sources: [],
      jobSources: [],
      state: "supported",
      concerns: [],
      nonfactual: true,
    },
    {
      field: "body",
      text: "Eu uso Java.",
      sources: [],
      jobSources: [],
      state: "uncertain",
      concerns: ["missing_citations"],
      nonfactual: false,
    },
    {
      field: "applicationAnswers",
      text: "Eu uso Java.",
      sources: [],
      jobSources: [],
      state: "supported",
      concerns: [],
      nonfactual: false,
    },
  ],
}
test("one unresolved artifact blocks whole-draft acceptance; corrections own evidence and signature", () => {
  assert.throws(() => acceptArtifacts(review, doc), /unresolved/)
  const corrected = correctArtifact(
    review,
    2,
    "Eu uso Java em projetos pessoais.",
  )
  assert.deepEqual(corrected.claims[2].sources, [])
  assert.deepEqual(corrected.claims[2].jobSources, [])
  const accepted = acceptArtifacts(corrected, doc)
  assert.equal(
    accepted.coverLetter,
    "Prezada equipe,\n\nEu uso Java em projetos pessoais.\n\nAtenciosamente,\nJosé Silva",
  )
  assert.equal(review.claims[2].state, "uncertain")
  corrected.claims[2].text = "Mutated"
  assert.match(accepted.coverLetter, /projetos pessoais/)
  assert.throws(
    () => acceptArtifacts(corrected, { ...doc, revision: 2 }),
    /stale/,
  )
})
test("missing required answer cannot be removed to bypass its concern", () => {
  const unresolved = structuredClone(review)
  unresolved.claims[3].concerns = ["Required answer omits Java."]
  unresolved.claims[3].state = "unsupported"
  assert.throws(() => removeArtifactClaim(unresolved, 3), /required/)
})
test("parser rejects incomplete coverage and forged job citations", () => {
  const raw = structuredClone(review)
  raw.claims[3].state = "uncertain"
  const output = {
    jobTitle: null,
    company: null,
    jobSummary: "Java developer.",
    coverLetter: {
      greeting: "Prezada equipe,",
      body: "Eu uso Java.",
      closing: "Atenciosamente,",
    },
    applicationAnswers: "Eu uso Java.",
  }
  assert.ok(
    parseArtifactReview(
      raw,
      doc,
      review.identityProfile,
      "pt-BR",
      review.jobPosting,
      output,
    ),
  )
  assert.equal(
    parseArtifactReview(
      { ...raw, claims: raw.claims.slice(1) },
      doc,
      review.identityProfile,
      "pt-BR",
      review.jobPosting,
      output,
    ),
    null,
  )
  const forged = structuredClone(raw)
  forged.claims[0].jobSources[0].quote = "Invented employer"
  assert.equal(
    parseArtifactReview(
      forged,
      doc,
      review.identityProfile,
      "pt-BR",
      review.jobPosting,
      output,
    ),
    null,
  )
})

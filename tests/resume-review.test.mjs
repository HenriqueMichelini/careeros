import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync, writeFileSync, mkdtempSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"
const path = join(mkdtempSync(join(tmpdir(), "resume-review-")), "review.mjs")
writeFileSync(
  path,
  ts.transpileModule(
    readFileSync(
      new URL("../src/lib/resumeReview.ts", import.meta.url),
      "utf8",
    ),
    {
      compilerOptions: {
        module: ts.ModuleKind.ESNext,
        target: ts.ScriptTarget.ES2022,
      },
    },
  ).outputText,
)
const {
  acceptResume,
  correctResume,
  removeResumeClaim,
  resumeReviewCurrent,
  renderReviewedResume,
  parseResumeReview,
} = await import(path)
const sourceSnapshot = JSON.parse(
  readFileSync(
    new URL("../internal/profiledocument/fixtures.json", import.meta.url),
  ),
)[0].document
const claim = {
  section: "skills",
  kind: "bullet",
  text: "Java",
  sources: [{ profileId: "p", id: "skill", revision: 1 }],
  state: "uncertain",
  concerns: ["check_unavailable"],
}
const review = {
  version: "resume-review-v1",
  claims: [claim],
  facts: [],
  check: "unavailable",
  sourceSnapshot,
  identityProfile: { fullName: "Avery" },
  cvLanguage: "en",
}

test("uncertain resume cannot be accepted or exported", () => {
  assert.throws(() => acceptResume(review, sourceSnapshot), /unresolved/)
  assert.equal(resumeReviewCurrent(review, sourceSnapshot), true)
})

test("explicit correction owns new support and never inherits citations", () => {
  const corrected = correctResume(
    review,
    0,
    "I use Java for personal projects.",
  )
  assert.deepEqual(corrected.claims[0].sources, [])
  assert.equal(
    corrected.claims[0].userSupport.evidence,
    "I use Java for personal projects.",
  )
  assert.deepEqual(
    corrected.claims[0].userSupport.previousSources,
    claim.sources,
  )
  const accepted = acceptResume(corrected, sourceSnapshot)
  assert.equal(
    accepted.markdown,
    "## Technical Skills\n- I use Java for personal projects.",
  )
  assert.equal(review.claims[0].state, "uncertain")
  corrected.claims[0].text = "Later edit"
  assert.equal(accepted.claims[0].text, "I use Java for personal projects.")
})

test("source revisions and evidence edits block acceptance while preserving a valid snapshot", () => {
  const supported = {
    ...review,
    claims: [{ ...claim, state: "supported", concerns: [] }],
  }
  const accepted = acceptResume(supported, sourceSnapshot)
  const stale = structuredClone(sourceSnapshot)
  stale.facts[0].revision++
  assert.equal(resumeReviewCurrent(supported, stale), false)
  assert.throws(() => acceptResume(supported, stale), /stale/)
  const changed = structuredClone(sourceSnapshot)
  changed.evidence[0].excerpt = "Changed assertion"
  assert.equal(resumeReviewCurrent(supported, changed), false)
  assert.equal(accepted.markdown, "## Technical Skills\n- Java")
})

test("remove unsupported assertions before accepting remaining content", () => {
  const mixed = {
    ...review,
    claims: [
      { ...claim, state: "supported", concerns: [] },
      { ...claim, text: "Led a team of 99", state: "unsupported" },
    ],
  }
  assert.throws(() => acceptResume(mixed, sourceSnapshot), /unresolved/)
  const removed = removeResumeClaim(mixed, 1)
  assert.equal(
    acceptResume(removed, sourceSnapshot).markdown,
    "## Technical Skills\n- Java",
  )
  assert.equal(mixed.claims[1].removed, undefined)
  assert.throws(
    () => acceptResume(removeResumeClaim(removed, 0), sourceSnapshot),
    /unresolved/,
  )
  assert.equal(
    renderReviewedResume(removed.claims, "pt-BR"),
    "## Competências Técnicas\n- Java",
  )
})

test("malformed unresolved references fail at the review boundary instead of crashing the view", () => {
  const invalid = {
    version: "resume-review-v1",
    check: "not_needed",
    claims: [{ ...claim, state: "unsupported", sources: [null] }],
    facts: [],
  }
  assert.equal(
    parseResumeReview(
      invalid,
      sourceSnapshot,
      {},
      "en",
      "## Technical Skills\n- Java",
    ),
    null,
  )
})

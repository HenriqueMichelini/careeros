// New synthetic cross-feature cases. No provider calls or semantic-quality claims.
import test from "node:test"
import assert from "node:assert/strict"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"
const dir = mkdtempSync(join(tmpdir(), "evidence-mvp-"))
const names = [
  "profileDocument",
  "profileStorage",
  "ingestionDocument",
  "ingestion",
  "profile",
  "fieldDecision",
  "cvGeneration",
  "qualificationEvidence",
  "resumeReview",
  "artifactReview",
  "cover-letter",
]
for (const name of names) {
  let output = ts.transpileModule(
    readFileSync(new URL(`../src/lib/${name}.ts`, import.meta.url), "utf8"),
    {
      compilerOptions: {
        module: ts.ModuleKind.ESNext,
        target: ts.ScriptTarget.ES2022,
      },
    },
  ).outputText
  for (const dependency of names)
    output = output.replaceAll(`"./${dependency}"`, `"./${dependency}.mjs"`)
  for (const [path, file] of [
    ["profiledocument/contract", "contract"],
    ["qualificationmatching/fields", "fields"],
  ]) {
    writeFileSync(
      join(dir, `${file}.mjs`),
      `export default ${readFileSync(new URL(`../internal/${path}.json`, import.meta.url), "utf8")}`,
    )
    output = output.replaceAll(
      `"../../internal/${path}.json"`,
      `"./${file}.mjs"`,
    )
  }
  writeFileSync(join(dir, `${name}.mjs`), output)
}
const load = (name) => import(join(dir, `${name}.mjs`))
const { profileView, editProfile } = await load("profileDocument")
const { openProfile, saveProfile, PROFILE_KEY, LEGACY_KEY } =
  await load("profileStorage")
const { applyIngestionDocument } = await load("ingestionDocument")
const { stableCvFacts, createCuratedCv, parseCuratedCv, cvSupportStatus } =
  await load("cvGeneration")
const { qualificationProjection } = await load("qualificationEvidence")
const { acceptResume } = await load("resumeReview")
const { acceptArtifacts } = await load("artifactReview")
const lock = async (fn) => fn()
function memory(values) {
  const data = new Map(Object.entries(values))
  return {
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => data.set(key, value),
  }
}
for (const [
  language,
  source,
  negative,
  aspiration,
  period,
  greeting,
  closing,
] of [
  [
    "en",
    "I use Rust.",
    "I do not use Java.",
    "Hope to learn Go.",
    "around 2021",
    "Dear team,",
    "Sincerely,",
  ],
  [
    "pt-BR",
    "Eu uso Rust.",
    "Não uso Java.",
    "Quero aprender Go.",
    "por volta de 2021",
    "Prezada equipe,",
    "Atenciosamente,",
  ],
]) {
  test(`held-out ${language}: migration → edit → approved excerpt → CV and all draft reviews → source revision`, async () => {
    const legacy = {
      fullName: "Alex Rivera",
      skills: "",
      careerGoals: aspiration,
      experience: [
        {
          id: "first",
          company: "North Lab",
          title: "Engineer",
          startDate: period,
          endDate: "2023",
          current: false,
          description: negative,
        },
        {
          id: "second",
          company: "South Lab",
          title: "Engineer",
          startDate: "2024",
          endDate: "",
          current: true,
          description: "Rust",
        },
      ],
      projects: [
        {
          id: "project",
          name: "Orbit",
          description: aspiration,
          technologies: "Go",
        },
      ],
    }
    legacy.experience = legacy.experience.map((e) => ({
      location: "",
      responsibilities: "",
      achievements: "",
      ...e,
    }))
    const original = JSON.stringify(legacy)
    const storage = memory({ [LEGACY_KEY]: original })
    let doc = await openProfile(storage, lock)
    assert.deepEqual(profileView(doc).experience, legacy.experience)
    assert.equal(profileView(doc).careerGoals, aspiration)
    const edited = editProfile(doc, {
      type: "field",
      ownerId: doc.id,
      field: "tools",
      value: "Git",
    })
    await saveProfile(storage, doc, edited, lock)
    doc = await openProfile(storage, lock)
    const input = source + " PRIVATE_UNSELECTED_REMAINDER"
    const proposal = {
      claims: [
        {
          id: "heldout",
          source,
          text: source,
          targets: ["skills"],
          question: "",
          meaning: {
            assertion: "affirmed",
            intent: "actual",
            certainty: "certain",
            temporal: { wording: "", precision: "unknown" },
          },
        },
      ],
      operations: [
        {
          claimId: "heldout",
          target: "skills",
          entryId: "",
          field: "skills",
          action: "add",
          value: "Rust",
          finding: "addition",
          approved: true,
        },
      ],
      outcomes: [
        {
          claimId: "heldout",
          kind: "change",
          reason: "Explicit supported addition",
          relatedFacts: [],
          relatedClaimIds: [],
          operationIndexes: [0],
        },
      ],
      coverage: {
        validClaims: 1,
        invalidClaims: 0,
        discoveryComplete: false,
        capacity: "within_limit",
      },
      skippedClaims: [],
      unverifiedClaimCount: 0,
      unresolvedClaimIds: [],
      unplacedOperationCount: 0,
    }
    const before = storage.getItem(PROFILE_KEY)
    const rejected = structuredClone(proposal)
    rejected.operations[0].approved = false
    assert.deepEqual(
      applyIngestionDocument(doc, doc, input, rejected).evidence,
      doc.evidence,
    )
    const candidate = applyIngestionDocument(doc, doc, input, proposal)
    assert.equal(storage.getItem(PROFILE_KEY), before, "review is transient")
    const failedStorage = {
      getItem: storage.getItem,
      setItem: () => {
        throw Error("quota")
      },
    }
    await assert.rejects(saveProfile(failedStorage, doc, candidate, lock), {
      code: "storage",
    })
    assert.equal(storage.getItem(PROFILE_KEY), before)
    await saveProfile(storage, doc, candidate, lock)
    await assert.rejects(saveProfile(storage, doc, candidate, lock), {
      code: "stale",
    })
    doc = await openProfile(storage, lock)
    assert.equal(storage.getItem(LEGACY_KEY), original)
    assert.deepEqual(
      doc.evidence.map((e) => e.excerpt),
      [source],
    )
    assert.ok(!JSON.stringify(doc).includes("PRIVATE_UNSELECTED_REMAINDER"))
    assert.deepEqual(profileView(doc).experience, legacy.experience)
    const projected = qualificationProjection(doc)
    assert.ok(!projected.facts.some((f) => f.field === "careerGoals"))
    const facts = stableCvFacts(doc)
    const skill = facts.find((f) => f.field === "skills" && f.text === "Rust")
    assert.ok(skill)
    const result = {
      selected: [skill.id],
      summary: [{ sourceId: skill.id, text: "Rust" }],
    }
    const cv = createCuratedCv(
      profileView(doc),
      facts,
      result,
      language,
      "balanced",
      doc,
    )
    assert.equal(cvSupportStatus(cv, doc), "current")
    assert.deepEqual(parseCuratedCv(JSON.stringify(cv)), cv)
    const snapshot = JSON.stringify(doc)
    const base = {
      id: "heldout-review",
      sourceSnapshot: doc,
      identityProfile: profileView(doc),
      cvLanguage: language,
      facts: [],
      check: "complete",
    }
    const resume = {
      ...base,
      version: "resume-review-v1",
      claims: [
        {
          section: "skills",
          kind: "bullet",
          text: "Rust",
          sources: [skill.reference],
          state: "supported",
          concerns: [],
        },
      ],
    }
    const jobPosting = "Rust"
    const careerClaim = (field) => ({
      field,
      text: source,
      sources: [skill.reference],
      jobSources: [],
      state: "supported",
      concerns: [],
      nonfactual: false,
      scope: "career",
    })
    const artifacts = {
      ...base,
      version: "artifact-review-v1",
      jobPosting,
      closing,
      claims: [
        {
          field: "jobSummary",
          text: jobPosting,
          sources: [],
          jobSources: [{ start: 0, end: 4, quote: jobPosting }],
          state: "supported",
          concerns: [],
          nonfactual: false,
          scope: "job",
        },
        {
          field: "greeting",
          text: greeting,
          sources: [],
          jobSources: [],
          state: "supported",
          concerns: [],
          nonfactual: true,
          scope: "nonfactual",
        },
        careerClaim("body"),
        careerClaim("applicationAnswers"),
      ],
    }
    assert.ok(acceptResume(resume, doc).markdown.includes("Rust"))
    const accepted = acceptArtifacts(artifacts, doc)
    assert.equal(accepted.jobTitle, null)
    assert.equal(accepted.company, null)
    assert.equal(accepted.applicationAnswers, source)
    assert.ok(accepted.coverLetter.endsWith("Alex Rivera"))
    for (const field of ["jobSummary", "body", "applicationAnswers"]) {
      const uncertain = structuredClone(artifacts)
      uncertain.claims.find((c) => c.field === field).state = "uncertain"
      assert.throws(() => acceptArtifacts(uncertain, doc), /unresolved/)
    }
    assert.equal(
      JSON.stringify(doc),
      snapshot,
      "temporary application answers and reviews never write Profile",
    )
    const revised = editProfile(doc, {
      type: "field",
      ownerId: doc.id,
      field: "skills",
      value: "Rust and C",
    })
    assert.equal(cvSupportStatus(cv, revised), "stale")
    assert.throws(() => acceptResume(resume, revised), /stale/)
    assert.throws(() => acceptArtifacts(artifacts, revised), /stale/)
    assert.throws(() => applyIngestionDocument(revised, doc, input, proposal), {
      code: "stale",
    })
    assert.deepEqual(
      revised.evidence,
      doc.evidence,
      "accepted excerpt survives correction for traceability",
    )
    assert.ok(
      revised.links
        .filter((l) => l.kind === "supports")
        .every((l) => l.state === "invalidated"),
    )
    assert.equal(
      JSON.stringify(cv),
      JSON.stringify(parseCuratedCv(JSON.stringify(cv))),
      "saved CV is independent of live edits",
    )
  })
}

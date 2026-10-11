// New synthetic cross-feature cases. No provider calls or semantic-quality claims.
import test from "node:test"
import { execFileSync } from "node:child_process"
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
  "ai",
  "jobContext",
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
const {
  stableCvFacts,
  createCuratedCv,
  parseCuratedCv,
  cvSupportStatus,
  generateCv,
} = await load("cvGeneration")
const { qualificationProjection } = await load("qualificationEvidence")
const { acceptResume } = await load("resumeReview")
const { acceptArtifacts } = await load("artifactReview")
const corpus = JSON.parse(
  readFileSync(
    new URL(
      "../docs/evaluations/workflow-settings/cases.v1.json",
      import.meta.url,
    ),
    "utf8",
  ),
)
const executable = join(dir, "semantic-quality")
execFileSync("go", ["build", "-o", executable, "./scripts/semantic-quality"], {
  env: { ...process.env, GOCACHE: "/tmp/careeros-go-cache" },
})
let runIndex = 0
function handler(templateId, request, provider, expectedStatus = 200) {
  const sample = structuredClone(corpus.cases.find((c) => c.id === templateId))
  delete sample.steps
  delete sample.sequenceOutput
  sample.id = `integration-${runIndex++}`
  sample.split = "heldout"
  sample.request = request
  sample.provider = provider
  const fixture = join(dir, `${sample.id}.json`),
    report = join(dir, `${sample.id}-result.json`)
  writeFileSync(
    fixture,
    JSON.stringify({ version: "mvp-integration-v1", cases: [sample] }),
  )
  execFileSync(
    executable,
    [
      "--mode",
      "controlled",
      "--fixtures",
      fixture,
      "--ids",
      sample.id,
      "--output",
      report,
    ],
    { stdio: "pipe" },
  )
  const result = JSON.parse(readFileSync(report, "utf8")).runs[0]
  assert.equal(result.status, expectedStatus, JSON.stringify(result.output))
  if (expectedStatus === 400) assert.equal((result.calls ?? []).length, 0)
  assert.equal(result.noStore, true)
  return result.output
}
const { findProfileGaps, generateMaterials } = await load("ai")
async function viaHandler(templateId, provider, operation) {
  const previous = globalThis.fetch
  globalThis.fetch = async (url, options) =>
    new Response(
      JSON.stringify(handler(templateId, JSON.parse(options.body), provider)),
      { status: 200, headers: { "Content-Type": "application/json" } },
    )
  try {
    return await operation()
  } finally {
    globalThis.fetch = previous
  }
}
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
    const rawProposal = handler(
      "en-fact",
      { input, profile: profileView(doc), document: doc },
      [
        {
          claims: [
            {
              id: "heldout",
              source,
              text: "Rust",
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
        },
        {
          operations: [
            {
              claimId: "heldout",
              target: "skills",
              entryId: "",
              field: "skills",
              action: "add",
              value: "Rust",
              finding: "addition",
            },
          ],
        },
      ],
    )
    const { decision, ...proposal } = rawProposal
    proposal.operations = proposal.operations.map((op) => ({
      ...op,
      approved: true,
    }))
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
    // Continue the frozen large-input case against the same durable document.
    // Every leg is a real production handler call with controlled transports.
    const prefix = language === "en" ? "en" : "pt"
    const sequence = corpus.cases.find(
      (c) => c.id === `${prefix}-continuation-35`,
    )
    for (const step of sequence.steps) {
      const providers = structuredClone(step.provider)
      for (const claim of providers[0].claims) claim.targets = ["tools"]
      for (const op of providers[1].operations) {
        op.target = "tools"
        op.field = "tools"
      }
      const output = handler(
        `${prefix}-fact`,
        { ...step.request, profile: profileView(doc), document: doc },
        providers,
      )
      assert.equal(output.continuation.processed, true)
      const { decision, ...review } = output
      review.operations = review.operations.map((op) => ({
        ...op,
        approved: true,
      }))
      const next = applyIngestionDocument(doc, doc, step.request.input, review)
      await saveProfile(storage, doc, next, lock)
      doc = await openProfile(storage, lock)
    }
    for (let i = 0; i < 35; i++)
      assert.ok(
        profileView(doc).tools.includes(`Tool${String(i).padStart(2, "0")}`),
      )
    const clarify = structuredClone(
      corpus.cases.find((c) => c.id === `${prefix}-clarification`),
    )
    for (const step of clarify.steps) {
      for (const provider of step.provider) {
        for (const claim of provider.claims ?? [])
          if (claim.targets.length) claim.targets = ["competencies"]
        for (const op of provider.operations ?? []) {
          op.target = "competencies"
          op.field = "competencies"
        }
      }
    }
    // Use a distinct skill so clarification does not duplicate the earlier Rust.
    const initial = handler(
      `${prefix}-fact`,
      { ...clarify.steps[0].request, profile: profileView(doc), document: doc },
      clarify.steps[0].provider,
    )
    assert.ok(initial.claims[0].question)
    const answerStep = clarify.steps[1]
    const answer = handler(
      `${prefix}-fact`,
      {
        ...answerStep.request,
        profile: profileView(doc),
        document: doc,
        clarification: {
          ...answerStep.request.clarification,
          claim: initial.claims[0],
        },
      },
      answerStep.provider,
    )
    const { decision: answerDecision, ...answerReview } = answer
    answerReview.operations = answerReview.operations.map((op) => ({
      ...op,
      approved: true,
    }))
    const clarified = applyIngestionDocument(
      doc,
      doc,
      answerStep.request.input,
      answerReview,
    )
    await saveProfile(storage, doc, clarified, lock)
    doc = await openProfile(storage, lock)
    assert.ok(
      doc.evidence.some(
        (e) =>
          e.origin === "clarification_answer" &&
          e.excerpt === answerStep.request.clarification.answer,
      ),
    )
    const projected = qualificationProjection(doc)
    assert.ok(!projected.facts.some((f) => f.field === "careerGoals"))
    const facts = stableCvFacts(doc)
    const skill = facts.find((f) => f.field === "skills" && f.text === "Rust")
    assert.ok(skill)
    const result = await viaHandler(
      "en-stable-cv-metric",
      [
        {
          selected: [skill.id],
          summary: [{ sourceId: skill.id, text: "Rust" }],
        },
      ],
      () =>
        generateCv(
          facts,
          language,
          "sk-controlled",
          new AbortController().signal,
        ),
    )
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
    const qualification = corpus.cases.find(
      (c) => c.id === `${prefix}-evidence-matching`,
    )
    const requirementProvider = structuredClone(qualification.provider)
    requirementProvider[1].matches[0] = {
      requirementIndex: 0,
      state: "not_evidenced",
      factIds: [],
      explanation:
        "Conflicting role and competency wording needs conservative review.",
      question: "",
    }
    const analysis = await viaHandler(
      `${prefix}-evidence-matching`,
      requirementProvider,
      () =>
        findProfileGaps(
          profileView(doc),
          qualification.request.jobPosting,
          "sk-controlled",
          "controlled",
          true,
          doc,
        ),
    )
    assert.equal(analysis.matches[0].state, "not_evidenced")
    assert.equal(analysis.jobContext.job.company, null)
    const draft = corpus.cases.find((c) => c.id === `${prefix}-reviewed-draft`)
    const providers = structuredClone(draft.provider)
    const supported = source
    providers[0].resume[0].text = supported
    providers[0].resume[0].sources = [skill.reference]
    providers[0].coverLetter.body = supported
    providers[0].applicationAnswers = supported
    for (const judgment of providers[2].judgments)
      if (judgment.scope === "career") judgment.sources = [skill.reference]
    const generated = await viaHandler(
      `${prefix}-reviewed-draft`,
      providers,
      () =>
        generateMaterials(
          profileView(doc),
          draft.request.jobPosting,
          "sk-controlled",
          [],
          language,
          "controlled",
          undefined,
          [],
          doc,
        ),
    )
    const resume = generated.resumeReview,
      artifacts = generated.artifactReview
    assert.ok(acceptResume(resume, doc).markdown.includes("Rust"))
    const accepted = acceptArtifacts(artifacts, doc)
    assert.equal(accepted.company, null)
    assert.equal(accepted.applicationAnswers, source)
    assert.ok(accepted.coverLetter.endsWith("Alex Rivera"))
    for (const field of ["jobSummary", "body", "applicationAnswers"]) {
      const uncertain = structuredClone(artifacts)
      uncertain.claims.find((c) => c.field === field).state = "uncertain"
      assert.throws(() => acceptArtifacts(uncertain, doc), /unresolved/)
    }
    // Retain original adversarial text and exercise current HTTP construction,
    // independent support assessment, client parsing and explicit acceptance.
    const legacyCases = JSON.parse(
      readFileSync(
        new URL(
          "../docs/evaluations/semantic-quality/cases.v2.json",
          import.meta.url,
        ),
        "utf8",
      ),
    ).cases
    for (const kind of [
      "invented-management",
      "requirement-as-skill",
      "transferred-metric",
    ]) {
      const legacy = legacyCases.find((c) => c.id === `${prefix}-draft-${kind}`)
      const bad = structuredClone(providers)
      bad[0].coverLetter.body = legacy.provider[0].coverLetter.body
      const judgment = bad[2].judgments.find((j) => j.index === 3)
      judgment.state = "unsupported"
      judgment.reason =
        "Retained baseline candidate has unsupported ownership or factual additions."
      judgment.sources = []
      const reviewed = await viaHandler(`${prefix}-reviewed-draft`, bad, () =>
        generateMaterials(
          profileView(doc),
          draft.request.jobPosting,
          "sk-controlled",
          [],
          language,
          "controlled",
          undefined,
          [],
          doc,
        ),
      )
      assert.throws(
        () => acceptArtifacts(reviewed.artifactReview, doc),
        /unresolved/,
      )
      assert.equal(JSON.stringify(doc), snapshot)
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
        .filter((l) => l.kind === "supports" && l.from.id === skill.id)
        .every((l) => l.state === "invalidated"),
    )
    assert.equal(
      JSON.stringify(cv),
      JSON.stringify(parseCuratedCv(JSON.stringify(cv))),
      "saved CV is independent of live edits",
    )
  })
}

test("fresh empty canonical Profile accepts the first bounded ingestion request", async () => {
  const doc = await openProfile(memory({}), lock)
  const sample = corpus.cases.find((c) => c.id === "en-fact")
  const output = handler(
    "en-fact",
    {
      ...sample.request,
      profile: profileView(doc),
      document: doc,
      portion: { index: 0, bytes: 2000 },
    },
    sample.provider,
  )
  assert.equal(output.claims.length, 1)
  for (const field of [
    "fullName",
    "email",
    "phone",
    "location",
    "professionalLinks",
  ]) {
    const profile = { ...profileView(doc), [field]: "different" }
    const rejected = handler(
      "en-fact",
      {
        ...sample.request,
        profile,
        document: doc,
        portion: { index: 0, bytes: 2000 },
      },
      sample.provider,
      400,
    )
    assert.equal(rejected.error, "input")
  }
})

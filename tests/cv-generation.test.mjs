import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync, writeFileSync, mkdtempSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"
const path = join(
  mkdtempSync(join(tmpdir(), "cv-generation-")),
  "generation.mjs",
)
writeFileSync(
  path,
  ts.transpileModule(
    readFileSync(
      new URL("../src/lib/cvGeneration.ts", import.meta.url),
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
  cvFacts,
  stableCvFacts,
  cvSupportStatus,
  cvWordingTrust,
  createCuratedCv,
  parseCuratedCv,
  validateCvResult,
  validateCvSupport,
  cvSupportUnits,
  cvReviewUnits,
  cvReviewResolved,
  cvReviewEdited,
  cvReviewConfirmed,
  acceptCvResult,
  resolveCvReview,
  validateCvReviewRecords,
  userAuthoredCvText,
  emptyCvReview,
} = await import(path)
const profile = {
  fullName: "Avery",
  email: "private@example.com",
  phone: "secret",
  location: "Boston",
  professionalLinks: "https://example.com",
  careerGoals: "Become CEO",
  employmentStatus: "unemployed",
  currentSalary: "100",
  desiredSalary: "200",
  additionalInfo: "private health",
  skills: "Research, Research, Design",
  competencies: "",
  tools: "Figma",
  experience: [
    {
      id: "e",
      title: "Designer",
      company: "Harbor",
      startDate: "2022",
      endDate: "2024",
      current: false,
      location: "",
      description: "Designed services.",
      responsibilities: "Interviewed customers.\nInterviewed customers.",
      achievements: "Reduced wait by 20%.",
    },
  ],
  projects: [],
  education: [],
  certifications: [],
  languages: [],
}
test("professional projection excludes private and aspirational fields and deduplicates repeated facts", () => {
  const facts = cvFacts(profile)
  const text = JSON.stringify(facts)
  for (const secret of [
    "private@example.com",
    "secret",
    "Become CEO",
    "unemployed",
    "currentSalary",
    "desiredSalary",
    "private health",
  ])
    assert.ok(!text.includes(secret), secret)
  assert.equal(facts.filter((f) => f.text === "Research").length, 1)
  assert.equal(
    facts.filter((f) => f.text === "Interviewed customers.").length,
    1,
  )
})
test("accepted CV is an independent snapshot with factual identity and explicit source excerpts", () => {
  const facts = cvFacts(profile)
  const anchor = facts.find((f) => f.field === "title")
  const impact = facts.find((f) => f.field === "achievements")
  const result = {
    summary: [{ sourceId: impact.id, text: "Reduced wait by 20%." }],
    selected: [anchor.id, impact.id],
  }
  assert.ok(validateCvResult(result, facts))
  assert.ok(
    validateCvResult(
      {
        ...result,
        summary: [{ sourceId: impact.id, text: "Cut waiting time by 20%." }],
      },
      facts,
    ),
  )
  assert.ok(
    !validateCvResult(
      {
        ...result,
        summary: [{ sourceId: impact.id, text: "Reduced wait by 90%." }],
      },
      facts,
    ),
  )
  assert.ok(!validateCvResult({ ...result, selected: ["invented"] }, facts))
  const saved = createCuratedCv(profile, facts, result, "en")
  assert.equal(saved.repository.experience[0].company, "Harbor")
  assert.equal(saved.repository.experience[0].responsibilities, "")
  assert.equal(saved.summary, "Reduced wait by 20%.")
  assert.equal(saved.repository.careerGoals, "")
  assert.equal(saved.repository.additionalInfo, "")
  profile.experience[0].company = "Changed"
  assert.equal(saved.repository.experience[0].company, "Harbor")
  assert.deepEqual(parseCuratedCv(JSON.stringify(saved)), saved)
  assert.equal(parseCuratedCv("{"), null)
})

test("supporting paraphrases condense source facts while protected metadata and originals stay intact", () => {
  const repo = structuredClone(profile)
  repo.experience[0].description =
    "Designed customer services. Designed services for customers. Used research to improve services by 20%."
  const facts = cvFacts(repo)
  const source = facts.find((f) => f.field === "description")
  const title = facts.find((f) => f.field === "title")
  const result = {
    summary: [
      {
        sourceId: source.id,
        text: "Designed customer services using research.",
      },
    ],
    selected: [title.id, source.id],
    wording: {
      [source.id]: "Improved customer services by 20% using research.",
    },
  }
  assert.ok(validateCvResult(result, facts))
  const cv = createCuratedCv(repo, facts, result, "en")
  assert.equal(
    cv.repository.experience[0].description,
    "Improved customer services by 20% using research.",
  )
  assert.ok(
    cv.sources
      .find((f) => f.id === source.id)
      .text.includes("Designed services for customers."),
  )
  assert.ok(
    !validateCvResult({ ...result, wording: { [title.id]: "CEO" } }, facts),
  )
})

test("requested density travels with facts and CV language; accepted density survives reload with legacy default", async () => {
  const { generateCv } = await import(path)
  const repo = structuredClone(profile)
  const facts = cvFacts(repo)
  const source = facts.find((f) => f.field === "achievements")
  const response = {
    summary: [{ sourceId: source.id, text: source.text }],
    selected: [source.id],
  }
  const original = structuredClone(repo)
  const originalFetch = globalThis.fetch
  try {
    for (const density of ["compact", "balanced", "detailed"]) {
      let outbound
      globalThis.fetch = async (_, options) => {
        outbound = JSON.parse(options.body)
        return new Response(JSON.stringify(response))
      }
      const result = await generateCv(
        facts,
        "pt-BR",
        "sk-test",
        new AbortController().signal,
        density,
      )
      assert.equal(outbound.density, density)
      assert.equal(outbound.cvLanguage, "pt-BR")
      assert.equal(outbound.locale, undefined)
      assert.equal(outbound.uiLocale, undefined)
      const saved = createCuratedCv(repo, facts, result, "pt-BR", density)
      assert.equal(parseCuratedCv(JSON.stringify(saved)).density, density)
      delete saved.density
      assert.equal(parseCuratedCv(JSON.stringify(saved)).density, "balanced")
    }
    assert.deepEqual(repo, original)
  } finally {
    globalThis.fetch = originalFetch
  }
})

test("CV language survives saved document reload and migrates legacy locale", () => {
  const facts = cvFacts(profile)
  const result = {
    summary: [{ sourceId: facts[0].id, text: facts[0].text }],
    selected: [facts[0].id],
  }
  const saved = createCuratedCv(profile, facts, result, "pt-BR")
  assert.equal(saved.cvLanguage, "pt-BR")
  assert.equal(saved.locale, undefined)
  assert.equal(parseCuratedCv(JSON.stringify(saved)).cvLanguage, "pt-BR")
  const { cvLanguage, ...legacy } = saved
  assert.equal(
    parseCuratedCv(JSON.stringify({ ...legacy, locale: cvLanguage }))
      .cvLanguage,
    "pt-BR",
  )
})

const stableDocument = () => ({
  id: "profile-a",
  revision: 1,
  entities: [],
  evidence: [],
  links: [],
  facts: [
    {
      id: "skill-a",
      revision: 3,
      owner: { profileId: "profile-a", id: "profile-a", revision: 1 },
      context: [],
      field: "skills",
      order: 0,
      kind: "legacy_block",
      value: "Research, Design\nResearch",
      assertion: "unknown",
      intent: "unknown",
      certainty: "unknown",
      support: "unsupported",
    },
  ],
})
test("stable CV projection retains a whole legacy block and fact revision across unrelated edits", () => {
  const doc = stableDocument()
  const facts = stableCvFacts(doc)
  assert.equal(facts.length, 1)
  assert.equal(facts[0].id, "skill-a")
  assert.equal(facts[0].text, "Research, Design\nResearch")
  assert.deepEqual(facts[0].reference, {
    profileId: "profile-a",
    id: "skill-a",
    revision: 3,
  })
  doc.revision++
  doc.facts.unshift({
    ...doc.facts[0],
    id: "private",
    field: "currentSalary",
    value: "secret",
  })
  assert.deepEqual(stableCvFacts(doc), facts)
})
test("accepted stable CV retains revisions and evidence; changed live support never rewrites it", () => {
  const doc = stableDocument()
  doc.facts[0].support = "supported"
  doc.evidence = [
    {
      id: "evidence-a",
      revision: 1,
      excerpt: "Original accepted Research and Design excerpt",
      origin: "user submission",
      approval: "approved",
    },
  ]
  doc.links = [
    {
      kind: "supports",
      state: "active",
      from: { id: "skill-a", revision: 3 },
      to: { id: "evidence-a", revision: 1 },
    },
  ]
  const facts = stableCvFacts(doc)
  const result = {
    summary: [{ sourceIds: ["skill-a"], text: facts[0].text }],
    selected: ["skill-a"],
  }
  const saved = createCuratedCv(profile, facts, result, "pt-BR", "compact", doc)
  assert.equal(saved.version, 2)
  const raw = JSON.stringify(saved)
  assert.deepEqual(parseCuratedCv(raw), saved)
  assert.equal(cvSupportStatus(saved, doc), "current")
  doc.revision++
  assert.equal(cvSupportStatus(saved, doc), "current")
  doc.facts[0].revision++
  doc.facts[0].value = "Edited"
  assert.equal(cvSupportStatus(saved, doc), "stale")
  assert.equal(JSON.stringify(saved), raw)
  assert.equal(
    saved.sources[0].evidence[0].excerpt,
    "Original accepted Research and Design excerpt",
  )
  const legacy = createCuratedCv(
    profile,
    cvFacts(profile),
    { summary: [{ sourceId: "f0", text: "Research" }], selected: ["f0"] },
    "en",
  )
  assert.equal(cvSupportStatus(legacy, doc), "legacy")
  assert.equal(
    cvSupportStatus(saved, { ...doc, id: "another-profile" }),
    "stale",
  )
})
test("combined summary cites every fact of the same role; aspirations and unknowns stay qualified", () => {
  const a = {
    id: "impact-a",
    section: "experience",
    entryId: "role-a",
    field: "achievements",
    text: "Reduced wait by 20%.",
  }
  const b = { ...a, id: "impact-b", text: "Designed services." }
  const result = {
    summary: [
      {
        sourceIds: [a.id, b.id],
        text: "Designed services and reduced wait by 20%.",
      },
    ],
    selected: [a.id, b.id],
  }
  assert.ok(validateCvResult(result, [a, b]))
  assert.ok(
    !validateCvResult(
      {
        ...result,
        summary: [
          {
            sourceIds: [a.id],
            text: result.summary[0].text.replace("20%", "90%"),
          },
        ],
      },
      [a, b],
    ),
  )
  assert.ok(!validateCvResult(result, [a, { ...b, entryId: "another-role" }]))
  for (const text of [
    "I hope to become certified in Java.",
    "Talvez tenha experiência em Java.",
  ]) {
    const f = { ...a, text }
    assert.ok(
      !validateCvResult(
        {
          summary: [{ sourceIds: [a.id], text: "Certified in Java." }],
          selected: [a.id],
        },
        [f],
      ),
    )
  }
  const doc = stableDocument()
  Object.assign(doc.facts[0], {
    kind: "statement",
    value: "Go",
    intent: "aspiration",
    assertion: "affirmed",
    certainty: "certain",
  })
  const facts = stableCvFacts(doc)
  assert.ok(
    !validateCvResult(
      {
        summary: [{ sourceIds: ["skill-a"], text: "Experienced in Go." }],
        selected: ["skill-a"],
      },
      facts,
    ),
  )
})
test("only minimized stable facts leave the client; evidence and contact stay local", async () => {
  const { generateCv } = await import(path)
  const doc = stableDocument()
  const facts = stableCvFacts(doc)
  facts[0].evidence = [
    {
      id: "e",
      revision: 1,
      excerpt: "PRIVATE-EXCERPT",
      origin: "professional_information",
      approval: "approved",
    },
  ]
  const originalFetch = globalThis.fetch
  try {
    globalThis.fetch = async (_, options) => {
      const outbound = JSON.parse(options.body)
      assert.equal(outbound.facts[0].reference.revision, 3)
      assert.equal(outbound.facts[0].id, "skill-a")
      assert.ok(!options.body.includes("PRIVATE-EXCERPT"))
      assert.equal(outbound.facts[0].evidence, undefined)
      return new Response(
        JSON.stringify({
          summary: [{ sourceIds: ["skill-a"], text: facts[0].text }],
          selected: ["skill-a"],
        }),
      )
    }
    await generateCv(facts, "en", "sk-test", new AbortController().signal)
    const tooHeavy = Array.from({ length: 501 }, (_, i) => ({
      ...facts[0],
      id: `s${i}`,
    }))
    await assert.rejects(
      () => generateCv(tooHeavy, "en", "sk-test", new AbortController().signal),
      /input/,
    )
  } finally {
    globalThis.fetch = originalFetch
  }
})
test("including an entry cannot implicitly promote aspirational protected qualifications", () => {
  const facts = [
    {
      id: "details",
      section: "education",
      entryId: "school",
      field: "details",
      text: "Studied design.",
    },
    {
      id: "degree",
      section: "education",
      entryId: "school",
      field: "degree",
      text: "PhD",
      kind: "statement",
      intent: "aspiration",
      assertion: "affirmed",
      certainty: "certain",
    },
  ]
  assert.equal(
    validateCvResult(
      {
        summary: [{ sourceIds: ["details"], text: "Studied design." }],
        selected: ["details"],
      },
      facts,
    ),
    false,
  )
})
test("saved support context excludes unrelated private Profile fields", () => {
  const doc = stableDocument()
  doc.facts.push({
    ...doc.facts[0],
    id: "salary",
    field: "currentSalary",
    value: "PRIVATE-SALARY",
  })
  const facts = stableCvFacts(doc)
  const saved = createCuratedCv(
    profile,
    facts,
    {
      summary: [{ sourceIds: ["skill-a"], text: facts[0].text }],
      selected: ["skill-a"],
    },
    "en",
    "balanced",
    doc,
  )
  assert.ok(!JSON.stringify(saved).includes("PRIVATE-SALARY"))
})

test("EN/PT production-handler results reach client acceptance without changing protected facts", async () => {
  const { execFileSync } = await import("node:child_process")
  const { generateCv } = await import(path)
  const root = new URL("..", import.meta.url).pathname
  const outputPath = join(
    mkdtempSync(join(tmpdir(), "cv-reliability-")),
    "controlled.json",
  )
  // Reuse #34's keyless adapter; consume fresh production HTTP output, not a
  // second hand-written client response or a stale committed report.
  execFileSync(
    "go",
    [
      "run",
      "./scripts/semantic-quality",
      "--fixtures",
      "docs/evaluations/semantic-quality/cases.cv-reliability.v1.json",
      "--split",
      "all",
      "--output",
      outputPath,
    ],
    {
      cwd: root,
      env: { ...process.env, GOCACHE: "/tmp/careeros-go-cache" },
      stdio: "pipe",
    },
  )
  const report = JSON.parse(readFileSync(outputPath, "utf8"))
  const corpus = JSON.parse(
    readFileSync(
      new URL(
        "../docs/evaluations/semantic-quality/cases.cv-reliability.v1.json",
        import.meta.url,
      ),
      "utf8",
    ),
  )
  const originalFetch = globalThis.fetch
  try {
    for (const c of corpus.cases) {
      const run = report.runs.find((r) => r.id === c.id)
      const facts = c.request.facts.map((f) => ({ ...f, evidence: [] }))
      let calls = 0
      globalThis.fetch = async () => {
        calls++
        return new Response(JSON.stringify(run.output), { status: run.status })
      }
      if (c.role === "negative_control") {
        await assert.rejects(
          () =>
            generateCv(
              facts,
              c.language,
              "sk-controlled",
              new AbortController().signal,
            ),
          /invalid_output/,
          c.id,
        )
        assert.equal(calls, 1, c.id)
        continue
      }
      const result = await generateCv(
        facts,
        c.language,
        "sk-controlled",
        new AbortController().signal,
      )
      assert.equal(calls, 1, c.id)
      assert.ok(validateCvResult(result, facts), c.id)
      assert.ok(result.summary.length > 0, c.id)
      const repo = {
        ...structuredClone(profile),
        skills: "",
        tools: "",
        competencies: "",
        experience: [],
        projects: [],
        education: [],
        certifications: [],
        languages: [],
      }
      for (const f of facts) {
        if (!f.entryId) {
          repo[f.field] = f.text
          continue
        }
        let entry = repo[f.section].find((e) => e.id === f.entryId)
        if (!entry) {
          const defaults = {
            experience: {
              title: "",
              company: "",
              startDate: "",
              endDate: "",
              current: false,
              location: "",
              description: "",
              responsibilities: "",
              achievements: "",
            },
            projects: {
              name: "",
              description: "",
              technologies: "",
              url: "",
              highlights: "",
            },
            education: {
              degree: "",
              institution: "",
              location: "",
              graduationDate: "",
              details: "",
            },
            certifications: {
              name: "",
              issuer: "",
              date: "",
              credentialId: "",
              url: "",
            },
            languages: { name: "", proficiency: "" },
          }
          entry = { id: f.entryId, ...defaults[f.section] }
          repo[f.section].push(entry)
        }
        entry[f.field] = f.text
      }
      const original = structuredClone(repo)
      const doc = facts[0].reference
        ? {
            id: "synthetic-profile",
            revision: 1,
            entities: [],
            evidence: [],
            links: [],
            facts: facts.map((f) => ({ ...f, value: f.text, revision: 1 })),
          }
        : undefined
      for (const density of ["compact", "balanced", "detailed"]) {
        const saved = createCuratedCv(
          repo,
          facts,
          result,
          c.language,
          density,
          doc,
        )
        assert.equal(saved.cvLanguage, c.language)
        assert.equal(saved.density, density)
        assert.deepEqual(parseCuratedCv(JSON.stringify(saved)), saved, c.id)
        assert.deepEqual(repo, original, c.id)
        assert.deepEqual(
          saved.sources.map((f) => f.text),
          result.selected.map((id) => facts.find((f) => f.id === id).text),
          c.id,
        )
        if (c.id.endsWith("heavy")) {
          assert.equal(saved.repository.experience.length, 1)
          assert.equal(saved.repository.experience[0].company, "Harbor 18")
          assert.equal(saved.repository.experience[0].startDate, "2021")
          assert.equal(saved.repository.experience[0].endDate, "2024")
          assert.equal(saved.repository.education[0].degree, "BSc Design")
          assert.equal(
            saved.repository.certifications[0].name,
            "Design Certificate",
          )
          assert.equal(
            saved.repository.languages[0].proficiency,
            "Intermediate",
          )
          assert.equal(saved.repository.projects[0].technologies, "Python")
          assert.match(saved.summary, /20%/)
          assert.ok(!saved.repository.projects[0].highlights.includes("20%"))
        }
      }
    }
  } finally {
    globalThis.fetch = originalFetch
  }
})

test("retained authorized EN/PT live summaries also pass client validation and acceptance", () => {
  const live = JSON.parse(
    readFileSync(
      new URL(
        "../docs/release-evidence/issue-65/live.v1.json",
        import.meta.url,
      ),
      "utf8",
    ),
  )
  const corpus = JSON.parse(
    readFileSync(
      new URL(
        "../docs/evaluations/semantic-quality/cases.cv-reliability.v1.json",
        import.meta.url,
      ),
      "utf8",
    ),
  )
  assert.equal(live.runs.length, 2)
  for (const run of live.runs) {
    const c = corpus.cases.find((c) => c.id === run.id)
    assert.equal(run.status, 200)
    assert.equal(run.calls.length, 1)
    assert.ok(validateCvResult(run.output, c.request.facts), run.id)
    const repo = structuredClone(profile)
    repo.experience[0].id = "e1"
    repo.experience[0].achievements = c.request.facts[0].text
    const saved = createCuratedCv(repo, c.request.facts, run.output, c.language)
    assert.deepEqual(parseCuratedCv(JSON.stringify(saved)), saved)
    assert.equal(saved.sources[0].text, c.request.facts[0].text)
    assert.match(saved.summary, /20%/)
  }
})

// ADR 0003 / issue #125 client review gate. One review unit exists per summary
// item and rewrite; a global acceptance never promotes uncertain or unsupported
// machine wording, and unresolved text can only be removed or reverted.
const gateCore = () => {
  const facts = cvFacts(profile)
  const skill = facts.find((f) => f.field === "skills")
  const desc = facts.find((f) => f.field === "description")
  return {
    facts,
    skill,
    desc,
    core: {
      summary: [{ sourceIds: [skill.id], text: skill.text }],
      selected: [skill.id, desc.id],
      wording: { [desc.id]: desc.text },
    },
  }
}
const gateSupport = (statements) => ({
  policyVersion: "cv-statement-support-v1",
  assessment: "complete",
  statements,
})
const gateStatement = (id, text, sourceIds, status, reason = "reason") => ({
  id,
  text,
  sourceIds,
  status,
  reason,
})

test("review units map one-to-one to generated statements and support fails closed on any mismatch", () => {
  const { skill, desc, core } = gateCore()
  assert.deepEqual(
    cvSupportUnits(core).map((unit) => [unit.id, unit.kind, unit.text]),
    [
      ["summary:0", "summary", skill.text],
      [`wording:${desc.id}`, "wording", desc.text],
    ],
  )
  const good = gateSupport([
    gateStatement("summary:0", skill.text, [skill.id], "supported"),
    gateStatement(`wording:${desc.id}`, desc.text, [desc.id], "uncertain"),
  ])
  assert.ok(validateCvSupport(good, core))
  const rejects = (mutate) => {
    const value = structuredClone(good)
    mutate(value)
    assert.equal(validateCvSupport(value, core), false)
  }
  rejects((v) => (v.policyVersion = "cv-statement-support-v2"))
  rejects((v) => (v.assessment = "unknown"))
  rejects((v) => (v.statements[0].text = "Kubernetes platforms"))
  rejects((v) => (v.statements[0].sourceIds = [desc.id]))
  rejects((v) => (v.statements[0].status = "supported!"))
  rejects((v) => (v.statements[0].reason = "x".repeat(501)))
  rejects((v) => v.statements.pop())
  rejects((v) =>
    v.statements.push(
      gateStatement("summary:1", skill.text, [skill.id], "supported"),
    ),
  )
  rejects((v) => (v.statements[1].id = "summary:0"))
  rejects((v) => (v.statements[0].extra = "field"))
  // An unavailable assessment must already present every unit as uncertain.
  const unavailable = gateSupport([
    gateStatement("summary:0", skill.text, [skill.id], "uncertain"),
    gateStatement(`wording:${desc.id}`, desc.text, [desc.id], "uncertain"),
  ])
  unavailable.assessment = "unavailable"
  assert.ok(validateCvSupport(unavailable, core))
  rejects((v) => {
    v.assessment = "unavailable"
    v.statements[0].status = "supported"
  })
})

test("client consumes a v3 response and rejects an incomplete or unknown-version payload", async () => {
  const { generateCv } = await import(path)
  const { facts, skill, desc, core } = gateCore()
  const support = gateSupport([
    gateStatement("summary:0", skill.text, [skill.id], "supported"),
    gateStatement(
      `wording:${desc.id}`,
      desc.text,
      [desc.id],
      "unsupported",
      "invented",
    ),
  ])
  const original = globalThis.fetch
  try {
    let outbound
    globalThis.fetch = async (_url, options) => {
      outbound = JSON.parse(options.body)
      return new Response(
        JSON.stringify({ schemaVersion: 3, ...core, support }),
      )
    }
    const result = await generateCv(
      facts,
      "en",
      "sk-test",
      new AbortController().signal,
    )
    // Activation is #128: the legacy request is unchanged until then.
    assert.equal(outbound.statementSupport, undefined)
    assert.equal(result.support.assessment, "complete")
    assert.deepEqual(
      cvReviewUnits(result).map((unit) => [unit.id, unit.status]),
      [
        ["summary:0", "supported"],
        [`wording:${desc.id}`, "unsupported"],
      ],
    )
    const respond = (payload) =>
      (globalThis.fetch = async () =>
        new Response(JSON.stringify(payload), { status: 200 }))
    // Missing one record fails closed.
    respond({
      schemaVersion: 3,
      ...core,
      support: gateSupport([support.statements[0]]),
    })
    await assert.rejects(
      () => generateCv(facts, "en", "sk-test", new AbortController().signal),
      /invalid_output/,
    )
    // An unknown/malformed version never becomes accepted legacy wording.
    respond({ schemaVersion: 2, ...core })
    await assert.rejects(
      () => generateCv(facts, "en", "sk-test", new AbortController().signal),
      /invalid_output/,
    )
    respond({
      schemaVersion: 3,
      ...core,
      support: { ...support, assessment: "nope" },
    })
    await assert.rejects(
      () => generateCv(facts, "en", "sk-test", new AbortController().signal),
      /invalid_output/,
    )
    // A legacy v2 response without any version stays accepted.
    respond(core)
    const legacy = await generateCv(
      facts,
      "en",
      "sk-test",
      new AbortController().signal,
    )
    assert.equal(legacy.support, undefined)
    assert.equal(cvReviewUnits(legacy), null)
    assert.ok(cvReviewResolved(legacy, emptyCvReview))
  } finally {
    globalThis.fetch = original
  }
})

test("partial acceptance removes only reviewed-out units and never invents filler", () => {
  const { facts, skill, desc, core } = gateCore()
  const result = {
    ...core,
    support: gateSupport([
      gateStatement("summary:0", skill.text, [skill.id], "supported"),
      gateStatement(
        `wording:${desc.id}`,
        desc.text,
        [desc.id],
        "uncertain",
        "ambiguous",
      ),
    ]),
  }
  assert.equal(cvReviewResolved(result, emptyCvReview), false)
  assert.throws(
    () => acceptCvResult(result, facts, emptyCvReview),
    /unresolved_output/,
  )
  // The unresolved rewrite may be reverted; the supported summary stays.
  const resolved = acceptCvResult(result, facts, {
    discardedSummaryIds: [],
    revertedWordingIds: [desc.id],
  })
  assert.deepEqual(resolved.summary, [
    { sourceIds: [skill.id], text: skill.text },
  ])
  assert.deepEqual(resolved.wording, {})
  assert.deepEqual(resolved.selected, [skill.id, desc.id])
  assert.equal(resolved.support, undefined)
  assert.ok(validateCvResult(resolved, facts))
  // Removing the only summary sentence cannot fall back to invented filler.
  const emptied = {
    discardedSummaryIds: ["summary:0"],
    revertedWordingIds: [desc.id],
  }
  assert.equal(cvReviewResolved(result, emptied), false)
  assert.throws(() => acceptCvResult(result, facts, emptied), /invalid_output/)
  // Unresolved text kept inside the acceptance is rejected, not promoted.
  assert.throws(
    () => createCuratedCv(profile, facts, result, "en"),
    /unresolved_output/,
  )
})

test("all-supported v3 content is accepted with judgments and discards applied", () => {
  const { facts, skill, desc, core } = gateCore()
  const result = {
    ...core,
    support: gateSupport([
      gateStatement("summary:0", skill.text, [skill.id], "supported"),
      gateStatement(`wording:${desc.id}`, desc.text, [desc.id], "supported"),
    ]),
  }
  assert.ok(cvReviewResolved(result, emptyCvReview))
  assert.deepEqual(acceptCvResult(result, facts, emptyCvReview), core)
  // Discarding the only summary sentence cannot invent filler.
  assert.throws(
    () =>
      acceptCvResult(result, facts, {
        discardedSummaryIds: ["summary:0"],
        revertedWordingIds: [],
      }),
    /invalid_output/,
  )
  // Reverting a supported rewrite falls back to the verbatim source fact.
  const reverted = acceptCvResult(result, facts, {
    discardedSummaryIds: [],
    revertedWordingIds: [desc.id],
  })
  assert.deepEqual(reverted.wording, {})
  assert.ok(validateCvResult(reverted, facts))
})

// ADR 0003 / issue #126 user-authored provenance. An explicit edit or
// confirmation is recorded as the user's own wording and never inherits the
// machine judgment; only a real text change resolves an unresolved unit.
const reviewCore = () => {
  const facts = cvFacts(profile)
  const skill = facts.find((f) => f.field === "skills")
  const achievement = facts.find((f) => f.field === "achievements")
  const desc = facts.find((f) => f.field === "description")
  const core = {
    summary: [
      { sourceIds: [skill.id], text: skill.text },
      { sourceIds: [achievement.id], text: achievement.text },
    ],
    selected: [skill.id, achievement.id, desc.id],
    wording: { [desc.id]: desc.text },
  }
  const support = gateSupport([
    gateStatement("summary:0", skill.text, [skill.id], "supported", "direct"),
    gateStatement(
      "summary:1",
      achievement.text,
      [achievement.id],
      "unsupported",
      "invented scope",
    ),
    gateStatement(
      `wording:${desc.id}`,
      desc.text,
      [desc.id],
      "uncertain",
      "ambiguous",
    ),
  ])
  return { facts, skill, achievement, desc, core, result: { ...core, support } }
}

test("explicit edits and confirmations record user provenance and never inherit machine support", () => {
  const { facts, skill, achievement, desc, result } = reviewCore()
  const decisions = {
    discardedSummaryIds: [],
    revertedWordingIds: [],
    editedText: { "summary:1": "Own claim: improved service outcomes." },
    confirmedIds: [`wording:${desc.id}`],
  }
  assert.ok(cvReviewResolved(result, decisions))
  const {
    result: resolved,
    provenance,
    authored,
  } = resolveCvReview(result, facts, decisions)
  assert.deepEqual(resolved.summary, [
    { sourceIds: [skill.id], text: skill.text },
    {
      sourceIds: [achievement.id],
      text: "Own claim: improved service outcomes.",
    },
  ])
  assert.deepEqual(resolved.wording, { [desc.id]: desc.text })
  // The edited/confirmed text is carried as the exact authored value.
  assert.equal(authored.summary[1], "Own claim: improved service outcomes.")
  assert.equal(authored.summary[0], undefined)
  assert.deepEqual(authored.wording, {})
  const byUnit = Object.fromEntries(provenance.map((r) => [r.id, r]))
  assert.deepEqual([byUnit["summary:0"].origin, byUnit["summary:0"].decision], [
    "generated",
    "keep_generated",
  ])
  assert.equal(byUnit["summary:0"].original, undefined)
  assert.deepEqual([byUnit["summary:1"].origin, byUnit["summary:1"].decision], [
    "user_edited",
    "edit",
  ])
  assert.deepEqual(byUnit["summary:1"].original, {
    text: achievement.text,
    sourceIds: [achievement.id],
    status: "unsupported",
    reason: "invented scope",
  })
  assert.deepEqual(
    [
      byUnit[`wording:${desc.id}`].origin,
      byUnit[`wording:${desc.id}`].decision,
    ],
    ["user_confirmed", "confirm"],
  )
  assert.equal(byUnit[`wording:${desc.id}`].original.status, "uncertain")
  // User text is never exposed with a machine status.
  assert.ok(
    provenance
      .filter((r) => r.origin !== "generated")
      .every((r) => !("status" in r)),
  )
  assert.ok(validateCvReviewRecords(provenance, facts))
})

test("only a changed, non-empty wording resolves an unresolved unit", () => {
  const { facts, achievement, desc, result } = reviewCore()
  // Keep the supported summary and rewrite so only `summary:1` stays unresolved.
  const wordingSupported = {
    ...result,
    support: {
      ...result.support,
      statements: result.support.statements.map((statement) =>
        statement.id === `wording:${desc.id}`
          ? { ...statement, status: "supported" }
          : statement,
      ),
    },
  }
  // `editedText` equal to the machine text is not an edit and never promotes it.
  const asTyped = {
    ...emptyCvReview,
    editedText: { "summary:1": achievement.text },
  }
  assert.equal(cvReviewResolved(wordingSupported, asTyped), false)
  assert.equal(
    cvReviewEdited({ id: "summary:1", text: achievement.text }, asTyped),
    undefined,
  )
  assert.throws(
    () => resolveCvReview(wordingSupported, facts, asTyped),
    /unresolved_output/,
  )
  // An empty edit is not a correction either.
  const blank = { ...emptyCvReview, editedText: { "summary:1": "   " } }
  assert.equal(cvReviewResolved(wordingSupported, blank), false)
  assert.throws(
    () => resolveCvReview(wordingSupported, facts, blank),
    /unresolved_output/,
  )
  // A real edit resolves the unit and records user provenance.
  const realEdit = {
    ...emptyCvReview,
    editedText: { "summary:1": "Own statement about impact." },
  }
  assert.ok(cvReviewResolved(wordingSupported, realEdit))
  // Clearing an edit restores the original machine judgment.
  const allSupported = {
    ...result,
    support: {
      ...result.support,
      statements: result.support.statements.map((statement) => ({
        ...statement,
        status: "supported",
      })),
    },
  }
  const supportedEdit = {
    ...emptyCvReview,
    editedText: { "summary:0": "Reworded supported statement." },
  }
  assert.equal(
    resolveCvReview(allSupported, facts, supportedEdit).provenance.find(
      (r) => r.id === "summary:0",
    ).origin,
    "user_edited",
  )
  assert.equal(
    resolveCvReview(allSupported, facts, emptyCvReview).provenance.find(
      (r) => r.id === "summary:0",
    ).origin,
    "generated",
  )
  assert.ok(userAuthoredCvText("ok"))
  assert.equal(userAuthoredCvText("  "), false)
  assert.equal(userAuthoredCvText("x".repeat(12_289)), false)
})

test("the machine numeric guard applies to machine text but not to an explicit user edit", () => {
  const { facts, achievement, desc, result } = reviewCore()
  const edited = "Reduced wait by 90%."
  const machineRewrite = {
    summary: [{ sourceIds: [achievement.id], text: edited }],
    selected: [achievement.id],
  }
  // Machine text that introduces an unsupported metric is still rejected.
  assert.equal(validateCvResult(machineRewrite, facts), false)
  // The same text is accepted only when recorded as the user's own wording.
  assert.ok(validateCvResult(machineRewrite, facts, { summary: [edited] }))
  const decisions = {
    ...emptyCvReview,
    editedText: { "summary:1": edited },
  }
  const wordingSupported = {
    ...result,
    support: {
      ...result.support,
      statements: result.support.statements.map((statement) =>
        statement.id === `wording:${desc.id}`
          ? { ...statement, status: "supported" }
          : statement,
      ),
    },
  }
  const { result: resolved } = resolveCvReview(
    wordingSupported,
    facts,
    decisions,
  )
  assert.equal(resolved.summary[1].text, edited)
  // The supported machine unit is untouched.
  assert.deepEqual(resolved.summary[0], result.summary[0])
})

const provenanceDocument = () => ({
  id: "profile-b",
  revision: 1,
  entities: [
    { id: "role-b", revision: 1, kind: "experience", legacyId: "role-legacy" },
  ],
  evidence: [],
  links: [],
  facts: [
    {
      id: "desc-b",
      revision: 1,
      owner: { profileId: "profile-b", id: "role-b", revision: 1 },
      context: [],
      field: "description",
      order: 0,
      kind: "legacy_block",
      value: "Designed customer services.",
      assertion: "unknown",
      intent: "unknown",
      certainty: "unknown",
      support: "unsupported",
    },
    {
      id: "impact-b",
      revision: 1,
      owner: { profileId: "profile-b", id: "role-b", revision: 1 },
      context: [],
      field: "achievements",
      order: 1,
      kind: "legacy_block",
      value: "Reduced wait by 20%.",
      assertion: "unknown",
      intent: "unknown",
      certainty: "unknown",
      support: "unsupported",
    },
  ],
})

test("an accepted v3 snapshot persists provenance and round-trips without changing the Profile", () => {
  const doc = provenanceDocument()
  const original = structuredClone(doc)
  const facts = stableCvFacts(doc)
  const desc = facts.find((f) => f.field === "description")
  const impact = facts.find((f) => f.field === "achievements")
  const result = {
    summary: [{ sourceIds: [desc.id], text: desc.text }],
    selected: [desc.id, impact.id],
    wording: { [impact.id]: impact.text },
    support: gateSupport([
      gateStatement("summary:0", desc.text, [desc.id], "supported", "direct"),
      gateStatement(
        `wording:${impact.id}`,
        impact.text,
        [impact.id],
        "uncertain",
        "unclear",
      ),
    ]),
  }
  const decisions = {
    discardedSummaryIds: [],
    revertedWordingIds: [],
    editedText: { "summary:0": "Designed customer services for 30 teams." },
    confirmedIds: [`wording:${impact.id}`],
  }
  const saved = createCuratedCv(
    profile,
    facts,
    result,
    "pt-BR",
    "compact",
    doc,
    decisions,
  )
  assert.equal(saved.version, 3)
  assert.equal(saved.summary, "Designed customer services for 30 teams.")
  assert.deepEqual(
    saved.provenance.map((r) => [r.id, r.origin, r.decision]),
    [
      ["summary:0", "user_edited", "edit"],
      [`wording:${impact.id}`, "user_confirmed", "confirm"],
    ],
  )
  assert.deepEqual(parseCuratedCv(JSON.stringify(saved)), saved)
  assert.equal(cvSupportStatus(saved, doc), "current")
  assert.deepEqual(doc, original)
  // A user confirmation cannot waive the source identity guard.
  const other = provenanceDocument()
  other.facts[0].revision = 2
  assert.equal(cvSupportStatus(saved, other), "stale")
})

test("malformed or incomplete v3 provenance never loads as an accepted CV", () => {
  const doc = provenanceDocument()
  const facts = stableCvFacts(doc)
  const desc = facts.find((f) => f.field === "description")
  const impact = facts.find((f) => f.field === "achievements")
  const result = {
    summary: [{ sourceIds: [desc.id], text: desc.text }],
    selected: [desc.id, impact.id],
    wording: { [impact.id]: impact.text },
    support: gateSupport([
      gateStatement("summary:0", desc.text, [desc.id], "supported", "direct"),
      gateStatement(
        `wording:${impact.id}`,
        impact.text,
        [impact.id],
        "supported",
        "direct",
      ),
    ]),
  }
  const saved = createCuratedCv(
    profile,
    facts,
    result,
    "en",
    "balanced",
    doc,
    emptyCvReview,
  )
  assert.equal(saved.version, 3)
  assert.ok(validateCvReviewRecords(saved.provenance, facts))
  assert.deepEqual(parseCuratedCv(JSON.stringify(saved)), saved)
  const mutate = (change) => {
    const value = structuredClone(saved)
    change(value)
    assert.equal(parseCuratedCv(JSON.stringify(value)), null)
  }
  // Origin/decision mismatch would silently mislabel user text.
  mutate((v) => (v.provenance[0].origin = "user_confirmed"))
  mutate((v) => (v.provenance[0].decision = "confirm"))
  // Unknown citations and unknown decisions are rejected.
  mutate((v) => (v.provenance[0].sourceIds = ["ghost"]))
  mutate((v) => (v.provenance[0].decision = "approve"))
  // Missing provenance cannot fall back to unverified v2 content.
  mutate((v) => delete v.provenance)
  mutate((v) => (v.provenance = []))
  // A record that does not correspond to the stored summary is rejected.
  mutate((v) =>
    v.provenance.push({
      id: "summary:9",
      kind: "summary",
      decision: "keep_generated",
      origin: "generated",
      text: "Ghost sentence.",
      sourceIds: [desc.id],
    }),
  )
  mutate((v) => (v.provenance[0].text = "Rewritten without a record."))
  // A fabricated original judgment on a kept machine record is rejected.
  mutate(
    (v) =>
      (v.provenance[0].original = {
        text: v.provenance[0].text,
        sourceIds: [...v.provenance[0].sourceIds],
        status: "supported",
        reason: "fabricated",
      }),
  )
})

test("reverting a rewrite and discarding a sentence keep their decisions as history", () => {
  const { facts, skill, achievement, desc, result } = reviewCore()
  const decisions = {
    discardedSummaryIds: ["summary:1"],
    revertedWordingIds: [desc.id],
  }
  const { result: resolved, provenance } = resolveCvReview(
    result,
    facts,
    decisions,
  )
  assert.deepEqual(resolved.summary, [
    { sourceIds: [skill.id], text: skill.text },
  ])
  assert.deepEqual(resolved.wording, {})
  const byUnit = Object.fromEntries(provenance.map((r) => [r.id, r]))
  assert.deepEqual([byUnit["summary:1"].decision, byUnit["summary:1"].text], [
    "discard",
    "",
  ])
  assert.equal(byUnit["summary:1"].original.text, achievement.text)
  assert.deepEqual(
    [byUnit[`wording:${desc.id}`].decision, byUnit[`wording:${desc.id}`].text],
    ["revert_to_source", ""],
  )
  assert.equal(byUnit[`wording:${desc.id}`].original.status, "uncertain")
  assert.ok(validateCvReviewRecords(provenance, facts))
})

// ADR 0003 / issue #127 legacy snapshot compatibility. Saved v1/v2 snapshots
// predate the statement assessment: their generated wording is historical and
// unverified. Trust is derived from the stored metadata, is independent of
// source freshness and is never upgraded by loading or re-saving.
const legacyV1 = () => {
  const facts = cvFacts(profile)
  const source = facts.find((f) => f.field === "achievements")
  const saved = createCuratedCv(
    profile,
    facts,
    {
      summary: [{ sourceId: source.id, text: source.text }],
      selected: [source.id],
    },
    "en",
  )
  assert.equal(saved.version, 1)
  return saved
}
const legacyV2 = () => {
  const doc = stableDocument()
  doc.facts[0].support = "supported"
  const facts = stableCvFacts(doc)
  const saved = createCuratedCv(
    profile,
    facts,
    {
      summary: [{ sourceIds: [facts[0].id], text: facts[0].text }],
      selected: [facts[0].id],
    },
    "pt-BR",
    "balanced",
    doc,
  )
  assert.equal(saved.version, 2)
  return { saved, doc }
}
// A valid v3 snapshot with a user-authored summary sentence and a
// machine-supported one, built from the same idea as the #126 provenance tests.
const assessedV3 = (cvLanguage = "en") => {
  const doc = provenanceDocument()
  const facts = stableCvFacts(doc)
  const desc = facts.find((f) => f.field === "description")
  const impact = facts.find((f) => f.field === "achievements")
  const result = {
    summary: [
      { sourceIds: [desc.id], text: desc.text },
      { sourceIds: [impact.id], text: impact.text },
    ],
    selected: [desc.id, impact.id],
    support: gateSupport([
      gateStatement("summary:0", desc.text, [desc.id], "supported", "direct"),
      gateStatement(
        "summary:1",
        impact.text,
        [impact.id],
        "supported",
        "direct",
      ),
    ]),
  }
  const saved = createCuratedCv(
    profile,
    facts,
    result,
    cvLanguage,
    "balanced",
    doc,
    {
      discardedSummaryIds: [],
      revertedWordingIds: [],
      editedText: { "summary:0": "Designed customer services for 30 teams." },
    },
  )
  assert.equal(saved.version, 3)
  return { saved, facts, doc, desc, impact }
}

test("saved v1/v2 wording is unverified independently of source freshness", () => {
  const v1 = legacyV1()
  assert.equal(cvWordingTrust(v1), "unverified")
  assert.equal(cvSupportStatus(v1, null), "legacy")
  const { saved, doc } = legacyV2()
  assert.equal(cvWordingTrust(saved), "unverified")
  // Unchanged sources are "current" yet the wording stays unverified: freshness
  // and semantic trust are separate axes.
  assert.equal(cvSupportStatus(saved, doc), "current")
  assert.equal(cvWordingTrust(saved), "unverified")
  doc.facts[0].value = "Changed"
  assert.equal(cvSupportStatus(saved, doc), "stale")
  assert.equal(cvWordingTrust(saved), "unverified")
})

test("only a complete, self-consistent v3 provenance reads as assessed", () => {
  const { saved } = assessedV3()
  assert.equal(cvWordingTrust(saved), "assessed")
  // A malformed or incomplete v3 document never defaults to machine-supported.
  const invalid = (change) => {
    const value = structuredClone(saved)
    change(value)
    assert.equal(cvWordingTrust(value), "unverified")
  }
  invalid((v) => (v.version = 2))
  invalid((v) => delete v.provenance)
  invalid((v) => (v.provenance = []))
  invalid((v) => (v.provenance[0].decision = "approve"))
  invalid((v) => (v.provenance[0].origin = "user_confirmed"))
  invalid((v) => (v.provenance[0].sourceIds = ["ghost"]))
  invalid((v) => delete v.sources)
  // A legacy v2 snapshot whose provenance is fabricated is not assessed either.
  const { saved: legacy } = legacyV2()
  assert.equal(cvWordingTrust({ ...legacy, version: 3 }), "unverified")
})

test("loading legacy v1/v2 snapshots preserves content, calls no provider and never mutates the Profile", () => {
  const { saved, doc } = legacyV2()
  const originalDoc = structuredClone(doc)
  const v1 = legacyV1()
  const raws = [JSON.stringify(v1), JSON.stringify(saved)]
  const originalFetch = globalThis.fetch
  globalThis.fetch = () => {
    throw new Error("legacy load must not call the provider")
  }
  try {
    for (const raw of raws) {
      const parsed = parseCuratedCv(raw)
      assert.ok(parsed)
      // Round-trips byte for byte: content loss and automatic source rewriting
      // would change the serialized document.
      assert.equal(JSON.stringify(parsed), raw)
      assert.ok(parsed.summary.length > 0)
      assert.ok(parsed.sources.length > 0)
      assert.ok(parsed.sources.every((f) => f.text.length > 0))
      cvSupportStatus(parsed, doc)
      cvWordingTrust(parsed)
    }
  } finally {
    globalThis.fetch = originalFetch
  }
  assert.deepEqual(doc, originalDoc)
})

test("an assessed v3 snapshot round-trips with its trust intact in EN/PT", () => {
  for (const cvLanguage of ["en", "pt-BR"]) {
    const { saved } = assessedV3(cvLanguage)
    assert.equal(cvWordingTrust(saved), "assessed")
    const reloaded = parseCuratedCv(JSON.stringify(saved))
    assert.ok(reloaded)
    assert.equal(reloaded.cvLanguage, cvLanguage)
    assert.equal(cvWordingTrust(reloaded), "assessed")
    assert.deepEqual(reloaded, saved)
  }
})

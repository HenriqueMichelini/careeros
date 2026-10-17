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
  createCuratedCv,
  parseCuratedCv,
  validateCvResult,
  validateCvSupport,
  cvSupportUnits,
  cvReviewUnits,
  cvReviewResolved,
  acceptCvResult,
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

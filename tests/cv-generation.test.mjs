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

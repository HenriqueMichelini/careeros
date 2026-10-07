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
const { cvFacts, createCuratedCv, parseCuratedCv, validateCvResult } =
  await import(path)
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

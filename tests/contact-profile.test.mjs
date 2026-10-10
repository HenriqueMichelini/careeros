import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync, writeFileSync, mkdtempSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"

const temp = mkdtempSync(join(tmpdir(), "careeros-contact-"))
for (const name of ["profile", "cover-letter", "fieldDecision", "jobContext", "qualificationEvidence", "ai"]) {
  const source = readFileSync(new URL(`../src/lib/${name}.ts`, import.meta.url), "utf8")
  const compiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText.replace(/from ['"]\.\/jobContext['"]/g, 'from "./jobContext.mjs"').replace(/from ['"]\.\/fieldDecision['"]/g, 'from "./fieldDecision.mjs"').replace(/from ['"]\.\/cover-letter['"]/g, 'from "./cover-letter.mjs"').replace(/from ['"]\.\/profile['"]/g, 'from "./profile.mjs"')
  writeFileSync(join(temp, `${name}.mjs`), compiled.replace(/import fields from [^\n]+\n/, 'const fields = '+readFileSync(new URL("../internal/qualificationmatching/fields.json", import.meta.url),"utf8")+';\n').replaceAll('"./qualificationEvidence"', '"./qualificationEvidence.mjs"'))
}
const { withContactFields, careerProfile, cvQualifications, validQualifications } = await import(join(temp, "profile.mjs"))
const { findProfileGaps, generateMaterials } = await import(join(temp, "ai.mjs"))

const oldProfile = {
  careerGoals: "Lead teams", skills: "Go", competencies: "Communication",
  experience: [], tools: "Docker", projects: [], employmentStatus: "",
  currentSalary: "", desiredSalary: "", additionalInfo: "Certificate",
}

test("old saved Profiles gain blank contact fields without losing existing facts", () => {
  const migrated = withContactFields(oldProfile)
  assert.deepEqual(careerProfile(migrated), oldProfile)
  assert.deepEqual(
    [migrated.fullName, migrated.email, migrated.phone, migrated.location, migrated.professionalLinks],
    ["", "", "", "", ""],
  )
  assert.deepEqual(cvQualifications(migrated), { education: [], certifications: [], languages: [] })
  assert.equal(migrated.additionalInfo, "Certificate")
})

test("application AI requests exclude contact facts", async () => {
  const repo = {
    ...withContactFields(oldProfile), fullName: "Ada Lovelace", email: "ada@example.test",
    phone: "+1 555 0100", location: "London", professionalLinks: "example.test/ada",
  }
  const originalFetch = globalThis.fetch
  const requests = []
  globalThis.fetch = async (url, options) => {
    requests.push({ url, body: JSON.parse(options.body) })
    if (url === "/api/qualification-gaps") return { ok: true, json: async () => ({ gaps: [] }) }
    return { ok: true, json: async () => ({
      jobTitle: "Engineer", company: "Acme", jobSummary: "Role", resume: "Resume",
      coverLetter: { greeting: "Dear team,", body: "I build systems.", closing: "Sincerely," }, applicationAnswers: "Answers",
    }) }
  }
  try {
    await findProfileGaps(repo, "Engineer", "test-key")
    const materials = await generateMaterials(repo, "Engineer", "test-key", [], "pt-BR")
    assert.equal(materials.cvLanguage, "pt-BR")
    assert.equal(requests.at(-1).body.cvLanguage, "pt-BR")
    assert.equal(requests.at(-1).body.uiLocale, undefined)
    assert.deepEqual(requests.map(request => request.url), [
      "/api/qualification-gaps", "/api/application-draft",
    ])
    for (const request of requests) assert.deepEqual(request.body.repository, oldProfile)
    assert.deepEqual(requests[0].body.qualifications, { education: [], certifications: [], languages: [] })
    assert.deepEqual(requests[1].body.qualifications, { education: [], certifications: [], languages: [] })
  } finally {
    globalThis.fetch = originalFetch
  }
})

test("structured qualifications reach relevant application AI requests", async () => {
  const repo = {
    ...withContactFields(oldProfile),
    education: [{ id: "e1", degree: "BSc", institution: "Example University", location: "London", graduationDate: "2018", details: "Honors" }],
    certifications: [{ id: "c1", name: "Cloud certificate", issuer: "Example", date: "2024", credentialId: "ABC", url: "https://example.test" }],
    languages: [{ id: "l1", name: "English", proficiency: "Fluent" }],
  }
  assert.equal(validQualifications(repo), true)
  const originalFetch = globalThis.fetch
  const requests = []
  globalThis.fetch = async (url, options) => {
    requests.push({ url, body: JSON.parse(options.body) })
    if (url === "/api/qualification-gaps") return { ok: true, json: async () => ({ gaps: [] }) }
    return { ok: true, json: async () => ({ jobTitle: "Engineer", company: "Acme", jobSummary: "Role", resume: "Resume", coverLetter: { greeting: "Dear team,", body: "I build systems.", closing: "Sincerely," }, applicationAnswers: "Answers" }) }
  }
  try {
    await findProfileGaps(repo, "Engineer", "test-key")
    await generateMaterials(repo, "Engineer", "test-key")
    assert.deepEqual(requests[0].body.qualifications, cvQualifications(repo))
    assert.deepEqual(requests[1].body.qualifications, cvQualifications(repo))
    assert.equal(requests.every(request => !JSON.stringify(request.body).includes("London") || request.url !== "/api/profile/review"), true)
  } finally { globalThis.fetch = originalFetch }
})

test("qualification validation matches request limits", () => {
  const repo = withContactFields(oldProfile)
  const language = (id, name = "English") => ({ id, name, proficiency: "Fluent" })
  repo.languages = Array.from({ length: 40 }, (_, index) => language(`l${index}`))
  assert.equal(validQualifications(repo), true)
  repo.languages.push(language("l40"))
  assert.equal(validQualifications(repo), false)
  repo.languages.pop()
  repo.languages[0] = language("l0", "é".repeat(1001))
  assert.equal(validQualifications(repo), false)
})

test("new draft signatures use the trimmed saved name exactly once, with no identity sent to AI", async () => {
  const originalFetch = globalThis.fetch
  const parts = { greeting: "Prezada equipe,", body: "Minha experiência com Go atende aos requisitos da vaga.", closing: "Atenciosamente," }
  let requestBody
  globalThis.fetch = async (_url, options) => {
    requestBody = options.body
    return { ok: true, json: async () => ({ jobTitle: "Engenheiro", company: "Acme", jobSummary: "Vaga", resume: "Resume", coverLetter: parts, applicationAnswers: "Answers" }) }
  }
  try {
    const named = await generateMaterials({ ...withContactFields(oldProfile), fullName: "  João Gonçalves  " }, "Go", "test-key")
    assert.equal(named.coverLetter, "Prezada equipe,\n\nMinha experiência com Go atende aos requisitos da vaga.\n\nAtenciosamente,\nJoão Gonçalves")
    assert.equal(named.coverLetterHasSignature, true)
    assert.equal(requestBody.includes("João"), false)
    for (const repo of [{ ...withContactFields(oldProfile), fullName: undefined }, { ...withContactFields(oldProfile), fullName: "  " }]) {
      const unnamed = await generateMaterials(repo, "Go", "test-key")
      assert.equal(unnamed.coverLetter, "Prezada equipe,\n\nMinha experiência com Go atende aos requisitos da vaga.\n\nAtenciosamente,")
      assert.equal(unnamed.coverLetterHasSignature, false)
    }
    parts.greeting = "Dear team,"
    parts.body = "I build reliable Go systems."
    parts.closing = "Sincerely,"
    const english = await generateMaterials({ ...withContactFields(oldProfile), fullName: "Érica Müller" }, "Go", "test-key")
    assert.equal(english.coverLetter, "Dear team,\n\nI build reliable Go systems.\n\nSincerely,\nÉrica Müller")
    for (const body of ["Additionally, I build reliable systems.", "Atualmente, desenvolvo sistemas confiáveis.", "I am AWS certified.", "At Harbor Works, I build reliable systems."]) {
      parts.body = body
      const draft = await generateMaterials({ ...withContactFields(oldProfile), fullName: "Érica Müller" }, "Go", "test-key")
      assert.equal(draft.coverLetter, `Dear team,\n\n${body}\n\nSincerely,\nÉrica Müller`)
    }
  } finally { globalThis.fetch = originalFetch }
})

test("model signatures and incomplete structures fail safely rather than duplicating or inventing identity", async () => {
  const originalFetch = globalThis.fetch
  try {
    for (const coverLetter of [
      "Dear team,\nI build systems.\nSincerely,\nJane Doe",
      { greeting: "Dear team,", body: "I build systems." },
      { greeting: "Dear team,", body: "I build systems.", closing: "Sincerely,\nJane Doe" },
      { greeting: "Dear team,", body: "I build systems.\n\nJane Doe", closing: "Sincerely," },
      { greeting: "Dear team,", body: "I build systems.\n\nJane Doe, Ph.D.", closing: "Sincerely," },
      { greeting: "Dear team,", body: "I build systems.\n\nDr. João da Silva, Engenheiro.", closing: "Sincerely," },
      { greeting: "Dear team,", body: "My name is Jane Doe.", closing: "Sincerely," },
      { greeting: "Dear team,", body: "Minha experiência com Go é sólida. João Gonçalves.", closing: "Sincerely," },
      { greeting: "Dear team,", body: "I build systems.", closing: "Sincerely,", signature: "Jane Doe" },
    ]) {
      globalThis.fetch = async () => ({ ok: true, json: async () => ({ jobTitle: "Engineer", company: "Acme", jobSummary: "Role", resume: "Resume", coverLetter, applicationAnswers: "Answers" }) })
      for (const fullName of coverLetter.body?.includes("João Gonçalves") ? ["João Gonçalves"] : ["João Gonçalves", ""]) {
        await assert.rejects(generateMaterials({ ...withContactFields(oldProfile), fullName }, "Go", "test-key"), error => error.code === "invalid_output")
      }
    }
  } finally { globalThis.fetch = originalFetch }
})

test("draft requests use approved canonical evidence and preserve coverage disclosure with local identity", async () => {
  const doc = JSON.parse(
    readFileSync(
      new URL("../internal/profiledocument/fixtures.json", import.meta.url),
    ),
  )[0].document
  const repo = {
    ...withContactFields(oldProfile),
    fullName: "Ada Lovelace",
    currentSalary: "PRIVATE_SALARY",
  }
  const originalFetch = globalThis.fetch
  let sent
  const contextSelection = {
    version: "application-context-v1",
    sources: ["p/skill@1"],
    budgetExcluded: 1,
    relevanceExcluded: 0,
    complete: false,
    bytes: 100,
  }
  globalThis.fetch = async (_, options) => {
    sent = JSON.parse(options.body)
    return {
      ok: true,
      json: async () => ({
        jobTitle: null,
        company: null,
        jobSummary: "Role",
        resume: "Resume",
        coverLetter: {
          greeting: "Dear team,",
          body: "I build systems.",
          closing: "Sincerely,",
        },
        applicationAnswers: "Answers",
        contextSelection,
      }),
    }
  }
  try {
    const before = JSON.stringify(doc)
    const result = await generateMaterials(
      repo,
      "Java required.",
      "test-key",
      [],
      "en",
      "synthetic",
      undefined,
      [],
      doc,
      ["skill"],
    )
    assert.equal(sent.profileEvidence.facts[0].id, "skill")
    assert.deepEqual(sent.selectedFactIds,["skill"])
    assert.deepEqual(result.contextSelection, contextSelection)
    assert.ok(result.coverLetter.endsWith("Ada Lovelace"))
    assert.equal(JSON.stringify(doc), before)
    assert.ok(!JSON.stringify(sent).includes("Ada Lovelace"))
  } finally {
    globalThis.fetch = originalFetch
  }
})

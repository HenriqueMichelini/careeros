import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync, writeFileSync, mkdtempSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"

const temp = mkdtempSync(join(tmpdir(), "careeros-contact-"))
for (const name of ["profile", "ai"]) {
  const source = readFileSync(new URL(`../src/lib/${name}.ts`, import.meta.url), "utf8")
  const compiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText.replace(/from ['"]\.\/profile['"]/g, 'from "./profile.mjs"')
  writeFileSync(join(temp, `${name}.mjs`), compiled)
}
const { withContactFields, careerProfile, cvQualifications, validQualifications } = await import(join(temp, "profile.mjs"))
const { reviewRepository, findProfileGaps, generateMaterials } = await import(join(temp, "ai.mjs"))

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

test("the three AI requests exclude contact facts, and review preserves them", async () => {
  const repo = {
    ...withContactFields(oldProfile), fullName: "Ada Lovelace", email: "ada@example.test",
    phone: "+1 555 0100", location: "London", professionalLinks: "example.test/ada",
  }
  const originalFetch = globalThis.fetch
  const requests = []
  globalThis.fetch = async (url, options) => {
    requests.push({ url, body: JSON.parse(options.body) })
    if (url === "/api/profile/review") return {
      ok: true,
      json: async () => ({ updatedRepository: { ...oldProfile, skills: "Go, TypeScript" }, summary: "Updated skills" }),
    }
    if (url === "/api/qualification-gaps") return { ok: true, json: async () => ({ gaps: [] }) }
    return { ok: true, json: async () => ({
      jobTitle: "Engineer", company: "Acme", jobSummary: "Role", resume: "Resume",
      coverLetter: "Letter", applicationAnswers: "Answers",
    }) }
  }
  try {
    const reviewed = await reviewRepository(repo, "Skills", "test-key")
    assert.equal(reviewed.updatedRepo.skills, "Go, TypeScript")
    for (const key of ["fullName", "email", "phone", "location", "professionalLinks"]) {
      assert.equal(reviewed.updatedRepo[key], repo[key])
    }
    await findProfileGaps(repo, "Engineer", "test-key")
    await generateMaterials(repo, "Engineer", "test-key")
    assert.deepEqual(requests.map(request => request.url), [
      "/api/profile/review", "/api/qualification-gaps", "/api/application-draft",
    ])
    for (const request of requests) assert.deepEqual(request.body.repository, oldProfile)
    assert.equal("qualifications" in requests[0].body, false)
    assert.deepEqual(requests[1].body.qualifications, { education: [], certifications: [], languages: [] })
    assert.deepEqual(requests[2].body.qualifications, { education: [], certifications: [], languages: [] })
  } finally {
    globalThis.fetch = originalFetch
  }
})

test("structured qualifications survive review and reach only the relevant AI requests", async () => {
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
    if (url === "/api/profile/review") return { ok: true, json: async () => ({ updatedRepository: oldProfile, summary: "Reviewed" }) }
    if (url === "/api/qualification-gaps") return { ok: true, json: async () => ({ gaps: [] }) }
    return { ok: true, json: async () => ({ jobTitle: "Engineer", company: "Acme", jobSummary: "Role", resume: "Resume", coverLetter: "Letter", applicationAnswers: "Answers" }) }
  }
  try {
    const reviewed = await reviewRepository(repo, "Education", "test-key")
    assert.deepEqual(cvQualifications(reviewed.updatedRepo), cvQualifications(repo))
    await findProfileGaps(repo, "Engineer", "test-key")
    await generateMaterials(repo, "Engineer", "test-key")
    assert.equal("qualifications" in requests[0].body, false)
    assert.deepEqual(requests[1].body.qualifications, cvQualifications(repo))
    assert.deepEqual(requests[2].body.qualifications, cvQualifications(repo))
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

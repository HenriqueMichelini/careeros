import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"

const compiled = ts.transpileModule(
  readFileSync(new URL("../src/lib/ingestion.ts", import.meta.url), "utf8"),
  {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
      verbatimModuleSyntax: false,
    },
  },
).outputText
const output = join(tmpdir(), "careeros-ingestion-" + process.pid + ".mjs")
writeFileSync(output, compiled)
const { validateIngestionResult, applyIngestion, beforeValue, afterValue, previewValues, validProfile } =
  await import(output)

const profile = () => ({
  careerGoals: "",
  skills: "React",
  competencies: "",
  experience: [
    {
      id: "e1",
      company: "Acme",
      title: "Engineer",
      startDate: "",
      endDate: "",
      current: false,
      location: "",
      description: "",
      responsibilities: "",
      achievements: "Built search",
    },
  ],
  tools: "",
  projects: [
    {
      id: "p1",
      name: "Dashboard",
      description: "",
      technologies: "",
      url: "",
      highlights: "",
    },
  ],
  employmentStatus: "",
  currentSalary: "",
  desiredSalary: "",
  additionalInfo: "",
})
const op = (overrides = {}) => ({
  claimId: "c1",
  target: "skills",
  entryId: "",
  field: "skills",
  action: "add",
  value: "TypeScript",
  finding: "addition",
  approved: true,
  ...overrides,
})
const claim = (overrides = {}) => ({
  id: "c1",
  source: "TypeScript",
  text: "TypeScript",
  targets: ["skills"],
  question: "",
  ...overrides,
})
const review = (data) => ({ unverifiedClaimCount: 0, unresolvedClaimIds: [], unplacedOperationCount: 0, ...data })

test("maps one claim to linked sections and validates source and target", () => {
  const result = validateIngestionResult(
    review({
      claims: [claim({ targets: ["skills", "experience"] })],
      operations: [
        {
          claimId: "c1",
          target: "skills",
          entryId: "",
          field: "skills",
          action: "add",
          value: "TypeScript",
          finding: "addition",
        },
        {
          claimId: "c1",
          target: "experience",
          entryId: "e1",
          field: "achievements",
          action: "add",
          value: "Used TypeScript",
          finding: "in_place",
        },
      ],
    }),
    "TypeScript",
    profile(),
  )
  assert.equal(result.operations.length, 2)
  assert.ok(result.operations.every((o) => o.approved === false))
  assert.throws(() =>
    validateIngestionResult(
      review({ claims: [claim({ source: "invented" })], operations: [] }),
      "TypeScript",
      profile(),
    ),
  )
  assert.throws(() =>
    validateIngestionResult(
      review({
        claims: [claim()],
        operations: [
          { ...op(), target: "desiredSalary", field: "desiredSalary" },
        ],
      }),
      "TypeScript",
      profile(),
    ),
  )
})

test("partial review identifies withheld claims and cannot carry their operations", () => {
  const { approved: _approved, ...rawOp } = op()
  const partial = review({
    claims: [claim(), claim({ id: "c2", text: "Project", source: "Project", targets: ["projects"] })],
    operations: [rawOp],
    unverifiedClaimCount: 1,
    unresolvedClaimIds: ["c2"],
  })
  const result = validateIngestionResult(partial, "TypeScript Project", profile())
  assert.equal(result.unverifiedClaimCount, 1)
  assert.deepEqual(result.unresolvedClaimIds, ["c2"])
  assert.throws(() => validateIngestionResult({ ...partial, operations: [{ ...rawOp, claimId: "c2", target: "projects", entryId: "p1", field: "name" }] }, "TypeScript Project", profile()))
})

test("applies edited in-place facts while preserving IDs and unrelated fields", () => {
  const before = profile()
  const operations = [
    op({ value: "TypeScript at Acme" }),
    op({
      target: "experience",
      entryId: "e1",
      field: "achievements",
      value: "Improved search",
      finding: "in_place",
    }),
    op({
      target: "projects",
      entryId: "p1",
      field: "highlights",
      value: "Rejected",
      approved: false,
    }),
  ]
  const next = applyIngestion(before, JSON.stringify(before), operations)
  assert.equal(next.skills, "React\nTypeScript at Acme")
  assert.equal(next.experience[0].achievements, "Built search\nImproved search")
  assert.equal(next.experience[0].id, "e1")
  assert.equal(next.projects[0].highlights, "")
  assert.equal(before.skills, "React")
  assert.equal(afterValue(before, operations[0]), next.skills)
  assert.equal(beforeValue(before, operations[0]), "React")
})

test("repeat adds, explicit removal, and new IDs", () => {
  const before = profile()
  assert.equal(
    applyIngestion(before, JSON.stringify(before), [op({ value: "React" })])
      .skills,
    "React",
  )
  assert.equal(
    applyIngestion(before, JSON.stringify(before), [
      op({ action: "remove", value: "" }),
    ]).skills,
    "",
  )
  const operations = [
    op({
      target: "experience",
      entryId: "new:c1",
      field: "company",
      value: "Beta",
    }),
    op({
      target: "experience",
      entryId: "new:c1",
      field: "title",
      value: "Lead",
    }),
  ]
  const next = applyIngestion(before, JSON.stringify(before), operations)
  assert.equal(next.experience.length, 2)
  assert.notEqual(next.experience[1].id, "e1")
  assert.equal(next.experience[1].company, "Beta")
  assert.throws(
    () =>
      applyIngestion(before, JSON.stringify(before), operations.slice(0, 1)),
    /incomplete/,
  )
})

test("stale or invalid patches do not mutate the Profile", () => {
  const before = profile()
  assert.throws(
    () =>
      applyIngestion({ ...before, skills: "Changed" }, JSON.stringify(before), [
        op(),
      ]),
    /stale/,
  )
  assert.throws(() =>
    applyIngestion(before, JSON.stringify(before), [
      op({
        target: "employmentStatus",
        field: "employmentStatus",
        value: "unknown",
      }),
    ]),
  )
  assert.equal(before.skills, "React")
})

test("preview includes preceding approved changes and matches boolean apply", () => {
  const before=profile()
  const operations=[op({value:"TypeScript"}),op({value:"Go"})]
  assert.deepEqual(previewValues(before,operations,1),{before:"React\nTypeScript",after:"React\nTypeScript\nGo"})
  assert.equal(applyIngestion(before,JSON.stringify(before),operations).skills,previewValues(before,operations,1).after)
  const current=op({target:"experience",entryId:"e1",field:"current",value:"true"})
  assert.deepEqual(previewValues(before,[current],0),{before:"false",after:"true"})
  assert.equal(applyIngestion(before,JSON.stringify(before),[current]).experience[0].current,true)
  assert.throws(() => applyIngestion(before,JSON.stringify(before),[{...current,value:"maybe"}]))
  const removal={...current,action:"remove",value:""}
  assert.equal(previewValues(before,[removal],0).after,"false")
})

test("legacy saved employment status is preserved by Quick Add", () => {
  const before={...profile(),employmentStatus:"Employed — Full-time"}
  assert.equal(validProfile(before),true)
  assert.equal(applyIngestion(before,JSON.stringify(before),[op()]).employmentStatus,"Employed — Full-time")
})

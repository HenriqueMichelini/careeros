// Pure review/selection model evidence (#183). Transpiles the TypeScript module
// the same way the other node contracts do and exercises destination groups,
// dependency-safe selection, previews and batch/source identity.
import test from "node:test"
import assert from "node:assert/strict"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"

const temp = mkdtempSync(join(tmpdir(), "ingestion-review-"))
writeFileSync(
  join(temp, "contract.mjs"),
  `export default ${readFileSync(
    new URL("../internal/profiledocument/contract.json", import.meta.url),
    "utf8",
  )}`,
)
const rewrites = [
  ['"./profileDocument"', '"./profileDocument.mjs"'],
  ['"./ingestion"', '"./ingestion.mjs"'],
  ['"./ingestionProtocol"', '"./ingestionProtocol.mjs"'],
  ['"./profile"', '"./profile.mjs"'],
  ['"./fieldDecision"', '"./fieldDecision.mjs"'],
  ['"../../internal/profiledocument/contract.json"', '"./contract.mjs"'],
]
for (const name of [
  "profileDocument",
  "profile",
  "fieldDecision",
  "ingestion",
  "ingestionProtocol",
  "ingestionReview",
]) {
  let source = readFileSync(
    new URL(`../src/lib/${name}.ts`, import.meta.url),
    "utf8",
  )
  for (const [from, to] of rewrites) source = source.replaceAll(from, to)
  writeFileSync(
    join(temp, `${name}.mjs`),
    ts.transpileModule(source, {
      compilerOptions: {
        module: ts.ModuleKind.ESNext,
        target: ts.ScriptTarget.ES2022,
      },
    }).outputText,
  )
}
const { migrateProfile, profileView } = await import(
  join(temp, "profileDocument.mjs")
)
const protocol = await import(join(temp, "ingestionProtocol.mjs"))
const review = await import(join(temp, "ingestionReview.mjs"))

const experience = (id, overrides = {}) => ({
  id,
  company: "",
  title: "",
  startDate: "",
  endDate: "",
  current: false,
  location: "",
  description: "",
  responsibilities: "",
  achievements: "",
  ...overrides,
})
const project = (id, overrides = {}) => ({
  id,
  name: "",
  description: "",
  technologies: "",
  url: "",
  highlights: "",
  ...overrides,
})
const legacy = (overrides = {}) => ({
  fullName: "",
  email: "",
  phone: "",
  location: "",
  professionalLinks: "",
  careerGoals: "",
  skills: "",
  competencies: "",
  tools: "",
  employmentStatus: "",
  currentSalary: "",
  desiredSalary: "",
  additionalInfo: "",
  experience: [],
  projects: [],
  education: [],
  certifications: [],
  languages: [],
  ...overrides,
})
const snapshotOf = (overrides = {}) =>
  migrateProfile(legacy(overrides), "profile")

const claim = (id, overrides = {}) => ({
  id,
  source: id,
  text: id,
  targets: [],
  question: "",
  ...overrides,
})
const operation = (claimId, overrides = {}) => ({
  claimId,
  target: "skills",
  entryId: "",
  field: "skills",
  action: "add",
  value: "Value",
  finding: "addition",
  approved: false,
  ...overrides,
})
const submission = (overrides = {}) => ({
  protocolVersion: "profile-submission-v1",
  submissionId: "sub-1",
  runId: "run",
  sourceId: "source-1",
  profileId: "profile",
  planVersion: "portions-v1",
  batches: [],
  claims: [],
  operations: [],
  outcomes: [],
  unresolvedClaimIds: [],
  ...overrides,
})
const build = (sub, snapshot) =>
  review.buildReviewPlan({ submission: sub, snapshot })
const unitFor = (plan, predicate) => plan.units.find(predicate)
const fieldsOf = (plan) =>
  Object.fromEntries(
    review
      .reviewPreview(plan)
      .fields.map((field) => [`${field.target}.${field.field}`, field]),
  )
const toggle = (plan, unitId) =>
  review.selectReviewUnit(plan, unitId, "selected")

test("review starts with no selection and exposes destination, identity, value and material status", () => {
  const snapshot = snapshotOf({
    skills: "React",
    experience: [
      experience("e1", {
        company: "Acme",
        title: "Engineer",
        startDate: "2018-01",
        endDate: "2020-12",
      }),
    ],
    projects: [project("p1", { name: "Harbor" })],
  })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1", "run/0/c2", "run/0/c3"] }],
    claims: [
      claim("run/0/c1", { targets: ["skills"] }),
      claim("run/0/c2", { targets: ["experience"] }),
      claim("run/0/c3", { targets: ["experience"] }),
    ],
    operations: [
      operation("run/0/c1", { value: "TypeScript" }),
      operation("run/0/c2", {
        target: "experience",
        entryId: "e1",
        field: "description",
        value: "Backend engineer.",
      }),
      // A new entry whose anchor operation carries company and title.
      operation("run/0/c3", {
        target: "experience",
        entryId: "new:run/0/c3",
        field: "company",
        value: "Aster Labs",
      }),
      operation("run/0/c3", {
        target: "experience",
        entryId: "new:run/0/c3",
        field: "title",
        value: "Staff Engineer",
      }),
    ],
    outcomes: [
      {
        claimId: "run/0/c1",
        kind: "change",
        reason: "r",
        relatedFacts: [],
        relatedClaimIds: [],
        operationIndexes: [0],
      },
      {
        claimId: "run/0/c2",
        kind: "change",
        reason: "r",
        relatedFacts: [],
        relatedClaimIds: [],
        operationIndexes: [1],
      },
      {
        claimId: "run/0/c3",
        kind: "change",
        reason: "r",
        relatedFacts: [],
        relatedClaimIds: [],
        operationIndexes: [2, 3],
      },
    ],
  })
  const plan = build(sub, snapshot)
  assert.equal(plan.protocolVersion, protocol.submissionProtocolVersion)
  assert.equal(review.reviewPlanError(plan), "")
  assert.deepEqual(
    plan.destinations.map((destination) => destination.id),
    ["skills#@profile", "experience#e1", "experience#new:run/0/c3"],
  )
  assert.ok(plan.units.every((unit) => unit.status === "pending"))
  assert.ok(plan.units.every((unit) => unit.sourceId === "source-1"))
  assert.ok(plan.units.every((unit) => unit.batchIndexes.join() === "0"))

  const existing = review.reviewDestination(plan, "experience#e1")
  assert.deepEqual(existing.identity, [
    { field: "company", value: "Acme" },
    { field: "title", value: "Engineer" },
    { field: "startDate", value: "2018-01" },
    { field: "endDate", value: "2020-12" },
  ])
  const fresh = review.reviewDestination(plan, "experience#new:run/0/c3")
  assert.deepEqual(fresh.identity, [
    { field: "company", value: "Aster Labs" },
    { field: "title", value: "Staff Engineer" },
  ])
  const skill = unitFor(
    plan,
    (unit) => unit.destinationId === "skills#@profile",
  )
  assert.equal(skill.kind, "addition")
  assert.deepEqual(skill.values, [
    { field: "skills", action: "add", before: "React", proposed: "TypeScript" },
  ])
  assert.equal(skill.material, true, "appending to saved content is material")
  const description = unitFor(
    plan,
    (unit) => unit.destinationId === "experience#e1",
  )
  assert.equal(
    description.material,
    false,
    "filling an empty field is additive",
  )
  const identity = unitFor(plan, (unit) => unit.required)
  assert.equal(identity.kind, "addition")
  assert.equal(identity.entryKind, "new")
  assert.equal(identity.material, true, "a new entry is material")
})

test("an incomplete new entry is never selectable and blocks its dependent details", () => {
  const snapshot = snapshotOf({ experience: [] })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1", "run/0/c2"] }],
    claims: [
      claim("run/0/c1", { targets: ["experience"] }),
      claim("run/0/c2", { targets: ["experience"] }),
    ],
    operations: [
      operation("run/0/c1", {
        target: "experience",
        entryId: "new:run/0/c1",
        field: "company",
        value: "Aster Labs",
      }),
      operation("run/0/c2", {
        target: "experience",
        entryId: "new:run/0/c1",
        field: "description",
        value: "Backend engineer.",
      }),
    ],
  })
  const plan = build(sub, snapshot)
  const identity = unitFor(plan, (unit) => unit.required)
  assert.equal(identity.blockedReason, "incomplete_identity")
  assert.equal(identity.selectable, false)
  const detail = unitFor(plan, (unit) => unit.field === "description")
  assert.equal(detail.blockedReason, "dependency_blocked")
  assert.equal(detail.selectable, false)
  assert.throws(() => toggle(plan, identity.id), { code: "unselectable" })
  assert.equal(review.reviewGroupState(plan, "experience#new:run/0/c1"), "none")
  assert.equal(review.reviewSelectionError(plan), "")
})

test("cascades same-destination identity dependencies and clears dependents", () => {
  const snapshot = snapshotOf({ experience: [] })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1", "run/0/c2"] }],
    claims: [
      claim("run/0/c1", { targets: ["experience"] }),
      claim("run/0/c2", { targets: ["experience"] }),
    ],
    operations: [
      operation("run/0/c1", {
        target: "experience",
        entryId: "new:run/0/c1",
        field: "company",
        value: "Aster Labs",
      }),
      operation("run/0/c1", {
        target: "experience",
        entryId: "new:run/0/c1",
        field: "title",
        value: "Staff Engineer",
      }),
      operation("run/0/c2", {
        target: "experience",
        entryId: "new:run/0/c1",
        field: "description",
        value: "Backend engineer.",
      }),
    ],
  })
  const plan = build(sub, snapshot)
  const identity = unitFor(plan, (unit) => unit.required)
  const detail = unitFor(plan, (unit) => unit.field === "description")
  assert.deepEqual(detail.dependencies, [identity.id])
  assert.deepEqual(identity.dependents, [detail.id])

  const selected = toggle(plan, detail.id)
  assert.equal(review.reviewUnit(selected, detail.id).status, "selected")
  assert.equal(
    review.reviewUnit(selected, identity.id).status,
    "selected",
    "the coherent identity unit is cascaded",
  )
  assert.equal(review.reviewSelectionError(selected), "")
  assert.equal(review.reviewGroupState(selected, identity.destinationId), "all")

  const cleared = review.selectReviewUnit(selected, identity.id, "pending")
  assert.equal(review.reviewUnit(cleared, detail.id).status, "pending")
  assert.equal(review.reviewUnit(cleared, identity.id).status, "pending")
})

test("support dependencies stay visible across destinations and never borrow another entry's support", () => {
  const snapshot = snapshotOf({
    skills: "React",
    experience: [experience("e1", { company: "Acme", title: "Engineer" })],
  })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/sup", "run/0/c2", "run/0/c3"] }],
    claims: [
      claim("run/0/sup", { targets: ["skills"] }),
      claim("run/0/c2", { targets: ["experience"] }),
      claim("run/0/c3", { targets: ["experience"] }),
    ],
    operations: [
      operation("run/0/sup", { value: "Go" }),
      operation("run/0/c2", {
        target: "experience",
        entryId: "e1",
        field: "description",
        value: "Backend engineer.",
        supportingClaimIds: ["run/0/c2", "run/0/sup"],
      }),
      operation("run/0/c3", {
        target: "experience",
        entryId: "e1",
        field: "achievements",
        value: "Cut latency.",
      }),
      operation("run/0/c3", {
        target: "experience",
        entryId: "e1",
        field: "responsibilities",
        value: "Owned payments.",
      }),
    ],
  })
  const plan = build(sub, snapshot)
  const support = unitFor(
    plan,
    (unit) => unit.destinationId === "skills#@profile",
  )
  const dependent = unitFor(plan, (unit) => unit.field === "description")
  assert.deepEqual(dependent.dependencies, [support.id])
  assert.equal(support.destinationId === dependent.destinationId, false)

  // Same-destination ops of one claim are coherent: an explicit self-claim
  // support never creates a cycle.
  const sibling = unitFor(plan, (unit) => unit.field === "achievements")
  assert.deepEqual(sibling.dependencies, [])

  const partial = toggle(plan, dependent.id)
  assert.equal(review.reviewUnit(partial, support.id).status, "pending")
  assert.equal(review.reviewSelectionError(partial), "selection_closure")
  assert.deepEqual(review.reviewMissingDependencies(partial), [
    { unitId: dependent.id, unitIds: [support.id] },
  ])

  const complete = toggle(partial, support.id)
  assert.equal(review.reviewSelectionError(complete), "")
  assert.equal(review.reviewPreview(complete).error, "")

  // A claim that serves two entries of one target borrows support.
  const borrow = submission({
    batches: [{ index: 0, claimIds: ["run/0/c4"] }],
    claims: [claim("run/0/c4", { targets: ["experience"] })],
    operations: [
      operation("run/0/c4", {
        target: "experience",
        entryId: "b1",
        field: "description",
        value: "first",
      }),
      operation("run/0/c4", {
        target: "experience",
        entryId: "b2",
        field: "description",
        value: "second",
      }),
    ],
  })
  const snapshot2 = snapshotOf({
    experience: [
      experience("b1", { company: "A", title: "T" }),
      experience("b2", { company: "B", title: "T" }),
    ],
  })
  const plan2 = build(borrow, snapshot2)
  assert.ok(
    plan2.units.every(
      (unit) => unit.blockedReason === "shared_support" && !unit.selectable,
    ),
  )
})

test("repeated local ids across batches keep distinct global identity and batch membership", () => {
  const snapshot = snapshotOf({ experience: [] })
  const sub = submission({
    batches: [
      { index: 0, claimIds: ["run/0/c1"] },
      { index: 1, claimIds: ["run/1/c1"] },
    ],
    claims: [
      claim("run/0/c1", { targets: ["experience"] }),
      claim("run/1/c1", { targets: ["experience"] }),
    ],
    operations: [
      operation("run/0/c1", {
        target: "experience",
        entryId: "new:run/0/c1",
        field: "company",
        value: "Aster Labs",
      }),
      operation("run/0/c1", {
        target: "experience",
        entryId: "new:run/0/c1",
        field: "title",
        value: "Engineer",
      }),
      operation("run/1/c1", {
        target: "experience",
        entryId: "new:run/1/c1",
        field: "company",
        value: "Borealis",
      }),
      operation("run/1/c1", {
        target: "experience",
        entryId: "new:run/1/c1",
        field: "title",
        value: "Engineer",
      }),
    ],
  })
  const plan = build(sub, snapshot)
  assert.deepEqual(
    plan.destinations.map((destination) => destination.entryId).sort(),
    ["new:run/0/c1", "new:run/1/c1"],
  )
  const first = unitFor(plan, (unit) => unit.batchIndexes.join() === "0")
  const second = unitFor(plan, (unit) => unit.batchIndexes.join() === "1")
  assert.notEqual(first.id, second.id)
  assert.deepEqual(first.claimIds, ["run/0/c1"])
  assert.deepEqual(second.claimIds, ["run/1/c1"])
  assert.equal(plan.units.length, 2)
})

test("same-name entries with different periods stay distinct destinations", () => {
  const snapshot = snapshotOf({
    experience: [
      experience("e1", {
        company: "Acme",
        title: "Engineer",
        startDate: "2018-01",
        endDate: "2020-12",
      }),
      experience("e2", {
        company: "Acme",
        title: "Engineer",
        startDate: "2024-01",
        endDate: "2025-12",
      }),
    ],
  })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1", "run/0/c2", "run/0/c3"] }],
    claims: [
      claim("run/0/c1", { targets: ["experience"] }),
      claim("run/0/c2", { targets: ["experience"] }),
      claim("run/0/c3", { targets: ["experience"] }),
    ],
    operations: [
      operation("run/0/c1", {
        target: "experience",
        entryId: "e1",
        field: "description",
        value: "first",
      }),
      operation("run/0/c2", {
        target: "experience",
        entryId: "e2",
        field: "description",
        value: "second",
      }),
      operation("run/0/c3", {
        target: "experience",
        entryId: "new:run/0/c3",
        field: "company",
        value: "Acme",
      }),
      operation("run/0/c3", {
        target: "experience",
        entryId: "new:run/0/c3",
        field: "title",
        value: "Engineer",
      }),
      operation("run/0/c3", {
        target: "experience",
        entryId: "new:run/0/c3",
        field: "startDate",
        value: "2021-01",
      }),
      operation("run/0/c3", {
        target: "experience",
        entryId: "new:run/0/c3",
        field: "endDate",
        value: "2021-12",
      }),
    ],
  })
  const plan = build(sub, snapshot)
  const first = review.reviewDestination(plan, "experience#e1")
  const second = review.reviewDestination(plan, "experience#e2")
  const fresh = review.reviewDestination(plan, "experience#new:run/0/c3")
  assert.deepEqual(
    first.identity.map((identity) => identity.value),
    ["Acme", "Engineer", "2018-01", "2020-12"],
  )
  assert.deepEqual(
    second.identity.map((identity) => identity.value),
    ["Acme", "Engineer", "2024-01", "2025-12"],
  )
  assert.deepEqual(
    fresh.identity.map((identity) => identity.value),
    ["Acme", "Engineer", "2021-01", "2021-12"],
  )
  assert.equal(plan.destinations.length, 3)
  assert.equal(first.unitIds.length, 1)
  assert.equal(second.unitIds.length, 1)
})

test("mixed-field claims map to one destination per section without collapsing identity", () => {
  const snapshot = snapshotOf({
    skills: "React",
    experience: [experience("e1", { company: "Acme", title: "Engineer" })],
    projects: [project("p1", { name: "Harbor" })],
  })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [
      claim("run/0/c1", { targets: ["skills", "experience", "projects"] }),
    ],
    operations: [
      operation("run/0/c1", { value: "TypeScript" }),
      operation("run/0/c1", {
        target: "experience",
        entryId: "e1",
        field: "responsibilities",
        value: "Built search.",
      }),
      operation("run/0/c1", {
        target: "projects",
        entryId: "p1",
        field: "highlights",
        value: "Added audit trails.",
      }),
    ],
  })
  const plan = build(sub, snapshot)
  assert.equal(plan.destinations.length, 3)
  assert.ok(plan.units.every((unit) => unit.claimIds.join() === "run/0/c1"))
  assert.deepEqual(
    plan.destinations.map((destination) => destination.target),
    ["skills", "experience", "projects"],
  )
  assert.ok(
    plan.destinations.every((destination) => destination.unitIds.length === 1),
  )
})

test("selection toggles, tri-state group selection and scoped additions", () => {
  const snapshot = snapshotOf({ skills: "React" })
  const sub = submission({
    batches: [
      { index: 0, claimIds: ["run/0/c1", "run/0/c2", "run/0/c3", "run/0/c4"] },
    ],
    claims: [
      claim("run/0/c1", { targets: ["skills"] }),
      claim("run/0/c2", { targets: ["skills"] }),
      claim("run/0/c3", { targets: ["skills"] }),
      claim("run/0/c4", { targets: ["skills"] }),
    ],
    operations: [
      operation("run/0/c1", { value: "TypeScript" }),
      operation("run/0/c2", { value: "Go" }),
      operation("run/0/c3", { action: "update", value: "Rust" }),
      operation("run/0/c4", { action: "evidence", value: "React" }),
    ],
  })
  const plan = build(sub, snapshot)
  const destination = "skills#@profile"
  assert.equal(review.reviewGroupState(plan, destination), "none")
  assert.deepEqual(review.reviewPreview(plan).fields, [])
  assert.equal(review.reviewSelectionError(plan), "")

  const first = toggle(
    plan,
    unitFor(plan, (unit) => unit.claimIds[0] === "run/0/c1").id,
  )
  assert.equal(review.reviewGroupState(first, destination), "partial")

  const all = review.selectReviewAdditions(first)
  assert.equal(review.reviewGroupState(all, destination), "partial")
  assert.equal(
    review.reviewUnit(
      all,
      unitFor(plan, (unit) => unit.kind === "replacement").id,
    ).status,
    "pending",
    "bulk additions exclude replacements",
  )
  assert.equal(
    review.reviewUnit(all, unitFor(plan, (unit) => unit.kind === "evidence").id)
      .status,
    "pending",
    "bulk additions exclude evidence-only changes",
  )
  assert.equal(
    review.reviewUnit(
      all,
      unitFor(plan, (unit) => unit.claimIds[0] === "run/0/c2").id,
    ).status,
    "selected",
    "bulk additions include clear additions",
  )

  const group = review.selectReviewGroup(all, destination, "selected")
  assert.equal(review.reviewGroupState(group, destination), "all")

  const cleared = review.selectReviewGroup(group, destination, "pending")
  assert.equal(review.reviewGroupState(cleared, destination), "none")
  assert.equal(
    review.reviewUnit(
      cleared,
      unitFor(plan, (unit) => unit.claimIds[0] === "run/0/c1").id,
    ).status,
    "pending",
  )
})

test("bulk additions exclude removals, replacements, evidence and control units", () => {
  const snapshot = snapshotOf({ skills: "React" })
  const sub = submission({
    batches: [
      {
        index: 0,
        claimIds: ["run/0/c1", "run/0/c2", "run/0/c3", "run/0/c4", "run/0/c5"],
      },
    ],
    claims: [
      claim("run/0/c1", { targets: ["skills"] }),
      claim("run/0/c2", { targets: ["skills"] }),
      claim("run/0/c3", { targets: ["skills"] }),
      claim("run/0/c4", { targets: ["skills"] }),
      claim("run/0/c5", { targets: ["skills"] }),
    ],
    operations: [
      operation("run/0/c1", { value: "TypeScript" }),
      operation("run/0/c2", { action: "update", value: "Rust" }),
      operation("run/0/c3", { action: "remove", value: "" }),
      operation("run/0/c4", { action: "evidence", value: "React" }),
    ],
    outcomes: [
      {
        claimId: "run/0/c1",
        kind: "change",
        reason: "r",
        relatedFacts: [],
        relatedClaimIds: [],
        operationIndexes: [0],
      },
      {
        claimId: "run/0/c2",
        kind: "change",
        reason: "r",
        relatedFacts: [],
        relatedClaimIds: [],
        operationIndexes: [1],
      },
      {
        claimId: "run/0/c3",
        kind: "change",
        reason: "r",
        relatedFacts: [],
        relatedClaimIds: [],
        operationIndexes: [2],
      },
      {
        claimId: "run/0/c4",
        kind: "additional_support",
        reason: "r",
        relatedFacts: [],
        relatedClaimIds: [],
        operationIndexes: [3],
      },
      {
        claimId: "run/0/c5",
        kind: "contradiction",
        reason: "r",
        relatedFacts: [],
        relatedClaimIds: [],
      },
    ],
  })
  const plan = build(sub, snapshot)
  const conflict = unitFor(plan, (unit) => unit.kind === "conflict")
  assert.equal(conflict.selectable, false)
  assert.equal(conflict.blockedReason, "conflict")

  const additions = review.selectReviewAdditions(plan)
  const status = (kind) =>
    review.reviewUnit(additions, unitFor(plan, (unit) => unit.kind === kind).id)
      .status
  assert.equal(status("addition"), "selected")
  assert.equal(status("replacement"), "pending")
  assert.equal(status("removal"), "pending")
  assert.equal(status("evidence"), "pending")
  assert.equal(review.reviewUnit(additions, conflict.id).status, "pending")
})

test("computes exact append, replace and remove previews from the selection and pinned snapshot", () => {
  const snapshot = snapshotOf({
    skills: "React",
    experience: [
      experience("e1", {
        company: "Acme",
        title: "Engineer",
        location: "London",
        achievements: "Built search",
      }),
    ],
  })
  const sub = submission({
    batches: [
      {
        index: 0,
        claimIds: ["run/0/c1", "run/0/c2", "run/0/c3", "run/0/c4", "run/0/c5"],
      },
    ],
    claims: [
      claim("run/0/c1", { targets: ["skills"] }),
      claim("run/0/c2", { targets: ["experience"] }),
      claim("run/0/c3", { targets: ["experience"] }),
      claim("run/0/c4", { targets: ["experience"] }),
      claim("run/0/c5", { targets: ["skills"] }),
    ],
    operations: [
      operation("run/0/c1", { value: "TypeScript" }),
      operation("run/0/c2", {
        target: "experience",
        entryId: "e1",
        field: "description",
        action: "update",
        value: "Backend engineer.",
      }),
      operation("run/0/c3", {
        target: "experience",
        entryId: "e1",
        field: "achievements",
        value: "Improved search",
      }),
      operation("run/0/c4", {
        target: "experience",
        entryId: "e1",
        field: "location",
        action: "remove",
        value: "",
      }),
      operation("run/0/c5", { value: "Go" }),
    ],
  })
  const plan = build(sub, snapshot)
  const select = [
    "run/0/c1",
    "run/0/c2",
    "run/0/c3",
    "run/0/c4",
    "run/0/c5",
  ].reduce(
    (current, claimId) =>
      toggle(
        current,
        unitFor(current, (unit) => unit.claimIds[0] === claimId).id,
      ),
    plan,
  )
  const fields = fieldsOf(select)
  assert.deepEqual(fields["skills.skills"], {
    target: "skills",
    entryId: "",
    field: "skills",
    action: "add",
    change: "append",
    before: "React",
    after: "React\nTypeScript\nGo",
    unitIds: [
      unitFor(select, (unit) => unit.claimIds[0] === "run/0/c1").id,
      unitFor(select, (unit) => unit.claimIds[0] === "run/0/c5").id,
    ],
  })
  assert.equal(fields["experience.description"].change, "replace")
  assert.equal(fields["experience.description"].after, "Backend engineer.")
  assert.equal(fields["experience.achievements"].change, "append")
  assert.equal(
    fields["experience.achievements"].after,
    "Built search\nImproved search",
  )
  assert.equal(fields["experience.location"].change, "remove")
  assert.equal(fields["experience.location"].before, "London")
  assert.equal(fields["experience.location"].after, "")

  const preview = review.reviewPreview(select)
  assert.equal(preview.error, "")
  assert.equal(preview.complete, true)
  assert.equal(preview.selectedOperationCount, 5)
  assert.equal(preview.view.skills, "React\nTypeScript\nGo")
  assert.equal(preview.view.experience[0].description, "Backend engineer.")
  assert.equal(
    preview.view.experience[0].achievements,
    "Built search\nImproved search",
  )
  assert.equal(preview.view.experience[0].location, "")
  assert.deepEqual(
    review
      .selectedReviewOperations(select)
      .map((operation) => [operation.claimId, operation.approved]),
    [
      ["run/0/c1", true],
      ["run/0/c2", true],
      ["run/0/c3", true],
      ["run/0/c4", true],
      ["run/0/c5", true],
    ],
  )
})

test("an unsupported selection is blocked before Save and pending entries are never saved", () => {
  const snapshot = snapshotOf({ skills: "React" })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [claim("run/0/c1", { targets: ["skills"] })],
    operations: [
      operation("run/0/c1", { value: "TypeScript", approved: true }),
    ],
  })
  const plan = build(sub, snapshot)
  assert.ok(
    plan.units.every((unit) => unit.status === "pending"),
    "a pre-approved input operation is not treated as a saved fact",
  )
  const selected = toggle(plan, plan.units[0].id)
  assert.equal(review.reviewSelectionError(selected), "")
  assert.equal(review.reviewPreview(selected).complete, true)

  // A new entry selected without its identity unit is structurally impossible,
  // but a direct status edit is still caught before Save.
  const corrupted = {
    ...selected,
    units: selected.units.map((unit) => ({ ...unit, status: "pending" })),
  }
  const detailPlan = build(
    submission({
      batches: [{ index: 0, claimIds: ["run/0/c1", "run/0/c2"] }],
      claims: [
        claim("run/0/c1", { targets: ["experience"] }),
        claim("run/0/c2", { targets: ["experience"] }),
      ],
      operations: [
        operation("run/0/c1", {
          target: "experience",
          entryId: "new:run/0/c1",
          field: "company",
          value: "Aster Labs",
        }),
        operation("run/0/c1", {
          target: "experience",
          entryId: "new:run/0/c1",
          field: "title",
          value: "Engineer",
        }),
        operation("run/0/c2", {
          target: "experience",
          entryId: "new:run/0/c1",
          field: "description",
          value: "Backend engineer.",
        }),
      ],
    }),
    snapshotOf({ experience: [] }),
  )
  const forged = {
    ...detailPlan,
    units: detailPlan.units.map((unit) =>
      unit.required
        ? { ...unit, status: "pending" }
        : { ...unit, status: "selected" },
    ),
  }
  assert.equal(review.reviewSelectionError(forged), "selection_closure")
  assert.equal(review.reviewPreview(forged).complete, false)
})

test("never resolves conflicts, aliases wording or invents optional decisions", () => {
  const snapshot = snapshotOf({ skills: "React", experience: [] })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1", "run/0/c2", "run/0/c3"] }],
    claims: [
      claim("run/0/c1", {
        targets: ["skills"],
        text: "Javascript",
        meaning: {
          assertion: "affirmed",
          intent: "actual",
          certainty: "certain",
          temporal: { wording: "", precision: "unknown" },
        },
      }),
      claim("run/0/c2", {
        targets: ["skills"],
        text: "JavaScript",
        meaning: {
          assertion: "affirmed",
          intent: "aspiration",
          certainty: "uncertain",
          temporal: { wording: "", precision: "unknown" },
        },
      }),
      claim("run/0/c3", { targets: ["skills"], question: "Who used it?" }),
    ],
    operations: [
      operation("run/0/c1", { value: "Javascript" }),
      operation("run/0/c2", { value: "JavaScript" }),
    ],
    outcomes: [
      {
        claimId: "run/0/c1",
        kind: "change",
        reason: "r",
        relatedFacts: [],
        relatedClaimIds: [],
        operationIndexes: [0],
      },
      {
        claimId: "run/0/c2",
        kind: "change",
        reason: "r",
        relatedFacts: [],
        relatedClaimIds: [],
        operationIndexes: [1],
      },
      {
        claimId: "run/0/c3",
        kind: "clarification",
        reason: "source_ambiguity",
        relatedFacts: [],
        relatedClaimIds: [],
      },
    ],
  })
  const plan = build(sub, snapshot)
  const additions = plan.units.filter((unit) => unit.kind === "addition")
  assert.equal(additions.length, 2, "aliases are never merged")
  assert.ok(
    additions.every(
      (unit) =>
        unit.claimIds.length === 1 && unit.values[0].proposed !== undefined,
    ),
  )
  // Meaning stays presentation data: never used for grouping or selection.
  assert.equal(additions[0].meaning.intent, "actual")
  assert.equal(additions[1].meaning.intent, "aspiration")
  const clarification = unitFor(plan, (unit) => unit.kind === "clarification")
  assert.equal(clarification.selectable, false)
  assert.equal(clarification.operationIndexes.length, 0)
  assert.ok(
    plan.units.every(
      (unit) => !unit.claimIds.includes("run/0/c3") || unit === clarification,
    ),
  )

  // An unknown optional value produces no decision.
  const newEntry = submission({
    batches: [{ index: 0, claimIds: ["run/0/c4"] }],
    claims: [claim("run/0/c4", { targets: ["experience"] })],
    operations: [
      operation("run/0/c4", {
        target: "experience",
        entryId: "new:run/0/c4",
        field: "company",
        value: "Aster Labs",
      }),
      operation("run/0/c4", {
        target: "experience",
        entryId: "new:run/0/c4",
        field: "title",
        value: "Engineer",
      }),
    ],
  })
  const newPlan = build(newEntry, snapshot)
  assert.equal(newPlan.destinations.length, 1)
  assert.equal(newPlan.units.length, 1)
  assert.equal(newPlan.units[0].required, true)
})

test("scales past one bounded batch while retaining batch and source identity", () => {
  const batches = []
  const claims = []
  const operations = []
  const counts = [12, 12, 10]
  counts.forEach((count, index) => {
    const ids = []
    for (let local = 0; local < count; local++) {
      const id = `run/${index}/c${local}`
      ids.push(id)
      claims.push(claim(id, { targets: ["skills", "additionalInfo"] }))
      operations.push(operation(id, { value: `${id}-skills` }))
      operations.push(
        operation(id, {
          target: "additionalInfo",
          field: "additionalInfo",
          value: `${id}-note`,
        }),
      )
    }
    batches.push({ index, claimIds: ids })
  })
  const sub = submission({ batches, claims, operations })
  const plan = build(sub, snapshotOf({ skills: "React" }))
  assert.equal(plan.units.length, 68)
  assert.ok(plan.units.length > 60)
  assert.ok(sub.claims.length > 30)
  assert.equal(review.reviewPlanError(plan), "")
  assert.ok(plan.units.every((unit) => unit.status === "pending"))
  assert.ok(plan.units.every((unit) => unit.sourceId === "source-1"))
  assert.deepEqual(
    [...new Set(plan.units.flatMap((unit) => unit.batchIndexes))].sort(),
    [0, 1, 2],
  )
  assert.ok(
    plan.units.some((unit) => unit.batchIndexes.join() === "2"),
    "batch identity is not flattened away",
  )
  const session = {
    protocolVersion: protocol.submissionProtocolVersion,
    submissionId: "sub-1",
    status: "reviewing",
    totalBatches: 3,
    placedBatches: [0, 1, 2],
    selections: review.toSubmissionSelections(plan),
  }
  assert.equal(protocol.submissionSessionError(session), "")
  assert.equal(session.selections.length, 68)
})

test("exact duplicates, unsupported and unresolved outcomes stay distinct from changes", () => {
  const snapshot = snapshotOf({ skills: "React" })
  const sub = submission({
    batches: [{ index: 1, claimIds: ["run/1/d", "run/1/u", "run/1/x"] }],
    claims: [
      claim("run/1/d", { targets: ["skills"] }),
      claim("run/1/u", { targets: ["skills"] }),
      claim("run/1/x", { targets: ["skills"] }),
    ],
    outcomes: [
      {
        claimId: "run/1/d",
        kind: "exact_duplicate",
        reason: "duplicate",
        relatedFacts: [{ profileId: "profile", id: "f1", revision: 1 }],
        relatedClaimIds: [],
      },
      {
        claimId: "run/1/u",
        kind: "unsupported",
        reason: "no_source",
        relatedFacts: [],
        relatedClaimIds: [],
      },
    ],
    unresolvedClaimIds: ["run/1/x"],
  })
  const plan = build(sub, snapshot)
  assert.deepEqual(plan.units.map((unit) => unit.kind).sort(), [
    "duplicate",
    "unresolved",
    "unsupported",
  ])
  assert.ok(plan.units.every((unit) => unit.selectable === false))
  assert.ok(plan.units.every((unit) => unit.operationIndexes.length === 0))
  assert.equal(review.reviewSelectionError(plan), "")
  assert.ok(
    review
      .selectReviewAdditions(plan)
      .units.every((unit) => unit.status === "pending"),
  )
})

test("unit ids and dependency edges are deterministic across identical inputs", () => {
  const make = () => {
    const snapshot = snapshotOf({ experience: [] })
    const sub = submission({
      batches: [{ index: 0, claimIds: ["run/0/c1", "run/0/c2"] }],
      claims: [
        claim("run/0/c1", { targets: ["experience"] }),
        claim("run/0/c2", { targets: ["experience"] }),
      ],
      operations: [
        operation("run/0/c1", {
          target: "experience",
          entryId: "new:run/0/c1",
          field: "company",
          value: "Aster Labs",
        }),
        operation("run/0/c1", {
          target: "experience",
          entryId: "new:run/0/c1",
          field: "title",
          value: "Engineer",
        }),
        operation("run/0/c2", {
          target: "experience",
          entryId: "new:run/0/c1",
          field: "description",
          value: "Backend engineer.",
        }),
      ],
    })
    return build(sub, snapshot)
  }
  const first = make()
  const second = make()
  assert.deepEqual(
    first.units.map((unit) => unit.id),
    second.units.map((unit) => unit.id),
  )
  assert.deepEqual(
    first.units.map((unit) => unit.dependencies),
    second.units.map((unit) => unit.dependencies),
  )
  assert.deepEqual(
    first.destinations.map((destination) => destination.id),
    second.destinations.map((destination) => destination.id),
  )
})

test("the plan never mutates its pinned snapshot or submission", () => {
  const snapshot = snapshotOf({ skills: "React" })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [claim("run/0/c1", { targets: ["skills"] })],
    operations: [operation("run/0/c1", { value: "TypeScript" })],
  })
  const before = JSON.stringify({ snapshot, sub })
  const plan = build(sub, snapshot)
  toggle(plan, plan.units[0].id)
  review.reviewPreview(plan)
  assert.equal(JSON.stringify({ snapshot, sub }), before)
  assert.equal(plan.snapshot, snapshot)
  assert.deepEqual(profileView(snapshot).skills, "React")
})

test("rejects inputs that flatten batch identity or violate the versioned contract", () => {
  const snapshot = snapshotOf({ skills: "React" })
  const claimWithoutBatch = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [
      claim("run/0/c1", { targets: ["skills"] }),
      claim("run/0/c2", { targets: ["skills"] }),
    ],
    operations: [operation("run/0/c1", { value: "TypeScript" })],
  })
  assert.throws(() => build(claimWithoutBatch, snapshot), {
    code: "manifest_incomplete",
  })
  const wrongNamespace = submission({
    batches: [{ index: 0, claimIds: ["run/1/c1"] }],
    claims: [claim("run/1/c1", { targets: ["skills"] })],
    operations: [operation("run/1/c1", { value: "TypeScript" })],
  })
  assert.throws(() => build(wrongNamespace, snapshot), {
    code: "manifest_incomplete",
  })
  const wrongVersion = submission({
    protocolVersion: "profile-submission-v2",
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [claim("run/0/c1", { targets: ["skills"] })],
    operations: [operation("run/0/c1", { value: "TypeScript" })],
  })
  assert.throws(() => build(wrongVersion, snapshot), { code: "version" })
  const unknownOperation = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [claim("run/0/c1", { targets: ["skills"] })],
    operations: [operation("run/0/missing", { value: "TypeScript" })],
  })
  assert.throws(() => build(unknownOperation, snapshot), { code: "input" })
})

// --- Compact review presentation and focused edit (#190) -------------------

test("summary distinguishes pending changes, attention and already represented information", () => {
  const snapshot = snapshotOf({ skills: "React" })
  const fact = snapshot.facts.find((candidate) => candidate.field === "skills")
  const duplicate = (id, text) => ({
    claimId: id,
    kind: "exact_duplicate",
    reason: "same assertion",
    relatedFacts: [
      { profileId: snapshot.id, id: fact.id, revision: fact.revision },
    ],
    relatedClaimIds: [],
    operationIndexes: [],
  })
  const withChange = build(
    submission({
      batches: [{ index: 0, claimIds: ["run/0/c1", "run/0/c2"] }],
      claims: [
        claim("run/0/c1", { targets: ["skills"] }),
        claim("run/0/c2", { targets: ["skills"], text: "React" }),
      ],
      operations: [operation("run/0/c1", { value: "Go" })],
      outcomes: [
        duplicate("run/0/c2", "React"),
        {
          claimId: "run/0/c1",
          kind: "change",
          reason: "new skill",
          relatedFacts: [],
          relatedClaimIds: [],
          operationIndexes: [0],
        },
      ],
    }),
    snapshot,
  )
  const summary = review.reviewSummary(withChange)
  assert.equal(summary.selectable, 1)
  assert.equal(summary.selected, 0)
  assert.equal(summary.attention, 0)
  assert.equal(summary.duplicates, 1)
  assert.equal(summary.noChange, false)

  const duplicateOnly = build(
    submission({
      batches: [{ index: 0, claimIds: ["run/0/c2"] }],
      claims: [claim("run/0/c2", { targets: ["skills"], text: "React" })],
      outcomes: [duplicate("run/0/c2", "React")],
    }),
    snapshot,
  )
  const unchanged = review.reviewSummary(duplicateOnly)
  assert.equal(unchanged.selectable, 0)
  assert.equal(unchanged.duplicates, 1)
  assert.equal(unchanged.noChange, true)

  const conflicted = build(
    submission({
      batches: [{ index: 0, claimIds: ["run/0/c3"] }],
      claims: [claim("run/0/c3", { targets: ["skills"], text: "Python" })],
      outcomes: [
        {
          claimId: "run/0/c3",
          kind: "contradiction",
          reason: "competing levels",
          relatedFacts: [
            { profileId: snapshot.id, id: fact.id, revision: fact.revision },
          ],
          relatedClaimIds: [],
          operationIndexes: [],
        },
      ],
    }),
    snapshot,
  )
  const attention = review.reviewSummary(conflicted)
  assert.equal(attention.attention, 1)
  assert.equal(attention.conflicts, 1)
  assert.equal(attention.clarifications, 0)
  assert.equal(attention.noChange, false)

  const clarified = build(
    submission({
      batches: [{ index: 0, claimIds: ["run/0/c4"] }],
      claims: [
        claim("run/0/c4", {
          targets: ["experience"],
          question: "Full-time or contract?",
        }),
      ],
      outcomes: [
        {
          claimId: "run/0/c4",
          kind: "clarification",
          reason: "role type missing",
          relatedFacts: [],
          relatedClaimIds: [],
          operationIndexes: [],
        },
      ],
    }),
    snapshot,
  )
  const question = review.reviewSummary(clarified)
  assert.equal(question.attention, 1)
  assert.equal(question.clarifications, 1)
  assert.equal(question.questions, 1)
  assert.equal(question.noChange, false)
})

test("units disclose deduplicated sources with origins and resolved comparison context", () => {
  const snapshot = snapshotOf({ skills: "React" })
  const fact = snapshot.facts.find((candidate) => candidate.field === "skills")
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1", "run/0/c2"] }],
    claims: [
      claim("run/0/c1", {
        targets: ["skills"],
        source: "I use TypeScript",
        text: "TypeScript",
        supportingSources: [
          { source: "At Acme I used TypeScript." },
          {
            source: "I used it in production.",
            origin: "clarification_answer",
          },
          { source: "I use TypeScript" },
        ],
      }),
      claim("run/0/c2", {
        targets: ["skills"],
        text: "React",
        source: "React",
      }),
    ],
    operations: [operation("run/0/c1", { value: "TypeScript" })],
    outcomes: [
      {
        claimId: "run/0/c1",
        kind: "change",
        reason: "not represented",
        relatedFacts: [],
        relatedClaimIds: ["run/0/c2"],
        operationIndexes: [0],
      },
      {
        claimId: "run/0/c2",
        kind: "exact_duplicate",
        reason: "same assertion",
        relatedFacts: [
          { profileId: snapshot.id, id: fact.id, revision: fact.revision },
        ],
        relatedClaimIds: [],
        operationIndexes: [],
      },
    ],
  })
  const plan = build(sub, snapshot)
  const addition = unitFor(plan, (unit) => unit.kind === "addition")
  assert.deepEqual(
    addition.sources.map((source) => [source.excerpt, source.origin]),
    [
      ["I use TypeScript", "professional_information"],
      ["At Acme I used TypeScript.", "professional_information"],
      ["I used it in production.", "clarification_answer"],
    ],
  )
  assert.equal(addition.reason, "not represented")
  assert.deepEqual(
    addition.relatedClaims.map((related) => related.text),
    ["React"],
  )
  const duplicate = unitFor(plan, (unit) => unit.kind === "duplicate")
  assert.deepEqual(duplicate.relatedFacts, [
    {
      id: fact.id,
      revision: fact.revision,
      field: "skills",
      ownerKind: "profile",
      value: "React",
      normalization: fact.normalization,
    },
  ])
})

test("focused edits record the machine proposal without changing selection or support", () => {
  const snapshot = snapshotOf({ skills: "" })
  const plan = build(
    submission({
      batches: [{ index: 0, claimIds: ["run/0/c1"] }],
      claims: [claim("run/0/c1", { targets: ["skills"] })],
      operations: [operation("run/0/c1", { value: "TypeScript" })],
    }),
    snapshot,
  )
  const unit = plan.units[0]
  const selected = toggle(plan, unit.id)
  assert.equal(selected.units[0].status, "selected")
  const edited = review.editReviewUnit(selected, unit.id, "skills", "TS")
  assert.equal(edited.operations[0].value, "TS")
  assert.equal(edited.operations[0].proposedValue, "TypeScript")
  assert.equal(edited.units[0].status, "selected")
  assert.equal(edited.units[0].values[0].proposed, "TS")
  assert.equal(review.reviewUnitEdited(edited, unit.id), true)
  assert.equal(review.reviewUnitEdited(selected, unit.id), false)
  assert.equal(review.reviewPreview(edited).fields[0].after, "TS")
  const reedited = review.editReviewUnit(
    edited,
    unit.id,
    "skills",
    "TypeScript",
  )
  assert.equal(reedited.operations[0].proposedValue, "TypeScript")
  assert.equal(review.reviewUnitEdited(reedited, unit.id), false)
})

test("focused edits reject evidence-only wording and unknown fields", () => {
  const snapshot = snapshotOf({ skills: "React" })
  const fact = snapshot.facts.find((candidate) => candidate.field === "skills")
  const plan = build(
    submission({
      batches: [{ index: 0, claimIds: ["run/0/c1"] }],
      claims: [claim("run/0/c1", { targets: ["skills"] })],
      operations: [
        operation("run/0/c1", {
          action: "evidence",
          value: "React",
          finding: "in_place",
          supportingClaimIds: ["run/0/c1"],
        }),
      ],
      outcomes: [
        {
          claimId: "run/0/c1",
          kind: "additional_support",
          reason: "additional support",
          relatedFacts: [
            { profileId: snapshot.id, id: fact.id, revision: fact.revision },
          ],
          relatedClaimIds: [],
          operationIndexes: [0],
        },
      ],
    }),
    snapshot,
  )
  const unit = plan.units[0]
  assert.equal(unit.kind, "evidence")
  assert.throws(
    () => review.editReviewUnit(plan, unit.id, "skills", "React!"),
    { code: "readonly" },
  )
  assert.throws(() => review.editReviewUnit(plan, unit.id, "unknown", "x"), {
    code: "readonly",
  })
  const addition = build(
    submission({
      batches: [{ index: 0, claimIds: ["run/0/c1"] }],
      claims: [claim("run/0/c1", { targets: ["skills"] })],
      operations: [operation("run/0/c1", { value: "TypeScript" })],
    }),
    snapshot,
  )
  assert.throws(
    () => review.editReviewUnit(addition, addition.units[0].id, "title", "x"),
    { code: "input" },
  )
})

// transferReviewState (#191): a focused clarification rebuilds the plan, but
// reviewed wording and selected units survive when their operation identities
// and dependencies are untouched. The revised claim and dependent units restart.
const transferRun = "run/0"
const transferIds = ["run/0/c1", "run/0/c2", "run/0/c3"]
const transferSubmission = (values, outcomes) =>
  submission({
    batches: [{ index: 0, claimIds: transferIds }],
    claims: [
      claim(transferIds[0], { targets: ["skills", "competencies"] }),
      claim(transferIds[1], { targets: ["skills"] }),
      claim(transferIds[2], { targets: ["skills"] }),
    ],
    operations: [
      operation(transferIds[0], { value: values[0] }),
      operation(transferIds[0], {
        target: "competencies",
        field: "competencies",
        value: values[1],
      }),
      operation(transferIds[1], {
        value: values[2],
        supportingClaimIds: [transferIds[0]],
      }),
      operation(transferIds[2], { value: values[3] }),
    ],
    outcomes,
  })
const transferOutcomes = (revisedIndexes) => [
  {
    claimId: transferIds[0],
    kind: "change",
    reason: "r",
    relatedFacts: [],
    relatedClaimIds: [],
    operationIndexes: revisedIndexes,
  },
  {
    claimId: transferIds[1],
    kind: "change",
    reason: "r",
    relatedFacts: [],
    relatedClaimIds: [],
    operationIndexes: [2],
  },
  {
    claimId: transferIds[2],
    kind: "change",
    reason: "r",
    relatedFacts: [],
    relatedClaimIds: [],
    operationIndexes: [3],
  },
]

test("transferReviewState retains unrelated edits and selections after a synthesis", () => {
  const snapshot = snapshotOf({})
  const previous = build(
    transferSubmission(["Java", "Debugging", "Rust", "Kept"], transferOutcomes([0, 1])),
    snapshot,
  )
  const unrelated = unitFor(
    previous,
    (unit) => unit.claimIds[0] === transferIds[2] && unit.kind === "addition",
  )
  const edited = review.editReviewUnit(
    toggle(previous, unrelated.id),
    unrelated.id,
    "skills",
    "Reviewed wording",
  )
  const dependent = unitFor(
    edited,
    (unit) => unit.claimIds[0] === transferIds[1] && unit.kind === "addition",
  )
  const selected = toggle(edited, dependent.id)

  const next = build(
    transferSubmission(
      ["TypeScript", "Debugging", "Rust", "Kept"],
      transferOutcomes([0, 1]),
    ),
    snapshot,
  )
  const transferred = review.transferReviewState(edited, next, [transferIds[0]])
  assert.notEqual(transferred, next)

  const revised = unitFor(
    transferred,
    (unit) => unit.claimIds[0] === transferIds[0] && unit.kind === "addition",
  )
  assert.equal(revised.status, "pending")
  assert.equal(
    revised.values.find((value) => value.field === "skills").proposed,
    "TypeScript",
  )

  const retained = unitFor(
    transferred,
    (unit) => unit.claimIds[0] === transferIds[2] && unit.kind === "addition",
  )
  assert.equal(retained.status, "selected")
  assert.equal(
    retained.values.find((value) => value.field === "skills").proposed,
    "Reviewed wording",
  )
  assert.equal(review.reviewUnitEdited(transferred, retained.id), true)

  const resetDependent = unitFor(
    transferred,
    (unit) => unit.claimIds[0] === transferIds[1] && unit.kind === "addition",
  )
  assert.equal(resetDependent.status, "pending")
  assert.equal(review.reviewPlanError(transferred), "")
})

test("transferReviewState resets a unit whose machine proposal changed", () => {
  const snapshot = snapshotOf({})
  const previous = build(
    transferSubmission(["Java", "Debugging", "Rust", "Kept"], transferOutcomes([0, 1])),
    snapshot,
  )
  const unrelated = unitFor(
    previous,
    (unit) => unit.claimIds[0] === transferIds[2] && unit.kind === "addition",
  )
  const edited = review.editReviewUnit(
    toggle(previous, unrelated.id),
    unrelated.id,
    "skills",
    "Reviewed wording",
  )

  const next = build(
    transferSubmission(
      ["Java", "Debugging", "Rust", "Changed"],
      transferOutcomes([0, 1]),
    ),
    snapshot,
  )
  const transferred = review.transferReviewState(edited, next, [transferIds[0]])
  const changed = unitFor(
    transferred,
    (unit) => unit.claimIds[0] === transferIds[2] && unit.kind === "addition",
  )
  assert.equal(changed.status, "pending")
  assert.equal(
    changed.values.find((value) => value.field === "skills").proposed,
    "Changed",
  )
  assert.equal(review.reviewUnitEdited(transferred, changed.id), false)
})

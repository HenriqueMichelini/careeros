import { workflowModuleURL } from "./workflow-module.mjs"
// Submission-level save evidence (#189). Transpiles the review model, the
// canonical document/storage seams and the new save compiler, then drives one
// selection spanning many bounded results through ONE canonical transaction:
// aggregate above the per-response 30/60 caps, dependency-safe mixed selection,
// new entries, duplicate identities, partial evidence, edited wording, unrelated
// data preservation, stale/two-tab writes, quota/lock failures and post-commit
// acknowledgement. It also proves the single-result validator is not weakened.
import test from "node:test"
import assert from "node:assert/strict"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"

const temp = mkdtempSync(join(tmpdir(), "ingestion-save-"))
writeFileSync(
  join(temp, "contract.mjs"),
  `export default ${readFileSync(
    new URL("../internal/profiledocument/contract.json", import.meta.url),
    "utf8",
  )}`,
)
const rewrites = [
  ['"./profileDocument"', '"./profileDocument.mjs"'],
  ['"./ingestionProtocol"', '"./ingestionProtocol.mjs"'],
  ['"./ingestionReview"', '"./ingestionReview.mjs"'],
  ['"./ingestionDocument"', '"./ingestionDocument.mjs"'],
  ['"./ingestionSave"', '"./ingestionSave.mjs"'],
  ['"./profileStorage"', '"./profileStorage.mjs"'],
  ['"./ingestion"', '"./ingestion.mjs"'],
  ['"./profile"', '"./profile.mjs"'],
  ['"./fieldDecision"', '"./fieldDecision.mjs"'],
  ['"./types"', '"./types.mjs"'],
  ['"../../internal/profiledocument/contract.json"', '"./contract.mjs"'],
]
for (const name of [
  "types",
  "profileDocument",
  "profile",
  "fieldDecision",
  "ingestionProtocol",
  "ingestion",
  "ingestionReview",
  "ingestionDocument",
  "profileStorage",
  "ingestionSave",
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
        verbatimModuleSyntax: false,
      },
    }).outputText.replaceAll('"./workflowRequest"', JSON.stringify(workflowModuleURL)),
  )
}

const profileDocument = await import(join(temp, "profileDocument.mjs"))
const review = await import(join(temp, "ingestionReview.mjs"))
const ingestion = await import(join(temp, "ingestion.mjs"))
const storageModule = await import(join(temp, "profileStorage.mjs"))
const save = await import(join(temp, "ingestionSave.mjs"))

const { migrateProfile, profileView, validateProfileDocument } = profileDocument
const { PROFILE_KEY, ProfileStorageError } = storageModule

// --- Fixtures ---------------------------------------------------------------

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
  submissionId: "sub-189",
  runId: "run",
  sourceId: "source-189",
  profileId: "profile",
  planVersion: "portions-v1",
  batches: [],
  focus: [],
  claims: [],
  operations: [],
  outcomes: [],
  unresolvedClaimIds: [],
  ...overrides,
})
const planOf = (sub, snapshot) =>
  review.buildReviewPlan({ submission: sub, snapshot })
const toggle = (plan, unitId) =>
  review.selectReviewUnit(plan, unitId, "selected")

// A fake OriginStorage that records every committed key.
function fakeStorage(initial) {
  const map = new Map()
  if (initial !== undefined) map.set(PROFILE_KEY, JSON.stringify(initial))
  const writes = []
  return {
    writes,
    getItem: (key) => (map.has(key) ? map.get(key) : null),
    setItem: (key, value) => {
      writes.push(key)
      map.set(key, value)
    },
    raw: () => map.get(PROFILE_KEY),
  }
}
const lock = async (op) => op()

const context = (sub, snapshot, plan, input) => ({
  submission: sub,
  snapshot,
  plan,
  input,
})

// --- One canonical transaction ---------------------------------------------

test("reviewed submission uses account Save without browser storage and awaits acknowledgment", async () => {
  const input = "I use Go."
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    focus: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [claim("run/0/c1", { source: input, text: input, targets: ["skills"] })],
    operations: [operation("run/0/c1", { value: "Go" })],
  })
  const snapshot = snapshotOf()
  const plan = review.selectReviewAdditions(planOf(sub, snapshot))
  let calls = 0
  let release
  const saving = save.saveSubmissionSelect(context(sub, snapshot, plan, input), {
    save: async (expected, candidate) => {
      calls++
      assert.equal(expected, snapshot)
      assert.ok(validateProfileDocument(candidate))
      assert.equal(profileView(candidate).skills, "Go")
      return new Promise(resolve => { release = resolve })
    },
  })
  assert.equal(calls, 1)
  let settled = false
  void saving.then(() => { settled = true })
  await Promise.resolve()
  assert.equal(settled, false)
  release(true)
  assert.equal((await saving).acknowledged, true)
  const denied = await save.saveSubmissionSelect(context(sub, snapshot, plan, input), { save: async () => false })
  assert.equal(denied.acknowledged, false)
  assert.equal(denied.status, "failed")
})

test("a selection above 30 claims and 60 operations saves in one transaction", async () => {
  const batches = []
  const claims = []
  const operations = []
  const sources = []
  const counts = [12, 12, 12]
  counts.forEach((count, index) => {
    const ids = []
    for (let local = 0; local < count; local++) {
      const id = `run/${index}/c${local}`
      ids.push(id)
      const source = `i use skill${index}-${local} and note${index}-${local}`
      sources.push(source)
      claims.push(claim(id, { targets: ["skills", "additionalInfo"], source }))
      operations.push(operation(id, { value: `skill${index}-${local}` }))
      operations.push(
        operation(id, {
          target: "additionalInfo",
          field: "additionalInfo",
          value: `note${index}-${local}`,
        }),
      )
    }
    batches.push({ index, claimIds: ids })
  })
  const input = sources.join("\n")
  const focus = batches.map((batch) => ({
    index: batch.index,
    claimIds: batch.claimIds,
  }))
  const sub = submission({ batches, focus, claims, operations })
  const snapshot = snapshotOf({
    skills: "React",
    additionalInfo: "Kept note",
    careerGoals: "Lead teams",
  })
  const untouched = structuredClone(snapshot)
  let plan = planOf(sub, snapshot)
  assert.ok(sub.claims.length > 30, "the aggregate exceeds the claim cap")
  assert.ok(
    sub.operations.length > 60,
    "the aggregate exceeds the operation cap",
  )
  assert.equal(plan.units.length, 72)
  plan = review.selectReviewAdditions(plan)
  assert.equal(save.validateSubmissionSave(sub, snapshot, plan), "")

  const storage = fakeStorage(snapshot)
  const outcome = await save.saveSubmissionSelect(
    context(sub, snapshot, plan, input),
    { storage, lock },
  )

  assert.equal(outcome.status, "saved")
  assert.equal(outcome.acknowledged, true)
  const candidate = outcome.candidate
  assert.equal(validateProfileDocument(candidate), true)
  assert.equal(candidate.revision, snapshot.revision + 1)
  // Exactly one atomic write, and only after durability is acknowledged.
  assert.deepEqual(storage.writes, [PROFILE_KEY])
  assert.equal(storage.raw(), JSON.stringify(candidate))

  const view = profileView(candidate)
  assert.equal(view.skills.split("\n").length, 37, "36 appended skills")
  assert.equal(view.additionalInfo.split("\n").length, 37)
  assert.equal(view.careerGoals, "Lead teams", "unrelated scalar preserved")
  // Every accepted excerpt persists, and nothing else does.
  assert.equal(candidate.evidence.length, 72)
  assert.ok(
    candidate.evidence.every((e) => e.origin === "professional_information"),
  )
  assert.ok(
    candidate.evidence.every((e) => input.includes(e.excerpt)),
    "only accepted excerpts persist",
  )
  assert.equal(JSON.stringify(candidate).includes(input), false)
  assert.deepEqual(snapshot, untouched, "the pinned snapshot is never mutated")
})

test("the 60-operation bound is per reconciliation response, not per extraction batch", () => {
  // Extraction batches [20, 20] claims and reconciliation responses [30, 60]
  // operations. By extraction batch this looks like [20, 70], which is not a
  // real bound: one focus spans both batches and each response respects its own
  // 60-operation cap.
  const claims = []
  const firstIds = []
  const secondIds = []
  for (let index = 0; index < 20; index++) {
    const id = `run/0/c${index}`
    firstIds.push(id)
    claims.push(claim(id, { source: `i use first${index}` }))
  }
  for (let index = 0; index < 20; index++) {
    const id = `run/1/c${index}`
    secondIds.push(id)
    claims.push(claim(id, { source: `i use second${index}` }))
  }
  const focusFirst = [...firstIds, ...secondIds.slice(0, 10)]
  const focusSecond = secondIds.slice(10)
  const fields = [
    "skills",
    "tools",
    "competencies",
    "additionalInfo",
    "careerGoals",
    "desiredSalary",
  ]
  const operations = []
  for (const id of focusFirst)
    operations.push(operation(id, { value: `v-${id}` }))
  for (const id of focusSecond)
    for (const field of fields)
      operations.push(
        operation(id, { target: field, field, value: `${field}-${id}` }),
      )
  const sub = submission({
    batches: [
      { index: 0, claimIds: firstIds },
      { index: 1, claimIds: secondIds },
    ],
    focus: [
      { index: 0, claimIds: focusFirst },
      { index: 1, claimIds: focusSecond },
    ],
    claims,
    operations,
  })
  assert.equal(operations.length, 90, "aggregate exceeds the operation cap")
  // Attributed to extraction batches: batch 0 = 20, batch 1 = 70.
  assert.equal(
    operations.filter((op) => op.claimId.startsWith("run/1/")).length,
    70,
  )
  const snapshot = snapshotOf({
    skills: "",
    tools: "",
    competencies: "",
    additionalInfo: "",
    careerGoals: "",
    desiredSalary: "",
  })
  let plan = planOf(sub, snapshot)
  plan = review.selectReviewAdditions(plan)
  assert.equal(save.validateSubmissionSave(sub, snapshot, plan), "")
})

test("a reconciliation response above 60 operations is rejected as capacity", () => {
  const claims = []
  const ids = []
  for (let index = 0; index < 11; index++) {
    const id = `run/0/c${index}`
    ids.push(id)
    claims.push(claim(id, { source: `i use c${index}` }))
  }
  const fields = [
    "skills",
    "tools",
    "competencies",
    "additionalInfo",
    "careerGoals",
    "desiredSalary",
  ]
  const operations = []
  for (const id of ids)
    for (const field of fields)
      operations.push(
        operation(id, { target: field, field, value: `${field}-${id}` }),
      )
  // 11 claims x 6 fields = 66 operations in one response.
  assert.equal(operations.length, 66)
  const sub = submission({
    batches: [{ index: 0, claimIds: ids }],
    focus: [{ index: 0, claimIds: ids }],
    claims,
    operations,
  })
  const snapshot = snapshotOf({ skills: "" })
  let plan = planOf(sub, snapshot)
  plan = review.selectReviewAdditions(plan)
  assert.equal(save.validateSubmissionSave(sub, snapshot, plan), "capacity")
})

test("the single-result validator still enforces the 30/60 caps; only the aggregate relaxes", () => {
  const snapshot = snapshotOf({ skills: "" })
  const view = profileView(snapshot)
  const sources = []
  const claims = []
  for (let index = 0; index < 31; index++) {
    const source = `claim${index}`
    sources.push(source)
    claims.push(claim(`c${index}`, { source, targets: ["skills"] }))
  }
  const input = sources.join(" ")
  const raw = {
    claims,
    operations: [],
    unverifiedClaimCount: 0,
    unresolvedClaimIds: claims.map((c) => c.id),
    unplacedOperationCount: 0,
  }
  assert.throws(
    () => ingestion.validateIngestionResult(raw, input, view, snapshot),
    {
      code: "invalid_output",
    },
  )
  const aggregate = ingestion.validateIngestionAggregate(
    raw,
    input,
    view,
    snapshot,
  )
  assert.equal(aggregate.claims.length, 31)
})

// --- Dependencies, identity and selection validity --------------------------

test("a detail selected without its new-entry identity is rejected before mutation", async () => {
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
  const plan = planOf(sub, snapshot)
  const detail = plan.units.find((unit) => unit.field === "description")
  // Even a forged partial selection is caught by the review model and the save.
  const forged = {
    ...plan,
    units: plan.units.map((unit) => ({
      ...unit,
      status: unit.required ? "pending" : "selected",
    })),
  }
  assert.equal(
    save.validateSubmissionSave(sub, snapshot, forged),
    "selection_closure",
  )
  const storage = fakeStorage(snapshot)
  const outcome = await save.saveSubmissionSelect(
    context(sub, snapshot, forged, "Aster Labs Engineer Backend engineer."),
    { storage, lock },
  )
  assert.equal(outcome.acknowledged, false)
  assert.equal(outcome.status, "failed")
  assert.equal(storage.writes.length, 0)
  assert.notEqual(detail, undefined)
})

test("a selected operation with dangling support is rejected before mutation", () => {
  const snapshot = snapshotOf({ skills: "React" })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [claim("run/0/c1", { targets: ["skills"], source: "I use Go." })],
    operations: [operation("run/0/c1", { value: "Go" })],
  })
  const built = planOf(sub, snapshot)
  // Forge a support edge to a claim the submission never assembled.
  const forged = {
    ...built,
    operations: built.operations.map((op) => ({
      ...op,
      supportingClaimIds: ["run/9/missing"],
    })),
  }
  const plan = toggle(forged, forged.units[0].id)
  assert.equal(
    save.validateSubmissionSave(sub, snapshot, plan),
    "dangling_support",
  )
})

test("a complete new entry and its dependent detail save as one entry", async () => {
  const snapshot = snapshotOf({ experience: [] })
  const input = "Aster Labs Staff Engineer Backend engineer."
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1", "run/0/c2"] }],
    claims: [
      claim("run/0/c1", {
        targets: ["experience"],
        source: "Aster Labs Staff Engineer",
      }),
      claim("run/0/c2", {
        targets: ["experience"],
        source: "Backend engineer.",
      }),
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
  const plan = review.selectReviewAdditions(planOf(sub, snapshot))
  const storage = fakeStorage(snapshot)
  const outcome = await save.saveSubmissionSelect(
    context(sub, snapshot, plan, input),
    { storage, lock },
  )
  assert.equal(outcome.acknowledged, true)
  const view = profileView(outcome.candidate)
  assert.equal(view.experience.length, 1)
  assert.equal(view.experience[0].company, "Aster Labs")
  assert.equal(view.experience[0].title, "Staff Engineer")
  assert.equal(view.experience[0].description, "Backend engineer.")
  assert.equal(outcome.candidate.entities.length, 1)
})

test("two same-name entries stay distinct; identical duplicates are rejected", async () => {
  const makeEntry = (n, overrides) =>
    submission({
      batches: [{ index: 0, claimIds: [`run/0/e${n}`] }],
      claims: [
        claim(`run/0/e${n}`, {
          targets: ["experience"],
          source: `Acme Engineer ${overrides?.startDate ?? ""}`.trim(),
        }),
      ],
      operations: [
        operation(`run/0/e${n}`, {
          target: "experience",
          entryId: `new:run/0/e${n}`,
          field: "company",
          value: "Acme",
        }),
        operation(`run/0/e${n}`, {
          target: "experience",
          entryId: `new:run/0/e${n}`,
          field: "title",
          value: "Engineer",
        }),
        operation(`run/0/e${n}`, {
          target: "experience",
          entryId: `new:run/0/e${n}`,
          field: "startDate",
          value: overrides?.startDate ?? "",
        }),
      ],
    })
  // Distinct periods keep the two stints apart.
  const snapshot = snapshotOf({ experience: [] })
  const first = makeEntry(1, { startDate: "2018-01" })
  const second = makeEntry(2, { startDate: "2024-01" })
  const merged = submission({
    batches: [{ index: 0, claimIds: ["run/0/e1", "run/0/e2"] }],
    claims: [...first.claims, ...second.claims],
    operations: [...first.operations, ...second.operations],
  })
  let plan = planOf(merged, snapshot)
  plan = review.selectReviewAdditions(plan)
  assert.equal(save.validateSubmissionSave(merged, snapshot, plan), "")
  const storage = fakeStorage(snapshot)
  const outcome = await save.saveSubmissionSelect(
    context(
      merged,
      snapshot,
      plan,
      "Acme Engineer 2018-01 Acme Engineer 2024-01",
    ),
    { storage, lock },
  )
  assert.equal(outcome.acknowledged, true)
  assert.equal(outcome.candidate.entities.length, 2)

  // Identical identity is rejected: the canonical preview refuses duplicates.
  const duplicate = submission({
    batches: [{ index: 0, claimIds: ["run/0/e1"] }],
    claims: [claim("run/0/e1", { targets: ["experience"] })],
    operations: [
      operation("run/0/e1", {
        target: "experience",
        entryId: "new:run/0/e1",
        field: "company",
        value: "Acme",
      }),
      operation("run/0/e1", {
        target: "experience",
        entryId: "new:run/0/e2",
        field: "company",
        value: "Acme",
      }),
      operation("run/0/e1", {
        target: "experience",
        entryId: "new:run/0/e1",
        field: "title",
        value: "Engineer",
      }),
      operation("run/0/e1", {
        target: "experience",
        entryId: "new:run/0/e2",
        field: "title",
        value: "Engineer",
      }),
    ],
  })
  const dupSnapshot = snapshotOf({ experience: [] })
  const dupPlan = review.selectReviewAdditions(planOf(duplicate, dupSnapshot))
  const dupStorage = fakeStorage(dupSnapshot)
  const dupOutcome = await save.saveSubmissionSelect(
    context(duplicate, dupSnapshot, dupPlan, "Acme Engineer"),
    { storage: dupStorage, lock },
  )
  assert.equal(dupOutcome.acknowledged, false)
  assert.equal(dupStorage.writes.length, 0)
})

// --- Evidence and wording ---------------------------------------------------

async function reactSnapshot() {
  // A first accepted save establishes a supported React fact with one excerpt.
  const snapshot = snapshotOf({ skills: "" })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [
      claim("run/0/c1", { targets: ["skills"], source: "I use React." }),
    ],
    operations: [operation("run/0/c1", { value: "React" })],
  })
  const plan = review.selectReviewAdditions(planOf(sub, snapshot))
  const outcome = await save.saveSubmissionSelect(
    context(sub, snapshot, plan, "I use React."),
    { storage: fakeStorage(snapshot), lock },
  )
  assert.equal(outcome.acknowledged, true)
  return outcome.candidate
}

test("an evidence-only addition retains wording and prior support, separate from updates", async () => {
  const snapshot = await reactSnapshot()
  const fact = snapshot.facts.find((f) => f.field === "skills")
  assert.equal(fact.support, "supported")
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [
      claim("run/0/c1", {
        targets: ["skills"],
        source: "React is my main tool.",
      }),
    ],
    operations: [operation("run/0/c1", { action: "evidence", value: "React" })],
  })
  const plan = toggle(planOf(sub, snapshot), planOf(sub, snapshot).units[0].id)
  const storage = fakeStorage(snapshot)
  const outcome = await save.saveSubmissionSelect(
    context(sub, snapshot, plan, "I use React. React is my main tool."),
    { storage, lock },
  )
  assert.equal(outcome.acknowledged, true)
  const view = profileView(outcome.candidate)
  assert.equal(view.skills, "React", "wording retained")
  const updated = outcome.candidate.facts.find((f) => f.field === "skills")
  assert.equal(updated.id, fact.id)
  assert.equal(updated.revision, fact.revision + 1)
  assert.equal(updated.support, "supported", "prior support retained")
  assert.equal(
    outcome.candidate.evidence.length,
    2,
    "the new excerpt is separate",
  )
})

test("mixing evidence-only support with a wording update is rejected", async () => {
  const snapshot = await reactSnapshot()
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1", "run/0/c2"] }],
    claims: [
      claim("run/0/c1", { targets: ["skills"], source: "I use React." }),
      claim("run/0/c2", { targets: ["skills"], source: "I use TypeScript." }),
    ],
    operations: [
      operation("run/0/c1", { action: "evidence", value: "React" }),
      operation("run/0/c2", { action: "update", value: "TypeScript" }),
    ],
  })
  let plan = planOf(sub, snapshot)
  plan = toggle(plan, plan.units.find((u) => u.kind === "evidence").id)
  plan = toggle(plan, plan.units.find((u) => u.kind === "replacement").id)
  assert.equal(
    save.validateSubmissionSave(sub, snapshot, plan),
    "incompatible_evidence",
  )
  const storage = fakeStorage(snapshot)
  const outcome = await save.saveSubmissionSelect(
    context(sub, snapshot, plan, "I use React. I use TypeScript."),
    { storage, lock },
  )
  assert.equal(outcome.acknowledged, false)
  assert.equal(storage.writes.length, 0)
})

test("edited proposal wording keeps manual provenance and never inherits machine support", async () => {
  const snapshot = snapshotOf({ skills: "" })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [
      claim("run/0/c1", { targets: ["skills"], source: "I use TypeScript." }),
    ],
    operations: [
      operation("run/0/c1", {
        value: "TypeScript",
        proposedValue: "TypeScript",
      }),
    ],
  })
  const built = planOf(sub, snapshot)
  // An edited wording flips the saved value while the machine proposal remains.
  const edited = {
    ...built,
    operations: built.operations.map((op) => ({
      ...op,
      value: "TypeScript for browsers",
      proposedValue: "TypeScript",
    })),
  }
  const plan = toggle(edited, edited.units[0].id)
  const outcome = await save.saveSubmissionSelect(
    context(sub, snapshot, plan, "I use TypeScript."),
    { storage: fakeStorage(snapshot), lock },
  )
  assert.equal(outcome.acknowledged, true)
  const fact = outcome.candidate.facts.find((f) => f.field === "skills")
  assert.deepEqual(fact.origin, { kind: "manual_edit", original: "user" })
  assert.equal(fact.support, "unsupported")
  assert.equal(outcome.candidate.evidence.length, 0)
})

test("clarification-answer origin stays distinct from professional information", async () => {
  const snapshot = snapshotOf({ skills: "" })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [
      claim("run/0/c1", {
        targets: ["skills"],
        source: "I use React.",
        supportingSources: [
          { source: "React indeed", origin: "clarification_answer" },
        ],
      }),
    ],
    operations: [operation("run/0/c1", { value: "React" })],
  })
  const plan = review.selectReviewAdditions(planOf(sub, snapshot))
  const outcome = await save.saveSubmissionSelect(
    context(sub, snapshot, plan, "I use React. React indeed"),
    { storage: fakeStorage(snapshot), lock },
  )
  assert.equal(outcome.acknowledged, true)
  const origins = outcome.candidate.evidence.map((e) => e.origin).sort()
  assert.deepEqual(origins, [
    "clarification_answer",
    "professional_information",
  ])
})

// --- Preservation, staleness and failure ------------------------------------

test("unrelated IDs, order, revisions, qualifiers and evidence links are preserved", async () => {
  const snapshot = snapshotOf({
    skills: "React",
    careerGoals: "Lead teams",
    experience: [experience("e1", { company: "Acme", title: "Engineer" })],
  })
  // Qualify an untouched fact so we can prove it survives byte-for-byte.
  const qualifier = snapshot.facts.find((f) => f.field === "careerGoals")
  qualifier.kind = "statement"
  qualifier.assertion = "negated"
  const qualifierBefore = structuredClone(qualifier)
  const before = JSON.stringify(snapshot)
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [claim("run/0/c1", { targets: ["skills"], source: "I use Go." })],
    operations: [operation("run/0/c1", { value: "Go" })],
  })
  const plan = review.selectReviewAdditions(planOf(sub, snapshot))
  const outcome = await save.saveSubmissionSelect(
    context(sub, snapshot, plan, "I use Go."),
    { storage: fakeStorage(snapshot), lock },
  )
  assert.equal(outcome.acknowledged, true)
  const untouched = outcome.candidate.facts.find((f) => f.id === qualifier.id)
  // The owner ref tracks the Profile revision, but the fact's own id, revision,
  // order, wording, qualifier, approval and support are untouched.
  assert.equal(untouched.id, qualifierBefore.id)
  assert.equal(untouched.revision, qualifierBefore.revision)
  assert.equal(untouched.order, qualifierBefore.order)
  assert.equal(untouched.kind, "statement")
  assert.equal(untouched.assertion, "negated")
  assert.equal(untouched.value, "Lead teams")
  assert.equal(untouched.approval, "unreviewed")
  assert.equal(untouched.support, "unsupported")
  const entity = outcome.candidate.entities.find((e) => e.legacyId === "e1")
  assert.equal(entity.order, 0)
  assert.equal(entity.revision, 1)
  assert.equal(
    JSON.stringify(snapshot),
    before,
    "the snapshot is never mutated",
  )
})

test("a two-tab write detected as stale fails without touching storage", async () => {
  const snapshot = snapshotOf({ skills: "React" })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [claim("run/0/c1", { targets: ["skills"], source: "I use Go." })],
    operations: [operation("run/0/c1", { value: "Go" })],
  })
  const plan = review.selectReviewAdditions(planOf(sub, snapshot))
  // Another tab already advanced the canonical document.
  const other = snapshotOf({ skills: "Rust" })
  const storage = fakeStorage(other)
  const outcome = await save.saveSubmissionSelect(
    context(sub, snapshot, plan, "I use Go."),
    { storage, lock },
  )
  assert.equal(outcome.status, "failed")
  assert.equal(outcome.code, "stale")
  assert.equal(outcome.acknowledged, false)
  assert.equal(storage.writes.length, 0)
  assert.equal(storage.raw(), JSON.stringify(other))

  // A mismatched pinned snapshot is rejected before any compile.
  assert.equal(save.validateSubmissionSave(sub, other, plan), "stale")
})

test("storage quota and lock failures retain review and never write", async () => {
  const snapshot = snapshotOf({ skills: "React" })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [claim("run/0/c1", { targets: ["skills"], source: "I use Go." })],
    operations: [operation("run/0/c1", { value: "Go" })],
  })
  const plan = review.selectReviewAdditions(planOf(sub, snapshot))
  const ctx = context(sub, snapshot, plan, "I use Go.")

  const denied = fakeStorage(snapshot)
  denied.setItem = () => {
    throw new Error("QuotaExceededError")
  }
  const quota = await save.saveSubmissionSelect(ctx, { storage: denied, lock })
  assert.equal(quota.status, "failed")
  assert.equal(quota.code, "storage")
  assert.equal(quota.acknowledged, false)

  const locked = fakeStorage(snapshot)
  const lockFailure = await save.saveSubmissionSelect(ctx, {
    storage: locked,
    lock: async () => {
      throw new ProfileStorageError("lock")
    },
  })
  assert.equal(lockFailure.status, "failed")
  assert.equal(lockFailure.code, "lock")
  assert.equal(lockFailure.acknowledged, false)
  assert.equal(locked.writes.length, 0)
})

test("a committed write with failed lock feedback is acknowledged, not repeated", async () => {
  const snapshot = snapshotOf({ skills: "React" })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [claim("run/0/c1", { targets: ["skills"], source: "I use Go." })],
    operations: [operation("run/0/c1", { value: "Go" })],
  })
  const plan = review.selectReviewAdditions(planOf(sub, snapshot))
  const storage = fakeStorage(snapshot)
  // The lock delivers the commit then throws before returning feedback.
  const lateFailure = async (op) => {
    await op()
    throw new ProfileStorageError("lock")
  }
  const first = await save.saveSubmissionSelect(
    context(sub, snapshot, plan, "I use Go."),
    { storage, lock: lateFailure },
  )
  assert.equal(first.acknowledged, true, "the atomic commit is acknowledged")
  assert.equal(storage.writes.length, 1)
  const committed = storage.raw()

  // Retry after the committed write is a pure re-run: zero AI calls, no second
  // write and no revision change.
  const retry = await save.saveSubmissionSelect(
    context(sub, snapshot, plan, "I use Go."),
    { storage, lock: lateFailure },
  )
  assert.equal(retry.acknowledged, false)
  assert.equal(retry.status, "failed")
  assert.equal(
    storage.writes.length,
    1,
    "the committed write is never duplicated",
  )
  assert.equal(storage.raw(), committed)
})

test("an empty selection is a no-op with nothing committed", async () => {
  const snapshot = snapshotOf({ skills: "React" })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [claim("run/0/c1", { targets: ["skills"], source: "I use Go." })],
    operations: [operation("run/0/c1", { value: "Go" })],
  })
  const plan = planOf(sub, snapshot)
  assert.equal(review.selectedReviewOperations(plan).length, 0)
  const storage = fakeStorage(snapshot)
  const outcome = await save.saveSubmissionSelect(
    context(sub, snapshot, plan, "I use Go."),
    { storage, lock },
  )
  assert.equal(outcome.status, "empty")
  assert.equal(outcome.acknowledged, false)
  assert.equal(storage.writes.length, 0)
})

test("validate, compile and save make zero provider calls on every path", async () => {
  const snapshot = snapshotOf({ skills: "React" })
  const sub = submission({
    batches: [{ index: 0, claimIds: ["run/0/c1"] }],
    claims: [claim("run/0/c1", { targets: ["skills"], source: "I use Go." })],
    operations: [operation("run/0/c1", { value: "Go" })],
  })
  const plan = review.selectReviewAdditions(planOf(sub, snapshot))
  const original = globalThis.fetch
  let calls = 0
  globalThis.fetch = async () => {
    calls += 1
    throw new Error("provider call is out of scope for a save")
  }
  try {
    save.validateSubmissionSave(sub, snapshot, plan)
    save.compileSubmissionSave(sub, snapshot, plan)
    await save.saveSubmissionSelect(context(sub, snapshot, plan, "I use Go."), {
      storage: fakeStorage(snapshot),
      lock,
    })
  } finally {
    globalThis.fetch = original
  }
  assert.equal(calls, 0)
})

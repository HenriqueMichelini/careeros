// Whole-submission controller evidence (#187). Transpiles the TypeScript
// controller and its seams, then drives one foreground submission through a
// controlled transport: exact stage/call order, exact pinned contexts, bounded
// focus batches above the old per-batch aggregate ceilings, deadline exhaustion
// without rolling reset, first-failure stop with retained partial state, response
// validation, no pre-save writes and a clean handoff to the #183 review model.
import test from "node:test"
import assert from "node:assert/strict"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"

const temp = mkdtempSync(join(tmpdir(), "ingestion-submission-"))
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
  "ingestionSubmission",
]) {
  let source = readFileSync(new URL(`../src/lib/${name}.ts`, import.meta.url), "utf8")
  for (const [from, to] of rewrites) source = source.replaceAll(from, to)
  writeFileSync(
    join(temp, `${name}.mjs`),
    ts.transpileModule(source, {
      compilerOptions: {
        module: ts.ModuleKind.ESNext,
        target: ts.ScriptTarget.ES2022,
        verbatimModuleSyntax: false,
      },
    }).outputText,
  )
}

const submission = await import(join(temp, "ingestionSubmission.mjs"))
const protocol = await import(join(temp, "ingestionProtocol.mjs"))
const review = await import(join(temp, "ingestionReview.mjs"))
const profileDocument = await import(join(temp, "profileDocument.mjs"))
const profileModule = await import(join(temp, "profile.mjs"))

// --- Fixtures ---------------------------------------------------------------

const NOW = 1_700_000_000_000
const RUN_ID = "run-187"
const RECEIPT_TAIL = "s".repeat(43)
const receipt = (label) => `${label}.${RECEIPT_TAIL}`
const segments = ["I use TypeScript.", "I built payment APIs.", "I lead a team."]
const original = segments.join(" ")
const perBatch = 15

const baseProfile = () => ({
  careerGoals: "",
  skills: "React",
  competencies: "",
  experience: [],
  tools: "",
  projects: [],
  employmentStatus: "",
  currentSalary: "",
  desiredSalary: "",
  additionalInfo: "",
})
const legacy = (overrides = {}) => ({
  ...baseProfile(),
  fullName: "",
  email: "",
  phone: "",
  location: "",
  professionalLinks: "",
  education: [],
  certifications: [],
  languages: [],
  ...overrides,
})
const makeSnapshot = (overrides = {}) => ({
  input: original,
  profile: baseProfile(),
  document: profileDocument.migrateProfile(legacy({ skills: "React" }), "profile"),
  ...overrides,
})

function planFixture(overrides = {}) {
  let offset = 0
  const batches = segments.map((segment, index) => {
    const end =
      index === segments.length - 1 ? original.length : offset + segment.length + 1
    const batch = {
      index,
      primary: [{ Start: offset, End: end }],
      context: [],
      preparedBytes: 200,
      payloadBytes: 128,
    }
    offset = end
    return batch
  })
  return {
    protocolVersion: "profile-submission-v1",
    sourceId: "source-187",
    profileId: "profile-187",
    runId: RUN_ID,
    planVersion: "portions-v1",
    status: "ready",
    complete: true,
    originalBytes: new TextEncoder().encode(original).length,
    preparedBytes: batches.length * 200,
    batches,
    remaining: [],
    ...overrides,
  }
}
const localClaimIds = Array.from({ length: perBatch }, (_, index) => `c${index}`)
const globalClaimIds = (index) =>
  localClaimIds.map((id) => `${RUN_ID}/${index}/${id}`)

function planResponse(plan = planFixture()) {
  return {
    stage: { version: 2, kind: "plan" },
    planReceipt: receipt("plan"),
    expiresAt: NOW + 600_000,
    plan,
  }
}
function extractionClaims(index) {
  return localClaimIds.map((id) => ({
    id,
    source: segments[index],
    text: segments[index],
    targets: ["skills"],
    question: "",
  }))
}
function extractionResponse(index) {
  return {
    stage: { version: 2, kind: "extract" },
    batchReceipt: receipt(`batch${index}`),
    expiresAt: NOW + 600_000,
    batchIndex: index,
    claims: extractionClaims(index),
    unverifiedCount: 0,
    skippedClaims: [],
    coverage: {
      validClaims: perBatch,
      invalidClaims: 0,
      discoveryComplete: false,
      capacity: "within_limit",
    },
  }
}
function aggregateFixture(plan = planFixture()) {
  const batches = plan.batches.map((batch) => {
    const claimIds = globalClaimIds(batch.index)
    return {
      index: batch.index,
      primary: batch.primary,
      claimIds,
      claimCount: claimIds.length,
      skippedCount: 0,
    }
  })
  return {
    protocolVersion: "profile-submission-v1",
    sourceId: plan.sourceId,
    profileId: plan.profileId,
    runId: plan.runId,
    planVersion: plan.planVersion,
    batches,
    coverage: {
      validClaims: batches.length * perBatch,
      invalidClaims: 0,
      discoveryComplete: false,
      capacity: "within_limit",
    },
  }
}
function assemblyResponse(plan = planFixture(), context = aggregateFixture(plan)) {
  return {
    stage: { version: 2, kind: "assemble" },
    contextReceipt: receipt("context"),
    expiresAt: NOW + 600_000,
    context,
  }
}
function reconciliationResponse(focusClaimIds) {
  return {
    stage: { version: 2, kind: "reconcile" },
    focus: { claimIds: [...focusClaimIds] },
    operations: focusClaimIds.map((claimId) => ({
      claimId,
      target: "skills",
      entryId: "",
      field: "skills",
      action: "add",
      value: `Skill ${claimId}`,
      finding: "addition",
    })),
    outcomes: focusClaimIds.map((claimId) => ({
      claimId,
      kind: "change",
      reason: "validated_operation",
      relatedFacts: [],
      relatedClaimIds: [],
    })),
  }
}

// --- Harness ----------------------------------------------------------------

function harness(overrides = {}) {
  const calls = []
  const respond = async (kind, body, keys, signal) => {
    calls.push({ kind, body, keys, signal })
    if (overrides[kind]) return overrides[kind](body, keys, signal)
    if (kind === "plan") return planResponse()
    if (kind === "extract") return extractionResponse(body.batchIndex)
    if (kind === "assemble") return assemblyResponse()
    return reconciliationResponse(body.focus.claimIds)
  }
  const transport = {
    plan: (body, keys, signal) => respond("plan", body, keys, signal),
    extract: (body, keys, signal) => respond("extract", body, keys, signal),
    assemble: (body, keys, signal) => respond("assemble", body, keys, signal),
    reconcile: (body, keys, signal) => respond("reconcile", body, keys, signal),
  }
  return { calls, transport }
}

function makeTimeouts() {
  const entries = []
  return {
    entries,
    timeout: (ms) => {
      const controller = new AbortController()
      entries.push({ ms, controller })
      return controller.signal
    },
    abort(ms) {
      for (const entry of entries)
        if (entry.ms === ms)
          entry.controller.abort(new DOMException("Timeout", "TimeoutError"))
    },
  }
}

function controllerFor(transport, timeouts = makeTimeouts()) {
  return {
    timeouts,
    controller: submission.createSubmissionController({
      transport,
      runtime: { now: () => NOW, id: () => "sub-187", timeout: timeouts.timeout },
    }),
  }
}

const keys = { apiKey: "sk-user", typesafeKey: "typesafe-user" }
const expectedProfile = profileModule.withContactFields(baseProfile())

function withStorageSpies(run) {
  const spy = (counter) =>
    new Proxy(
      {},
      {
        get(target, property) {
          counter.n += 1
          return target[property]
        },
        set(target, property, value) {
          counter.n += 1
          target[property] = value
          return true
        },
        has(target, property) {
          counter.n += 1
          return property in target
        },
      },
    )
  const localCounter = { n: 0 }
  const sessionCounter = { n: 0 }
  const local = Object.getOwnPropertyDescriptor(globalThis, "localStorage")
  const session = Object.getOwnPropertyDescriptor(globalThis, "sessionStorage")
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: spy(localCounter),
  })
  Object.defineProperty(globalThis, "sessionStorage", {
    configurable: true,
    value: spy(sessionCounter),
  })
  const restore = () => {
    if (local) Object.defineProperty(globalThis, "localStorage", local)
    else delete globalThis.localStorage
    if (session) Object.defineProperty(globalThis, "sessionStorage", session)
    else delete globalThis.sessionStorage
  }
  return (async () => {
    try {
      return { value: await run(), counts: { local: localCounter.n, session: sessionCounter.n } }
    } finally {
      restore()
    }
  })()
}

// --- Tests ------------------------------------------------------------------

test("one start drives plan -> sequential batches -> assemble -> bounded focus -> review", async () => {
  const snapshot = makeSnapshot()
  const untouched = structuredClone(snapshot)
  const { calls, transport } = harness()
  const { controller } = controllerFor(transport)
  const stages = []
  const progress = []
  const { value: result, counts } = await withStorageSpies(() =>
    controller.start(snapshot, keys, {
      onStage: (stage) => stages.push(stage),
      onProgress: (event) => progress.push({ ...event }),
    }),
  )

  // Prescribed stage and call order: plan -> extract 0..2 -> assemble -> focus.
  assert.deepEqual(stages, ["plan", "extract", "extract", "extract", "assemble", "reconcile", "reconcile"])
  assert.deepEqual(
    calls.map((call) => call.kind),
    ["plan", "extract", "extract", "extract", "assemble", "reconcile", "reconcile"],
  )
  assert.deepEqual(
    calls.filter((call) => call.kind === "extract").map((call) => call.body.batchIndex),
    [0, 1, 2],
  )

  // Above the old per-batch aggregate ceilings: 3 batches x 15 = 45 claims,
  // split into bounded focus batches of at most 30.
  assert.equal(result.batches.length, 3)
  assert.equal(result.batches.reduce((total, batch) => total + batch.claims.length, 0), 45)
  assert.equal(result.focus.length, 2)
  assert.deepEqual(
    result.focus.map((focus) => focus.focus.claimIds.length),
    [30, 15],
  )
  assert.equal(result.plan.planVersion, submission.submissionPlanVersion)
  assert.equal(protocol.submissionResultError(result), "")

  // Exact pinned contexts on every envelope, plus the user credential identity.
  for (const call of calls) {
    assert.deepEqual(call.keys, keys)
    assert.equal(call.body.input, original)
    assert.deepEqual(call.body.profile, expectedProfile)
    assert.deepEqual(call.body.document, snapshot.document)
  }
  for (const call of calls.filter((item) => item.kind === "extract")) {
    assert.deepEqual(call.body.stage, { version: 2, kind: "extract" })
    assert.equal(call.body.planReceipt, receipt("plan"))
  }
  const assemble = calls.find((call) => call.kind === "assemble")
  assert.deepEqual(assemble.body.stage, { version: 2, kind: "assemble" })
  assert.deepEqual(assemble.body.batchReceipts, [receipt("batch0"), receipt("batch1"), receipt("batch2")])
  const reconciles = calls.filter((call) => call.kind === "reconcile")
  assert.deepEqual(reconciles.map((call) => call.body.focus.claimIds.length), [30, 15])
  for (const call of reconciles) {
    assert.deepEqual(call.body.stage, { version: 2, kind: "reconcile" })
    assert.equal(call.body.contextReceipt, receipt("context"))
  }

  // Progress distinguishes primary source coverage from returned-claim counts.
  assert.deepEqual(progress.at(-1), {
    status: "reviewing",
    stage: "reconcile",
    plannedBatches: 3,
    extractedBatches: 3,
    claims: 45,
    skippedClaims: 0,
    plannedFocusBatches: 2,
    reconciledFocusBatches: 2,
    discoveryComplete: false,
  })

  // No pre-save writes and no mutation of the pinned source on any path.
  assert.deepEqual(counts, { local: 0, session: 0 })
  assert.deepEqual(snapshot, untouched)
  assert.deepEqual(controller.getPin().keys, keys)
  assert.equal(controller.getPin().submissionId, "sub-187")
})

test("the validated result hands off to the review model without runtime errors", async () => {
  const snapshot = makeSnapshot()
  const { transport } = harness()
  const { controller } = controllerFor(transport)
  const result = await controller.start(snapshot, keys)
  const reviewSubmission = submission.toReviewSubmission(result)
  const plan = review.buildReviewPlan({ submission: reviewSubmission, snapshot: snapshot.document })
  assert.equal(review.reviewPlanError(plan), "")
  assert.equal(plan.sourceId, "source-187")
  assert.equal(plan.planVersion, "portions-v1")
  assert.equal(plan.units.length, 45)
  assert.deepEqual(
    [...new Set(plan.units.flatMap((unit) => unit.batchIndexes))].sort((a, b) => a - b),
    [0, 1, 2],
  )
  assert.ok(plan.units.every((unit) => unit.status === "pending"))
})

test("absolute 300 s, per-request 29 s and bounded-unit 65 s deadlines abort without rolling reset", async () => {
  // The absolute foreground cap is created once and never recreated per batch.
  {
    const snapshot = makeSnapshot()
    const { transport } = harness()
    const { controller, timeouts } = controllerFor(transport)
    await controller.start(snapshot, keys)
    assert.equal(timeouts.entries.filter((entry) => entry.ms === 300_000).length, 1)
    assert.equal(
      timeouts.entries.filter((entry) => entry.ms === 65_000).length,
      7,
      "one bounded unit per dispatch",
    )
  }
  // Absolute cap: exhausted during planning stops the queue.
  {
    const snapshot = makeSnapshot()
    const { calls, transport } = harness({
      plan: (_body, _keys, _signal) => {
        timeouts.abort(300_000)
        throw new Error("aborted")
      },
    })
    const { controller, timeouts } = controllerFor(transport)
    await assert.rejects(controller.start(snapshot, keys), { code: "timeout" })
    assert.deepEqual(calls.map((call) => call.kind), ["plan"])
    assert.equal(controller.getState().status, "failed")
  }
  // Per-request 29 s: exhausted on a later batch, no further dispatch.
  {
    const snapshot = makeSnapshot()
    const { calls, transport } = harness({
      extract: (body) => {
        if (body.batchIndex === 1) {
          timeouts.abort(29_000)
          throw new Error("aborted")
        }
        return extractionResponse(body.batchIndex)
      },
    })
    const { controller, timeouts } = controllerFor(transport)
    await assert.rejects(controller.start(snapshot, keys), { code: "timeout" })
    assert.deepEqual(calls.map((call) => call.kind), ["plan", "extract", "extract"])
    assert.equal(controller.getState().batches.length, 1)
  }
  // Bounded-unit 65 s: exhausted during assembly stops before focus dispatch.
  {
    const snapshot = makeSnapshot()
    const { calls, transport } = harness({
      assemble: () => {
        timeouts.abort(65_000)
        throw new Error("aborted")
      },
    })
    const { controller, timeouts } = controllerFor(transport)
    await assert.rejects(controller.start(snapshot, keys), { code: "timeout" })
    assert.deepEqual(calls.map((call) => call.kind), ["plan", "extract", "extract", "extract", "assemble"])
    assert.equal(controller.getState().batches.length, 3)
    assert.equal(controller.getState().focus.length, 0)
  }
})

test("a failure stops the queue and retains the completed validated partial state", async () => {
  const snapshot = makeSnapshot()
  const { calls, transport } = harness({
    extract: (body) =>
      body.batchIndex === 1
        ? (() => {
            const malformed = extractionResponse(1)
            delete malformed.unverifiedCount
            return malformed
          })()
        : extractionResponse(body.batchIndex),
  })
  const { controller } = controllerFor(transport)
  const { value: result, counts } = await withStorageSpies(async () => {
    await assert.rejects(controller.start(snapshot, keys), { code: "invalid_output" })
    return null
  })
  assert.equal(result, null)
  assert.deepEqual(counts, { local: 0, session: 0 })
  assert.deepEqual(calls.map((call) => call.kind), ["plan", "extract", "extract"])
  const state = controller.getState()
  assert.equal(state.status, "failed")
  assert.ok(state.plan, "the validated plan is retained")
  assert.equal(state.batches.length, 1, "only the completed validated batch is retained")
  assert.deepEqual(state.batchReceipts, [receipt("batch0")])
  assert.equal(state.context, null)
  assert.equal(state.error, "invalid_output")
})

test("response validation rejects malformed, mismatched and truncated results", async () => {
  const cases = [
    ["plan original bytes mismatch", { plan: () => planResponse(planFixture({ originalBytes: 1 })) }, "manifest_incomplete"],
    ["plan declares a different version", { plan: () => planResponse(planFixture({ planVersion: "portions-v2" })) }, "version"],
    ["plan declares a partial limit", { plan: () => planResponse(planFixture({ status: "insufficient_capacity", capacity: "portion_count", complete: false, remaining: [{ Start: 0, End: 1 }] })) }, "manifest_incomplete"],
    ["extraction coverage claims discovery", { extract: (body) => ({ ...extractionResponse(body.batchIndex), coverage: { ...extractionResponse(body.batchIndex).coverage, discoveryComplete: true } }) }, "invalid_output"],
    ["extraction batch index out of order", { extract: (body) => ({ ...extractionResponse(body.batchIndex), batchIndex: body.batchIndex + 5 }) }, "manifest_incomplete"],
    ["assembly identity mismatch", { assemble: () => assemblyResponse(planFixture(), { ...aggregateFixture(), sourceId: "other" }) }, "batch_mismatch"],
    ["assembly missing a batch", { assemble: () => assemblyResponse(planFixture(), { ...aggregateFixture(), batches: aggregateFixture().batches.slice(0, 2) }) }, "manifest_incomplete"],
    ["focus names an unknown claim", { reconcile: (body) => ({ ...reconciliationResponse(body.focus.claimIds), focus: { claimIds: [...body.focus.claimIds, "unknown/0/x"] } }) }, "focus_invalid"],
    ["operation escapes its focus", { reconcile: (body) => ({ ...reconciliationResponse(body.focus.claimIds), operations: [...reconciliationResponse(body.focus.claimIds).operations, { claimId: "unknown/0/x", target: "skills", entryId: "", field: "skills", action: "add", value: "x", finding: "addition" }] }) }, "invalid_output"],
    ["focus exceeds the operation cap", { reconcile: (body) => ({ ...reconciliationResponse(body.focus.claimIds), operations: Array.from({ length: 61 }, () => ({ claimId: body.focus.claimIds[0], target: "skills", entryId: "", field: "skills", action: "add", value: "x", finding: "addition" })) }) }, "capacity"],
  ]
  for (const [name, overrides, code] of cases) {
    const snapshot = makeSnapshot()
    const { transport } = harness(overrides)
    const { controller } = controllerFor(transport)
    await assert.rejects(controller.start(snapshot, keys), { code }, name)
  }
})

test("a changed pinned input, Profile or version invalidates the run instead of mixing", async () => {
  // Profile mutation detected between planning and extraction.
  {
    const snapshot = makeSnapshot()
    const { calls, transport } = harness({
      plan: () => {
        snapshot.profile.skills = "Go"
        return planResponse()
      },
    })
    const { controller } = controllerFor(transport)
    await assert.rejects(controller.start(snapshot, keys), { code: "stale" })
    assert.deepEqual(calls.map((call) => call.kind), ["plan"])
    assert.equal(controller.getState().status, "stale")
    assert.ok(controller.getState().plan)
  }
  // Original input mutation detected between planning and extraction.
  {
    const snapshot = makeSnapshot()
    const { calls, transport } = harness({
      plan: () => {
        snapshot.input = `${original} changed`
        return planResponse()
      },
    })
    const { controller } = controllerFor(transport)
    await assert.rejects(controller.start(snapshot, keys), { code: "stale" })
    assert.deepEqual(calls.map((call) => call.kind), ["plan"])
  }
})

test("a controller runs exactly one foreground submission", async () => {
  const snapshot = makeSnapshot()
  const { transport } = harness()
  const { controller } = controllerFor(transport)
  await controller.start(snapshot, keys)
  await assert.rejects(controller.start(makeSnapshot(), keys), { code: "state" })
})

test("the transport adapter POSTs v2 envelopes with the user keys and maps failures", async () => {
  const requests = []
  const transport = submission.createSubmissionTransport(async (url, options) => {
    requests.push({ url, options })
    return new Response(JSON.stringify({ ok: true }), { status: 200 })
  })
  const signal = new AbortController().signal
  const body = {
    stage: { version: 2, kind: "plan" },
    input: original,
    profile: baseProfile(),
    document: makeSnapshot().document,
  }
  await transport.plan(body, keys, signal)
  assert.equal(requests[0].url, "/api/profile/ingest")
  assert.equal(requests[0].options.method, "POST")
  assert.equal(requests[0].options.cache, "no-store")
  assert.equal(requests[0].options.headers["X-OpenAI-Api-Key"], "sk-user")
  assert.equal(requests[0].options.headers["X-TypeSafe-Api-Key"], "typesafe-user")
  assert.deepEqual(JSON.parse(requests[0].options.body), body)

  const failing = submission.createSubmissionTransport(async (_url, options) => {
    return new Response(JSON.stringify({ error: "capacity" }), { status: 400 })
  })
  await assert.rejects(failing.plan(body, keys, signal), { code: "capacity" })
  const gateway = submission.createSubmissionTransport(
    async () => new Response("<html>Inactivity Timeout</html>", { status: 504 }),
  )
  await assert.rejects(gateway.plan(body, keys, signal), { code: "timeout" })
})

test("the pin seam reports staleness for a changed snapshot", () => {
  const profile = baseProfile()
  const pin = {
    protocolVersion: "profile-submission-v1",
    planVersion: "portions-v1",
    submissionId: "sub-1",
    input: original,
    profile,
    document: undefined,
    keys,
    pinId: protocol.canonicalForm({
      input: original,
      profile: profileModule.withContactFields(profile),
      document: null,
    }),
  }
  assert.equal(submission.submissionPinError(pin, { input: original, profile }), "")
  assert.equal(submission.submissionPinError(pin, { input: `${original} changed`, profile }), "stale")
  assert.equal(
    submission.submissionPinError(pin, { input: original, profile, planVersion: "portions-v2" }),
    "stale",
  )
})

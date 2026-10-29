import { workflowModuleURL } from "./workflow-module.mjs"
// In-memory draft/recovery lifecycle evidence (#188). Transpiles the
// submission controller and drives it through a controlled transport to prove
// stop/pause/resume/retry/discard, retained handoff identity and expiry,
// staleness invalidation, save blocking, late-response/cross-tab/repeated-click
// races and the unmount-surviving session registry. It also asserts the
// outbound-call and no-storage invariants.
import test from "node:test"
import assert from "node:assert/strict"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import ts from "typescript"

const temp = mkdtempSync(join(tmpdir(), "ingestion-recovery-"))
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

const submission = await import(join(temp, "ingestionSubmission.mjs"))
const protocol = await import(join(temp, "ingestionProtocol.mjs"))
const review = await import(join(temp, "ingestionReview.mjs"))
const profileDocument = await import(join(temp, "profileDocument.mjs"))
const profileModule = await import(join(temp, "profile.mjs"))
const ingestion = await import(join(temp, "ingestion.mjs"))

// --- Fixtures ---------------------------------------------------------------

const NOW = 1_700_000_000_000
const RUN_ID = "run-188"
const RECEIPT_TAIL = "s".repeat(43)
const receipt = (label) => `${label}.${RECEIPT_TAIL}`
const segments = [
  "I use TypeScript.",
  "I built payment APIs.",
  "I lead a team.",
]
const original = segments.join(" ")
const perBatch = 15

const baseProfile = (overrides = {}) => ({
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
  ...overrides,
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
  document: profileDocument.migrateProfile(
    legacy({ skills: "React" }),
    "profile",
  ),
  ...overrides,
})

function planFixture(overrides = {}) {
  let offset = 0
  const batches = segments.map((segment, index) => {
    const end =
      index === segments.length - 1
        ? original.length
        : offset + segment.length + 1
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
    sourceId: "source-188",
    profileId: "profile-188",
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
const localClaimIds = Array.from(
  { length: perBatch },
  (_, index) => `c${index}`,
)
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
function assemblyResponse(
  plan = planFixture(),
  context = aggregateFixture(plan),
) {
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
function defaultResponse(kind, body) {
  if (kind === "plan") return planResponse()
  if (kind === "extract") return extractionResponse(body.batchIndex)
  if (kind === "assemble") return assemblyResponse()
  return reconciliationResponse(body.focus.claimIds)
}

// --- Harness ----------------------------------------------------------------

const tick = () => new Promise((resolve) => setTimeout(resolve, 0))
const deferred = () => {
  let resolve
  let reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
// A dispatch that never settles until the caller's signal aborts, standing in
// for a slow request or an in-progress body read.
const abortable = (signal) =>
  new Promise((_, reject) => {
    const fail = () =>
      reject(signal.reason ?? new DOMException("Aborted", "AbortError"))
    if (signal.aborted) return fail()
    signal.addEventListener("abort", fail, { once: true })
  })

function makeHarness() {
  const calls = []
  const handlers = {}
  const dispatch = (kind, body, keys, signal) => {
    calls.push({ kind, body, keys, signal })
    const handler = handlers[kind]
    if (handler)
      return Promise.resolve().then(() => handler(body, keys, signal))
    return Promise.resolve(defaultResponse(kind, body))
  }
  const transport = {
    plan: (body, keys, signal) => dispatch("plan", body, keys, signal),
    extract: (body, keys, signal) => dispatch("extract", body, keys, signal),
    assemble: (body, keys, signal) => dispatch("assemble", body, keys, signal),
    reconcile: (body, keys, signal) =>
      dispatch("reconcile", body, keys, signal),
  }
  return { calls, handlers, transport }
}

function controllerFor(transport, clock) {
  return submission.createSubmissionController({
    transport,
    runtime: {
      now: () => clock.now,
      id: () => "sub-188",
      timeout: () => new AbortController().signal,
    },
  })
}

const keys = { apiKey: "sk-user", typesafeKey: "typesafe-user" }
const expectedProfile = profileModule.withContactFields(baseProfile())
const kinds = (calls) => calls.map((call) => call.kind)

function withStorageSpies(run) {
  const spy = (counter) =>
    new Proxy({}, {
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
    })
  const localCounter = { n: 0 }
  const sessionCounter = { n: 0 }
  const fetchCounter = { n: 0 }
  const local = Object.getOwnPropertyDescriptor(globalThis, "localStorage")
  const session = Object.getOwnPropertyDescriptor(globalThis, "sessionStorage")
  const fetchImpl = globalThis.fetch
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: spy(localCounter),
  })
  Object.defineProperty(globalThis, "sessionStorage", {
    configurable: true,
    value: spy(sessionCounter),
  })
  globalThis.fetch = (...args) => {
    fetchCounter.n += 1
    return fetchImpl(...args)
  }
  const restore = () => {
    if (local) Object.defineProperty(globalThis, "localStorage", local)
    else delete globalThis.localStorage
    if (session) Object.defineProperty(globalThis, "sessionStorage", session)
    else delete globalThis.sessionStorage
    globalThis.fetch = fetchImpl
  }
  return (async () => {
    try {
      return {
        value: await run(),
        counts: {
          local: localCounter.n,
          session: sessionCounter.n,
          fetch: fetchCounter.n,
        },
      }
    } finally {
      restore()
    }
  })()
}

// --- Stops during every stage -----------------------------------------------

test("stop aborts the active request in every stage and dispatches nothing further", async () => {
  const cases = [
    {
      name: "plan",
      block: (kind) => kind === "plan",
      kinds: ["plan"],
      after: (state) => assert.equal(state.plan, null),
    },
    {
      name: "extract",
      block: (kind, body) => kind === "extract" && body.batchIndex === 0,
      kinds: ["plan", "extract"],
      after: (state) => {
        assert.ok(state.plan, "the completed plan is retained")
        assert.equal(state.batches.length, 0)
      },
    },
    {
      name: "assemble",
      block: (kind) => kind === "assemble",
      kinds: ["plan", "extract", "extract", "extract", "assemble"],
      after: (state) => {
        assert.equal(state.batches.length, 3)
        assert.equal(state.context, null)
      },
    },
    {
      name: "reconcile",
      block: (kind) => kind === "reconcile",
      kinds: ["plan", "extract", "extract", "extract", "assemble", "reconcile"],
      after: (state) => {
        assert.ok(state.context, "the completed context is retained")
        assert.equal(state.focus.length, 0)
      },
    },
  ]
  for (const item of cases) {
    const { calls, handlers, transport } = makeHarness()
    const clock = { now: NOW }
    const controller = controllerFor(transport, clock)
    handlers.plan = (body, _keys, signal) =>
      item.block("plan", body)
        ? abortable(signal)
        : defaultResponse("plan", body)
    handlers.extract = (body, _keys, signal) =>
      item.block("extract", body)
        ? abortable(signal)
        : defaultResponse("extract", body)
    handlers.assemble = (body, _keys, signal) =>
      item.block("assemble", body)
        ? abortable(signal)
        : defaultResponse("assemble", body)
    handlers.reconcile = (body, _keys, signal) =>
      item.block("reconcile", body)
        ? abortable(signal)
        : defaultResponse("reconcile", body)

    const started = controller.start(makeSnapshot(), keys)
    await tick()
    const inFlight = controller.getState().requestId
    assert.ok(inFlight, `${item.name}: a request identity is assigned`)
    controller.stop()

    await assert.rejects(started, { code: "cancelled" }, item.name)
    const dispatched = calls.length
    await tick()
    assert.equal(
      calls.length,
      dispatched,
      `${item.name}: no dispatch after stop`,
    )
    assert.deepEqual(kinds(calls), item.kinds, item.name)
    const state = controller.getState()
    assert.equal(state.status, "stopped", item.name)
    assert.equal(state.reason, "stopped", item.name)
    assert.equal(
      state.input,
      original,
      `${item.name}: the draft text is retained`,
    )
    assert.ok(
      state.cancelledRequestIds.includes(inFlight),
      `${item.name}: the in-flight handoff is invalidated`,
    )
    assert.ok(
      !state.handoffs.some(
        (handoff) => handoff.requestId === inFlight && handoff.valid,
      ),
    )
    item.after(state)

    // The stopped attempt dispatches nothing further even if its response
    // arrives late.
    await tick()
    assert.equal(
      calls.length,
      dispatched,
      `${item.name}: a late response dispatches nothing`,
    )
  }
})

test("stop during a request body read aborts the read and blocks further dispatch", async () => {
  const bodyGate = deferred()
  const fetches = []
  const transport = submission.createSubmissionTransport(
    async (url, options) => {
      fetches.push({ url, options })
      return { ok: true, status: 200, json: () => bodyGate.promise }
    },
  )
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  const started = controller.start(makeSnapshot(), keys)
  await tick()
  assert.equal(fetches.length, 1, "the plan request was dispatched")
  controller.stop()
  bodyGate.resolve(planResponse())
  await assert.rejects(started, { code: "cancelled" })
  assert.equal(fetches.length, 1, "no extraction was dispatched")
  const state = controller.getState()
  assert.equal(state.status, "stopped")
  assert.equal(state.input, original)
  assert.equal(state.plan, null)
  assert.equal(
    state.handoffs.length,
    0,
    "the aborted handoff is never recorded",
  )
})

test("a late response for a stopped request is never recorded and cannot overwrite newer text", async () => {
  const gate = deferred()
  const { calls, handlers, transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  handlers.plan = () => gate.promise
  const started = controller.start(makeSnapshot(), keys)
  await tick()
  controller.stop()
  assert.equal(controller.getState().status, "stopped")
  // Newer text replaces the retained draft before the late response lands.
  assert.equal(controller.setDraft("a newer draft"), "stale")
  gate.resolve(planResponse())
  await assert.rejects(started, { code: "cancelled" })
  const state = controller.getState()
  assert.equal(state.status, "stale")
  assert.equal(state.plan, null, "the late plan is discarded")
  assert.equal(state.handoffs.length, 0)
  assert.equal(state.input, "a newer draft", "newer text is not overwritten")
  assert.deepEqual(kinds(calls), ["plan"])
})

// --- Resume and comparison-only retry ---------------------------------------

test("resume after stop processes only unfinished work and never re-plans", async () => {
  const { calls, handlers, transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  handlers.extract = (body, _keys, signal) =>
    body.batchIndex === 2 ? abortable(signal) : defaultResponse("extract", body)
  const started = controller.start(makeSnapshot(), keys)
  await tick()
  await tick()
  await tick()
  controller.stop()
  await assert.rejects(started, { code: "cancelled" })
  assert.deepEqual(kinds(calls), ["plan", "extract", "extract", "extract"])
  assert.equal(controller.getState().batches.length, 2)

  delete handlers.extract
  const before = calls.length
  await controller.resume()
  assert.deepEqual(kinds(calls.slice(before)), [
    "extract",
    "assemble",
    "reconcile",
    "reconcile",
  ])
  assert.equal(controller.getState().status, "reviewing")
  assert.ok(controller.getResult())
})

test("a comparison-only retry reuses valid extraction/context with one reconcile call", async () => {
  const { calls, handlers, transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  handlers.reconcile = (body) =>
    body.focus.claimIds.length === 15
      ? Promise.reject(new ingestion.IngestionError("outage"))
      : defaultResponse("reconcile", body)
  await assert.rejects(controller.start(makeSnapshot(), keys), {
    code: "outage",
  })
  assert.equal(controller.getState().status, "failed")
  assert.equal(controller.getState().reason, "failed")
  assert.equal(controller.getState().focus.length, 1)
  const before = calls.length
  assert.deepEqual(kinds(calls), [
    "plan",
    "extract",
    "extract",
    "extract",
    "assemble",
    "reconcile",
    "reconcile",
  ])

  delete handlers.reconcile
  const failedFocus = calls.at(-1).body.focus.claimIds
  const result = await controller.retry()
  // No new plan, extraction or assembly: only the outstanding comparison.
  assert.deepEqual(kinds(calls.slice(before)), ["reconcile"])
  assert.equal(calls.at(-1).body.contextReceipt, receipt("context"))
  assert.deepEqual(
    calls.at(-1).body.focus.claimIds,
    failedFocus,
    "the same focus portion is retried",
  )
  assert.equal(result.focus.length, 2)
  assert.equal(controller.getState().status, "reviewing")
  assert.equal(protocol.submissionResultError(result), "")
})

test("resume with a changed snapshot stops before dispatch", async () => {
  const { calls, handlers, transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  handlers.extract = (body, _keys, signal) =>
    body.batchIndex === 1 ? abortable(signal) : defaultResponse("extract", body)
  const started = controller.start(makeSnapshot(), keys)
  await tick()
  await tick()
  controller.stop()
  await assert.rejects(started, { code: "cancelled" })
  const before = calls.length
  await assert.rejects(
    controller.resume({
      candidate: { input: original, profile: baseProfile({ skills: "Go" }) },
    }),
    { code: "stale" },
  )
  assert.equal(
    calls.length,
    before,
    "a changed Profile stops before any dispatch",
  )
  assert.equal(controller.getState().status, "stale")
})

// --- Expiry ----------------------------------------------------------------

test("an expired retained handoff stops with an explicit restart requirement and zero fallback", async () => {
  const { calls, handlers, transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  handlers.reconcile = (body) =>
    body.focus.claimIds.length === 15
      ? Promise.reject(new Error("boom"))
      : defaultResponse("reconcile", body)
  await assert.rejects(controller.start(makeSnapshot(), keys), {
    code: "outage",
  })
  // Time passes beyond the ten-minute receipt lifetime.
  clock.now = NOW + 600_001
  const before = calls.length
  await assert.rejects(controller.retry(), { code: "receipt_expired" })
  assert.equal(calls.length, before, "no provider fallback after expiry")
  const state = controller.getState()
  assert.equal(state.status, "expired")
  assert.equal(state.reason, "expired")
  assert.equal(state.restartRequired, true)
  assert.ok(state.handoffs.every((handoff) => !handoff.valid))
  // The explicit restart requirement refuses a further resume.
  await assert.rejects(controller.resume(), { code: "receipt_expired" })
  // The retained partial work is still visible for recovery.
  assert.ok(state.plan)
  assert.equal(state.error, "receipt_expired")
})

test("an expired/invalid handoff error from the transport requires a restart without fallback", async () => {
  const { calls, handlers, transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  handlers.plan = () =>
    Promise.reject(new ingestion.IngestionError("receipt_invalid"))
  await assert.rejects(controller.start(makeSnapshot(), keys), {
    code: "receipt_invalid",
  })
  assert.deepEqual(
    kinds(calls),
    ["plan"],
    "no extraction fallback is dispatched",
  )
  const state = controller.getState()
  assert.equal(state.status, "expired")
  assert.equal(state.reason, "expired")
  assert.equal(state.restartRequired, true)
})

test("the recovery reason distinguishes stopped, timed out, failed, expired, stale and discarded", async () => {
  // A transport timeout is a `failed` status with a distinct `timed_out` reason.
  {
    const { handlers, transport } = makeHarness()
    const controller = controllerFor(transport, { now: NOW })
    handlers.plan = () =>
      Promise.reject(new ingestion.IngestionError("timeout"))
    await assert.rejects(controller.start(makeSnapshot(), keys), {
      code: "timeout",
    })
    assert.equal(controller.getState().status, "failed")
    assert.equal(controller.getState().reason, "timed_out")
  }
  // A plain outage stays `failed`, not `timed_out`.
  {
    const { handlers, transport } = makeHarness()
    const controller = controllerFor(transport, { now: NOW })
    handlers.plan = () => Promise.reject(new ingestion.IngestionError("outage"))
    await assert.rejects(controller.start(makeSnapshot(), keys), {
      code: "outage",
    })
    assert.equal(controller.getState().status, "failed")
    assert.equal(controller.getState().reason, "failed")
  }
  // Pause and discard are distinct recoveries.
  {
    const { handlers, transport } = makeHarness()
    const controller = controllerFor(transport, { now: NOW })
    handlers.plan = (_body, _keys, signal) => abortable(signal)
    const started = controller.start(makeSnapshot(), keys)
    await tick()
    controller.pause()
    await assert.rejects(started, { code: "cancelled" })
    assert.equal(controller.getState().status, "paused")
    assert.equal(controller.getState().reason, "paused")
    controller.discard()
    assert.equal(controller.getState().status, "discarded")
    assert.equal(controller.getState().reason, "discarded")
  }
  // A changed identity is `stale`.
  {
    const { transport } = makeHarness()
    const controller = controllerFor(transport, { now: NOW })
    await controller.start(makeSnapshot(), keys)
    assert.equal(
      controller.refresh({
        input: original,
        profile: baseProfile({ skills: "Go" }),
      }),
      "stale",
    )
    assert.equal(controller.getState().status, "stale")
    assert.equal(controller.getState().reason, "stale")
  }
})

test("a completed local review survives receipt expiry", async () => {
  const { transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  const snapshot = makeSnapshot()
  const result = await controller.start(snapshot, keys)
  clock.now = NOW + 600_001
  const state = controller.getState()
  assert.equal(state.status, "reviewing")
  assert.equal(state.applicable, true)
  assert.equal(controller.getResult(), result)
  // Identity is unaffected by time; a same-identity refresh does not expire it.
  assert.equal(
    controller.refresh({
      input: original,
      profile: baseProfile(),
      document: snapshot.document,
    }),
    "",
  )
  assert.equal(controller.getState().status, "reviewing")
})

// --- Staleness and save blocking -------------------------------------------

test("input, Profile, document, credential and protocol changes invalidate and block saves", async () => {
  const cases = [
    [
      "same identity",
      (snapshot) => ({
        input: snapshot.input,
        profile: baseProfile(),
        document: snapshot.document,
      }),
      "",
    ],
    [
      "changed input",
      (snapshot) => ({
        input: `${original} changed`,
        profile: baseProfile(),
        document: snapshot.document,
      }),
      "stale",
    ],
    [
      "changed Profile",
      (snapshot) => ({
        input: original,
        profile: baseProfile({ skills: "Go" }),
        document: snapshot.document,
      }),
      "stale",
    ],
    [
      "changed document",
      (snapshot) => ({
        input: original,
        profile: baseProfile(),
        document: {
          ...snapshot.document,
          revision: snapshot.document.revision + 1,
        },
      }),
      "stale",
    ],
    [
      "changed credential",
      (snapshot) => ({
        input: original,
        profile: baseProfile(),
        document: snapshot.document,
        keys: { apiKey: "other", typesafeKey: "typesafe-user" },
      }),
      "stale",
    ],
    [
      "changed protocol",
      (snapshot) => ({
        input: original,
        profile: baseProfile(),
        document: snapshot.document,
        protocolVersion: "profile-submission-v2",
      }),
      "stale",
    ],
  ]
  for (const [name, candidate, expected] of cases) {
    const { transport } = makeHarness()
    const clock = { now: NOW }
    const controller = controllerFor(transport, clock)
    const snapshot = makeSnapshot()
    await controller.start(snapshot, keys)
    const plan = controller.getReviewPlan()
    const selectable = plan.units.find((unit) => unit.selectable)
    if (selectable) controller.selectUnit(selectable.id, "selected")
    assert.equal(
      controller.canSave().allowed,
      true,
      `${name}: a fresh review may save`,
    )
    assert.equal(controller.refresh(candidate(snapshot)), expected, name)
    if (expected !== "") {
      const state = controller.getState()
      assert.equal(state.stale, true, name)
      assert.equal(
        state.applicable,
        false,
        `${name}: stale changes are not applicable`,
      )
      assert.equal(state.restartRequired, true, name)
      assert.equal(
        controller.getResult(),
        null,
        `${name}: stale result is withheld`,
      )
      assert.equal(controller.getReviewPlan(), null, name)
      assert.equal(controller.canSave().allowed, false, name)
      assert.equal(controller.canSave().code, "stale", name)
      assert.equal(
        state.input,
        original,
        `${name}: the recoverable text is retained`,
      )
    } else {
      assert.equal(controller.getState().applicable, true, name)
      assert.equal(controller.canSave().allowed, true, name)
    }
  }
})

test("changing the draft text invalidates receipts and result but retains the new text", async () => {
  const { transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  await controller.start(makeSnapshot(), keys)
  assert.equal(controller.setDraft(original), "", "an unchanged draft is inert")
  assert.equal(controller.setDraft("a different paste"), "stale")
  const state = controller.getState()
  assert.equal(state.input, "a different paste")
  assert.equal(state.stale, true)
  assert.equal(state.applicable, false)
  assert.equal(controller.getResult(), null)
  assert.equal(controller.canSave().allowed, false)
})

test("a delayed save is rejected once the selection changed", async () => {
  const { transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  await controller.start(makeSnapshot(), keys)
  const units = controller
    .getReviewPlan()
    .units.filter((unit) => unit.selectable)
  assert.ok(units.length >= 2, "the fixture exposes selectable units")
  controller.selectUnit(units[0].id, "selected")
  const guard = controller.saveGuard()
  assert.equal(controller.canSave(guard).allowed, true)
  controller.selectUnit(units[1].id, "selected")
  const delayed = controller.canSave(guard)
  assert.equal(
    delayed.allowed,
    false,
    "the delayed save cannot overwrite newer selection",
  )
  assert.equal(delayed.code, "stale")
  assert.equal(
    controller.canSave().allowed,
    true,
    "a fresh save is still allowed",
  )
  const session = controller.sessionState()
  assert.equal(protocol.submissionSessionError(session), "")
  assert.equal(
    session.selections.filter((item) => item.status === "selected").length,
    2,
  )
})

// --- Races -----------------------------------------------------------------

test("cross-tab staleness aborts the in-flight request and prevents stale work", async () => {
  const gate = deferred()
  const { calls, handlers, transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  handlers.plan = () => gate.promise
  const started = controller.start(makeSnapshot(), keys)
  await tick()
  assert.equal(
    controller.refresh({
      input: original,
      profile: baseProfile({ skills: "Go" }),
    }),
    "stale",
  )
  gate.resolve(planResponse())
  await assert.rejects(started, { code: "cancelled" })
  const state = controller.getState()
  assert.equal(state.status, "stale")
  assert.equal(state.applicable, false)
  assert.equal(state.plan, null)
  assert.deepEqual(
    kinds(calls),
    ["plan"],
    "a cross-tab change dispatches nothing further",
  )
})

test("repeated clicks cannot start or resume a second attempt", async () => {
  const { handlers, transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  handlers.extract = (_body, _keys, signal) => abortable(signal)
  const started = controller.start(makeSnapshot(), keys)
  await tick()
  await assert.rejects(controller.start(makeSnapshot(), keys), {
    code: "state",
  })
  await assert.rejects(controller.resume(), { code: "state" })
  await assert.rejects(controller.retry(), { code: "state" })
  assert.throws(() => controller.selectUnit("unit:x", "selected"), {
    code: "state",
  })
  controller.stop()
  await assert.rejects(started, { code: "cancelled" })
})

// --- Pause, discard and unmount survival ------------------------------------

test("pause is distinct from stop and retains the draft for an in-app return", async () => {
  const { calls, handlers, transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  handlers.extract = (body, _keys, signal) =>
    body.batchIndex === 1 ? abortable(signal) : defaultResponse("extract", body)
  const started = controller.start(makeSnapshot(), keys)
  await tick()
  await tick()
  controller.pause()
  await assert.rejects(started, { code: "cancelled" })
  const paused = controller.getState()
  assert.equal(paused.status, "paused")
  assert.equal(paused.reason, "paused")
  assert.equal(paused.input, original)
  assert.ok(paused.plan)
  assert.equal(paused.batches.length, 1)
  assert.ok(
    paused.cancelledRequestIds.length > 0,
    "pause invalidates the in-flight handoff",
  )
  // Pause cannot start a new session; it resumes only unfinished work.
  await assert.rejects(controller.start(makeSnapshot(), keys), {
    code: "state",
  })
  delete handlers.extract
  const before = calls.length
  await controller.resume()
  assert.deepEqual(kinds(calls.slice(before)), [
    "extract",
    "extract",
    "assemble",
    "reconcile",
    "reconcile",
  ])
  assert.equal(controller.getState().status, "reviewing")
})

test("discard drops the draft while stop retains it", async () => {
  const { handlers, transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  handlers.extract = (_body, _keys, signal) => abortable(signal)
  const started = controller.start(makeSnapshot(), keys)
  await tick()
  controller.stop()
  await assert.rejects(started, { code: "cancelled" })
  assert.equal(controller.getState().input, original)

  controller.discard()
  const discarded = controller.getState()
  assert.equal(discarded.status, "discarded")
  assert.equal(discarded.reason, "discarded")
  assert.equal(discarded.input, "")
  assert.equal(discarded.plan, null)
  assert.equal(controller.getResult(), null)
  assert.equal(controller.canSave().allowed, false)
  assert.equal(controller.unsaved(), false)
  // A discarded session may be explicitly restarted with fresh text.
  delete handlers.extract
  await controller.start(makeSnapshot(), keys)
  assert.equal(controller.getState().status, "reviewing")
  assert.equal(
    controller.unsaved(),
    true,
    "a completed review is unsaved work the UI warns about",
  )
})

test("the session owner survives an internal unmount/remount without duplicating", async () => {
  const { transport } = makeHarness()
  const clock = { now: NOW }
  const options = {
    transport,
    runtime: {
      now: () => clock.now,
      id: () => "sub-188",
      timeout: () => new AbortController().signal,
    },
  }
  const mounted = submission.submissionSessionOwner(
    "repository:test-remount",
    options,
  )
  await mounted.start(makeSnapshot(), keys)
  const selected = mounted.getReviewPlan().units.find((unit) => unit.selectable)
  mounted.selectUnit(selected.id, "selected")

  // A component-level unmount/remount re-reads the same owner.
  const remounted = submission.submissionSessionOwner(
    "repository:test-remount",
    options,
  )
  assert.equal(remounted, mounted)
  assert.equal(submission.submissionSession("repository:test-remount"), mounted)
  const state = remounted.getState()
  assert.equal(state.status, "reviewing")
  assert.equal(state.input, original)
  assert.equal(state.submissionId, mounted.getPin().submissionId)
  assert.ok(
    remounted.getReviewPlan().units.some((unit) => unit.status === "selected"),
    "reviewed edits survive",
  )
  assert.equal(remounted.getState().selectionRevision, 1)
  submission.releaseSubmissionSession("repository:test-remount")
  assert.equal(
    submission.submissionSession("repository:test-remount"),
    undefined,
  )
})

test("an unmount/remount during a paused run resumes only the unfinished work", async () => {
  const { handlers, transport } = makeHarness()
  const clock = { now: NOW }
  const options = {
    transport,
    runtime: {
      now: () => clock.now,
      id: () => "sub-188",
      timeout: () => new AbortController().signal,
    },
  }
  handlers.extract = (body, _keys, signal) =>
    body.batchIndex === 1 ? abortable(signal) : defaultResponse("extract", body)
  const owner = submission.submissionSessionOwner(
    "repository:test-paused-remount",
    options,
  )
  const started = owner.start(makeSnapshot(), keys)
  await tick()
  owner.pause()
  await assert.rejects(started, { code: "cancelled" })
  assert.equal(owner.getState().status, "paused")

  delete handlers.extract
  const remounted = submission.submissionSessionOwner(
    "repository:test-paused-remount",
    options,
  )
  assert.equal(remounted, owner)
  assert.equal(remounted.getState().input, original)
  await remounted.resume()
  assert.equal(remounted.getState().status, "reviewing")
  submission.releaseSubmissionSession("repository:test-paused-remount")
})

// --- Invariants -------------------------------------------------------------

test("the whole lifecycle performs no storage or server write", async () => {
  const { counts } = await withStorageSpies(async () => {
    const { handlers, transport } = makeHarness()
    const clock = { now: NOW }
    const controller = controllerFor(transport, clock)
    const snapshot = makeSnapshot()
    handlers.reconcile = (body) =>
      body.focus.claimIds.length === 15
        ? Promise.reject(new Error("boom"))
        : defaultResponse("reconcile", body)
    await assert.rejects(controller.start(snapshot, keys), { code: "outage" })
    controller.pause()
    delete handlers.reconcile
    await controller.retry()
    const unit = controller
      .getReviewPlan()
      .units.find((candidate) => candidate.selectable)
    controller.selectUnit(unit.id, "selected")
    controller.saveGuard()
    controller.canSave()
    controller.setDraft("a changed draft")
    controller.refresh({
      input: original,
      profile: baseProfile({ skills: "Go" }),
    })
    controller.discard()
    submission.submissionSessionOwner("repository:test-storage", {
      transport,
      runtime: {
        now: () => clock.now,
        id: () => "sub-188",
        timeout: () => new AbortController().signal,
      },
    })
    return null
  })
  assert.deepEqual(counts, { local: 0, session: 0, fetch: 0 })
})

test("every outbound envelope carries only the pinned context and stage frame", async () => {
  const { calls, transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  const snapshot = makeSnapshot()
  const base = ["stage", "input", "profile", "document"]
  const expectedFields = {
    plan: base,
    extract: [...base, "planReceipt", "batchIndex"],
    assemble: [...base, "planReceipt", "batchReceipts"],
    reconcile: [...base, "contextReceipt", "focus"],
  }
  await controller.start(snapshot, keys)
  for (const call of calls) {
    assert.deepEqual(
      Object.keys(call.body).sort(),
      [...expectedFields[call.kind]].sort(),
      call.kind,
    )
    assert.deepEqual(call.keys, keys)
    assert.equal(call.body.input, original)
    assert.deepEqual(call.body.profile, expectedProfile)
    assert.deepEqual(call.body.document, snapshot.document)
  }
})

test("the session projection reports typed lifecycle statuses without weakening the contract", async () => {
  const { handlers, transport } = makeHarness()
  const clock = { now: NOW }
  const controller = controllerFor(transport, clock)
  assert.equal(controller.getState().status, "idle")
  handlers.extract = (_body, _keys, signal) => abortable(signal)
  const started = controller.start(makeSnapshot(), keys)
  await tick()
  assert.equal(protocol.submissionStatusError(controller.getState().status), "")
  controller.stop()
  await assert.rejects(started, { code: "cancelled" })
  const stopped = controller.sessionState()
  assert.equal(stopped.status, "stopped")
  assert.equal(protocol.submissionSessionError(stopped), "")
  controller.pause()
  const paused = controller.sessionState()
  assert.equal(paused.status, "stopped", "pause after stop is inert")
  assert.equal(protocol.submissionSessionError(paused), "")
})

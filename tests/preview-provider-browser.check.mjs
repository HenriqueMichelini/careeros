import assert from "node:assert/strict"
import { createHash } from "node:crypto"
import { readFileSync, writeFileSync } from "node:fs"
import { inspectionBrowserFixture } from "./inspection-browser-fixture.mjs"
import {
  createPreviewProviderRunner,
  createProviderWorkflowBudget,
} from "./preview-inspection-harness.mjs"

const browser = await inspectionBrowserFixture()
const digest = (raw) => createHash("sha256").update(raw).digest("hex")
const keys = {
  "X-OpenAI-Api-Key": "controlled-openai-placeholder",
  "X-TypeSafe-Api-Key": "controlled-typesafe-placeholder",
}
const qualificationStages = [
  "field_validation",
  "qualification_matching",
  "requirement_evidence",
]
const draftStages = [
  "field_validation",
  "generation",
  "resume_support",
  "artifact_support",
]
function approval(draft = false) {
  const requestBody = JSON.stringify({
    jobPosting: "New held-out synthetic posting. Português: relatório interno.",
    understandJob: true,
  })
  const value = {
    candidateSHA: "c154f40ca04e8ef88bb0b917b89603b586797d0f",
    deployID: "6acecf58423a9ea41a9137f6",
    origin: browser.origin,
    deployment: {
      id: "6acecf58423a9ea41a9137f6",
      commitRef: "c154f40ca04e8ef88bb0b917b89603b586797d0f",
      url: browser.origin,
      deployURL: browser.origin,
    },
    documentURL: browser.origin + "/app",
    endpoint:
      browser.origin +
      (draft ? "/api/application-draft" : "/api/qualification-gaps"),
    caseID: draft ? "controlled-paid-draft" : "controlled-paid-qualification",
    expiresAt: new Date(Date.now() + 30 * 60 * 1000).toISOString(),
    requestBody,
    requestSHA256: digest(requestBody),
    dispatch: true,
    expectedStatus: null,
    stages: draft ? draftStages : qualificationStages,
    batchID: "controlled-closed-batch",
    providerDispatchLimit: draft ? 4 : 3,
  }
  value.deterministicEnvelopes = output(value)
    .releaseCapture.envelopes.slice(0, 2)
    .map(({ stage, requestBytes, requestSHA256 }) => ({
      stage,
      requestBytes,
      requestSHA256,
    }))
  return value
}
const budgetFor = (a) =>
  createProviderWorkflowBudget({
    id: a.batchID,
    cases: [{ id: a.caseID, maxDispatches: a.stages.length }],
  })
const readDeployment = async () => ({
  ...approval().deployment,
  context: "deploy-preview",
  observedAt: new Date().toISOString(),
})
function output(a) {
  return {
    releaseCapture: {
      version: "release-capture-v1",
      caseId: a.caseID,
      requestSHA256: a.requestSHA256,
      outcome: "complete",
      envelopes: a.stages.map((stage, index) => {
        const body = JSON.stringify(
          index === 0
            ? {
                model: "jev-1.13.0",
                state: {
                  field: "job_posting",
                  submission: JSON.parse(a.requestBody).jobPosting,
                },
                questions: [],
              }
            : {
                model: "gpt-6-luna",
                reasoning_effort: "none",
                messages: [{ role: "user", content: "controlled request" }],
              },
        )
        const response = "{}"
        return {
          stage,
          outcome: "captured",
          admission: "release-egress-v1",
          endpoint:
            index === 0
              ? "https://api.typesafe.ai/v1/systemone"
              : "https://api.openai.com/v1/chat/completions",
          requestBody: body,
          requestEncoding: "utf8",
          requestBytes: Buffer.byteLength(body),
          requestSHA256: digest(body),
          responseBody: response,
          responseEncoding: "utf8",
          responseBytes: Buffer.byteLength(response),
          responseSHA256: digest(response),
          httpStatus: 200,
          durationMs: 1,
        }
      }),
    },
  }
}
const results = []
async function scenario(
  name,
  {
    draft = false,
    transform = (x) => x,
    status = 200,
    redirect = false,
    setup,
    check,
  },
) {
  const a = approval(draft),
    budget = budgetFor(a)
  const frozenServerApproval = structuredClone(a)
  await browser.navigate(browser.origin + "/app")
  browser.respond((_request, res) => {
    res.writeHead(redirect ? 302 : status, {
      "Content-Type": "application/json",
      "Cache-Control": "no-store",
      ...(redirect ? { Location: browser.authOrigin + "/auth" } : {}),
    })
    res.end(JSON.stringify(transform(output(frozenServerApproval))))
  })
  const before = browser.requests.filter((r) => r.method === "POST").length
  const runner = await createPreviewProviderRunner(
    { ...browser, readDeployment },
    a,
    { keys, budget },
  )
  try {
    assert.equal(await runner.prepare(), true)
    await setup?.(a)
    const evidence = await runner.inspect()
    const posts =
      browser.requests.filter((r) => r.method === "POST").length - before
    await check(evidence, posts, budget)
    await assert.rejects(() => runner.inspect(), /single-use/)
    assert.equal(
      browser.requests.filter((r) => r.auth && r.method === "POST").length,
      0,
    )
    results.push({
      name,
      outcome: evidence.outcome,
      reason: evidence.reason,
      posts,
      captureOutcome: evidence.captureOutcome ?? null,
      ...budget.snapshot(),
    })
  } finally {
    await runner.close()
  }
}
try {
  for (const draft of [false, true])
    await scenario(
      draft ? "four-stage draft capture" : "three-stage qualification capture",
      {
        draft,
        check: (e, n, b) => {
          assert.equal(e.outcome, "provider_capture_verified")
          assert.equal(e.captureOutcome, "complete")
          assert.equal(n, 1)
          assert.equal(e.providerEnvelopes.length, draft ? 4 : 3)
          assert.equal(b.snapshot().reservedProviderDispatches, draft ? 4 : 3)
          assert.equal(e.response.status, 200)
        },
      },
    )
  for (const failure of ["transport_failure", "authentication_failure"])
    await scenario("partial " + failure, {
      status: 502,
      transform: (o) => {
        o.releaseCapture.outcome = "incomplete"
        o.releaseCapture.envelopes.length = 1
        const e = o.releaseCapture.envelopes[0]
        e.outcome = failure
        e.httpStatus = failure === "transport_failure" ? null : 401
        e.responseBytes = null
        delete e.responseBody
        delete e.responseSHA256
        delete e.responseEncoding
        return o
      },
      check: (e, n) => {
        assert.equal(e.outcome, "provider_capture_verified")
        assert.equal(e.captureOutcome, "incomplete")
        assert.equal(e.response.status, 502)
        assert.equal(n, 1)
      },
    })
  await scenario("missing capture is not accepted", {
    status: 404,
    transform: () => ({ error: "not found" }),
    check: (e, n) => {
      assert.equal(e.reason, "missing_or_invalid_capture")
      assert.equal(n, 1)
    },
  })
  await scenario("tampered serialized response hash", {
    transform: (o) => {
      o.releaseCapture.envelopes[1].responseSHA256 = "0".repeat(64)
      return o
    },
    check: (e) => assert.equal(e.reason, "missing_or_invalid_capture"),
  })
  await scenario("changed original Jev input", {
    transform: (o) => {
      const e = o.releaseCapture.envelopes[0]
      const body = JSON.parse(e.requestBody)
      body.state.submission = "changed"
      e.requestBody = JSON.stringify(body)
      e.requestSHA256 = digest(e.requestBody)
      e.requestBytes = Buffer.byteLength(e.requestBody)
      return o
    },
    check: (e) => assert.equal(e.reason, "missing_or_invalid_capture"),
  })
  await scenario("unexpected provider model", {
    transform: (o) => {
      const e = o.releaseCapture.envelopes[1]
      const body = JSON.parse(e.requestBody)
      body.model = "other"
      e.requestBody = JSON.stringify(body)
      e.requestSHA256 = digest(e.requestBody)
      e.requestBytes = Buffer.byteLength(e.requestBody)
      return o
    },
    check: (e) => assert.equal(e.reason, "missing_or_invalid_capture"),
  })
  await scenario("redirect is not followed", {
    redirect: true,
    check: (e, n) => {
      assert.equal(e.reason, "unexpected_redirect")
      assert.equal(n, 1)
      assert.equal(e.redirects.length, 1)
    },
  })
  await scenario("changed deterministic envelope despite valid model", {
    transform: (o) => {
      const entry = o.releaseCapture.envelopes[1],
        body = JSON.parse(entry.requestBody)
      body.messages[0].content = "unexpected payload"
      entry.requestBody = JSON.stringify(body)
      entry.requestBytes = Buffer.byteLength(entry.requestBody)
      entry.requestSHA256 = digest(entry.requestBody)
      return o
    },
    check: (e) => assert.equal(e.reason, "missing_or_invalid_capture"),
  })
  await scenario("escaped credential echo is withheld", {
    transform: (o) => ({
      echo: keys["X-OpenAI-Api-Key"].replaceAll("-", "\\u002d"),
    }),
    check: (e) => {
      assert.equal(e.reason, "credential_content_withheld")
      assert.equal(e.browser.body, undefined)
      assert.equal(e.browser.bodyWithheld, true)
      assert.ok(!JSON.stringify(e).includes("controlled-openai"))
    },
  })
  await scenario("caller approval mutation cannot change reservation", {
    setup: (a) => {
      a.caseID = "mutated-case"
      a.requestBody = "changed"
    },
    check: (e, n, b) => {
      assert.equal(e.outcome, "provider_capture_verified")
      assert.equal(n, 1)
      assert.equal(b.snapshot().reservedProviderDispatches, 3)
    },
  })

  const a = approval(),
    budget = budgetFor(a)
  assert.equal(budget.reserve(a.caseID, 3), true)
  await browser.navigate(browser.origin + "/app")
  const runner = await createPreviewProviderRunner(
    { ...browser, readDeployment },
    a,
    { keys, budget },
  )
  const before = browser.requests.filter((r) => r.method === "POST").length
  try {
    assert.equal(await runner.prepare(), true)
    const e = await runner.inspect()
    assert.equal(e.reason, "provider_budget_exhausted")
    assert.equal(
      browser.requests.filter((r) => r.method === "POST").length,
      before,
    )
    assert.deepEqual(budget.snapshot(), {
      reservedCaseIds: [a.caseID],
      reservedWorkflowAttempts: 1,
      reservedProviderDispatches: 3,
    })
    results.push({
      name: "reserved case cannot be repeated",
      outcome: e.outcome,
      reason: e.reason,
      posts: 0,
      ...budget.snapshot(),
    })
  } finally {
    await runner.close()
  }

  const full = createProviderWorkflowBudget({
    id: "full",
    cases: [
      { id: "q-en", maxDispatches: 3 },
      { id: "q-pt", maxDispatches: 3 },
      { id: "d-en", maxDispatches: 4 },
      { id: "d-pt", maxDispatches: 4 },
    ],
  })
  for (const [caseID, slots] of [
    ["q-en", 3],
    ["q-pt", 3],
    ["d-en", 4],
    ["d-pt", 4],
  ])
    assert.equal(full.reserve(caseID, slots), true)
  assert.equal(full.reserve("extra", 3), false)
  assert.equal(full.reserve("q-en", 3), false)
  assert.deepEqual(full.snapshot(), {
    reservedCaseIds: ["q-en", "q-pt", "d-en", "d-pt"],
    reservedWorkflowAttempts: 4,
    reservedProviderDispatches: 14,
  })
  let saved
  const persistentCases = [
    { id: "saved-q", maxDispatches: 3 },
    { id: "saved-d", maxDispatches: 4 },
  ]
  const persistent = createProviderWorkflowBudget({
    id: "saved",
    cases: persistentCases,
    persist: (value) => {
      saved = structuredClone(value)
    },
  })
  assert.equal(persistent.reserve("saved-q", 3), true)
  const restored = createProviderWorkflowBudget({
    id: "saved",
    cases: persistentCases,
    reservedCaseIds: saved.reservedCaseIds,
  })
  assert.equal(restored.reserve("saved-q", 3), false)
  assert.equal(restored.reserve("saved-d", 4), true)
  assert.equal(restored.snapshot().reservedProviderDispatches, 7)

  const failureApproval = approval(),
    failureBudget = createProviderWorkflowBudget({
      id: failureApproval.batchID,
      cases: [{ id: failureApproval.caseID, maxDispatches: 3 }],
      persist: () => {
        throw new Error("controlled disk failure")
      },
    })
  await browser.navigate(browser.origin + "/app")
  const failureRunner = await createPreviewProviderRunner(
    { ...browser, readDeployment },
    failureApproval,
    { keys, budget: failureBudget },
  )
  const failureBefore = browser.requests.filter(
    (r) => r.method === "POST",
  ).length
  try {
    assert.equal(await failureRunner.prepare(), true)
    await assert.rejects(
      () => failureRunner.inspect(),
      /controlled disk failure/,
    )
    assert.equal(
      browser.requests.filter((r) => r.method === "POST").length,
      failureBefore,
    )
    results.push({
      name: "budget persistence failure sends zero POSTs",
      outcome: "stopped",
      reason: "controlled_disk_failure",
      posts: 0,
      ...failureBudget.snapshot(),
    })
  } finally {
    await failureRunner.close()
  }
  for (const cases of [
    Array.from({ length: 4 }, (_, i) => ({ id: "d-" + i, maxDispatches: 4 })),
    Array.from({ length: 5 }, (_, i) => ({ id: "q-" + i, maxDispatches: 3 })),
    [
      { id: "same", maxDispatches: 3 },
      { id: "same", maxDispatches: 3 },
    ],
  ])
    assert.throws(
      () => createProviderWorkflowBudget({ id: "invalid", cases }),
      /Invalid bounded/,
    )
  for (const change of [
    { dispatch: false },
    { expectedStatus: 502 },
    { batchID: "other" },
    { providerDispatchLimit: 14 },
    { deterministicEnvelopes: [] },
  ])
    await assert.rejects(() =>
      createPreviewProviderRunner(
        { ...browser, readDeployment },
        { ...approval(), ...change },
        { keys, budget: budgetFor(approval()) },
      ),
    )
  await assert.rejects(
    () =>
      createPreviewProviderRunner({ ...browser, readDeployment }, approval(), {
        keys,
        budget: { id: a.batchID, reserve: () => true },
      }),
    /Invalid provider operator/,
  )
  const leaked = approval()
  leaked.requestBody = JSON.stringify({
    jobPosting: keys["X-OpenAI-Api-Key"].replaceAll("-", "\\u002d"),
  })
  leaked.requestSHA256 = digest(leaked.requestBody)
  await assert.rejects(
    () =>
      createPreviewProviderRunner({ ...browser, readDeployment }, leaked, {
        keys,
        budget: budgetFor(leaked),
      }),
    /Invalid provider operator/,
  )
  const report = {
    evidence:
      "controlled loopback browser with fabricated provider captures; no real provider dispatch or deployed/provider quality evidence",
    sourceHashes: Object.fromEntries(
      [
        "inspection-browser-fixture.mjs",
        "preview-inspection-harness.mjs",
        "preview-provider-browser.check.mjs",
      ].map((p) => [
        "tests/" + p,
        digest(readFileSync(new URL(p, import.meta.url))),
      ]),
    ),
    results,
    providerDispatches: 0,
    realCredentialReads: 0,
    fullBudgetReservation: full.snapshot(),
    budgetAndApprovalRejectionChecks: 10,
    restoredBudgetReservation: restored.snapshot(),
    externalWorkflowRequests: 0,
    retries: 0,
  }
  if (process.argv[2] === "--output")
    writeFileSync(process.argv[3], JSON.stringify(report, null, 2) + "\n")
  console.log(JSON.stringify(report, null, 2))
} finally {
  await browser.close()
}

import assert from "node:assert/strict"
import { createHash } from "node:crypto"
import { readFileSync, writeFileSync } from "node:fs"
import { inspectionBrowserFixture } from "./inspection-browser-fixture.mjs"
import { createPreviewInspector } from "./preview-inspection-harness.mjs"

const browser = await inspectionBrowserFixture()
const digest = (body) => createHash("sha256").update(body).digest("hex")
const requestBody = JSON.stringify({
  jobPosting: "Ruby developer. Ruby required. Português: manutenção CLI.",
  understandJob: true,
})
const approval = () => ({
  candidateSHA: "6f87542c83241351966401a4f77f4f16c174ddf1",
  deployID: "6acebb527a80ae056877e041",
  origin: browser.origin,
  deployment: {
    id: "6acebb527a80ae056877e041",
    commitRef: "6f87542c83241351966401a4f77f4f16c174ddf1",
    url: browser.origin,
    deployURL: browser.origin,
  },
  documentURL: browser.origin + "/app",
  endpoint: browser.origin + "/api/qualification-gaps",
  caseID: "controlled-inspection",
  expiresAt: new Date(Date.now() + 30 * 60 * 1000).toISOString(),
  requestBody,
  requestSHA256: digest(requestBody),
  dispatch: false,
  expectedStatus: 502,
  stages: [
    "field_validation",
    "qualification_matching",
    "requirement_evidence",
  ],
})
const envelopeBody = JSON.stringify({
  model: "jev-1.13.0",
  state: {
    field: "job_posting",
    submission: JSON.parse(requestBody).jobPosting,
  },
  questions: [],
})
function captureResponse(config = approval()) {
  return {
    releaseCapture: {
      version: "release-capture-v1",
      caseId: config.caseID,
      requestSHA256: config.requestSHA256,
      outcome: "inspection_only",
      envelopes: [
        {
          stage: "field_validation",
          outcome: "not_dispatched",
          admission: "release-egress-v1",
          endpoint: "https://api.typesafe.ai/v1/systemone",
          requestBody: envelopeBody,
          requestEncoding: "utf8",
          requestBytes: Buffer.byteLength(envelopeBody),
          requestSHA256: digest(envelopeBody),
          httpStatus: null,
          responseBytes: null,
          durationMs: null,
        },
      ],
    },
  }
}
const results = []
let deploymentOverride = {}
const readDeployment = async () => ({
  ...approval().deployment,
  context: "deploy-preview",
  observedAt: new Date().toISOString(),
  ...deploymentOverride,
})
async function scenario(name, setup, check, adapt = (value) => value) {
  deploymentOverride = {}
  await browser.navigate(browser.origin + "/app")
  const before = browser.requests.filter((req) => req.method === "POST").length
  browser.respond((_req, res) => {
    res.writeHead(502, {
      "Content-Type": "application/json",
      "Cache-Control": "no-store",
    })
    res.end(JSON.stringify(captureResponse()))
  })
  const inspector = await createPreviewInspector(
    adapt({ ...browser, readDeployment }),
    approval(),
  )
  try {
    assert.equal(
      await inspector.prepare(),
      true,
      "fresh same-origin authenticated document",
    )
    await setup?.(inspector)
    const evidence = await inspector.inspect()
    const posts =
      browser.requests.filter((req) => req.method === "POST").length - before
    await check(evidence, posts)
    await assert.rejects(() => inspector.inspect(), /single-use/)
    results.push({
      name,
      outcome: evidence.outcome,
      reason: evidence.reason,
      posts,
      diagnostic: evidence.endpointDiagnostic,
      redirects: evidence.redirects.length,
    })
  } finally {
    await inspector.close()
  }
}
try {
  await scenario(
    "navigation after readiness",
    async () => {
      await browser.evaluate("history.pushState({},'', '/auth')")
    },
    (evidence, posts) => {
      assert.equal(evidence.reason, "navigation_changed")
      assert.equal(evidence.endpointDiagnostic, "no_endpoint_request_observed")
      assert.equal(posts, 0)
    },
  )
  await scenario(
    "deploy changes after readiness",
    async () => {
      deploymentOverride = { id: "0".repeat(24) }
    },
    (evidence, posts) => {
      assert.equal(evidence.reason, "deployment_identity_unverified")
      assert.equal(evidence.releasedRequests, 0)
      assert.equal(posts, 0)
    },
  )
  await scenario(
    "navigation while browser request is paused",
    null,
    (evidence, posts) => {
      assert.equal(evidence.reason, "navigation_changed")
      assert.equal(evidence.releasedRequests, 0)
      assert.equal(posts, 0)
    },
    (adapter) => {
      let reads = 0
      return {
        ...adapter,
        readDeployment: async () => {
          if (++reads === 2)
            await browser.evaluate("history.pushState({},'', '/auth')")
          return readDeployment()
        },
      }
    },
  )
  await scenario(
    "missing Network event despite response",
    null,
    (evidence, posts) => {
      assert.equal(evidence.reason, "network_request_event_missing")
      assert.equal(evidence.endpointDiagnostic, "network_request_event_missing")
      assert.equal(evidence.outcome, "stopped")
      assert.equal(posts, 1)
    },
    (adapter) => ({
      ...adapter,
      onEvent: (name, listener) =>
        name === "Network.requestWillBeSent"
          ? () => {}
          : adapter.onEvent(name, listener),
    }),
  )
  await scenario(
    "absolute endpoint ignores changed base URL",
    async () => {
      await browser.evaluate(
        `document.head.innerHTML='<base href="${browser.authOrigin}/auth/">'`,
      )
    },
    (evidence, posts) => {
      assert.equal(
        evidence.outcome,
        "inspection_verified",
        JSON.stringify(evidence),
      )
      assert.equal(evidence.request.url, approval().endpoint)
      assert.equal(evidence.request.origin, browser.origin)
      assert.equal(evidence.request.body, requestBody)
      assert.equal(evidence.request.sha256, digest(requestBody))
      assert.equal(evidence.request.bytes, Buffer.byteLength(requestBody))
      assert.equal(evidence.browser.url, approval().endpoint)
      assert.equal(evidence.response.url, approval().endpoint)
      assert.equal(evidence.networkRequestObserved, true)
      assert.equal(posts, 1)
    },
  )
  for (const status of [301, 302, 303, 307, 308]) {
    await scenario(
      `authentication redirect ${status}`,
      async () => {
        browser.respond((_req, res) => {
          res.writeHead(status, { Location: browser.authOrigin + "/auth" })
          res.end()
        })
      },
      (evidence, posts) => {
        assert.equal(evidence.reason, "unexpected_redirect")
        assert.equal(evidence.response.status, status)
        assert.equal(evidence.response.url, approval().endpoint)
        assert.equal(
          evidence.redirects[0].location,
          browser.authOrigin + "/auth",
        )
        assert.equal(evidence.redirects[0].followed, false)
        assert.equal(posts, 1)
      },
    )
  }
  await scenario(
    "unexpected HTTP 404",
    async () => {
      browser.respond((_req, res) => {
        res.writeHead(404, { "Content-Type": "text/html" })
        res.end("Page not found")
      })
    },
    (evidence, posts) => {
      assert.equal(evidence.reason, "unexpected_http_response")
      assert.equal(evidence.browser.status, 404)
      assert.equal(evidence.browser.url, approval().endpoint)
      assert.equal(evidence.endpointDiagnostic, "endpoint_request_observed")
      assert.equal(posts, 1)
    },
  )
  await scenario(
    "successful-looking response without endpoint request",
    async () => {
      await browser.evaluate(
        `window.fetch=async()=>({status:502,url:${JSON.stringify(approval().endpoint)},redirected:false,body:new Response(${JSON.stringify(JSON.stringify(captureResponse()))}).body})`,
      )
    },
    (evidence, posts) => {
      assert.equal(evidence.reason, "no_endpoint_request_observed")
      assert.equal(evidence.outcome, "stopped")
      assert.equal(posts, 0)
    },
  )
  await scenario(
    "mutated browser body blocked before release",
    async () => {
      await browser.evaluate(
        "const original=fetch;window.fetch=(url,options)=>original(url,{...options,body:options.body+' '})",
      )
    },
    (evidence, posts) => {
      assert.ok(
        ["network_bytes_mismatch", "request_bytes_mismatch"].includes(
          evidence.reason,
        ),
      )
      assert.equal(evidence.request.body, undefined)
      assert.equal(evidence.request.bodyWithheld, true)
      assert.equal(evidence.releasedRequests, 0)
      assert.equal(posts, 0)
    },
  )
  await scenario(
    "concurrent duplicate POST cannot spend a second attempt",
    async () => {
      await browser.evaluate(`const original=fetch;window.fetch=(url,options)=>{
      const first=original(url,options),second=original(url,options).catch(()=>{});
      return Promise.all([first,second]).then(([response])=>response);
    }`)
    },
    (evidence, posts) => {
      assert.ok(
        evidence.releasedRequests <= 1,
        `released ${evidence.releasedRequests} concurrent POSTs`,
      )
      assert.ok(posts <= 1, `server received ${posts} concurrent POSTs`)
      assert.equal(evidence.reason, "unexpected_workflow_request")
      assert.equal(evidence.outcome, "stopped")
    },
    (adapter) => ({
      ...adapter,
      readDeployment: async () => {
        await new Promise((resolve) => setTimeout(resolve, 50))
        return readDeployment()
      },
    }),
  )
  for (const [name, change] of [
    [
      "missing capture",
      (output) => {
        delete output.releaseCapture
      },
    ],
    [
      "dispatched envelope",
      (output) => {
        output.releaseCapture.envelopes[0].outcome = "captured"
      },
    ],
    [
      "wrong case",
      (output) => {
        output.releaseCapture.caseId = "other-case"
      },
    ],
    [
      "wrong provider hash",
      (output) => {
        output.releaseCapture.envelopes[0].requestSHA256 = "0".repeat(64)
      },
    ],
    [
      "changed original in provider envelope",
      (output) => {
        const envelope = output.releaseCapture.envelopes[0]
        envelope.requestBody = JSON.stringify({
          model: "jev-1.13.0",
          state: { field: "job_posting", submission: "different" },
        })
        envelope.requestBytes = Buffer.byteLength(envelope.requestBody)
        envelope.requestSHA256 = digest(envelope.requestBody)
      },
    ],
  ])
    await scenario(
      name,
      async () => {
        const output = captureResponse()
        change(output)
        browser.respond((_req, res) => {
          res.writeHead(502, {
            "Content-Type": "application/json",
            "Cache-Control": "no-store",
          })
          res.end(JSON.stringify(output))
        })
      },
      (evidence, posts) => {
        assert.equal(evidence.reason, "missing_or_invalid_capture")
        assert.equal(posts, 1)
      },
    )

  // Readiness must reject an unauthenticated page without any API request.
  deploymentOverride = {}
  await browser.navigate(browser.authOrigin + "/auth")
  const inspector = await createPreviewInspector(
    { ...browser, readDeployment },
    approval(),
  )
  try {
    assert.equal(await inspector.prepare(), false)
    const evidence = await inspector.inspect()
    assert.equal(evidence.reason, "authenticated_page_unverified")
    assert.equal(evidence.releasedRequests, 0)
  } finally {
    await inspector.close()
  }
  await browser.navigate(browser.origin + "/app")
  browser.setAuthenticated(false)
  const expiredSession = await createPreviewInspector(
    { ...browser, readDeployment },
    approval(),
  )
  try {
    assert.equal(await expiredSession.prepare(), false)
    const evidence = await expiredSession.inspect()
    assert.equal(evidence.reason, "unexpected_redirect")
    assert.equal(evidence.releasedRequests, 0)
    assert.equal(evidence.redirects[0].followed, false)
    results.push({
      name: "authentication expires before readiness fetch",
      outcome: evidence.outcome,
      reason: evidence.reason,
      posts: 0,
      redirects: evidence.redirects.length,
    })
  } finally {
    await expiredSession.close()
    browser.setAuthenticated(true)
  }
  for (const invalid of [
    { dispatch: true },
    { endpoint: "/api/qualification-gaps" },
    { requestSHA256: "0".repeat(64) },
    { expiresAt: "2000-01-01T00:00:00Z" },
    { deployment: { ...approval().deployment, commitRef: "0".repeat(40) } },
    {
      origin: "https://deploy-preview-114--beamish-bavarois-333d03.netlify.app",
    },
  ])
    await assert.rejects(
      () =>
        createPreviewInspector({ ...browser, readDeployment }, {
          ...approval(),
          ...invalid,
        }),
      /Invalid inspection approval/,
    )
  await assert.rejects(
    () => createPreviewInspector(browser, approval()),
    /metadata reader/,
  )
  assert.equal(
    browser.requests.filter((req) => req.auth && req.method === "POST").length,
    0,
    "no workflow leaks to the authentication origin",
  )
  const sourceHashes = Object.fromEntries(
    [
      "inspection-browser-fixture.mjs",
      "preview-inspection-harness.mjs",
      "preview-inspection-browser.check.mjs",
    ].map((path) => [
      `tests/${path}`,
      digest(readFileSync(new URL(path, import.meta.url))),
    ]),
  )
  const report = {
    evidence:
      "controlled loopback browser only; no provider quality or deployed acceptance",
    sourceHashes,
    results,
    approvalRejectionChecks: 7,
    authenticationPageRejection: true,
    retries: 0,
    externalWorkflowRequests: 0,
    providerDispatches: 0,
    deploymentReader:
      "controlled metadata fixture; actual Netlify reader remains separately verified at P2",
  }
  if (process.argv[2] === "--output")
    writeFileSync(process.argv[3], JSON.stringify(report, null, 2) + "\n")
  console.log(JSON.stringify(report, null, 2))
} finally {
  await browser.close()
}

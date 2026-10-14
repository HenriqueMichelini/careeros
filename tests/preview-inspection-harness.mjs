// Private, synthetic, inspection-only operator harness. Importing never sends.
// readDeployment must query current Netlify metadata, without cached attestations.
import { createHash } from "node:crypto"

export const inspectionKeys = Object.freeze({
  "X-OpenAI-Api-Key": "sk-release-inspection-inert",
  "X-TypeSafe-Api-Key": "release-inspection-inert",
})
const digest = (raw) => createHash("sha256").update(raw).digest("hex")
const endpointPath = "/api/qualification-gaps"
const boundedBody = async (response, limit) => {
  const reader = response.body.getReader(),
    chunks = []
  let size = 0
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    size += value.length
    if (size > limit) {
      await reader.cancel()
      throw new Error("response_capacity")
    }
    chunks.push(value)
  }
  const bytes = new Uint8Array(size)
  let offset = 0
  for (const chunk of chunks) {
    bytes.set(chunk, offset)
    offset += chunk.length
  }
  return new TextDecoder("utf-8", { fatal: true }).decode(bytes)
}

function validate(approval) {
  const origin = new URL(approval.origin)
  const local =
    origin.protocol === "http:" &&
    ["127.0.0.1", "localhost", "[::1]"].includes(origin.hostname)
  const previewHost =
    /^(?:deploy-preview-\d+|[a-f0-9]{24})--([a-z0-9-]+\.netlify\.app)$/.exec(
      origin.hostname,
    )
  const deployURL = local
    ? approval.origin
    : `https://${approval.deployID}--${previewHost?.[1]}`
  if (
    origin.origin !== approval.origin ||
    origin.username ||
    origin.password ||
    (!local &&
      (origin.protocol !== "https:" ||
        !previewHost ||
        (!origin.hostname.startsWith("deploy-preview-") &&
          !origin.hostname.startsWith(approval.deployID + "--")))) ||
    !/^[a-f0-9]{24}$/.test(approval.deployID) ||
    !/^[a-f0-9]{40}$/.test(approval.candidateSHA) ||
    approval.deployment.id !== approval.deployID ||
    approval.deployment.commitRef !== approval.candidateSHA ||
    approval.deployment.url !== approval.origin ||
    approval.deployment.deployURL !== deployURL ||
    approval.endpoint !== approval.origin + endpointPath ||
    new URL(approval.documentURL).origin !== approval.origin ||
    !/^[a-z0-9][a-z0-9-]{0,63}$/.test(approval.caseID) ||
    approval.dispatch !== false ||
    approval.expectedStatus !== 502 ||
    approval.stages?.join(",") !==
      "field_validation,qualification_matching,requirement_evidence" ||
    typeof approval.requestBody !== "string" ||
    Buffer.byteLength(approval.requestBody) > 512 * 1024 ||
    digest(approval.requestBody) !== approval.requestSHA256 ||
    !fresh(approval)
  )
    throw new Error("Invalid inspection approval; no browser request sent")
}
function fresh(approval) {
  const remaining = Date.parse(approval.expiresAt) - Date.now()
  return remaining > 0 && remaining <= 60 * 60 * 1000
}

function observedRequest(approval, request) {
  const headers = Object.fromEntries(
    Object.entries(request.headers).map(([name, value]) => [
      name.toLowerCase(),
      value,
    ]),
  )
  const raw = request.postDataEntries
    ? Buffer.concat(
        request.postDataEntries.map(({ bytes }) =>
          Buffer.from(bytes, "base64"),
        ),
      )
    : Buffer.from(request.postData ?? "", "utf8")
  const approvedBytes =
    raw.equals(Buffer.from(approval.requestBody)) &&
    digest(raw) === approval.requestSHA256
  return {
    headers,
    approvedBytes,
    observation: {
      url: request.url,
      method: request.method,
      origin: headers.origin ?? null,
      ...(approvedBytes
        ? { body: raw.toString("utf8") }
        : { bodyWithheld: true }),
      bytes: raw.length,
      sha256: digest(raw),
      source: "CDP.Fetch.requestPaused",
    },
  }
}

function requestAdmissionIssue(approval, request, { headers, approvedBytes }) {
  if (request.url !== approval.endpoint || request.method !== "POST")
    return "unexpected_route"
  if (headers.origin !== approval.origin) return "request_origin_mismatch"
  if (
    headers["x-careeros-release-capture"] !== approval.caseID ||
    Object.entries(inspectionKeys).some(
      ([name, value]) => headers[name.toLowerCase()] !== value,
    )
  )
    return "request_headers_mismatch"
  if (!approvedBytes) return "request_bytes_mismatch"
  if (!fresh(approval)) return "approval_expired"
  return null
}

function endpointDiagnostic(approval, evidence) {
  if (
    evidence.request?.url !== approval.endpoint ||
    evidence.request?.method !== "POST"
  )
    return "no_endpoint_request_observed"
  if (!evidence.networkRequestObserved) return "network_request_event_missing"
  return "endpoint_request_observed"
}

function responseObservationIssue(approval, evidence) {
  const result = evidence.browser
  if (result?.stopped) return result.stopped
  const diagnostic = endpointDiagnostic(approval, evidence)
  if (diagnostic !== "endpoint_request_observed") return diagnostic
  if (evidence.releasedRequests !== 1) return "request_count_mismatch"
  if (!result || result.failed) return "browser_request_failed"
  if (result.url !== approval.endpoint || result.redirected)
    return "response_url_mismatch"
  if (
    result.status !== approval.expectedStatus ||
    evidence.response?.status !== approval.expectedStatus
  )
    return "unexpected_http_response"
  return null
}

function verifiedFirstEnvelope(approval, evidence) {
  const capture = JSON.parse(evidence.browser.body).releaseCapture
  const envelope = capture.envelopes[0]
  const raw =
    envelope.requestEncoding === "utf8"
      ? Buffer.from(envelope.requestBody)
      : null
  const providerInput = raw ? JSON.parse(raw.toString("utf8")) : null
  if (
    capture.version !== "release-capture-v1" ||
    capture.caseId !== approval.caseID ||
    capture.requestSHA256 !== approval.requestSHA256 ||
    capture.outcome !== "inspection_only" ||
    capture.envelopes.length !== 1 ||
    envelope.stage !== "field_validation" ||
    envelope.outcome !== "not_dispatched" ||
    envelope.admission !== "release-egress-v1" ||
    envelope.endpoint !== "https://api.typesafe.ai/v1/systemone" ||
    !raw ||
    raw.length !== envelope.requestBytes ||
    digest(raw) !== envelope.requestSHA256 ||
    providerInput.model !== "jev-1.13.0" ||
    providerInput.state?.field !== "job_posting" ||
    providerInput.state.submission !==
      JSON.parse(approval.requestBody).jobPosting ||
    envelope.httpStatus !== null ||
    envelope.responseBytes !== null ||
    envelope.durationMs !== null ||
    envelope.responseBody !== undefined ||
    envelope.responseSHA256 !== undefined ||
    envelope.responseEncoding !== undefined ||
    evidence.response.cacheControl !== "no-store" ||
    !evidence.response.contentType?.includes("application/json")
  )
    throw new Error("incomplete capture")
  return envelope
}

export async function createPreviewInspector(
  { call, evaluate, onEvent, readDeployment },
  suppliedApproval,
) {
  const approval = structuredClone(suppliedApproval)
  validate(approval)
  if (typeof readDeployment !== "function")
    throw new Error("A fresh deployment metadata reader is required")
  const evidence = {
    version: "preview-inspection-browser-v1",
    candidateSHA: approval.candidateSHA,
    deployID: approval.deployID,
    origin: approval.origin,
    endpoint: approval.endpoint,
    caseID: approval.caseID,
    requestSHA256: approval.requestSHA256,
    expiresAt: approval.expiresAt,
    outcome: "not_attempted",
    reason: null,
    request: null,
    response: null,
    redirects: [],
    blockedRequests: [],
    networkRequestObserved: false,
    releasedRequests: 0,
    readiness: null,
    browser: null,
    deploymentChecks: [],
  }
  let phase = "readiness",
    prepared = false,
    consumed = false,
    closed = false,
    requestClaimed = false
  const tasks = new Set(),
    unsubscribers = []
  const stop = (reason) => {
    evidence.reason ??= reason
  }
  const subscribe = (name, fn) => {
    unsubscribers.push(
      onEvent(name, (event) => {
        const task = Promise.resolve()
          .then(() => fn(event))
          .catch(() => stop("recorder_failure"))
        tasks.add(task)
        task
          .finally(() => tasks.delete(task))
          .catch(() => stop("recorder_failure"))
      }),
    )
  }
  const snapshot = () =>
    evaluate(`({url:location.href,origin:location.origin,ready:document.readyState,
    app:!!document.querySelector('#root')?.childElementCount})`)
  const atApprovedPage = (state) =>
    state.url === approval.documentURL &&
    state.origin === approval.origin &&
    state.ready === "complete" &&
    state.app
  const verifyDeployment = async () => {
    try {
      const controller = new AbortController()
      let timer
      const timeout = new Promise((_, reject) => {
        timer = setTimeout(() => {
          controller.abort()
          reject(new Error("deployment read deadline"))
        }, 5000)
      })
      let current
      try {
        current = await Promise.race([
          readDeployment({ signal: controller.signal }),
          timeout,
        ])
      } finally {
        clearTimeout(timer)
      }
      const age = Date.now() - Date.parse(current.observedAt)
      if (
        current.id !== approval.deployID ||
        current.commitRef !== approval.candidateSHA ||
        current.url !== approval.origin ||
        current.deployURL !== approval.deployment.deployURL ||
        current.context !== "deploy-preview" ||
        !Number.isFinite(age) ||
        age < 0 ||
        age > 5000
      )
        throw new Error("deployment mismatch")
      evidence.deploymentChecks.push({
        id: current.id,
        commitRef: current.commitRef,
        url: current.url,
        deployURL: current.deployURL,
        observedAt: current.observedAt,
        context: current.context,
      })
      return true
    } catch {
      stop("deployment_identity_unverified")
      return false
    }
  }

  subscribe("Network.requestWillBeSent", ({ request, redirectResponse }) => {
    if (request.url === approval.endpoint && request.method === "POST") {
      evidence.networkRequestObserved = true
      if (request.postData !== approval.requestBody)
        stop("network_bytes_mismatch")
    }
    if (redirectResponse) {
      evidence.redirects.push({
        url: redirectResponse.url,
        status: redirectResponse.status,
        destination: request.url,
        followed: true,
      })
      stop("unexpected_redirect")
    }
  })
  const pausedWorkflowIssue = async (request) => {
    const observed = observedRequest(approval, request)
    evidence.request ??= observed.observation
    // Reserve synchronously before any deployment/page await.
    if (phase !== "inspection" || requestClaimed) {
      stop("unexpected_workflow_request")
      return "unexpected_workflow_request"
    }
    requestClaimed = true
    const admissionIssue = requestAdmissionIssue(approval, request, observed)
    if (admissionIssue) return admissionIssue
    if (!(await verifyDeployment())) return "deployment_identity_unverified"
    if (!atApprovedPage(await snapshot())) return "navigation_changed"
    // Time can advance during the awaited checks. This is a separate phase
    // from synchronous request admission, not a redundant predicate.
    if (!fresh(approval)) return "approval_expired"
    if (evidence.reason) return evidence.reason
    evidence.releasedRequests++
    return null
  }
  const pausedRequestIssue = async (request) => {
    const url = new URL(request.url)
    if (url.origin !== approval.origin) return "unexpected_origin"
    if (
      url.pathname.startsWith("/api/") ||
      url.pathname.startsWith("/.netlify/functions/")
    )
      return pausedWorkflowIssue(request)
    if (!["GET", "HEAD"].includes(request.method)) return "unexpected_method"
    return null
  }
  const pausedResponseIssue = ({
    request,
    responseStatusCode,
    responseHeaders = [],
  }) => {
    const headers = Object.fromEntries(
      responseHeaders.map(({ name, value }) => [name.toLowerCase(), value]),
    )
    if (request.url === approval.endpoint)
      evidence.response = {
        url: request.url,
        status: responseStatusCode,
        contentType: headers["content-type"] ?? null,
        cacheControl: headers["cache-control"] ?? null,
      }
    if (responseStatusCode >= 300 && responseStatusCode < 400) {
      evidence.redirects.push({
        url: request.url,
        status: responseStatusCode,
        location: headers.location ?? null,
        followed: false,
      })
      return "unexpected_redirect"
    }
    return null
  }
  subscribe("Fetch.requestPaused", async (event) => {
    const { requestId, request, responseStatusCode } = event
    try {
      const responsePaused = responseStatusCode !== undefined
      const reason = responsePaused
        ? pausedResponseIssue(event)
        : await pausedRequestIssue(request)
      if (reason) {
        stop(reason)
        if (!responsePaused)
          evidence.blockedRequests.push({
            url: request.url,
            method: request.method,
            reason,
          })
        await call("Fetch.failRequest", { requestId, errorReason: "Aborted" })
      } else await call("Fetch.continueRequest", { requestId })
    } catch {
      stop("recorder_failure")
      await call("Fetch.failRequest", {
        requestId,
        errorReason: "Aborted",
      }).catch(() => {})
    }
  })
  await call("Network.enable")
  await call("Network.setBypassServiceWorker", { bypass: true })
  await call("Network.setCacheDisabled", { cacheDisabled: true })
  await call("Fetch.enable", {
    patterns: [
      { urlPattern: "*", requestStage: "Request" },
      { urlPattern: "*", requestStage: "Response" },
    ],
  })

  return {
    async prepare() {
      if (closed || consumed || prepared)
        throw new Error("Inspector is single-use")
      if (!(await verifyDeployment())) return false
      if (!atApprovedPage(await snapshot())) {
        stop("authenticated_page_unverified")
        return false
      }
      // Read-only document fetch checks the session again; authentication redirects stop here.
      const result = await evaluate(`(async () => {
        try {
          const response = await fetch(${JSON.stringify(approval.documentURL)}, {
            credentials:'same-origin',redirect:'error',cache:'no-store',signal:AbortSignal.timeout(10000)});
          return {url:response.url,status:response.status,redirected:response.redirected,
            body:await (${boundedBody.toString()})(response,1048576)};
        } catch {return {failed:true}};
      })()`)
      await Promise.all(tasks)
      evidence.readiness = {
        url: result.url ?? null,
        status: result.status ?? null,
        documentSHA256: result.body === undefined ? null : digest(result.body),
      }
      if (
        result.failed ||
        result.status !== 200 ||
        result.url !== approval.documentURL ||
        result.redirected ||
        !/id=["']root["']/.test(result.body) ||
        !atApprovedPage(await snapshot())
      )
        stop("authenticated_page_unverified")
      prepared = !evidence.reason
      return prepared
    },
    async inspect() {
      if (consumed || closed)
        throw new Error("Inspector is single-use; no retry")
      consumed = true
      evidence.outcome = "stopped"
      if (!prepared) stop("authenticated_page_unverified")
      if (!fresh(approval)) stop("approval_expired")
      if (!evidence.reason) {
        phase = "inspection"
        try {
          evidence.browser = await evaluate(`(async () => {
            const started=performance.now();
            if(location.href!==${JSON.stringify(approval.documentURL)} || location.origin!==${JSON.stringify(approval.origin)} ||
               document.readyState!=='complete' || !document.querySelector('#root')?.childElementCount)
              return {stopped:'navigation_changed'};
            if(Date.now()>=${Date.parse(approval.expiresAt)}) return {stopped:'approval_expired'};
            try {
              const response=await fetch(${JSON.stringify(approval.endpoint)}, {
                method:'POST',redirect:'error',credentials:'same-origin',cache:'no-store',signal:AbortSignal.timeout(10000),
                headers:${JSON.stringify({ ...inspectionKeys, "Content-Type": "application/json", "X-CareerOS-Release-Capture": approval.caseID })},
                body:${JSON.stringify(approval.requestBody)}});
              return {url:response.url,status:response.status,redirected:response.redirected,
                body:await (${boundedBody.toString()})(response,3145728),durationMs:performance.now()-started};
            } catch {return {failed:true,durationMs:performance.now()-started}};
          })()`)
        } catch {
          stop("browser_execution_failed")
        }
      }
      await Promise.all(tasks)
      const result = evidence.browser
      if (!evidence.reason) stop(responseObservationIssue(approval, evidence))
      if (!evidence.reason) {
        try {
          const envelope = verifiedFirstEnvelope(approval, evidence)
          evidence.outcome = "inspection_verified"
          evidence.providerEnvelope = envelope
        } catch {
          stop("missing_or_invalid_capture")
        }
      }
      evidence.endpointDiagnostic = endpointDiagnostic(approval, evidence)
      if (result?.body !== undefined)
        evidence.responseBodySHA256 = digest(result.body)
      return structuredClone(evidence)
    },
    async close() {
      if (closed) return
      closed = true
      // Drain handlers before releasing interception on this disposable test target.
      await Promise.all(tasks)
      await call("Fetch.disable")
      for (const unsubscribe of unsubscribers) unsubscribe()
      await call("Network.setBypassServiceWorker", { bypass: false })
      await call("Network.setCacheDisabled", { cacheDisabled: false })
    },
  }
}

// Private synthetic preview operators. Importing never sends.
// readDeployment must query current Netlify metadata, without cached attestations.
import { createHash } from "node:crypto"

export const inspectionKeys = Object.freeze({
  "X-OpenAI-Api-Key": "sk-release-inspection-inert",
  "X-TypeSafe-Api-Key": "release-inspection-inert",
})
const digest = (raw) => createHash("sha256").update(raw).digest("hex")
const endpointPath = "/api/qualification-gaps"
const providerBudgets = new WeakSet()
function credentialContent(raw, keys) {
  let value = raw
  for (let depth = 0; depth < 64; depth++) {
    if (keys.some((key) => value.includes(key))) return true
    const decoded = value.replace(
      /\\(?:u([0-9a-f]{4})|([\\/"bfnrt]))/gi,
      (_, hex, escaped) =>
        hex
          ? String.fromCharCode(Number.parseInt(hex, 16))
          : ({ b: "\b", f: "\f", n: "\n", r: "\r", t: "\t" }[escaped] ??
            escaped),
    )
    if (decoded === value) return false
    value = decoded
  }
  return true
}
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

function validate(approval, policy) {
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
    approval.endpoint !== approval.origin + policy.path ||
    new URL(approval.documentURL).origin !== approval.origin ||
    !/^[a-z0-9][a-z0-9-]{0,63}$/.test(approval.caseID) ||
    approval.dispatch !== policy.dispatch ||
    approval.expectedStatus !== policy.expectedStatus ||
    approval.stages?.join(",") !== policy.stages.join(",") ||
    typeof approval.requestBody !== "string" ||
    Buffer.byteLength(approval.requestBody) > 512 * 1024 ||
    digest(approval.requestBody) !== approval.requestSHA256 ||
    !fresh(approval)
  )
    throw new Error("Invalid inspection approval; no browser request sent")
  policy.validateApproval?.(approval)
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

function requestAdmissionIssue(
  approval,
  request,
  { headers, approvedBytes },
  keys,
) {
  if (request.url !== approval.endpoint || request.method !== "POST")
    return "unexpected_route"
  if (headers.origin !== approval.origin) return "request_origin_mismatch"
  if (
    headers["x-careeros-release-capture"] !== approval.caseID ||
    Object.entries(keys).some(
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
    (approval.expectedStatus !== null &&
      result.status !== approval.expectedStatus) ||
    evidence.response?.status !== result.status
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

export async function createPreviewInspector(adapter, approval) {
  return createPreviewRecorder(adapter, approval, {
    path: endpointPath,
    dispatch: false,
    expectedStatus: 502,
    stages: [
      "field_validation",
      "qualification_matching",
      "requirement_evidence",
    ],
    keys: inspectionKeys,
    timeoutMs: 10000,
    version: "preview-inspection-browser-v1",
    outcome: "inspection_verified",
    verifyResponse: (value, evidence) => ({
      providerEnvelope: verifiedFirstEnvelope(value, evidence),
    }),
  })
}

// Paid use has its own explicit policy. The inspection API still rejects it.
export async function createPreviewProviderRunner(
  adapter,
  approval,
  { keys, budget },
) {
  approval = structuredClone(approval)
  const parsedOrigin = new URL(approval.origin)
  const local =
    parsedOrigin.protocol === "http:" &&
    ["127.0.0.1", "localhost", "[::1]"].includes(parsedOrigin.hostname)
  const stages =
    approval.endpoint === approval.origin + "/api/qualification-gaps"
      ? ["field_validation", "qualification_matching", "requirement_evidence"]
      : approval.endpoint === approval.origin + "/api/application-draft"
        ? [
            "field_validation",
            "generation",
            "resume_support",
            "artifact_support",
          ]
        : null
  if (
    !stages ||
    !providerBudgets.has(budget) ||
    (!local && !budget.persistenceConfigured) ||
    !keys ||
    Object.keys(keys).sort().join(",") !==
      "X-OpenAI-Api-Key,X-TypeSafe-Api-Key" ||
    Object.values(keys).some(
      (value) =>
        typeof value !== "string" || !value.trim() || /[\r\n]/.test(value),
    ) ||
    credentialContent(approval.requestBody, Object.values(keys))
  )
    throw new Error(
      "Invalid provider operator configuration; no browser request sent",
    )
  const privateKeys = { ...keys }
  return createPreviewRecorder(adapter, approval, {
    path: new URL(approval.endpoint).pathname,
    dispatch: true,
    expectedStatus: null,
    stages,
    keys: privateKeys,
    timeoutMs: 30000,
    version: "preview-provider-browser-v1",
    outcome: "provider_capture_verified",
    validateApproval: (value) => {
      if (
        value.batchID !== budget.id ||
        value.providerDispatchLimit !== stages.length ||
        value.deterministicEnvelopes?.length !== 2 ||
        value.deterministicEnvelopes.some(
          (entry, index) =>
            entry.stage !== stages[index] ||
            !/^[a-f0-9]{64}$/.test(entry.requestSHA256) ||
            !Number.isInteger(entry.requestBytes) ||
            entry.requestBytes <= 0 ||
            entry.requestBytes > 256 * 1024,
        )
      )
        throw new Error(
          "Provider budget approval mismatch; no browser request sent",
        )
    },
    beforeInspect: () => budget.reserve(approval.caseID, stages.length),
    sanitize: (evidence) => {
      if (
        evidence.browser?.body !== undefined &&
        credentialContent(evidence.browser.body, Object.values(privateKeys))
      ) {
        delete evidence.browser.body
        evidence.browser.bodyWithheld = true
        return "credential_content_withheld"
      }
      return null
    },
    verifyResponse: verifiedProviderCapture,
  })
}

export function createProviderWorkflowBudget({
  id,
  cases,
  reservedCaseIds = [],
  persist,
}) {
  if (
    typeof id !== "string" ||
    !id ||
    !Array.isArray(cases) ||
    !cases.length ||
    cases.length > 4 ||
    new Set(cases.map((value) => value.id)).size !== cases.length ||
    cases.some(
      (value) =>
        !/^[a-z0-9][a-z0-9-]{0,63}$/.test(value.id) ||
        ![3, 4].includes(value.maxDispatches),
    ) ||
    cases.reduce((sum, value) => sum + value.maxDispatches, 0) > 14 ||
    !Array.isArray(reservedCaseIds) ||
    new Set(reservedCaseIds).size !== reservedCaseIds.length ||
    reservedCaseIds.some(
      (caseID) => !cases.some((value) => value.id === caseID),
    ) ||
    (persist !== undefined && typeof persist !== "function")
  )
    throw new Error("Invalid bounded provider workflow budget")
  const allowed = new Map(cases.map((value) => [value.id, value.maxDispatches]))
  const spent = new Set(reservedCaseIds)
  const snapshot = () => ({
    reservedCaseIds: [...spent],
    reservedWorkflowAttempts: spent.size,
    reservedProviderDispatches: [...spent].reduce(
      (sum, caseID) => sum + allowed.get(caseID),
      0,
    ),
  })
  const budget = Object.freeze({
    id,
    persistenceConfigured: typeof persist === "function",
    reserve(caseID, dispatches) {
      if (spent.has(caseID) || allowed.get(caseID) !== dispatches) return false
      spent.add(caseID)
      const saved = persist?.({ id, ...snapshot() })
      if (saved?.then) {
        Promise.resolve(saved).catch(() => {})
        throw new Error(
          "Budget persistence must finish synchronously before dispatch",
        )
      }
      return true
    },
    snapshot,
  })
  providerBudgets.add(budget)
  return budget
}

function verifiedProviderCapture(approval, evidence) {
  const capture = JSON.parse(evidence.browser.body).releaseCapture
  if (
    capture?.version !== "release-capture-v1" ||
    capture.caseId !== approval.caseID ||
    capture.requestSHA256 !== approval.requestSHA256 ||
    !["complete", "incomplete"].includes(capture.outcome) ||
    !Array.isArray(capture.envelopes) ||
    capture.envelopes.length > approval.stages.length ||
    (capture.outcome === "complete" &&
      capture.envelopes.length !== approval.stages.length) ||
    evidence.response.cacheControl !== "no-store" ||
    !evidence.response.contentType?.includes("application/json")
  )
    throw new Error("Invalid provider capture")
  for (const [index, entry] of capture.envelopes.entries()) {
    const target =
      index === 0
        ? "https://api.typesafe.ai/v1/systemone"
        : "https://api.openai.com/v1/chat/completions"
    const request =
      entry.requestEncoding === "utf8"
        ? Buffer.from(entry.requestBody ?? "")
        : null
    if (
      entry.stage !== approval.stages[index] ||
      entry.admission !== "release-egress-v1" ||
      entry.endpoint !== target ||
      !request ||
      request.length !== entry.requestBytes ||
      digest(request) !== entry.requestSHA256 ||
      !["captured", "transport_failure", "authentication_failure"].includes(
        entry.outcome,
      ) ||
      (entry.outcome !== "captured" && index !== capture.envelopes.length - 1)
    )
      throw new Error("Invalid serialized provider envelope")
    const known = approval.deterministicEnvelopes[index]
    if (
      known &&
      (entry.requestSHA256 !== known.requestSHA256 ||
        entry.requestBytes !== known.requestBytes)
    )
      throw new Error("Deterministic preflight envelope mismatch")
    const data = JSON.parse(request.toString("utf8"))
    if (
      index === 0 &&
      (data.model !== "jev-1.13.0" ||
        data.state?.field !== "job_posting" ||
        data.state.submission !== JSON.parse(approval.requestBody).jobPosting)
    )
      throw new Error("Original Jev input mismatch")
    if (
      index > 0 &&
      (data.model !== "gpt-6-luna" || data.reasoning_effort !== "none")
    )
      throw new Error("Provider settings mismatch")
    if (entry.outcome === "captured") {
      const response =
        entry.responseEncoding === "utf8"
          ? Buffer.from(entry.responseBody ?? "")
          : entry.responseEncoding === "base64"
            ? Buffer.from(entry.responseBody ?? "", "base64")
            : null
      if (
        !response ||
        response.length !== entry.responseBytes ||
        digest(response) !== entry.responseSHA256 ||
        !Number.isInteger(entry.httpStatus) ||
        entry.httpStatus < 100 ||
        entry.httpStatus > 599 ||
        !Number.isFinite(entry.durationMs) ||
        entry.durationMs < 0
      )
        throw new Error("Invalid serialized provider response")
    } else if (
      entry.responseBytes !== null ||
      entry.responseBody !== undefined ||
      !Number.isFinite(entry.durationMs) ||
      entry.durationMs < 0 ||
      (entry.outcome === "transport_failure"
        ? entry.httpStatus !== null
        : ![401, 403].includes(entry.httpStatus))
    ) {
      throw new Error("Invalid failed-provider observation")
    }
  }
  return {
    captureOutcome: capture.outcome,
    providerEnvelopes: capture.envelopes,
  }
}

async function createPreviewRecorder(
  { call, evaluate, onEvent, readDeployment },
  suppliedApproval,
  policy,
) {
  const approval = structuredClone(suppliedApproval)
  validate(approval, policy)
  if (typeof readDeployment !== "function")
    throw new Error("A fresh deployment metadata reader is required")
  const evidence = {
    version: policy.version,
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
    const admissionIssue = requestAdmissionIssue(
      approval,
      request,
      observed,
      policy.keys,
    )
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
      if (policy.beforeInspect && !policy.beforeInspect())
        stop("provider_budget_exhausted")
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
                method:'POST',redirect:'error',credentials:'same-origin',cache:'no-store',signal:AbortSignal.timeout(${policy.timeoutMs}),
                headers:${JSON.stringify({ ...policy.keys, "Content-Type": "application/json", "X-CareerOS-Release-Capture": approval.caseID })},
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
      if (policy.sanitize) stop(policy.sanitize(evidence))
      if (!evidence.reason) stop(responseObservationIssue(approval, evidence))
      if (!evidence.reason) {
        try {
          Object.assign(evidence, policy.verifyResponse(approval, evidence))
          evidence.outcome = policy.outcome
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

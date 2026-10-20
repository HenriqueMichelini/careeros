import assert from "node:assert/strict"
import { spawn } from "node:child_process"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { createServer } from "node:net"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createBrowserEvaluation } from "./browser-evaluation.mjs"
import { savedProfileExpression } from "./profile-browser-storage.mjs"

// Browser evidence for the composed Profile ingestion journey (#192). Every
// provider response is synthetic; the canonical Profile is browser-local and no
// live request or deployment is used.
const root = new URL("..", import.meta.url).pathname
const work = mkdtempSync(join(tmpdir(), "careeros-submission-journey-"))
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const port = await new Promise((resolve) => {
  const server = createServer()
  server.listen(0, "127.0.0.1", () => {
    const value = server.address().port
    server.close(() => resolve(value))
  })
})
const populated = {
  fullName: "Ada Lovelace",
  email: "ada@example.test",
  phone: "",
  location: "",
  professionalLinks: "",
  careerGoals: "Build useful products.",
  skills: "Research",
  competencies: "",
  tools: "",
  projects: [],
  education: [],
  certifications: [],
  languages: [],
  employmentStatus: "",
  currentSalary: "",
  desiredSalary: "",
  additionalInfo: "",
  experience: [],
}

function journeyFetch(url, options) {
  if (url !== "/api/profile/ingest") return window.__realFetch(url, options)
  const body = JSON.parse(options.body)
  const calls = (window.__journeyCalls ??= [])
  calls.push({ kind: body.stage?.kind, body })
  const scenario = window.__journeyScenario || "normal"
  const runId = "run-journey"
  const bytes = new TextEncoder().encode(body.input)
  const splitAt = body.input.indexOf("I led")
  const split = splitAt > 0 ? new TextEncoder().encode(body.input.slice(0, splitAt)).length : bytes.length
  const ranges =
    scenario === "partial"
      ? [
          [0, split],
          [split, bytes.length],
        ]
      : [[0, bytes.length]]
  const batches = ranges.map(([Start, End], index) => ({
    index,
    primary: [{ Start, End }],
    context: [],
    preparedBytes: 200,
    payloadBytes: 128,
  }))
  const receipt = (label) => `${label}.${"s".repeat(43)}`
  const expiresAt = Date.now() + 600000
  const ok = (raw) => new Response(JSON.stringify(raw), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })

  if (body.stage?.kind === "plan")
    return ok({
      stage: { version: 2, kind: "plan" },
      planReceipt: receipt("plan"),
      expiresAt,
      plan: {
        protocolVersion: "profile-submission-v1",
        sourceId: "source-journey",
        profileId: "profile-journey",
        runId,
        planVersion: "portions-v1",
        status: "ready",
        complete: true,
        originalBytes: bytes.length,
        preparedBytes: batches.length * 200,
        batches,
        remaining: [],
      },
    })

  if (body.stage?.kind === "extract") {
    if (
      scenario === "partial" &&
      body.batchIndex === 1 &&
      window.__failBatchOnce
    ) {
      window.__failBatchOnce = false
      return new Response(JSON.stringify({ error: "outage" }), { status: 502 })
    }
    const claim = {
      id: "c0",
      source: body.input,
      text:
        scenario === "clarification"
          ? "TypeScript version is unclear"
          : body.input,
      targets: scenario === "clarification" ? [] : ["skills"],
      question:
        scenario === "clarification"
          ? "Which TypeScript version do you use?"
          : "",
    }
    const skipped =
      scenario === "partial" && body.batchIndex === 0
        ? [
            {
              index: 1,
              reason: "source",
              text: "Unvalidated extracted claim",
              source: "",
              shortened: false,
            },
          ]
        : []
    const response = () =>
      ok({
        stage: { version: 2, kind: "extract" },
        batchReceipt: receipt(`batch${body.batchIndex}`),
        expiresAt,
        batchIndex: body.batchIndex,
        claims: [claim],
        unverifiedCount: skipped.length,
        skippedClaims: skipped,
        coverage: {
          validClaims: 1,
          invalidClaims: skipped.length,
          discoveryComplete: false,
          capacity: "within_limit",
        },
      })
    if (window.__holdExtract) {
      window.__holdExtract = false
      window.__extractHeld = true
      return new Promise((resolve, reject) => {
        window.__releaseExtract = () => resolve(response())
        options.signal?.addEventListener(
          "abort",
          () => {
            window.__extractAborted = true
            reject(new DOMException("Aborted", "AbortError"))
          },
          { once: true },
        )
      })
    }
    return response()
  }

  if (body.stage?.kind === "assemble") {
    const claimIds = batches.map((batch) => `${runId}/${batch.index}/c0`)
    return ok({
      stage: { version: 2, kind: "assemble" },
      contextReceipt: receipt("context"),
      expiresAt,
      context: {
        protocolVersion: "profile-submission-v1",
        sourceId: "source-journey",
        profileId: "profile-journey",
        runId,
        planVersion: "portions-v1",
        batches: batches.map((batch, index) => ({
          index: batch.index,
          primary: batch.primary,
          claimIds: [claimIds[index]],
          claimCount: 1,
          skippedCount: scenario === "partial" && index === 0 ? 1 : 0,
        })),
        coverage: {
          validClaims: batches.length,
          invalidClaims: scenario === "partial" ? 1 : 0,
          discoveryComplete: false,
          capacity: "within_limit",
        },
      },
    })
  }

  if (body.stage?.kind === "reconcile") {
    const focused = body.focus.claimIds
    if (scenario === "duplicate")
      return ok({
        stage: { version: 2, kind: "reconcile" },
        focus: { claimIds: focused },
        operations: [],
        outcomes: focused.map((claimId) => ({
          claimId,
          kind: "exact_duplicate",
          reason: "verified_exact_alias_or_wording",
          relatedFacts: [],
          relatedClaimIds: [],
        })),
      })
    if (scenario === "clarification")
      return ok({
        stage: { version: 2, kind: "reconcile" },
        focus: { claimIds: focused },
        operations: [],
        outcomes: focused.map((claimId) => ({
          claimId,
          kind: "clarification",
          reason: "source_ambiguity",
          relatedFacts: [],
          relatedClaimIds: [],
        })),
      })
    if (scenario === "invalid")
      return ok({
        stage: { version: 2, kind: "reconcile" },
        focus: { claimIds: focused },
        operations: focused.flatMap((claimId) => [
          {
            claimId,
            target: "experience",
            entryId: "new:experience-1",
            field: "title",
            action: "add",
            value: "Engineer",
            finding: "addition",
          },
          {
            claimId,
            target: "experience",
            entryId: "new:experience-1",
            field: "startDate",
            action: "add",
            value: "2020",
            finding: "addition",
          },
        ]),
        outcomes: focused.map((claimId) => ({
          claimId,
          kind: "change",
          reason: "validated_operation",
          relatedFacts: [],
          relatedClaimIds: [],
        })),
      })
    return ok({
      stage: { version: 2, kind: "reconcile" },
      focus: { claimIds: focused },
      operations: focused.map((claimId, index) => ({
        claimId,
        target: "skills",
        entryId: "",
        field: "skills",
        action: "add",
        value: index === 0 ? "TypeScript" : "Payments leadership",
        finding: "addition",
      })),
      outcomes: focused.map((claimId) => ({
        claimId,
        kind: "change",
        reason: "validated_operation",
        relatedFacts: [],
        relatedClaimIds: [],
      })),
    })
  }

  if (body.stage?.kind === "clarify") {
    const answer = body.answer.trim()
    const response = () =>
      ok({
        stage: { version: 2, kind: "clarify" },
        claim: {
          ...body.claim,
          targets: ["skills"],
          question: "",
          supportingSources: [
            { source: answer, origin: "clarification_answer" },
          ],
        },
        focus: { claimIds: [body.claim.id] },
        operations: [
          {
            claimId: body.claim.id,
            target: "skills",
            entryId: "",
            field: "skills",
            action: "add",
            value: answer,
            finding: "addition",
          },
        ],
        outcomes: [
          {
            claimId: body.claim.id,
            kind: "change",
            reason: "validated_operation",
            relatedFacts: [],
            relatedClaimIds: [],
          },
        ],
      })
    if (window.__holdClarify) {
      window.__holdClarify = false
      window.__clarifyHeld = true
      return new Promise((resolve, reject) => {
        window.__releaseClarify = () => resolve(response())
        options.signal?.addEventListener(
          "abort",
          () => reject(new DOMException("Aborted", "AbortError")),
          { once: true },
        )
      })
    }
    return response()
  }

  throw new Error(`Unexpected stage ${body.stage?.kind}`)
}

const vite = spawn(
  join(root, "node_modules/.bin/vite"),
  ["--host", "127.0.0.1", "--port", String(port), "--strictPort"],
  { cwd: root, stdio: "ignore" },
)
const chrome = spawn(
  "google-chrome",
  [
    "--headless=new",
    "--no-sandbox",
    "--disable-dev-shm-usage",
    "--disable-gpu",
    "--remote-allow-origins=*",
    "--remote-debugging-port=0",
    `--user-data-dir=${join(work, "chrome")}`,
    "about:blank",
  ],
  { stdio: "ignore" },
)
let socket
try {
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      if ((await fetch(`http://127.0.0.1:${port}/`)).ok) break
    } catch {}
    if (attempt === 99) throw new Error("Vite did not start")
    await pause(100)
  }
  let debugPort
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      debugPort = Number(
        readFileSync(join(work, "chrome", "DevToolsActivePort"), "utf8").split(
          "\n",
        )[0],
      )
      break
    } catch {}
    if (attempt === 99) throw new Error("Chrome did not start")
    await pause(100)
  }
  const tabs = await (
    await fetch(`http://127.0.0.1:${debugPort}/json/list`)
  ).json()
  socket = new WebSocket(
    tabs.find((tab) => tab.type === "page").webSocketDebuggerUrl,
  )
  await new Promise((resolve, reject) => {
    socket.onopen = resolve
    socket.onerror = reject
  })
  let nextId = 0
  const pending = new Map()
  const browserEvaluation = createBrowserEvaluation(pause)
  socket.onmessage = ({ data }) => {
    const message = JSON.parse(data)
    browserEvaluation.observe(message)
    if (!message.id) return
    const callback = pending.get(message.id)
    pending.delete(message.id)
    if (message.error) callback.reject(new Error(message.error.message))
    else callback.resolve(message.result)
  }
  const call = (method, params = {}) =>
    new Promise((resolve, reject) => {
      const id = ++nextId
      pending.set(id, { resolve, reject })
      socket.send(JSON.stringify({ id, method, params }))
    })
  const evaluate = (expression) =>
    browserEvaluation.evaluate(call, expression, { userGesture: true })
  const until = async (expression) => {
    for (let attempt = 0; attempt < 120; attempt++) {
      if (await evaluate(expression)) return
      await pause(100)
    }
    throw new Error(
      `Timed out: ${expression}\n${await evaluate(
        "JSON.stringify({text:document.body.innerText.slice(0,2500),alerts:Array.from(document.querySelectorAll('[role=alert]')).map(e=>e.textContent),calls:window.__journeyCalls?.map(c=>c.kind),state:window.__journeyDebug})",
      )}`,
    )
  }
  await call("Page.enable")
  await call("Emulation.setDeviceMetricsOverride", {
    width: 1280,
    height: 900,
    deviceScaleFactor: 1,
    mobile: false,
  })
  await call("Page.navigate", { url: `http://127.0.0.1:${port}/` })
  await until(
    "document.readyState === 'complete' && !!document.querySelector('header button')",
  )
  const openProfile = async () => {
    await pause(150)
    if (
      !(await evaluate(
        "document.querySelectorAll('header nav button').length === 4",
      ))
    ) {
      const opened = await evaluate(
        `(() => { const el = Array.from(document.querySelectorAll('button')).find(el=>/open app/i.test(el.textContent) || /abrir app/i.test(el.textContent)); if (!el) return false; el.click(); return true; })()`,
      )
      assert.ok(opened, "Landing entry point is missing")
    }
    await until("document.querySelectorAll('header nav button').length === 4")
    await evaluate("document.querySelectorAll('header nav button')[1].click()")
  }
  const byText = (text) =>
    `Array.from(document.querySelectorAll('button')).find(el=>el.textContent.trim()===${JSON.stringify(text)})`
  const clickText = async (text) => {
    const found = await evaluate(`!!${byText(text)}`)
    assert.ok(found, `Missing button ${text}`)
    await evaluate(`${byText(text)}.click()`)
    await pause(80)
  }
  const fill = async (selector, value) => {
    await evaluate(
      `(() => { const el = document.querySelector(${JSON.stringify(selector)}); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, ${JSON.stringify(value)}); el.dispatchEvent(new Event('input', { bubbles: true })); })()`,
    )
    await pause(60)
  }
  const installMock = async (scenario) => {
    await evaluate(
      `(() => {
        if (!window.__realFetch) window.__realFetch = window.fetch.bind(window);
        window.__journeyScenario = ${JSON.stringify(scenario)};
        window.__journeyCalls = [];
        window.__holdExtract = false;
        window.__extractHeld = false;
        window.__extractAborted = false;
        window.__holdClarify = false;
        window.__clarifyHeld = false;
        window.__failBatchOnce = ${scenario === "partial"};
        window.__failSaveOnce = false;
        window.fetch = ${journeyFetch.toString()};
      })()`,
    )
  }
  const resetStorage = async (profile) => {
    await evaluate(
      `(() => {
        localStorage.removeItem('careeros_profile_v2');
        ${profile ? `localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(profile))});` : "localStorage.removeItem('careeros_repo');"}
        localStorage.setItem('careeros_apikey', 'synthetic-test-key');
        localStorage.setItem('careeros_typesafe_key', 'synthetic-typesafe');
        location.reload();
      })()`,
    )
    await until(
      "document.readyState === 'complete' && !!document.querySelector('header button')",
    )
  }
  const watchWrites = async () => {
    await evaluate(
      `(() => {
        window.__profileWrites = 0;
        if (!window.__setPatched) {
          window.__realSet = Storage.prototype.setItem;
          Storage.prototype.setItem = function(key, value) {
            if (key === 'careeros_profile_v2') {
              if (window.__failSaveOnce) {
                window.__failSaveOnce = false;
                throw new DOMException('quota', 'QuotaExceededError');
              }
              window.__profileWrites++;
            }
            return window.__realSet.call(this, key, value);
          };
          window.__setPatched = true;
        }
      })()`,
    )
  }

  // Empty Profile: pasting is the primary entry, Stop keeps the draft, Resume
  // completes the same submission, a failed save retains the review, and one
  // explicit retry saves exactly the selected operation.
  await resetStorage(null)
  await installMock("normal")
  await openProfile()
  await until("!!document.querySelector('#submission-text')")
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Add professional information/,
  )
  assert.equal(
    await evaluate(
      "!!(document.querySelector('[data-submission-journey]').compareDocumentPosition(document.querySelector('#contact-fullName')) & Node.DOCUMENT_POSITION_FOLLOWING)",
    ),
    true,
    "Empty Profile must expose the ingestion journey before contact editing",
  )
  assert.equal(
    await evaluate("document.querySelector('[data-submission-journey]').innerText.includes('Process next portion')"),
    false,
  )
  await fill("#submission-text", "I use TypeScript.")
  await watchWrites()
  await evaluate("window.__holdExtract = true")
  await clickText("Prepare changes")
  await until("window.__extractHeld === true")
  assert.equal(await evaluate("window.__profileWrites"), 0)
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey] [role=status]').innerText"),
    /Extracting information/,
  )
  await clickText("Stop")
  await until("window.__extractAborted === true")
  await until(`!!${byText("Resume processing")}`)
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /I use TypeScript\./,
  )
  await clickText("Resume processing")
  await until("!!document.querySelector('#review-heading')")
  assert.equal(
    await evaluate("!!document.querySelector('#submission-text')"),
    false,
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Your pasted information/,
  )
  assert.equal(
    await evaluate(`${byText("Save to Profile")}.disabled`),
    true,
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-actions]').innerText"),
    /No changes selected/,
  )
  await evaluate(
    "document.querySelector('[data-review-unit] input[type=checkbox]').click()",
  )
  await until(`${byText("Save to Profile")}.disabled === false`)
  assert.match(
    await evaluate("document.querySelector('[data-submission-actions]').innerText"),
    /1 of 1 changes selected/,
  )
  const reviewShot = await call("Page.captureScreenshot", {
    format: "png",
    captureBeyondViewport: true,
  })
  writeFileSync(
    join(work, "journey-review-en-1280.png"),
    Buffer.from(reviewShot.data, "base64"),
  )
  assert.equal(await evaluate("window.__profileWrites"), 0)
  assert.equal(
    await evaluate("window.__journeyCalls.filter(c=>c.kind==='clarify').length"),
    0,
  )
  await evaluate("window.__failSaveOnce = true")
  await clickText("Save to Profile")
  await until(`!!${byText("Retry save")}`)
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey] [role=alert]').innerText"),
    /Not saved/,
  )
  const providerCalls = await evaluate("window.__journeyCalls.length")
  assert.equal(await evaluate("window.__profileWrites"), 0)
  await clickText("Retry save")
  await until(`!!${byText("Add more information")}`)
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey] [role=status]').innerText"),
    /Saved to your Profile/,
  )
  assert.equal(
    await evaluate("window.__journeyCalls.length"),
    providerCalls,
    "Retry save must make zero provider calls",
  )
  assert.equal(await evaluate("window.__profileWrites"), 1)
  const savedProfile = JSON.parse(await evaluate(savedProfileExpression))
  assert.match(savedProfile.skills, /TypeScript/)
  // Leaving and returning after a durable save drops the success panel and
  // returns to the populated Profile entry instead of showing duplicate state.
  await evaluate("document.querySelectorAll('header nav button')[0].click()")
  await pause(100)
  await openProfile()
  await until("!!document.querySelector('#contact-fullName')")
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Add information/,
  )
  assert.equal(
    await evaluate("document.querySelector('[data-submission-journey]').innerText.includes('Saved to your Profile')"),
    false,
  )
  await clickText("Add information")
  await until("!!document.querySelector('#submission-text')")
  assert.equal(
    await evaluate("document.querySelector('#submission-text').value"),
    "",
  )

  console.log("PASS empty Profile journey: stop/resume, explicit save, save retry, one write")

  const clickJourneyDiscard = async () => {
    const clicked = await evaluate(
      `(() => { const el = Array.from(document.querySelectorAll('[data-submission-journey] button')).find(el => el.textContent.trim() === 'Discard' && !el.closest('dialog')); if (!el) return false; el.click(); return true; })()`,
    )
    assert.ok(clicked, "Journey Discard action is missing")
    await until("!!document.querySelector('[data-submission-journey] dialog[open]')")
  }
  const confirmDiscard = async () => {
    await evaluate(
      "document.querySelector('[data-submission-journey] dialog[open] button:last-of-type').click()",
    )
    await until(
      "!document.querySelector('[data-submission-journey] dialog[open]')",
    )
  }
  const openJourney = async () => {
    if (!(await evaluate("!!document.querySelector('#submission-text')"))) {
      await clickText("Add information")
      await until("!!document.querySelector('#submission-text')")
    }
  }
  const runToReview = async (scenario, input) => {
    await installMock(scenario)
    await openJourney()
    await fill("#submission-text", input)
    await clickText("Prepare changes")
    await until("!!document.querySelector('#review-heading')")
  }

  // Populated Profile: manual editing stays separate, Add information opens the
  // same journey, and the existing discard confirmation preserves work on cancel.
  await resetStorage(populated)
  await installMock("normal")
  await openProfile()
  await until("!!document.querySelector('#contact-fullName')")
  assert.equal(
    await evaluate("document.querySelector('#contact-fullName').value"),
    "Ada Lovelace",
  )
  assert.equal(
    await evaluate("!!document.querySelector('#submission-text')"),
    false,
  )
  assert.equal(
    await evaluate(
      "document.querySelector('[data-submission-journey]').innerText.includes('Process next portion')",
    ),
    false,
  )
  await openJourney()
  await fill("#submission-text", "I use TypeScript.")
  await clickJourneyDiscard()
  await evaluate(
    "document.querySelector('[data-submission-journey] dialog[open] button:first-of-type').click()",
  )
  await until(
    "!document.querySelector('[data-submission-journey] dialog[open]')",
  )
  assert.equal(
    await evaluate("document.querySelector('#submission-text').value"),
    "I use TypeScript.",
  )
  await clickJourneyDiscard()
  await confirmDiscard()
  await until(
    "!document.querySelector('#submission-text') && !!Array.from(document.querySelectorAll('[data-submission-journey] button')).find(el=>el.textContent.trim()==='Add information')",
  )
  console.log("PASS populated Profile entry and discard confirmation")

  // No-change: duplicate-only output ends without approval work or any write.
  await resetStorage(null)
  await installMock("duplicate")
  await openProfile()
  await watchWrites()
  await openJourney()
  await fill("#submission-text", "I use TypeScript.")
  await clickText("Prepare changes")
  await until("!!document.querySelector('#review-heading')")
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Everything you pasted is already in your Profile/,
  )
  assert.equal(
    await evaluate("!!document.querySelector('[data-review-unit] input[type=checkbox]')"),
    false,
  )
  assert.equal(await evaluate(`${byText("Save to Profile")}.disabled`), true)
  assert.equal(await evaluate("window.__profileWrites"), 0)
  await clickJourneyDiscard()
  await confirmDiscard()
  await until("!!document.querySelector('#submission-text')")
  assert.equal(
    await evaluate("document.querySelector('#submission-text').value"),
    "",
  )
  console.log("PASS all-duplicate no-change journey")

  // A selection that cannot form a complete transaction keeps Save disabled.
  await resetStorage(null)
  await installMock("invalid")
  await openProfile()
  await watchWrites()
  await openJourney()
  await fill("#submission-text", "I worked as an engineer.")
  await clickText("Prepare changes")
  await until("!!document.querySelector('#review-heading')")
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Cannot be saved yet/,
  )
  assert.equal(
    await evaluate("document.querySelectorAll('[data-review-unit] input[type=checkbox]').length"),
    0,
  )
  assert.equal(
    await evaluate(`${byText("Save to Profile")}.disabled`),
    true,
    "Save must stay disabled for a blocked selection",
  )
  assert.equal(await evaluate("window.__profileWrites"), 0)
  await clickJourneyDiscard()
  await confirmDiscard()
  await until("!!document.querySelector('#submission-text')")
  console.log("PASS invalid selection keeps Save disabled")

  // Partial extraction failure: Retry processing reuses the plan and completed
  // batch, marks unprocessed source explicitly, and saves only selected work.
  await resetStorage(null)
  await installMock("partial")
  await openProfile()
  await watchWrites()
  await openJourney()
  await fill(
    "#submission-text",
    "I use TypeScript. I led a payments project.",
  )
  await clickText("Prepare changes")
  await until(`!!${byText("Retry processing")}`)
  assert.equal(
    await evaluate(
      "window.__journeyCalls.filter(c=>c.kind==='plan').length",
    ),
    1,
  )
  assert.equal(
    await evaluate(
      "window.__journeyCalls.filter(c=>c.kind==='extract').length",
    ),
    2,
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /I use TypeScript\. I led a payments project\./,
  )
  await clickText("Retry processing")
  await until("!!document.querySelector('#review-heading')")
  assert.equal(
    await evaluate(
      "window.__journeyCalls.filter(c=>c.kind==='plan').length",
    ),
    1,
  )
  assert.equal(
    await evaluate(
      "window.__journeyCalls.filter(c=>c.kind==='extract').length",
    ),
    3,
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Some pasted regions could not be fully processed/,
  )
  await clickText("Select clear additions")
  await until(`${byText("Save to Profile")}.disabled === false`)
  await clickText("Save to Profile")
  await until(`!!${byText("Add more information")}`)
  assert.equal(await evaluate("window.__profileWrites"), 1)
  const partialSaved = JSON.parse(await evaluate(savedProfileExpression))
  assert.match(partialSaved.skills, /TypeScript/)
  assert.match(partialSaved.skills, /Payments leadership/)
  await clickText("Add more information")
  await until("!!document.querySelector('#submission-text')")
  assert.equal(
    await evaluate("document.querySelector('#submission-text').value"),
    "",
  )
  console.log("PASS partial failure/resume, explicit unprocessed notice, one save")

  // Focused clarification stays inside the journey, revises only its claim and
  // keeps the final save explicit.
  await resetStorage(null)
  await installMock("clarification")
  await openProfile()
  await watchWrites()
  await openJourney()
  await fill("#submission-text", "I use TypeScript.")
  await clickText("Prepare changes")
  await until("!!document.querySelector('#review-heading')")
  await evaluate(
    "document.querySelector('[data-review-kind=clarification] details summary').click()",
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Which TypeScript version do you use\?/,
  )
  await evaluate("document.querySelector('[data-review-answer-open]').click()")
  await until("!!document.querySelector('[data-review-answer] textarea')")
  await fill(
    "[data-review-answer] textarea",
    "TypeScript 5 for frontend work.",
  )
  await evaluate("window.__holdClarify = true")
  await evaluate(
    "document.querySelector('[data-review-answer] button[type=submit]').click()",
  )
  await until("window.__clarifyHeld === true")
  assert.equal(
    await evaluate(`${byText("Save to Profile")}.disabled`),
    true,
    "Save must be disabled while the focused clarification is in flight",
  )
  await evaluate("window.__releaseClarify()")
  await until(
    "window.__journeyCalls.filter(c=>c.kind==='clarify').length === 1 && !document.querySelector('[data-review-kind=clarification]')",
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /TypeScript 5 for frontend work/,
  )
  assert.equal(await evaluate("window.__profileWrites"), 0)
  await clickText("Select clear additions")
  await until(`${byText("Save to Profile")}.disabled === false`)
  await clickText("Save to Profile")
  await until(`!!${byText("Add more information")}`)
  assert.equal(await evaluate("window.__profileWrites"), 1)
  const clarifiedSaved = JSON.parse(await evaluate(savedProfileExpression))
  assert.match(clarifiedSaved.skills, /TypeScript 5 for frontend work/)
  console.log("PASS focused clarification without leaving the journey")

  // A same-origin peer writes a newer Profile revision. The review stays visible
  // but unsavable until Reload; then the journey requires Edit/Discard.
  await resetStorage(null)
  await installMock("normal")
  await openProfile()
  await openJourney()
  await fill("#submission-text", "I use TypeScript.")
  await clickText("Prepare changes")
  await until("!!document.querySelector('#review-heading')")
  assert.equal(await evaluate("window.__profileWrites ?? 0"), 0)
  await evaluate(
    `(() => {
      window.__peer = window.open('about:blank', 'profile-peer');
      const doc = JSON.parse(localStorage.getItem('careeros_profile_v2'));
      doc.revision++;
      for (const fact of doc.facts)
        if (fact.owner.id === doc.id) fact.owner.revision = doc.revision;
      window.__peer.localStorage.setItem('careeros_profile_v2', JSON.stringify(doc));
    })()`,
  )
  await until(
    "!!document.querySelector('[data-profile-content] [role=alert]')",
  )
  assert.match(
    await evaluate("document.body.innerText"),
    /another tab/i,
  )
  assert.match(
    await evaluate("document.querySelector('[data-profile-content] [role=alert]').innerText"),
    /Profile changed since these suggestions/i,
  )
  assert.equal(
    await evaluate("!!document.querySelector('#review-heading')"),
    true,
  )
  assert.equal(await evaluate(`${byText("Save to Profile")}.disabled`), true)
  await evaluate("document.querySelector('[data-profile-refresh]').click()")
  await until(
    "document.querySelector('[data-submission-journey]').innerText.includes('can no longer continue')",
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /I use TypeScript\./,
  )
  assert.match(
    await evaluate("document.querySelector('[data-submission-journey]').innerText"),
    /Start a new submission/,
  )
  await clickJourneyDiscard()
  await confirmDiscard()
  await until("!!document.querySelector('#submission-text')")
  assert.equal(
    await evaluate("document.querySelector('#submission-text').value"),
    "",
  )
  await evaluate("window.__peer.close()")
  console.log("PASS stale/cross-tab review, reload, explicit discard")
  console.log("Screenshots:", work)
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
}

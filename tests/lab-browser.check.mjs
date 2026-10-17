// Actual standalone launcher + isolated synthetic browser; external requests denied.
import assert from "node:assert/strict"
import { spawn } from "node:child_process"
import { mkdtempSync, readFileSync, writeFileSync, readdirSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
const root = new URL("..", import.meta.url).pathname
const work = mkdtempSync(join(tmpdir(), "careeros-lab-browser-"))
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const launcher = spawn(
  process.execPath,
  ["scripts/lab.mjs", "--data-dir", join(work, "history")],
  {
    cwd: root,
    stdio: ["ignore", "pipe", "pipe"],
    env: { ...process.env, GOCACHE: "/tmp/careeros-go-cache" },
  },
)
let logs = ""
launcher.stdout.on("data", (value) => (logs += value))
launcher.stderr.on("data", (value) => (logs += value))
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
  let origin
  for (let i = 0; i < 300; i++) {
    origin = /Evaluation Lab: (http:\/\/127\.0\.0\.1:\d+)\/lab\//.exec(
      logs,
    )?.[1]
    if (origin) break
    if (launcher.exitCode !== null)
      throw new Error("Launcher failed: " + logs.slice(-1000))
    await pause(100)
  }
  assert.ok(origin, "one-command local launch did not start")
  let port
  for (let i = 0; i < 100; i++) {
    try {
      port = Number(
        readFileSync(join(work, "chrome", "DevToolsActivePort"), "utf8").split(
          "\n",
        )[0],
      )
      break
    } catch {}
    await pause(100)
  }
  const targets = await (
    await fetch(`http://127.0.0.1:${port}/json/list`)
  ).json()
  socket = new WebSocket(
    targets.find((t) => t.type === "page").webSocketDebuggerUrl,
  )
  await new Promise((resolve, reject) => {
    socket.onopen = resolve
    socket.onerror = reject
  })
  let next = 0,
    mockStatus = null,
    holdPath = ""
  const held = []
  const pending = new Map(),
    external = [],
    starts = [],
    exceptions = []
  const call = (method, params = {}) =>
    new Promise((resolve, reject) => {
      const id = ++next
      pending.set(id, { resolve, reject })
      socket.send(JSON.stringify({ id, method, params }))
    })
  socket.onmessage = ({ data }) => {
    const event = JSON.parse(data)
    if (event.id) {
      const callback = pending.get(event.id)
      pending.delete(event.id)
      if (event.error) callback.reject(new Error(event.error.message))
      else callback.resolve(event.result)
      return
    }
    if (event.method === "Runtime.exceptionThrown")
      exceptions.push(event.params.exceptionDetails.text)
    if (event.method === "Fetch.requestPaused") {
      const request = event.params.request,
        url = request.url
      if (url === origin + "/lab/api/status" && mockStatus) {
        void call("Fetch.fulfillRequest", {
          requestId: event.params.requestId,
          responseCode: 200,
          responseHeaders: [
            { name: "Content-Type", value: "application/json" },
          ],
          body: Buffer.from(JSON.stringify(mockStatus)).toString("base64"),
        })
      } else if (url.startsWith(origin + "/")) {
        if (holdPath && url.includes(holdPath)) {
          held.push(event.params.requestId)
          return
        }
        if (request.method === "POST" && url === origin + "/lab/api/runs")
          starts.push(url)
        void call("Fetch.continueRequest", {
          requestId: event.params.requestId,
        })
      } else {
        external.push(url)
        void call("Fetch.failRequest", {
          requestId: event.params.requestId,
          errorReason: "BlockedByClient",
        })
      }
    }
  }
  const evaluate = async (expression) => {
    const result = await call("Runtime.evaluate", {
      expression,
      returnByValue: true,
      awaitPromise: true,
      userGesture: true,
    })
    if (result.exceptionDetails)
      throw new Error(
        result.exceptionDetails.exception?.description ??
          result.exceptionDetails.text,
      )
    return result.result.value
  }
  const until = async (expression, label = expression, attempts = 200) => {
    for (let i = 0; i < attempts; i++) {
      if (await evaluate(expression)) return
      await pause(25)
    }
    throw new Error(
      "Timed out: " +
        label +
        "; " +
        (await evaluate("document.body.innerText.slice(0,1200)")),
    )
  }
  const button = async (label) =>
    evaluate(
      `(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent===${JSON.stringify(label)});if(!b||b.disabled)throw new Error('Button unavailable');b.click()})()`,
    )
  const input = async (label, value) =>
    evaluate(
      `(()=>{const e=[...document.querySelectorAll('label')].find(l=>l.querySelector('span')?.textContent===${JSON.stringify(label)})?.querySelector('input,select');if(!e)throw new Error('Field unavailable');Object.getOwnPropertyDescriptor(e.tagName==='SELECT'?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value').set.call(e,${JSON.stringify(value)});e.dispatchEvent(new Event(e.tagName==='SELECT'?'change':'input',{bubbles:true}))})()`,
    )
  const choose = async (suite, caseID, configuration = "current") => {
    await input("Suite", suite)
    await input("Configuration", configuration)
    await button("Clear selection")
    await until(
      `!!document.querySelector('input[aria-label="Select ${caseID}"]')`,
    )
    await evaluate(
      `document.querySelector('input[aria-label="Select ${caseID}"]').click()`,
    )
    await button("Preview frozen plan")
    await until(`!!document.querySelector('article[aria-label="Frozen plan"]')`)
    const previous = await evaluate("location.hash")
    await button("Start controlled / offline run")
    await until(
      `location.hash.startsWith('#run=')&&location.hash!==${JSON.stringify(previous)}`,
    )
    const id = await evaluate(`location.hash.slice(5)`)
    await until(
      `document.querySelector('#run-title')?.parentElement.innerText.includes('Engine completed')`,
      "terminal controlled run",
      suite === "test-adapters" ? 3000 : 200,
    )
    return id
  }
  await call("Page.enable")
  await call("Runtime.enable")
  await call("Fetch.enable", { patterns: [{ urlPattern: "*" }] })
  await call("Browser.setDownloadBehavior", {
    behavior: "allow",
    downloadPath: work,
  })
  await call("Page.addScriptToEvaluateOnNewDocument", {
    source: `window.__storageCalls=[];for(const name of ['getItem','setItem','removeItem','clear','key'])Storage.prototype[name]=function(){window.__storageCalls.push(name);throw new Error('Lab attempted browser storage access')}`,
  })
  await call("Emulation.setDeviceMetricsOverride", {
    width: 1440,
    height: 1000,
    deviceScaleFactor: 1,
    mobile: false,
  })
  await call("Page.navigate", { url: origin + "/lab/" })
  await until(`document.body.innerText.includes('Private history ready')`)
  await choose("test-adapters", "node-profile-document")
  await button("node-profile-document")
  await until(`!!document.querySelector('article[aria-label="Case evidence"]')`)
  assert.ok(
    await evaluate(
      `document.querySelector('article[aria-label="Case evidence"]').innerText.includes('operational outcome passed')`,
    ),
    "Node exit result not shown",
  )
  assert.ok(
    await evaluate(
      `document.querySelector('#run-title').parentElement.innerText.includes('Test drivers: 1/1 finished')`,
    ),
    "test-driver progress missing",
  )
  await choose("test-adapters", "browser-resume-review")
  await button("browser-resume-review")
  await until(`!!document.querySelector('[aria-label="Typed test artifacts"]')`)
  assert.ok(
    await evaluate(
      `document.querySelector('[aria-label="Typed test artifacts"]').innerText.includes('cdp-produced-pdf')`,
    ),
    "CDP PDF kind missing",
  )
  await button("Download verified artifact 1")
  for (
    let i = 0;
    i < 100 &&
    !readdirSync(work).some(
      (name) => name === "lab-screenshot.png" || name === "lab-cdp.pdf",
    );
    i++
  )
    await pause(50)
  assert.ok(
    readdirSync(work).some(
      (name) => name === "lab-screenshot.png" || name === "lab-cdp.pdf",
    ),
    "typed artifact download unavailable",
  )
  const en = await choose("semantic-quality", "en-fact")
  assert.equal(
    await evaluate(
      `!!document.querySelector('article[aria-label="Case evidence"]')`,
    ),
    false,
  )
  await button("en-fact")
  await until(`!!document.querySelector('article[aria-label="Case evidence"]')`)
  assert.ok(
    await evaluate(
      `document.body.innerText.includes('Original synthetic source and context')`,
    ),
  )
  assert.ok(
    await evaluate(
      `document.querySelector('article[aria-label="Case evidence"]').innerText.includes('Curated missing / unmatched units')&&document.querySelector('article[aria-label="Case evidence"]').innerText.includes('Captured model outputs / judgments')`,
    ),
    "case evidence facets are not separate",
  )
  await button("Create pending source review")
  await until(
    `[...document.querySelectorAll('[aria-label="Source review worksheet"] button')].some(b=>b.textContent==='Export source review' && !b.disabled)`,
  )
  assert.ok(
    await evaluate(
      `document.querySelector('[aria-label="Source review worksheet"]').innerText.includes('Human approval is disabled')`,
    ),
    "default launch offered human gold approval",
  )
  await button("Export source review")
  for (
    let i = 0;
    i < 100 && !readdirSync(work).includes("source-review.json");
    i++
  )
    await pause(50)
  const worksheet = JSON.parse(
    readFileSync(join(work, "source-review.json"), "utf8"),
  )
  assert.equal(worksheet.version, "lab-source-review-v1")
  assert.equal(worksheet.state, "pending")
  assert.ok(worksheet.binding.sourceBytes)
  await button("Export json")
  for (
    let i = 0;
    i < 100 && !readdirSync(work).includes(`evaluation-${en}.json`);
    i++
  )
    await pause(50)
  const exported = JSON.parse(
    readFileSync(join(work, `evaluation-${en}.json`), "utf8"),
  )
  assert.equal(exported.version, "lab-synthetic-export-v1")
  await button("Export markdown")
  for (
    let i = 0;
    i < 100 && !readdirSync(work).includes(`evaluation-${en}.md`);
    i++
  )
    await pause(50)
  assert.ok(
    readFileSync(join(work, `evaluation-${en}.md`), "utf8").includes("lab-"),
  )
  const beforeReload = starts.length
  await call("Page.reload")
  await until(
    `document.querySelector('#run-title')?.parentElement.innerText.includes('Engine completed')`,
  )
  assert.equal(starts.length, beforeReload, "refresh started another run")
  await input("language", "pt-BR")
  await until(
    `document.querySelector('#history-title')?.parentElement.innerText.includes('No matching stored evidence.')`,
  )
  await input("language", "")
  await until(
    `document.querySelector('#history-title')?.parentElement.innerText.includes('semantic-quality')`,
  )
  const current = await choose("workflow-settings", "en-fact", "current")
  const alternate = await choose("workflow-settings", "en-fact", "luna-low")
  assert.notEqual(current, alternate)
  const openStored = async (id) => {
    await evaluate(
      `(()=>{const row=[...document.querySelectorAll('#history-title ~ .table tbody tr')].find(r=>r.innerText.includes(${JSON.stringify(id)}));if(!row)throw new Error('Stored row unavailable');row.querySelector('button').click()})()`,
    )
    await until(
      `location.hash==='#run='+${JSON.stringify(id)}&&document.querySelector('#run-title')?.parentElement.innerText.includes('Engine completed')`,
    )
  }
  const releaseHeld = async () => {
    holdPath = ""
    for (const requestId of held.splice(0))
      await call("Fetch.continueRequest", { requestId })
    await pause(300)
  }
  await openStored(current)
  holdPath = `/runs/${current}/cases/`
  await button("en-fact")
  for (let i = 0; i < 100 && !held.length; i++) await pause(10)
  assert.equal(held.length, 1, "case request not captured")
  await openStored(alternate)
  await releaseHeld()
  assert.equal(
    await evaluate(
      `!!document.querySelector('article[aria-label="Case evidence"]')`,
    ),
    false,
    "late evidence appeared under another run",
  )
  await input("Left run ID", current)
  await input("Right run ID", alternate)
  await button("Compare safety and omission evidence")
  await until(
    `document.querySelector('#compare-title')?.parentElement.innerText.includes('Compatible finite evidence')`,
  )
  await evaluate(
    `(()=>{const row=[...document.querySelectorAll('#history-title ~ .table tbody tr')].find(r=>r.innerText.includes(${JSON.stringify(en)}));[...row.querySelectorAll('button')].find(b=>b.textContent==='Set left').click()})()`,
  )
  assert.equal(
    await evaluate(
      `document.querySelector('#compare-title')?.parentElement.innerText.includes('Compatible finite evidence')`,
    ),
    false,
    "history selection kept the old comparison",
  )
  await input("Left run ID", current)
  holdPath = "/lab/api/comparisons"
  await button("Compare safety and omission evidence")
  for (let i = 0; i < 100 && !held.length; i++) await pause(10)
  assert.equal(held.length, 1, "comparison request not captured")
  await input("Left run ID", en)
  await releaseHeld()
  assert.equal(
    await evaluate(
      `document.querySelector('#compare-title')?.parentElement.innerText.includes('Compatible finite evidence')`,
    ),
    false,
    "late comparison described previous IDs",
  )
  await input("Right run ID", alternate)
  await button("Compare safety and omission evidence")
  await until(
    `document.querySelector('#compare-title')?.parentElement.innerText.includes('Incompatible:')`,
  )
  await input("Suite", "semantic-quality")
  await button("Clear selection")
  await evaluate(
    `document.querySelector('input[aria-label="Select en-fact"]').click()`,
  )
  holdPath = "/lab/api/plans"
  await button("Preview frozen plan")
  for (let i = 0; i < 100 && !held.length; i++) await pause(10)
  assert.equal(held.length, 1, "plan request not captured")
  await input("Suite", "qualification-evidence")
  await releaseHeld()
  assert.equal(
    await evaluate(
      `!!document.querySelector('article[aria-label="Frozen plan"]')`,
    ),
    false,
    "late plan restored previous composer selection",
  )
  await choose("qualification-evidence", "explicit-support")
  await choose("application-context", "explicit-support")
  await choose("section-review", "en-outage")
  await button("en-outage")
  await until(
    `document.querySelector('article[aria-label="Case evidence"]')?.innerText.includes('502')`,
  )
  // Actual long controlled run: reconnect reads the same admission, then UI cancel.
  await input("Suite", "semantic-quality")
  await button("Select filtered cases")
  await button("Preview frozen plan")
  await until(`!!document.querySelector('article[aria-label="Frozen plan"]')`)
  const previous = await evaluate("location.hash"),
    beforeActive = starts.length
  await button("Start controlled / offline run")
  await until(`location.hash!==${JSON.stringify(previous)}`)
  await call("Page.reload")
  await until(
    `[...document.querySelectorAll('button')].some(b=>b.textContent==='Cancel active run'&&!b.disabled)`,
    "active run after reconnect",
  )
  await button("Cancel active run")
  await until(
    `document.querySelector('#run-title')?.parentElement.innerText.includes('Engine canceled')`,
  )
  assert.equal(
    starts.length,
    beforeActive + 1,
    "active reconnect started another run",
  )
  const setFile = async (file) => {
    const doc = await call("DOM.getDocument")
    const node = await call("DOM.querySelector", {
      nodeId: doc.root.nodeId,
      selector: "input[type=file]",
    })
    await call("DOM.setFileInputFiles", { nodeId: node.nodeId, files: [file] })
  }
  writeFileSync(join(work, "malformed.json"), "{bad")
  await setFile(join(work, "malformed.json"))
  await button("Import selected bytes")
  await until(
    `document.querySelector('[role=alert]')?.innerText.includes('Import is invalid')`,
  )
  writeFileSync(join(work, "unknown.json"), "{}")
  await setFile(join(work, "unknown.json"))
  await button("Import selected bytes")
  await until(
    `document.querySelector('#run-title')?.parentElement.innerText.includes('Quarantined:')`,
  )
  const imported = await evaluate("location.hash.slice(5)")
  await evaluate(
    `[...document.querySelectorAll('summary')].find(e=>e.textContent==='Deliberately delete this evidence').click()`,
  )
  await input("Type exact run ID to confirm deletion", imported)
  await button("Delete confirmed evidence")
  await until(`!document.querySelector('#run-title')`)
  // Synthetic API fault response exercises UI preflight; native I/O failures are
  // independently tested through FileStore/API public seams.
  mockStatus = {
    version: "lab-api-v1",
    storageReady: false,
    usage: {
      bytes: 0,
      files: 0,
      softWarning: false,
      softWarningBytes: 512 * 1024 * 1024,
    },
    suites: [
      {
        id: "semantic-quality",
        available: false,
        reason: "Synthetic missing fixture capability",
      },
    ],
    metricDefinitions: [],
  }
  await button("Refresh storage")
  await until(
    `document.body.innerText.includes('Storage unavailable')&&document.body.innerText.includes('Synthetic missing fixture capability')`,
  )
  assert.ok(
    await evaluate(
      `[...document.querySelectorAll('button')].find(b=>b.textContent==='Preview frozen plan').disabled`,
    ),
  )
  mockStatus = null
  await button("Refresh storage")
  await until(`document.body.innerText.includes('Private history ready')`)
  const key = async (name, code, number) => {
    await call("Input.dispatchKeyEvent", {
      type: "keyDown",
      key: name,
      code,
      windowsVirtualKeyCode: number,
      ...(name === "Enter" ? { text: "\r" } : {}),
    })
    await call("Input.dispatchKeyEvent", {
      type: "keyUp",
      key: name,
      code,
      windowsVirtualKeyCode: number,
    })
  }
  const tabActivate = async (label) => {
    for (let i = 0; i < 100; i++) {
      await key("Tab", "Tab", 9)
      if (
        await evaluate(
          `document.activeElement?.tagName==='BUTTON'&&document.activeElement.textContent===${JSON.stringify(label)}`,
        )
      ) {
        await key("Enter", "Enter", 13)
        return
      }
    }
    throw new Error("Keyboard target unavailable: " + label)
  }
  for (const width of [1440, 390]) {
    await call("Emulation.setDeviceMetricsOverride", {
      width,
      height: 900,
      deviceScaleFactor: 1,
      mobile: false,
    })
    await input("Suite", "semantic-quality")
    await button("Clear selection")
    await evaluate(
      `[...document.querySelectorAll('label')].find(l=>l.querySelector('span')?.textContent==='Case ID contains').querySelector('input').focus();[...document.querySelectorAll('label')].find(l=>l.querySelector('span')?.textContent==='Case ID contains').querySelector('input').select()`,
    )
    await call("Input.insertText", { text: "en-fact" })
    await tabActivate("Select filtered cases")
    await tabActivate("Preview frozen plan")
    await until(`!!document.querySelector('article[aria-label="Frozen plan"]')`)
    const prior = await evaluate("location.hash")
    await tabActivate("Start controlled / offline run")
    await until(`location.hash!==${JSON.stringify(prior)}`)
    await until(
      `document.querySelector('#run-title')?.parentElement.innerText.includes('Engine completed')`,
    )
    assert.ok(
      await evaluate(
        `document.documentElement.scrollWidth<=window.innerWidth+1`,
      ),
      "narrow keyboard flow document overflows",
    )
    await evaluate("window.scrollTo(0,0)")
  }
  for (const width of [390, 1440]) {
    await call("Emulation.setDeviceMetricsOverride", {
      width,
      height: 900,
      deviceScaleFactor: 1,
      mobile: false,
    })
    const screenshot = await call("Page.captureScreenshot", { format: "png" })
    writeFileSync(
      join(work, `dashboard-${width}.png`),
      Buffer.from(screenshot.data, "base64"),
    )
  }
  assert.deepEqual(await evaluate("window.__storageCalls"), [])
  assert.deepEqual(external, [], "Lab requested external network resources")
  assert.deepEqual(exceptions, [], "browser runtime exception")
  console.log(
    "PASS actual Lab launch → fixed Node driver and isolated Resume harness with typed artifact download → selective semantic/settings/retrieval/section runs → lazy detail → JSON/Markdown export → refresh/reconnect without restart → cancel → indexed history filters → compatible/incompatible comparisons → malformed/quarantined imports → deliberate deletion; synthetic UI preflight faults; keyboard and 390px layout; zero browser storage/external requests.",
  )
  console.log("Evidence:", work)
} finally {
  socket?.close()
  chrome.kill("SIGTERM")
  launcher.kill("SIGINT")
  await new Promise((resolve) => {
    if (launcher.exitCode !== null) return resolve()
    const timer = setTimeout(resolve, 15000)
    launcher.once("exit", () => {
      clearTimeout(timer)
      resolve()
    })
  })
  if (launcher.exitCode === null) launcher.kill("SIGKILL")
}

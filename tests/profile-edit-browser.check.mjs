import { savedProfileExpression } from "./profile-browser-storage.mjs"

// Browser regression for raw professional-information review and explicit apply.
// Uses synthetic API responses; no provider request is made.
import assert from "node:assert/strict"
import { createHash } from "node:crypto"
import { spawn } from "node:child_process"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createServer } from "node:net"

const root = new URL("..", import.meta.url).pathname
const work = mkdtempSync(join(tmpdir(), "careeros-manual-browser-"))
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const port = await new Promise((resolve) => {
  const server = createServer()
  server.listen(0, "127.0.0.1", () => {
    const value = server.address().port
    server.close(() => resolve(value))
  })
})
const repo = {
  fullName: "",
  email: "",
  phone: "",
  location: "",
  professionalLinks: "",
  careerGoals: "Product leader focused on useful services.",
  skills: "Research, Product strategy",
  competencies: "",
  tools: "Figma",
  projects: [
    {
      id: "project",
      name: "Service Atlas",
      description: "Mapped service journeys.",
      technologies: "Figma",
      url: "",
      highlights: "Improved task completion.",
    },
  ],
  education: [
    {
      id: "school",
      degree: "BSc Design",
      institution: "East College",
      location: "",
      graduationDate: "2018",
      details: "",
    },
  ],
  certifications: [
    {
      id: "certificate",
      name: "Research Certificate",
      issuer: "Design Guild",
      date: "2020",
      credentialId: "",
      url: "",
    },
  ],
  languages: [{ id: "language", name: "English", proficiency: "Fluent" }],
  employmentStatus: "",
  currentSalary: "",
  desiredSalary: "",
  additionalInfo: "",
  experience: [
    {
      id: "job",
      company: "Harbor Works",
      title: "Product Lead",
      startDate: "2021",
      endDate: "2024",
      current: false,
      location: "",
      description: "Led a customer platform.",
      responsibilities: "",
      achievements: "Improved onboarding.",
    },
  ],
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
  socket.onmessage = ({ data }) => {
    const message = JSON.parse(data)
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
  const evaluate = async (expression) => {
    const result = await call("Runtime.evaluate", {
      expression,
      returnByValue: true,
      awaitPromise: true,
      userGesture: true,
    }).catch((error) => {
      throw new Error(expression + "\n" + error.message)
    })
    if (result.exceptionDetails)
      throw new Error(
        result.exceptionDetails.exception?.description ||
          result.exceptionDetails.text,
      )
    return result.result.value
  }
  const until = async (expression) => {
    for (let attempt = 0; attempt < 100; attempt++) {
      if (await evaluate(expression)) return
      await pause(100)
    }
    throw new Error(
      `Timed out: ${expression}; ${await evaluate("JSON.stringify({text:document.body.innerText.slice(0,1500),injected:typeof window.__originalStorageSet,canonical:localStorage.getItem('careeros_profile_v2')!==null})")}`,
    )
  }
  await call("Page.enable")
  await call("Page.navigate", { url: `http://127.0.0.1:${port}/` })
  await until(
    "document.readyState === 'complete' && !!document.querySelector('header button')",
  )
  await evaluate(
    `localStorage.removeItem('careeros_profile_v2'); localStorage.setItem('careeros_repo', ${JSON.stringify(JSON.stringify(repo))}); localStorage.setItem('careeros_apikey', 'synthetic-test-key'); localStorage.setItem('careeros_typesafe_key','synthetic-typesafe'); location.reload()`,
  )
  await until(
    "document.readyState === 'complete' && !!document.querySelector('header button')",
  )
  const openProfile = async () => {
    await evaluate("document.querySelector('header button:last-child').click()")
    await until("document.querySelectorAll('header nav button').length === 4")
    await evaluate("document.querySelectorAll('header nav button')[1].click()")
    await until("!!document.querySelector('#ingestion-text')")
  }
  const fill = async (value, selector = "#ingestion-text") => {
    await evaluate(
      `(() => { const el = document.querySelector(${JSON.stringify(selector)}); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(el, ${JSON.stringify(value)}); el.dispatchEvent(new Event('input', { bubbles: true })); })()`,
    )
    await pause(60)
  }
  const saved = () => evaluate(savedProfileExpression)
  const byText = (text) =>
    `Array.from(document.querySelectorAll('button')).find(el=>el.textContent.trim()===${JSON.stringify(text)})`

  const canonical = () =>
    evaluate("JSON.parse(localStorage.getItem('careeros_profile_v2'))")
  const sections = [
    "profile",
    "goals",
    "skills",
    "experience",
    "projects",
    "education",
    "certifications",
    "languages",
    "compensation",
    "other",
  ]
  const section = async (name) => {
    await evaluate(
      `document.querySelectorAll('aside nav button')[${sections.indexOf(name)}].click()`,
    )
    await pause(70)
  }
  const reload = async (name) => {
    await evaluate("location.reload()")
    await until(
      "document.readyState === 'complete' && !!document.querySelector('header button')",
    )
    await openProfile()
    await section(name)
  }
  const controls = `Array.from(document.querySelectorAll('[data-profile-content] input:not([type=checkbox]),[data-profile-content] textarea,[data-profile-content] select')).filter(e=>!e.closest('[data-fact-details]'))`
  const setControl = async (index, value) => {
    await evaluate(
      `(() => { const e=(${controls})[${index}]; const proto=e instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:e instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype; Object.getOwnPropertyDescriptor(proto,'value').set.call(e,${JSON.stringify(value)}); e.dispatchEvent(new Event(e instanceof HTMLSelectElement?'change':'input',{bubbles:true})); })()`,
    )
    await pause(90)
  }
  const assertFit = async () =>
    assert.equal(
      await evaluate("document.documentElement.scrollWidth <= innerWidth"),
      true,
    )
  for (const locale of ["en", "pt-BR"])
    for (const width of [1440, 390]) {
      await call("Emulation.setDeviceMetricsOverride", {
        width,
        height: 1000,
        deviceScaleFactor: 1,
        mobile: false,
      })
      await evaluate(
        `localStorage.setItem('careeros_locale',${JSON.stringify(locale)}); localStorage.removeItem('careeros_profile_v2'); localStorage.setItem('careeros_repo',${JSON.stringify(JSON.stringify(repo))}); location.reload()`,
      )
      await until(
        "document.readyState==='complete' && !!document.querySelector('header button')",
      )
      await openProfile()
      const legacyRaw = await evaluate("localStorage.getItem('careeros_repo')")
      // All scalar fields, including contact, compensation and additional information.
      for (const [name, fields] of [
        [
          "profile",
          ["fullName", "email", "phone", "location", "professionalLinks"],
        ],
        ["goals", ["careerGoals"]],
        ["skills", ["skills", "competencies", "tools"]],
        [
          "compensation",
          ["employmentStatus", "currentSalary", "desiredSalary"],
        ],
        ["other", ["additionalInfo"]],
      ]) {
        await section(name)
        for (let i = 0; i < fields.length; i++) {
          const value =
            fields[i] === "employmentStatus"
              ? "student"
              : `${locale}: No Java; aspire to Go; approximately 2020 (${fields[i]})`
          await setControl(i, value)
          assert.equal(JSON.parse(await saved())[fields[i]], value)
        }
        await reload(name)
        for (let i = 0; i < fields.length; i++)
          assert.equal(
            await evaluate(`(${controls})[${i}].value`),
            JSON.parse(await saved())[fields[i]],
          )
        await assertFit()
        // Remove individual fields through explicit fact controls, then recreate via existing form.
        await evaluate(
          "document.querySelector('[data-fact-details]').open=true",
        )
        const doc = await canonical()
        const f = doc.facts.find(
          (f) => f.owner.id === doc.id && f.field === fields[fields.length - 1],
        )
        await evaluate(
          `document.querySelector('[data-fact-id="${f.id}"]').open=true; document.querySelector('[data-fact-id="${f.id}"] button').click()`,
        )
        await pause(100)
        assert.equal(JSON.parse(await saved())[fields[fields.length - 1]], "")
        await reload(name)
        await setControl(
          fields.length - 1,
          "Recreated; meaning remains uncertain",
        )
        await reload(name)
        assert.equal(
          JSON.parse(await saved())[fields[fields.length - 1]],
          "Recreated; meaning remains uncertain",
        )
      }
      for (const name of [
        "experience",
        "projects",
        "education",
        "certifications",
        "languages",
      ]) {
        await section(name)
        console.log("Checking", locale, width, name)
        await until("!!document.querySelector('[data-profile-content] button')")
        const count = JSON.parse(await saved())[name].length
        // First section button outside the details panel is Add.
        await evaluate(
          "Array.from(document.querySelectorAll('[data-profile-content] button')).find(e=>!e.closest('[data-fact-details]')).click()",
        )
        await pause(100)
        assert.equal(JSON.parse(await saved())[name].length, count + 1)
        await setControl(0, "New uncertain qualification")
        await setControl(1, "Unknown institution / employer")
        await reload(name)
        assert.equal(
          await evaluate(`(${controls})[0].value`),
          "New uncertain qualification",
        )
        await setControl(0, "Edited; no claimed qualification")
        await reload(name)
        assert.equal(
          await evaluate(`(${controls})[0].value`),
          "Edited; no claimed qualification",
        )
        await assertFit()
        const remove = locale === "en" ? "Remove" : "Remover"
        await evaluate(`${byText(remove)}.click()`)
        await pause(100)
        assert.equal(JSON.parse(await saved())[name].length, count)
        await reload(name)
        assert.equal(JSON.parse(await saved())[name].length, count)
      }
      // Real supported fixture at the public storage boundary; the editor must invalidate it.
      await evaluate(
        `(async()=>{const {readProfile}=await import('/src/lib/profileStorage.ts');const d=readProfile(localStorage);const f=d.facts.find(f=>f.field==='skills');f.approval='approved';f.support='supported';const ref=id=>({profileId:d.id,id,revision:1});d.evidence.push({id:'support-e',revision:1,excerpt:'Research, Product strategy',origin:'approved career notes',approval:'approved'});d.links.push({id:'support-l',kind:'supports',from:{profileId:d.id,id:f.id,revision:f.revision},to:ref('support-e'),state:'active'});localStorage.setItem('careeros_profile_v2',JSON.stringify(d))})()`,
      )
      await reload("skills")
      let doc = await canonical()
      const fact = doc.facts.find((f) => f.field === "skills")
      await setControl(0, "No Java; hope to learn Go")
      doc = await canonical()
      assert.equal(
        doc.facts.find((f) => f.id === fact.id).revision,
        fact.revision + 1,
      )
      assert.equal(
        doc.links.find((l) => l.id === "support-l").state,
        "invalidated",
      )
      await evaluate(
        `document.querySelector('[data-fact-details]').open=true; document.querySelector('[data-fact-id="${fact.id}"]').open=true`,
      )
      const choose = async (index, value) => {
        await evaluate(
          `(()=>{const e=document.querySelectorAll('[data-fact-id="${fact.id}"] select')[${index}];Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value').set.call(e,${JSON.stringify(value)});e.dispatchEvent(new Event('change',{bubbles:true}))})()`,
        )
        await pause(100)
      }
      await choose(0, "negated")
      await choose(1, "aspiration")
      await choose(2, "uncertain")
      await choose(3, "approximate")
      const role = doc.entities.find((e) => e.kind === "experience")
      const project = doc.entities.find((e) => e.kind === "projects")
      await choose(4, role.id)
      await choose(5, project.id)
      doc = await canonical()
      assert.equal(
        doc.links.filter((l) => l.from.id === fact.id && l.state === "active")
          .length,
        2,
      )
      await choose(4, "")
      doc = await canonical()
      assert.equal(
        doc.links.filter((l) => l.from.id === fact.id && l.state === "active")
          .length,
        1,
      )
      assert.equal(
        doc.links.find(
          (l) =>
            l.from.id === fact.id &&
            l.kind === "project_context" &&
            l.state === "active",
        ).to.id,
        project.id,
      )
      await reload("skills")
      doc = await canonical()
      const edited = doc.facts.find((f) => f.id === fact.id)
      assert.equal(edited.assertion, "negated")
      assert.equal(edited.intent, "aspiration")
      assert.equal(edited.certainty, "uncertain")
      assert.equal(edited.temporal.precision, "approximate")
      await evaluate(
        `document.querySelector('[data-fact-details]').open=true; document.querySelector('[data-fact-id="${fact.id}"]').open=true`,
      )
      await assertFit()
      const screenshot = await call("Page.captureScreenshot", {
        format: "png",
        captureBeyondViewport: false,
      })
      writeFileSync(
        join(work, `manual-${locale}-${width}.png`),
        Buffer.from(screenshot.data, "base64"),
      )
      // Quota failure keeps bytes intact and draft/qualifiers visible for recovery.
      const raw = await evaluate("localStorage.getItem('careeros_profile_v2')")
      await evaluate(
        "window.__set=Storage.prototype.setItem;Storage.prototype.setItem=function(k,v){if(k==='careeros_profile_v2')throw new DOMException('quota','QuotaExceededError');return window.__set.call(this,k,v)}",
      )
      await setControl(0, "Unsaved manual wording")
      await until("!!document.querySelector('[role=alert]')")
      assert.equal(
        await evaluate("localStorage.getItem('careeros_profile_v2')"),
        raw,
      )
      assert.equal(
        await evaluate(`(${controls})[0].value`),
        "Unsaved manual wording",
      )
      assert.equal(
        await evaluate("localStorage.getItem('careeros_repo')"),
        legacyRaw,
      )
      await evaluate("Storage.prototype.setItem=window.__set")
      await reload("skills")
      assert.equal(
        await evaluate(`(${controls})[0].value`),
        "No Java; hope to learn Go",
      )
      // A sibling tab installs a new revision. Local manual input survives and cannot overwrite it.
      await evaluate("window.__peer=window.open(location.href,'manual-peer'); void 0")
      await until(
        "!!window.__peer && window.__peer.document.readyState==='complete'",
      )
      await evaluate(
        `window.__peer.eval("const d=JSON.parse(localStorage.getItem('careeros_profile_v2'));d.revision++;d.facts.forEach(f=>{if(f.owner.id===d.id)f.owner.revision=d.revision});localStorage.setItem('careeros_profile_v2',JSON.stringify(d))")`,
      )
      await until("!!document.querySelector('[role=alert]')")
      const peerRaw = await evaluate(
        "localStorage.getItem('careeros_profile_v2')",
      )
      await setControl(0, "Local stale draft")
      assert.equal(
        await evaluate(`(${controls})[0].value`),
        "Local stale draft",
      )
      assert.equal(
        await evaluate("localStorage.getItem('careeros_profile_v2')"),
        peerRaw,
      )
      await evaluate("window.__peer.close()")
      await reload("skills")
      assert.equal(
        await evaluate(`(${controls})[0].value`),
        "No Java; hope to learn Go",
      )
      console.log(
        `PASS all manual sections, metadata, support, quota and cross-tab ${locale} ${width}px`,
      )
    }
  console.log("Screenshots:", work)
} finally {
  socket?.close()
  chrome.kill()
  vite.kill()
}
